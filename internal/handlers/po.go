package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type POHandler struct {
	db *pgxpool.Pool
}

func NewPOHandler(db *pgxpool.Pool) *POHandler {
	return &POHandler{db: db}
}

// normalizeDescription trims a PO line description, maps "" to nil, and enforces a max length.
func normalizeDescription(d *string) (*string, error) {
	if d == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*d)
	if s == "" {
		return nil, nil
	}
	if len(s) > 1000 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "description must be at most 1000 characters")
	}
	return &s, nil
}

// poLinePRRef is the minimal shape needed to reconcile purchase_request_line.qty_ordered
// against a set of purchase_order_line rows (existing or proposed).
type poLinePRRef struct {
	PRLineID *int64
	Qty      float64
}

// reconcilePRLineQty accumulates purchase_request_line.qty_ordered by the net delta between
// oldLines (what was on the PO before this save, empty for a brand-new PO) and newLines (what
// the PO line set is being saved as now), grouped by pr_line_id. A pr_line_id present on both
// sides only moves by the difference — e.g. 50 -> 60 on the same line only consumes 10 more of
// the PR line's remaining qty, it doesn't re-consume all 60.
//
// Touched PR lines are locked with SELECT ... FOR UPDATE first so two concurrent PO saves
// against the same PR line can't both pass the remaining check and jointly over-order it. Any
// net increase is rejected with 400 if it would push qty_ordered past qty_to_order; net
// decreases (or a PO line's pr_line_id being removed/reduced) are always allowed. Must be
// called inside the same transaction as the purchase_order_line insert/delete it reconciles.
//
// The bound check compares this call's NEW TOTAL for a pr_line_id (newSum) against the max it
// could hold (qty_to_order minus every OTHER PO's claim, i.e. current qty_ordered net of this
// call's own OLD contribution) — not the raw delta against a raw remaining. Comparing delta
// against (qty_to_order - qty_ordered) directly double-counts this call's own prior contribution
// as already "spent," which falsely blocks (or, in an earlier broken attempt at this fix, falsely
// allows) growing a PO line that already holds part of a PR line shared with other POs.
func reconcilePRLineQty(ctx context.Context, tx pgx.Tx, oldLines, newLines []poLinePRRef) error {
	deltas := map[int64]float64{}
	oldSum := map[int64]float64{}
	newSum := map[int64]float64{}
	for _, l := range oldLines {
		if l.PRLineID != nil {
			deltas[*l.PRLineID] -= l.Qty
			oldSum[*l.PRLineID] += l.Qty
		}
	}
	for _, l := range newLines {
		if l.PRLineID != nil {
			deltas[*l.PRLineID] += l.Qty
			newSum[*l.PRLineID] += l.Qty
		}
	}
	if len(deltas) == 0 {
		return nil
	}

	prLineIDs := make([]int64, 0, len(deltas))
	for id := range deltas {
		prLineIDs = append(prLineIDs, id)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, qty_to_order, qty_ordered FROM purchase_request_line
		WHERE id = ANY($1)
		FOR UPDATE`, prLineIDs)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "lock error: "+err.Error())
	}
	current := map[int64][2]float64{}
	for rows.Next() {
		var id int64
		var qtyReq, qtyOrd float64
		if err := rows.Scan(&id, &qtyReq, &qtyOrd); err != nil {
			rows.Close()
			return err
		}
		current[id] = [2]float64{qtyReq, qtyOrd}
	}
	rows.Close()

	for _, id := range prLineIDs {
		vals, ok := current[id]
		if !ok {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("pr_line_id %d not found", id))
		}
		delta := deltas[id]
		if delta > 0 {
			othersClaim := vals[1] - oldSum[id]
			maxAllowedForMe := vals[0] - othersClaim
			newTotalForMe := newSum[id]
			if newTotalForMe-maxAllowedForMe > 1e-9 {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf(
					"pr_line_id %d: qty exceeds remaining (requesting %.4f more, only %.4f remaining)",
					id, delta, maxAllowedForMe-oldSum[id]))
			}
		}
	}

	for id, delta := range deltas {
		if delta == 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE purchase_request_line SET qty_ordered = qty_ordered + $1 WHERE id = $2`,
			delta, id,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "pr line update error: "+err.Error())
		}
	}
	return nil
}

// ListPO godoc
// @Summary      List purchase orders
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        status    query  string  false  "status filter"
// @Param        supplier  query  string  false  "supplier_id filter"
// @Param        my        query  bool    false  "when true, only POs created by the current user"
// @Param        page      query  int     false  "page"  default(1)
// @Param        page_size query  int     false  "page_size"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /po [get]
func (h *POHandler) List(c *fiber.Ctx) error {
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	var conditions []string
	var args []any
	if c.QueryBool("my", false) {
		claims := middleware.GetClaims(c)
		if claims == nil {
			return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
		}
		args = append(args, claims.UserID)
		conditions = append(conditions, fmt.Sprintf("po.created_by = $%d", len(args)))
	}
	if v := strings.TrimSpace(c.Query("status")); v != "" {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("po.status = $%d", len(args)))
	}

	var where string
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	h.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM purchase_order po `+where, args...).Scan(&total)

	args = append(args, size, offset)
	rows, err := h.db.Query(context.Background(), `
		SELECT po.id, po.po_no, po.po_date, po.supplier_id, s.supplier_name,
		       po.status, po.status_receive, po.order_type, po.work_type, po.currency, po.total_amount, po.vat_amount, po.net_amount,
		       po.expected_date::text, po.created_at, po.updated_at, po.project_code,
		       COALESCE(cu.full_name, '') AS created_by_name,
		       COALESCE(uu.full_name, '') AS updated_by_name,
		       (SELECT COUNT(*) FROM po_edit_log pel WHERE pel.po_id = po.id) AS revision_round
		FROM purchase_order po
		LEFT JOIN supplier s ON s.id = po.supplier_id
		LEFT JOIN users cu ON cu.id = po.created_by
		LEFT JOIN users uu ON uu.id = po.updated_by
		`+where+`
		ORDER BY po.created_at DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type PORow struct {
		POID          int64     `json:"po_id"`
		PONo          string    `json:"po_no"`
		PODate        time.Time `json:"po_date"`
		SupplierID    *int64    `json:"supplier_id,omitempty"`
		SupplierName  *string   `json:"supplier_name,omitempty"`
		Status        string    `json:"status"`
		StatusReceive string    `json:"status_receive"`
		OrderType     string    `json:"order_type"`
		WorkType      *string   `json:"work_type,omitempty"`
		Currency      string    `json:"currency"`
		TotalAmount   float64   `json:"total_amount"`
		VATAmount     float64   `json:"vat_amount"`
		NetAmount     float64   `json:"net_amount"`
		ExpectedDate  *string   `json:"expected_date,omitempty"`
		CreatedAt     time.Time `json:"created_at"`
		UpdatedAt     time.Time `json:"updated_at"`
		ProjectCode   *string   `json:"project_code,omitempty"`
		CreatedByName string    `json:"created_by_name"`
		UpdatedByName string    `json:"updated_by_name"`
		JobNames      []string  `json:"job_names,omitempty"`
		// RevisionRound is how many times this PO has been edited-and-resent for re-approval
		// (COUNT of po_edit_log rows). 0 = original, never edited. po_no itself never changes;
		// the frontend composes a display suffix like "#R2" from this when > 0.
		RevisionRound int `json:"revision_round"`
	}

	var items []PORow
	for rows.Next() {
		var r PORow
		if err := rows.Scan(&r.POID, &r.PONo, &r.PODate, &r.SupplierID, &r.SupplierName,
			&r.Status, &r.StatusReceive, &r.OrderType, &r.WorkType, &r.Currency, &r.TotalAmount, &r.VATAmount, &r.NetAmount,
			&r.ExpectedDate, &r.CreatedAt, &r.UpdatedAt, &r.ProjectCode,
			&r.CreatedByName, &r.UpdatedByName, &r.RevisionRound); err != nil {
			return err
		}
		items = append(items, r)
	}

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data":    models.PaginatedResponse{Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages},
	})
}

// ListLineItems godoc
// @Summary      List approved POs with their line items (grouped by PO)
// @Description  Cross-PO report — every PO with status=APPROVED (approval status only; status_receive is
// @Description  not considered), one object per PO with a nested "lines" array. mat_code filter is applied
// @Description  at the line level: only lines matching the filter appear in "lines", and a PO is dropped
// @Description  from the result entirely if none of its lines match. Pagination (page/page_size)
// @Description  paginates POs, not lines.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        date_from    query  string  false  "po_date >= (YYYY-MM-DD)"
// @Param        date_to      query  string  false  "po_date <= (YYYY-MM-DD)"
// @Param        po_no        query  string  false  "po_no ILIKE filter"
// @Param        mat_code     query  string  false  "mat_code ILIKE filter — only matching lines are kept, POs with no match are excluded"
// @Param        requested_by query  int     false  "requested_by user id filter"
// @Param        project_code query  string  false  "po.project_code filter"
// @Param        page         query  int     false  "page (paginates POs)"  default(1)
// @Param        page_size    query  int     false  "page_size (POs per page)"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /po/line-items [get]
func (h *POHandler) ListLineItems(c *fiber.Ctx) error {
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	// "PO is complete" = status = 'APPROVED' (approval flow only). status_receive tracks
	// receiving separately and is NOT part of this filter — confirmed 2026-08-10.
	var conditions []string
	var args []any
	conditions = append(conditions, "po.status = 'APPROVED'")

	// mat_code is a line-level filter: a PO qualifies only if it has at least one
	// line matching it, and only those matching lines are returned.
	// lineFilterSQL holds the raw "col op ?" fragments (placeholder-free); lineFilterArgs
	// holds their values in the same order, reused verbatim for both the header EXISTS
	// clause and the later per-line query.
	var lineFilterSQL []string
	var lineFilterArgs []any

	if v := strings.TrimSpace(c.Query("date_from")); v != "" {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("po.po_date >= $%d", len(args)))
	}
	if v := strings.TrimSpace(c.Query("date_to")); v != "" {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("po.po_date <= $%d", len(args)))
	}
	if v := strings.TrimSpace(c.Query("po_no")); v != "" {
		args = append(args, "%"+v+"%")
		conditions = append(conditions, fmt.Sprintf("po.po_no ILIKE $%d", len(args)))
	}
	if v := strings.TrimSpace(c.Query("mat_code")); v != "" {
		lineFilterArgs = append(lineFilterArgs, "%"+v+"%")
		lineFilterSQL = append(lineFilterSQL, "pol.mat_code ILIKE")
	}
	if v := c.QueryInt("requested_by", 0); v > 0 {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("po.requested_by = $%d", len(args)))
	}
	if v := strings.TrimSpace(c.Query("project_code")); v != "" {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("po.project_code = $%d", len(args)))
	}

	if len(lineFilterSQL) > 0 {
		existsConds := make([]string, len(lineFilterSQL))
		for i, col := range lineFilterSQL {
			args = append(args, lineFilterArgs[i])
			existsConds[i] = fmt.Sprintf("%s $%d", col, len(args))
		}
		conditions = append(conditions, fmt.Sprintf(
			`EXISTS (
				SELECT 1 FROM purchase_order_line pol
				WHERE pol.po_id = po.id AND %s
			)`, strings.Join(existsConds, " AND ")))
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	var total int64
	countSQL := `SELECT COUNT(*) FROM purchase_order po ` + where
	if err := h.db.QueryRow(context.Background(), countSQL, args...).Scan(&total); err != nil {
		return err
	}

	poArgs := append(append([]any{}, args...), size, offset)
	poRows, err := h.db.Query(context.Background(), `
		SELECT po.id, po.po_no, po.po_date, po.supplier_id, s.supplier_name, s.contact_phone,
		       COALESCE(u.full_name, ''), po.project_code, po.status,
		       po.total_amount, po.discount_amount, po.vat_amount, po.net_amount, po.currency
		FROM purchase_order po
		LEFT JOIN users    u ON u.id = po.requested_by
		LEFT JOIN supplier s ON s.id = po.supplier_id
		`+where+`
		ORDER BY po.po_date DESC, po.po_no DESC
		LIMIT $`+strconv.Itoa(len(poArgs)-1)+` OFFSET $`+strconv.Itoa(len(poArgs)), poArgs...)
	if err != nil {
		return err
	}
	defer poRows.Close()

	type LineItemRow struct {
		MatCode    string  `json:"mat_code"`
		MatName    *string `json:"mat_name,omitempty"`
		QtyOrdered float64 `json:"qty_ordered"`
		UnitPrice  float64 `json:"unit_price"`
		Amount     float64 `json:"amount"`
	}

	type POGroup struct {
		POID           int64         `json:"po_id"`
		PONo           string        `json:"po_no"`
		PODate         time.Time     `json:"po_date"`
		SupplierID     *int64        `json:"supplier_id,omitempty"`
		SupplierName   *string       `json:"supplier_name,omitempty"`
		ContactPhone   *string       `json:"contact_phone,omitempty"`
		RequestedBy    string        `json:"requested_by"`
		ProjectCode    *string       `json:"project_code,omitempty"`
		Status         string        `json:"status"`
		TotalAmount    float64       `json:"total_amount"`
		DiscountAmount float64       `json:"discount_amount"`
		VATAmount      float64       `json:"vat_amount"`
		NetAmount      float64       `json:"net_amount"`
		Currency       string        `json:"currency"`
		Lines          []LineItemRow `json:"lines"`
	}

	var groups []POGroup
	poIDs := make([]int64, 0, size)
	groupByID := make(map[int64]*POGroup, size)
	for poRows.Next() {
		var g POGroup
		if err := poRows.Scan(&g.POID, &g.PONo, &g.PODate, &g.SupplierID, &g.SupplierName, &g.ContactPhone,
			&g.RequestedBy, &g.ProjectCode, &g.Status,
			&g.TotalAmount, &g.DiscountAmount, &g.VATAmount, &g.NetAmount, &g.Currency); err != nil {
			return err
		}
		g.Lines = []LineItemRow{}
		groups = append(groups, g)
		poIDs = append(poIDs, g.POID)
	}
	for i := range groups {
		groupByID[groups[i].POID] = &groups[i]
	}

	if len(poIDs) > 0 {
		lineQueryArgs := []any{poIDs}
		lineQueryConds := []string{"pol.po_id = ANY($1)"}
		for i, col := range lineFilterSQL {
			lineQueryArgs = append(lineQueryArgs, lineFilterArgs[i])
			lineQueryConds = append(lineQueryConds, fmt.Sprintf("%s $%d", col, len(lineQueryArgs)))
		}
		lineQueryWhere := strings.Join(lineQueryConds, " AND ")
		lineRows, err := h.db.Query(context.Background(), `
			SELECT pol.po_id, pol.mat_code, mn.mat_name, pol.qty_ordered, pol.unit_price, pol.amount
			FROM purchase_order_line pol
			LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
			LEFT JOIN mat_name      mn ON mn.id = mc.mat_name_id
			WHERE `+lineQueryWhere+`
			ORDER BY pol.po_id, pol.line_no`, lineQueryArgs...)
		if err != nil {
			return err
		}
		defer lineRows.Close()

		for lineRows.Next() {
			var poID int64
			var l LineItemRow
			if err := lineRows.Scan(&poID, &l.MatCode, &l.MatName, &l.QtyOrdered, &l.UnitPrice, &l.Amount); err != nil {
				return err
			}
			if g, ok := groupByID[poID]; ok {
				g.Lines = append(g.Lines, l)
			}
		}
	}

	// EXISTS above already guarantees each returned PO has >=1 matching line, but filter
	// defensively in case of any edge-case divergence between the two queries.
	filtered := groups[:0]
	for _, g := range groups {
		if len(g.Lines) > 0 {
			filtered = append(filtered, g)
		}
	}
	groups = filtered

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data":    models.PaginatedResponse{Data: groups, Total: total, Page: page, PageSize: size, TotalPages: totalPages},
	})
}

// GetPO godoc
// @Summary      Get purchase order by ID
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {object}  models.PurchaseOrder
// @Failure      404  {object}  fiber.Map
// @Router       /po/{id} [get]
func (h *POHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	row := h.db.QueryRow(context.Background(), `
		SELECT po.id, po.po_no, po.po_date, po.supplier_id, po.pr_id, po.rfq_id, po.ref,
		       po.location_text, po.warehouse_code, po.project_code,
		       po.requested_by, u.full_name, po.approver_id,
		       po.currency, po.total_amount, po.vat_amount, po.net_amount,
		       po.expected_date::text,
		       po.use_discount, po.discount_type, po.discount_amount,
		       po.use_vat, po.use_wht, po.wht_amount,
		       po.status, po.status_receive, po.order_type, po.work_type, po.payment_terms, po.remarks,
		       po.created_by, po.created_at, po.updated_at,
		       COALESCE(s.supplier_name, ''),
		       s.office_phone, s.sales_person,
		       s.contact_email, s.contact_phone,
		       pr.pr_no, w.address,
		       (SELECT COUNT(*) FROM po_edit_log pel WHERE pel.po_id = po.id)
		FROM purchase_order po
		LEFT JOIN users u ON u.id = po.requested_by
		LEFT JOIN supplier s ON s.id = po.supplier_id
		LEFT JOIN purchase_request pr ON pr.id = po.pr_id
		LEFT JOIN warehouse w ON w.warehouse_code = po.warehouse_code
		WHERE po.id = $1`, id)

	var po models.PurchaseOrder
	var requestedByName *string
	if err := row.Scan(&po.POID, &po.PONo, &po.PODate, &po.SupplierID, &po.PRID,
		&po.RFQID, &po.Ref, &po.LocationText, &po.WarehouseCode, &po.ProjectCode,
		&po.RequestedBy, &requestedByName, &po.ApproverID,
		&po.Currency, &po.TotalAmount, &po.VATAmount, &po.NetAmount,
		&po.ExpectedDate,
		&po.UseDiscount, &po.DiscountType, &po.DiscountAmount,
		&po.UseVAT, &po.UseWHT, &po.WHTAmount,
		&po.Status, &po.StatusReceive, &po.OrderType, &po.WorkType, &po.PaymentTerms, &po.Remarks,
		&po.CreatedBy, &po.CreatedAt, &po.UpdatedAt,
		&po.SupplierName,
		&po.OfficePhone, &po.SalesPerson,
		&po.ContactEmail, &po.ContactPhone,
		&po.PRNo, &po.WarehouseAddress,
		&po.RevisionRound); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if requestedByName != nil {
		po.RequestedByName = *requestedByName
	}
	po.CanEditApproved = po.Status == "APPROVED" && time.Since(po.CreatedAt) < 365*24*time.Hour

	rows, _ := h.db.Query(context.Background(), `
		SELECT pol.id, pol.po_id, pol.line_no, pol.mat_code, pol.pr_line_id, pol.qty_ordered, pol.qty_received,
		       pol.unit_price, pol.disc_type, pol.discount, pol.line_discount, pol.line_vat, pol.line_wht,
		       pol.line_net, pol.wht_rate, pol.amount, pol.description, pol.remarks, pol.status,
		       mn.mat_name, ss.spec_description, b.brand_name,
		       COALESCE(si.qty, 0), prl.qty_reserved
		FROM purchase_order_line pol
		LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
		LEFT JOIN mat_name      mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size     ss ON ss.id = mc.spec_id
		LEFT JOIN brand         b  ON b.id  = mc.brand_id
		LEFT JOIN stock_item    si  ON si.mat_code = pol.mat_code AND si.warehouse_code = $2
		LEFT JOIN purchase_request_line prl ON prl.id = pol.pr_line_id
		WHERE pol.po_id=$1 ORDER BY pol.line_no`, id, po.WarehouseCode)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var l models.POLine
			rows.Scan(&l.LineID, &l.POID, &l.LineNo, &l.MatCode, &l.PRLineID,
				&l.QtyOrdered, &l.QtyReceived, &l.UnitPrice, &l.DiscType, &l.Discount, &l.LineDiscount,
				&l.LineVAT, &l.LineWHT, &l.LineNet, &l.WhtRate, &l.Amount, &l.Description, &l.Remarks, &l.Status,
				&l.MatName, &l.SpecDescription, &l.BrandName, &l.CurrentStock, &l.PRLineQtyReserved)
			po.Lines = append(po.Lines, l)
		}
	}

	poAttRows, err := h.db.Query(context.Background(), `
		SELECT id, po_id, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at
		FROM po_attachment WHERE po_id=$1 AND source_pr_attachment_id IS NULL
		ORDER BY uploaded_at`, id)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch po attachments: "+err.Error())
	}
	poAtts := []models.POAttachment{}
	for poAttRows.Next() {
		var a models.POAttachment
		if err := poAttRows.Scan(&a.ID, &a.POID, &a.FileName, &a.FilePath, &a.FileSize, &a.FileType, &a.UploadedBy, &a.UploadedAt); err != nil {
			poAttRows.Close()
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan po attachments: "+err.Error())
		}
		poAtts = append(poAtts, a)
	}
	poAttRows.Close()
	po.Attachments.PO = poAtts

	if po.PRID != nil {
		prAttRows, err := h.db.Query(context.Background(), `
			SELECT id, pr_id, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at
			FROM pr_attachment WHERE pr_id=$1 ORDER BY uploaded_at`, *po.PRID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch pr attachments: "+err.Error())
		}
		prAtts := []models.PRAttachment{}
		for prAttRows.Next() {
			var a models.PRAttachment
			if err := prAttRows.Scan(&a.ID, &a.PRID, &a.FileName, &a.FilePath, &a.FileSize, &a.FileType, &a.UploadedBy, &a.UploadedAt); err != nil {
				prAttRows.Close()
				return fiber.NewError(fiber.StatusInternalServerError, "failed to scan pr attachments: "+err.Error())
			}
			prAtts = append(prAtts, a)
		}
		prAttRows.Close()
		po.Attachments.PR = &prAtts

		var memoID *int64
		if err := h.db.QueryRow(context.Background(), `SELECT memo_id FROM purchase_request WHERE id=$1`, *po.PRID).Scan(&memoID); err != nil && err != pgx.ErrNoRows {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch pr memo_id: "+err.Error())
		}

		if memoID != nil {
			memoAttRows, err := h.db.Query(context.Background(), `
				SELECT id, memo_id, file_path, file_name, file_size, file_type, uploaded_by, uploaded_at
				FROM memo_attachment WHERE memo_id=$1 ORDER BY uploaded_at`, *memoID)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch memo attachments: "+err.Error())
			}
			memoAtts := []models.MemoAttachment{}
			for memoAttRows.Next() {
				var a models.MemoAttachment
				if err := memoAttRows.Scan(&a.ID, &a.MemoID, &a.FilePath, &a.FileName, &a.FileSize, &a.FileType, &a.UploadedBy, &a.UploadedAt); err != nil {
					memoAttRows.Close()
					return fiber.NewError(fiber.StatusInternalServerError, "failed to scan memo attachments: "+err.Error())
				}
				memoAtts = append(memoAtts, a)
			}
			memoAttRows.Close()
			po.Attachments.Memo = &memoAtts
		}
	}

	return c.JSON(fiber.Map{"success": true, "data": po})
}

// resolvePOAutoFields determines project_code, requested_by and warehouse_code for a PO
// being created or updated. When prID is set, all three are always taken server-side from
// the referenced PR (which must be COMPLETED, and every pr_line_id in lines must belong to
// it) — client-supplied values are ignored on that path. When prID is nil, the client-supplied
// values are used as-is after validating project_code/warehouse_code against their tables.
func (h *POHandler) resolvePOAutoFields(
	ctx context.Context, prID *int64, reqProjectCode *string, reqRequestedBy *int64,
	reqWarehouseCode *string, lines []models.CreatePOLine,
) (projectCode *string, requestedBy *int64, warehouseCode *string, err error) {
	if prID != nil {
		var prStatus string
		if err := h.db.QueryRow(ctx, `SELECT status, project_code, requested_by, warehouse_code FROM purchase_request WHERE id=$1`, *prID).Scan(&prStatus, &projectCode, &requestedBy, &warehouseCode); err != nil {
			return nil, nil, nil, fiber.NewError(fiber.StatusBadRequest, "PR not found")
		}
		if prStatus != "COMPLETED" {
			return nil, nil, nil, fiber.NewError(fiber.StatusBadRequest, "PR must be COMPLETED before creating a PO")
		}

		rows, err := h.db.Query(ctx, `SELECT id FROM purchase_request_line WHERE pr_id=$1`, *prID)
		if err != nil {
			return nil, nil, nil, err
		}
		validLines := map[int64]bool{}
		for rows.Next() {
			var lineID int64
			rows.Scan(&lineID)
			validLines[lineID] = true
		}
		rows.Close()
		for i, l := range lines {
			if l.PRLineID != nil && !validLines[*l.PRLineID] {
				return nil, nil, nil, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: pr_line_id does not belong to pr_id %d", i, *prID))
			}
		}
		return projectCode, requestedBy, warehouseCode, nil
	}

	if reqProjectCode != nil && strings.TrimSpace(*reqProjectCode) != "" {
		var exists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code=$1)`, *reqProjectCode).Scan(&exists); err != nil {
			return nil, nil, nil, err
		}
		if !exists {
			return nil, nil, nil, fiber.NewError(fiber.StatusBadRequest, "invalid project_code")
		}
		projectCode = reqProjectCode
	}
	if reqWarehouseCode != nil && strings.TrimSpace(*reqWarehouseCode) != "" {
		var exists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM warehouse WHERE warehouse_code=$1)`, *reqWarehouseCode).Scan(&exists); err != nil {
			return nil, nil, nil, err
		}
		if !exists {
			return nil, nil, nil, fiber.NewError(fiber.StatusBadRequest, "invalid warehouse_code")
		}
		warehouseCode = reqWarehouseCode
	}
	requestedBy = reqRequestedBy
	return projectCode, requestedBy, warehouseCode, nil
}

// CreatePO godoc
// @Summary      Create purchase order
// @Description  Creates a PO as DRAFT or PENDING_APPROVAL. If pr_id is set, the referenced PR must be COMPLETED and any pr_line_id must belong to it. Line totals and the PO total are always computed server-side. Submitting with status=PENDING_APPROVAL opens a step-1 approval_request.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreatePORequest  true  "PO data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /po [post]
func (h *POHandler) Create(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	var req models.CreatePORequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if req.SupplierID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "supplier_id is required")
	}
	if req.LocationText == "" {
		return fiber.NewError(fiber.StatusBadRequest, "location_text is required")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line required")
	}
	for i, l := range req.Lines {
		if l.MatCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: mat_code is required", i))
		}
		if l.QtyOrdered <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: qty_ordered must be > 0", i))
		}
		if l.UnitPrice <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: unit_price must be > 0", i))
		}
	}
	{
		matCodes := make([]string, len(req.Lines))
		for i, l := range req.Lines {
			matCodes[i] = l.MatCode
		}
		if err := validateMatCodesExist(context.Background(), h.db, matCodes); err != nil {
			return err
		}
	}
	if req.ApproverID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "approver_id is required")
	}

	status := req.Status
	if status == "" {
		status = "DRAFT"
	}
	if status != "DRAFT" && status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "status must be DRAFT or PENDING_APPROVAL")
	}
	if req.Currency == "" {
		req.Currency = "THB"
	}

	if req.OrderType == "" {
		req.OrderType = "stock"
	}
	if req.OrderType != "stock" && req.OrderType != "cost" {
		return fiber.NewError(fiber.StatusBadRequest, "order_type must be 'stock' or 'cost'")
	}
	if req.OrderType == "cost" && (req.PRID == nil || *req.PRID == 0) {
		return fiber.NewError(fiber.StatusBadRequest, "pr_id is required when order_type is 'cost'")
	}

	ctx := context.Background()

	// Note: a PR may be linked to more than one active PO — split ordering lets a single
	// PR line be divided across multiple POs/suppliers. reconcilePRLineQty (below, inside
	// the transaction) is what actually prevents over-ordering, at the pr_line_id level.

	var approverExists bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *req.ApproverID,
	).Scan(&approverExists); err != nil {
		return err
	}
	if !approverExists {
		return fiber.NewError(fiber.StatusBadRequest, "approver not found")
	}

	projectCode, requestedBy, warehouseCode, err := h.resolvePOAutoFields(ctx, req.PRID, req.ProjectCode, req.RequestedBy, req.WarehouseCode, req.Lines)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	poNo, err := nextPONumber(ctx, tx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate PO number: "+err.Error())
	}

	useDiscount := req.UseDiscount != nil && *req.UseDiscount
	useVAT := req.UseVAT != nil && *req.UseVAT
	useWHT := req.UseWHT != nil && *req.UseWHT
	discountType := "pct"
	if req.DiscountType != nil && *req.DiscountType != "" {
		discountType = *req.DiscountType
	}

	// Totals and per-line discount/VAT/WHT are always computed server-side, never
	// trusted from the request — only the use_discount/discount_type/use_vat/use_wht
	// flags and per-line discount/wht_rate inputs come from the client.
	type lineCalc struct {
		base, discAmt, afterDisc, vatAmt, whtAmt, net float64
		discType                                      string
	}
	calcs := make([]lineCalc, len(req.Lines))
	var totalAmount, discountAmount, whtAmount float64
	for i, l := range req.Lines {
		lc := lineCalc{base: l.QtyOrdered * l.UnitPrice}
		lc.discType = l.DiscType
		if lc.discType == "" {
			lc.discType = "pct"
		}
		if useDiscount && l.Discount != 0 {
			if lc.discType == "amt" {
				lc.discAmt = l.Discount
			} else {
				lc.discAmt = lc.base * l.Discount / 100
			}
		}
		lc.afterDisc = lc.base - lc.discAmt
		if useVAT {
			lc.vatAmt = lc.afterDisc * 0.07
		}
		if useWHT && l.WhtRate != nil {
			lc.whtAmt = lc.afterDisc * (*l.WhtRate) / 100
		}
		lc.net = lc.afterDisc + lc.vatAmt - lc.whtAmt
		calcs[i] = lc

		totalAmount += lc.base
		discountAmount += lc.discAmt
		whtAmount += lc.whtAmt
	}
	vatAmount := totalAmount - discountAmount
	if useVAT {
		vatAmount *= 0.07
	} else {
		vatAmount = 0
	}
	netAmount := totalAmount - discountAmount + vatAmount - whtAmount

	var poID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO purchase_order
		  (po_no, po_date, supplier_id, pr_id, rfq_id, location_text, warehouse_code, project_code, requested_by, approver_id, ref, currency,
		   total_amount, vat_amount, net_amount, expected_date,
		   use_discount, discount_type, discount_amount, use_vat, use_wht, wht_amount,
		   status, order_type, work_type, payment_terms, remarks, created_by)
		VALUES ($1,CURRENT_DATE,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
		RETURNING id`,
		poNo, req.SupplierID, req.PRID, req.RFQID, req.LocationText, warehouseCode, projectCode, requestedBy, req.ApproverID, req.Ref, req.Currency,
		totalAmount, vatAmount, netAmount, req.ExpectedDate,
		useDiscount, discountType, discountAmount, useVAT, useWHT, whtAmount,
		status, req.OrderType, req.WorkType, req.PaymentTerms, req.Remarks, claims.UserID,
	).Scan(&poID)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
			detail := "invalid supplier_id, pr_id, rfq_id, warehouse_code or project_code"
			if pgErr.ConstraintName != "" {
				detail += " (constraint: " + pgErr.ConstraintName + ")"
			}
			return fiber.NewError(fiber.StatusBadRequest, detail)
		}
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create PO: "+err.Error())
	}

	for i, line := range req.Lines {
		desc, err := normalizeDescription(line.Description)
		if err != nil {
			return err
		}
		lc := calcs[i]
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_order_line
			  (po_id, line_no, mat_code, pr_line_id, qty_ordered, unit_price, disc_type,
			   discount, line_discount, line_vat, line_wht, line_net, wht_rate,
			   description, remarks, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'OPEN')`,
			poID, i+1, line.MatCode, line.PRLineID, line.QtyOrdered, line.UnitPrice, lc.discType,
			line.Discount, lc.discAmt, lc.vatAmt, lc.whtAmt, lc.net, line.WhtRate,
			desc, line.Remarks,
		); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: invalid mat_code or pr_line_id", i))
			}
			return fiber.NewError(fiber.StatusInternalServerError, "line insert error: "+err.Error())
		}
	}

	{
		newRefs := make([]poLinePRRef, 0, len(req.Lines))
		for _, l := range req.Lines {
			if l.PRLineID != nil {
				newRefs = append(newRefs, poLinePRRef{PRLineID: l.PRLineID, Qty: l.QtyOrdered})
			}
		}
		if err := reconcilePRLineQty(ctx, tx, nil, newRefs); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,NULL,$2,$3,'PO created')`, poID, status, claims.UserID,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "status log error: "+err.Error())
	}

	// Submitting straight to PENDING_APPROVAL opens the step-1 approval request.
	var approvalID *int64
	if status == "PENDING_APPROVAL" {
		var hasConfig bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM approval_config WHERE doc_type='PO' AND step_no=1 AND is_active=true)`,
		).Scan(&hasConfig); err != nil {
			return err
		}
		if hasConfig {
			var id int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status, amount)
				VALUES ('PO',$1,$2,1,$3,$4,'PENDING',$5)
				RETURNING id`, poID, poNo, claims.UserID, req.ApproverID, totalAmount,
			).Scan(&id); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "approval request error: "+err.Error())
			}
			approvalID = &id
		}
	}

	// Copy PR attachments onto the new PO — same physical file on disk, a second
	// reference row so PO detail can show them without touching pr_attachment.
	if req.PRID != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO po_attachment (po_id, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at, source_pr_attachment_id)
			SELECT $1, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at, id
			FROM pr_attachment WHERE pr_id = $2`, poID, *req.PRID,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to copy PR attachments: "+err.Error())
		}
	}

	auditData, _ := json.Marshal(fiber.Map{
		"po_no": poNo, "status": status, "supplier_id": req.SupplierID,
		"pr_id": req.PRID, "total_amount": totalAmount,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO erp_audit_log (table_name, record_id, action, changed_by, new_data)
		VALUES ('purchase_order',$1,'INSERT',$2,$3)`, poID, claims.UserID, auditData,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "audit log error: "+err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	data := fiber.Map{
		"po_id": poID, "po_no": poNo, "status": status,
		"total_amount": totalAmount, "vat_amount": vatAmount, "net_amount": netAmount,
	}
	if approvalID != nil {
		data["approval_request_id"] = *approvalID
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": data})
}

// GetAvailablePRs godoc
// @Summary      List PRs eligible to be linked to a new PO
// @Description  Returns COMPLETED PRs that still have at least one line with remaining (qty_to_order - qty_ordered) > 0. A PR can appear here even if it already has an active PO — split ordering lets the same PR line be divided across multiple POs/suppliers as long as some quantity is left; see GET /po/pr-lines/{pr_id} for the per-line remaining breakdown.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /po/available-prs [get]
func (h *POHandler) GetAvailablePRs(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(), `
		SELECT
		    pr.id,
		    pr.pr_no,
		    pr.status,
		    pr.requested_by,
		    u.full_name AS requested_by_name,
		    pr.created_at
		FROM purchase_request pr
		LEFT JOIN users u ON u.id = pr.requested_by
		WHERE pr.status = 'COMPLETED'
		  AND EXISTS (
		      SELECT 1 FROM purchase_request_line prl
		      WHERE prl.pr_id = pr.id
		      AND prl.qty_to_order - prl.qty_ordered > 0
		  )
		ORDER BY pr.created_at DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type AvailablePR struct {
		PRID            int64     `json:"pr_id"`
		PRNo            string    `json:"pr_no"`
		Status          string    `json:"status"`
		RequestedBy     *int64    `json:"requested_by,omitempty"`
		RequestedByName *string   `json:"requested_by_name,omitempty"`
		CreatedAt       time.Time `json:"created_at"`
	}

	items := []AvailablePR{}
	for rows.Next() {
		var r AvailablePR
		if err := rows.Scan(&r.PRID, &r.PRNo, &r.Status, &r.RequestedBy, &r.RequestedByName, &r.CreatedAt); err != nil {
			return err
		}
		items = append(items, r)
	}

	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetPRLinesForPO godoc
// @Summary      List a PR's lines with remaining orderable qty
// @Description  Returns every line of the given PR with a computed remaining = qty_to_order - qty_ordered, so the PO-create form can show accurate "still orderable" quantities per line when building a split order (qty already covered by stock via qty_reserved is excluded from what's orderable). Lines that are fully ordered (remaining <= 0) are included with is_fully_ordered=true rather than dropped, so the UI can show them struck out instead of silently disappearing.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        pr_id  path  int  true  "PR ID"
// @Success      200    {object}  fiber.Map
// @Router       /po/pr-lines/{pr_id} [get]
func (h *POHandler) GetPRLinesForPO(c *fiber.Ctx) error {
	prID, err := strconv.ParseInt(c.Params("pr_id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid pr_id")
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT prl.id, prl.pr_id, prl.line_no, prl.mat_code, mn.mat_name,
		       prl.qty_requested, prl.qty_reserved, prl.qty_to_order, prl.qty_ordered,
		       (prl.qty_to_order - prl.qty_ordered) AS remaining, prl.status, prl.remarks
		FROM purchase_request_line prl
		LEFT JOIN material_code mc ON mc.mat_code = prl.mat_code
		LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
		WHERE prl.pr_id = $1
		ORDER BY prl.line_no`, prID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type PRLineAvailable struct {
		ID             int64   `json:"id"`
		PRID           int64   `json:"pr_id"`
		LineNo         int     `json:"line_no"`
		MatCode        string  `json:"mat_code"`
		MatName        *string `json:"mat_name,omitempty"`
		QtyRequested   float64 `json:"qty_requested"`
		QtyReserved    float64 `json:"qty_reserved"`
		QtyToOrder     float64 `json:"qty_to_order"`
		QtyOrdered     float64 `json:"qty_ordered"`
		Remaining      float64 `json:"remaining"`
		Status         string  `json:"status"`
		Remarks        *string `json:"remarks,omitempty"`
		IsFullyOrdered bool    `json:"is_fully_ordered"`
	}

	items := []PRLineAvailable{}
	for rows.Next() {
		var r PRLineAvailable
		if err := rows.Scan(&r.ID, &r.PRID, &r.LineNo, &r.MatCode, &r.MatName,
			&r.QtyRequested, &r.QtyReserved, &r.QtyToOrder, &r.QtyOrdered, &r.Remaining, &r.Status, &r.Remarks); err != nil {
			return err
		}
		if r.Remaining <= 0 {
			r.IsFullyOrdered = true
		}
		items = append(items, r)
	}

	return c.JSON(fiber.Map{"success": true, "data": items})
}

// UpdatePO godoc
// @Summary      Update a DRAFT purchase order
// @Description  Replaces header fields and all lines of a PO that is still in DRAFT status. Not usable once the PO has left DRAFT (use PUT /po/{id}/edit-approved for APPROVED POs; other statuses cannot be edited). Line totals and the PO total are always recomputed server-side. Same pr_id/project_code/requested_by/warehouse_code auto-fill rules as POST /po.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                     true  "PO ID"
// @Param        body  body  models.CreatePORequest  true  "PO data"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /po/{id} [put]
func (h *POHandler) Update(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	poID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PO id")
	}

	var req models.CreatePORequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if req.SupplierID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "supplier_id is required")
	}
	if req.LocationText == "" {
		return fiber.NewError(fiber.StatusBadRequest, "location_text is required")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line required")
	}
	for i, l := range req.Lines {
		if l.MatCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: mat_code is required", i))
		}
		if l.QtyOrdered <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: qty_ordered must be > 0", i))
		}
		if l.UnitPrice <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: unit_price must be > 0", i))
		}
	}
	{
		matCodes := make([]string, len(req.Lines))
		for i, l := range req.Lines {
			matCodes[i] = l.MatCode
		}
		if err := validateMatCodesExist(context.Background(), h.db, matCodes); err != nil {
			return err
		}
	}
	if req.ApproverID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "approver_id is required")
	}
	if req.Currency == "" {
		req.Currency = "THB"
	}

	ctx := context.Background()

	var approverExists bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *req.ApproverID,
	).Scan(&approverExists); err != nil {
		return err
	}
	if !approverExists {
		return fiber.NewError(fiber.StatusBadRequest, "approver not found")
	}

	var currentStatus, currentOrderType string
	if err := h.db.QueryRow(ctx, `SELECT status, order_type FROM purchase_order WHERE id=$1`, poID).Scan(&currentStatus, &currentOrderType); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if currentStatus != "DRAFT" && currentStatus != "PENDING_APPROVAL" {
		detail := fmt.Sprintf("PO must be DRAFT or PENDING_APPROVAL to use this endpoint (current status: %s)", currentStatus)
		if currentStatus == "APPROVED" {
			detail += " — use PUT /po/{id}/edit-approved instead"
		}
		return fiber.NewError(fiber.StatusBadRequest, detail)
	}

	// Default to the PO's current status, not a hardcoded "DRAFT" — otherwise editing a
	// PENDING_APPROVAL PO without the client explicitly re-sending status would silently
	// downgrade it back to DRAFT. Editing while PENDING_APPROVAL is meant to leave status
	// untouched (see status-log/approval_request guards below, which key off newStatus ==
	// currentStatus to avoid re-logging/re-opening an approval that's already in flight).
	newStatus := req.Status
	if newStatus == "" {
		newStatus = currentStatus
	}
	if newStatus != "DRAFT" && newStatus != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "status must be DRAFT or PENDING_APPROVAL")
	}

	orderType := req.OrderType
	if orderType == "" {
		orderType = currentOrderType
	}
	if orderType != "stock" && orderType != "cost" {
		return fiber.NewError(fiber.StatusBadRequest, "order_type must be 'stock' or 'cost'")
	}
	if orderType == "cost" && (req.PRID == nil || *req.PRID == 0) {
		return fiber.NewError(fiber.StatusBadRequest, "pr_id is required when order_type is 'cost'")
	}

	projectCode, requestedBy, warehouseCode, err := h.resolvePOAutoFields(ctx, req.PRID, req.ProjectCode, req.RequestedBy, req.WarehouseCode, req.Lines)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Totals and per-line discount/VAT/WHT are always computed server-side, never
	// trusted from the request — only the use_discount/discount_type/use_vat/use_wht
	// flags and per-line discount/wht_rate inputs come from the client. Mirrors
	// Create's calculation exactly; Update previously hardcoded a flat 7% VAT here
	// and dropped discount/wht_rate from the line re-insert below entirely, which
	// silently reset a DRAFT's tax/discount panel settings back to defaults on
	// every save-after-the-first.
	useDiscount := req.UseDiscount != nil && *req.UseDiscount
	useVAT := req.UseVAT != nil && *req.UseVAT
	useWHT := req.UseWHT != nil && *req.UseWHT
	discountType := "pct"
	if req.DiscountType != nil && *req.DiscountType != "" {
		discountType = *req.DiscountType
	}

	type lineCalc struct {
		base, discAmt, afterDisc, vatAmt, whtAmt, net float64
		discType                                      string
	}
	calcs := make([]lineCalc, len(req.Lines))
	var totalAmount, discountAmount, whtAmount float64
	for i, l := range req.Lines {
		lc := lineCalc{base: l.QtyOrdered * l.UnitPrice}
		lc.discType = l.DiscType
		if lc.discType == "" {
			lc.discType = "pct"
		}
		if useDiscount && l.Discount != 0 {
			if lc.discType == "amt" {
				lc.discAmt = l.Discount
			} else {
				lc.discAmt = lc.base * l.Discount / 100
			}
		}
		lc.afterDisc = lc.base - lc.discAmt
		if useVAT {
			lc.vatAmt = lc.afterDisc * 0.07
		}
		if useWHT && l.WhtRate != nil {
			lc.whtAmt = lc.afterDisc * (*l.WhtRate) / 100
		}
		lc.net = lc.afterDisc + lc.vatAmt - lc.whtAmt
		calcs[i] = lc

		totalAmount += lc.base
		discountAmount += lc.discAmt
		whtAmount += lc.whtAmt
	}
	vatAmount := totalAmount - discountAmount
	if useVAT {
		vatAmount *= 0.07
	} else {
		vatAmount = 0
	}
	netAmount := totalAmount - discountAmount + vatAmount - whtAmount

	if _, err := tx.Exec(ctx, `
		UPDATE purchase_order SET
		    supplier_id=$1, pr_id=$2, rfq_id=$3, location_text=$4, warehouse_code=$5, project_code=$6, requested_by=$7, approver_id=$8, ref=$9,
		    currency=$10, expected_date=$11, payment_terms=$12, remarks=$13,
		    total_amount=$14, vat_amount=$15, net_amount=$16, status=$17,
		    use_discount=$18, discount_type=$19, discount_amount=$20, use_vat=$21, use_wht=$22, wht_amount=$23,
		    order_type=$24, work_type=$25,
		    updated_at=NOW(), updated_by=$26
		WHERE id=$27`,
		req.SupplierID, req.PRID, req.RFQID, req.LocationText, warehouseCode, projectCode, requestedBy, req.ApproverID, req.Ref,
		req.Currency, req.ExpectedDate, req.PaymentTerms, req.Remarks,
		totalAmount, vatAmount, netAmount, newStatus,
		useDiscount, discountType, discountAmount, useVAT, useWHT, whtAmount,
		orderType, req.WorkType,
		claims.UserID, poID,
	); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
			detail := "invalid supplier_id, pr_id, rfq_id, warehouse_code or project_code"
			if pgErr.ConstraintName != "" {
				detail += " (constraint: " + pgErr.ConstraintName + ")"
			}
			return fiber.NewError(fiber.StatusBadRequest, detail)
		}
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update PO: "+err.Error())
	}

	oldRefs := []poLinePRRef{}
	{
		oldRows, err := tx.Query(ctx, `SELECT pr_line_id, qty_ordered FROM purchase_order_line WHERE po_id=$1`, poID)
		if err != nil {
			return err
		}
		for oldRows.Next() {
			var prLineID *int64
			var qty float64
			if err := oldRows.Scan(&prLineID, &qty); err != nil {
				oldRows.Close()
				return err
			}
			if prLineID != nil {
				oldRefs = append(oldRefs, poLinePRRef{PRLineID: prLineID, Qty: qty})
			}
		}
		oldRows.Close()
	}

	if _, err := tx.Exec(ctx, `DELETE FROM purchase_order_line WHERE po_id=$1`, poID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to clear old lines: "+err.Error())
	}

	for i, line := range req.Lines {
		desc, err := normalizeDescription(line.Description)
		if err != nil {
			return err
		}
		lc := calcs[i]
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_order_line
			  (po_id, line_no, mat_code, pr_line_id, qty_ordered, unit_price, disc_type,
			   discount, line_discount, line_vat, line_wht, line_net, wht_rate,
			   description, remarks, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'OPEN')`,
			poID, i+1, line.MatCode, line.PRLineID, line.QtyOrdered, line.UnitPrice, lc.discType,
			line.Discount, lc.discAmt, lc.vatAmt, lc.whtAmt, lc.net, line.WhtRate,
			desc, line.Remarks,
		); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: invalid mat_code or pr_line_id", i))
			}
			return fiber.NewError(fiber.StatusInternalServerError, "line insert error: "+err.Error())
		}
	}

	{
		newRefs := make([]poLinePRRef, 0, len(req.Lines))
		for _, l := range req.Lines {
			if l.PRLineID != nil {
				newRefs = append(newRefs, poLinePRRef{PRLineID: l.PRLineID, Qty: l.QtyOrdered})
			}
		}
		if err := reconcilePRLineQty(ctx, tx, oldRefs, newRefs); err != nil {
			return err
		}
	}

	if newStatus != currentStatus {
		if _, err := tx.Exec(ctx, `
			INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
			VALUES ($1,$2,$3,$4,'PO updated')`, poID, currentStatus, newStatus, claims.UserID,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "status log error: "+err.Error())
		}
	}

	// Submitting to PENDING_APPROVAL opens the step-1 approval request, same as POST /po — but
	// only on the transition INTO PENDING_APPROVAL. Editing a PO that's already
	// PENDING_APPROVAL (newStatus == currentStatus) must not open a second approval_request
	// alongside the one already PENDING from the original submit.
	var approvalID *int64
	if newStatus == "PENDING_APPROVAL" && currentStatus != "PENDING_APPROVAL" {
		var hasConfig bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM approval_config WHERE doc_type='PO' AND step_no=1 AND is_active=true)`,
		).Scan(&hasConfig); err != nil {
			return err
		}
		if hasConfig {
			var poNo string
			if err := tx.QueryRow(ctx, `SELECT po_no FROM purchase_order WHERE id=$1`, poID).Scan(&poNo); err != nil {
				return err
			}
			var id int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status, amount)
				VALUES ('PO',$1,$2,1,$3,$4,'PENDING',$5)
				RETURNING id`, poID, poNo, claims.UserID, req.ApproverID, totalAmount,
			).Scan(&id); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "approval request error: "+err.Error())
			}
			approvalID = &id
		}
	}

	auditData, _ := json.Marshal(fiber.Map{
		"supplier_id": req.SupplierID,
		"pr_id":       req.PRID, "total_amount": totalAmount, "status": newStatus,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO erp_audit_log (table_name, record_id, action, changed_by, new_data)
		VALUES ('purchase_order',$1,'UPDATE',$2,$3)`, poID, claims.UserID, auditData,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "audit log error: "+err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	data := fiber.Map{
		"po_id": poID, "status": newStatus,
		"total_amount": totalAmount, "vat_amount": vatAmount, "net_amount": netAmount,
	}
	if approvalID != nil {
		data["approval_request_id"] = *approvalID
	}
	return c.JSON(fiber.Map{"success": true, "data": data})
}

// NextPONumber godoc
// @Summary      Preview the next PO number
// @Description  Read-only preview of the next po_no, computed the same way as POST /po's real generation. This is NOT reserved — it's not written or locked anywhere, so two users previewing at the same time may see the same value; only whoever actually saves first gets it, since POST /po recomputes fresh inside its own transaction at save time. Purely cosmetic for the create-page hint.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /po/next-number [get]
func (h *POHandler) NextPONumber(c *fiber.Ctx) error {
	now := time.Now()
	prefix := fmt.Sprintf("PO-%d%02d-", now.Year(), int(now.Month()))

	var lastNo string
	err := h.db.QueryRow(context.Background(), `
		SELECT po_no FROM purchase_order
		WHERE po_no LIKE $1 AND status NOT IN ('CANCELLED')
		ORDER BY po_no DESC LIMIT 1`, prefix+"%").Scan(&lastNo)

	seq := 1
	if err == nil {
		parts := strings.Split(lastNo, "-")
		if n, convErr := strconv.Atoi(parts[len(parts)-1]); convErr == nil {
			seq = n + 1
		}
	} else if err != pgx.ErrNoRows {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"next_number": fmt.Sprintf("%s%04d", prefix, seq)},
	})
}

// nextPONumber returns the next sequential PO number for the current month, e.g. PO-202506-0001.
func nextPONumber(ctx context.Context, tx pgx.Tx) (string, error) {
	now := time.Now()
	prefix := fmt.Sprintf("PO-%d%02d-", now.Year(), int(now.Month()))

	var lastNo string
	err := tx.QueryRow(ctx, `
		SELECT po_no FROM purchase_order
		WHERE po_no LIKE $1 AND status NOT IN ('CANCELLED')
		ORDER BY po_no DESC LIMIT 1`, prefix+"%").Scan(&lastNo)

	seq := 1
	if err == nil {
		parts := strings.Split(lastNo, "-")
		if n, convErr := strconv.Atoi(parts[len(parts)-1]); convErr == nil {
			seq = n + 1
		}
	} else if err != pgx.ErrNoRows {
		return "", err
	}

	return fmt.Sprintf("%s%04d", prefix, seq), nil
}

// ApprovePO godoc
// SUPERSEDED: see also POApprovalHandler.Approve (po_approval.go) — there were previously
// two independent PO approve implementations on the same /po/{id}/approve path (POST here,
// PUT there). Both are now superseded by PUT /approval/PO/{id}/approve (generic_approval.go).
// Left in place, still routed at POST /po/{id}/approve, only for rollback safety.
// @Summary      Approve or reject PO (Manager/Director/MD)
// @Tags         Purchase Order
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                          true  "PO ID"
// @Param        body  body  models.ApprovalActionRequest true  "action"
// @Success      200  {object}  fiber.Map
// @Router       /po/{id}/approve [post]
func (h *POHandler) Approve(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	id := c.Params("id")

	var req models.ApprovalActionRequest
	c.BodyParser(&req)

	var currentStatus, poNo string
	h.db.QueryRow(context.Background(), `SELECT status, po_no FROM purchase_order WHERE id=$1`, id).Scan(&currentStatus, &poNo)
	if currentStatus != "PENDING_APPROVAL" && currentStatus != "PENDING_REAPPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not pending approval")
	}

	newStatus := "APPROVED"
	if req.Action == "REJECT" {
		newStatus = "REJECTED"
	}

	tx, _ := h.db.Begin(context.Background())
	defer tx.Rollback(context.Background())

	tx.Exec(context.Background(), `UPDATE purchase_order SET status=$1, updated_at=NOW() WHERE id=$2`, newStatus, id)
	tx.Exec(context.Background(), `
		INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,$3,$4,$5)`, id, currentStatus, newStatus, claims.UserID, req.Action)

	// Create approval_log
	var approvalID int64
	h.db.QueryRow(context.Background(), `
		SELECT id FROM approval_request
		WHERE doc_type='PO' AND doc_id=$1 AND status='PENDING'
		ORDER BY created_at DESC LIMIT 1`, id).Scan(&approvalID)

	if approvalID > 0 {
		approvalStatus := "APPROVED"
		if req.Action == "REJECT" {
			approvalStatus = "REJECTED"
		}
		tx.Exec(context.Background(), `UPDATE approval_request SET status=$1 WHERE id=$2`, approvalStatus, approvalID)
		tx.Exec(context.Background(), `
			INSERT INTO approval_log (approval_id, doc_type, doc_id, doc_no, step_no, action, action_by, comments, old_status, new_status)
			VALUES ($1,'PO',$2,$3,1,$4,$5,$6,$7,$8)`,
			approvalID, id, poNo, req.Action, claims.UserID, req.Comments, currentStatus, newStatus)
	}

	tx.Commit(context.Background())
	return c.JSON(fiber.Map{"success": true, "message": fmt.Sprintf("PO %s", newStatus)})
}

// EditApprovedPO godoc
// @Summary      Edit an already-approved PO (requires reason, triggers re-approval)
// @Description  Allows editing a PO with status=APPROVED or PENDING_REAPPROVAL, provided the PO's created_at is less than 1 year old. Requires a mandatory non-empty reason, logged to po_edit_log. Replaces header fields and all lines, recomputes totals server-side, sets status to PENDING_REAPPROVAL, and reopens the PO's existing approval_request row (same approval_id / assigned_to — not a fresh approver lookup). Idempotent while already PENDING_REAPPROVAL: repeated saves before the approver acts just keep re-editing and re-pending the same approval_request, so the frontend can call this endpoint for every save in this flow, not just the first.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                          true  "PO ID"
// @Param        body  body  models.EditApprovedPORequest true  "Updated PO data + reason"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /po/{id}/edit-approved [put]
func (h *POHandler) EditApprovedPO(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	poID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PO id")
	}

	var req models.EditApprovedPORequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Reason) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "reason is required")
	}
	if req.SupplierID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "supplier_id is required")
	}
	if req.LocationText == "" {
		return fiber.NewError(fiber.StatusBadRequest, "location_text is required")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line required")
	}
	for i, l := range req.Lines {
		if l.MatCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: mat_code is required", i))
		}
		if l.QtyOrdered <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: qty_ordered must be > 0", i))
		}
		if l.UnitPrice <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: unit_price must be > 0", i))
		}
	}
	{
		matCodes := make([]string, len(req.Lines))
		for i, l := range req.Lines {
			matCodes[i] = l.MatCode
		}
		if err := validateMatCodesExist(context.Background(), h.db, matCodes); err != nil {
			return err
		}
	}
	if req.Currency == "" {
		req.Currency = "THB"
	}

	ctx := context.Background()

	if req.ProjectCode != nil && strings.TrimSpace(*req.ProjectCode) != "" {
		var exists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code=$1)`, *req.ProjectCode).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, "invalid project_code")
		}
	}

	var currentStatus string
	var createdAt time.Time
	if err := h.db.QueryRow(ctx, `SELECT status, created_at FROM purchase_order WHERE id=$1`, poID).Scan(&currentStatus, &createdAt); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if currentStatus != "APPROVED" && currentStatus != "PENDING_REAPPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("PO must be APPROVED or PENDING_REAPPROVAL to use this endpoint (current status: %s)", currentStatus))
	}
	if time.Since(createdAt) >= 365*24*time.Hour {
		return fiber.NewError(fiber.StatusBadRequest, "PO นี้สร้างมาเกิน 1 ปีแล้ว ไม่สามารถแก้ไขได้")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Totals and per-line discount/VAT/WHT are always computed server-side, never trusted
	// from the request — mirrors Create/Update's calculation exactly. This previously
	// hardcoded a flat 7% VAT here and dropped discount/wht_rate/line_discount/line_vat/
	// line_wht/line_net from the line re-insert below entirely, which silently reset an
	// APPROVED PO's tax/discount panel settings back to defaults on every edit-approved save
	// — the exact bug already fixed once in Update (see its comment), recurring here because
	// this is a separate code path.
	useDiscount := req.UseDiscount != nil && *req.UseDiscount
	useVAT := req.UseVAT != nil && *req.UseVAT
	useWHT := req.UseWHT != nil && *req.UseWHT
	discountType := "pct"
	if req.DiscountType != nil && *req.DiscountType != "" {
		discountType = *req.DiscountType
	}

	type lineCalc struct {
		base, discAmt, afterDisc, vatAmt, whtAmt, net float64
		discType                                      string
	}
	calcs := make([]lineCalc, len(req.Lines))
	var totalAmount, discountAmount, whtAmount float64
	for i, l := range req.Lines {
		lc := lineCalc{base: l.QtyOrdered * l.UnitPrice}
		lc.discType = l.DiscType
		if lc.discType == "" {
			lc.discType = "pct"
		}
		if useDiscount && l.Discount != 0 {
			if lc.discType == "amt" {
				lc.discAmt = l.Discount
			} else {
				lc.discAmt = lc.base * l.Discount / 100
			}
		}
		lc.afterDisc = lc.base - lc.discAmt
		if useVAT {
			lc.vatAmt = lc.afterDisc * 0.07
		}
		if useWHT && l.WhtRate != nil {
			lc.whtAmt = lc.afterDisc * (*l.WhtRate) / 100
		}
		lc.net = lc.afterDisc + lc.vatAmt - lc.whtAmt
		calcs[i] = lc

		totalAmount += lc.base
		discountAmount += lc.discAmt
		whtAmount += lc.whtAmt
	}
	vatAmount := totalAmount - discountAmount
	if useVAT {
		vatAmount *= 0.07
	} else {
		vatAmount = 0
	}
	netAmount := totalAmount - discountAmount + vatAmount - whtAmount

	if _, err := tx.Exec(ctx, `
		UPDATE purchase_order SET
		    supplier_id=$1, location_text=$2, project_code=COALESCE($3, project_code), requested_by=COALESCE($4, requested_by),
		    warehouse_code=COALESCE($5, warehouse_code), currency=$6, expected_date=$7,
		    payment_terms=$8, remarks=$9, total_amount=$10, vat_amount=$11, net_amount=$12,
		    use_discount=$13, discount_type=$14, discount_amount=$15, use_vat=$16, use_wht=$17, wht_amount=$18,
		    status='PENDING_REAPPROVAL', updated_at=NOW(), updated_by=$19
		WHERE id=$20`,
		req.SupplierID, req.LocationText, req.ProjectCode, req.RequestedBy, req.WarehouseCode, req.Currency, req.ExpectedDate,
		req.PaymentTerms, req.Remarks, totalAmount, vatAmount, netAmount,
		useDiscount, discountType, discountAmount, useVAT, useWHT, whtAmount,
		claims.UserID, poID,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update PO: "+err.Error())
	}

	oldRefs := []poLinePRRef{}
	{
		oldRows, err := tx.Query(ctx, `SELECT pr_line_id, qty_ordered FROM purchase_order_line WHERE po_id=$1`, poID)
		if err != nil {
			return err
		}
		for oldRows.Next() {
			var prLineID *int64
			var qty float64
			if err := oldRows.Scan(&prLineID, &qty); err != nil {
				oldRows.Close()
				return err
			}
			if prLineID != nil {
				oldRefs = append(oldRefs, poLinePRRef{PRLineID: prLineID, Qty: qty})
			}
		}
		oldRows.Close()
	}

	if _, err := tx.Exec(ctx, `DELETE FROM purchase_order_line WHERE po_id=$1`, poID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to clear old lines: "+err.Error())
	}

	for i, line := range req.Lines {
		desc, err := normalizeDescription(line.Description)
		if err != nil {
			return err
		}
		lc := calcs[i]
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_order_line
			  (po_id, line_no, mat_code, pr_line_id, qty_ordered, unit_price, disc_type,
			   discount, line_discount, line_vat, line_wht, line_net, wht_rate,
			   description, remarks, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'OPEN')`,
			poID, i+1, line.MatCode, line.PRLineID, line.QtyOrdered, line.UnitPrice, lc.discType,
			line.Discount, lc.discAmt, lc.vatAmt, lc.whtAmt, lc.net, line.WhtRate,
			desc, line.Remarks,
		); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: invalid mat_code or pr_line_id", i))
			}
			return fiber.NewError(fiber.StatusInternalServerError, "line insert error: "+err.Error())
		}
	}

	{
		newRefs := make([]poLinePRRef, 0, len(req.Lines))
		for _, l := range req.Lines {
			if l.PRLineID != nil {
				newRefs = append(newRefs, poLinePRRef{PRLineID: l.PRLineID, Qty: l.QtyOrdered})
			}
		}
		if err := reconcilePRLineQty(ctx, tx, oldRefs, newRefs); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO po_edit_log (po_id, edited_by, reason, edited_at)
		VALUES ($1,$2,$3,NOW())`, poID, claims.UserID, req.Reason,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "edit log error: "+err.Error())
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'PENDING_REAPPROVAL',$3,$4)`, poID, currentStatus, claims.UserID, req.Reason,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "status log error: "+err.Error())
	}

	// Reopen the PO's existing approval_request row for re-approval. Same approval_id,
	// same assigned_to — deliberately not recomputed, since PO approval is role-gated
	// (RequireRole at the route level), not routed to a specific individual.
	tag, err := tx.Exec(ctx, `
		UPDATE approval_request
		SET status='PENDING'
		WHERE id = (
		    SELECT id FROM approval_request
		    WHERE doc_type='PO' AND doc_id=$1
		    ORDER BY created_at DESC LIMIT 1
		)`, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "approval_request update error: "+err.Error())
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "no existing approval_request found for this PO — cannot reopen approval without a guessed approver")
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "PO updated and sent back for re-approval",
		"data": fiber.Map{
			"po_id": poID, "status": "PENDING_REAPPROVAL",
			"total_amount": totalAmount, "vat_amount": vatAmount, "net_amount": netAmount,
		},
	})
}

// SendPO godoc
// @Summary      Send PO to supplier
// @Description  Marks the PO as sent to the supplier. Only touches status_receive (NOT_SENT -> SENT); the approval status field is left untouched.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {object}  fiber.Map
// @Router       /po/{id}/send [post]
func (h *POHandler) Send(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	id := c.Params("id")

	var currentStatus, currentStatusReceive string
	if err := h.db.QueryRow(context.Background(),
		`SELECT status, status_receive FROM purchase_order WHERE id=$1`, id,
	).Scan(&currentStatus, &currentStatusReceive); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if currentStatus != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO must be approved before sending")
	}
	if currentStatusReceive != "NOT_SENT" {
		return fiber.NewError(fiber.StatusBadRequest, "PO has already been sent (status_receive: "+currentStatusReceive+")")
	}

	if _, err := h.db.Exec(context.Background(),
		`UPDATE purchase_order SET status_receive='SENT', updated_at=NOW() WHERE id=$1`, id,
	); err != nil {
		return err
	}
	h.db.Exec(context.Background(), `
		INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,'NOT_SENT','SENT',$2,'Sent to supplier')`, id, claims.UserID)

	return c.JSON(fiber.Map{"success": true, "message": "PO sent to supplier"})
}

// AddLines godoc
// @Summary      Add lines to an existing purchase order
// @Description  Inserts purchase_order_line rows. For lines that reference a pr_line_id, increments purchase_request_line.qty_ordered and re-evaluates the parent PR status to PARTIALLY_FILLED or FULFILLED. Runs in one transaction with SELECT ... FOR UPDATE on the touched PR lines to prevent concurrent double-booking.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                       true  "PO ID"
// @Param        body  body  models.AddPOLinesRequest  true  "Lines to add"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /po/{id}/lines [post]
func (h *POHandler) AddLines(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	poID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PO id")
	}

	var req models.AddPOLinesRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line required")
	}
	for i, l := range req.Lines {
		if l.MatCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: mat_code is required", i))
		}
		if l.QtyOrdered <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: qty_ordered must be > 0", i))
		}
		if l.UnitPrice < 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: unit_price must be >= 0", i))
		}
	}
	{
		matCodes := make([]string, len(req.Lines))
		for i, l := range req.Lines {
			matCodes[i] = l.MatCode
		}
		if err := validateMatCodesExist(context.Background(), h.db, matCodes); err != nil {
			return err
		}
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var poPRID *int64
	if err := tx.QueryRow(ctx, `SELECT pr_id FROM purchase_order WHERE id=$1`, poID).Scan(&poPRID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}

	// Validate every pr_line_id actually belongs to this PO's PR before locking/reconciling.
	newRefs := make([]poLinePRRef, 0, len(req.Lines))
	for i, l := range req.Lines {
		if l.PRLineID == nil {
			continue
		}
		if poPRID == nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: PO is not linked to a PR, pr_line_id not allowed", i))
		}
		newRefs = append(newRefs, poLinePRRef{PRLineID: l.PRLineID, Qty: l.QtyOrdered})
	}
	if len(newRefs) > 0 {
		prLineIDs := make([]int64, 0, len(newRefs))
		for _, r := range newRefs {
			prLineIDs = append(prLineIDs, *r.PRLineID)
		}
		belongRows, err := tx.Query(ctx, `SELECT id FROM purchase_request_line WHERE id = ANY($1) AND pr_id = $2`, prLineIDs, *poPRID)
		if err != nil {
			return err
		}
		belongs := map[int64]bool{}
		for belongRows.Next() {
			var id int64
			belongRows.Scan(&id)
			belongs[id] = true
		}
		belongRows.Close()
		for _, id := range prLineIDs {
			if !belongs[id] {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("pr_line_id %d does not belong to this PO's PR", id))
			}
		}
	}

	var startLineNo int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(line_no),0) FROM purchase_order_line WHERE po_id=$1`, poID).Scan(&startLineNo); err != nil {
		return err
	}

	insertedIDs := make([]int64, 0, len(req.Lines))
	for i, l := range req.Lines {
		desc, err := normalizeDescription(l.Description)
		if err != nil {
			return err
		}
		discType := l.DiscType
		if discType == "" {
			discType = "pct"
		}
		var lineID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO purchase_order_line (po_id, line_no, mat_code, pr_line_id, qty_ordered, unit_price, disc_type, description, remarks, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'OPEN')
			RETURNING id`,
			poID, startLineNo+i+1, l.MatCode, l.PRLineID, l.QtyOrdered, l.UnitPrice, discType, desc, l.Remarks,
		).Scan(&lineID); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: invalid mat_code or pr_line_id", i))
			}
			return fiber.NewError(fiber.StatusInternalServerError, "line insert error: "+err.Error())
		}
		insertedIDs = append(insertedIDs, lineID)
	}

	if err := reconcilePRLineQty(ctx, tx, nil, newRefs); err != nil {
		return err
	}

	var newPRStatus string
	if poPRID != nil {
		var currentPRStatus string
		if err := tx.QueryRow(ctx, `SELECT status FROM purchase_request WHERE id=$1`, *poPRID).Scan(&currentPRStatus); err != nil {
			return err
		}

		lineRows, err := tx.Query(ctx, `SELECT qty_to_order, qty_ordered FROM purchase_request_line WHERE pr_id=$1`, *poPRID)
		if err != nil {
			return err
		}
		allFulfilled, someOrdered := true, false
		for lineRows.Next() {
			var qtyToOrder, qtyOrd float64
			lineRows.Scan(&qtyToOrder, &qtyOrd)
			if qtyOrd < qtyToOrder {
				allFulfilled = false
			}
			if qtyOrd > 0 {
				someOrdered = true
			}
		}
		lineRows.Close()

		newPRStatus = currentPRStatus
		if allFulfilled {
			newPRStatus = "FULFILLED"
		} else if someOrdered {
			newPRStatus = "PARTIALLY_FILLED"
		}

		if newPRStatus != currentPRStatus {
			if _, err := tx.Exec(ctx, `UPDATE purchase_request SET status=$1, updated_at=NOW() WHERE id=$2`, newPRStatus, *poPRID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO pr_status_log (pr_id, from_status, to_status, changed_by, remarks)
				VALUES ($1,$2,$3,$4,'PO lines saved')`, *poPRID, currentPRStatus, newPRStatus, claims.UserID,
			); err != nil {
				return err
			}
		}
	}

	// Totals are always derived from lines, never trusted from the client.
	if _, err := tx.Exec(ctx, `
		UPDATE purchase_order po SET
		    total_amount = sub.total,
		    vat_amount   = sub.total * 0.07,
		    net_amount   = sub.total * 1.07,
		    updated_at   = NOW()
		FROM (SELECT COALESCE(SUM(amount),0) AS total FROM purchase_order_line WHERE po_id=$1) sub
		WHERE po.id=$1`, poID,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	data := fiber.Map{"po_id": poID, "line_ids": insertedIDs}
	if poPRID != nil {
		data["pr_id"] = *poPRID
		data["pr_status"] = newPRStatus
	}
	return c.JSON(fiber.Map{"success": true, "data": data})
}

// UpdateLine godoc
// @Summary      Update a PO line description
// @Description  Updates the free-text description on an existing purchase_order_line. Empty string clears it (stored as NULL). Max 1000 chars.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path  int                          true  "PO ID"
// @Param        line_id  path  int                          true  "PO line ID"
// @Param        body     body  models.UpdatePOLineRequest   true  "Fields to update"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/{id}/lines/{line_id} [put]
func (h *POHandler) UpdateLine(c *fiber.Ctx) error {
	poID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PO id")
	}
	lineID, err := strconv.ParseInt(c.Params("line_id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid line id")
	}

	var req models.UpdatePOLineRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.DiscType != nil && *req.DiscType != "pct" && *req.DiscType != "amt" {
		return fiber.NewError(fiber.StatusBadRequest, "disc_type must be 'pct' or 'amt'")
	}
	desc, err := normalizeDescription(req.Description)
	if err != nil {
		return err
	}

	var tag pgconn.CommandTag
	if req.DiscType != nil {
		tag, err = h.db.Exec(context.Background(), `
			UPDATE purchase_order_line SET description=$1, disc_type=$2 WHERE id=$3 AND po_id=$4`,
			desc, *req.DiscType, lineID, poID)
	} else {
		tag, err = h.db.Exec(context.Background(), `
			UPDATE purchase_order_line SET description=$1 WHERE id=$2 AND po_id=$3`,
			desc, lineID, poID)
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "update error: "+err.Error())
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "PO line not found")
	}
	respData := fiber.Map{"line_id": lineID, "description": desc}
	if req.DiscType != nil {
		respData["disc_type"] = *req.DiscType
	}
	return c.JSON(fiber.Map{"success": true, "data": respData})
}

// GetPOLogs godoc
// @Summary      Get PO status history
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {array}  fiber.Map
// @Router       /po/{id}/logs [get]
func (h *POHandler) GetLogs(c *fiber.Ctx) error {
	id := c.Params("id")
	rows, err := h.db.Query(context.Background(), `
		SELECT l.id, l.from_status, l.to_status, u.full_name, l.changed_at, l.remarks
		FROM po_status_log l JOIN users u ON u.id = l.changed_by
		WHERE l.po_id=$1 ORDER BY l.changed_at`, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	type LogRow struct {
		LogID     int64     `json:"log_id"`
		From      *string   `json:"from_status"`
		To        string    `json:"to_status"`
		ChangedBy string    `json:"changed_by"`
		ChangedAt time.Time `json:"changed_at"`
		Remarks   *string   `json:"remarks"`
	}
	var logs []LogRow
	for rows.Next() {
		var l LogRow
		rows.Scan(&l.LogID, &l.From, &l.To, &l.ChangedBy, &l.ChangedAt, &l.Remarks)
		logs = append(logs, l)
	}
	return c.JSON(fiber.Map{"success": true, "data": logs})
}

// poPrintSupplier is the nested supplier block of the print-data response.
type poPrintSupplier struct {
	Name          string  `json:"name"`
	Address1      *string `json:"address1"`
	TermOfPayment *string `json:"termOfPayment"`
	Contact       *string `json:"contact"`
}

// poPrintItem is one line of the print-data response.
type poPrintItem struct {
	No           string  `json:"no"`
	Code         string  `json:"code"`
	Desc         string  `json:"desc"`
	Qty          float64 `json:"qty"`
	Unit         string  `json:"unit"`
	PricePerUnit float64 `json:"pricePerUnit"`
	DiscPct      float64 `json:"discPct"`
	VatPct       float64 `json:"vatPct"`
	WhtPct       float64 `json:"whtPct"`
}

// poPrintData is the full response shape consumed by the PurchaseOrderPrint component.
type poPrintData struct {
	PONo string `json:"poNo"`
	// RevisionRound is how many times this PO has been edited-and-resent for re-approval
	// (COUNT of po_edit_log rows). 0 = original. poNo itself never changes; the print
	// component composes a suffix like "#R2" from this when > 0 — required correct here
	// since this is the literal printed/legal PO document.
	RevisionRound    int             `json:"revisionRound"`
	PODate           string          `json:"poDate"`
	PRNo             string          `json:"prNo"`
	DeliveryDate     string          `json:"deliveryDate"`
	Project          string          `json:"project"`
	DeliveryPlace    string          `json:"deliveryPlace"`
	Job              string          `json:"job"`
	ContractDelivery string          `json:"contractDelivery"`
	QuotationNo      string          `json:"quotationNo"`
	Tel              *string         `json:"tel"`
	Supplier         poPrintSupplier `json:"supplier"`
	Items            []poPrintItem   `json:"items"`
	ExtraDiscAmt     float64         `json:"extraDiscAmt"`
	ShippingAmt      float64         `json:"shippingAmt"`
	VatAmt           float64         `json:"vatAmt"`
	WhtAmt           float64         `json:"whtAmt"`
	TotalAmt         float64         `json:"totalAmt"`
	NetAmt           float64         `json:"netAmt"`
	Remark           *string         `json:"remark"`
	UseDiscount      bool            `json:"useDiscount"`
	UseVat           bool            `json:"useVat"`
	UseWht           bool            `json:"useWht"`
}

// PrintData godoc
// @Summary      Get print-ready data for a purchase order
// @Description  Aggregates purchase_order, purchase_order_line, supplier, project, location and purchase_request into the shape consumed by the PurchaseOrderPrint frontend component. "job", "contractDelivery" and "quotationNo" are literal "***" placeholders — those fields don't exist in the schema yet.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/{id}/print-data [get]
func (h *POHandler) PrintData(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := context.Background()

	var (
		poNo                        string
		revisionRound               int
		poDate                      time.Time
		expectedDate                *time.Time
		projectCode                 *string
		locationText                *string
		poPaymentTerms              *string
		remarks                     *string
		discountAmount              *float64
		vatAmount, whtAmount        float64
		totalAmount, netAmount      float64
		useDiscount, useVat, useWht *bool
		prNo                        *string
		supplierName                *string
		supplierAddress             *string
		supplierPaymentTerms        *string
		supplierContactName         *string
		supplierContactPhone        *string
	)

	err := h.db.QueryRow(ctx, `
		SELECT po.po_no, po.po_date, po.expected_date, po.project_code, po.location_text,
		       po.payment_terms, po.remarks, po.discount_amount, po.use_discount, po.use_vat, po.use_wht,
		       po.vat_amount, po.wht_amount, po.total_amount, po.net_amount,
		       pr.pr_no,
		       s.supplier_name, s.address, s.payment_terms, s.contact_name, s.contact_phone,
		       (SELECT COUNT(*) FROM po_edit_log pel WHERE pel.po_id = po.id)
		FROM purchase_order po
		LEFT JOIN supplier s ON s.id = po.supplier_id
		LEFT JOIN purchase_request pr ON pr.id = po.pr_id
		WHERE po.id = $1`, id,
	).Scan(&poNo, &poDate, &expectedDate, &projectCode, &locationText,
		&poPaymentTerms, &remarks, &discountAmount, &useDiscount, &useVat, &useWht,
		&vatAmount, &whtAmount, &totalAmount, &netAmount,
		&prNo,
		&supplierName, &supplierAddress, &supplierPaymentTerms, &supplierContactName, &supplierContactPhone,
		&revisionRound,
	)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}

	rows, err := h.db.Query(ctx, `
		SELECT pol.line_no, pol.mat_code, pol.description, pol.qty_ordered, pol.unit_price,
		       pol.discount, pol.wht_rate, mn.mat_name, u.unit_name
		FROM purchase_order_line pol
		LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
		LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
		LEFT JOIN unit u ON u.id = mc.unit_id
		WHERE pol.po_id = $1
		ORDER BY pol.line_no`, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []poPrintItem{}
	for rows.Next() {
		var (
			lineNo      int
			matCode     string
			description *string
			qty         float64
			unitPrice   float64
			discount    *float64
			whtRate     *float64
			matName     *string
			unitName    *string
		)
		if err := rows.Scan(&lineNo, &matCode, &description, &qty, &unitPrice, &discount, &whtRate, &matName, &unitName); err != nil {
			return err
		}

		desc := ""
		if description != nil && strings.TrimSpace(*description) != "" {
			desc = *description
		} else if matName != nil {
			desc = *matName
		}
		unit := ""
		if unitName != nil {
			unit = *unitName
		}

		items = append(items, poPrintItem{
			No:           strconv.Itoa(lineNo),
			Code:         matCode,
			Desc:         desc,
			Qty:          qty,
			Unit:         unit,
			PricePerUnit: unitPrice,
			DiscPct:      derefFloat(discount),
			VatPct:       0, // no per-line VAT rate column exists; PO-level VAT is a flat 7% (see handler notes)
			WhtPct:       derefFloat(whtRate),
		})
	}

	data := poPrintData{
		PONo:             poNo,
		RevisionRound:    revisionRound,
		PODate:           poDate.Format("02/01/2006"),
		PRNo:             derefString(prNo),
		DeliveryDate:     formatPrintDate(expectedDate),
		Project:          derefString(projectCode),
		DeliveryPlace:    derefString(locationText),
		Job:              "***",
		ContractDelivery: "***",
		QuotationNo:      "***",
		Tel:              supplierContactPhone,
		Supplier: poPrintSupplier{
			Name:          derefString(supplierName),
			Address1:      supplierAddress,
			TermOfPayment: firstNonNil(poPaymentTerms, supplierPaymentTerms),
			Contact:       supplierContactName,
		},
		Items:        items,
		ExtraDiscAmt: derefFloat(discountAmount),
		ShippingAmt:  0,
		VatAmt:       vatAmount,
		WhtAmt:       whtAmount,
		TotalAmt:     totalAmount,
		NetAmt:       netAmount,
		Remark:       remarks,
		UseDiscount:  derefBool(useDiscount),
		UseVat:       derefBool(useVat),
		UseWht:       derefBool(useWht),
	}

	return c.JSON(fiber.Map{"success": true, "data": data})
}

// GetReceivablePOs godoc
// @Summary      List POs that are eligible for goods receiving (GRN)
// @Description  Returns approved POs that have not been fully received yet: status=APPROVED, status_receive IN (NOT_SENT, SENT, PARTIALLY_RECEIVED), and at least one line still OPEN or PARTIAL.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        search    query  string  false  "search by po_no"
// @Param        page      query  int  false  "page"  default(1)
// @Param        page_size query  int  false  "page_size"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /po/receivable [get]
func (h *POHandler) GetReceivablePOs(c *fiber.Ctx) error {
	search := c.Query("search")
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	ctx := context.Background()
	filter := `
		FROM purchase_order po
		WHERE po.status = 'APPROVED'
		  AND po.status_receive IN ('NOT_SENT', 'SENT', 'PARTIALLY_RECEIVED')
		  AND EXISTS (
		      SELECT 1 FROM purchase_order_line pol
		      WHERE pol.po_id = po.id AND pol.status IN ('OPEN', 'PARTIAL')
		  )`
	args := []interface{}{}
	idx := 1
	if search != "" {
		filter += fmt.Sprintf(" AND po.po_no ILIKE $%d", idx)
		args = append(args, "%"+search+"%")
		idx++
	}

	var total int64
	if err := h.db.QueryRow(ctx, `SELECT COUNT(*) `+filter, args...).Scan(&total); err != nil {
		return err
	}

	args = append(args, size, offset)
	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT po.id, po.po_no, po.po_date, po.supplier_id, po.status, po.status_receive,
		       (SELECT COUNT(*) FROM po_edit_log pel WHERE pel.po_id = po.id) AS revision_round
		%s
		ORDER BY po.po_date DESC, po.po_no
		LIMIT $%d OFFSET $%d`, filter, idx, idx+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type receivablePO struct {
		POID          int64     `json:"po_id"`
		PONo          string    `json:"po_no"`
		PODate        time.Time `json:"po_date"`
		SupplierID    *int64    `json:"supplier_id,omitempty"`
		Status        string    `json:"status"`
		StatusReceive string    `json:"status_receive"`
		// RevisionRound: see PurchaseOrder.RevisionRound. po_no never changes.
		RevisionRound int `json:"revision_round"`
	}

	items := []receivablePO{}
	for rows.Next() {
		var r receivablePO
		if err := rows.Scan(&r.POID, &r.PONo, &r.PODate, &r.SupplierID, &r.Status, &r.StatusReceive, &r.RevisionRound); err != nil {
			return err
		}
		items = append(items, r)
	}

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data":    models.PaginatedResponse{Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages},
	})
}

// GetReceivableLines godoc
// @Summary      List the not-yet-fully-received lines of a PO
// @Description  Returns purchase_order_line rows with status IN (OPEN, PARTIAL) for the given PO, including qty_remaining = qty_ordered - qty_received, so the GRN form can prevent over-receiving.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/{id}/receivable-lines [get]
func (h *POHandler) GetReceivableLines(c *fiber.Ctx) error {
	poID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PO id")
	}
	ctx := context.Background()

	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM purchase_order WHERE id=$1)`, poID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}

	rows, err := h.db.Query(ctx, `
		SELECT pol.id, pol.po_id, pol.line_no, pol.mat_code, pol.qty_ordered, pol.qty_received,
		       (pol.qty_ordered - pol.qty_received) AS qty_remaining, pol.unit_price, pol.status,
		       mn.mat_name
		FROM purchase_order_line pol
		LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
		LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
		WHERE pol.po_id = $1 AND pol.status IN ('OPEN', 'PARTIAL')
		ORDER BY pol.line_no`, poID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type receivableLine struct {
		LineID       int64   `json:"line_id"`
		POID         int64   `json:"po_id"`
		LineNo       int     `json:"line_no"`
		MatCode      string  `json:"mat_code"`
		MatName      *string `json:"mat_name,omitempty"`
		QtyOrdered   float64 `json:"qty_ordered"`
		QtyReceived  float64 `json:"qty_received"`
		QtyRemaining float64 `json:"qty_remaining"`
		UnitPrice    float64 `json:"unit_price"`
		Status       string  `json:"status"`
	}

	items := []receivableLine{}
	for rows.Next() {
		var l receivableLine
		if err := rows.Scan(&l.LineID, &l.POID, &l.LineNo, &l.MatCode, &l.QtyOrdered, &l.QtyReceived,
			&l.QtyRemaining, &l.UnitPrice, &l.Status, &l.MatName); err != nil {
			return err
		}
		items = append(items, l)
	}

	return c.JSON(fiber.Map{"success": true, "data": items})
}

func formatPrintDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("02/01/2006")
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func firstNonNil(vals ...*string) *string {
	for _, v := range vals {
		if v != nil && strings.TrimSpace(*v) != "" {
			return v
		}
	}
	return nil
}

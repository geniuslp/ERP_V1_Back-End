package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
)

// GoodsReceiptHandler implements the "รับเข้า" (Goods Receipt) flow on top of
// purchase_order / purchase_order_line / grn / grn_line / stock_transaction —
// separate from the legacy DRAFT->confirm GRNHandler which posts to the old
// inventory/inventory_transaction tables.
type GoodsReceiptHandler struct{ db *pgxpool.Pool }

func NewGoodsReceiptHandler(db *pgxpool.Pool) *GoodsReceiptHandler {
	return &GoodsReceiptHandler{db: db}
}

// ─── 1. Search approved PO ─────────────────────────────────────────────────

// SearchApprovedPO godoc
// @Summary      Search receivable Purchase Orders by partial po_no match
// @Description  Matches POs with status=APPROVED and status_receive IN (NOT_SENT, SENT, PARTIALLY_RECEIVED).
// @Tags         GoodsReceipt
// @Security     BearerAuth
// @Produce      json
// @Param        po_no  query  string  true  "PO number keyword (partial match)"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/search [get]
func (h *GoodsReceiptHandler) SearchApprovedPO(c *fiber.Ctx) error {
	poNo := c.Query("po_no")
	if poNo == "" {
		return fiber.NewError(fiber.StatusBadRequest, "po_no is required")
	}
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT po.id, po.po_no, po.po_date::text, po.expected_date::text, po.supplier_id,
		       COALESCE(po.warehouse_code, ''), po.status, po.status_receive, po.currency, COALESCE(po.net_amount, 0),
		       s.supplier_name, po.project_code, pj.project_name
		FROM purchase_order po
		LEFT JOIN supplier s ON s.id = po.supplier_id
		LEFT JOIN project pj ON pj.project_code = po.project_code
		WHERE po.po_no ILIKE '%' || $1 || '%'
		  AND po.status = 'APPROVED'
		  AND po.status_receive IN ('NOT_SENT', 'SENT', 'PARTIALLY_RECEIVED')
		ORDER BY po.po_date DESC, po.po_no ASC`, poNo)
	if err != nil {
		return err
	}
	defer rows.Close()

	type poResp struct {
		POID          int64   `json:"po_id"`
		PONo          string  `json:"po_no"`
		PODate        string  `json:"po_date"`
		ExpectedDate  *string `json:"expected_date"`
		SupplierID    *int64  `json:"supplier_id,omitempty"`
		WarehouseCode string  `json:"warehouse_code"`
		Status        string  `json:"status"`
		StatusReceive string  `json:"status_receive"`
		Currency      string  `json:"currency"`
		NetAmount     float64 `json:"net_amount"`
		SupplierName  *string `json:"supplier_name,omitempty"`
		ProjectCode   *string `json:"project_code,omitempty"`
		ProjectName   *string `json:"project_name,omitempty"`
	}
	var results []poResp
	for rows.Next() {
		var p poResp
		if err := rows.Scan(&p.POID, &p.PONo, &p.PODate, &p.ExpectedDate, &p.SupplierID,
			&p.WarehouseCode, &p.Status, &p.StatusReceive, &p.Currency, &p.NetAmount,
			&p.SupplierName, &p.ProjectCode, &p.ProjectName); err != nil {
			return err
		}
		results = append(results, p)
	}

	if len(results) == 0 {
		return fiber.NewError(fiber.StatusNotFound, "No approved PO found matching the given keyword")
	}

	return c.JSON(fiber.Map{"data": results})
}

// ─── 2. Create Goods Receipt (single full receive) ─────────────────────────

type receiveGRNLine struct {
	POLineID int64   `json:"po_line_id" validate:"required"`
	MatCode  string  `json:"mat_code" validate:"required"`
	AddQty   float64 `json:"add_qty" validate:"required,gt=0"`
}

type receiveGRNRequest struct {
	GRNNo        string           `json:"grn_no" validate:"required"`
	POID         int64            `json:"po_id" validate:"required"`
	InvoiceNo    string           `json:"invoice_no" validate:"required"`
	DeliveryNote *string          `json:"delivery_note,omitempty"`
	DeliveryDate *string          `json:"delivery_date,omitempty" validate:"omitempty,datetime=2006-01-02"`
	Lines        []receiveGRNLine `json:"lines" validate:"required,min=1,dive"`
}

// ReserveGRNNumber godoc
// @Summary      Reserve the next GRN number (consumes grn_seq)
// @Description  Calls nextval('grn_seq') and formats it immediately as the real grn_no (GRN-<YYYYMM>-NNNN). This is the single, consolidated format for grn_no — both POST /grn/receive and POST /grn now require a number reserved from this endpoint. Unlike the old per-handler generators, this actually consumes the sequence right away — the number is reserved even if the create is never submitted (a gap is expected and fine). The frontend calls this once when the create/receive page opens, then submits the returned grn_no with the create/receive request.
// @Tags         GoodsReceipt
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /grn/reserve-number [get]
func (h *GoodsReceiptHandler) ReserveGRNNumber(c *fiber.Ctx) error {
	var seq int64
	if err := h.db.QueryRow(context.Background(), `SELECT nextval('grn_seq')`).Scan(&seq); err != nil {
		return err
	}
	grnNo := fmt.Sprintf("GRN-%s-%04d", time.Now().Format("200601"), seq)
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"grn_no": grnNo}})
}

// Receive godoc
// @Summary      Create Goods Receipt (single full receive) against an APPROVED PO
// @Description  Writes grn/grn_line (including optional delivery_date, distinct from purchase_order.expected_date), upserts stock_inventory.qty_on_hand per item+location, rolls up stock_item.qty as the sum, logs stock_transaction (IN), updates purchase_order_line/purchase_order status
// @Tags         GoodsReceipt
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "Goods receipt request"
// @Success      201  {object}  fiber.Map
// @Router       /grn/receive [post]
func (h *GoodsReceiptHandler) Receive(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "no claims found")
	}

	var req receiveGRNRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.POID == 0 || strings.TrimSpace(req.InvoiceNo) == "" || len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "po_id, invoice_no, lines are required")
	}
	if req.DeliveryDate != nil {
		if _, err := time.Parse("2006-01-02", *req.DeliveryDate); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "delivery_date must be a valid date (YYYY-MM-DD)")
		}
	}
	for _, l := range req.Lines {
		if l.POLineID == 0 || l.MatCode == "" || l.AddQty <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, "each line requires po_line_id, mat_code and add_qty > 0")
		}
	}

	ctx := context.Background()

	var poStatus, poStatusReceive string
	var poLocationCode, poWarehouseCode *string
	var supplierID *int64
	if err := h.db.QueryRow(ctx, `SELECT status, status_receive, location_code, warehouse_code, supplier_id FROM purchase_order WHERE id=$1`, req.POID).Scan(&poStatus, &poStatusReceive, &poLocationCode, &poWarehouseCode, &supplierID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if poStatus != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not approved, current status: "+poStatus)
	}
	if poStatusReceive != "NOT_SENT" && poStatusReceive != "SENT" && poStatusReceive != "PARTIALLY_RECEIVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not in a receivable status_receive (NOT_SENT/SENT/PARTIALLY_RECEIVED), current: "+poStatusReceive)
	}
	if supplierID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "PO has no supplier_id set — cannot create a goods receipt against it")
	}
	if poWarehouseCode == nil || *poWarehouseCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "PO has no warehouse_code set — cannot create a goods receipt against it")
	}
	warehouseCode := *poWarehouseCode

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// grn_no is generated internally from grn_seq inside this transaction — the
	// create/receive frontend page does not call reserve-number, unlike PR/PO.
	var seq int64
	if err := tx.QueryRow(ctx, `SELECT nextval('grn_seq')`).Scan(&seq); err != nil {
		return err
	}
	grnNo := fmt.Sprintf("GRN-%s-%04d", time.Now().Format("200601"), seq)

	var grnID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO grn (grn_no, grn_date, po_id, warehouse_code, supplier_id, delivery_note, delivery_date, invoice_no, status, quality_status, received_by)
		VALUES ($1, CURRENT_DATE, $2, $3, $4, $5, $6, $7, 'CONFIRMED', 'PENDING', $8)
		RETURNING id`,
		grnNo, req.POID, warehouseCode, *supplierID, req.DeliveryNote, req.DeliveryDate, req.InvoiceNo, claims.UserID,
	).Scan(&grnID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create GRN: "+err.Error())
	}

	var lineErrors []string
	for i, line := range req.Lines {
		var qtyOrdered, qtyReceivedBefore, poUnitPrice float64
		var poLinePOID int64
		if err := tx.QueryRow(ctx, `
			SELECT po_id, qty_ordered, qty_received, unit_price FROM purchase_order_line WHERE id=$1 AND mat_code=$2`,
			line.POLineID, line.MatCode,
		).Scan(&poLinePOID, &qtyOrdered, &qtyReceivedBefore, &poUnitPrice); err != nil {
			lineErrors = append(lineErrors, fmt.Sprintf("po_line_id %d: PO line not found for mat_code %s", line.POLineID, line.MatCode))
			continue
		}
		if poLinePOID != req.POID {
			lineErrors = append(lineErrors, fmt.Sprintf("po_line_id %d: does not belong to po_id %d", line.POLineID, req.POID))
			continue
		}

		var itemID int64
		var itemLocationCode string
		if err := tx.QueryRow(ctx, `
			SELECT id, location_code FROM stock_item WHERE mat_code = $1`, line.MatCode,
		).Scan(&itemID, &itemLocationCode); err != nil {
			lineErrors = append(lineErrors, fmt.Sprintf("mat_code %s: not found in stock_item — cannot receive into a non-existent stock item", line.MatCode))
			continue
		}

		// PO's location_code wins when set; otherwise fall back to the item's
		// own default location (stock_inventory.location_code has a FK to
		// location, so this must always resolve to a real location row).
		locationCode := itemLocationCode
		if poLocationCode != nil && *poLocationCode != "" {
			locationCode = *poLocationCode
		}

		// qty_before must come from stock_item.qty (the authoritative "current stock" the
		// receive page's "จำนวนสุทธิ" reads), locked FOR UPDATE — not from
		// SUM(stock_inventory.qty_on_hand). stock_inventory can have zero rows for an item
		// whose stock was last written by a different flow (Requisition/Transfer write
		// stock_item.qty directly without touching stock_inventory — see CLAUDE.md "Session
		// learnings 2026-08-16 #3"), which previously made qty_before silently read as 0.
		var qtyBefore float64
		if err := tx.QueryRow(ctx, `
			SELECT qty FROM stock_item WHERE id = $1 FOR UPDATE`,
			itemID,
		).Scan(&qtyBefore); err != nil {
			return err
		}
		qtyAfter := qtyBefore + line.AddQty

		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_inventory (item_id, location_code, warehouse_code, qty_on_hand, updated_at)
			VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (item_id, location_code) DO UPDATE
			SET qty_on_hand = stock_inventory.qty_on_hand + EXCLUDED.qty_on_hand,
			    warehouse_code = EXCLUDED.warehouse_code,
			    updated_at = NOW()`,
			itemID, locationCode, warehouseCode, line.AddQty,
		); err != nil {
			return fmt.Errorf("upsert stock_inventory for mat_code %s: %w", line.MatCode, err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE stock_item SET qty = $1, updated_at = NOW() WHERE id = $2`,
			qtyAfter, itemID,
		); err != nil {
			return fmt.Errorf("update qty for mat_code %s: %w", line.MatCode, err)
		}

		// Last Cost strategy: unit_cost is always overwritten to this receipt's PO line
		// unit_price, never averaged with the item's prior unit_cost. unit_price is the
		// plain per-unit price ("ราคา/หน่วย" on the PO Create screen), read before any
		// discount/VAT/WHT is applied — those are computed separately per PO line and are
		// not folded into this cost.
		if _, err := tx.Exec(ctx, `
			UPDATE stock_item SET unit_cost = $1, updated_at = NOW() WHERE id = $2`,
			poUnitPrice, itemID,
		); err != nil {
			return fmt.Errorf("update unit_cost for mat_code %s: %w", line.MatCode, err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO grn_line (grn_id, line_no, po_line_id, mat_code, qty_received, qty_accepted, qty_rejected)
			VALUES ($1,$2,$3,$4,$5,$5,0)`,
			grnID, i+1, line.POLineID, line.MatCode, line.AddQty,
		); err != nil {
			return err
		}

		txnNo, err := generateTxnNo(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_transaction (txn_no, txn_type, item_id, qty, qty_before, qty_after, ref_doc_type, ref_doc_id, txn_date, created_by)
			VALUES ($1,'IN',$2,$3,$4,$5,'GRN',$6,CURRENT_DATE,$7)`,
			txnNo, itemID, line.AddQty, qtyBefore, qtyAfter, grnID, claims.UserID,
		); err != nil {
			return err
		}

		newQtyReceived := qtyReceivedBefore + line.AddQty
		lineStatus := "PARTIAL"
		if newQtyReceived >= qtyOrdered {
			lineStatus = "RECEIVED"
		}
		if _, err := tx.Exec(ctx, `
			UPDATE purchase_order_line SET qty_received = $1, status = $2 WHERE id = $3`,
			newQtyReceived, lineStatus, line.POLineID,
		); err != nil {
			return err
		}
	}

	if len(lineErrors) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, "goods receipt failed: "+fmt.Sprintf("%v", lineErrors))
	}

	var openCount int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM purchase_order_line WHERE po_id=$1 AND status <> 'RECEIVED' AND status <> 'CANCELLED'`,
		req.POID).Scan(&openCount); err != nil {
		return err
	}
	newStatusReceive := "PARTIALLY_RECEIVED"
	if openCount == 0 {
		newStatusReceive = "RECEIVED"
	}
	if _, err := tx.Exec(ctx, `UPDATE purchase_order SET status_receive=$1, updated_at=NOW() WHERE id=$2`, newStatusReceive, req.POID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,$3,$4,'Goods received')`, req.POID, poStatusReceive, newStatusReceive, claims.UserID,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"grn_id":         grnID,
			"grn_no":         grnNo,
			"status_receive": newStatusReceive,
		},
	})
}

// ─── 3. Score the receipt ───────────────────────────────────────────────────

type scoreGRNRequest struct {
	ScoreQuality  int     `json:"score_quality" validate:"required,min=1,max=5"`
	ScoreQuantity int     `json:"score_quantity" validate:"required,min=1,max=5"`
	ScoreOntime   int     `json:"score_ontime" validate:"required,min=1,max=5"`
	ScoreNotes    *string `json:"score_notes,omitempty"`
}

// Score godoc
// @Summary      Score a confirmed goods receipt and mark it POSTED
// @Tags         GoodsReceipt
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int              true  "GRN ID"
// @Param        body  body  object  true  "Score request"
// @Success      200  {object}  fiber.Map
// @Router       /grn/{id}/score [post]
func (h *GoodsReceiptHandler) Score(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "no claims found")
	}
	id := c.Params("id")

	var req scoreGRNRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.ScoreQuality < 1 || req.ScoreQuality > 5 ||
		req.ScoreQuantity < 1 || req.ScoreQuantity > 5 ||
		req.ScoreOntime < 1 || req.ScoreOntime > 5 {
		return fiber.NewError(fiber.StatusBadRequest, "score_quality, score_quantity, score_ontime must each be 1-5")
	}

	ctx := context.Background()
	var status string
	if err := h.db.QueryRow(ctx, `SELECT status FROM grn WHERE id=$1`, id).Scan(&status); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "GRN not found")
	}
	if status != "CONFIRMED" {
		return fiber.NewError(fiber.StatusBadRequest, "GRN must be CONFIRMED before scoring, current status: "+status)
	}

	tag, err := h.db.Exec(ctx, `
		UPDATE grn
		SET score_quality=$1, score_quantity=$2, score_ontime=$3, score_notes=$4,
		    confirmed_by=$5, confirmed_at=NOW(), status='POSTED', updated_at=NOW()
		WHERE id=$6`,
		req.ScoreQuality, req.ScoreQuantity, req.ScoreOntime, req.ScoreNotes, claims.UserID, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "GRN not found")
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"grn_id": id, "status": "POSTED"}})
}

// ─── 4. GRN history ─────────────────────────────────────────────────────────

// History godoc
// @Summary      Paginated GRN history with optional supplier/date filters
// @Tags         GoodsReceipt
// @Security     BearerAuth
// @Produce      json
// @Param        page           query  int     false  "Page number (default 1)"
// @Param        limit          query  int     false  "Page size (default 20)"
// @Param        supplier_id    query  int     false  "Filter by supplier id"
// @Param        date_from      query  string  false  "Filter grn_date >= (YYYY-MM-DD)"
// @Param        date_to        query  string  false  "Filter grn_date <= (YYYY-MM-DD)"
// @Success      200  {object}  fiber.Map
// @Router       /grn/history [get]
func (h *GoodsReceiptHandler) History(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit < 1 {
		limit = 20
	}
	supplierID := c.Query("supplier_id")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	ctx := context.Background()

	where := "WHERE 1=1"
	args := []any{}
	if supplierID != "" {
		args = append(args, supplierID)
		where += fmt.Sprintf(" AND g.supplier_id = $%d", len(args))
	}
	if dateFrom != "" {
		args = append(args, dateFrom)
		where += fmt.Sprintf(" AND g.grn_date >= $%d", len(args))
	}
	if dateTo != "" {
		args = append(args, dateTo)
		where += fmt.Sprintf(" AND g.grn_date <= $%d", len(args))
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM grn g " + where
	if err := h.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return err
	}

	args = append(args, limit, (page-1)*limit)
	listQuery := fmt.Sprintf(`
		SELECT
		    g.id, g.grn_no, g.grn_date::text, po.po_no, g.supplier_id, g.warehouse_code, g.status,
		    g.score_quality, g.score_quantity, g.score_ontime, g.score_notes,
		    COALESCE(u.full_name, '') AS received_by_name,
		    COALESCE(gl.line_count, 0) AS line_count,
		    COALESCE(gl.total_qty_received, 0) AS total_qty_received,
		    g.delivery_date::text
		FROM grn g
		LEFT JOIN purchase_order po ON po.id = g.po_id
		LEFT JOIN users u ON u.id = g.received_by
		LEFT JOIN (
		    SELECT grn_id, COUNT(*) AS line_count, SUM(qty_received) AS total_qty_received
		    FROM grn_line GROUP BY grn_id
		) gl ON gl.grn_id = g.id
		%s
		ORDER BY g.grn_date DESC, g.id DESC
		LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))

	rows, err := h.db.Query(ctx, listQuery, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type historyResp struct {
		GRNID            int64   `json:"grn_id"`
		GRNNo            string  `json:"grn_no"`
		GRNDate          string  `json:"grn_date"`
		PONo             *string `json:"po_no"`
		SupplierID       *int64  `json:"supplier_id,omitempty"`
		WarehouseCode    string  `json:"warehouse_code"`
		Status           string  `json:"status"`
		ScoreQuality     *int    `json:"score_quality"`
		ScoreQuantity    *int    `json:"score_quantity"`
		ScoreOntime      *int    `json:"score_ontime"`
		ScoreNotes       *string `json:"score_notes"`
		ReceivedByName   string  `json:"received_by_name"`
		LineCount        int     `json:"line_count"`
		TotalQtyReceived float64 `json:"total_qty_received"`
		DeliveryDate     *string `json:"delivery_date"`
	}
	var results []historyResp
	for rows.Next() {
		var r historyResp
		if err := rows.Scan(&r.GRNID, &r.GRNNo, &r.GRNDate, &r.PONo, &r.SupplierID, &r.WarehouseCode, &r.Status,
			&r.ScoreQuality, &r.ScoreQuantity, &r.ScoreOntime, &r.ScoreNotes,
			&r.ReceivedByName, &r.LineCount, &r.TotalQtyReceived, &r.DeliveryDate); err != nil {
			return err
		}
		results = append(results, r)
	}
	if results == nil {
		results = []historyResp{}
	}

	return c.JSON(fiber.Map{
		"data": results,
		"meta": fiber.Map{"page": page, "limit": limit, "total": total},
	})
}

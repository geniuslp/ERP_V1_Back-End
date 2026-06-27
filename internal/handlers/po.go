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

// ListPO godoc
// @Summary      List purchase orders
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        status    query  string  false  "status filter"
// @Param        supplier  query  string  false  "supplier_code filter"
// @Param        page      query  int     false  "page"  default(1)
// @Param        page_size query  int     false  "page_size"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /po [get]
func (h *POHandler) List(c *fiber.Ctx) error {
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	var total int64
	h.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM v_po_full`).Scan(&total)

	rows, err := h.db.Query(context.Background(), `
		SELECT po_id, po_no, po_date, expected_date, po_status, supplier_name, supplier_contact,
		       currency, total_amount, vat_amount, net_amount, pr_no, warehouse_name, created_by_name, created_at
		FROM v_po_full ORDER BY created_at DESC LIMIT $1 OFFSET $2`, size, offset)
	if err != nil {
		return err
	}
	defer rows.Close()

	type PORow struct {
		POID            int64     `json:"po_id"`
		PONo            string    `json:"po_no"`
		PODate          time.Time `json:"po_date"`
		ExpectedDate    *string   `json:"expected_date,omitempty"`
		Status          string    `json:"status"`
		SupplierName    string    `json:"supplier_name"`
		SupplierContact *string   `json:"supplier_contact,omitempty"`
		Currency        string    `json:"currency"`
		TotalAmount     float64   `json:"total_amount"`
		VATAmount       float64   `json:"vat_amount"`
		NetAmount       float64   `json:"net_amount"`
		PRNo            *string   `json:"pr_no,omitempty"`
		WarehouseName   string    `json:"warehouse_name"`
		CreatedBy       string    `json:"created_by"`
		CreatedAt       time.Time `json:"created_at"`
	}

	var items []PORow
	for rows.Next() {
		var r PORow
		rows.Scan(&r.POID, &r.PONo, &r.PODate, &r.ExpectedDate, &r.Status,
			&r.SupplierName, &r.SupplierContact, &r.Currency, &r.TotalAmount,
			&r.VATAmount, &r.NetAmount, &r.PRNo, &r.WarehouseName, &r.CreatedBy, &r.CreatedAt)
		items = append(items, r)
	}

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data":    models.PaginatedResponse{Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages},
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
		SELECT po_id, po_no, po_date, supplier_code, pr_id, rfq_id, warehouse_code,
		       currency, total_amount, vat_amount, net_amount, expected_date::text,
		       status, payment_terms, remarks, created_by, created_at, updated_at
		FROM purchase_order WHERE po_id=$1`, id)

	var po models.PurchaseOrder
	if err := row.Scan(&po.POID, &po.PONo, &po.PODate, &po.SupplierCode, &po.PRID,
		&po.RFQID, &po.WarehouseCode, &po.Currency, &po.TotalAmount, &po.VATAmount,
		&po.NetAmount, &po.ExpectedDate, &po.Status, &po.PaymentTerms, &po.Remarks,
		&po.CreatedBy, &po.CreatedAt, &po.UpdatedAt); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}

	rows, _ := h.db.Query(context.Background(), `
		SELECT line_id, po_id, line_no, mat_code, pr_line_id, qty_ordered, qty_received, unit_price, amount, description, remarks, status
		FROM purchase_order_line WHERE po_id=$1 ORDER BY line_no`, id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var l models.POLine
			rows.Scan(&l.LineID, &l.POID, &l.LineNo, &l.MatCode, &l.PRLineID,
				&l.QtyOrdered, &l.QtyReceived, &l.UnitPrice, &l.Amount, &l.Description, &l.Remarks, &l.Status)
			po.Lines = append(po.Lines, l)
		}
	}

	return c.JSON(fiber.Map{"success": true, "data": po})
}

// CreatePO godoc
// @Summary      Create purchase order
// @Description  Creates a PO as DRAFT or PENDING_APPROVAL. If pr_id is set, the referenced PR must be APPROVED and any pr_line_id must belong to it. Line totals and the PO total are always computed server-side. Submitting with status=PENDING_APPROVAL opens a step-1 approval_request.
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

	if req.SupplierCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "supplier_code is required")
	}
	if req.WarehouseCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "warehouse_code is required")
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

	ctx := context.Background()

	// PR must be APPROVED before it can be referenced, and every pr_line_id must belong to it.
	if req.PRID != nil {
		var prStatus string
		if err := h.db.QueryRow(ctx, `SELECT status FROM purchase_request WHERE pr_id=$1`, *req.PRID).Scan(&prStatus); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "PR not found")
		}
		if prStatus != "APPROVED" {
			return fiber.NewError(fiber.StatusBadRequest, "PR must be APPROVED before creating a PO")
		}

		rows, err := h.db.Query(ctx, `SELECT line_id FROM purchase_request_line WHERE pr_id=$1`, *req.PRID)
		if err != nil {
			return err
		}
		validLines := map[int64]bool{}
		for rows.Next() {
			var lineID int64
			rows.Scan(&lineID)
			validLines[lineID] = true
		}
		rows.Close()
		for i, l := range req.Lines {
			if l.PRLineID != nil && !validLines[*l.PRLineID] {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: pr_line_id does not belong to pr_id %d", i, *req.PRID))
			}
		}
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

	// Totals are always computed server-side, never trusted from the request.
	var totalAmount float64
	for _, l := range req.Lines {
		totalAmount += l.QtyOrdered * l.UnitPrice
	}
	vatAmount := totalAmount * 0.07
	netAmount := totalAmount + vatAmount

	var poID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO purchase_order
		  (po_no, po_date, supplier_code, pr_id, rfq_id, warehouse_code, currency,
		   total_amount, vat_amount, net_amount, expected_date, status, payment_terms, remarks, created_by)
		VALUES ($1,CURRENT_DATE,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING po_id`,
		poNo, req.SupplierCode, req.PRID, req.RFQID, req.WarehouseCode, req.Currency,
		totalAmount, vatAmount, netAmount, req.ExpectedDate, status, req.PaymentTerms, req.Remarks, claims.UserID,
	).Scan(&poID)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
			return fiber.NewError(fiber.StatusBadRequest, "invalid supplier_code, warehouse_code, pr_id or rfq_id")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create PO: "+err.Error())
	}

	for i, line := range req.Lines {
		desc, err := normalizeDescription(line.Description)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_order_line (po_id, line_no, mat_code, pr_line_id, qty_ordered, unit_price, description, remarks, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'OPEN')`,
			poID, i+1, line.MatCode, line.PRLineID, line.QtyOrdered, line.UnitPrice, desc, line.Remarks,
		); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: invalid mat_code or pr_line_id", i))
			}
			return fiber.NewError(fiber.StatusInternalServerError, "line insert error: "+err.Error())
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
				INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, status, amount)
				VALUES ('PO',$1,$2,1,$3,'PENDING',$4)
				RETURNING approval_id`, poID, poNo, claims.UserID, totalAmount,
			).Scan(&id); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "approval request error: "+err.Error())
			}
			approvalID = &id
		}
	}

	auditData, _ := json.Marshal(fiber.Map{
		"po_no": poNo, "status": status, "supplier_code": req.SupplierCode,
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

// nextPONumber returns the next sequential PO number for the current month, e.g. PO-202506-0001.
func nextPONumber(ctx context.Context, tx pgx.Tx) (string, error) {
	now := time.Now()
	prefix := fmt.Sprintf("PO-%d%02d-", now.Year(), int(now.Month()))

	var lastNo string
	err := tx.QueryRow(ctx, `
		SELECT po_no FROM purchase_order
		WHERE po_no LIKE $1
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
	h.db.QueryRow(context.Background(), `SELECT status, po_no FROM purchase_order WHERE po_id=$1`, id).Scan(&currentStatus, &poNo)
	if currentStatus != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not pending approval")
	}

	newStatus := "APPROVED"
	if req.Action == "REJECT" {
		newStatus = "REJECTED"
	}

	tx, _ := h.db.Begin(context.Background())
	defer tx.Rollback(context.Background())

	tx.Exec(context.Background(), `UPDATE purchase_order SET status=$1, updated_at=NOW() WHERE po_id=$2`, newStatus, id)
	tx.Exec(context.Background(), `
		INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,$3,$4,$5)`, id, currentStatus, newStatus, claims.UserID, req.Action)

	// Create approval_log
	var approvalID int64
	h.db.QueryRow(context.Background(), `
		SELECT approval_id FROM approval_request
		WHERE doc_type='PO' AND doc_id=$1 AND status='PENDING'
		ORDER BY created_at DESC LIMIT 1`, id).Scan(&approvalID)

	if approvalID > 0 {
		approvalStatus := "APPROVED"
		if req.Action == "REJECT" {
			approvalStatus = "REJECTED"
		}
		tx.Exec(context.Background(), `UPDATE approval_request SET status=$1 WHERE approval_id=$2`, approvalStatus, approvalID)
		tx.Exec(context.Background(), `
			INSERT INTO approval_log (approval_id, doc_type, doc_id, doc_no, step_no, action, action_by, comments, old_status, new_status)
			VALUES ($1,'PO',$2,$3,1,$4,$5,$6,$7,$8)`,
			approvalID, id, poNo, req.Action, claims.UserID, req.Comments, currentStatus, newStatus)
	}

	tx.Commit(context.Background())
	return c.JSON(fiber.Map{"success": true, "message": fmt.Sprintf("PO %s", newStatus)})
}

// SendPO godoc
// @Summary      Send PO to supplier
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {object}  fiber.Map
// @Router       /po/{id}/send [post]
func (h *POHandler) Send(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	id := c.Params("id")

	var currentStatus string
	h.db.QueryRow(context.Background(), `SELECT status FROM purchase_order WHERE po_id=$1`, id).Scan(&currentStatus)
	if currentStatus != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "PO must be approved before sending")
	}

	h.db.Exec(context.Background(), `UPDATE purchase_order SET status='SENT', updated_at=NOW() WHERE po_id=$1`, id)
	h.db.Exec(context.Background(), `
		INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,'APPROVED','SENT',$2,'Sent to supplier')`, id, claims.UserID)

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

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var poPRID *int64
	if err := tx.QueryRow(ctx, `SELECT pr_id FROM purchase_order WHERE po_id=$1`, poID).Scan(&poPRID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}

	// Aggregate by pr_line_id since several request lines can target the same PR line.
	deltas := map[int64]float64{}
	for i, l := range req.Lines {
		if l.PRLineID == nil {
			continue
		}
		if poPRID == nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: PO is not linked to a PR, pr_line_id not allowed", i))
		}
		deltas[*l.PRLineID] += l.QtyOrdered
	}

	if len(deltas) > 0 {
		prLineIDs := make([]int64, 0, len(deltas))
		for id := range deltas {
			prLineIDs = append(prLineIDs, id)
		}

		// Lock the touched PR lines so two concurrent PO saves can't double-book the same line.
		lockRows, err := tx.Query(ctx, `
			SELECT line_id FROM purchase_request_line
			WHERE line_id = ANY($1) AND pr_id = $2
			FOR UPDATE`, prLineIDs, *poPRID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "lock error: "+err.Error())
		}
		locked := map[int64]bool{}
		for lockRows.Next() {
			var id int64
			lockRows.Scan(&id)
			locked[id] = true
		}
		lockRows.Close()
		for _, id := range prLineIDs {
			if !locked[id] {
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
		var lineID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO purchase_order_line (po_id, line_no, mat_code, pr_line_id, qty_ordered, unit_price, description, remarks, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'OPEN')
			RETURNING line_id`,
			poID, startLineNo+i+1, l.MatCode, l.PRLineID, l.QtyOrdered, l.UnitPrice, desc, l.Remarks,
		).Scan(&lineID); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: invalid mat_code or pr_line_id", i))
			}
			return fiber.NewError(fiber.StatusInternalServerError, "line insert error: "+err.Error())
		}
		insertedIDs = append(insertedIDs, lineID)
	}

	for prLineID, qty := range deltas {
		if _, err := tx.Exec(ctx,
			`UPDATE purchase_request_line SET qty_ordered = qty_ordered + $1 WHERE line_id = $2`,
			qty, prLineID,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "pr line update error: "+err.Error())
		}
	}

	var newPRStatus string
	if poPRID != nil {
		var currentPRStatus string
		if err := tx.QueryRow(ctx, `SELECT status FROM purchase_request WHERE pr_id=$1`, *poPRID).Scan(&currentPRStatus); err != nil {
			return err
		}

		lineRows, err := tx.Query(ctx, `SELECT qty_requested, qty_ordered FROM purchase_request_line WHERE pr_id=$1`, *poPRID)
		if err != nil {
			return err
		}
		allFulfilled, someOrdered := true, false
		for lineRows.Next() {
			var qtyReq, qtyOrd float64
			lineRows.Scan(&qtyReq, &qtyOrd)
			if qtyOrd < qtyReq {
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
			if _, err := tx.Exec(ctx, `UPDATE purchase_request SET status=$1, updated_at=NOW() WHERE pr_id=$2`, newPRStatus, *poPRID); err != nil {
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
		WHERE po.po_id=$1`, poID,
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
	desc, err := normalizeDescription(req.Description)
	if err != nil {
		return err
	}

	tag, err := h.db.Exec(context.Background(), `
		UPDATE purchase_order_line SET description=$1 WHERE line_id=$2 AND po_id=$3`,
		desc, lineID, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "update error: "+err.Error())
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "PO line not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"line_id": lineID, "description": desc}})
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
		SELECT l.log_id, l.from_status, l.to_status, u.full_name, l.changed_at, l.remarks
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

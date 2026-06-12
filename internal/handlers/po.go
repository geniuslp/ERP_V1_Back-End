package handlers

import (
	"context"
	"fmt"
	"time"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type POHandler struct {
	db *pgxpool.Pool
}

func NewPOHandler(db *pgxpool.Pool) *POHandler {
	return &POHandler{db: db}
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
		POID          int64     `json:"po_id"`
		PONo          string    `json:"po_no"`
		PODate        time.Time `json:"po_date"`
		ExpectedDate  *string   `json:"expected_date,omitempty"`
		Status        string    `json:"status"`
		SupplierName  string    `json:"supplier_name"`
		SupplierContact *string `json:"supplier_contact,omitempty"`
		Currency      string    `json:"currency"`
		TotalAmount   float64   `json:"total_amount"`
		VATAmount     float64   `json:"vat_amount"`
		NetAmount     float64   `json:"net_amount"`
		PRNo          *string   `json:"pr_no,omitempty"`
		WarehouseName string    `json:"warehouse_name"`
		CreatedBy     string    `json:"created_by"`
		CreatedAt     time.Time `json:"created_at"`
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
		SELECT line_id, po_id, line_no, mat_code, pr_line_id, qty_ordered, qty_received, unit_price, amount, remarks, status
		FROM purchase_order_line WHERE po_id=$1 ORDER BY line_no`, id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var l models.POLine
			rows.Scan(&l.LineID, &l.POID, &l.LineNo, &l.MatCode, &l.PRLineID,
				&l.QtyOrdered, &l.QtyReceived, &l.UnitPrice, &l.Amount, &l.Remarks, &l.Status)
			po.Lines = append(po.Lines, l)
		}
	}

	return c.JSON(fiber.Map{"success": true, "data": po})
}

// CreatePO godoc
// @Summary      Create purchase order
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
	var req models.CreatePORequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line required")
	}

	now := time.Now()
	var seq int64
	h.db.QueryRow(context.Background(), `SELECT COALESCE(MAX(po_id),0)+1 FROM purchase_order`).Scan(&seq)
	poNo := fmt.Sprintf("PO-%s-%06d", now.Format("2006"), seq)

	// Calculate totals
	var totalAmount float64
	for _, l := range req.Lines {
		totalAmount += l.QtyOrdered * l.UnitPrice
	}
	vatAmount := totalAmount * 0.07
	netAmount := totalAmount + vatAmount

	tx, _ := h.db.Begin(context.Background())
	defer tx.Rollback(context.Background())

	var poID int64
	err := tx.QueryRow(context.Background(), `
		INSERT INTO purchase_order
		  (po_no, po_date, supplier_code, pr_id, rfq_id, warehouse_code, currency,
		   total_amount, vat_amount, net_amount, expected_date, status, payment_terms, remarks, created_by)
		VALUES ($1,CURRENT_DATE,$2,$3,$4,$5,$6,$7,$8,$9,$10,'DRAFT',$11,$12,$13)
		RETURNING po_id`,
		poNo, req.SupplierCode, req.PRID, req.RFQID, req.WarehouseCode, req.Currency,
		totalAmount, vatAmount, netAmount, req.ExpectedDate, req.PaymentTerms, req.Remarks, claims.UserID,
	).Scan(&poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create PO: "+err.Error())
	}

	for i, line := range req.Lines {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO purchase_order_line (po_id, line_no, mat_code, pr_line_id, qty_ordered, unit_price, remarks, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'OPEN')`,
			poID, i+1, line.MatCode, line.PRLineID, line.QtyOrdered, line.UnitPrice, line.Remarks)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "line insert error")
		}
	}

	tx.Exec(context.Background(), `
		INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,NULL,'DRAFT',$2,'PO created')`, poID, claims.UserID)

	tx.Commit(context.Background())
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"po_id": poID, "po_no": poNo, "total_amount": totalAmount, "vat_amount": vatAmount, "net_amount": netAmount},
	})
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

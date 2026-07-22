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

// ─── GRN Handler ─────────────────────────────────────────────────────────────

type GRNHandler struct {
	db *pgxpool.Pool
}

func NewGRNHandler(db *pgxpool.Pool) *GRNHandler {
	return &GRNHandler{db: db}
}

// CreateGRN godoc
// @Summary      Create Goods Receipt Note
// @Description  Record goods received against a PO, triggers inventory update on confirmation
// @Tags         GRN
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateGRNRequest  true  "GRN data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /grn [post]
func (h *GRNHandler) Create(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	var req models.CreateGRNRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	// Get supplier from PO
	var supplierCode string
	if err := h.db.QueryRow(context.Background(),
		`SELECT supplier_code FROM purchase_order WHERE id=$1`, req.POID).Scan(&supplierCode); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "PO not found")
	}

	now := time.Now()
	var seq int64
	h.db.QueryRow(context.Background(), `SELECT COALESCE(MAX(id),0)+1 FROM grn`).Scan(&seq)
	grnNo := fmt.Sprintf("GRN-%s-%06d", now.Format("2006"), seq)

	tx, _ := h.db.Begin(context.Background())
	defer tx.Rollback(context.Background())

	var grnID int64
	err := tx.QueryRow(context.Background(), `
		INSERT INTO grn (grn_no, grn_date, po_id, warehouse_code, supplier_code, delivery_note, status, quality_status, received_by, remarks)
		VALUES ($1,CURRENT_DATE,$2,$3,$4,$5,'DRAFT','PENDING',$6,$7)
		RETURNING id`,
		grnNo, req.POID, req.WarehouseCode, supplierCode, req.DeliveryNote, claims.UserID, req.Remarks,
	).Scan(&grnID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create GRN")
	}

	for i, line := range req.Lines {
		tx.Exec(context.Background(), `
			INSERT INTO grn_line (grn_id, line_no, po_line_id, mat_code, zone_id, qty_received, qty_accepted, qty_rejected, quality_remarks)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			grnID, i+1, line.POLineID, line.MatCode, line.ZoneID,
			line.QtyReceived, line.QtyAccepted, line.QtyRejected, line.QualityRemarks)
	}

	tx.Commit(context.Background())
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"grn_id": grnID, "grn_no": grnNo},
	})
}

// ConfirmGRN godoc
// @Summary      Confirm GRN and post to inventory
// @Description  Confirms GRN and automatically creates inventory_transaction (GRN_IN)
// @Tags         GRN
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "GRN ID"
// @Success      200  {object}  fiber.Map
// @Router       /grn/{id}/confirm [post]
func (h *GRNHandler) Confirm(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	id := c.Params("id")

	var status, warehouseCode string
	var grnNo string
	h.db.QueryRow(context.Background(),
		`SELECT status, warehouse_code, grn_no FROM grn WHERE id=$1`, id).Scan(&status, &warehouseCode, &grnNo)

	if status != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest, "GRN is not in DRAFT status")
	}

	tx, _ := h.db.Begin(context.Background())
	defer tx.Rollback(context.Background())

	tx.Exec(context.Background(), `
		UPDATE grn SET status='CONFIRMED', confirmed_by=$1, confirmed_at=NOW() WHERE id=$2`,
		claims.UserID, id)

	// Auto-post inventory transactions for accepted qty
	rows, _ := h.db.Query(context.Background(),
		`SELECT mat_code, qty_accepted FROM grn_line WHERE grn_id=$1 AND qty_accepted > 0`, id)
	if rows != nil {
		defer rows.Close()
		var seq int64
		h.db.QueryRow(context.Background(), `SELECT COALESCE(MAX(id),0)+1 FROM inventory_transaction`).Scan(&seq)

		for rows.Next() {
			var matCode string
			var qty float64
			rows.Scan(&matCode, &qty)

			txnNo := fmt.Sprintf("TXN-%s-%06d", time.Now().Format("2006"), seq)
			docType := "GRN"
			tx.Exec(context.Background(), `
				INSERT INTO inventory_transaction
				  (txn_no, txn_type, mat_code, to_warehouse, qty, ref_doc_type, ref_doc_no, txn_date, created_by)
				VALUES ($1,'GRN_IN',$2,$3,$4,$5,$6,CURRENT_DATE,$7)`,
				txnNo, matCode, warehouseCode, qty, docType, grnNo, claims.UserID)

			// Update inventory balance
			tx.Exec(context.Background(), `
				INSERT INTO inventory (mat_code, warehouse_code, qty_on_hand)
				VALUES ($1,$2,$3)
				ON CONFLICT (mat_code, warehouse_code)
				DO UPDATE SET qty_on_hand = inventory.qty_on_hand + $3, updated_at = NOW()`,
				matCode, warehouseCode, qty)

			// Update PO line received qty
			tx.Exec(context.Background(), `
				UPDATE purchase_order_line SET qty_received = qty_received + $1
				WHERE id IN (SELECT po_line_id FROM grn_line WHERE grn_id=$2 AND mat_code=$3 LIMIT 1)`,
				qty, id, matCode)

			seq++
		}
	}

	tx.Commit(context.Background())
	return c.JSON(fiber.Map{"success": true, "message": "GRN confirmed and inventory updated"})
}

// ListGRN godoc
// @Summary      List GRNs
// @Tags         GRN
// @Security     BearerAuth
// @Produce      json
// @Param        po_id     query  int     false  "PO ID filter"
// @Param        status    query  string  false  "status filter"
// @Param        page      query  int     false  "page"  default(1)
// @Param        page_size query  int     false  "page size"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /grn [get]
func (h *GRNHandler) List(c *fiber.Ctx) error {
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	var total int64
	h.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM grn`).Scan(&total)

	rows, err := h.db.Query(context.Background(), `
		SELECT g.id, g.grn_no, g.grn_date, g.po_id, po.po_no, g.warehouse_code,
		       g.supplier_code, s.supplier_name, g.status, g.quality_status,
		       u.full_name AS received_by, g.created_at
		FROM grn g
		JOIN purchase_order po ON po.id = g.po_id
		JOIN supplier s ON s.supplier_code = g.supplier_code
		JOIN users u ON u.id = g.received_by
		ORDER BY g.created_at DESC LIMIT $1 OFFSET $2`, size, offset)
	if err != nil {
		return err
	}
	defer rows.Close()

	type GRNRow struct {
		GRNID         int64     `json:"grn_id"`
		GRNNo         string    `json:"grn_no"`
		GRNDate       time.Time `json:"grn_date"`
		POID          int64     `json:"po_id"`
		PONo          string    `json:"po_no"`
		WarehouseCode string    `json:"warehouse_code"`
		SupplierCode  string    `json:"supplier_code"`
		SupplierName  string    `json:"supplier_name"`
		Status        string    `json:"status"`
		QualityStatus string    `json:"quality_status"`
		ReceivedBy    string    `json:"received_by"`
		CreatedAt     time.Time `json:"created_at"`
	}
	var items []GRNRow
	for rows.Next() {
		var r GRNRow
		rows.Scan(&r.GRNID, &r.GRNNo, &r.GRNDate, &r.POID, &r.PONo, &r.WarehouseCode,
			&r.SupplierCode, &r.SupplierName, &r.Status, &r.QualityStatus, &r.ReceivedBy, &r.CreatedAt)
		items = append(items, r)
	}

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data":    models.PaginatedResponse{Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages},
	})
}

// ─── Approval Handler ─────────────────────────────────────────────────────────

type ApprovalHandler struct {
	db *pgxpool.Pool
}

func NewApprovalHandler(db *pgxpool.Pool) *ApprovalHandler {
	return &ApprovalHandler{db: db}
}

// PendingApprovals godoc
// @Summary      List pending approvals for current user
// @Tags         Approvals
// @Security     BearerAuth
// @Produce      json
// @Param        doc_type  query  string  false  "filter by doc_type (PR|PO|GRN)"
// @Success      200  {array}  models.ApprovalRequest
// @Router       /approvals/pending [get]
func (h *ApprovalHandler) Pending(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	docType := c.Query("doc_type")

	q := `
		SELECT ar.id, ar.doc_type, ar.doc_id, ar.doc_no, ar.step_no,
		       ac.step_name, ar.requested_by, ar.assigned_to, ar.status,
		       ar.due_date, ar.amount, ar.created_at
		FROM v_pending_approvals ar
		JOIN approval_config ac ON ac.doc_type = ar.doc_type AND ac.step_no = ar.step_no
		WHERE (ar.assigned_to = $1 OR ar.assigned_to IS NULL)`
	args := []interface{}{claims.UserID}

	if docType != "" {
		q += ` AND ar.doc_type = $2`
		args = append(args, docType)
	}
	q += ` ORDER BY ar.created_at`

	rows, err := h.db.Query(context.Background(), q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.ApprovalRequest
	for rows.Next() {
		var r models.ApprovalRequest
		rows.Scan(&r.ApprovalID, &r.DocType, &r.DocID, &r.DocNo, &r.StepNo,
			&r.StepName, &r.RequestedBy, &r.AssignedTo, &r.Status, &r.DueDate, &r.Amount, &r.CreatedAt)
		items = append(items, r)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetApprovalLogs godoc
// @Summary      Get approval history for a document
// @Tags         Approvals
// @Security     BearerAuth
// @Produce      json
// @Param        doc_type  query  string  true  "PR|PO|GRN"
// @Param        doc_id    query  int     true  "document ID"
// @Success      200  {array}  models.ApprovalLog
// @Router       /approvals/logs [get]
func (h *ApprovalHandler) Logs(c *fiber.Ctx) error {
	docType := c.Query("doc_type")
	docID := c.Query("doc_id")

	rows, err := h.db.Query(context.Background(), `
		SELECT al.id, al.approval_id, al.doc_type, al.doc_no, al.step_no,
		       al.action, al.action_by, u.full_name, al.action_at, al.comments,
		       al.old_status, al.new_status
		FROM approval_log al
		JOIN users u ON u.id = al.action_by
		WHERE al.doc_type=$1 AND al.doc_id=$2
		ORDER BY al.action_at`, docType, docID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var logs []models.ApprovalLog
	for rows.Next() {
		var l models.ApprovalLog
		rows.Scan(&l.LogID, &l.ApprovalID, &l.DocType, &l.DocNo, &l.StepNo,
			&l.Action, &l.ActionBy, &l.ActionByName, &l.ActionAt, &l.Comments,
			&l.OldStatus, &l.NewStatus)
		logs = append(logs, l)
	}
	return c.JSON(fiber.Map{"success": true, "data": logs})
}

// GetAuditLogs godoc
// @Summary      Get general ERP audit log
// @Tags         Approvals
// @Security     BearerAuth
// @Produce      json
// @Param        table_name  query  string  false  "table name filter"
// @Param        record_id   query  int     false  "record ID"
// @Param        page        query  int     false  "page"  default(1)
// @Param        page_size   query  int     false  "page size"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /approvals/audit [get]
func (h *ApprovalHandler) AuditLogs(c *fiber.Ctx) error {
	tableName := c.Query("table_name")
	recordID := c.Query("record_id")
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	where := "1=1"
	args := []interface{}{}
	idx := 1
	if tableName != "" {
		where += fmt.Sprintf(" AND table_name=$%d", idx)
		args = append(args, tableName)
		idx++
	}
	if recordID != "" {
		where += fmt.Sprintf(" AND record_id=$%d", idx)
		args = append(args, recordID)
		idx++
	}

	var total int64
	h.db.QueryRow(context.Background(), fmt.Sprintf(`SELECT COUNT(*) FROM erp_audit_log WHERE %s`, where), args...).Scan(&total)

	args = append(args, size, offset)
	rows, err := h.db.Query(context.Background(), fmt.Sprintf(`
		SELECT al.id, al.table_name, al.record_id, al.action, u.full_name, al.changed_at, al.old_data, al.new_data
		FROM erp_audit_log al LEFT JOIN users u ON u.id = al.changed_by
		WHERE %s ORDER BY al.changed_at DESC LIMIT $%d OFFSET $%d`, where, idx, idx+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type AuditRow struct {
		AuditID   int64     `json:"audit_id"`
		TableName string    `json:"table_name"`
		RecordID  int64     `json:"record_id"`
		Action    string    `json:"action"`
		ChangedBy *string   `json:"changed_by"`
		ChangedAt time.Time `json:"changed_at"`
		OldData   *string   `json:"old_data,omitempty"`
		NewData   *string   `json:"new_data,omitempty"`
	}
	var items []AuditRow
	for rows.Next() {
		var r AuditRow
		rows.Scan(&r.AuditID, &r.TableName, &r.RecordID, &r.Action, &r.ChangedBy, &r.ChangedAt, &r.OldData, &r.NewData)
		items = append(items, r)
	}

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data":    models.PaginatedResponse{Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages},
	})
}

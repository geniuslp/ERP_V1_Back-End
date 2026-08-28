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
	"erp-api/internal/models"
)

// RequisitionHandler handles ใบเบิกของ (Stock Requisition): withdraw material
// from a warehouse to a project site. Uses the pre-existing requisition /
// requisition_line tables (requisition_status_log is deliberately unused —
// stock_transaction is the single source of truth for movement history) — a
// separate design from StockTransferHandler, sharing only stock_transaction.
type RequisitionHandler struct{ db *pgxpool.Pool }

func NewRequisitionHandler(db *pgxpool.Pool) *RequisitionHandler {
	return &RequisitionHandler{db: db}
}

// Create godoc
// @Summary      สร้างใบเบิกของ (DRAFT)
// @Tags         Requisition
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateRequisitionRequest  true  "Requisition data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      422   {object}  fiber.Map
// @Router       /requisition [post]
func (h *RequisitionHandler) Create(c *fiber.Ctx) error {
	var req models.CreateRequisitionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.ProjectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "project_code is required")
	}
	if req.WarehouseCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "warehouse_code is required")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line is required")
	}
	if req.IsStockHouse && (req.ContainerNo == nil || strings.TrimSpace(*req.ContainerNo) == "") {
		return fiber.NewError(fiber.StatusBadRequest, "container_no is required when is_stock_house is true")
	}
	for i, ln := range req.Lines {
		if ln.MatCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: mat_code is required", i))
		}
		if ln.QtyRequested <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: qty_requested must be positive", i))
		}
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var seq int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(id), 0)+1 FROM requisition`).Scan(&seq); err != nil {
		return err
	}
	reqNo := fmt.Sprintf("REQ-%s-%06d", time.Now().Format("2006"), seq)

	reqDate := time.Now().Format("2006-01-02")
	if req.ReqDate != nil && *req.ReqDate != "" {
		reqDate = *req.ReqDate
	}

	var reqID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO requisition (req_no, project_code, warehouse_code, requester_id, req_date, purpose, remarks, status, created_by, is_stock_house, container_no)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'DRAFT',$8,$9,$10)
		RETURNING id`,
		reqNo, req.ProjectCode, req.WarehouseCode, claims.UserID, reqDate, req.Purpose, req.Remarks, claims.UserID,
		req.IsStockHouse, req.ContainerNo,
	).Scan(&reqID)
	if err != nil {
		return err
	}

	for i, ln := range req.Lines {
		var stockItemID int64
		var unit string
		err := tx.QueryRow(ctx, `
			SELECT id, unit FROM stock_item WHERE mat_code=$1 AND warehouse_code=$2`,
			ln.MatCode, req.WarehouseCode,
		).Scan(&stockItemID, &unit)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity,
				fmt.Sprintf("line[%d]: mat_code %s not found in warehouse %s", i, ln.MatCode, req.WarehouseCode))
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO requisition_line (req_id, line_no, stock_item_id, mat_code, unit, qty_requested, remarks)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			reqID, i+1, stockItemID, ln.MatCode, unit, ln.QtyRequested, ln.Remarks,
		)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"id": reqID, "req_no": reqNo},
	})
}

// List godoc
// @Summary      รายการใบเบิกของ
// @Tags         Requisition
// @Security     BearerAuth
// @Produce      json
// @Param        status          query  string  false  "Status"
// @Param        project_code    query  string  false  "Project code"
// @Param        warehouse_code  query  string  false  "Warehouse code"
// @Param        is_stock_house  query  bool    false  "Filter by stock house flag"
// @Param        date_from       query  string  false  "Date from (YYYY-MM-DD)"
// @Param        date_to         query  string  false  "Date to (YYYY-MM-DD)"
// @Param        page            query  int     false  "Page"
// @Param        page_size       query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /requisition [get]
func (h *RequisitionHandler) List(c *fiber.Ctx) error {
	var f models.RequisitionFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}

	ctx := context.Background()

	where := []string{"1=1"}
	args := []any{}
	i := 1
	if f.Status != "" {
		where = append(where, fmt.Sprintf("r.status = $%d", i))
		args = append(args, f.Status)
		i++
	}
	if f.ProjectCode != "" {
		where = append(where, fmt.Sprintf("r.project_code = $%d", i))
		args = append(args, f.ProjectCode)
		i++
	}
	if f.WarehouseCode != "" {
		where = append(where, fmt.Sprintf("r.warehouse_code = $%d", i))
		args = append(args, f.WarehouseCode)
		i++
	}
	if f.IsStockHouse != "" {
		where = append(where, fmt.Sprintf("r.is_stock_house = $%d", i))
		args = append(args, f.IsStockHouse == "true")
		i++
	}
	if f.DateFrom != "" {
		where = append(where, fmt.Sprintf("r.req_date >= $%d", i))
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where = append(where, fmt.Sprintf("r.req_date <= $%d", i))
		args = append(args, f.DateTo)
		i++
	}
	whereClause := strings.Join(where, " AND ")

	joinClause := `
		FROM requisition r
		LEFT JOIN project p ON p.project_code = r.project_code
		LEFT JOIN warehouse w ON w.warehouse_code = r.warehouse_code
		LEFT JOIN users ru ON ru.id = r.requester_id`

	var total int64
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, joinClause, whereClause), args...).Scan(&total); err != nil {
		return err
	}

	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT r.id, r.req_no, r.project_code, p.project_name, r.warehouse_code, w.warehouse_name,
		       r.requester_id, ru.full_name, TO_CHAR(r.req_date,'YYYY-MM-DD'), r.purpose, r.status,
		       r.checked_by, r.checked_at, r.remarks, r.created_at, r.updated_at,
		       r.is_stock_house, r.container_no
		%s
		WHERE %s
		ORDER BY r.id DESC
		LIMIT $%d OFFSET $%d`, joinClause, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.Requisition
	for rows.Next() {
		var r models.Requisition
		if err := rows.Scan(
			&r.ID, &r.ReqNo, &r.ProjectCode, &r.ProjectName, &r.WarehouseCode, &r.WarehouseName,
			&r.RequesterID, &r.RequesterName, &r.ReqDate, &r.Purpose, &r.Status,
			&r.CheckedBy, &r.CheckedAt, &r.Remarks, &r.CreatedAt, &r.UpdatedAt,
			&r.IsStockHouse, &r.ContainerNo,
		); err != nil {
			return err
		}
		items = append(items, r)
	}
	if items == nil {
		items = []models.Requisition{}
	}

	totalPages := int(total) / f.PageSize
	if int(total)%f.PageSize != 0 {
		totalPages++
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: f.Page, PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// Get godoc
// @Summary      รายละเอียดใบเบิกของ
// @Tags         Requisition
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Requisition ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /requisition/{id} [get]
func (h *RequisitionHandler) Get(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var r models.Requisition
	err = h.db.QueryRow(ctx, `
		SELECT r.id, r.req_no, r.project_code, p.project_name, r.warehouse_code, w.warehouse_name,
		       r.requester_id, ru.full_name, TO_CHAR(r.req_date,'YYYY-MM-DD'), r.purpose, r.status,
		       r.checked_by, cu.full_name, r.checked_at, r.remarks, r.created_at, r.updated_at,
		       r.is_stock_house, r.container_no
		FROM requisition r
		LEFT JOIN project p ON p.project_code = r.project_code
		LEFT JOIN warehouse w ON w.warehouse_code = r.warehouse_code
		LEFT JOIN users ru ON ru.id = r.requester_id
		LEFT JOIN users cu ON cu.id = r.checked_by
		WHERE r.id=$1`, id,
	).Scan(&r.ID, &r.ReqNo, &r.ProjectCode, &r.ProjectName, &r.WarehouseCode, &r.WarehouseName,
		&r.RequesterID, &r.RequesterName, &r.ReqDate, &r.Purpose, &r.Status,
		&r.CheckedBy, &r.CheckedByName, &r.CheckedAt, &r.Remarks, &r.CreatedAt, &r.UpdatedAt,
		&r.IsStockHouse, &r.ContainerNo)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "requisition not found")
	}

	rows, err := h.db.Query(ctx, `
		SELECT rl.id, rl.req_id, rl.line_no, rl.stock_item_id, rl.mat_code, si.item_name, rl.unit,
		       rl.qty_requested, rl.qty_issued, rl.unit_cost, rl.total_cost, rl.remarks
		FROM requisition_line rl
		LEFT JOIN stock_item si ON si.id = rl.stock_item_id
		WHERE rl.req_id=$1
		ORDER BY rl.line_no`, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lines []models.RequisitionLine
	for rows.Next() {
		var ln models.RequisitionLine
		if err := rows.Scan(&ln.ID, &ln.ReqID, &ln.LineNo, &ln.StockItemID, &ln.MatCode, &ln.ItemName, &ln.Unit,
			&ln.QtyRequested, &ln.QtyIssued, &ln.UnitCost, &ln.TotalCost, &ln.Remarks); err != nil {
			return err
		}
		lines = append(lines, ln)
	}
	r.Lines = lines

	return c.JSON(fiber.Map{"success": true, "data": r})
}

// Cancel godoc
// @Summary      ยกเลิกใบเบิกของ (DRAFT → CANCELLED)
// @Tags         Requisition
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Requisition ID"
// @Success      200  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Router       /requisition/{id}/cancel [post]
func (h *RequisitionHandler) Cancel(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var status string
	if err := h.db.QueryRow(ctx, `SELECT status FROM requisition WHERE id=$1`, id).Scan(&status); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "requisition not found")
	}
	if status != "DRAFT" {
		return fiber.NewError(fiber.StatusConflict, "requisition must be DRAFT to cancel")
	}

	tag, err := h.db.Exec(ctx, `UPDATE requisition SET status='CANCELLED', updated_at=NOW(), updated_by=$1 WHERE id=$2`,
		claims.UserID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "requisition not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "requisition cancelled"})
}

// Confirm godoc
// @Summary      ยืนยันใบเบิกของ (DRAFT → CONFIRMED) — ตัดสต๊อกคลังทันที เพิ่มยอดที่โครงการ
// @Description  One DB transaction. Locks each line's stock_item row (SELECT ... FOR UPDATE), verifies
// @Description  sufficient qty, decrements stock_item.qty at the requisition's warehouse, upserts
// @Description  project_stock for the requisition's project_code, and records a stock_transaction
// @Description  (txn_type='OUT', ref_doc_type=REQUISITION) per line for movement history.
// @Tags         Requisition
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Requisition ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Router       /requisition/{id}/confirm [post]
func (h *RequisitionHandler) Confirm(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var status, projectCode, warehouseCode string
	if err := h.db.QueryRow(ctx, `SELECT status, project_code, warehouse_code FROM requisition WHERE id=$1`, id).
		Scan(&status, &projectCode, &warehouseCode); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "requisition not found")
	}
	if status != "DRAFT" {
		return fiber.NewError(fiber.StatusConflict, fmt.Sprintf("requisition must be DRAFT (current: %s)", status))
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, stock_item_id, mat_code, unit, qty_requested
		FROM requisition_line WHERE req_id=$1 ORDER BY line_no`, id)
	if err != nil {
		return err
	}
	type line struct {
		ID           int64
		StockItemID  *int64
		MatCode      string
		Unit         *string
		QtyRequested float64
	}
	var lines []line
	for rows.Next() {
		var ln line
		if err := rows.Scan(&ln.ID, &ln.StockItemID, &ln.MatCode, &ln.Unit, &ln.QtyRequested); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, ln)
	}
	rows.Close()

	for _, ln := range lines {
		if ln.StockItemID == nil {
			return fiber.NewError(fiber.StatusConflict, fmt.Sprintf("mat_code %s has no linked stock item", ln.MatCode))
		}

		var qtyOnHand float64
		if err := tx.QueryRow(ctx, `SELECT qty FROM stock_item WHERE id=$1 FOR UPDATE`, *ln.StockItemID).Scan(&qtyOnHand); err != nil {
			return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("stock item for %s not found", ln.MatCode))
		}
		if qtyOnHand < ln.QtyRequested {
			return fiber.NewError(fiber.StatusConflict,
				fmt.Sprintf("insufficient stock for %s (have %.4f, need %.4f)", ln.MatCode, qtyOnHand, ln.QtyRequested))
		}

		qtyAfter := qtyOnHand - ln.QtyRequested
		if _, err := tx.Exec(ctx, `UPDATE stock_item SET qty=$1, updated_at=NOW() WHERE id=$2`, qtyAfter, *ln.StockItemID); err != nil {
			return err
		}

		if err := addToProjectStock(ctx, tx, projectCode, ln.MatCode, ln.Unit, ln.QtyRequested); err != nil {
			return err
		}

		txnNo, err := generateTxnNo(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_transaction
				(txn_no, txn_type, item_id, from_location, to_location, qty, qty_before, qty_after,
				 ref_doc_type, ref_doc_id, created_by)
			VALUES ($1,'OUT',$2,$3,$4,$5,$6,$7,'REQUISITION',$8,$9)`,
			txnNo, *ln.StockItemID, warehouseCode, projectCode, ln.QtyRequested, qtyOnHand, qtyAfter, id, claims.UserID,
		); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `UPDATE requisition_line SET qty_issued=$1 WHERE id=$2`, ln.QtyRequested, ln.ID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE requisition SET status='CONFIRMED', checked_by=$1, checked_at=NOW(), updated_at=NOW(), updated_by=$1
		WHERE id=$2`, claims.UserID, id); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "requisition confirmed"})
}

// History godoc
// @Summary      ประวัติการเบิกของ (จาก stock_transaction)
// @Tags         Requisition
// @Security     BearerAuth
// @Produce      json
// @Param        mat_code   query  string  false  "Material code"
// @Param        date_from  query  string  false  "Date from (YYYY-MM-DD)"
// @Param        date_to    query  string  false  "Date to (YYYY-MM-DD)"
// @Param        page       query  int     false  "Page"
// @Param        page_size  query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /requisition/history [get]
func (h *RequisitionHandler) History(c *fiber.Ctx) error {
	matCode := c.Query("mat_code")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	ctx := context.Background()

	where := []string{"st.ref_doc_type = 'REQUISITION'"}
	args := []any{}
	i := 1
	if matCode != "" {
		where = append(where, fmt.Sprintf("si.mat_code = $%d", i))
		args = append(args, matCode)
		i++
	}
	if dateFrom != "" {
		where = append(where, fmt.Sprintf("st.txn_date >= $%d", i))
		args = append(args, dateFrom)
		i++
	}
	if dateTo != "" {
		where = append(where, fmt.Sprintf("st.txn_date <= $%d", i))
		args = append(args, dateTo)
		i++
	}
	whereClause := strings.Join(where, " AND ")

	joinClause := `
		FROM stock_transaction st
		LEFT JOIN stock_item si ON si.id = st.item_id`

	var total int64
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, joinClause, whereClause), args...).Scan(&total); err != nil {
		return err
	}

	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT st.id, st.txn_no, st.txn_type, st.item_id, si.mat_code, st.from_location, st.to_location,
		       st.qty, st.qty_before, st.qty_after, st.ref_doc_type, st.ref_doc_id, st.txn_date, st.remarks
		%s
		WHERE %s
		ORDER BY st.id DESC
		LIMIT $%d OFFSET $%d`, joinClause, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type historyRow struct {
		ID         int64     `json:"id"`
		TxnNo      string    `json:"txn_no"`
		TxnType    string    `json:"txn_type"`
		ItemID     int64     `json:"item_id"`
		MatCode    *string   `json:"mat_code"`
		FromLoc    *string   `json:"from_location"`
		ToLoc      *string   `json:"to_location"`
		Qty        float64   `json:"qty"`
		QtyBefore  float64   `json:"qty_before"`
		QtyAfter   float64   `json:"qty_after"`
		RefDocType *string   `json:"ref_doc_type"`
		RefDocID   *int64    `json:"ref_doc_id"`
		TxnDate    time.Time `json:"txn_date"`
		Remarks    *string   `json:"remarks"`
	}

	var items []historyRow
	for rows.Next() {
		var r historyRow
		if err := rows.Scan(&r.ID, &r.TxnNo, &r.TxnType, &r.ItemID, &r.MatCode, &r.FromLoc, &r.ToLoc,
			&r.Qty, &r.QtyBefore, &r.QtyAfter, &r.RefDocType, &r.RefDocID, &r.TxnDate, &r.Remarks); err != nil {
			return err
		}
		items = append(items, r)
	}
	if items == nil {
		items = []historyRow{}
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		totalPages++
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages,
		},
	})
}

package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

type StockBorrowHandler struct{ db *pgxpool.Pool }

func NewStockBorrowHandler(db *pgxpool.Pool) *StockBorrowHandler {
	return &StockBorrowHandler{db: db}
}

func generateBorrowNo(ctx context.Context, tx pgx.Tx) (string, error) {
	var seq int64
	if err := tx.QueryRow(ctx, "SELECT nextval('borrow_seq')").Scan(&seq); err != nil {
		return "", err
	}
	return fmt.Sprintf("BOR-%s-%04d", timeYYMM(), seq), nil
}

// rollupStockItemQty recomputes stock_item.qty as the sum of stock_inventory.qty_on_hand
// across all locations for itemID, mirroring the rollup goods_receipt.go performs after
// every stock_inventory write — keeps stock_item.qty an accurate system-wide total for
// any caller that reads it without going through stock_inventory directly.
func rollupStockItemQty(ctx context.Context, tx pgx.Tx, itemID int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE stock_item SET qty = (
			SELECT COALESCE(SUM(qty_on_hand), 0) FROM stock_inventory WHERE item_id = $1
		), updated_at = NOW()
		WHERE id = $1`, itemID)
	return err
}

func (h *StockBorrowHandler) loadLines(ctx context.Context, borrowID int64) ([]models.BorrowLine, error) {
	rows, err := h.db.Query(ctx, `
		SELECT bl.id, bl.borrow_id, bl.line_no,
		       bl.stock_item_id, si.mat_code, si.item_name, si.item_type,
		       bl.location_code, COALESCE(l.location_name,'') AS location_name,
		       si.unit, bl.qty_requested, bl.qty_approved,
		       bl.qty_borrowed, bl.qty_returned,
		       bl.condition_out, bl.condition_in, bl.remarks
		FROM borrow_line bl
		JOIN stock_item si ON si.id = bl.stock_item_id
		LEFT JOIN location l ON l.location_code = bl.location_code
		WHERE bl.borrow_id = $1
		ORDER BY bl.line_no`, borrowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []models.BorrowLine
	for rows.Next() {
		var ln models.BorrowLine
		if err := rows.Scan(
			&ln.ID, &ln.BorrowID, &ln.LineNo,
			&ln.StockItemID, &ln.MatCode, &ln.ItemName, &ln.ItemType,
			&ln.LocationCode, &ln.LocationName,
			&ln.Unit, &ln.QtyRequested, &ln.QtyApproved,
			&ln.QtyBorrowed, &ln.QtyReturned,
			&ln.ConditionOut, &ln.ConditionIn, &ln.Remarks,
		); err != nil {
			return nil, err
		}
		lines = append(lines, ln)
	}
	return lines, nil
}

// List godoc
// @Summary      รายการ Borrow Request
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        status    query  string  false  "Status"
// @Param        date_from query  string  false  "Date from"
// @Param        date_to   query  string  false  "Date to"
// @Param        search    query  string  false  "Search"
// @Param        page      query  int     false  "Page"
// @Param        page_size query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /stock/borrow [get]
func (h *StockBorrowHandler) List(c *fiber.Ctx) error {
	var f models.BorrowFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}

	ctx := context.Background()
	where := []string{"1=1"}
	args := []interface{}{}
	i := 1

	if f.Status != "" {
		where = append(where, fmt.Sprintf("b.status = $%d", i))
		args = append(args, f.Status)
		i++
	}
	if f.DateFrom != "" {
		where = append(where, fmt.Sprintf("b.borrow_date >= $%d", i))
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where = append(where, fmt.Sprintf("b.borrow_date <= $%d", i))
		args = append(args, f.DateTo)
		i++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf("(b.borrow_no ILIKE $%d OR u.full_name ILIKE $%d)", i, i+1))
		like := "%" + f.Search + "%"
		args = append(args, like, like)
		i += 2
	}

	whereClause := strings.Join(where, " AND ")

	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM borrow b
		JOIN users u ON u.id = b.borrower_id
		LEFT JOIN users ap ON ap.id = b.approved_by
		WHERE %s`, whereClause), countArgs...).Scan(&total); err != nil {
		return err
	}

	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT b.id, b.borrow_no, b.status, b.purpose,
		       u.full_name AS borrower_name, b.borrower_id,
		       ap.full_name AS approved_by_name,
		       TO_CHAR(b.borrow_date,'YYYY-MM-DD'),
		       TO_CHAR(b.expected_return,'YYYY-MM-DD'),
		       TO_CHAR(b.actual_return,'YYYY-MM-DD'),
		       b.remarks, b.created_at
		FROM borrow b
		JOIN users u ON u.id = b.borrower_id
		LEFT JOIN users ap ON ap.id = b.approved_by
		WHERE %s
		ORDER BY b.created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var list []models.BorrowRequest
	for rows.Next() {
		var br models.BorrowRequest
		if err := rows.Scan(
			&br.ID, &br.BorrowNo, &br.Status, &br.Purpose,
			&br.BorrowerName, &br.BorrowerID,
			&br.ApprovedByName,
			&br.BorrowDate, &br.ExpectedReturn, &br.ActualReturn,
			&br.Remarks, &br.CreatedAt,
		); err != nil {
			return err
		}
		list = append(list, br)
	}
	if list == nil {
		list = []models.BorrowRequest{}
	}

	totalPages := int(total) / f.PageSize
	if int(total)%f.PageSize != 0 {
		totalPages++
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: list, Total: total, Page: f.Page, PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// GetByID godoc
// @Summary      ดู Borrow Request ตาม ID
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Borrow ID"
// @Success      200  {object}  fiber.Map
// @Router       /stock/borrow/{id} [get]
func (h *StockBorrowHandler) GetByID(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var br models.BorrowRequest
	err = h.db.QueryRow(ctx, `
		SELECT b.id, b.borrow_no, b.status, b.purpose,
		       u.full_name AS borrower_name, b.borrower_id,
		       ap.full_name AS approved_by_name,
		       TO_CHAR(b.borrow_date,'YYYY-MM-DD'),
		       TO_CHAR(b.expected_return,'YYYY-MM-DD'),
		       TO_CHAR(b.actual_return,'YYYY-MM-DD'),
		       b.remarks, b.created_at
		FROM borrow b
		JOIN users u ON u.id = b.borrower_id
		LEFT JOIN users ap ON ap.id = b.approved_by
		WHERE b.id = $1`, id,
	).Scan(
		&br.ID, &br.BorrowNo, &br.Status, &br.Purpose,
		&br.BorrowerName, &br.BorrowerID,
		&br.ApprovedByName,
		&br.BorrowDate, &br.ExpectedReturn, &br.ActualReturn,
		&br.Remarks, &br.CreatedAt,
	)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "borrow request not found")
	}

	br.Lines, err = h.loadLines(ctx, id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": br})
}

// Create godoc
// @Summary      สร้าง Borrow Request
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateBorrowRequest  true  "Request body"
// @Success      201  {object}  fiber.Map
// @Router       /stock/borrow [post]
func (h *StockBorrowHandler) Create(c *fiber.Ctx) error {
	var req models.CreateBorrowRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "lines are required")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	for _, ln := range req.Lines {
		if ln.QtyRequested <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, "qty_requested must be positive")
		}
		var available float64
		err := h.db.QueryRow(ctx, `
			SELECT COALESCE(qty_on_hand - qty_reserved, 0)
			FROM stock_inventory
			WHERE item_id=$1 AND location_code=$2`, ln.StockItemID, ln.LocationCode).Scan(&available)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("item %d not found in location %s", ln.StockItemID, ln.LocationCode))
		}
		if available < ln.QtyRequested {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("insufficient stock for item %d", ln.StockItemID))
		}
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	borrowNo, err := generateBorrowNo(ctx, tx)
	if err != nil {
		return err
	}

	var borrowID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO borrow (borrow_no, borrower_id, purpose, expected_return, remarks, status)
		VALUES ($1,$2,$3,$4,$5,'DRAFT')
		RETURNING id`,
		borrowNo, claims.UserID, req.Purpose, req.ExpectedReturn, req.Remarks,
	).Scan(&borrowID)
	if err != nil {
		return err
	}

	for _, ln := range req.Lines {
		_, err = tx.Exec(ctx, `
			INSERT INTO borrow_line (borrow_id, line_no, stock_item_id, location_code, qty_requested, remarks)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			borrowID, ln.LineNo, ln.StockItemID, ln.LocationCode, ln.QtyRequested, ln.Remarks)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO borrow_status_log (borrow_id, old_status, new_status, changed_by)
		VALUES ($1, NULL, 'DRAFT', $2)`, borrowID, claims.UserID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": fiber.Map{"id": borrowID, "borrow_no": borrowNo}})
}

// Submit godoc
// @Summary      Submit Borrow Request
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Borrow ID"
// @Success      200  {object}  fiber.Map
// @Router       /stock/borrow/{id}/submit [post]
func (h *StockBorrowHandler) Submit(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var status string
	var borrowerID int64
	if err := h.db.QueryRow(ctx, `SELECT status, borrower_id FROM borrow WHERE id=$1`, id).Scan(&status, &borrowerID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "borrow request not found")
	}
	if status != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest, "only DRAFT requests can be submitted")
	}
	if borrowerID != claims.UserID {
		return fiber.NewError(fiber.StatusForbidden, "only the borrower can submit")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `UPDATE borrow SET status='PENDING_APPROVAL', updated_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO borrow_status_log (borrow_id, old_status, new_status, changed_by)
		VALUES ($1,'DRAFT','PENDING_APPROVAL',$2)`, id, claims.UserID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true})
}

// Approve godoc
// @Summary      Approve / Reject Borrow Request
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                           true  "Borrow ID"
// @Param        body  body  models.BorrowApprovalRequest  true  "Request body"
// @Success      200  {object}  fiber.Map
// @Router       /stock/borrow/{id}/approve [post]
func (h *StockBorrowHandler) Approve(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.BorrowApprovalRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.Action != "APPROVE" && req.Action != "REJECT" {
		return fiber.NewError(fiber.StatusBadRequest, "action must be APPROVE or REJECT")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var status string
	if err := h.db.QueryRow(ctx, `SELECT status FROM borrow WHERE id=$1`, id).Scan(&status); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "borrow request not found")
	}
	if status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "request is not pending approval")
	}

	newStatus := "APPROVED"
	if req.Action == "REJECT" {
		newStatus = "REJECTED"
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE borrow SET status=$1, approved_by=$2, updated_at=NOW() WHERE id=$3`,
		newStatus, claims.UserID, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO borrow_status_log (borrow_id, old_status, new_status, changed_by, remarks)
		VALUES ($1,'PENDING_APPROVAL',$2,$3,$4)`,
		id, newStatus, claims.UserID, req.Comments)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"status": newStatus}})
}

// Receive godoc
// @Summary      รับ Items จาก Borrow (scan QR)
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                          true  "Borrow ID"
// @Param        body  body  models.BorrowReceiveRequest  true  "Request body"
// @Success      200  {object}  fiber.Map
// @Router       /stock/borrow/{id}/receive [post]
func (h *StockBorrowHandler) Receive(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.BorrowReceiveRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var status string
	if err := h.db.QueryRow(ctx, `SELECT status FROM borrow WHERE id=$1`, id).Scan(&status); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "borrow request not found")
	}
	if status != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "borrow request must be APPROVED")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, ln := range req.Lines {
		var itemID int64
		var locationCode string
		if err := tx.QueryRow(ctx, `
			SELECT stock_item_id, location_code FROM borrow_line WHERE id=$1 AND borrow_id=$2`,
			ln.BorrowLineID, id).Scan(&itemID, &locationCode); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("borrow_line_id %d not found", ln.BorrowLineID))
		}

		_, err = tx.Exec(ctx, `
			UPDATE borrow_line SET qty_borrowed=$1, condition_out=$2 WHERE id=$3`,
			ln.QtyBorrowed, ln.ConditionOut, ln.BorrowLineID)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			UPDATE stock_inventory SET qty_on_hand = qty_on_hand - $1, updated_at=NOW()
			WHERE item_id=$2 AND location_code=$3`, ln.QtyBorrowed, itemID, locationCode)
		if err != nil {
			return err
		}
		if err := rollupStockItemQty(ctx, tx, itemID); err != nil {
			return err
		}

		var seq int64
		tx.QueryRow(ctx, "SELECT nextval('stock_txn_seq')").Scan(&seq)
		txnNo := fmt.Sprintf("TXN-%s-%04d", timeYYMM(), seq)

		_, err = tx.Exec(ctx, `
			INSERT INTO stock_transaction (txn_no, txn_type, item_id, from_location, qty, ref_doc_type, ref_doc_id, created_by)
			VALUES ($1,'BORROW_OUT',$2,$3,$4,'BORROW',$5,$6)`,
			txnNo, itemID, locationCode, ln.QtyBorrowed, id, claims.UserID)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `UPDATE borrow SET status='BORROWED', updated_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO borrow_status_log (borrow_id, old_status, new_status, changed_by)
		VALUES ($1,'APPROVED','BORROWED',$2)`, id, claims.UserID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true})
}

// Return godoc
// @Summary      คืน Items จาก Borrow (scan QR)
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                        true  "Borrow ID"
// @Param        body  body  models.BorrowReturnRequest  true  "Request body"
// @Success      200  {object}  fiber.Map
// @Router       /stock/borrow/{id}/return [post]
func (h *StockBorrowHandler) Return(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.BorrowReturnRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var status string
	if err := h.db.QueryRow(ctx, `SELECT status FROM borrow WHERE id=$1`, id).Scan(&status); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "borrow request not found")
	}
	if status != "BORROWED" && status != "PARTIALLY_RETURNED" {
		return fiber.NewError(fiber.StatusBadRequest, "borrow must be BORROWED or PARTIALLY_RETURNED")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, ln := range req.Lines {
		var itemID int64
		var locationCode string
		var itemType string
		var qtyBorrowed, qtyReturned float64
		if err := tx.QueryRow(ctx, `
			SELECT bl.stock_item_id, bl.location_code, si.item_type, bl.qty_borrowed, bl.qty_returned
			FROM borrow_line bl
			JOIN stock_item si ON si.id = bl.stock_item_id
			WHERE bl.id=$1 AND bl.borrow_id=$2`,
			ln.BorrowLineID, id).Scan(&itemID, &locationCode, &itemType, &qtyBorrowed, &qtyReturned); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("borrow_line_id %d not found", ln.BorrowLineID))
		}

		newQtyReturned := qtyReturned + ln.QtyReturned
		_, err = tx.Exec(ctx, `
			UPDATE borrow_line SET qty_returned=$1, condition_in=$2 WHERE id=$3`,
			newQtyReturned, ln.ConditionIn, ln.BorrowLineID)
		if err != nil {
			return err
		}

		if itemType == "RETURNABLE" {
			_, err = tx.Exec(ctx, `
				UPDATE stock_inventory SET qty_on_hand = qty_on_hand + $1, updated_at=NOW()
				WHERE item_id=$2 AND location_code=$3`, ln.QtyReturned, itemID, locationCode)
			if err != nil {
				return err
			}
			if err := rollupStockItemQty(ctx, tx, itemID); err != nil {
				return err
			}
		}

		var seq int64
		tx.QueryRow(ctx, "SELECT nextval('stock_txn_seq')").Scan(&seq)
		txnNo := fmt.Sprintf("TXN-%s-%04d", timeYYMM(), seq)

		_, err = tx.Exec(ctx, `
			INSERT INTO stock_transaction (txn_no, txn_type, item_id, to_location, qty, ref_doc_type, ref_doc_id, created_by)
			VALUES ($1,'BORROW_RETURN',$2,$3,$4,'BORROW',$5,$6)`,
			txnNo, itemID, locationCode, ln.QtyReturned, id, claims.UserID)
		if err != nil {
			return err
		}
		_ = qtyBorrowed
	}

	var totalLines, returnedLines int
	tx.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE qty_returned >= qty_borrowed)
		FROM borrow_line WHERE borrow_id=$1`, id).Scan(&totalLines, &returnedLines)

	newStatus := "PARTIALLY_RETURNED"
	if returnedLines >= totalLines {
		newStatus = "RETURNED"
	}

	_, err = tx.Exec(ctx, `
		UPDATE borrow SET status=$1, actual_return=CASE WHEN $1='RETURNED' THEN NOW() ELSE actual_return END, updated_at=NOW()
		WHERE id=$2`, newStatus, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO borrow_status_log (borrow_id, old_status, new_status, changed_by)
		VALUES ($1,$2,$3,$4)`, id, status, newStatus, claims.UserID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"status": newStatus}})
}

// ScanQR godoc
// @Summary      Scan QR Code เพื่อดูข้อมูล Item
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        item_code  path  string  true  "Item code or QR code"
// @Success      200  {object}  fiber.Map
// @Router       /stock/borrow/scan/{item_code} [get]
func (h *StockBorrowHandler) ScanQR(c *fiber.Ctx) error {
	itemCode := c.Params("item_code")
	ctx := context.Background()

	var item models.StockItem
	err := h.db.QueryRow(ctx, `
		SELECT si.id, si.mat_code, si.item_name, si.description,
		       si.category_id, sc.name,
		       si.item_type, si.tracking_type, si.unit, si.qty, si.unit_cost,
		       si.qr_code, si.is_active, si.created_at, si.updated_at
		FROM stock_item si
		LEFT JOIN stock_category sc ON sc.id = si.category_id
		WHERE si.mat_code = $1 OR si.qr_code = $1`, itemCode,
	).Scan(
		&item.ID, &item.MatCode, &item.ItemName, &item.Description,
		&item.CategoryID, &item.CategoryName,
		&item.ItemType, &item.TrackingType, &item.Unit, &item.Qty, &item.UnitCost,
		&item.QRCode, &item.IsActive, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "item not found")
	}

	rows, err := h.db.Query(ctx, `
		SELECT inv.location_code, COALESCE(l.location_name,'') AS location_name,
		       COALESCE(l.warehouse_code,'') AS warehouse_code,
		       inv.qty_on_hand, inv.qty_reserved,
		       (inv.qty_on_hand - inv.qty_reserved) AS qty_available
		FROM stock_inventory inv
		LEFT JOIN location l ON l.location_code = inv.location_code
		WHERE inv.item_id = $1 AND inv.qty_on_hand > 0
		ORDER BY inv.location_code`, item.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type InventorySummary struct {
		LocationCode  string  `json:"location_code"`
		LocationName  string  `json:"location_name"`
		WarehouseCode string  `json:"warehouse_code"`
		QtyOnHand     float64 `json:"qty_on_hand"`
		QtyReserved   float64 `json:"qty_reserved"`
		QtyAvailable  float64 `json:"qty_available"`
	}
	var inventory []InventorySummary
	for rows.Next() {
		var inv InventorySummary
		if err := rows.Scan(&inv.LocationCode, &inv.LocationName, &inv.WarehouseCode,
			&inv.QtyOnHand, &inv.QtyReserved, &inv.QtyAvailable); err != nil {
			return err
		}
		inventory = append(inventory, inv)
	}
	if inventory == nil {
		inventory = []InventorySummary{}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"item": item, "inventory": inventory},
	})
}

func init() {
	_ = strings.Contains // suppress unused import if any
}

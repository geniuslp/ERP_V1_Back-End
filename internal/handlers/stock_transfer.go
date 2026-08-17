package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

// StockTransferHandler handles ย้ายคลัง (Stock Transfer): move material between
// warehouses, or between a warehouse and a project, confirmed by destination
// staff. Uses stock_transfer / stock_transfer_line / stock_transfer_status_log.
type StockTransferHandler struct{ db *pgxpool.Pool }

func NewStockTransferHandler(db *pgxpool.Pool) *StockTransferHandler {
	return &StockTransferHandler{db: db}
}

var stockTransferTypes = map[string]bool{
	"WH_TO_WH":      true,
	"WH_TO_PROJECT": true,
	"PROJECT_TO_WH": true,
}

// Create godoc
// @Summary      สร้างใบย้ายคลัง (DRAFT)
// @Description  transfer_type WH_TO_WH requires from_warehouse_code+to_warehouse_code; WH_TO_PROJECT requires from_warehouse_code+to_project_code; PROJECT_TO_WH requires from_project_code+to_warehouse_code.
// @Tags         StockTransfer
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateStockTransferRequest  true  "Transfer data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      422   {object}  fiber.Map
// @Router       /stock-transfer [post]
func (h *StockTransferHandler) Create(c *fiber.Ctx) error {
	var req models.CreateStockTransferRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if !stockTransferTypes[req.TransferType] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid transfer_type")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line is required")
	}
	for i, ln := range req.Lines {
		if ln.MatCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: mat_code is required", i))
		}
		if ln.QtyRequested <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: qty_requested must be positive", i))
		}
	}

	var srcWarehouseForLookup string
	switch req.TransferType {
	case "WH_TO_WH":
		if strPtrEmpty(req.FromWarehouseCode) || strPtrEmpty(req.ToWarehouseCode) {
			return fiber.NewError(fiber.StatusBadRequest, "from_warehouse_code and to_warehouse_code are required")
		}
		if *req.FromWarehouseCode == *req.ToWarehouseCode {
			return fiber.NewError(fiber.StatusBadRequest, "from_warehouse_code and to_warehouse_code must differ")
		}
		srcWarehouseForLookup = *req.FromWarehouseCode
	case "WH_TO_PROJECT":
		if strPtrEmpty(req.FromWarehouseCode) || strPtrEmpty(req.ToProjectCode) {
			return fiber.NewError(fiber.StatusBadRequest, "from_warehouse_code and to_project_code are required")
		}
		srcWarehouseForLookup = *req.FromWarehouseCode
	case "PROJECT_TO_WH":
		if strPtrEmpty(req.FromProjectCode) || strPtrEmpty(req.ToWarehouseCode) {
			return fiber.NewError(fiber.StatusBadRequest, "from_project_code and to_warehouse_code are required")
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
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(id), 0)+1 FROM stock_transfer`).Scan(&seq); err != nil {
		return err
	}
	transferNo := fmt.Sprintf("TRF-%s-%06d", time.Now().Format("2006"), seq)

	transferDate := time.Now().Format("2006-01-02")
	if req.TransferDate != nil && *req.TransferDate != "" {
		transferDate = *req.TransferDate
	}

	var transferID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO stock_transfer
			(transfer_no, transfer_type, transfer_date, from_warehouse_code, from_project_code,
			 to_warehouse_code, to_project_code, requested_by, purpose, remarks, status, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'DRAFT',$11)
		RETURNING id`,
		transferNo, req.TransferType, transferDate, req.FromWarehouseCode, req.FromProjectCode,
		req.ToWarehouseCode, req.ToProjectCode, claims.UserID, req.Purpose, req.Remarks, claims.UserID,
	).Scan(&transferID)
	if err != nil {
		return err
	}

	for i, ln := range req.Lines {
		var itemID int64
		var unit string

		if req.TransferType == "PROJECT_TO_WH" {
			// item_id represents the destination stock_item — find or create it
			// at to_warehouse_code so the row exists (and is lockable) at confirm time.
			itemID, unit, err = findOrCreateStockItemAtWarehouse(ctx, tx, ln.MatCode, *req.ToWarehouseCode, claims.UserID)
			if err != nil {
				return fiber.NewError(fiber.StatusUnprocessableEntity,
					fmt.Sprintf("line[%d]: mat_code %s could not be resolved at warehouse %s: %v", i, ln.MatCode, *req.ToWarehouseCode, err))
			}
		} else {
			// item_id represents the source stock_item being deducted.
			err = tx.QueryRow(ctx, `SELECT id, unit FROM stock_item WHERE mat_code=$1 AND warehouse_code=$2`,
				ln.MatCode, srcWarehouseForLookup).Scan(&itemID, &unit)
			if err != nil {
				return fiber.NewError(fiber.StatusUnprocessableEntity,
					fmt.Sprintf("line[%d]: mat_code %s not found in warehouse %s", i, ln.MatCode, srcWarehouseForLookup))
			}
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO stock_transfer_line (transfer_id, line_no, item_id, mat_code, unit, qty_requested, remarks)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			transferID, i+1, itemID, ln.MatCode, unit, ln.QtyRequested, ln.Remarks,
		)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_transfer_status_log (transfer_id, from_status, to_status, changed_by)
		VALUES ($1, NULL, 'DRAFT', $2)`, transferID, claims.UserID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"id": transferID, "transfer_no": transferNo},
	})
}

func strPtrEmpty(s *string) bool {
	return s == nil || *s == ""
}

// List godoc
// @Summary      รายการใบย้ายคลัง
// @Tags         StockTransfer
// @Security     BearerAuth
// @Produce      json
// @Param        transfer_type       query  string  false  "Transfer type"
// @Param        status              query  string  false  "Status"
// @Param        from_warehouse_code query  string  false  "From warehouse code"
// @Param        to_warehouse_code   query  string  false  "To warehouse code"
// @Param        date_from           query  string  false  "Date from (YYYY-MM-DD)"
// @Param        date_to             query  string  false  "Date to (YYYY-MM-DD)"
// @Param        page                query  int     false  "Page"
// @Param        page_size           query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /stock-transfer [get]
func (h *StockTransferHandler) List(c *fiber.Ctx) error {
	var f models.StockTransferFilter
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
	if f.TransferType != "" {
		where = append(where, fmt.Sprintf("t.transfer_type = $%d", i))
		args = append(args, f.TransferType)
		i++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("t.status = $%d", i))
		args = append(args, f.Status)
		i++
	}
	if f.FromWarehouseCode != "" {
		where = append(where, fmt.Sprintf("t.from_warehouse_code = $%d", i))
		args = append(args, f.FromWarehouseCode)
		i++
	}
	if f.ToWarehouseCode != "" {
		where = append(where, fmt.Sprintf("t.to_warehouse_code = $%d", i))
		args = append(args, f.ToWarehouseCode)
		i++
	}
	if f.DateFrom != "" {
		where = append(where, fmt.Sprintf("t.transfer_date >= $%d", i))
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where = append(where, fmt.Sprintf("t.transfer_date <= $%d", i))
		args = append(args, f.DateTo)
		i++
	}
	whereClause := strings.Join(where, " AND ")

	joinClause := `FROM stock_transfer t`

	var total int64
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, joinClause, whereClause), args...).Scan(&total); err != nil {
		return err
	}

	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT t.id, t.transfer_no, t.transfer_type, TO_CHAR(t.transfer_date,'YYYY-MM-DD'),
		       t.from_warehouse_code, t.from_project_code, t.to_warehouse_code, t.to_project_code,
		       t.requested_by, t.purpose, t.remarks, t.status, t.checked_by, t.checked_at,
		       t.created_at, t.updated_at
		%s
		WHERE %s
		ORDER BY t.id DESC
		LIMIT $%d OFFSET $%d`, joinClause, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.StockTransfer
	for rows.Next() {
		var t models.StockTransfer
		if err := rows.Scan(&t.ID, &t.TransferNo, &t.TransferType, &t.TransferDate,
			&t.FromWarehouseCode, &t.FromProjectCode, &t.ToWarehouseCode, &t.ToProjectCode,
			&t.RequestedBy, &t.Purpose, &t.Remarks, &t.Status, &t.CheckedBy, &t.CheckedAt,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return err
		}
		items = append(items, t)
	}
	if items == nil {
		items = []models.StockTransfer{}
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
// @Summary      รายละเอียดใบย้ายคลัง
// @Tags         StockTransfer
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Transfer ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /stock-transfer/{id} [get]
func (h *StockTransferHandler) Get(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var t models.StockTransfer
	err = h.db.QueryRow(ctx, `
		SELECT t.id, t.transfer_no, t.transfer_type, TO_CHAR(t.transfer_date,'YYYY-MM-DD'),
		       t.from_warehouse_code, fw.warehouse_name, t.from_project_code, fp.project_name,
		       t.to_warehouse_code, tw.warehouse_name, t.to_project_code, tp.project_name,
		       t.requested_by, ru.full_name, t.purpose, t.remarks, t.status,
		       t.checked_by, cu.full_name, t.checked_at, t.created_at, t.updated_at
		FROM stock_transfer t
		LEFT JOIN warehouse fw ON fw.warehouse_code = t.from_warehouse_code
		LEFT JOIN project   fp ON fp.project_code   = t.from_project_code
		LEFT JOIN warehouse tw ON tw.warehouse_code = t.to_warehouse_code
		LEFT JOIN project   tp ON tp.project_code   = t.to_project_code
		LEFT JOIN users ru ON ru.id = t.requested_by
		LEFT JOIN users cu ON cu.id = t.checked_by
		WHERE t.id=$1`, id,
	).Scan(&t.ID, &t.TransferNo, &t.TransferType, &t.TransferDate,
		&t.FromWarehouseCode, &t.FromWarehouseName, &t.FromProjectCode, &t.FromProjectName,
		&t.ToWarehouseCode, &t.ToWarehouseName, &t.ToProjectCode, &t.ToProjectName,
		&t.RequestedBy, &t.RequestedByName, &t.Purpose, &t.Remarks, &t.Status,
		&t.CheckedBy, &t.CheckedByName, &t.CheckedAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "transfer not found")
	}

	rows, err := h.db.Query(ctx, `
		SELECT tl.id, tl.transfer_id, tl.line_no, tl.item_id, tl.mat_code, si.item_name, tl.unit,
		       tl.qty_requested, tl.qty_confirmed, tl.remarks
		FROM stock_transfer_line tl
		LEFT JOIN stock_item si ON si.id = tl.item_id
		WHERE tl.transfer_id=$1
		ORDER BY tl.line_no`, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lines []models.StockTransferLine
	for rows.Next() {
		var ln models.StockTransferLine
		if err := rows.Scan(&ln.ID, &ln.TransferID, &ln.LineNo, &ln.ItemID, &ln.MatCode, &ln.ItemName, &ln.Unit,
			&ln.QtyRequested, &ln.QtyConfirmed, &ln.Remarks); err != nil {
			return err
		}
		lines = append(lines, ln)
	}
	t.Lines = lines

	return c.JSON(fiber.Map{"success": true, "data": t})
}

// Cancel godoc
// @Summary      ยกเลิกใบย้ายคลัง (DRAFT เท่านั้น)
// @Tags         StockTransfer
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Transfer ID"
// @Success      200  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Router       /stock-transfer/{id}/cancel [post]
func (h *StockTransferHandler) Cancel(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var status string
	if err := h.db.QueryRow(ctx, `SELECT status FROM stock_transfer WHERE id=$1`, id).Scan(&status); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "transfer not found")
	}
	if status != "DRAFT" {
		return fiber.NewError(fiber.StatusConflict, "transfer must be DRAFT to cancel")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE stock_transfer SET status='CANCELLED', updated_at=NOW(), updated_by=$1 WHERE id=$2`,
		claims.UserID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO stock_transfer_status_log (transfer_id, from_status, to_status, changed_by)
		VALUES ($1,'DRAFT','CANCELLED',$2)`, id, claims.UserID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "transfer cancelled"})
}

// Confirm godoc
// @Summary      ยืนยันรับของ / ยืนยันการย้ายคลัง (DRAFT → CONFIRMED)
// @Description  One DB transaction. Locks each line's source stock_item row (SELECT ... FOR UPDATE),
// @Description  verifies sufficient qty, moves stock according to transfer_type (WH_TO_WH: deduct
// @Description  source warehouse item, credit destination warehouse item; WH_TO_PROJECT: deduct
// @Description  source warehouse item, credit project_stock; PROJECT_TO_WH: deduct project_stock,
// @Description  credit destination warehouse item), and records a stock_transaction per line
// @Description  (ref_doc_type=STOCK_TRANSFER) for the shared movement history.
// @Tags         StockTransfer
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Transfer ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Router       /stock-transfer/{id}/confirm [post]
func (h *StockTransferHandler) Confirm(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var status, transferType string
	var fromWH, fromProj, toWH, toProj *string
	if err := h.db.QueryRow(ctx, `
		SELECT status, transfer_type, from_warehouse_code, from_project_code, to_warehouse_code, to_project_code
		FROM stock_transfer WHERE id=$1`, id,
	).Scan(&status, &transferType, &fromWH, &fromProj, &toWH, &toProj); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "transfer not found")
	}
	if status != "DRAFT" {
		return fiber.NewError(fiber.StatusConflict, fmt.Sprintf("transfer must be DRAFT (current: %s)", status))
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, item_id, mat_code, unit, qty_requested
		FROM stock_transfer_line WHERE transfer_id=$1 ORDER BY line_no`, id)
	if err != nil {
		return err
	}
	type line struct {
		ID           int64
		ItemID       int64
		MatCode      string
		Unit         *string
		QtyRequested float64
	}
	var lines []line
	for rows.Next() {
		var ln line
		if err := rows.Scan(&ln.ID, &ln.ItemID, &ln.MatCode, &ln.Unit, &ln.QtyRequested); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, ln)
	}
	rows.Close()

	for _, ln := range lines {
		var txnType, fromLoc, toLoc string

		switch transferType {
		case "WH_TO_WH":
			qtyBefore, qtyAfter, err := deductStockItem(ctx, tx, ln.ItemID, ln.QtyRequested, ln.MatCode)
			if err != nil {
				return err
			}
			if err := addToWarehouseStock(ctx, tx, ln.MatCode, *toWH, ln.QtyRequested, claims.UserID); err != nil {
				return err
			}
			txnType, fromLoc, toLoc = "TRANSFER", *fromWH, *toWH
			if err := recordTransferTxn(ctx, tx, txnType, ln.ItemID, fromLoc, toLoc, ln.QtyRequested, qtyBefore, qtyAfter, id, claims.UserID); err != nil {
				return err
			}

		case "WH_TO_PROJECT":
			qtyBefore, qtyAfter, err := deductStockItem(ctx, tx, ln.ItemID, ln.QtyRequested, ln.MatCode)
			if err != nil {
				return err
			}
			if err := addToProjectStock(ctx, tx, *toProj, ln.MatCode, ln.Unit, ln.QtyRequested); err != nil {
				return err
			}
			txnType, fromLoc, toLoc = "OUT", *fromWH, *toProj
			if err := recordTransferTxn(ctx, tx, txnType, ln.ItemID, fromLoc, toLoc, ln.QtyRequested, qtyBefore, qtyAfter, id, claims.UserID); err != nil {
				return err
			}

		case "PROJECT_TO_WH":
			if err := deductProjectStock(ctx, tx, *fromProj, ln.MatCode, ln.QtyRequested); err != nil {
				return err
			}
			qtyBefore, qtyAfter, err := creditStockItem(ctx, tx, ln.ItemID, ln.QtyRequested)
			if err != nil {
				return err
			}
			txnType, fromLoc, toLoc = "IN", *fromProj, *toWH
			if err := recordTransferTxn(ctx, tx, txnType, ln.ItemID, fromLoc, toLoc, ln.QtyRequested, qtyBefore, qtyAfter, id, claims.UserID); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE stock_transfer_line SET qty_confirmed=$1 WHERE id=$2`, ln.QtyRequested, ln.ID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE stock_transfer SET status='CONFIRMED', checked_by=$1, checked_at=NOW(), updated_at=NOW(), updated_by=$1
		WHERE id=$2`, claims.UserID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO stock_transfer_status_log (transfer_id, from_status, to_status, changed_by)
		VALUES ($1,'DRAFT','CONFIRMED',$2)`, id, claims.UserID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "transfer confirmed"})
}

// ── shared stock-movement helpers (used by StockTransferHandler and RequisitionHandler) ──

// deductStockItem locks and decrements a stock_item row, returning qty before/after.
// Rejects with 409 if the requested qty exceeds what's on hand.
func deductStockItem(ctx context.Context, tx pgx.Tx, itemID int64, qty float64, matCode string) (before, after float64, err error) {
	if err := tx.QueryRow(ctx, `SELECT qty FROM stock_item WHERE id=$1 FOR UPDATE`, itemID).Scan(&before); err != nil {
		return 0, 0, fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("stock item for %s not found", matCode))
	}
	if before < qty {
		return 0, 0, fiber.NewError(fiber.StatusConflict, fmt.Sprintf("insufficient stock for %s (have %.4f, need %.4f)", matCode, before, qty))
	}
	after = before - qty
	if _, err := tx.Exec(ctx, `UPDATE stock_item SET qty=$1, updated_at=NOW() WHERE id=$2`, after, itemID); err != nil {
		return 0, 0, err
	}
	return before, after, nil
}

// creditStockItem locks and increments a stock_item row, returning qty before/after.
func creditStockItem(ctx context.Context, tx pgx.Tx, itemID int64, qty float64) (before, after float64, err error) {
	if err := tx.QueryRow(ctx, `SELECT qty FROM stock_item WHERE id=$1 FOR UPDATE`, itemID).Scan(&before); err != nil {
		return 0, 0, err
	}
	after = before + qty
	if _, err := tx.Exec(ctx, `UPDATE stock_item SET qty=$1, updated_at=NOW() WHERE id=$2`, after, itemID); err != nil {
		return 0, 0, err
	}
	return before, after, nil
}

// findOrCreateStockItemAtWarehouse finds the stock_item row for mat_code at
// whCode, or clones one from any existing row with that mat_code (qty=0) if
// none exists yet. Requires stock_item to have a UNIQUE(mat_code, warehouse_code)
// constraint (not a bare UNIQUE(mat_code)) so more than one warehouse can carry
// the same mat_code — see the ALTER in the schema SQL provided alongside this handler.
func findOrCreateStockItemAtWarehouse(ctx context.Context, tx pgx.Tx, matCode, whCode string, userID int64) (itemID int64, unit string, err error) {
	err = tx.QueryRow(ctx, `SELECT id, unit FROM stock_item WHERE mat_code=$1 AND warehouse_code=$2 FOR UPDATE`,
		matCode, whCode).Scan(&itemID, &unit)
	if err == nil {
		return itemID, unit, nil
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO stock_item (mat_code, item_name, description, category_id, item_type, tracking_type,
		                         unit, qty, unit_cost, warehouse_code, location_code, created_by)
		SELECT mat_code, item_name, description, category_id, item_type, tracking_type,
		       unit, 0, unit_cost, $2, location_code, $3
		FROM stock_item WHERE mat_code=$1 LIMIT 1
		RETURNING id, unit`, matCode, whCode, userID,
	).Scan(&itemID, &unit)
	if err != nil {
		return 0, "", err
	}
	return itemID, unit, nil
}

// addToWarehouseStock finds-or-creates the destination stock_item and increments its qty.
func addToWarehouseStock(ctx context.Context, tx pgx.Tx, matCode, whCode string, qty float64, userID int64) error {
	itemID, _, err := findOrCreateStockItemAtWarehouse(ctx, tx, matCode, whCode, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE stock_item SET qty = qty + $1, updated_at=NOW() WHERE id=$2`, qty, itemID)
	return err
}

// addToProjectStock upserts project_stock, adding qty.
func addToProjectStock(ctx context.Context, tx pgx.Tx, projectCode, matCode string, unit *string, qty float64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO project_stock (project_code, mat_code, unit, qty_on_hand)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (project_code, mat_code)
		DO UPDATE SET qty_on_hand = project_stock.qty_on_hand + $4, updated_at=NOW()`,
		projectCode, matCode, unit, qty)
	return err
}

// deductProjectStock locks the project_stock row and decrements it. Rejects
// with 409 if the project doesn't have enough on hand (can't return/move more
// than it actually has).
func deductProjectStock(ctx context.Context, tx pgx.Tx, projectCode, matCode string, qty float64) error {
	var qtyOnHand float64
	err := tx.QueryRow(ctx, `
		SELECT qty_on_hand FROM project_stock WHERE project_code=$1 AND mat_code=$2 FOR UPDATE`,
		projectCode, matCode).Scan(&qtyOnHand)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("no project stock for %s at %s", matCode, projectCode))
	}
	if qtyOnHand < qty {
		return fiber.NewError(fiber.StatusConflict,
			fmt.Sprintf("insufficient project stock for %s at %s (have %.4f, need %.4f)", matCode, projectCode, qtyOnHand, qty))
	}
	_, err = tx.Exec(ctx, `
		UPDATE project_stock SET qty_on_hand = qty_on_hand - $1, updated_at=NOW()
		WHERE project_code=$2 AND mat_code=$3`, qty, projectCode, matCode)
	return err
}

func recordTransferTxn(ctx context.Context, tx pgx.Tx, txnType string, itemID int64, fromLoc, toLoc string, qty, qtyBefore, qtyAfter float64, transferID int64, userID int64) error {
	txnNo, err := generateTxnNo(ctx, tx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO stock_transaction
			(txn_no, txn_type, item_id, from_location, to_location, qty, qty_before, qty_after,
			 ref_doc_type, ref_doc_id, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'STOCK_TRANSFER',$9,$10)`,
		txnNo, txnType, itemID, fromLoc, toLoc, qty, qtyBefore, qtyAfter, transferID, userID)
	return err
}

// History godoc
// @Summary      ประวัติการเคลื่อนไหว (Requisition + Stock Transfer รวมกัน)
// @Description  Reads stock_transaction WHERE ref_doc_type IN ('STOCK_TRANSFER','REQUISITION'),
// @Description  joined to stock_item for mat_code/name and to warehouse/project for from/to display
// @Description  names (a location can be either, so both are LEFT JOINed and whichever matches is used).
// @Tags         StockTransfer
// @Security     BearerAuth
// @Produce      json
// @Param        ref_doc_type  query  string  false  "Filter: STOCK_TRANSFER or REQUISITION"
// @Param        mat_code      query  string  false  "Material code"
// @Param        date_from     query  string  false  "Date from (YYYY-MM-DD)"
// @Param        date_to       query  string  false  "Date to (YYYY-MM-DD)"
// @Param        page          query  int     false  "Page"
// @Param        page_size     query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /stock-transfer/history [get]
func (h *StockTransferHandler) History(c *fiber.Ctx) error {
	refDocType := c.Query("ref_doc_type")
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

	where := []string{"st.ref_doc_type IN ('STOCK_TRANSFER','REQUISITION')"}
	args := []any{}
	i := 1
	if refDocType != "" {
		where = append(where, fmt.Sprintf("st.ref_doc_type = $%d", i))
		args = append(args, refDocType)
		i++
	}
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
		JOIN stock_item si ON si.id = st.item_id
		LEFT JOIN warehouse fw ON fw.warehouse_code = st.from_location
		LEFT JOIN project   fp ON fp.project_code   = st.from_location
		LEFT JOIN warehouse tw ON tw.warehouse_code = st.to_location
		LEFT JOIN project   tp ON tp.project_code   = st.to_location
		LEFT JOIN users u ON u.id = st.created_by`

	var total int64
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, joinClause, whereClause), args...).Scan(&total); err != nil {
		return err
	}

	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT st.id, st.txn_no, st.txn_type, st.item_id, si.mat_code, si.item_name,
		       st.from_location, COALESCE(fw.warehouse_name, fp.project_name) AS from_name,
		       st.to_location, COALESCE(tw.warehouse_name, tp.project_name) AS to_name,
		       st.qty, st.qty_before, st.qty_after, st.ref_doc_type, st.ref_doc_id, st.remarks,
		       TO_CHAR(st.txn_date,'YYYY-MM-DD'), COALESCE(u.full_name,''), st.created_at
		%s
		WHERE %s
		ORDER BY st.created_at DESC
		LIMIT $%d OFFSET $%d`, joinClause, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type historyRow struct {
		ID            int64     `json:"id"`
		TxnNo         string    `json:"txn_no"`
		TxnType       string    `json:"txn_type"`
		ItemID        int64     `json:"item_id"`
		MatCode       string    `json:"mat_code"`
		ItemName      string    `json:"item_name"`
		FromLocation  *string   `json:"from_location"`
		FromName      *string   `json:"from_name"`
		ToLocation    *string   `json:"to_location"`
		ToName        *string   `json:"to_name"`
		Qty           float64   `json:"qty"`
		QtyBefore     *float64  `json:"qty_before"`
		QtyAfter      *float64  `json:"qty_after"`
		RefDocType    *string   `json:"ref_doc_type"`
		RefDocID      *int64    `json:"ref_doc_id"`
		Remarks       *string   `json:"remarks"`
		TxnDate       string    `json:"txn_date"`
		CreatedByName string    `json:"created_by_name"`
		CreatedAt     time.Time `json:"created_at"`
	}

	var items []historyRow
	for rows.Next() {
		var r historyRow
		if err := rows.Scan(&r.ID, &r.TxnNo, &r.TxnType, &r.ItemID, &r.MatCode, &r.ItemName,
			&r.FromLocation, &r.FromName, &r.ToLocation, &r.ToName,
			&r.Qty, &r.QtyBefore, &r.QtyAfter, &r.RefDocType, &r.RefDocID, &r.Remarks,
			&r.TxnDate, &r.CreatedByName, &r.CreatedAt); err != nil {
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

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

type InventoryHandler struct {
	db *pgxpool.Pool
}

func NewInventoryHandler(db *pgxpool.Pool) *InventoryHandler {
	return &InventoryHandler{db: db}
}

// ListInventory godoc
// @Summary      List inventory balances
// @Description  Returns stock levels per material per warehouse with status
// @Tags         Inventory
// @Security     BearerAuth
// @Produce      json
// @Param        warehouse  query  string  false  "warehouse_code filter"
// @Param        mat_code   query  string  false  "material code filter"
// @Param        status     query  string  false  "stock_status filter (OK|LOW|CRITICAL)"
// @Param        page       query  int     false  "page"       default(1)
// @Param        page_size  query  int     false  "page size"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /inventory [get]
func (h *InventoryHandler) ListInventory(c *fiber.Ctx) error {
	wh := c.Query("warehouse")
	mat := c.Query("mat_code")
	status := c.Query("status")
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	args := []interface{}{}
	idx := 1
	where := "1=1"

	if wh != "" {
		where += fmt.Sprintf(" AND warehouse_code=$%d", idx)
		args = append(args, wh)
		idx++
	}
	if mat != "" {
		where += fmt.Sprintf(" AND mat_code=$%d", idx)
		args = append(args, mat)
		idx++
	}
	if status != "" {
		where += fmt.Sprintf(" AND stock_status=$%d", idx)
		args = append(args, status)
		idx++
	}

	var total int64
	h.db.QueryRow(context.Background(), fmt.Sprintf(`SELECT COUNT(*) FROM v_inventory_full WHERE %s`, where), args...).Scan(&total)

	args = append(args, size, offset)
	rows, err := h.db.Query(context.Background(), fmt.Sprintf(`
		SELECT inventory_id, mat_code, mat_name_th, group_name, subgroup_name, unit_name,
		       warehouse_code, warehouse_name, zone_code, zone_name,
		       qty_on_hand, qty_reserved, qty_on_order, qty_available,
		       reorder_point, min_stock, max_stock, stock_status, last_counted_at, updated_at
		FROM v_inventory_full WHERE %s
		ORDER BY mat_code, warehouse_code
		LIMIT $%d OFFSET $%d`, where, idx, idx+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type InventoryRow struct {
		models.Inventory
		MatNameTH     string  `json:"mat_name_th"`
		GroupName     string  `json:"group_name"`
		SubgroupName  string  `json:"subgroup_name"`
		UnitName      string  `json:"unit_name"`
		WarehouseName string  `json:"warehouse_name"`
		ZoneCode      *string `json:"zone_code,omitempty"`
		ZoneName      *string `json:"zone_name,omitempty"`
	}

	var items []InventoryRow
	for rows.Next() {
		var r InventoryRow
		rows.Scan(
			&r.InventoryID, &r.MatCode, &r.MatNameTH, &r.GroupName, &r.SubgroupName, &r.UnitName,
			&r.WarehouseCode, &r.WarehouseName, &r.ZoneCode, &r.ZoneName,
			&r.QtyOnHand, &r.QtyReserved, &r.QtyOnOrder, &r.QtyAvailable,
			&r.ReorderPoint, &r.MinStock, &r.MaxStock, &r.StockStatus, nil, nil,
		)
		items = append(items, r)
	}

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data":    models.PaginatedResponse{Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages},
	})
}

// CreateTransaction godoc
// @Summary      Create inventory transaction
// @Description  Record a stock movement (issue, return, transfer, adjustment, borrow)
// @Tags         Inventory
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateTransactionRequest  true  "Transaction"
// @Success      201   {object}  models.InventoryTransaction
// @Failure      400   {object}  fiber.Map
// @Router       /inventory/transactions [post]
func (h *InventoryHandler) CreateTransaction(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	var req models.CreateTransactionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	txnDate := time.Now()
	if req.TxnDate != nil {
		if d, err := time.Parse("2006-01-02", *req.TxnDate); err == nil {
			txnDate = d
		}
	}

	// Generate txn_no
	var seq int64
	h.db.QueryRow(context.Background(), `SELECT COALESCE(MAX(txn_id),0)+1 FROM inventory_transaction`).Scan(&seq)
	txnNo := fmt.Sprintf("TXN-%s-%06d", txnDate.Format("2006"), seq)

	var txnID int64
	err := h.db.QueryRow(context.Background(), `
		INSERT INTO inventory_transaction
		  (txn_no, txn_type, mat_code, from_warehouse, to_warehouse, from_zone_id, to_zone_id,
		   qty, location_code, reason, txn_date, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING txn_id`,
		txnNo, req.TxnType, req.MatCode, req.FromWarehouse, req.ToWarehouse,
		req.FromZoneID, req.ToZoneID, req.Qty, req.LocationCode, req.Reason, txnDate, claims.UserID,
	).Scan(&txnID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create transaction: "+err.Error())
	}

	// Update inventory balances
	if err := h.updateInventoryBalance(req, txnDate); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "transaction created but balance update failed: "+err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"txn_id": txnID, "txn_no": txnNo},
	})
}

func (h *InventoryHandler) updateInventoryBalance(req models.CreateTransactionRequest, _ time.Time) error {
	ctx := context.Background()

	upsertInventory := func(wh, mat string, delta float64) error {
		_, err := h.db.Exec(ctx, `
			INSERT INTO inventory (mat_code, warehouse_code, qty_on_hand)
			VALUES ($1,$2,$3)
			ON CONFLICT (mat_code, warehouse_code)
			DO UPDATE SET qty_on_hand = inventory.qty_on_hand + $3, updated_at = NOW()`,
			mat, wh, delta)
		return err
	}

	switch req.TxnType {
	case "ISSUE", "BORROW_OUT":
		if req.FromWarehouse != nil {
			return upsertInventory(*req.FromWarehouse, req.MatCode, -req.Qty)
		}
	case "RETURN", "BORROW_RETURN", "GRN_IN", "ADJUST_PLUS":
		if req.ToWarehouse != nil {
			return upsertInventory(*req.ToWarehouse, req.MatCode, req.Qty)
		}
	case "ADJUST_MINUS":
		if req.FromWarehouse != nil {
			return upsertInventory(*req.FromWarehouse, req.MatCode, -req.Qty)
		}
	case "TRANSFER_OUT":
		if req.FromWarehouse != nil {
			upsertInventory(*req.FromWarehouse, req.MatCode, -req.Qty)
		}
		if req.ToWarehouse != nil {
			upsertInventory(*req.ToWarehouse, req.MatCode, req.Qty)
		}
	case "TRANSFER_IN":
		if req.ToWarehouse != nil {
			return upsertInventory(*req.ToWarehouse, req.MatCode, req.Qty)
		}
	}
	return nil
}

// ListTransactions godoc
// @Summary      List inventory transactions
// @Tags         Inventory
// @Security     BearerAuth
// @Produce      json
// @Param        mat_code    query  string  false  "material code"
// @Param        warehouse   query  string  false  "warehouse code"
// @Param        txn_type    query  string  false  "transaction type"
// @Param        date_from   query  string  false  "date from (YYYY-MM-DD)"
// @Param        date_to     query  string  false  "date to (YYYY-MM-DD)"
// @Param        page        query  int     false  "page"  default(1)
// @Param        page_size   query  int     false  "page size"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /inventory/transactions [get]
func (h *InventoryHandler) ListTransactions(c *fiber.Ctx) error {
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	var total int64
	h.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM inventory_transaction`).Scan(&total)

	rows, err := h.db.Query(context.Background(), `
		SELECT txn_id, txn_no, txn_type, mat_code, from_warehouse, to_warehouse,
		       qty, ref_doc_type, ref_doc_no, location_code, reason, txn_date, created_by, created_at
		FROM inventory_transaction
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, size, offset)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.InventoryTransaction
	for rows.Next() {
		var t models.InventoryTransaction
		rows.Scan(&t.TxnID, &t.TxnNo, &t.TxnType, &t.MatCode, &t.FromWarehouse, &t.ToWarehouse,
			&t.Qty, &t.RefDocType, &t.RefDocNo, &t.LocationCode, &t.Reason, &t.TxnDate, &t.CreatedBy, &t.CreatedAt)
		items = append(items, t)
	}

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data":    models.PaginatedResponse{Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages},
	})
}

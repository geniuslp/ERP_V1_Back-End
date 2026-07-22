package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

type StockInventoryHandler struct{ db *pgxpool.Pool }

func NewStockInventoryHandler(db *pgxpool.Pool) *StockInventoryHandler {
	return &StockInventoryHandler{db: db}
}

// List godoc
// @Summary      รายการ Stock Inventory
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        search         query  string  false  "Search"
// @Param        warehouse_code query  string  false  "Warehouse code"
// @Param        location_code  query  string  false  "Location code"
// @Param        low_stock_only query  bool    false  "Low stock only"
// @Param        page           query  int     false  "Page"
// @Param        page_size      query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /stock/inventory [get]
func (h *StockInventoryHandler) List(c *fiber.Ctx) error {
	var f models.StockInventoryFilter
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

	if f.Search != "" {
		where = append(where, fmt.Sprintf("(si.mat_code ILIKE $%d OR si.item_name ILIKE $%d)", i, i+1))
		like := "%" + f.Search + "%"
		args = append(args, like, like)
		i += 2
	}
	if f.WarehouseCode != "" {
		where = append(where, fmt.Sprintf("l.warehouse_code = $%d", i))
		args = append(args, f.WarehouseCode)
		i++
	}
	if f.LocationCode != "" {
		where = append(where, fmt.Sprintf("inv.location_code = $%d", i))
		args = append(args, f.LocationCode)
		i++
	}
	if f.LowStockOnly {
		where = append(where, "inv.qty_on_hand <= si.qty")
	}

	whereClause := strings.Join(where, " AND ")

	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*)
		FROM stock_inventory inv
		JOIN stock_item si ON si.id = inv.item_id
		JOIN location l ON l.location_code = inv.location_code
		WHERE %s`, whereClause), countArgs...).Scan(&total); err != nil {
		return err
	}

	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT inv.id, inv.item_id,
		       si.mat_code, si.item_name, si.unit, si.qty,
		       inv.location_code, l.location_name, l.warehouse_code,
		       inv.qty_on_hand, inv.qty_reserved,
		       (inv.qty_on_hand - inv.qty_reserved) AS qty_available
		FROM stock_inventory inv
		JOIN stock_item si ON si.id = inv.item_id
		JOIN location l ON l.location_code = inv.location_code
		WHERE %s
		ORDER BY si.mat_code, inv.location_code
		LIMIT $%d OFFSET $%d`, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.StockInventory
	for rows.Next() {
		var it models.StockInventory
		if err := rows.Scan(
			&it.ID, &it.ItemID,
			&it.MatCode, &it.ItemName, &it.Unit, &it.MinQty,
			&it.LocationCode, &it.LocationName, &it.WarehouseCode,
			&it.QtyOnHand, &it.QtyReserved, &it.QtyAvailable,
		); err != nil {
			return err
		}
		items = append(items, it)
	}
	if items == nil {
		items = []models.StockInventory{}
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

// BatchLookup godoc
// @Summary      Get total stock quantity for multiple material codes
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.BatchStockLookupRequest  true  "list of codes"
// @Success      200  {object}  fiber.Map
// @Router       /stock/inventory/batch-lookup [post]
func (h *StockInventoryHandler) BatchLookup(c *fiber.Ctx) error {
	var req models.BatchStockLookupRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(req.Codes) == 0 {
		return c.JSON(fiber.Map{"success": true, "data": []any{}})
	}

	ctx := context.Background()
	rows, err := h.db.Query(ctx, `
		SELECT mat_code, item_name, qty, unit_cost, warehouse_code, location_code
		FROM stock_item
		WHERE mat_code = ANY($1)`,
		req.Codes,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type stockLookupResult struct {
		MatCode       string  `json:"mat_code"`
		ItemName      string  `json:"item_name"`
		Qty           float64 `json:"qty"`
		UnitCost      float64 `json:"unit_cost"`
		WarehouseCode string  `json:"warehouse_code"`
		LocationCode  string  `json:"location_code"`
		Found         bool    `json:"found"`
	}

	found := map[string]stockLookupResult{}
	for rows.Next() {
		var r stockLookupResult
		if err := rows.Scan(&r.MatCode, &r.ItemName, &r.Qty, &r.UnitCost, &r.WarehouseCode, &r.LocationCode); err != nil {
			return err
		}
		r.Found = true
		found[r.MatCode] = r
	}

	results := make([]stockLookupResult, 0, len(req.Codes))
	for _, code := range req.Codes {
		if r, ok := found[code]; ok {
			results = append(results, r)
		} else {
			results = append(results, stockLookupResult{MatCode: code, Found: false})
		}
	}

	return c.JSON(fiber.Map{"success": true, "data": results})
}

// AdjustStock godoc
// @Summary      ปรับ Stock (ADJUST_PLUS / ADJUST_MINUS)
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.AdjustStockRequest  true  "Request body"
// @Success      200  {object}  fiber.Map
// @Router       /stock/inventory/adjust [post]
func (h *StockInventoryHandler) AdjustStock(c *fiber.Ctx) error {
	var req models.AdjustStockRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.TxnType != "ADJUST_PLUS" && req.TxnType != "ADJUST_MINUS" {
		return fiber.NewError(fiber.StatusBadRequest, "txn_type must be ADJUST_PLUS or ADJUST_MINUS")
	}
	if req.Qty <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "qty must be positive")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if req.TxnType == "ADJUST_MINUS" {
		var qtyOnHand float64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(qty_on_hand, 0)
			FROM stock_inventory
			WHERE item_id=$1 AND location_code=$2`, req.ItemID, req.LocationCode).Scan(&qtyOnHand); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "inventory record not found")
		}
		if qtyOnHand-req.Qty < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "insufficient stock")
		}
	}

	delta := req.Qty
	if req.TxnType == "ADJUST_MINUS" {
		delta = -req.Qty
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_inventory (item_id, location_code, qty_on_hand, qty_reserved)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (item_id, location_code) DO UPDATE
		SET qty_on_hand = stock_inventory.qty_on_hand + $3, updated_at = NOW()`,
		req.ItemID, req.LocationCode, delta)
	if err != nil {
		return err
	}

	var seq int64
	tx.QueryRow(ctx, "SELECT nextval('stock_txn_seq')").Scan(&seq)
	txnNo := fmt.Sprintf("TXN-%s-%04d", timeYYMM(), seq)

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_transaction (txn_no, txn_type, item_id, to_location, qty, remarks, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		txnNo, req.TxnType, req.ItemID, req.LocationCode, req.Qty, req.Remarks, claims.UserID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"txn_no": txnNo}})
}

// Transfer godoc
// @Summary      โอน Stock ระหว่าง Location
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.TransferStockRequest  true  "Request body"
// @Success      200  {object}  fiber.Map
// @Router       /stock/inventory/transfer [post]
func (h *StockInventoryHandler) Transfer(c *fiber.Ctx) error {
	var req models.TransferStockRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.Qty <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "qty must be positive")
	}
	if req.FromLocation == req.ToLocation {
		return fiber.NewError(fiber.StatusBadRequest, "from_location and to_location must be different")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var qtyAvailable float64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(qty_on_hand - qty_reserved, 0)
		FROM stock_inventory
		WHERE item_id=$1 AND location_code=$2`, req.ItemID, req.FromLocation).Scan(&qtyAvailable); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "source inventory not found")
	}
	if qtyAvailable < req.Qty {
		return fiber.NewError(fiber.StatusBadRequest, "insufficient available stock")
	}

	_, err = tx.Exec(ctx, `
		UPDATE stock_inventory SET qty_on_hand = qty_on_hand - $1, updated_at=NOW()
		WHERE item_id=$2 AND location_code=$3`, req.Qty, req.ItemID, req.FromLocation)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_inventory (item_id, location_code, qty_on_hand, qty_reserved)
		VALUES ($1,$2,$3,0)
		ON CONFLICT (item_id, location_code) DO UPDATE
		SET qty_on_hand = stock_inventory.qty_on_hand + $3, updated_at=NOW()`,
		req.ItemID, req.ToLocation, req.Qty)
	if err != nil {
		return err
	}

	var seq int64
	tx.QueryRow(ctx, "SELECT nextval('stock_txn_seq')").Scan(&seq)
	txnNo := fmt.Sprintf("TXN-%s-%04d", timeYYMM(), seq)

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_transaction (txn_no, txn_type, item_id, from_location, to_location, qty, remarks, created_by)
		VALUES ($1,'TRANSFER',$2,$3,$4,$5,$6,$7)`,
		txnNo, req.ItemID, req.FromLocation, req.ToLocation, req.Qty, req.Remarks, claims.UserID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"txn_no": txnNo}})
}

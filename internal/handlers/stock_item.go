package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"

	"erp-api/internal/models"
)

type StockItemHandler struct{ db *pgxpool.Pool }

func NewStockItemHandler(db *pgxpool.Pool) *StockItemHandler {
	return &StockItemHandler{db: db}
}

func (h *StockItemHandler) generateItemCode(ctx context.Context) (string, error) {
	var seq int64
	if err := h.db.QueryRow(ctx, "SELECT nextval('stock_item_seq')").Scan(&seq); err != nil {
		return "", err
	}
	return fmt.Sprintf("STK-%s-%04d", time.Now().Format("0601"), seq), nil
}

// ListCategories godoc
// @Summary      รายการ Stock Category
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /stock/categories [get]
func (h *StockItemHandler) ListCategories(c *fiber.Ctx) error {
	ctx := context.Background()
	rows, err := h.db.Query(ctx, `
		SELECT id, code, name, description, is_active, created_at
		FROM stock_category
		WHERE is_active = true
		ORDER BY name`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var cats []models.StockCategory
	for rows.Next() {
		var cat models.StockCategory
		if err := rows.Scan(&cat.ID, &cat.Code, &cat.Name, &cat.Description, &cat.IsActive, &cat.CreatedAt); err != nil {
			return err
		}
		cats = append(cats, cat)
	}
	if cats == nil {
		cats = []models.StockCategory{}
	}
	return c.JSON(fiber.Map{"success": true, "data": cats})
}

// List godoc
// @Summary      รายการ Stock Item
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        search    query  string  false  "Search"
// @Param        item_type query  string  false  "RETURNABLE|CONSUMABLE"
// @Param        is_active query  string  false  "true|false"
// @Param        page      query  int     false  "Page"
// @Param        page_size query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /stock/items [get]
func (h *StockItemHandler) List(c *fiber.Ctx) error {
	var f models.StockItemListFilter
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
	if f.ItemType != "" {
		where = append(where, fmt.Sprintf("si.item_type = $%d", i))
		args = append(args, f.ItemType)
		i++
	}
	if f.IsActive != "" {
		active := f.IsActive == "true"
		where = append(where, fmt.Sprintf("si.is_active = $%d", i))
		args = append(args, active)
		i++
	}

	whereClause := strings.Join(where, " AND ")

	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM stock_item si
		LEFT JOIN stock_category sc ON sc.id = si.category_id
		WHERE %s`, whereClause), countArgs...).Scan(&total)
	if err != nil {
		return err
	}

	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT si.id, si.mat_code, si.item_name, si.description,
		       si.category_id, sc.name AS category_name,
		       si.item_type, si.tracking_type, si.unit, si.qty, si.unit_cost,
		       si.qr_code,
		       si.is_active, si.created_at, si.updated_at
		FROM stock_item si
		LEFT JOIN stock_category sc ON sc.id = si.category_id
		WHERE %s
		ORDER BY si.mat_code
		LIMIT $%d OFFSET $%d`, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.StockItem
	for rows.Next() {
		var it models.StockItem
		if err := rows.Scan(
			&it.ID, &it.MatCode, &it.ItemName, &it.Description,
			&it.CategoryID, &it.CategoryName,
			&it.ItemType, &it.TrackingType, &it.Unit, &it.Qty, &it.UnitCost,
			&it.QRCode, &it.IsActive, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return err
		}
		items = append(items, it)
	}
	if items == nil {
		items = []models.StockItem{}
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

// GetByID godoc
// @Summary      ดู Stock Item ตาม ID
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Item ID"
// @Success      200  {object}  fiber.Map
// @Router       /stock/items/{id} [get]
func (h *StockItemHandler) GetByID(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var it models.StockItem
	err = h.db.QueryRow(ctx, `
		SELECT si.id, si.mat_code, si.item_name, si.description,
		       si.category_id, sc.name AS category_name,
		       si.item_type, si.tracking_type, si.unit, si.qty, si.unit_cost,
		       si.qr_code,
		       si.is_active, si.created_at, si.updated_at
		FROM stock_item si
		LEFT JOIN stock_category sc ON sc.id = si.category_id
		WHERE si.id = $1`, id,
	).Scan(
		&it.ID, &it.MatCode, &it.ItemName, &it.Description,
		&it.CategoryID, &it.CategoryName,
		&it.ItemType, &it.TrackingType, &it.Unit, &it.Qty, &it.UnitCost,
		&it.QRCode, &it.IsActive, &it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "item not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": it})
}

// Create godoc
// @Summary      สร้าง Stock Item
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateStockItemRequest  true  "Request body"
// @Success      201  {object}  fiber.Map
// @Router       /stock/items [post]
func (h *StockItemHandler) Create(c *fiber.Ctx) error {
	var req models.CreateStockItemRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.ItemName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "item_name is required")
	}
	if req.ItemType != "RETURNABLE" && req.ItemType != "CONSUMABLE" {
		return fiber.NewError(fiber.StatusBadRequest, "item_type must be RETURNABLE or CONSUMABLE")
	}
	if req.Unit == "" {
		return fiber.NewError(fiber.StatusBadRequest, "unit is required")
	}

	ctx := context.Background()

	itemCode := req.MatCode
	if itemCode == "" {
		var err error
		itemCode, err = h.generateItemCode(ctx)
		if err != nil {
			return err
		}
	}
	qrCode := itemCode

	var id int64
	err := h.db.QueryRow(ctx, `
		INSERT INTO stock_item (mat_code, item_name, description, category_id, item_type, unit, qty, unit_cost, qr_code)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id`,
		itemCode, req.ItemName, req.Description, req.CategoryID,
		req.ItemType, req.Unit, req.Qty, req.UnitCost, qrCode,
	).Scan(&id)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id, "mat_code": itemCode}})
}

// Update godoc
// @Summary      แก้ไข Stock Item
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                            true  "Item ID"
// @Param        body  body  models.UpdateStockItemRequest  true  "Request body"
// @Success      200  {object}  fiber.Map
// @Router       /stock/items/{id} [put]
func (h *StockItemHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.UpdateStockItemRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.ItemName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "item_name is required")
	}
	if req.ItemType != "RETURNABLE" && req.ItemType != "CONSUMABLE" {
		return fiber.NewError(fiber.StatusBadRequest, "item_type must be RETURNABLE or CONSUMABLE")
	}
	if req.Unit == "" {
		return fiber.NewError(fiber.StatusBadRequest, "unit is required")
	}

	ctx := context.Background()
	tag, err := h.db.Exec(ctx, `
		UPDATE stock_item
		SET item_name=$1, description=$2, category_id=$3, item_type=$4, unit=$5, qty=$6, unit_cost=$7, updated_at=NOW()
		WHERE id=$8`,
		req.ItemName, req.Description, req.CategoryID, req.ItemType, req.Unit, req.Qty, req.UnitCost, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "item not found")
	}
	return c.JSON(fiber.Map{"success": true})
}

// SoftDelete godoc
// @Summary      ลบ Stock Item (soft delete)
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Item ID"
// @Success      200  {object}  fiber.Map
// @Router       /stock/items/{id} [delete]
func (h *StockItemHandler) SoftDelete(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var count int
	err = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM borrow b
		JOIN borrow_line bl ON bl.borrow_id = b.id
		WHERE bl.stock_item_id = $1
		  AND b.status NOT IN ('RETURNED','CANCELLED','REJECTED')`, id).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return fiber.NewError(fiber.StatusConflict, "item has active borrow requests")
	}

	tag, err := h.db.Exec(ctx, `UPDATE stock_item SET is_active=false, updated_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "item not found")
	}
	return c.JSON(fiber.Map{"success": true})
}

// ImportExcel godoc
// @Summary      Import Stock Items จาก Excel
// @Tags         Stock
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        file  formData  file  true  "Excel file"
// @Success      200   {object}  fiber.Map
// @Router       /stock/items/import [post]
func (h *StockItemHandler) ImportExcel(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file is required")
	}
	f, err := file.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cannot open file")
	}
	defer f.Close()

	xlsx, err := excelize.OpenReader(f)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid excel file")
	}

	sheetName := xlsx.GetSheetName(0)
	rows, err := xlsx.GetRows(sheetName)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cannot read sheet")
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	imported := 0
	var errs []string

	// Source file has a 3-row merged header (row 1-3); data starts at row 4 (index 3).
	// Columns: 0 no., 1 item, 2 description, 3 qty, 4 unit, 5 unit_code, 6 mat_code,
	// 7 cost_code_material (unused — cost code is now chosen per PR line, not per material),
	// 8 cost_code_labor, 9 unit_price, 10 amount.
	for rowIdx, row := range rows {
		if rowIdx < 3 {
			continue
		}
		itemName := strings.TrimSpace(getCell(row, 2))
		qtyStr := strings.TrimSpace(getCell(row, 3))
		unit := strings.TrimSpace(getCell(row, 4))
		matCode := strings.TrimSpace(getCell(row, 6))
		unitCostStr := strings.TrimSpace(getCell(row, 9))

		if itemName == "" && matCode == "" {
			continue // blank trailing row
		}
		if itemName == "" {
			errs = append(errs, fmt.Sprintf("row %d: item_name is required", rowIdx+1))
			continue
		}
		if matCode == "" {
			errs = append(errs, fmt.Sprintf("row %d: mat_code is required", rowIdx+1))
			continue
		}

		qty := 0.0
		if qtyStr != "" {
			var perr error
			qty, perr = strconv.ParseFloat(sanitizeNumericCell(qtyStr), 64)
			if perr != nil {
				errs = append(errs, fmt.Sprintf("row %d: จำนวนไม่ถูกต้อง (ค่า: '%s')", rowIdx+1, qtyStr))
				continue
			}
		}
		unitCost := 0.0
		if unitCostStr != "" {
			var perr error
			unitCost, perr = strconv.ParseFloat(sanitizeNumericCell(unitCostStr), 64)
			if perr != nil {
				errs = append(errs, fmt.Sprintf("row %d: ราคาต้นทุนไม่ถูกต้อง (ค่า: '%s')", rowIdx+1, unitCostStr))
				continue
			}
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO stock_item (mat_code, item_name, item_type, unit, qty, unit_cost)
			VALUES ($1,$2,'CONSUMABLE',$3,$4,$5)
			ON CONFLICT (mat_code) DO UPDATE
			SET item_name=EXCLUDED.item_name,
			    unit=EXCLUDED.unit,
			    qty=EXCLUDED.qty,
			    unit_cost=EXCLUDED.unit_cost,
			    updated_at=NOW()`,
			matCode, itemName, unit, qty, unitCost,
		)
		if err != nil {
			errs = append(errs, fmt.Sprintf("row %d: %s", rowIdx+1, err.Error()))
			continue
		}
		imported++
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"imported": imported,
		"errors":   errs,
	}})
}

func getCell(row []string, idx int) string {
	if idx < len(row) {
		return row[idx]
	}
	return ""
}

// sanitizeNumericCell strips thousands separators, currency symbols, and
// whitespace so values like "1,500.00" or "฿1,500" parse with ParseFloat.
func sanitizeNumericCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "฿", "")
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// Lookup godoc
// @Summary      หา stock qty_on_hand จาก mat_code (สำหรับหน้า PO)
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        mat_code  query  string  true  "Material code"
// @Success      200  {object}  fiber.Map
// @Router       /stock/lookup [get]
func (h *StockItemHandler) Lookup(c *fiber.Ctx) error {
	matCode := strings.TrimSpace(c.Query("mat_code"))
	if matCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "mat_code is required")
	}
	ctx := context.Background()

	var itemName string
	var qtyOnHand float64
	err := h.db.QueryRow(ctx, `
		SELECT si.item_name, COALESCE(SUM(inv.qty_on_hand), 0) AS qty_on_hand
		FROM stock_item si
		LEFT JOIN stock_inventory inv ON inv.item_id = si.id
		WHERE si.mat_code = $1
		GROUP BY si.item_name`, matCode,
	).Scan(&itemName, &qtyOnHand)
	if err != nil {
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"mat_code": matCode, "found": false}})
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"mat_code":    matCode,
		"item_name":   itemName,
		"qty_on_hand": qtyOnHand,
		"found":       true,
	}})
}

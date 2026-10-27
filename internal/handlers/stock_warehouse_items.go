package handlers

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ListWarehouseItems godoc
// @Summary      Stock list of ONE warehouse (Master > คลังสินค้า page)
// @Description  Same rows and columns as the WAREHOUSE-source balance of the Stock page (Cost Code, MatCode, MatName, store description, quantity, unit, unit cost), but addressed by warehouse_code instead of a project. An item belongs in the warehouse when it has a quantity OR a Cost Code: only_with_stock (default true) keeps rows with qty > 0 or a Cost Code and hides the empty placeholder rows that mirror the material master; pass only_with_stock=false to list every row.
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        warehouse_code   query  string  true   "BO | FAC-P | FAC-S | HO ..."
// @Param        search           query  string  false  "mat_code / name / spec / store description"
// @Param        only_with_stock  query  bool    false  "default true"
// @Param        page             query  int     false  "Page"
// @Param        page_size        query  int     false  "Page size (max 100)"
// @Router       /stock/warehouse-items [get]
func (h *StockByProjectHandler) ListWarehouseItems(c *fiber.Ctx) error {
	wh := strings.TrimSpace(c.Query("warehouse_code"))
	if wh == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกคลัง")
	}
	ctx := context.Background()
	var ok bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM warehouse WHERE warehouse_code=$1)`, wh).Scan(&ok); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดข้อมูลคลังได้")
	}
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่พบคลังที่เลือก")
	}

	page, size, offset := stockPaging(c)
	items, total, err := h.warehouseBalance(ctx, wh, strings.TrimSpace(c.Query("search")), c.QueryBool("only_with_stock", true), true, size, offset)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"warehouse_code": wh,
		"data":           items,
		"total":          total,
		"page":           page,
		"page_size":      size,
		"total_pages":    stockTotalPages(total, size),
	}})
}

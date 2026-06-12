package handlers

import (
	"context"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WarehouseHandler struct {
	db *pgxpool.Pool
}

func NewWarehouseHandler(db *pgxpool.Pool) *WarehouseHandler {
	return &WarehouseHandler{db: db}
}

// ListWarehouses godoc
// @Summary      List all warehouses
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /master/warehouses [get]
func (h *WarehouseHandler) ListWarehouses(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT id, warehouse_code, warehouse_name, address, is_active, created_at, updated_at, created_by, updated_by
		FROM warehouse WHERE is_active = true ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.WarehouseFull
	for rows.Next() {
		var w models.WarehouseFull
		if err := rows.Scan(&w.Id, &w.WarehouseCode, &w.WarehouseName, &w.Address, &w.IsActive,
			&w.CreatedAt, &w.UpdatedAt, &w.CreatedBy, &w.UpdatedBy); err != nil {
			return err
		}
		items = append(items, w)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetWarehouse godoc
// @Summary      Get warehouse by code
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Warehouse Code"
// @Success      200   {object}  models.WarehouseFull
// @Failure      404   {object}  fiber.Map
// @Router       /master/warehouses/{code} [get]
func (h *WarehouseHandler) GetWarehouse(c *fiber.Ctx) error {
	code := c.Params("code")

	var w models.WarehouseFull
	err := h.db.QueryRow(context.Background(),
		`SELECT id, warehouse_code, warehouse_name, address, is_active, created_at, updated_at, created_by, updated_by
		FROM warehouse WHERE warehouse_code = $1`, code).
		Scan(&w.Id, &w.WarehouseCode, &w.WarehouseName, &w.Address, &w.IsActive,
			&w.CreatedAt, &w.UpdatedAt, &w.CreatedBy, &w.UpdatedBy)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "warehouse not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": w})
}

// CreateWarehouse godoc
// @Summary      Create warehouse
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateWarehouseReq  true  "Warehouse data"
// @Success      201   {object}  models.WarehouseFull
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /master/warehouses [post]
func (h *WarehouseHandler) CreateWarehouse(c *fiber.Ctx) error {
	var req models.CreateWarehouseReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.WarehouseCode == "" || req.WarehouseName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "warehouse_code and warehouse_name are required")
	}

	claims := middleware.GetClaims(c)

	var w models.WarehouseFull
	err := h.db.QueryRow(context.Background(),
		`INSERT INTO warehouse (warehouse_code, warehouse_name, address, is_active, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING id, warehouse_code, warehouse_name, address, is_active, created_at, updated_at, created_by, updated_by`,
		req.WarehouseCode, req.WarehouseName, req.Address, req.IsActive, claims.UserID).
		Scan(&w.Id, &w.WarehouseCode, &w.WarehouseName, &w.Address, &w.IsActive,
			&w.CreatedAt, &w.UpdatedAt, &w.CreatedBy, &w.UpdatedBy)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusConflict, "warehouse_code already exists")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": w})
}

// UpdateWarehouse godoc
// @Summary      Update warehouse
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        code  path  string                    true  "Warehouse Code"
// @Param        body  body  models.UpdateWarehouseReq  true  "Update data"
// @Success      200   {object}  models.WarehouseFull
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /master/warehouses/{code} [put]
func (h *WarehouseHandler) UpdateWarehouse(c *fiber.Ctx) error {
	code := c.Params("code")

	var req models.UpdateWarehouseReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.WarehouseName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "warehouse_name is required")
	}

	claims := middleware.GetClaims(c)

	var w models.WarehouseFull
	err := h.db.QueryRow(context.Background(),
		`UPDATE warehouse SET warehouse_name = $1, address = $2, updated_at = NOW(), updated_by = $3
		WHERE warehouse_code = $4 AND is_active = true
		RETURNING id, warehouse_code, warehouse_name, address, is_active, created_at, updated_at, created_by, updated_by`,
		req.WarehouseName, req.Address, claims.UserID, code).
		Scan(&w.Id, &w.WarehouseCode, &w.WarehouseName, &w.Address, &w.IsActive,
			&w.CreatedAt, &w.UpdatedAt, &w.CreatedBy, &w.UpdatedBy)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "warehouse not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": w})
}

// DeleteWarehouse godoc
// @Summary      Soft-delete warehouse
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Warehouse Code"
// @Success      200   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /master/warehouses/{code} [delete]
func (h *WarehouseHandler) DeleteWarehouse(c *fiber.Ctx) error {
	code := c.Params("code")
	claims := middleware.GetClaims(c)

	tag, err := h.db.Exec(context.Background(),
		`UPDATE warehouse SET is_active = false, updated_at = NOW(), updated_by = $2
		WHERE warehouse_code = $1 AND is_active = true`,
		code, claims.UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "warehouse not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "warehouse deleted"})
}

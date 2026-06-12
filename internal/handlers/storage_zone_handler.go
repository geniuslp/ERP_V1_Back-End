package handlers

import (
	"context"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StorageZoneHandler struct {
	db *pgxpool.Pool
}

func NewStorageZoneHandler(db *pgxpool.Pool) *StorageZoneHandler {
	return &StorageZoneHandler{db: db}
}

// ListZones godoc
// @Summary      List storage zones
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        warehouse_id  query  int  false  "Filter by warehouse_id"
// @Success      200   {object}  fiber.Map
// @Failure      500   {object}  fiber.Map
// @Router       /master/zones [get]
func (h *StorageZoneHandler) ListZones(c *fiber.Ctx) error {
	query := `SELECT id, warehouse_id, zone_code, zone_name, zone_type,
		created_at, updated_at, created_by, updated_by FROM storage_zone`
	args := []interface{}{}
	if wid := c.QueryInt("warehouse_id"); wid > 0 {
		query += ` WHERE warehouse_id = $1`
		args = append(args, wid)
	}
	query += ` ORDER BY id ASC`

	rows, err := h.db.Query(context.Background(), query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.StorageZoneFull
	for rows.Next() {
		var z models.StorageZoneFull
		if err := rows.Scan(&z.Id, &z.WarehouseId, &z.ZoneCode, &z.ZoneName, &z.ZoneType,
			&z.CreatedAt, &z.UpdatedAt, &z.CreatedBy, &z.UpdatedBy); err != nil {
			return err
		}
		items = append(items, z)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetZone godoc
// @Summary      Get storage zone by id
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Zone ID"
// @Success      200  {object}  models.StorageZoneFull
// @Failure      404  {object}  fiber.Map
// @Router       /master/zones/{id} [get]
func (h *StorageZoneHandler) GetZone(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var z models.StorageZoneFull
	err = h.db.QueryRow(context.Background(),
		`SELECT id, warehouse_id, zone_code, zone_name, zone_type,
		created_at, updated_at, created_by, updated_by
		FROM storage_zone WHERE id = $1`, id).
		Scan(&z.Id, &z.WarehouseId, &z.ZoneCode, &z.ZoneName, &z.ZoneType,
			&z.CreatedAt, &z.UpdatedAt, &z.CreatedBy, &z.UpdatedBy)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "zone not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": z})
}

// CreateZone godoc
// @Summary      Create storage zone
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateZoneReq  true  "Zone data"
// @Success      201   {object}  models.StorageZoneFull
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /master/zones [post]
func (h *StorageZoneHandler) CreateZone(c *fiber.Ctx) error {
	var req models.CreateZoneReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.WarehouseId == 0 || req.ZoneCode == "" || req.ZoneName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "warehouse_id, zone_code and zone_name are required")
	}

	claims := middleware.GetClaims(c)

	var z models.StorageZoneFull
	err := h.db.QueryRow(context.Background(),
		`INSERT INTO storage_zone (warehouse_id, zone_code, zone_name, zone_type, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING id, warehouse_id, zone_code, zone_name, zone_type,
		created_at, updated_at, created_by, updated_by`,
		req.WarehouseId, req.ZoneCode, req.ZoneName, req.ZoneType, claims.UserID).
		Scan(&z.Id, &z.WarehouseId, &z.ZoneCode, &z.ZoneName, &z.ZoneType,
			&z.CreatedAt, &z.UpdatedAt, &z.CreatedBy, &z.UpdatedBy)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusConflict, "zone_code already exists in this warehouse")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": z})
}

// UpdateZone godoc
// @Summary      Update storage zone
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                   true  "Zone ID"
// @Param        body  body  models.UpdateZoneReq  true  "Update data"
// @Success      200   {object}  models.StorageZoneFull
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /master/zones/{id} [put]
func (h *StorageZoneHandler) UpdateZone(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var req models.UpdateZoneReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.ZoneName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "zone_name is required")
	}

	claims := middleware.GetClaims(c)

	var z models.StorageZoneFull
	err = h.db.QueryRow(context.Background(),
		`UPDATE storage_zone SET zone_name=$1, zone_type=$2, updated_at=NOW(), updated_by=$3
		WHERE id=$4
		RETURNING id, warehouse_id, zone_code, zone_name, zone_type,
		created_at, updated_at, created_by, updated_by`,
		req.ZoneName, req.ZoneType, claims.UserID, id).
		Scan(&z.Id, &z.WarehouseId, &z.ZoneCode, &z.ZoneName, &z.ZoneType,
			&z.CreatedAt, &z.UpdatedAt, &z.CreatedBy, &z.UpdatedBy)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "zone not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": z})
}

// DeleteZone godoc
// @Summary      Delete storage zone (hard delete — no is_active column)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Zone ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/zones/{id} [delete]
func (h *StorageZoneHandler) DeleteZone(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	claims := middleware.GetClaims(c)
	_ = claims

	tag, err := h.db.Exec(context.Background(),
		`DELETE FROM storage_zone WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "zone not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "zone deleted"})
}

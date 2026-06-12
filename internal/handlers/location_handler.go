package handlers

import (
	"context"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LocationHandler struct {
	db *pgxpool.Pool
}

func NewLocationHandler(db *pgxpool.Pool) *LocationHandler {
	return &LocationHandler{db: db}
}

const locationCols = `id, location_code, location_name, location_type, parent_id, is_active,
	created_at, updated_at, created_by, updated_by`

func scanLocation(l *models.LocationFull, row interface {
	Scan(dest ...any) error
}) error {
	return row.Scan(&l.Id, &l.LocationCode, &l.LocationName, &l.LocationType, &l.ParentId,
		&l.IsActive, &l.CreatedAt, &l.UpdatedAt, &l.CreatedBy, &l.UpdatedBy)
}

// ListLocations godoc
// @Summary      List all active locations
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        type  query  string  false  "location_type filter (department|project|site)"
// @Success      200   {object}  fiber.Map
// @Failure      500   {object}  fiber.Map
// @Router       /master/locations [get]
func (h *LocationHandler) ListLocations(c *fiber.Ctx) error {
	locType := c.Query("type")
	rows, err := h.db.Query(context.Background(), `
		SELECT `+locationCols+`
		FROM location
		WHERE ($1 = '' OR location_type = $1) AND is_active = true
		ORDER BY location_name`, locType)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.LocationFull
	for rows.Next() {
		var l models.LocationFull
		if err := scanLocation(&l, rows); err != nil {
			return err
		}
		items = append(items, l)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetLocation godoc
// @Summary      Get location by ID
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Location ID"
// @Success      200  {object}  models.LocationFull
// @Failure      404  {object}  fiber.Map
// @Router       /master/locations/{id} [get]
func (h *LocationHandler) GetLocation(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var l models.LocationFull
	if err := scanLocation(&l, h.db.QueryRow(context.Background(),
		`SELECT `+locationCols+` FROM location WHERE id = $1`, id)); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "location not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": l})
}

// CreateLocation godoc
// @Summary      Create location
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateLocationReq  true  "Location data"
// @Success      201   {object}  models.LocationFull
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /master/locations [post]
func (h *LocationHandler) CreateLocation(c *fiber.Ctx) error {
	var req models.CreateLocationReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.LocationCode == "" || req.LocationName == "" || req.LocationType == "" {
		return fiber.NewError(fiber.StatusBadRequest, "location_code, location_name and location_type are required")
	}

	claims := middleware.GetClaims(c)

	var l models.LocationFull
	err := scanLocation(&l, h.db.QueryRow(context.Background(), `
		INSERT INTO location (location_code, location_name, location_type, parent_id, is_active, created_by, updated_by)
		VALUES ($1, $2, $3, $4, true, $5, $5)
		RETURNING `+locationCols,
		req.LocationCode, req.LocationName, req.LocationType, req.ParentId, claims.UserID))
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusConflict, "location_code already exists")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": l})
}

// UpdateLocation godoc
// @Summary      Update location
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                       true  "Location ID"
// @Param        body  body  models.UpdateLocationReq  true  "Update data"
// @Success      200   {object}  models.LocationFull
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /master/locations/{id} [put]
func (h *LocationHandler) UpdateLocation(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.UpdateLocationReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.LocationCode == "" || req.LocationName == "" || req.LocationType == "" {
		return fiber.NewError(fiber.StatusBadRequest, "location_code, location_name and location_type are required")
	}

	claims := middleware.GetClaims(c)

	var l models.LocationFull
	err = scanLocation(&l, h.db.QueryRow(context.Background(), `
		UPDATE location
		SET location_code=$1, location_name=$2, location_type=$3, parent_id=$4, is_active=$5,
		    updated_at=NOW(), updated_by=$6
		WHERE id=$7
		RETURNING `+locationCols,
		req.LocationCode, req.LocationName, req.LocationType, req.ParentId, req.IsActive,
		claims.UserID, id))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "location not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": l})
}

// DeleteLocation godoc
// @Summary      Soft-delete location
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Location ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/locations/{id} [delete]
func (h *LocationHandler) DeleteLocation(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	claims := middleware.GetClaims(c)

	tag, err := h.db.Exec(context.Background(),
		`UPDATE location SET is_active=false, updated_at=NOW(), updated_by=$2 WHERE id=$1`,
		id, claims.UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "location not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "location deleted"})
}

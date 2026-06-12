package handlers

import (
	"context"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GroupHandler struct {
	db *pgxpool.Pool
}

func NewGroupHandler(db *pgxpool.Pool) *GroupHandler {
	return &GroupHandler{db: db}
}

// ListGroups godoc
// @Summary      List all material groups
// @Tags         Groups
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /groups [get]
func (h *GroupHandler) ListGroups(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT id, group_code, group_name, created_at, updated_at, is_active, created_by, updated_by
		FROM mat_group
		WHERE is_active = true ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.MatGroup
	for rows.Next() {
		var g models.MatGroup
		if err := rows.Scan(&g.Id, &g.GroupCode, &g.GroupName, &g.CreatedAt, &g.UpdatedAt, &g.IsActive, &g.CreatedBy, &g.UpdatedBy); err != nil {
			return err
		}
		items = append(items, g)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetGroup godoc
// @Summary      Get material group by code
// @Tags         Groups
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Group Code"
// @Success      200   {object}  models.MatGroup
// @Failure      404   {object}  fiber.Map
// @Router       /groups/{code} [get]
func (h *GroupHandler) GetGroup(c *fiber.Ctx) error {
	code := c.Params("code")

	var g models.MatGroup
	err := h.db.QueryRow(context.Background(),
		`SELECT id, group_code, group_name, created_at, updated_at, is_active, created_by, updated_by
		FROM mat_group WHERE group_code = $1`, code).
		Scan(&g.Id, &g.GroupCode, &g.GroupName, &g.CreatedAt, &g.UpdatedAt, &g.IsActive, &g.CreatedBy, &g.UpdatedBy)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "group not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": g})
}

// CreateGroup godoc
// @Summary      Create material group
// @Tags         Groups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateGroupRequest  true  "Group data"
// @Success      201   {object}  models.MatGroup
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /groups [post]
func (h *GroupHandler) CreateGroup(c *fiber.Ctx) error {
	var req models.CreateGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.GroupCode == "" || req.GroupName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "group_code and group_name are required")
	}

	claims := middleware.GetClaims(c)

	var g models.MatGroup
	err := h.db.QueryRow(context.Background(),
		`INSERT INTO mat_group (group_code, group_name, is_active, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $4)
		RETURNING id, group_code, group_name, created_at, updated_at, is_active, created_by, updated_by`,
		req.GroupCode, req.GroupName, req.IsActive, claims.UserID).
		Scan(&g.Id, &g.GroupCode, &g.GroupName, &g.CreatedAt, &g.UpdatedAt, &g.IsActive, &g.CreatedBy, &g.UpdatedBy)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusConflict, "group_code already exists")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": g})
}

// UpdateGroup godoc
// @Summary      Update material group
// @Tags         Groups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        code  path  string                    true  "Group Code"
// @Param        body  body  models.UpdateGroupRequest  true  "Group data"
// @Success      200   {object}  models.MatGroup
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /groups/{code} [put]
func (h *GroupHandler) UpdateGroup(c *fiber.Ctx) error {
	code := c.Params("code")

	var req models.UpdateGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.GroupName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "group_name is required")
	}

	claims := middleware.GetClaims(c)

	var g models.MatGroup
	err := h.db.QueryRow(context.Background(),
		`UPDATE mat_group
		SET group_name = $1, updated_at = NOW(), updated_by = $2
		WHERE group_code = $3 AND is_active = true
		RETURNING id, group_code, group_name, created_at, updated_at, is_active, created_by, updated_by`,
		req.GroupName, claims.UserID, code).
		Scan(&g.Id, &g.GroupCode, &g.GroupName, &g.CreatedAt, &g.UpdatedAt, &g.IsActive, &g.CreatedBy, &g.UpdatedBy)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "group not found")
	}

	return c.JSON(fiber.Map{"success": true, "data": g})
}

// DeleteGroup godoc
// @Summary      Delete material group (soft delete)
// @Tags         Groups
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Group Code"
// @Success      200   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /groups/{code} [delete]
func (h *GroupHandler) DeleteGroup(c *fiber.Ctx) error {
	code := c.Params("code")
	claims := middleware.GetClaims(c)

	tag, err := h.db.Exec(context.Background(),
		`UPDATE mat_group SET is_active = false, updated_at = NOW(), updated_by = $2
		WHERE group_code = $1 AND is_active = true`,
		code, claims.UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "group not found")
	}

	return c.JSON(fiber.Map{"success": true, "message": "group deleted"})
}

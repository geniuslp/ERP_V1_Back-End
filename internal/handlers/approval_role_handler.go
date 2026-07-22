package handlers

import (
	"context"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApprovalRoleHandler struct{ db *pgxpool.Pool }

func NewApprovalRoleHandler(db *pgxpool.Pool) *ApprovalRoleHandler {
	return &ApprovalRoleHandler{db: db}
}

// List godoc
// @Summary      List roles
// @Tags         Roles
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /roles [get]
func (h *ApprovalRoleHandler) List(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT id, role_code, role_name, COALESCE(description,''), dept_code, is_active FROM roles ORDER BY id`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type item struct {
		ID          int64  `json:"id"`
		RoleCode    string `json:"role_code"`
		RoleName    string `json:"role_name"`
		Description string `json:"description"`
		DeptCode    *string `json:"dept_code"`
		IsActive    bool   `json:"is_active"`
	}
	var out []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.RoleCode, &it.RoleName, &it.Description, &it.DeptCode, &it.IsActive); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out = append(out, it)
	}
	if out == nil {
		out = []item{}
	}
	return c.JSON(fiber.Map{"data": out})
}

// Create godoc
// @Summary      Create role
// @Tags         Roles
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "role_name, description"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /roles [post]
func (h *ApprovalRoleHandler) Create(c *fiber.Ctx) error {
	var body struct {
		RoleName    string `json:"role_name"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.RoleName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role_name is required"})
	}

	roleCode := "ROLE_" + strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(body.RoleName), " ", "_"))

	var id int64
	err := h.db.QueryRow(context.Background(),
		`INSERT INTO roles (role_code, role_name, description, is_active) VALUES ($1, $2, $3, true) RETURNING id`,
		roleCode, body.RoleName, body.Description,
	).Scan(&id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": fiber.Map{
		"id": id, "role_code": roleCode, "role_name": body.RoleName,
		"description": body.Description, "is_active": true,
	}})
}

// Toggle godoc
// @Summary      Toggle role active status
// @Tags         Roles
// @Accept       json
// @Produce      json
// @Param        id    path  int     true  "Role ID"
// @Param        body  body  object  true  "is_active"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /roles/{id} [patch]
func (h *ApprovalRoleHandler) Toggle(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	tag, err := h.db.Exec(context.Background(),
		`UPDATE roles SET is_active = $1, updated_at = NOW() WHERE id = $2`, body.IsActive, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"id": id, "is_active": body.IsActive}})
}

// Delete godoc
// @Summary      Delete role (rejected if referenced by approval configs)
// @Tags         Roles
// @Produce      json
// @Param        id  path  int  true  "Role ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /roles/{id} [delete]
func (h *ApprovalRoleHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	var count int
	h.db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM approval_configs WHERE approver_role_id = $1`, id).Scan(&count)
	if count > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "role is referenced by approval configs"})
	}

	tag, err := h.db.Exec(context.Background(), `DELETE FROM roles WHERE id = $1`, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"id": id, "deleted": true}})
}

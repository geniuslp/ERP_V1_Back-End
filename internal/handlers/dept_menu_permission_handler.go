package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeptMenuPermissionHandler struct{ db *pgxpool.Pool }

func NewDeptMenuPermissionHandler(db *pgxpool.Pool) *DeptMenuPermissionHandler {
	return &DeptMenuPermissionHandler{db: db}
}

// List godoc
// @Summary      List department menu permissions
// @Tags         Dept-Menu Permissions
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /dept-menu-permissions [get]
func (h *DeptMenuPermissionHandler) List(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT id, dept_code, menu_id, can_read, can_write, can_update, can_delete
		 FROM dept_menu_permissions
		 ORDER BY dept_code, menu_id`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type item struct {
		ID        int64  `json:"id"`
		DeptCode  string `json:"dept_code"`
		MenuID    int64  `json:"menu_id"`
		CanRead   bool   `json:"can_read"`
		CanWrite  bool   `json:"can_write"`
		CanUpdate bool   `json:"can_update"`
		CanDelete bool   `json:"can_delete"`
	}
	var out []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.DeptCode, &it.MenuID, &it.CanRead, &it.CanWrite, &it.CanUpdate, &it.CanDelete); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out = append(out, it)
	}
	if out == nil {
		out = []item{}
	}
	return c.JSON(fiber.Map{"data": out})
}

// BatchUpsert godoc
// @Summary      Batch upsert department menu permissions
// @Tags         Dept-Menu Permissions
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "permissions array"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /dept-menu-permissions/batch [post]
func (h *DeptMenuPermissionHandler) BatchUpsert(c *fiber.Ctx) error {
	var body struct {
		Permissions []struct {
			DeptCode  string `json:"dept_code"`
			MenuID    int64  `json:"menu_id"`
			CanRead   bool   `json:"can_read"`
			CanWrite  bool   `json:"can_write"`
			CanUpdate bool   `json:"can_update"`
			CanDelete bool   `json:"can_delete"`
		} `json:"permissions"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if len(body.Permissions) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "permissions must not be empty"})
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer tx.Rollback(ctx)

	for _, p := range body.Permissions {
		if !p.CanRead {
			p.CanWrite = false
			p.CanUpdate = false
			p.CanDelete = false
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO dept_menu_permissions (dept_code, menu_id, can_read, can_write, can_update, can_delete)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (dept_code, menu_id) DO UPDATE SET
				can_read   = EXCLUDED.can_read,
				can_write  = EXCLUDED.can_write,
				can_update = EXCLUDED.can_update,
				can_delete = EXCLUDED.can_delete`,
			p.DeptCode, p.MenuID, p.CanRead, p.CanWrite, p.CanUpdate, p.CanDelete,
		); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"updated": len(body.Permissions)}})
}

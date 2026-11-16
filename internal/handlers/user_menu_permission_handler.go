package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserMenuPermissionHandler struct{ db *pgxpool.Pool }

func NewUserMenuPermissionHandler(db *pgxpool.Pool) *UserMenuPermissionHandler {
	return &UserMenuPermissionHandler{db: db}
}

// BatchUpsert godoc
// @Summary      Batch upsert user menu permissions
// @Tags         User-Menu Permissions
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "user_id and permissions array"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /user-menu-permissions/batch [post]
func (h *UserMenuPermissionHandler) BatchUpsert(c *fiber.Ctx) error {
	var body struct {
		UserID      int64 `json:"user_id"`
		Permissions []struct {
			MenuID    int64 `json:"menu_id"`
			CanRead   bool  `json:"can_read"`
			CanWrite  bool  `json:"can_write"`
			CanUpdate bool  `json:"can_update"`
			CanDelete bool  `json:"can_delete"`
		} `json:"permissions"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.UserID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id is required"})
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
		// An override identical to what the user already inherits (role first, then department; same
		// per-column precedence as RequireMenuPermission) adds nothing and, once saved, would shadow any
		// later change made at the role/department level. Store nothing and drop any existing row instead.
		var inhRead, inhWrite, inhUpdate, inhDelete bool
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(rp.can_read,   dmp.can_read,   false),
			       COALESCE(rp.can_write,  dmp.can_write,  false),
			       COALESCE(rp.can_update, dmp.can_update, false),
			       COALESCE(rp.can_delete, dmp.can_delete, false)
			FROM menus m
			JOIN users u ON u.id = $1
			LEFT JOIN LATERAL (
				SELECT BOOL_OR(rmp.can_read)   AS can_read,
				       BOOL_OR(rmp.can_write)  AS can_write,
				       BOOL_OR(rmp.can_update) AS can_update,
				       BOOL_OR(rmp.can_delete) AS can_delete
				FROM user_roles ur
				JOIN role_menu_permissions rmp ON rmp.role_id = ur.role_id AND rmp.menu_id = m.id
				WHERE ur.user_id = u.id
			) rp ON TRUE
			LEFT JOIN dept_menu_permissions dmp ON dmp.dept_code = u.dept_code AND dmp.menu_id = m.id
			WHERE m.id = $2`,
			body.UserID, p.MenuID,
		).Scan(&inhRead, &inhWrite, &inhUpdate, &inhDelete); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unknown user or menu"})
		}
		if p.CanRead == inhRead && p.CanWrite == inhWrite && p.CanUpdate == inhUpdate && p.CanDelete == inhDelete {
			if _, err := tx.Exec(ctx,
				`DELETE FROM user_menu_permissions WHERE user_id = $1 AND menu_id = $2`,
				body.UserID, p.MenuID); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_menu_permissions (user_id, menu_id, can_read, can_write, can_update, can_delete)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, menu_id) DO UPDATE SET
				can_read   = EXCLUDED.can_read,
				can_write  = EXCLUDED.can_write,
				can_update = EXCLUDED.can_update,
				can_delete = EXCLUDED.can_delete`,
			body.UserID, p.MenuID, p.CanRead, p.CanWrite, p.CanUpdate, p.CanDelete,
		); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"updated": len(body.Permissions)}})
}

// DeleteByUser godoc
// @Summary      Delete all menu permissions for a user
// @Tags         User-Menu Permissions
// @Produce      json
// @Param        user_id  query  int  true  "User ID"
// @Success      204
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /user-menu-permissions [delete]
func (h *UserMenuPermissionHandler) DeleteByUser(c *fiber.Ctx) error {
	userID, err := strconv.ParseInt(c.Query("user_id"), 10, 64)
	if err != nil || userID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id query param is required"})
	}
	if _, err := h.db.Exec(context.Background(),
		`DELETE FROM user_menu_permissions WHERE user_id = $1`, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

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

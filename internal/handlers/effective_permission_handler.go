package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EffectivePermissionHandler struct{ db *pgxpool.Pool }

func NewEffectivePermissionHandler(db *pgxpool.Pool) *EffectivePermissionHandler {
	return &EffectivePermissionHandler{db: db}
}

type effectivePermissionRow struct {
	MenuID       int64   `json:"menu_id"`
	MenuCode     string  `json:"menu_code"`
	MenuName     string  `json:"menu_name"`
	ParentID     *int64  `json:"parent_id"`
	DeptCanRead  *bool   `json:"dept_can_read"`
	DeptCanWrite *bool   `json:"dept_can_write"`
	DeptCanUpdate *bool  `json:"dept_can_update"`
	DeptCanDelete *bool  `json:"dept_can_delete"`
	RoleCanRead  *bool   `json:"role_can_read"`
	RoleCanWrite *bool   `json:"role_can_write"`
	RoleCanUpdate *bool  `json:"role_can_update"`
	RoleCanDelete *bool  `json:"role_can_delete"`
	UserCanRead  *bool   `json:"user_can_read"`
	UserCanWrite *bool   `json:"user_can_write"`
	UserCanUpdate *bool  `json:"user_can_update"`
	UserCanDelete *bool  `json:"user_can_delete"`
}

// GetEffective godoc
// @Summary      Get effective permissions for a user (dept + role + user layers)
// @Tags         Permissions
// @Produce      json
// @Param        user_id  query  int  true  "User ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /permissions/effective [get]
func (h *EffectivePermissionHandler) GetEffective(c *fiber.Ctx) error {
	userID, err := strconv.ParseInt(c.Query("user_id"), 10, 64)
	if err != nil || userID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id query param is required"})
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT
			m.id          AS menu_id,
			m.menu_code,
			m.menu_name,
			m.parent_id,

			d.can_read    AS dept_can_read,
			d.can_write   AS dept_can_write,
			d.can_update  AS dept_can_update,
			d.can_delete  AS dept_can_delete,

			r.can_read    AS role_can_read,
			r.can_write   AS role_can_write,
			r.can_update  AS role_can_update,
			r.can_delete  AS role_can_delete,

			u.can_read    AS user_can_read,
			u.can_write   AS user_can_write,
			u.can_update  AS user_can_update,
			u.can_delete  AS user_can_delete

		FROM menus m
		LEFT JOIN users usr ON usr.id = $1
		LEFT JOIN dept_menu_permissions d
			ON d.dept_code = usr.dept_code AND d.menu_id = m.id
		LEFT JOIN user_roles ur ON ur.user_id = $1
		LEFT JOIN role_menu_permissions r
			ON r.role_id = ur.role_id AND r.menu_id = m.id
		LEFT JOIN user_menu_permissions u
			ON u.user_id = $1 AND u.menu_id = m.id
		WHERE m.is_active = true
		ORDER BY m.parent_id NULLS FIRST, m.sort_order`,
		userID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var out []effectivePermissionRow
	for rows.Next() {
		var p effectivePermissionRow
		if err := rows.Scan(
			&p.MenuID, &p.MenuCode, &p.MenuName, &p.ParentID,
			&p.DeptCanRead, &p.DeptCanWrite, &p.DeptCanUpdate, &p.DeptCanDelete,
			&p.RoleCanRead, &p.RoleCanWrite, &p.RoleCanUpdate, &p.RoleCanDelete,
			&p.UserCanRead, &p.UserCanWrite, &p.UserCanUpdate, &p.UserCanDelete,
		); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out = append(out, p)
	}
	if out == nil {
		out = []effectivePermissionRow{}
	}
	return c.JSON(fiber.Map{"data": out})
}

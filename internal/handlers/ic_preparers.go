package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
)

// icPreparerMenuCode is the menu the IC project list page (where the ผู้จัดทำ picker lives) is
// guarded by on the frontend (App.tsx: RequirePermission menuCode="MENU_IC_PROJECT").
const icPreparerMenuCode = "MENU_IC_PROJECT"

type icPreparer struct {
	ID         int64   `json:"id"`
	Username   string  `json:"username"`
	FullName   string  `json:"full_name"`
	Department *string `json:"department"`
}

// ListPreparers godoc
// @Summary      List users who may open the IC pages (options for the ผู้จัดทำ picker)
// @Description  Active users whose EFFECTIVE can_read on menu MENU_IC_PROJECT is true. Effective read uses the same
// @Description  precedence as the frontend PermissionContext.can(): a per-user row (user_menu_permissions.can_read) wins
// @Description  — including an explicit false, which denies; otherwise the role layer (role_menu_permissions, OR across
// @Description  the user's roles); otherwise the department layer (dept_menu_permissions); otherwise no access.
// @Description  Sorted by username. department is users.department (free text, used as position).
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/preparers [get]
func (h *ICHandler) ListPreparers(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(), `
		SELECT u.id, u.username, u.full_name, u.department
		FROM users u
		JOIN menus m ON m.menu_code = $1 AND m.is_active = true
		LEFT JOIN user_menu_permissions ump ON ump.user_id = u.id AND ump.menu_id = m.id
		LEFT JOIN LATERAL (
			SELECT BOOL_OR(rmp.can_read) AS can_read
			FROM user_roles ur
			JOIN role_menu_permissions rmp ON rmp.role_id = ur.role_id AND rmp.menu_id = m.id
			WHERE ur.user_id = u.id
		) rp ON TRUE
		LEFT JOIN dept_menu_permissions dmp ON dmp.dept_code = u.dept_code AND dmp.menu_id = m.id
		WHERE u.is_active = true
		  AND COALESCE(ump.can_read, rp.can_read, dmp.can_read, false)
		ORDER BY u.username`, icPreparerMenuCode)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายชื่อผู้จัดทำได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()

	items := []icPreparer{}
	for rows.Next() {
		var p icPreparer
		if err := rows.Scan(&p.ID, &p.Username, &p.FullName, &p.Department); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านรายชื่อผู้จัดทำได้")
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายชื่อผู้จัดทำได้ กรุณาลองใหม่อีกครั้ง")
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

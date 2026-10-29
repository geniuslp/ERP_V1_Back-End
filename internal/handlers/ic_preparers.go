package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
)

// Only these people are offered in the picker: the Supply Chain department and the system admin role.
const (
	icPreparerDeptCode  = "SC"
	icPreparerAdminRole = "ADMIN_CENTER"
)

type icPreparer struct {
	ID         int64   `json:"id"`
	Username   string  `json:"username"`
	FullName   string  `json:"full_name"`
	Department *string `json:"department"`
}

// ListPreparers godoc
// @Summary      List the people offered in the IC ผู้จัดทำ / ผู้ขอเบิก pickers
// @Description  Exactly the active users of the Supply Chain department (users.dept_code = SC) plus active users with the
// @Description  ADMIN_CENTER role — nobody else. Menu permissions are NOT consulted: this is just who may be named on an IC
// @Description  document. department is users.department (free text, used as position). Sorted by username.
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
		WHERE u.is_active = true
		  AND (u.dept_code = $1 OR EXISTS (
		        SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		        WHERE ur.user_id = u.id AND r.role_code = $2))
		ORDER BY u.username`, icPreparerDeptCode, icPreparerAdminRole)
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

package middleware

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// menuPermColumns whitelists the action -> column mapping (the column name is interpolated into SQL).
var menuPermColumns = map[string]string{
	"read": "can_read", "write": "can_write", "update": "can_update", "delete": "can_delete",
}

// RequireMenuPermission guards a route by menu_code + action (read|write|update|delete) using the same
// precedence as the frontend PermissionContext.can(): a per-user row (user_menu_permissions) wins,
// including an explicit false; otherwise the role layer (role_menu_permissions, OR across the user's
// roles); otherwise the department layer (dept_menu_permissions); otherwise denied. No role is
// hardcoded. Must run after JWTProtected.
func RequireMenuPermission(db *pgxpool.Pool, menuCode, action string) fiber.Handler {
	col, ok := menuPermColumns[action]
	if !ok {
		panic("RequireMenuPermission: unknown action " + action)
	}
	query := fmt.Sprintf(`
		SELECT COALESCE(ump.%[1]s, rp.%[1]s, dmp.%[1]s, false)
		FROM menus m
		JOIN users u ON u.id = $2
		LEFT JOIN user_menu_permissions ump ON ump.user_id = u.id AND ump.menu_id = m.id
		LEFT JOIN LATERAL (
			SELECT BOOL_OR(rmp.%[1]s) AS %[1]s
			FROM user_roles ur
			JOIN role_menu_permissions rmp ON rmp.role_id = ur.role_id AND rmp.menu_id = m.id
			WHERE ur.user_id = u.id
		) rp ON TRUE
		LEFT JOIN dept_menu_permissions dmp ON dmp.dept_code = u.dept_code AND dmp.menu_id = m.id
		WHERE m.menu_code = $1 AND m.is_active = true`, col)

	return func(c *fiber.Ctx) error {
		claims := GetClaims(c)
		if claims == nil {
			return fiber.NewError(fiber.StatusUnauthorized, "กรุณาเข้าสู่ระบบอีกครั้ง")
		}
		var allowed bool
		if err := db.QueryRow(context.Background(), query, menuCode, claims.UserID).Scan(&allowed); err != nil || !allowed {
			return fiber.NewError(fiber.StatusForbidden, "คุณไม่มีสิทธิ์ดำเนินการนี้")
		}
		return c.Next()
	}
}

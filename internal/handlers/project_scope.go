package handlers

import (
	"context"

	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Users of the Project department (departments.dept_code = projectDeptCode) only see the projects they are
// responsible for (project_responsible_person) or involved in (project_related_person), and the documents of those
// projects. Everyone else is unrestricted.
const projectDeptCode = "PJ"

// projectScopeSQL returns a boolean SQL condition that limits rows to the projects the user may see.
//   - userParam: the placeholder holding the user id, e.g. "$3" (the caller appends claims.UserID to its args)
//   - projectCol: the row's project code column, e.g. "m.project_code"
//   - ownExpr: optional extra condition that always lets a row through (e.g. the user's own document:
//     "m.created_by = $3"); "" for none
func projectScopeSQL(userParam, projectCol, ownExpr string) string {
	return projectScopeCore(userParam, projectCol, ownExpr, true)
}

// projectBudgetScopeSQL is the Budget variant: the dedicated warehouse projects (WH family) are hidden from a
// restricted Project-department user even when linked to one. Everyone else (other departments, admin, Board)
// is unrestricted and still sees them.
func projectBudgetScopeSQL(userParam, projectCol string) string {
	return projectScopeCore(userParam, projectCol, "", false)
}

func projectScopeCore(userParam, projectCol, ownExpr string, warehouseVisible bool) string {
	whExempt := ""
	whHide := ""
	if warehouseVisible {
		whExempt = `
			   OR NULLIF(sp.warehouse_code, '') IS NOT NULL`
	} else {
		whHide = ` AND NULLIF(sp.warehouse_code, '') IS NULL AND sp.project_code NOT LIKE '%-WH-%'`
	}
	s := `(NOT EXISTS (SELECT 1 FROM users su WHERE su.id = ` + userParam + ` AND su.dept_code = '` + projectDeptCode + `')
		OR EXISTS (SELECT 1 FROM user_roles sur JOIN roles sr ON sr.id = sur.role_id
		           WHERE sur.user_id = ` + userParam + ` AND sr.role_code IN ('ADMIN_CENTER', 'ROLE_B', 'ROLE_D', 'ROLE_E', 'ROLE_MD'))
		OR ` + projectCol + ` IN (SELECT sp.project_code FROM project sp
			WHERE (EXISTS (SELECT 1 FROM project_responsible_person spa WHERE spa.project_id = sp.id AND spa.user_id = ` + userParam + `)
			   OR EXISTS (SELECT 1 FROM project_related_person spb WHERE spb.project_id = sp.id AND spb.user_id = ` + userParam + `)` + whExempt + `)` + whHide + `)`
	if ownExpr != "" {
		s += ` OR ` + ownExpr
	}
	return s + `)`
}

// searchUserID is the JWT user id of the request (0 when absent, which projectScopeSQL treats as unrestricted).
func searchUserID(c *fiber.Ctx) int64 {
	if claims := middleware.GetClaims(c); claims != nil {
		return claims.UserID
	}
	return 0
}

// requireProjectAccess answers 403 when the caller is a Project-department user who is neither responsible for nor
// involved in the project (no warehouse-project exemption). Used by the Budget endpoints addressed by project code.
func requireProjectAccess(c *fiber.Ctx, db *pgxpool.Pool, projectCode string) error {
	uid := searchUserID(c)
	if uid == 0 {
		return nil
	}
	var ok bool
	if err := db.QueryRow(context.Background(),
		`SELECT `+projectBudgetScopeSQL("$1", "$2::text"), uid, projectCode).Scan(&ok); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบสิทธิ์โครงการได้ กรุณาลองใหม่อีกครั้ง")
	}
	if !ok {
		return fiber.NewError(fiber.StatusForbidden, "คุณไม่มีสิทธิ์เข้าถึงโครงการนี้")
	}
	return nil
}

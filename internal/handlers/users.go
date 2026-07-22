package handlers

import (
	"context"
	"strconv"
	"strings"

	"erp-api/internal/auth"
	"erp-api/internal/config"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UsersHandler struct {
	db  *pgxpool.Pool
	cfg *config.Config
}

func NewUsersHandler(db *pgxpool.Pool, cfg *config.Config) *UsersHandler {
	return &UsersHandler{db: db, cfg: cfg}
}

// List godoc
// @Summary      List users
// @Description  Get all users
// @Tags         Users
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /users/allUser [get]
func (h *UsersHandler) List(c *fiber.Ctx) error {
	role := c.Query("role")

	var query string
	switch role {
	case "approver":
		query = `SELECT u.id, u.username, u.full_name, u.email, u.location_code, u.employee_code, u.department, u.dept_code, u.is_active, u.created_at, u.updated_at
				 FROM users u WHERE u.department = 'บริหาร' AND u.is_active = true ORDER BY u.full_name`
	case "requester":
		query = `SELECT u.id, u.username, u.full_name, u.email, u.location_code, u.employee_code, u.department, u.dept_code, u.is_active, u.created_at, u.updated_at
				 FROM users u
				 WHERE (u.department IN ('วิศวกรรม', 'ฝ่ายจัดซื้อ') OR u.department IS NULL)
				 AND u.is_active = true
				 ORDER BY u.full_name`
	case "engineering":
		query = `SELECT u.id, u.username, u.full_name, u.email, u.location_code, u.employee_code, u.department, u.dept_code, u.is_active, u.created_at, u.updated_at
				 FROM users u
				 WHERE u.department = 'วิศวกรรม' AND u.is_active = true
				 ORDER BY u.full_name`
	default:
		query = `SELECT u.id, u.username, u.full_name, u.email, u.location_code, u.employee_code, u.department, u.dept_code, u.is_active, u.created_at, u.updated_at
				 FROM users u ORDER BY u.id`
	}

	rows, err := h.db.Query(context.Background(), query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []fiber.Map
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.FullName, &u.Email, &u.LocationCode,
			&u.EmployeeCode, &u.Department, &u.DeptCode, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return err
		}

		roleInfos, err := h.fetchRoleInfos(u.ID)
		if err != nil {
			return err
		}
		if roleInfos == nil {
			roleInfos = []models.UserRoleInfo{}
		}

		items = append(items, fiber.Map{
			"id":            u.ID,
			"username":      u.Username,
			"full_name":     u.FullName,
			"email":         u.Email,
			"location_code": u.LocationCode,
			"employee_code": u.EmployeeCode,
			"department":    u.Department,
			"dept_code":     u.DeptCode,
			"is_active":     u.IsActive,
			"created_at":    u.CreatedAt,
			"updated_at":    u.UpdatedAt,
			"roles":         roleInfos,
		})
	}
	if items == nil {
		items = []fiber.Map{}
	}

	return c.JSON(fiber.Map{"success": true, "data": items})
}

func (h *UsersHandler) fetchRoles(userID int64) ([]string, error) {
	rows, err := h.db.Query(context.Background(), `
		SELECT r.role_code FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var r string
		rows.Scan(&r)
		roles = append(roles, r)
	}
	return roles, nil
}

func (h *UsersHandler) fetchRoleInfos(userID int64) ([]models.UserRoleInfo, error) {
	rows, err := h.db.Query(context.Background(), `
		SELECT r.id, r.role_code, r.role_name
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1
		ORDER BY r.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []models.UserRoleInfo
	for rows.Next() {
		var ri models.UserRoleInfo
		if err := rows.Scan(&ri.RoleID, &ri.RoleCode, &ri.RoleName); err != nil {
			return nil, err
		}
		roles = append(roles, ri)
	}
	return roles, nil
}

// Get godoc
// @Summary      Get user by ID
// @Description  Get a single user by ID
// @Tags         Users
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "User ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /users/{id} [get]
func (h *UsersHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")

	row := h.db.QueryRow(context.Background(), `
		SELECT u.id, u.username, u.full_name, u.email, u.dept_code, d.dept_name
		FROM users u
		LEFT JOIN departments d ON d.dept_code = u.dept_code
		WHERE u.id=$1`, id)

	var (
		userID   int64
		username string
		fullName string
		email    *string
		deptCode *string
		deptName *string
	)
	if err := row.Scan(&userID, &username, &fullName, &email, &deptCode, &deptName); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}

	roleInfos, err := h.fetchRoleInfos(userID)
	if err != nil {
		return err
	}
	if roleInfos == nil {
		roleInfos = []models.UserRoleInfo{}
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"id":        userID,
		"username":  username,
		"full_name": fullName,
		"email":     email,
		"dept_code": deptCode,
		"dept_name": deptName,
		"roles":     roleInfos,
	}})
}

// Create godoc
// @Summary      Create user
// @Description  Create a new user account, optionally assigning roles
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateUserRequest true  "User data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /users [post]
func (h *UsersHandler) Create(c *fiber.Ctx) error {
	var req models.CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" || req.FullName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "username, password and full_name required")
	}

	hash, err := auth.HashPassword(req.Password, h.cfg.BcryptCost)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to hash password")
	}

	tx, err := h.db.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	var userID int64
	err = tx.QueryRow(context.Background(), `
		INSERT INTO users (username, password_hash, full_name, email, location_code, employee_code, department, is_active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,true,NOW(),NOW())
		RETURNING id`,
		req.Username, hash, req.FullName, req.Email, req.LocationCode, req.EmployeeCode, req.Department).Scan(&userID)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, "username already exists or constraint error")
	}

	for _, roleCode := range req.RoleCodes {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO user_roles (user_id, role_id)
			SELECT $1, id FROM roles WHERE role_code = $2`, userID, roleCode); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid role_code: "+roleCode)
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "message": "user created", "data": fiber.Map{"id": userID}})
}

// Update godoc
// @Summary      Update user
// @Description  Update user profile information
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                      true  "User ID"
// @Param        body  body  models.UpdateUserRequest true  "User data"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /users/{id} [put]
func (h *UsersHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var req models.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if req.DeptCode != nil {
		var count int
		if err := h.db.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM departments WHERE dept_code=$1`, *req.DeptCode).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "invalid dept_code")
		}
	}
	if req.RoleID != nil {
		var count int
		if err := h.db.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM roles WHERE id=$1`, *req.RoleID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "invalid role_id")
		}
	}

	tx, err := h.db.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	tag, err := tx.Exec(context.Background(), `
		UPDATE users
		SET full_name=$1, email=$2, location_code=$3, employee_code=$4, department=$5, dept_code=COALESCE($6, dept_code), updated_at=NOW()
		WHERE id=$7`,
		req.FullName, req.Email, req.LocationCode, req.EmployeeCode, req.Department, req.DeptCode, id)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, "failed to update user")
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}

	if req.RoleID != nil {
		if _, err := tx.Exec(context.Background(), `DELETE FROM user_roles WHERE user_id=$1`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(),
			`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, id, *req.RoleID); err != nil {
			return err
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "user updated"})
}

// ResetPassword godoc
// @Summary      Reset user password (admin)
// @Description  Admin sets a new password for another user, no old password required
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                          true  "User ID"
// @Param        body  body  models.ResetPasswordRequest  true  "New password"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /users/{id}/password [put]
func (h *UsersHandler) ResetPassword(c *fiber.Ctx) error {
	id := c.Params("id")

	var req models.ResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(req.NewPassword) < 8 {
		return fiber.NewError(fiber.StatusBadRequest, "new password must be at least 8 characters")
	}

	newHash, err := auth.HashPassword(req.NewPassword, h.cfg.BcryptCost)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to hash password")
	}

	tag, err := h.db.Exec(context.Background(),
		`UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`, newHash, id)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, "failed to reset password")
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}

	return c.JSON(fiber.Map{"success": true, "message": "password reset"})
}

// Delete godoc
// @Summary      Delete user
// @Description  Soft delete user by setting is_active = false
// @Tags         Users
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "User ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /users/{id} [delete]
func (h *UsersHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")

	tag, err := h.db.Exec(context.Background(),
		`UPDATE users SET is_active=false, updated_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, "failed to delete user")
	}
	if tag.RowsAffected() == 0 {

		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}

	return c.JSON(fiber.Map{"success": true, "message": "user deactivated"})
}

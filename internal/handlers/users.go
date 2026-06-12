package handlers

import (
	"context"
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
		query = `SELECT id, username, full_name, email, location_code, employee_code, department, is_active, created_at, updated_at
				 FROM users WHERE department = 'บริหาร' AND is_active = true ORDER BY full_name`
	case "requester":
		query = `SELECT id, username, full_name, email, location_code, employee_code, department, is_active, created_at, updated_at
				 FROM users 
				 WHERE (department IN ('วิศวกรรม', 'ฝ่ายจัดซื้อ') OR department IS NULL)
				 AND is_active = true 
				 ORDER BY full_name`
	default:
		query = `SELECT id, username, full_name, email, location_code, employee_code, department, is_active, created_at, updated_at
				 FROM users ORDER BY id`
	}

	rows, err := h.db.Query(context.Background(), query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.FullName, &u.Email, &u.LocationCode,
			&u.EmployeeCode, &u.Department, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return err
		}
		items = append(items, u)
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
		SELECT id, username, full_name, email, location_code, employee_code, department, is_active, created_at, updated_at
		FROM users WHERE id=$1`, id)

	var u models.User
	if err := row.Scan(&u.ID, &u.Username, &u.FullName, &u.Email, &u.LocationCode,
		&u.EmployeeCode, &u.Department, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}

	roles, _ := h.fetchRoles(u.ID)
	u.Roles = roles

	return c.JSON(fiber.Map{"success": true, "data": u})
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
	id := c.Params("id")

	var req models.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	tag, err := h.db.Exec(context.Background(), `
		UPDATE users
		SET full_name=$1, email=$2, location_code=$3, employee_code=$4, department=$5, updated_at=NOW()
		WHERE id=$6`,
		req.FullName, req.Email, req.LocationCode, req.EmployeeCode, req.Department, id)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, "failed to update user")
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}

	return c.JSON(fiber.Map{"success": true, "message": "user updated"})
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

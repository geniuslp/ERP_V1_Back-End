package handlers

import (
	"context"
	"log"
	"strings"

	"erp-api/internal/auth"
	"erp-api/internal/config"
	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthHandler struct {
	db  *pgxpool.Pool
	cfg *config.Config
	svc *auth.Service
}

func NewAuthHandler(db *pgxpool.Pool, cfg *config.Config) *AuthHandler {
	return &AuthHandler{db: db, cfg: cfg, svc: auth.NewService(cfg)}
}

// Login godoc
// @Summary      Login
// @Description  Authenticate and receive JWT token pair
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.LoginRequest  true  "Credentials"
// @Success      200   {object}  auth.TokenPair
// @Failure      400   {object}  fiber.Map
// @Failure      401   {object}  fiber.Map
// @Router       /auth/login [post]
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req models.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		return fiber.NewError(fiber.StatusBadRequest, "username and password required")
	}

	// Fetch user + roles

	row := h.db.QueryRow(context.Background(), `
		SELECT u.id, u.username, u.full_name, u.password_hash, u.is_active
		FROM users u WHERE u.username = $1`, req.Username)

	var user models.User
	if err := row.Scan(&user.ID, &user.Username, &user.FullName, &user.PasswordHash, &user.IsActive); err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	if !user.IsActive {
		return fiber.NewError(fiber.StatusUnauthorized, "account is disabled")
	}

	log.Println("hash from db:", user.PasswordHash)

	result := auth.CheckPassword(user.PasswordHash, req.Password)
	log.Println("hash:", user.PasswordHash)
	log.Println("password:", req.Password)
	log.Println("check result:", result)

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}

	roles, _ := h.fetchRoles(user.ID)
	pair, err := h.svc.GenerateTokenPair(user.ID, user.Username, user.FullName, roles)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(fiber.Map{"success": true, "data": pair})
}

// RefreshToken godoc
// @Summary      Refresh token
// @Description  Get new access token using refresh token
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.RefreshRequest  true  "Refresh token"
// @Success      200   {object}  auth.TokenPair
// @Failure      401   {object}  fiber.Map
// @Router       /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	var req models.RefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	username, err := h.svc.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid refresh token")
	}

	row := h.db.QueryRow(context.Background(), `
		SELECT id, username, full_name, is_active FROM users WHERE username = $1`, username)

	var user models.User
	if err := row.Scan(&user.ID, &user.Username, &user.FullName, &user.IsActive); err != nil || !user.IsActive {
		return fiber.NewError(fiber.StatusUnauthorized, "user not found or inactive")
	}

	roles, _ := h.fetchRoles(user.ID)
	pair, err := h.svc.GenerateTokenPair(user.ID, user.Username, user.FullName, roles)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(fiber.Map{"success": true, "data": pair})
}

// Me godoc
// @Summary      Current user profile
// @Description  Get the authenticated user's profile and roles
// @Tags         Auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  models.User
// @Failure      401  {object}  fiber.Map
// @Router       /auth/me [get]
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)

	// Dev bypass: return fake profile without hitting DB
	if claims.Username == "dev" {
		return c.JSON(fiber.Map{"success": true, "data": models.User{
			ID:       claims.UserID,
			Username: claims.Username,
			FullName: claims.FullName,
			IsActive: true,
			Roles:    claims.Roles,
		}})
	}

	row := h.db.QueryRow(context.Background(), `
		SELECT id, username, full_name, email, location_code, employee_code, department, is_active, created_at, updated_at
		FROM users WHERE id = $1`, claims.UserID)

	var u models.User
	if err := row.Scan(&u.ID, &u.Username, &u.FullName, &u.Email, &u.LocationCode,
		&u.EmployeeCode, &u.Department, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}
	u.Roles = claims.Roles

	return c.JSON(fiber.Map{"success": true, "data": u})
}

// ChangePassword godoc
// @Summary      Change password
// @Description  Change the current user's password
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      models.ChangePasswordRequest  true  "Passwords"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      401   {object}  fiber.Map
// @Router       /auth/change-password [post]
func (h *AuthHandler) ChangePassword(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	var req models.ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(req.NewPassword) < 8 {
		return fiber.NewError(fiber.StatusBadRequest, "new password must be at least 8 characters")
	}

	var hash string
	h.db.QueryRow(context.Background(), `SELECT password_hash FROM users WHERE id = $1`, claims.UserID).Scan(&hash)
	if !auth.CheckPassword(hash, req.OldPassword) {
		return fiber.NewError(fiber.StatusUnauthorized, "old password is incorrect")
	}

	newHash, err := auth.HashPassword(req.NewPassword, h.cfg.BcryptCost)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to hash password")
	}

	h.db.Exec(context.Background(), `UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`, newHash, claims.UserID)
	return c.JSON(fiber.Map{"success": true, "message": "password changed"})
}

func (h *AuthHandler) fetchRoles(userID int64) ([]string, error) {
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

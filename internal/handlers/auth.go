package handlers

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
// @Failure      429   {object}  fiber.Map
// @Router       /auth/change-password [post]
func (h *AuthHandler) ChangePassword(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "no claims found")
	}
	ctx := context.Background()

	var req models.ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
	}

	if pwChangeLimiter.blocked(claims.UserID) {
		return fiber.NewError(fiber.StatusTooManyRequests, "ลองใหม่ภายหลัง หรือติดต่อผู้ดูแลระบบ")
	}

	// NOTE: wrong old password is 400, not 401 — the frontend treats 401 as session expiry.
	var hash string
	if err := h.db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1 AND is_active`, claims.UserID).Scan(&hash); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสผ่านเดิมไม่ถูกต้อง")
	}
	if req.OldPassword == "" || !auth.CheckPassword(hash, req.OldPassword) {
		pwChangeLimiter.fail(claims.UserID)
		return fiber.NewError(fiber.StatusBadRequest, "รหัสผ่านเดิมไม่ถูกต้อง")
	}
	if utf8.RuneCountInString(req.NewPassword) < 8 {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสผ่านใหม่ต้องมีอย่างน้อย 8 ตัวอักษร")
	}
	if req.NewPassword == req.OldPassword {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสผ่านใหม่ต้องไม่ซ้ำกับรหัสผ่านเดิม")
	}

	newHash, err := auth.HashPassword(req.NewPassword, h.cfg.BcryptCost)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถเปลี่ยนรหัสผ่านได้ กรุณาลองใหม่")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถเปลี่ยนรหัสผ่านได้ กรุณาลองใหม่")
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`, newHash, claims.UserID); err != nil {
		log.Printf("change-password: update failed for user %d: %v", claims.UserID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถเปลี่ยนรหัสผ่านได้ กรุณาลองใหม่")
	}
	auditData, _ := json.Marshal(fiber.Map{"event": "CHANGE_PASSWORD", "ip": c.IP()})
	if _, err := tx.Exec(ctx, `
		INSERT INTO erp_audit_log (table_name, record_id, action, changed_by, new_data)
		VALUES ('users',$1,'UPDATE',$1,$2)`, claims.UserID, auditData); err != nil {
		log.Printf("change-password: audit failed for user %d: %v", claims.UserID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถเปลี่ยนรหัสผ่านได้ กรุณาลองใหม่")
	}
	if err := tx.Commit(ctx); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถเปลี่ยนรหัสผ่านได้ กรุณาลองใหม่")
	}

	pwChangeLimiter.reset(claims.UserID)
	return c.JSON(fiber.Map{"success": true, "message": "เปลี่ยนรหัสผ่านสำเร็จ"})
}

// pwChangeLimiter counts wrong-old-password attempts per user (in-memory, per process):
// pwChangeMaxFails failures within pwChangeWindow blocks further attempts until the window expires.
const (
	pwChangeMaxFails = 5
	pwChangeWindow   = 15 * time.Minute
)

type pwAttemptLimiter struct {
	mu    sync.Mutex
	fails map[int64][]time.Time
}

var pwChangeLimiter = &pwAttemptLimiter{fails: map[int64][]time.Time{}}

// prune drops expired entries; caller must hold mu.
func (l *pwAttemptLimiter) prune(uid int64) []time.Time {
	cutoff := time.Now().Add(-pwChangeWindow)
	kept := l.fails[uid][:0]
	for _, t := range l.fails[uid] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.fails, uid)
		return nil
	}
	l.fails[uid] = kept
	return kept
}

func (l *pwAttemptLimiter) blocked(uid int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(uid)) >= pwChangeMaxFails
}

func (l *pwAttemptLimiter) fail(uid int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(uid)
	l.fails[uid] = append(l.fails[uid], time.Now())
}

func (l *pwAttemptLimiter) reset(uid int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, uid)
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

package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"erp-api/internal/config"
	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// How To use: tutorial videos grouped by section (see migrations/052_howto_videos.sql).
//
// Who sees what:
//   - Viewer endpoints need MENU_HOWTO_VIDEOS "read". A section's videos are visible to a user only when the
//     section is active AND (its menu_code is empty OR the user has "read" on that menu — Memo section needs
//     Memo permission, PR section needs PR permission, ... — same 4-layer precedence as RequireMenuPermission).
//   - Manage endpoints use MENU_HOWTO_UPLOAD write/update/delete.
//   - Video bytes are NEVER served by the public /uploads static route (blocked in routes.go); they are streamed
//     through a short-lived HMAC-signed URL because <video> cannot send an Authorization header.

const (
	howToMenuVideos = "MENU_HOWTO_VIDEOS"
	howToMenuUpload = "MENU_HOWTO_UPLOAD"

	howToMaxVideoBytes = 100 << 20 // 100 MB per video (BodyLimit in main.go is 120 MB to leave room for multipart overhead)
	howToStreamTTL     = 15 * time.Minute
)

type HowToHandler struct {
	db     *pgxpool.Pool
	secret []byte
}

func NewHowToHandler(db *pgxpool.Pool, cfg *config.Config) *HowToHandler {
	return &HowToHandler{db: db, secret: []byte(cfg.JWTSecret)}
}

// ── types ────────────────────────────────────────────────────────────────────

type howToVideo struct {
	ID           int64     `json:"id"`
	SectionID    int64     `json:"section_id"`
	Title        string    `json:"title"`
	Description  *string   `json:"description"`
	OriginalName *string   `json:"original_name"`
	MimeType     *string   `json:"mime_type"`
	SizeBytes    *int64    `json:"size_bytes"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
}

type howToSection struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Description *string      `json:"description"`
	MenuCode    *string      `json:"menu_code"`
	MenuName    *string      `json:"menu_name"`
	SortOrder   int          `json:"sort_order"`
	IsActive    bool         `json:"is_active"`
	Videos      []howToVideo `json:"videos"`
}

type howToSectionBody struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	MenuCode    *string `json:"menu_code"`
	SortOrder   *int    `json:"sort_order"`
	IsActive    *bool   `json:"is_active"`
}

type howToVideoBody struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	SortOrder   *int    `json:"sort_order"`
	SectionID   *int64  `json:"section_id"`
}

// ── helpers ──────────────────────────────────────────────────────────────────

func howToSign(secret []byte, videoID, userID, exp int64) string {
	m := hmac.New(sha256.New, secret)
	fmt.Fprintf(m, "howto|%d|%d|%d", videoID, userID, exp)
	return hex.EncodeToString(m.Sum(nil))
}

// howToVerify checks a stream signature. now is injected for tests.
func howToVerify(secret []byte, videoID, userID, exp int64, sig string, now time.Time) bool {
	if now.Unix() > exp {
		return false
	}
	want := howToSign(secret, videoID, userID, exp)
	return hmac.Equal([]byte(want), []byte(sig))
}

// howToVideoKind sniffs the first bytes of an uploaded file and returns its extension and MIME type, or ok=false
// when it is not an MP4 or WebM video (the extension alone is never trusted).
func howToVideoKind(head []byte, filename string) (ext, mime string, ok bool) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4":
		ext = ".mp4"
	case ".webm":
		ext = ".webm"
	default:
		return "", "", false
	}
	sniffed := http.DetectContentType(head)
	switch {
	case ext == ".mp4" && strings.HasPrefix(sniffed, "video/mp4"):
		return ext, "video/mp4", true
	case ext == ".webm" && strings.HasPrefix(sniffed, "video/webm"):
		return ext, "video/webm", true
	}
	return "", "", false
}

func howToDiskPath(rel string) (string, error) {
	root := filepath.Clean(uploadRoot())
	full := filepath.Join(root, filepath.FromSlash(rel))
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", errors.New("path outside upload root")
	}
	return full, nil
}

func howToRandomName() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func howToID(c *fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	return id, nil
}

// howToValidateMenuCode returns nil for an empty value or an existing menu_code.
func (h *HowToHandler) howToValidateMenuCode(ctx context.Context, code *string) (*string, error) {
	if code == nil || strings.TrimSpace(*code) == "" {
		return nil, nil
	}
	v := strings.TrimSpace(*code)
	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM menus WHERE menu_code = $1)`, v).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fiber.NewError(fiber.StatusBadRequest, "menu_code ไม่ถูกต้อง")
	}
	return &v, nil
}

// canWatchMenu reports whether the user may watch sections tied to menuCode: "read" on that menu itself or —
// when menuCode is a GROUP menu (e.g. MENU_MEMO) — "read" on any of its active child menus. A group row normally
// has no permission rows of its own in the Permission Matrix (access is granted per child menu), so checking the
// group alone would hide the section from everyone.
func (h *HowToHandler) canWatchMenu(ctx context.Context, userID int64, menuCode string) bool {
	if middleware.UserCanMenu(ctx, h.db, userID, menuCode, "read") {
		return true
	}
	rows, err := h.db.Query(ctx, `
		SELECT c.menu_code
		FROM menus c JOIN menus p ON p.id = c.parent_id
		WHERE p.menu_code = $1 AND c.is_active = true`, menuCode)
	if err != nil {
		return false
	}
	var children []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err == nil {
			children = append(children, code)
		}
	}
	rows.Close()
	for _, code := range children {
		if middleware.UserCanMenu(ctx, h.db, userID, code, "read") {
			return true
		}
	}
	return false
}

// loadSections reads every section with its videos. When onlyVisibleFor > 0 it keeps only active sections the
// user may watch; otherwise (manage view) it returns everything.
func (h *HowToHandler) loadSections(ctx context.Context, onlyVisibleFor int64) ([]howToSection, error) {
	rows, err := h.db.Query(ctx, `
		SELECT s.id, s.name, s.description, s.menu_code, m.menu_name, s.sort_order, s.is_active
		FROM howto_section s
		LEFT JOIN menus m ON m.menu_code = s.menu_code
		ORDER BY s.sort_order, s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sections := []howToSection{}
	canMenu := map[string]bool{}
	for rows.Next() {
		var s howToSection
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.MenuCode, &s.MenuName, &s.SortOrder, &s.IsActive); err != nil {
			return nil, err
		}
		if onlyVisibleFor > 0 {
			if !s.IsActive {
				continue
			}
			if s.MenuCode != nil {
				allowed, seen := canMenu[*s.MenuCode]
				if !seen {
					allowed = h.canWatchMenu(ctx, onlyVisibleFor, *s.MenuCode)
					canMenu[*s.MenuCode] = allowed
				}
				if !allowed {
					continue
				}
			}
		}
		s.Videos = []howToVideo{}
		sections = append(sections, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	index := make(map[int64]int, len(sections))
	for i := range sections {
		index[sections[i].ID] = i
	}
	vrows, err := h.db.Query(ctx, `
		SELECT id, section_id, title, description, original_name, mime_type, size_bytes, sort_order, created_at
		FROM howto_video
		ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer vrows.Close()
	for vrows.Next() {
		var v howToVideo
		if err := vrows.Scan(&v.ID, &v.SectionID, &v.Title, &v.Description, &v.OriginalName, &v.MimeType, &v.SizeBytes, &v.SortOrder, &v.CreatedAt); err != nil {
			return nil, err
		}
		if i, ok := index[v.SectionID]; ok {
			sections[i].Videos = append(sections[i].Videos, v)
		}
	}
	return sections, vrows.Err()
}

// ── viewer ───────────────────────────────────────────────────────────────────

// ListSections godoc
// @Summary      Sections and videos the current user may watch
// @Description  Active sections whose menu_code is empty or readable by the user (user override > role > department; a group menu counts as readable when any of its child menus is). Needs MENU_HOWTO_VIDEOS read.
// @Tags         HowTo
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /howto/sections [get]
func (h *HowToHandler) ListSections(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "กรุณาเข้าสู่ระบบอีกครั้ง")
	}
	sections, err := h.loadSections(c.Context(), claims.UserID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": sections})
}

// StreamURL godoc
// @Summary      Signed, short-lived URL to play one video
// @Description  Checks the same visibility rule as the list, then returns a URL valid for 15 minutes (HMAC bound to video + user). Managers (MENU_HOWTO_UPLOAD read) may preview any video.
// @Tags         HowTo
// @Security     BearerAuth
// @Produce      json
// @Param        id   path  int  true  "video id"
// @Success      200  {object}  fiber.Map
// @Failure      403  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /howto/videos/{id}/stream-url [get]
func (h *HowToHandler) StreamURL(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "กรุณาเข้าสู่ระบบอีกครั้ง")
	}
	id, err := howToID(c)
	if err != nil {
		return err
	}
	var menuCode *string
	var active bool
	if err := h.db.QueryRow(c.Context(), `
		SELECT s.menu_code, s.is_active
		FROM howto_video v JOIN howto_section s ON s.id = v.section_id
		WHERE v.id = $1`, id).Scan(&menuCode, &active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "ไม่พบวิดีโอ")
		}
		return err
	}
	manager := middleware.UserCanMenu(c.Context(), h.db, claims.UserID, howToMenuUpload, "read")
	if !manager {
		if !active || (menuCode != nil && !h.canWatchMenu(c.Context(), claims.UserID, *menuCode)) {
			return fiber.NewError(fiber.StatusForbidden, "คุณไม่มีสิทธิ์ดูวิดีโอนี้")
		}
	}
	exp := time.Now().Add(howToStreamTTL).Unix()
	sig := howToSign(h.secret, id, claims.UserID, exp)
	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			// Relative to the API base (".../api/v1"): the client prepends its own base URL.
			"url":        fmt.Sprintf("/howto/videos/%d/stream?uid=%d&exp=%d&sig=%s", id, claims.UserID, exp, sig),
			"expires_at": exp,
		},
	})
}

// Stream godoc
// @Summary      Stream a video (signed URL, no Authorization header)
// @Description  Verifies uid/exp/sig from StreamURL, then serves the file with HTTP Range support so the player can seek.
// @Tags         HowTo
// @Produce      octet-stream
// @Param        id   path   int     true  "video id"
// @Param        uid  query  int     true  "user id from the signed URL"
// @Param        exp  query  int     true  "expiry (unix seconds)"
// @Param        sig  query  string  true  "signature"
// @Success      200
// @Failure      403  {object}  fiber.Map
// @Router       /howto/videos/{id}/stream [get]
func (h *HowToHandler) Stream(c *fiber.Ctx) error {
	id, err := howToID(c)
	if err != nil {
		return err
	}
	uid, _ := strconv.ParseInt(c.Query("uid"), 10, 64)
	exp, _ := strconv.ParseInt(c.Query("exp"), 10, 64)
	if !howToVerify(h.secret, id, uid, exp, c.Query("sig"), time.Now()) {
		return fiber.NewError(fiber.StatusForbidden, "ลิงก์หมดอายุหรือไม่ถูกต้อง")
	}
	var rel string
	var mime *string
	if err := h.db.QueryRow(c.Context(), `SELECT file_path, mime_type FROM howto_video WHERE id = $1`, id).Scan(&rel, &mime); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "ไม่พบวิดีโอ")
		}
		return err
	}
	full, err := howToDiskPath(rel)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบไฟล์วิดีโอ")
	}
	if _, err := os.Stat(full); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบไฟล์วิดีโอ")
	}
	if mime != nil && *mime != "" {
		c.Set(fiber.HeaderContentType, *mime)
	}
	c.Set(fiber.HeaderCacheControl, "private, max-age=0, no-store")
	c.Set(fiber.HeaderContentDisposition, "inline")
	return c.SendFile(full, false)
}

// ── manage ───────────────────────────────────────────────────────────────────

// ManageSections godoc
// @Summary      All sections with videos (manage view)
// @Description  Every section including inactive ones, with the linked menu name. Needs MENU_HOWTO_UPLOAD read.
// @Tags         HowTo
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /howto/manage/sections [get]
func (h *HowToHandler) ManageSections(c *fiber.Ctx) error {
	sections, err := h.loadSections(c.Context(), 0)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": sections})
}

// CreateSection godoc
// @Summary      Create a section
// @Tags         HowTo
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Success      201  {object}  fiber.Map
// @Router       /howto/sections [post]
func (h *HowToHandler) CreateSection(c *fiber.Ctx) error {
	var b howToSectionBody
	if err := c.BodyParser(&b); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุชื่อ section")
	}
	menuCode, err := h.howToValidateMenuCode(c.Context(), b.MenuCode)
	if err != nil {
		return err
	}
	sortOrder := 0
	if b.SortOrder != nil {
		sortOrder = *b.SortOrder
	} else {
		_ = h.db.QueryRow(c.Context(), `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM howto_section`).Scan(&sortOrder)
	}
	claims := middleware.GetClaims(c)
	var id int64
	if err := h.db.QueryRow(c.Context(), `
		INSERT INTO howto_section (name, description, menu_code, sort_order, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $5) RETURNING id`,
		b.Name, b.Description, menuCode, sortOrder, claims.UserID).Scan(&id); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id}})
}

// UpdateSection godoc
// @Summary      Update a section
// @Tags         HowTo
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "section id"
// @Success      200  {object}  fiber.Map
// @Router       /howto/sections/{id} [put]
func (h *HowToHandler) UpdateSection(c *fiber.Ctx) error {
	id, err := howToID(c)
	if err != nil {
		return err
	}
	var b howToSectionBody
	if err := c.BodyParser(&b); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุชื่อ section")
	}
	menuCode, err := h.howToValidateMenuCode(c.Context(), b.MenuCode)
	if err != nil {
		return err
	}
	claims := middleware.GetClaims(c)
	tag, err := h.db.Exec(c.Context(), `
		UPDATE howto_section
		SET name = $2, description = $3, menu_code = $4,
		    sort_order = COALESCE($5, sort_order), is_active = COALESCE($6, is_active),
		    updated_at = NOW(), updated_by = $7
		WHERE id = $1`, id, b.Name, b.Description, menuCode, b.SortOrder, b.IsActive, claims.UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบ section")
	}
	return c.JSON(fiber.Map{"success": true})
}

// DeleteSection godoc
// @Summary      Delete a section and all of its videos (files included)
// @Tags         HowTo
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "section id"
// @Success      200  {object}  fiber.Map
// @Router       /howto/sections/{id} [delete]
func (h *HowToHandler) DeleteSection(c *fiber.Ctx) error {
	id, err := howToID(c)
	if err != nil {
		return err
	}
	rows, err := h.db.Query(c.Context(), `SELECT file_path FROM howto_video WHERE section_id = $1`, id)
	if err != nil {
		return err
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, p)
	}
	rows.Close()

	tag, err := h.db.Exec(c.Context(), `DELETE FROM howto_section WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบ section")
	}
	for _, p := range paths {
		if full, err := howToDiskPath(p); err == nil {
			_ = os.Remove(full)
		}
	}
	return c.JSON(fiber.Map{"success": true})
}

// UploadVideo godoc
// @Summary      Upload a video into a section
// @Description  multipart: file (.mp4 or .webm, content-sniffed, max 100 MB), title (defaults to the file name), description, sort_order. Stored outside the public /uploads route.
// @Tags         HowTo
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        id     path      int     true   "section id"
// @Param        file   formData  file    true   "video"
// @Param        title  formData  string  false  "title"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /howto/sections/{id}/videos [post]
func (h *HowToHandler) UploadVideo(c *fiber.Ctx) error {
	sectionID, err := howToID(c)
	if err != nil {
		return err
	}
	var exists bool
	if err := h.db.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM howto_section WHERE id = $1)`, sectionID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบ section")
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกไฟล์วิดีโอ")
	}
	if fh.Size <= 0 || fh.Size > howToMaxVideoBytes {
		return fiber.NewError(fiber.StatusBadRequest, "ไฟล์วิดีโอต้องมีขนาดไม่เกิน 100MB")
	}
	src, err := fh.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "อ่านไฟล์ไม่สำเร็จ")
	}
	head := make([]byte, 512)
	n, _ := src.Read(head)
	src.Close()
	ext, mime, ok := howToVideoKind(head[:n], fh.Filename)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "รองรับเฉพาะไฟล์วิดีโอ MP4 และ WebM")
	}

	rel := fmt.Sprintf("howto/%d/%s%s", sectionID, howToRandomName(), ext)
	full, err := howToDiskPath(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create upload directory")
	}
	if err := c.SaveFile(fh, full); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save file")
	}

	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" {
		title = strings.TrimSuffix(fh.Filename, filepath.Ext(fh.Filename))
	}
	var desc *string
	if d := strings.TrimSpace(c.FormValue("description")); d != "" {
		desc = &d
	}
	sortOrder := 0
	if v := strings.TrimSpace(c.FormValue("sort_order")); v != "" {
		sortOrder, _ = strconv.Atoi(v)
	} else {
		_ = h.db.QueryRow(c.Context(), `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM howto_video WHERE section_id = $1`, sectionID).Scan(&sortOrder)
	}
	claims := middleware.GetClaims(c)
	var v howToVideo
	err = h.db.QueryRow(c.Context(), `
		INSERT INTO howto_video (section_id, title, description, file_path, original_name, mime_type, size_bytes, sort_order, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, section_id, title, description, original_name, mime_type, size_bytes, sort_order, created_at`,
		sectionID, title, desc, rel, fh.Filename, mime, fh.Size, sortOrder, claims.UserID,
	).Scan(&v.ID, &v.SectionID, &v.Title, &v.Description, &v.OriginalName, &v.MimeType, &v.SizeBytes, &v.SortOrder, &v.CreatedAt)
	if err != nil {
		_ = os.Remove(full) // never leave an orphan file behind
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": v})
}

// UpdateVideo godoc
// @Summary      Edit a video's title/description/order, or move it to another section
// @Tags         HowTo
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "video id"
// @Success      200  {object}  fiber.Map
// @Router       /howto/videos/{id} [put]
func (h *HowToHandler) UpdateVideo(c *fiber.Ctx) error {
	id, err := howToID(c)
	if err != nil {
		return err
	}
	var b howToVideoBody
	if err := c.BodyParser(&b); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if b.Title != nil {
		t := strings.TrimSpace(*b.Title)
		if t == "" {
			return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุชื่อวิดีโอ")
		}
		b.Title = &t
	}
	tag, err := h.db.Exec(c.Context(), `
		UPDATE howto_video
		SET title = COALESCE($2, title), description = COALESCE($3, description),
		    sort_order = COALESCE($4, sort_order), section_id = COALESCE($5, section_id)
		WHERE id = $1`, id, b.Title, b.Description, b.SortOrder, b.SectionID)
	if err != nil {
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "23503" {
			return fiber.NewError(fiber.StatusBadRequest, "ไม่พบ section ปลายทาง")
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบวิดีโอ")
	}
	return c.JSON(fiber.Map{"success": true})
}

// DeleteVideo godoc
// @Summary      Delete a video (row and file)
// @Tags         HowTo
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "video id"
// @Success      200  {object}  fiber.Map
// @Router       /howto/videos/{id} [delete]
func (h *HowToHandler) DeleteVideo(c *fiber.Ctx) error {
	id, err := howToID(c)
	if err != nil {
		return err
	}
	var rel string
	if err := h.db.QueryRow(c.Context(), `DELETE FROM howto_video WHERE id = $1 RETURNING file_path`, id).Scan(&rel); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "ไม่พบวิดีโอ")
		}
		return err
	}
	if full, err := howToDiskPath(rel); err == nil {
		_ = os.Remove(full)
	}
	return c.JSON(fiber.Map{"success": true})
}

package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

var allowedMemoUploadExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".xlsx": true, ".xls": true, ".csv": true, ".pdf": true,
}

const maxMemoUploadSize = 10 * 1024 * 1024 // 10MB

type AttachmentHandler struct {
	db *pgxpool.Pool
}

func NewAttachmentHandler(db *pgxpool.Pool) *AttachmentHandler {
	return &AttachmentHandler{db: db}
}

// UploadPRFile godoc
// @Summary      Upload a file for PR attachment
// @Description  Upload multipart file; returns stored path to use when saving the attachment record
// @Tags         Attachments
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        file  formData  file  true  "File to upload"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /upload/pr [post]
func (h *AttachmentHandler) UploadPRFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file field is required")
	}

	now := time.Now()
	dir := filepath.Join("uploads", "pr", now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create upload directory")
	}

	saveName := fmt.Sprintf("%d_%s", now.UnixMilli(), filepath.Base(file.Filename))
	savePath := filepath.Join(dir, saveName)
	if err := c.SaveFile(file, savePath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save file")
	}

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": models.UploadFileResponse{
			FilePath: toAbsoluteFileURL(filepath.ToSlash(savePath)),
			FileName: file.Filename,
			FileSize: file.Size,
			FileType: contentType,
		},
	})
}

// UploadMemoFile godoc
// @Summary      Upload a file for Memo attachment
// @Description  Upload multipart file; returns stored path to use when saving the memo attachment record
// @Tags         Attachments
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        file  formData  file  true  "File to upload"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /upload/memo [post]
func (h *AttachmentHandler) UploadMemoFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file field is required")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedMemoUploadExts[ext] {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("file type %s not allowed", ext))
	}
	if file.Size > maxMemoUploadSize {
		return fiber.NewError(fiber.StatusBadRequest, "file size exceeds 10MB limit")
	}

	now := time.Now()
	dir := filepath.Join("uploads", "memo", now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create upload directory")
	}

	saveName := fmt.Sprintf("%d_%s", now.UnixMilli(), filepath.Base(file.Filename))
	savePath := filepath.Join(dir, saveName)
	if err := c.SaveFile(file, savePath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save file")
	}

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": models.UploadFileResponse{
			FilePath: toAbsoluteFileURL(filepath.ToSlash(savePath)),
			FileName: file.Filename,
			FileSize: file.Size,
			FileType: contentType,
		},
	})
}

// AddAttachment godoc
// @Summary      Upload and attach a file to a PR in one step
// @Description  Accepts the file directly (multipart) — writes it to disk, then inserts the
// @Description  attachment record only if the write succeeded. Replaces the old two-step
// @Description  upload-then-link flow for existing PRs (POST /upload/pr is still used for the
// @Description  create-PR-with-attachments flow, where no PR id exists yet).
// @Tags         Attachments
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        id    path      int   true  "PR ID"
// @Param        file  formData  file  true  "File to upload"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /pr/{id}/attachments [post]
func (h *AttachmentHandler) Add(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)

	prID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid pr_id")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file field is required")
	}

	now := time.Now()
	dir := filepath.Join("uploads", "pr", now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create upload directory")
	}
	saveName := fmt.Sprintf("%d_%s", now.UnixMilli(), filepath.Base(file.Filename))
	savePath := filepath.Join(dir, saveName)
	if err := c.SaveFile(file, savePath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save file")
	}
	relPath := filepath.ToSlash(savePath)

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var id int64
	err = h.db.QueryRow(context.Background(), `
		INSERT INTO pr_attachment (pr_id, file_name, file_path, file_size, file_type, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		prID, file.Filename, relPath, file.Size, contentType, claims.UserID,
	).Scan(&id)
	if err != nil {
		// Insert failed — don't leave an orphaned file with no DB row referencing it.
		os.Remove(savePath)
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"id":        id,
			"file_path": toAbsoluteFileURL(relPath),
		},
	})
}

// ListAttachments godoc
// @Summary      List attachments for a PR
// @Tags         Attachments
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PR ID"
// @Success      200  {array}   models.PRAttachment
// @Failure      400  {object}  fiber.Map
// @Router       /pr/{id}/attachments [get]
func (h *AttachmentHandler) List(c *fiber.Ctx) error {
	prID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid pr_id")
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT id, pr_id, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at
		FROM pr_attachment
		WHERE pr_id = $1
		ORDER BY uploaded_at ASC`, prID)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := make([]models.PRAttachment, 0)
	for rows.Next() {
		var a models.PRAttachment
		if err := rows.Scan(
			&a.ID, &a.PRID, &a.FileName, &a.FilePath,
			&a.FileSize, &a.FileType, &a.UploadedBy, &a.UploadedAt,
		); err != nil {
			return err
		}
		a.FilePath = toAbsoluteFileURL(a.FilePath)
		items = append(items, a)
	}

	return c.JSON(fiber.Map{"success": true, "data": items})
}

// AddPOAttachment godoc
// @Summary      Upload and attach a file to a PO in one step
// @Description  Accepts the file directly (multipart) — writes it to disk, then inserts the
// @Description  attachment record only if the write succeeded. Replaces the old two-step
// @Description  upload-then-link flow (POST /upload/po has been removed — nothing else used it).
// @Tags         Attachments
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        id    path      int   true  "PO ID"
// @Param        file  formData  file  true  "File to upload"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /po/{id}/attachments [post]
func (h *AttachmentHandler) AddPO(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)

	poID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid po_id")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file field is required")
	}

	now := time.Now()
	dir := filepath.Join("uploads", "po", now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create upload directory")
	}
	saveName := fmt.Sprintf("%d_%s", now.UnixMilli(), filepath.Base(file.Filename))
	savePath := filepath.Join(dir, saveName)
	if err := c.SaveFile(file, savePath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save file")
	}
	relPath := filepath.ToSlash(savePath)

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var id int64
	err = h.db.QueryRow(context.Background(), `
		INSERT INTO po_attachment (po_id, file_name, file_path, file_size, file_type, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		poID, file.Filename, relPath, file.Size, contentType, claims.UserID,
	).Scan(&id)
	if err != nil {
		os.Remove(savePath)
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"id":        id,
			"file_path": toAbsoluteFileURL(relPath),
		},
	})
}

// ListPOAttachments godoc
// @Summary      List attachments for a PO
// @Tags         Attachments
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {array}   models.POAttachment
// @Failure      400  {object}  fiber.Map
// @Router       /po/{id}/attachments [get]
func (h *AttachmentHandler) ListPO(c *fiber.Ctx) error {
	poID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid po_id")
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT id, po_id, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at
		FROM po_attachment
		WHERE po_id = $1
		ORDER BY uploaded_at ASC`, poID)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := make([]models.POAttachment, 0)
	for rows.Next() {
		var a models.POAttachment
		if err := rows.Scan(
			&a.ID, &a.POID, &a.FileName, &a.FilePath,
			&a.FileSize, &a.FileType, &a.UploadedBy, &a.UploadedAt,
		); err != nil {
			return err
		}
		a.FilePath = toAbsoluteFileURL(a.FilePath)
		items = append(items, a)
	}

	return c.JSON(fiber.Map{"success": true, "data": items})
}

// DeletePOAttachment godoc
// @Summary      Delete a PO attachment
// @Description  Removes the database record and deletes the file from disk
// @Tags         Attachments
// @Security     BearerAuth
// @Produce      json
// @Param        id         path  int  true  "PO ID"
// @Param        attach_id  path  int  true  "Attachment ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/{id}/attachments/{attach_id} [delete]
func (h *AttachmentHandler) DeletePO(c *fiber.Ctx) error {
	poID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid po_id")
	}
	attachID, err := strconv.ParseInt(c.Params("attach_id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid attachment id")
	}

	var filePath string
	err = h.db.QueryRow(context.Background(),
		`SELECT file_path FROM po_attachment WHERE id = $1 AND po_id = $2`,
		attachID, poID,
	).Scan(&filePath)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "attachment not found")
	}

	if _, err := h.db.Exec(context.Background(),
		`DELETE FROM po_attachment WHERE id = $1`, attachID); err != nil {
		return err
	}

	// Best-effort removal — don't fail the request if file is already gone
	os.Remove(filePath)

	return c.JSON(fiber.Map{"success": true})
}

// DeleteAttachment godoc
// @Summary      Delete a PR attachment
// @Description  Removes the database record and deletes the file from disk
// @Tags         Attachments
// @Security     BearerAuth
// @Produce      json
// @Param        id         path  int  true  "PR ID"
// @Param        attach_id  path  int  true  "Attachment ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /pr/{id}/attachments/{attach_id} [delete]
func (h *AttachmentHandler) Delete(c *fiber.Ctx) error {
	prID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid pr_id")
	}
	attachID, err := strconv.ParseInt(c.Params("attach_id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid attachment id")
	}

	var filePath string
	err = h.db.QueryRow(context.Background(),
		`SELECT file_path FROM pr_attachment WHERE id = $1 AND pr_id = $2`,
		attachID, prID,
	).Scan(&filePath)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "attachment not found")
	}

	if _, err := h.db.Exec(context.Background(),
		`DELETE FROM pr_attachment WHERE id = $1`, attachID); err != nil {
		return err
	}

	// Best-effort removal — don't fail the request if file is already gone
	os.Remove(filePath)

	return c.JSON(fiber.Map{"success": true})
}

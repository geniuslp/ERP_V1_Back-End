package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
			FilePath: filepath.ToSlash(savePath),
			FileName: file.Filename,
			FileSize: file.Size,
			FileType: contentType,
		},
	})
}

// AddAttachment godoc
// @Summary      Add attachment record to PR
// @Description  Link an already-uploaded file to a purchase request
// @Tags         Attachments
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                          true  "PR ID"
// @Param        body  body  models.AddAttachmentRequest  true  "Attachment metadata"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /pr/{id}/attachments [post]
func (h *AttachmentHandler) Add(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)

	prID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid pr_id")
	}

	var req models.AddAttachmentRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.FilePath == "" || req.FileName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "file_path and file_name are required")
	}

	var id int64
	err = h.db.QueryRow(context.Background(), `
		INSERT INTO pr_attachment (pr_id, file_name, file_path, file_size, file_type, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		prID, req.FileName, req.FilePath, req.FileSize, req.FileType, claims.UserID,
	).Scan(&id)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"id": id},
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
		items = append(items, a)
	}

	return c.JSON(fiber.Map{"success": true, "data": items})
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

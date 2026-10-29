package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

// Only PDF and JPEG are accepted for the contractor log's attachments.
var woContractorLogAttachmentTypes = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
}

const maxWOContractorLogAttachmentSize = 10 * 1024 * 1024 // 10MB

type woContractorLogAttachment struct {
	ID         int64      `json:"id"`
	LogID      int64      `json:"log_id"`
	FileName   string     `json:"file_name"`
	FilePath   string     `json:"file_path"`
	FileSize   *int64     `json:"file_size"`
	FileType   *string    `json:"file_type"`
	UploadedBy *int64     `json:"uploaded_by"`
	UploadedAt *time.Time `json:"uploaded_at"`
}

func (h *WOContractorLogHandler) listAttachments(ctx context.Context, logID int64) ([]woContractorLogAttachment, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, log_id, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at
		FROM wo_contractor_log_attachment WHERE log_id = $1 ORDER BY uploaded_at, id`, logID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []woContractorLogAttachment{}
	for rows.Next() {
		var a woContractorLogAttachment
		if err := rows.Scan(&a.ID, &a.LogID, &a.FileName, &a.FilePath, &a.FileSize, &a.FileType, &a.UploadedBy, &a.UploadedAt); err != nil {
			return nil, err
		}
		a.FilePath = toAbsoluteFileURL(a.FilePath)
		items = append(items, a)
	}
	return items, rows.Err()
}

// ListAttachments godoc
// @Summary      List the files attached to a work order contractor log
// @Tags         WO Contractor Log
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "wo_contractor_log.id"
// @Success      200  {object}  fiber.Map
// @Router       /wo-contractor-log/{id}/attachments [get]
func (h *WOContractorLogHandler) ListAttachments(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสเอกสารไม่ถูกต้อง")
	}
	items, err := h.listAttachments(context.Background(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดไฟล์แนบได้")
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// AddAttachment godoc
// @Summary      Attach a PDF / JPEG file to a work order contractor log
// @Description  Multipart field "file", max 10 MB, .pdf / .jpg / .jpeg only.
// @Tags         WO Contractor Log
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        id    path      int   true  "wo_contractor_log.id"
// @Param        file  formData  file  true  "PDF or JPEG file"
// @Success      201  {object}  fiber.Map
// @Router       /wo-contractor-log/{id}/attachments [post]
func (h *WOContractorLogHandler) AddAttachment(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสเอกสารไม่ถูกต้อง")
	}
	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM wo_contractor_log WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกไฟล์แนบได้")
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบเอกสาร")
	}

	file, err := c.FormFile("file")
	if err != nil || file == nil {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกไฟล์")
	}
	if file.Size > maxWOContractorLogAttachmentSize {
		return fiber.NewError(fiber.StatusBadRequest, "ไฟล์ต้องมีขนาดไม่เกิน 10 MB")
	}
	// Display name only - never used for the on-disk path.
	displayName := strings.TrimSpace(filepath.Base(strings.ReplaceAll(file.Filename, "\\", "/")))
	ext := strings.ToLower(filepath.Ext(displayName))
	fileType, ok := woContractorLogAttachmentTypes[ext]
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "รองรับเฉพาะไฟล์ PDF หรือ JPEG")
	}
	if !utf8.ValidString(displayName) {
		return fiber.NewError(fiber.StatusBadRequest, "ชื่อไฟล์มีอักขระที่ไม่รองรับ กรุณาเปลี่ยนชื่อไฟล์")
	}
	if displayName == "" || utf8.RuneCountInString(displayName) > 255 {
		return fiber.NewError(fiber.StatusBadRequest, "ชื่อไฟล์ยาวเกินไป (ไม่เกิน 255 ตัวอักษร)")
	}

	dir := uploadDiskDir("wo-contractor-log", strconv.FormatInt(id, 10))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกไฟล์ได้")
	}
	rnd := make([]byte, 6)
	if _, err := rand.Read(rnd); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกไฟล์ได้")
	}
	savePath := filepath.Join(dir, fmt.Sprintf("%d_%s%s", time.Now().UnixMilli(), hex.EncodeToString(rnd), ext))
	if err := c.SaveFile(file, savePath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกไฟล์ได้")
	}
	relPath := uploadURLPath(savePath)

	var attID int64
	if err := h.db.QueryRow(ctx, `
		INSERT INTO wo_contractor_log_attachment (log_id, file_name, file_path, file_size, file_type, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		id, displayName, relPath, file.Size, fileType, claims.UserID,
	).Scan(&attID); err != nil {
		os.Remove(savePath)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกไฟล์แนบได้")
	}

	items, err := h.listAttachments(ctx, id)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดไฟล์แนบได้")
	}
	for _, a := range items {
		if a.ID == attID {
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": a})
		}
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": fiber.Map{"id": attID}})
}

// DeleteAttachment godoc
// @Summary      Delete a file attached to a work order contractor log
// @Tags         WO Contractor Log
// @Security     BearerAuth
// @Produce      json
// @Param        id     path  int  true  "wo_contractor_log.id"
// @Param        attId  path  int  true  "attachment id"
// @Success      200  {object}  fiber.Map
// @Router       /wo-contractor-log/{id}/attachments/{attId} [delete]
func (h *WOContractorLogHandler) DeleteAttachment(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสเอกสารไม่ถูกต้อง")
	}
	attID, err := strconv.ParseInt(c.Params("attId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสไฟล์แนบไม่ถูกต้อง")
	}
	var filePath string
	if err := h.db.QueryRow(context.Background(),
		`DELETE FROM wo_contractor_log_attachment WHERE id = $1 AND log_id = $2 RETURNING file_path`, attID, id,
	).Scan(&filePath); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบไฟล์แนบ")
	}
	removeUploadFile(filePath) // best effort
	return c.JSON(fiber.Map{"success": true})
}

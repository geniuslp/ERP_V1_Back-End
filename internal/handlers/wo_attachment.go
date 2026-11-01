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

// Files attached to a work order itself (same rules as the contractor log's: PDF / JPEG, 10 MB).
type woAttachment struct {
	ID         int64      `json:"id"`
	WoID       int64      `json:"wo_id"`
	FileName   string     `json:"file_name"`
	FilePath   string     `json:"file_path"`
	FileSize   *int64     `json:"file_size"`
	FileType   *string    `json:"file_type"`
	UploadedBy *int64     `json:"uploaded_by"`
	UploadedAt *time.Time `json:"uploaded_at"`
}

func (h *WorkOrderHandler) listAttachments(ctx context.Context, woID int64) ([]woAttachment, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, wo_id, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at
		FROM work_order_attachment WHERE wo_id = $1 ORDER BY uploaded_at, id`, woID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []woAttachment{}
	for rows.Next() {
		var a woAttachment
		if err := rows.Scan(&a.ID, &a.WoID, &a.FileName, &a.FilePath, &a.FileSize, &a.FileType, &a.UploadedBy, &a.UploadedAt); err != nil {
			return nil, err
		}
		a.FilePath = toAbsoluteFileURL(a.FilePath)
		items = append(items, a)
	}
	return items, rows.Err()
}

// ListAttachments godoc
// @Summary      List the files attached to a work order
// @Tags         WorkOrder
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "work_order.id"
// @Success      200  {object}  fiber.Map
// @Router       /work-order/{id}/attachments [get]
func (h *WorkOrderHandler) ListAttachments(c *fiber.Ctx) error {
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
// @Summary      Attach a PDF / JPEG file to a work order
// @Description  Multipart field "file", max 10 MB, .pdf / .jpg / .jpeg only. Not allowed on a CANCELLED work order.
// @Tags         WorkOrder
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        id    path      int   true  "work_order.id"
// @Param        file  formData  file  true  "PDF or JPEG file"
// @Success      201  {object}  fiber.Map
// @Router       /work-order/{id}/attachments [post]
func (h *WorkOrderHandler) AddAttachment(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสเอกสารไม่ถูกต้อง")
	}
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	var status string
	if err := h.db.QueryRow(ctx, `SELECT status FROM work_order WHERE id = $1`, id).Scan(&status); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบหนังสือสั่งจ้าง")
	}
	if status == "CANCELLED" {
		return fiber.NewError(fiber.StatusBadRequest, "หนังสือสั่งจ้างที่ยกเลิกแล้วแนบไฟล์ไม่ได้")
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

	dir := uploadDiskDir("work-order", strconv.FormatInt(id, 10))
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
		INSERT INTO work_order_attachment (wo_id, file_name, file_path, file_size, file_type, uploaded_by)
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
// @Summary      Delete a file attached to a work order
// @Tags         WorkOrder
// @Security     BearerAuth
// @Produce      json
// @Param        id     path  int  true  "work_order.id"
// @Param        attId  path  int  true  "attachment id"
// @Success      200  {object}  fiber.Map
// @Router       /work-order/{id}/attachments/{attId} [delete]
func (h *WorkOrderHandler) DeleteAttachment(c *fiber.Ctx) error {
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
		`DELETE FROM work_order_attachment WHERE id = $1 AND wo_id = $2 RETURNING file_path`, attID, id,
	).Scan(&filePath); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบไฟล์แนบ")
	}
	removeUploadFile(filePath) // best effort
	return c.JSON(fiber.Map{"success": true})
}

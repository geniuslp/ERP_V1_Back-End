package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

const maxPettyCashAttachmentSize = 10 * 1024 * 1024 // 10 MB

// Extension -> content type. The stored file_type comes from here, never from the client header.
var pettyCashAttachmentTypes = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

// pcAttachmentEditable mirrors the petty cash Update rule: only DRAFT or REJECTED documents are editable.
func pcAttachmentEditable(status string) bool {
	return status == "DRAFT" || status == "REJECTED"
}

const pcAttachmentNotEditableMsg = "ไม่สามารถแนบไฟล์ได้ เนื่องจากเอกสารไม่ได้อยู่ในสถานะร่าง"

// listAttachments returns the attachments of a petty cash document (absolute file_path URLs).
func (h *PettyCashHandler) listAttachments(ctx context.Context, pcID int64) ([]models.PettyCashAttachment, error) {
	rows, err := h.db.Query(ctx, `
		SELECT a.id, a.file_name, a.file_path, a.file_size, a.file_type, a.uploaded_at, a.uploaded_by, u.full_name
		FROM petty_cash_attachment a
		LEFT JOIN users u ON u.id = a.uploaded_by
		WHERE a.pc_id = $1
		ORDER BY a.uploaded_at ASC, a.id ASC`, pcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.PettyCashAttachment{}
	for rows.Next() {
		var a models.PettyCashAttachment
		if err := rows.Scan(&a.ID, &a.FileName, &a.FilePath, &a.FileSize, &a.FileType,
			&a.UploadedAt, &a.UploadedBy, &a.UploadedByName); err != nil {
			return nil, err
		}
		a.FilePath = toAbsoluteFileURL(a.FilePath)
		items = append(items, a)
	}
	return items, rows.Err()
}

// ListAttachments godoc
// @Summary      List attachments of a petty cash requisition
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Petty cash requisition ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /petty-cash/{id}/attachments [get]
func (h *PettyCashHandler) ListAttachments(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสเอกสารไม่ถูกต้อง")
	}
	ctx := context.Background()
	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM petty_cash_requisition WHERE id=$1)`, id).Scan(&exists); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดไฟล์แนบได้")
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบเอกสาร")
	}
	items, err := h.listAttachments(ctx, id)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดไฟล์แนบได้")
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// AddAttachment godoc
// @Summary      Upload an attachment straight to a petty cash requisition
// @Description  multipart/form-data, field "file". Only while the document is DRAFT (or REJECTED, same rule as Update). Max 10 MB; pdf, jpg, jpeg, png, doc, docx, xls, xlsx. The file is written to uploads/petty-cash/{id}/ under a generated name, then the row is inserted; if the insert fails the file is removed. file_path in the response is an absolute URL.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       mpfd
// @Produce      json
// @Param        id    path      int   true  "Petty cash requisition ID"
// @Param        file  formData  file  true  "Attachment file"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /petty-cash/{id}/attachments [post]
func (h *PettyCashHandler) AddAttachment(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสเอกสารไม่ถูกต้อง")
	}
	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var status string
	if err := h.db.QueryRow(ctx, `SELECT status FROM petty_cash_requisition WHERE id=$1`, id).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "ไม่พบเอกสาร")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกไฟล์แนบได้")
	}
	if !pcAttachmentEditable(status) {
		return fiber.NewError(fiber.StatusConflict, pcAttachmentNotEditableMsg)
	}

	file, err := c.FormFile("file")
	if err != nil || file == nil {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกไฟล์")
	}
	if file.Size > maxPettyCashAttachmentSize {
		return fiber.NewError(fiber.StatusBadRequest, "ไฟล์ต้องมีขนาดไม่เกิน 10 MB")
	}
	// Display name only - never used for the on-disk path.
	displayName := strings.TrimSpace(filepath.Base(strings.ReplaceAll(file.Filename, "\\", "/")))
	ext := strings.ToLower(filepath.Ext(displayName))
	fileType, ok := pettyCashAttachmentTypes[ext]
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "ชนิดไฟล์ไม่รองรับ")
	}
	if !utf8.ValidString(displayName) {
		return fiber.NewError(fiber.StatusBadRequest, "ชื่อไฟล์มีอักขระที่ไม่รองรับ กรุณาเปลี่ยนชื่อไฟล์")
	}
	if displayName == "" || utf8.RuneCountInString(displayName) > 255 {
		return fiber.NewError(fiber.StatusBadRequest, "ชื่อไฟล์ยาวเกินไป (ไม่เกิน 255 ตัวอักษร)")
	}

	dir := uploadDiskDir("petty-cash", strconv.FormatInt(id, 10))
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

	// Insert only while the document is still editable (guards a status change after the check above).
	var attID int64
	err = h.db.QueryRow(ctx, `
		INSERT INTO petty_cash_attachment (pc_id, file_name, file_path, file_size, file_type, uploaded_by)
		SELECT pc.id, $2::varchar, $3::text, $4::bigint, $5::varchar, $6::bigint
		FROM petty_cash_requisition pc
		WHERE pc.id = $1 AND pc.status IN ('DRAFT','REJECTED')
		RETURNING id`,
		id, displayName, relPath, file.Size, fileType, claims.UserID,
	).Scan(&attID)
	if err != nil {
		os.Remove(savePath)
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusConflict, pcAttachmentNotEditableMsg)
		}
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
// @Summary      Delete a petty cash attachment
// @Description  Same DRAFT/REJECTED-only rule as upload. Removes the row and the file on disk (a missing file is ignored). The attachment must belong to the document.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        id     path  int  true  "Petty cash requisition ID"
// @Param        attId  path  int  true  "Attachment ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Router       /petty-cash/{id}/attachments/{attId} [delete]
func (h *PettyCashHandler) DeleteAttachment(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสเอกสารไม่ถูกต้อง")
	}
	attID, err := strconv.ParseInt(c.Params("attId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสไฟล์แนบไม่ถูกต้อง")
	}
	ctx := context.Background()

	var status string
	if err := h.db.QueryRow(ctx, `SELECT status FROM petty_cash_requisition WHERE id=$1`, id).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "ไม่พบเอกสาร")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถลบไฟล์แนบได้")
	}
	if !pcAttachmentEditable(status) {
		return fiber.NewError(fiber.StatusConflict, "ไม่สามารถลบไฟล์แนบได้ เนื่องจากเอกสารไม่ได้อยู่ในสถานะร่าง")
	}

	var filePath string
	if err := h.db.QueryRow(ctx,
		`DELETE FROM petty_cash_attachment WHERE id=$1 AND pc_id=$2 RETURNING file_path`, attID, id,
	).Scan(&filePath); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "ไม่พบไฟล์แนบ")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถลบไฟล์แนบได้")
	}
	removeUploadFile(filePath) // best effort - a missing file is fine
	return c.JSON(fiber.Map{"success": true})
}

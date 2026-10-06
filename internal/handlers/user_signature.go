package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

const maxSignatureUploadSize = 1 * 1024 * 1024 // 1MB

// derefStr returns "" for a nil pointer instead of panicking — used when scanning a nullable
// signature_path/signature_mime column straight into loadSignatureDataURL.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// loadSignatureDataURL reads a signature file from disk and returns it as a base64 data URL
// ("data:<mime>;base64,...."). Returns "" (never an error) when path is empty, mime is empty,
// or the file is missing/unreadable on disk — callers must treat "" as "no signature", not fail
// the whole request. Shared by GetSignature (raw bytes) and every detail endpoint that embeds a
// signature inline (PR/PO/Memo) so the disk-reading logic lives in exactly one place.
func loadSignatureDataURL(path, mime string) string {
	if path == "" || mime == "" {
		return ""
	}
	data, err := readUploadFile(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
}

var allowedSignatureMimes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
}

// UploadSignature godoc
// @Summary      Upload/replace a user's signature image
// @Description  multipart field "file", png/jpg only (sniffed by content, not just extension), max 1MB. Replaces and deletes any previous signature file for this user.
// @Tags         Users
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        id    path  int   true  "User ID"
// @Param        file  formData  file  true  "Signature image (png/jpg, max 1MB)"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /users/{id}/signature [post]
func (h *UsersHandler) UploadSignature(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	ctx := context.Background()

	var oldPath *string
	if err := h.db.QueryRow(ctx, `SELECT signature_path FROM users WHERE id=$1`, id).Scan(&oldPath); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file field is required")
	}
	if file.Size > maxSignatureUploadSize {
		return fiber.NewError(fiber.StatusBadRequest, "file too large — max 1MB")
	}

	fh, err := file.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "failed to read uploaded file")
	}
	defer fh.Close()

	head := make([]byte, 512)
	n, _ := fh.Read(head)
	sniffed := http.DetectContentType(head[:n])

	ext, ok := allowedSignatureMimes[sniffed]
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("only PNG/JPEG images are allowed (detected: %s)", sniffed))
	}

	dir := uploadDiskDir("signatures")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create upload directory")
	}

	saveName := fmt.Sprintf("user_%d_%d%s", id, time.Now().UnixMilli(), ext)
	savePath := filepath.Join(dir, saveName)
	if err := c.SaveFile(file, savePath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save file")
	}
	relPath := uploadURLPath(savePath)

	if _, err := h.db.Exec(ctx, `
		UPDATE users SET signature_path=$1, signature_mime=$2, signature_updated_at=NOW() WHERE id=$3`,
		relPath, sniffed, id,
	); err != nil {
		os.Remove(savePath)
		return err
	}

	// Delete the previous file only after the DB row points at the new one, so a failed
	// save/update above never leaves the user with no signature file at all.
	if oldPath != nil && *oldPath != "" {
		removeUploadFile(*oldPath)
	}

	return c.JSON(fiber.Map{"success": true, "message": "signature uploaded"})
}

// GetSignature godoc
// @Summary      Get a user's signature image
// @Description  Returns the raw image bytes with the correct Content-Type. 404 (plain message) if the user has no signature or the file is missing on disk — never 500.
// @Tags         Users
// @Security     BearerAuth
// @Produce      image/png
// @Produce      image/jpeg
// @Param        id  path  int  true  "User ID"
// @Success      200  {file}    binary
// @Failure      404  {object}  fiber.Map
// @Router       /users/{id}/signature [get]
func (h *UsersHandler) GetSignature(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var path, mime *string
	if err := h.db.QueryRow(context.Background(),
		`SELECT signature_path, signature_mime FROM users WHERE id=$1`, id,
	).Scan(&path, &mime); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}
	if path == nil || *path == "" {
		return fiber.NewError(fiber.StatusNotFound, "user has no signature")
	}

	data, err := readUploadFile(*path)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "signature file is missing on disk")
	}

	contentType := "application/octet-stream"
	if mime != nil && *mime != "" {
		contentType = *mime
	}
	c.Set(fiber.HeaderContentType, contentType)
	return c.Send(data)
}

// DeleteSignature godoc
// @Summary      Delete a user's signature image
// @Description  Removes the file from disk (ignored if already missing) and nulls signature_path/signature_mime/signature_updated_at.
// @Tags         Users
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "User ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /users/{id}/signature [delete]
func (h *UsersHandler) DeleteSignature(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	ctx := context.Background()

	var path *string
	if err := h.db.QueryRow(ctx, `SELECT signature_path FROM users WHERE id=$1`, id).Scan(&path); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}

	if _, err := h.db.Exec(ctx, `
		UPDATE users SET signature_path=NULL, signature_mime=NULL, signature_updated_at=NULL WHERE id=$1`, id,
	); err != nil {
		return err
	}

	if path != nil && *path != "" {
		removeUploadFile(*path) // ignore error if already missing
	}

	return c.JSON(fiber.Map{"success": true, "message": "signature deleted"})
}

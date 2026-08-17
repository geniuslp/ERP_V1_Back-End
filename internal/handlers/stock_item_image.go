package handlers

import (
	"context"
	"fmt"
	"math/rand"
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

var allowedStockImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
}

type StockItemImageHandler struct {
	db *pgxpool.Pool
}

func NewStockItemImageHandler(db *pgxpool.Pool) *StockItemImageHandler {
	return &StockItemImageHandler{db: db}
}

func stockItemExists(ctx context.Context, db *pgxpool.Pool, itemID int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM stock_item WHERE id = $1)`, itemID).Scan(&exists)
	return exists, err
}

// UploadImages godoc
// @Summary      Upload images for a stock item
// @Description  Upload one or more images (multipart field "files"); auto-marks the first as primary if none exists yet
// @Tags         Stock
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        id     path      int    true  "Item ID"
// @Param        files  formData  file   true  "Image files"
// @Success      201    {object}  fiber.Map
// @Failure      400    {object}  fiber.Map
// @Failure      404    {object}  fiber.Map
// @Router       /stock/items/{id}/images [post]
func (h *StockItemImageHandler) UploadImages(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)

	itemID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid item id")
	}

	ctx := context.Background()
	exists, err := stockItemExists(ctx, h.db, itemID)
	if err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "stock item not found")
	}

	form, err := c.MultipartForm()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid multipart form")
	}
	files := form.File["files"]
	if len(files) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "files field is required")
	}

	for _, fh := range files {
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if !allowedStockImageExts[ext] {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("file type %s not allowed", ext))
		}
	}

	var hasPrimary bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM stock_item_image WHERE item_id = $1 AND is_primary = true)`,
		itemID).Scan(&hasPrimary); err != nil {
		return err
	}

	dir := filepath.Join("uploads", "stock", strconv.FormatInt(itemID, 10))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create upload directory")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	uploaded := make([]models.StockItemImage, 0, len(files))
	for idx, fh := range files {
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		saveName := fmt.Sprintf("%d_%d%s", time.Now().UnixNano(), rand.Intn(1_000_000), ext)
		savePath := filepath.Join(dir, saveName)
		if err := c.SaveFile(fh, savePath); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to save file")
		}
		filePath := filepath.ToSlash(savePath)

		isPrimary := !hasPrimary && idx == 0
		fileName := fh.Filename
		img := models.StockItemImage{
			ItemID:    itemID,
			FilePath:  filePath,
			FileName:  &fileName,
			IsPrimary: isPrimary,
			SortOrder: idx,
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO stock_item_image (item_id, file_path, file_name, is_primary, sort_order, created_by)
			VALUES ($1,$2,$3,$4,$5,$6)
			RETURNING id, created_at`,
			itemID, filePath, fh.Filename, isPrimary, idx, claims.UserID,
		).Scan(&img.ID, &img.CreatedAt); err != nil {
			return err
		}
		uploaded = append(uploaded, img)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": uploaded})
}

// ListImages godoc
// @Summary      List images for a stock item
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Item ID"
// @Success      200  {object}  fiber.Map
// @Router       /stock/items/{id}/images [get]
func (h *StockItemImageHandler) ListImages(c *fiber.Ctx) error {
	itemID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid item id")
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT id, item_id, file_path, file_name, is_primary, sort_order, created_at
		FROM stock_item_image
		WHERE item_id = $1
		ORDER BY is_primary DESC, sort_order ASC, id ASC`, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()

	images := make([]models.StockItemImage, 0)
	for rows.Next() {
		var img models.StockItemImage
		if err := rows.Scan(&img.ID, &img.ItemID, &img.FilePath, &img.FileName, &img.IsPrimary, &img.SortOrder, &img.CreatedAt); err != nil {
			return err
		}
		images = append(images, img)
	}

	return c.JSON(fiber.Map{"success": true, "data": images})
}

// SetPrimary godoc
// @Summary      Set an image as the primary/thumbnail image
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        id        path  int  true  "Item ID"
// @Param        image_id  path  int  true  "Image ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /stock/items/{id}/images/{image_id}/primary [put]
func (h *StockItemImageHandler) SetPrimary(c *fiber.Ctx) error {
	itemID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid item id")
	}
	imageID, err := strconv.ParseInt(c.Params("image_id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid image id")
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`UPDATE stock_item_image SET is_primary = false WHERE item_id = $1 AND is_primary = true`, itemID)
	if err != nil {
		return err
	}
	_ = tag

	res, err := tx.Exec(ctx,
		`UPDATE stock_item_image SET is_primary = true WHERE id = $1 AND item_id = $2`, imageID, itemID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "image not found")
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true})
}

// DeleteImage godoc
// @Summary      Delete a stock item image
// @Description  Removes the DB record and the file on disk (best-effort); promotes the next image to primary if the deleted one was primary
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        id        path  int  true  "Item ID"
// @Param        image_id  path  int  true  "Image ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /stock/items/{id}/images/{image_id} [delete]
func (h *StockItemImageHandler) DeleteImage(c *fiber.Ctx) error {
	itemID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid item id")
	}
	imageID, err := strconv.ParseInt(c.Params("image_id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid image id")
	}

	ctx := context.Background()

	var filePath string
	var wasPrimary bool
	err = h.db.QueryRow(ctx,
		`SELECT file_path, is_primary FROM stock_item_image WHERE id = $1 AND item_id = $2`,
		imageID, itemID,
	).Scan(&filePath, &wasPrimary)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "image not found")
	}

	if _, err := h.db.Exec(ctx, `DELETE FROM stock_item_image WHERE id = $1`, imageID); err != nil {
		return err
	}

	os.Remove(filePath)

	if wasPrimary {
		var nextID int64
		err := h.db.QueryRow(ctx, `
			SELECT id FROM stock_item_image
			WHERE item_id = $1
			ORDER BY sort_order ASC, id ASC
			LIMIT 1`, itemID).Scan(&nextID)
		if err == nil {
			if _, err := h.db.Exec(ctx,
				`UPDATE stock_item_image SET is_primary = true WHERE id = $1`, nextID); err != nil {
				return err
			}
		}
	}

	return c.JSON(fiber.Map{"success": true})
}

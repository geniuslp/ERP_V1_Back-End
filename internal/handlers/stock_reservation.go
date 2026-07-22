package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

type StockReservationHandler struct{ db *pgxpool.Pool }

func NewStockReservationHandler(db *pgxpool.Pool) *StockReservationHandler {
	return &StockReservationHandler{db: db}
}

func generateReservationNo(ctx context.Context, db *pgxpool.Pool) (string, error) {
	var seq int64
	if err := db.QueryRow(ctx, "SELECT nextval('stock_reservation_seq')").Scan(&seq); err != nil {
		return "", err
	}
	return fmt.Sprintf("RES-%s-%04d", timeYYMM(), seq), nil
}

// List godoc
// @Summary      รายการ Stock Reservation
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        status    query  string  false  "Status"
// @Param        item_id   query  int     false  "Item ID"
// @Param        date_from query  string  false  "Date from"
// @Param        date_to   query  string  false  "Date to"
// @Param        page      query  int     false  "Page"
// @Param        page_size query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /stock/reservations [get]
func (h *StockReservationHandler) List(c *fiber.Ctx) error {
	var f models.StockReservationFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}

	ctx := context.Background()
	where := []string{"1=1"}
	args := []interface{}{}
	i := 1

	if f.Status != "" {
		where = append(where, fmt.Sprintf("r.status = $%d", i))
		args = append(args, f.Status)
		i++
	}
	if f.ItemID > 0 {
		where = append(where, fmt.Sprintf("r.item_id = $%d", i))
		args = append(args, f.ItemID)
		i++
	}
	if f.DateFrom != "" {
		where = append(where, fmt.Sprintf("r.created_at >= $%d", i))
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where = append(where, fmt.Sprintf("r.created_at <= $%d", i))
		args = append(args, f.DateTo)
		i++
	}

	whereClause := strings.Join(where, " AND ")

	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM stock_reservation r
		JOIN stock_item si ON si.id = r.item_id
		LEFT JOIN location l ON l.location_code = r.location_code
		JOIN users u ON u.id = r.requested_by
		WHERE %s`, whereClause), countArgs...).Scan(&total); err != nil {
		return err
	}

	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT r.id, r.reservation_no, r.status,
		       r.item_id, si.mat_code, si.item_name,
		       r.location_code, COALESCE(l.location_name,'') AS location_name,
		       r.qty_reserved, r.qty_fulfilled,
		       u.full_name AS requested_by,
		       TO_CHAR(r.needed_by,'YYYY-MM-DD'),
		       r.purpose, r.created_at
		FROM stock_reservation r
		JOIN stock_item si ON si.id = r.item_id
		LEFT JOIN location l ON l.location_code = r.location_code
		JOIN users u ON u.id = r.requested_by
		WHERE %s
		ORDER BY r.created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var list []models.StockReservation
	for rows.Next() {
		var r models.StockReservation
		if err := rows.Scan(
			&r.ID, &r.ReservationNo, &r.Status,
			&r.ItemID, &r.MatCode, &r.ItemName,
			&r.LocationCode, &r.LocationName,
			&r.QtyReserved, &r.QtyFulfilled,
			&r.RequestedBy, &r.NeededBy,
			&r.Purpose, &r.CreatedAt,
		); err != nil {
			return err
		}
		list = append(list, r)
	}
	if list == nil {
		list = []models.StockReservation{}
	}

	totalPages := int(total) / f.PageSize
	if int(total)%f.PageSize != 0 {
		totalPages++
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: list, Total: total, Page: f.Page, PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// Create godoc
// @Summary      สร้าง Stock Reservation
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateReservationRequest  true  "Request body"
// @Success      201  {object}  fiber.Map
// @Router       /stock/reservations [post]
func (h *StockReservationHandler) Create(c *fiber.Ctx) error {
	var req models.CreateReservationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.QtyReserved <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "qty_reserved must be positive")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var itemExists bool
	h.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM stock_item WHERE id=$1 AND is_active=true)", req.ItemID).Scan(&itemExists)
	if !itemExists {
		return fiber.NewError(fiber.StatusBadRequest, "item not found")
	}

	var qtyAvailable float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(qty_on_hand - qty_reserved, 0)
		FROM stock_inventory
		WHERE item_id=$1 AND location_code=$2`, req.ItemID, req.LocationCode).Scan(&qtyAvailable); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "item not found in specified location")
	}
	if qtyAvailable < req.QtyReserved {
		return fiber.NewError(fiber.StatusBadRequest, "insufficient available stock")
	}

	reservationNo, err := generateReservationNo(ctx, h.db)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var resID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO stock_reservation (reservation_no, item_id, location_code, qty_reserved, requested_by, needed_by, purpose, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'PENDING')
		RETURNING id`,
		reservationNo, req.ItemID, req.LocationCode, req.QtyReserved, claims.UserID, req.NeededBy, req.Purpose,
	).Scan(&resID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE stock_inventory SET qty_reserved = qty_reserved + $1, updated_at=NOW()
		WHERE item_id=$2 AND location_code=$3`, req.QtyReserved, req.ItemID, req.LocationCode)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"id": resID, "reservation_no": reservationNo},
	})
}

// Cancel godoc
// @Summary      ยกเลิก Stock Reservation
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Reservation ID"
// @Success      200  {object}  fiber.Map
// @Router       /stock/reservations/{id}/cancel [post]
func (h *StockReservationHandler) Cancel(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var status string
	var itemID int64
	var locationCode string
	var qtyReserved float64
	if err := h.db.QueryRow(ctx, `
		SELECT status, item_id, location_code, qty_reserved
		FROM stock_reservation WHERE id=$1`, id).Scan(&status, &itemID, &locationCode, &qtyReserved); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "reservation not found")
	}
	if status == "CANCELLED" || status == "FULFILLED" {
		return fiber.NewError(fiber.StatusBadRequest, "reservation cannot be cancelled")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `UPDATE stock_reservation SET status='CANCELLED', updated_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE stock_inventory SET qty_reserved = GREATEST(qty_reserved - $1, 0), updated_at=NOW()
		WHERE item_id=$2 AND location_code=$3`, qtyReserved, itemID, locationCode)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true})
}

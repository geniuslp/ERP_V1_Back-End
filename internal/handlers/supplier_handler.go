package handlers

import (
	"context"
	"fmt"
	"strconv"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SupplierHandler struct {
	db *pgxpool.Pool
}

func NewSupplierHandler(db *pgxpool.Pool) *SupplierHandler {
	return &SupplierHandler{db: db}
}

const supplierCols = `id, supplier_name, supplier_short_name, tax_id, address,
	contact_name, contact_phone, contact_email, office_phone, fax, sales_person, currency, payment_terms,
	is_active, created_at, updated_at, created_by, updated_by`

func scanSupplier(s *models.SupplierFull, row interface {
	Scan(dest ...any) error
}) error {
	return row.Scan(&s.Id, &s.SupplierName, &s.SupplierShortName, &s.TaxID, &s.Address,
		&s.ContactName, &s.ContactPhone, &s.ContactEmail, &s.OfficePhone, &s.Fax, &s.SalesPerson, &s.Currency, &s.PaymentTerms,
		&s.IsActive, &s.CreatedAt, &s.UpdatedAt, &s.CreatedBy, &s.UpdatedBy)
}

// ListSuppliers godoc
// @Summary      List all suppliers
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        q  query  string  false  "search by name"
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /master/suppliers [get]
func (h *SupplierHandler) ListSuppliers(c *fiber.Ctx) error {
	q := "%" + c.Query("q") + "%"
	rows, err := h.db.Query(context.Background(),
		`SELECT `+supplierCols+`
		FROM supplier
		WHERE is_active = true AND supplier_name ILIKE $1
		ORDER BY id ASC`, q)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.SupplierFull
	for rows.Next() {
		var s models.SupplierFull
		if err := scanSupplier(&s, rows); err != nil {
			return err
		}
		items = append(items, s)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetSupplier godoc
// @Summary      Get supplier by id
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Supplier ID"
// @Success      200 {object}  models.SupplierFull
// @Failure      404 {object}  fiber.Map
// @Router       /master/suppliers/{id} [get]
func (h *SupplierHandler) GetSupplier(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid supplier id")
	}

	var s models.SupplierFull
	row := h.db.QueryRow(context.Background(),
		`SELECT `+supplierCols+` FROM supplier WHERE id = $1`, id)
	if err := scanSupplier(&s, row); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "supplier not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": s})
}

// CreateSupplier godoc
// @Summary      Create supplier
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateSupplierReq  true  "Supplier data"
// @Success      201   {object}  models.SupplierFull
// @Failure      400   {object}  fiber.Map
// @Router       /master/suppliers [post]
func (h *SupplierHandler) CreateSupplier(c *fiber.Ctx) error {
	var req models.CreateSupplierReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.SupplierName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "supplier_name is required")
	}

	claims := middleware.GetClaims(c)

	var s models.SupplierFull
	row := h.db.QueryRow(context.Background(),
		`INSERT INTO supplier (supplier_name, tax_id, address,
		contact_name, contact_phone, contact_email, payment_terms, is_active, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
		RETURNING `+supplierCols,
		req.SupplierName, req.TaxID, req.Address,
		req.ContactName, req.ContactPhone, req.ContactEmail, req.PaymentTerms, req.IsActive, claims.UserID)
	if err := scanSupplier(&s, row); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": s})
}

// UpdateSupplier godoc
// @Summary      Update supplier
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                      true  "Supplier ID"
// @Param        body  body  models.UpdateSupplierReq  true  "Update data"
// @Success      200   {object}  models.SupplierFull
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /master/suppliers/{id} [put]
func (h *SupplierHandler) UpdateSupplier(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid supplier id")
	}

	var req models.UpdateSupplierReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.SupplierName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "supplier_name is required")
	}

	claims := middleware.GetClaims(c)

	var s models.SupplierFull
	row := h.db.QueryRow(context.Background(),
		`UPDATE supplier SET supplier_name=$1, supplier_short_name=$2, tax_id=$3, address=$4,
		contact_name=$5, contact_phone=$6, contact_email=$7, office_phone=$8, fax=$9,
		sales_person=$10, currency=$11, payment_terms=$12,
		updated_at=NOW(), updated_by=$13
		WHERE id=$14 AND is_active=true
		RETURNING `+supplierCols,
		req.SupplierName, req.SupplierShortName, req.TaxID, req.Address,
		req.ContactName, req.ContactPhone, req.ContactEmail, req.OfficePhone, req.Fax,
		req.SalesPerson, req.Currency, req.PaymentTerms,
		claims.UserID, id)
	if err := scanSupplier(&s, row); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "supplier not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": s})
}

// DeleteSupplier godoc
// @Summary      Soft-delete supplier
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Supplier ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/suppliers/{id} [delete]
func (h *SupplierHandler) DeleteSupplier(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid supplier id")
	}
	claims := middleware.GetClaims(c)

	tag, execErr := h.db.Exec(context.Background(),
		`UPDATE supplier SET is_active=false, updated_at=NOW(), updated_by=$2
		WHERE id=$1 AND is_active=true`,
		id, claims.UserID)
	if execErr != nil {
		return execErr
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "supplier not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "supplier deleted"})
}

// BulkCreateSupplier godoc
// @Summary      Bulk create suppliers
// @Description  Inserts multiple suppliers where supplier_name is the only required field per item — every other field is optional and unvalidated. There is no supplier_code anymore; each row gets its id from the DB's auto-increment PK, returned per row in the response.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      object  true  "{ \"items\": [ { \"supplier_name\": \"...\" } ] }"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /master/suppliers/bulk [post]
func (h *SupplierHandler) BulkCreateSupplier(c *fiber.Ctx) error {
	type Item struct {
		SupplierName string  `json:"supplier_name"`
		TaxID        *string `json:"tax_id"`
		Address      *string `json:"address"`
		ContactName  *string `json:"contact_name"`
		ContactPhone *string `json:"contact_phone"`
		ContactEmail *string `json:"contact_email"`
		PaymentTerms *string `json:"payment_terms"`
	}
	var body struct {
		Items []Item `json:"items"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(body.Items) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "items is empty")
	}
	for i, item := range body.Items {
		if item.SupplierName == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("item[%d]: supplier_name is required", i))
		}
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	results := make([]models.BulkInsertSupplierResultLine, 0, len(body.Items))
	for i, item := range body.Items {
		var inserted models.BulkInsertSupplierResultLine
		if err := tx.QueryRow(ctx, `
			INSERT INTO supplier
				(supplier_name, tax_id, address,
				 contact_name, contact_phone, contact_email, payment_terms,
				 created_by, updated_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
			RETURNING id, supplier_name`,
			item.SupplierName, item.TaxID, item.Address,
			item.ContactName, item.ContactPhone, item.ContactEmail, item.PaymentTerms,
			claims.UserID,
		).Scan(&inserted.SupplierID, &inserted.SupplierName); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("item[%d]: %s", i, err.Error()))
		}
		results = append(results, inserted)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data": models.BulkInsertSupplierResponse{
			Count:     len(results),
			Suppliers: results,
		},
	})
}

// BulkInsertSupplier godoc
// @Summary      Bulk insert suppliers
// @Description  Inserts a batch of suppliers where supplier_name is the only required field — every other field is optional and unvalidated. There is no supplier_code anymore; each row gets its id from the DB's auto-increment PK, returned per row in the response. No duplicate checking is performed on supplier_name — the same name can be inserted more than once, each as its own row.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.BulkInsertSupplierRequest  true  "Suppliers to insert"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /supplier/bulk [post]
func (h *SupplierHandler) BulkInsertSupplier(c *fiber.Ctx) error {
	var req models.BulkInsertSupplierRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(req.Suppliers) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "suppliers is empty")
	}
	for i, s := range req.Suppliers {
		if s.SupplierName == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("suppliers[%d]: supplier_name is required", i))
		}
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	results := make([]models.BulkInsertSupplierResultLine, 0, len(req.Suppliers))
	for i, s := range req.Suppliers {
		// DB CHECK constraint supplier_currency_check only allows 'BH'/'US' — despite
		// database.md documenting a 'THB' default, that value would violate the live
		// constraint. 'BH' matches every existing supplier row and the real schema.
		currency := "BH"
		if s.Currency != nil && *s.Currency != "" {
			currency = *s.Currency
		}

		var inserted models.BulkInsertSupplierResultLine
		if err := tx.QueryRow(ctx, `
			INSERT INTO supplier
				(supplier_name, supplier_short_name, tax_id, address,
				 contact_name, contact_phone, contact_email, office_phone, fax,
				 payment_terms, currency, sales_person, is_active, created_by, updated_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,true,$13,$13)
			RETURNING id, supplier_name`,
			s.SupplierName, s.SupplierShortName, s.TaxID, s.Address,
			s.ContactName, s.ContactPhone, s.ContactEmail, s.OfficePhone, s.Fax,
			s.PaymentTerms, currency, s.SalesPerson, claims.UserID,
		).Scan(&inserted.SupplierID, &inserted.SupplierName); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("suppliers[%d]: %s", i, err.Error()))
		}
		results = append(results, inserted)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data": models.BulkInsertSupplierResponse{
			Count:     len(results),
			Suppliers: results,
		},
	})
}

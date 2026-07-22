package handlers

import (
	"context"
	"fmt"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SupplierHandler struct {
	db *pgxpool.Pool
}

func NewSupplierHandler(db *pgxpool.Pool) *SupplierHandler {
	return &SupplierHandler{db: db}
}

const supplierCols = `id, supplier_code, supplier_name, supplier_short_name, tax_id, address,
	contact_name, contact_phone, contact_email, office_phone, fax, sales_person, currency, payment_terms,
	is_active, created_at, updated_at, created_by, updated_by`

func scanSupplier(s *models.SupplierFull, row interface {
	Scan(dest ...any) error
}) error {
	return row.Scan(&s.Id, &s.SupplierCode, &s.SupplierName, &s.SupplierShortName, &s.TaxID, &s.Address,
		&s.ContactName, &s.ContactPhone, &s.ContactEmail, &s.OfficePhone, &s.Fax, &s.SalesPerson, &s.Currency, &s.PaymentTerms,
		&s.IsActive, &s.CreatedAt, &s.UpdatedAt, &s.CreatedBy, &s.UpdatedBy)
}

// ListSuppliers godoc
// @Summary      List all suppliers
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        q  query  string  false  "search by name or code"
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /master/suppliers [get]
func (h *SupplierHandler) ListSuppliers(c *fiber.Ctx) error {
	q := "%" + c.Query("q") + "%"
	rows, err := h.db.Query(context.Background(),
		`SELECT `+supplierCols+`
		FROM supplier
		WHERE is_active = true AND (supplier_name ILIKE $1 OR supplier_code ILIKE $1)
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
// @Summary      Get supplier by code
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "Supplier Code"
// @Success      200   {object}  models.SupplierFull
// @Failure      404   {object}  fiber.Map
// @Router       /master/suppliers/{code} [get]
func (h *SupplierHandler) GetSupplier(c *fiber.Ctx) error {
	code := c.Params("code")

	var s models.SupplierFull
	row := h.db.QueryRow(context.Background(),
		`SELECT `+supplierCols+` FROM supplier WHERE supplier_code = $1`, code)
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
// @Failure      409   {object}  fiber.Map
// @Router       /master/suppliers [post]
func (h *SupplierHandler) CreateSupplier(c *fiber.Ctx) error {
	var req models.CreateSupplierReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.SupplierCode == "" || req.SupplierName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "supplier_code and supplier_name are required")
	}

	claims := middleware.GetClaims(c)

	var s models.SupplierFull
	row := h.db.QueryRow(context.Background(),
		`INSERT INTO supplier (supplier_code, supplier_name, tax_id, address,
		contact_name, contact_phone, contact_email, payment_terms, is_active, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
		RETURNING `+supplierCols,
		req.SupplierCode, req.SupplierName, req.TaxID, req.Address,
		req.ContactName, req.ContactPhone, req.ContactEmail, req.PaymentTerms, req.IsActive, claims.UserID)
	if err := scanSupplier(&s, row); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusConflict, "supplier_code already exists")
		}
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
// @Param        code  path  string                   true  "Supplier Code"
// @Param        body  body  models.UpdateSupplierReq  true  "Update data"
// @Success      200   {object}  models.SupplierFull
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /master/suppliers/{code} [put]
func (h *SupplierHandler) UpdateSupplier(c *fiber.Ctx) error {
	code := c.Params("code")

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
		WHERE supplier_code=$14 AND is_active=true
		RETURNING `+supplierCols,
		req.SupplierName, req.SupplierShortName, req.TaxID, req.Address,
		req.ContactName, req.ContactPhone, req.ContactEmail, req.OfficePhone, req.Fax,
		req.SalesPerson, req.Currency, req.PaymentTerms,
		claims.UserID, code)
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
// @Param        code  path  string  true  "Supplier Code"
// @Success      200   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /master/suppliers/{code} [delete]
func (h *SupplierHandler) DeleteSupplier(c *fiber.Ctx) error {
	code := c.Params("code")
	claims := middleware.GetClaims(c)

	tag, err := h.db.Exec(context.Background(),
		`UPDATE supplier SET is_active=false, updated_at=NOW(), updated_by=$2
		WHERE supplier_code=$1 AND is_active=true`,
		code, claims.UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "supplier not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "supplier deleted"})
}

// BulkCreateSupplier godoc
// @Summary      Bulk create suppliers
// @Description  Inserts multiple suppliers in a single UNNEST statement. is_active defaults to true at DB level.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      object  true  "{ \"items\": [ { \"supplier_code\": \"SUP001\", \"supplier_name\": \"...\" } ] }"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /master/suppliers/bulk [post]
func (h *SupplierHandler) BulkCreateSupplier(c *fiber.Ctx) error {
	type Item struct {
		SupplierCode string  `json:"supplier_code"`
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

	codes := make([]string, 0, len(body.Items))
	names := make([]string, 0, len(body.Items))
	taxIDs := make([]*string, 0, len(body.Items))
	addresses := make([]*string, 0, len(body.Items))
	contactNames := make([]*string, 0, len(body.Items))
	contactPhones := make([]*string, 0, len(body.Items))
	contactEmails := make([]*string, 0, len(body.Items))
	paymentTermsList := make([]*string, 0, len(body.Items))

	for i, item := range body.Items {
		if item.SupplierCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("item[%d]: supplier_code is required", i))
		}
		if item.SupplierName == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("item[%d]: supplier_name is required", i))
		}
		codes = append(codes, item.SupplierCode)
		names = append(names, item.SupplierName)
		taxIDs = append(taxIDs, item.TaxID)
		addresses = append(addresses, item.Address)
		contactNames = append(contactNames, item.ContactName)
		contactPhones = append(contactPhones, item.ContactPhone)
		contactEmails = append(contactEmails, item.ContactEmail)
		paymentTermsList = append(paymentTermsList, item.PaymentTerms)
	}

	claims := middleware.GetClaims(c)

	tag, err := h.db.Exec(context.Background(), `
		INSERT INTO supplier
			(supplier_code, supplier_name, tax_id, address,
			 contact_name, contact_phone, contact_email, payment_terms,
			 created_by, updated_by)
		SELECT UNNEST($1::text[]), UNNEST($2::text[]), UNNEST($3::text[]), UNNEST($4::text[]),
		       UNNEST($5::text[]), UNNEST($6::text[]), UNNEST($7::text[]), UNNEST($8::text[]),
		       $9, $9
		ON CONFLICT (supplier_code) DO NOTHING`,
		codes, names, taxIDs, addresses,
		contactNames, contactPhones, contactEmails, paymentTermsList,
		claims.UserID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	imported := int(tag.RowsAffected())
	duplicates := len(body.Items) - imported

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success":    true,
		"imported":   imported,
		"duplicates": duplicates,
	})
}

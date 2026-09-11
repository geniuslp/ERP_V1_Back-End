package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SupplierHandler struct {
	db *pgxpool.Pool
}

func NewSupplierHandler(db *pgxpool.Pool) *SupplierHandler {
	return &SupplierHandler{db: db}
}

const supplierCols = `id, supplier_name, tax_id, address,
	contact_name, contact_phone, contact_email, office_phone, fax, sales_person, sales_person_phone, currency, payment_terms, remarks,
	is_active, created_at, updated_at, created_by, updated_by`

func scanSupplier(s *models.SupplierFull, row interface {
	Scan(dest ...any) error
}) error {
	return row.Scan(&s.Id, &s.SupplierName, &s.TaxID, &s.Address,
		&s.ContactName, &s.ContactPhone, &s.ContactEmail, &s.OfficePhone, &s.Fax, &s.SalesPerson, &s.SalesPersonPhone, &s.Currency, &s.PaymentTerms, &s.Remarks,
		&s.IsActive, &s.CreatedAt, &s.UpdatedAt, &s.CreatedBy, &s.UpdatedBy)
}

// ListSuppliers godoc
// @Summary      List all suppliers
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        supplier_name  query  string  false  "search by supplier name"
// @Param        q              query  string  false  "search by supplier name (alias of supplier_name)"
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /master/suppliers [get]
func (h *SupplierHandler) ListSuppliers(c *fiber.Ctx) error {
	name := c.Query("supplier_name")
	if name == "" {
		name = c.Query("q")
	}

	where := "is_active = true"
	args := []interface{}{}
	if name != "" {
		where += " AND supplier_name ILIKE $1"
		args = append(args, "%"+name+"%")
	}

	var total int64
	if err := h.db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM supplier WHERE `+where, args...).Scan(&total); err != nil {
		return err
	}

	rows, err := h.db.Query(context.Background(),
		`SELECT `+supplierCols+`
		FROM supplier
		WHERE `+where+`
		ORDER BY id ASC`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []models.SupplierFull{}
	for rows.Next() {
		var s models.SupplierFull
		if err := scanSupplier(&s, rows); err != nil {
			return err
		}
		items = append(items, s)
	}
	return c.JSON(fiber.Map{"success": true, "data": items, "total": total})
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
// @Description  remarks is optional free text, unvalidated.
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
		contact_name, contact_phone, contact_email, sales_person, sales_person_phone, payment_terms, remarks, is_active, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
		RETURNING `+supplierCols,
		req.SupplierName, req.TaxID, req.Address,
		req.ContactName, req.ContactPhone, req.ContactEmail, req.SalesPerson, req.SalesPersonPhone, req.PaymentTerms, req.Remarks, req.IsActive, claims.UserID)
	if err := scanSupplier(&s, row); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": s})
}

// UpdateSupplier godoc
// @Summary      Update supplier
// @Description  remarks is optional free text, unvalidated.
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
		`UPDATE supplier SET supplier_name=$1, tax_id=$2, address=$3,
		contact_name=$4, contact_phone=$5, contact_email=$6, office_phone=$7, fax=$8,
		sales_person=$9, sales_person_phone=$10, currency=$11, payment_terms=$12, remarks=$13,
		updated_at=NOW(), updated_by=$14
		WHERE id=$15 AND is_active=true
		RETURNING `+supplierCols,
		req.SupplierName, req.TaxID, req.Address,
		req.ContactName, req.ContactPhone, req.ContactEmail, req.OfficePhone, req.Fax,
		req.SalesPerson, req.SalesPersonPhone, req.Currency, req.PaymentTerms, req.Remarks,
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
	ctx := context.Background()

	var inUse bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM purchase_order WHERE supplier_id = $1
			UNION ALL
			SELECT 1 FROM rfq WHERE supplier_id = $1
		)`, id).Scan(&inUse); err != nil {
		return err
	}
	if inUse {
		return fiber.NewError(fiber.StatusConflict, "supplier is referenced by existing purchase orders/RFQs and cannot be deleted")
	}

	tag, execErr := h.db.Exec(ctx,
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
	// Field mapping — sales_person/sales_person_phone/office_phone are separate DB columns
	// from the DB's own literal contact_name/contact_phone (those are left untouched by
	// this import path). Wire keys match DB column names directly (confirmed against the
	// live frontend payload — a prior version of these tags assumed the Excel header names
	// were the wire keys, which was wrong and left these three fields always nil).
	type Item struct {
		SupplierName     string  `json:"supplier_name"`
		TaxID            *string `json:"tax_id"`
		Address          *string `json:"address"`
		SalesPerson      *string `json:"sales_person"`
		SalesPersonPhone *string `json:"sales_person_phone"`
		OfficePhone      *string `json:"office_phone"`
		ContactEmail     *string `json:"contact_email"`
		PaymentTerms     *string `json:"payment_terms"`
		Currency         *string `json:"currency"`
		Remarks          *string `json:"remarks"`
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
	maxLen := func(i int, field, value string, max int) error {
		if n := utf8.RuneCountInString(value); n > max {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf(
				"item[%d]: %s exceeds max length %d (got %d)", i, field, max, n))
		}
		return nil
	}
	for i, item := range body.Items {
		if item.SupplierName == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("item[%d]: supplier_name is required", i))
		}
		if err := maxLen(i, "supplier_name", item.SupplierName, 200); err != nil {
			return err
		}
		if item.TaxID != nil {
			if err := maxLen(i, "tax_id", *item.TaxID, 50); err != nil {
				return err
			}
		}
		// sales_person (DB) has no length limit — matches BulkInsertSupplier's SalesPerson,
		// which is likewise unchecked (see that handler's own validation loop below).
		if item.SalesPersonPhone != nil {
			if err := maxLen(i, "sales_person_phone", *item.SalesPersonPhone, 50); err != nil {
				return err
			}
		}
		if item.OfficePhone != nil {
			if err := maxLen(i, "office_phone", *item.OfficePhone, 50); err != nil {
				return err
			}
		}
		if item.ContactEmail != nil {
			if err := maxLen(i, "contact_email", *item.ContactEmail, 200); err != nil {
				return err
			}
		}
		if item.PaymentTerms != nil {
			if err := maxLen(i, "payment_terms", *item.PaymentTerms, 100); err != nil {
				return err
			}
		}
		if item.Currency != nil {
			if err := maxLen(i, "currency", *item.Currency, 10); err != nil {
				return err
			}
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
	var importedCount, duplicateCount int
	for i, item := range body.Items {
		// DB CHECK constraint supplier_currency_check only allows 'BH'/'US' — same default
		// as BulkInsertSupplier below, kept in sync so both endpoints behave identically.
		currency := "BH"
		if item.Currency != nil && *item.Currency != "" {
			currency = *item.Currency
		}

		hasTaxID := item.TaxID != nil && strings.TrimSpace(*item.TaxID) != ""

		if hasTaxID {
			var existingID int64
			err := tx.QueryRow(ctx, `SELECT id FROM supplier WHERE tax_id = $1`, *item.TaxID).Scan(&existingID)
			if err != nil && err != pgx.ErrNoRows {
				return err
			}
			if err == nil {
				// Existing supplier with this tax_id — UPDATE only the fields this
				// import payload carries. is_active and every other column are left
				// untouched, per spec.
				var updated models.BulkInsertSupplierResultLine
				if err := tx.QueryRow(ctx, `
					UPDATE supplier SET
					    supplier_name=$1, address=$2, sales_person=$3, sales_person_phone=$4,
					    office_phone=$5, contact_email=$6, payment_terms=$7, currency=$8,
					    remarks=$9, updated_by=$10, updated_at=NOW()
					WHERE id=$11
					RETURNING id, supplier_name`,
					item.SupplierName, item.Address, item.SalesPerson, item.SalesPersonPhone,
					item.OfficePhone, item.ContactEmail, item.PaymentTerms, currency,
					item.Remarks, claims.UserID, existingID,
				).Scan(&updated.SupplierID, &updated.SupplierName); err != nil {
					return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("item[%d]: %s", i, err.Error()))
				}
				updated.Status = "updated"
				results = append(results, updated)
				duplicateCount++
				continue
			}
		}

		// No tax_id (can't dedup — always inserted, flagged distinctly below) or
		// tax_id not found among existing suppliers — insert as a new row.
		var inserted models.BulkInsertSupplierResultLine
		if err := tx.QueryRow(ctx, `
			INSERT INTO supplier
				(supplier_name, tax_id, address,
				 sales_person, sales_person_phone, office_phone, contact_email,
				 payment_terms, currency, remarks,
				 created_by, updated_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)
			RETURNING id, supplier_name`,
			item.SupplierName, item.TaxID, item.Address,
			item.SalesPerson, item.SalesPersonPhone, item.OfficePhone, item.ContactEmail,
			item.PaymentTerms, currency, item.Remarks,
			claims.UserID,
		).Scan(&inserted.SupplierID, &inserted.SupplierName); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("item[%d]: %s", i, err.Error()))
		}
		if hasTaxID {
			inserted.Status = "created"
		} else {
			inserted.Status = "created_no_tax_id"
		}
		results = append(results, inserted)
		importedCount++
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Flat (not nested under "data") to match SupplierPage.tsx's exact accessors:
	// `res.data.imported`, `res.data.duplicates`, `res.data.suppliers` — that page
	// reads straight off the axios body, not body.data. See handler comment on
	// BulkInsertSupplier below for the second call site's expectations.
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success":    true,
		"count":      len(results),
		"imported":   importedCount,
		"duplicates": duplicateCount,
		"suppliers":  results,
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
	maxLen := func(i int, field, value string, max int) error {
		if n := utf8.RuneCountInString(value); n > max {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf(
				"suppliers[%d]: %s exceeds max length %d (got %d)", i, field, max, n))
		}
		return nil
	}
	for i, s := range req.Suppliers {
		if s.SupplierName == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("suppliers[%d]: supplier_name is required", i))
		}
		if err := maxLen(i, "supplier_name", s.SupplierName, 200); err != nil {
			return err
		}
		if s.TaxID != nil {
			if err := maxLen(i, "tax_id", *s.TaxID, 50); err != nil {
				return err
			}
		}
		if s.ContactName != nil {
			if err := maxLen(i, "contact_name", *s.ContactName, 200); err != nil {
				return err
			}
		}
		if s.ContactPhone != nil {
			if err := maxLen(i, "contact_phone", *s.ContactPhone, 50); err != nil {
				return err
			}
		}
		if s.ContactEmail != nil {
			if err := maxLen(i, "contact_email", *s.ContactEmail, 200); err != nil {
				return err
			}
		}
		if s.OfficePhone != nil {
			if err := maxLen(i, "office_phone", *s.OfficePhone, 50); err != nil {
				return err
			}
		}
		if s.Fax != nil {
			if err := maxLen(i, "fax", *s.Fax, 50); err != nil {
				return err
			}
		}
		if s.PaymentTerms != nil {
			if err := maxLen(i, "payment_terms", *s.PaymentTerms, 100); err != nil {
				return err
			}
		}
		if s.Currency != nil {
			if err := maxLen(i, "currency", *s.Currency, 10); err != nil {
				return err
			}
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
	var importedCount, duplicateCount int
	for i, s := range req.Suppliers {
		// DB CHECK constraint supplier_currency_check only allows 'BH'/'US' — despite
		// database.md documenting a 'THB' default, that value would violate the live
		// constraint. 'BH' matches every existing supplier row and the real schema.
		currency := "BH"
		if s.Currency != nil && *s.Currency != "" {
			currency = *s.Currency
		}

		hasTaxID := s.TaxID != nil && strings.TrimSpace(*s.TaxID) != ""

		if hasTaxID {
			var existingID int64
			err := tx.QueryRow(ctx, `SELECT id FROM supplier WHERE tax_id = $1`, *s.TaxID).Scan(&existingID)
			if err != nil && err != pgx.ErrNoRows {
				return err
			}
			if err == nil {
				// Existing supplier with this tax_id — UPDATE only the fields this
				// import payload carries (its wider field set vs BulkCreateSupplier's,
				// since this endpoint's own payload includes them). is_active is left
				// untouched, per spec.
				var updated models.BulkInsertSupplierResultLine
				if err := tx.QueryRow(ctx, `
					UPDATE supplier SET
					    supplier_name=$1, address=$2, contact_name=$3, contact_phone=$4,
					    contact_email=$5, office_phone=$6, fax=$7, payment_terms=$8,
					    currency=$9, sales_person=$10, sales_person_phone=$11, remarks=$12,
					    updated_by=$13, updated_at=NOW()
					WHERE id=$14
					RETURNING id, supplier_name`,
					s.SupplierName, s.Address, s.ContactName, s.ContactPhone,
					s.ContactEmail, s.OfficePhone, s.Fax, s.PaymentTerms,
					currency, s.SalesPerson, s.SalesPersonPhone, s.Remarks,
					claims.UserID, existingID,
				).Scan(&updated.SupplierID, &updated.SupplierName); err != nil {
					return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("suppliers[%d]: %s", i, err.Error()))
				}
				updated.Status = "updated"
				results = append(results, updated)
				duplicateCount++
				continue
			}
		}

		var inserted models.BulkInsertSupplierResultLine
		if err := tx.QueryRow(ctx, `
			INSERT INTO supplier
				(supplier_name, tax_id, address,
				 contact_name, contact_phone, contact_email, office_phone, fax,
				 payment_terms, currency, sales_person, sales_person_phone, remarks,
				 is_active, created_by, updated_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,true,$14,$14)
			RETURNING id, supplier_name`,
			s.SupplierName, s.TaxID, s.Address,
			s.ContactName, s.ContactPhone, s.ContactEmail, s.OfficePhone, s.Fax,
			s.PaymentTerms, currency, s.SalesPerson, s.SalesPersonPhone, s.Remarks, claims.UserID,
		).Scan(&inserted.SupplierID, &inserted.SupplierName); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("suppliers[%d]: %s", i, err.Error()))
		}
		if hasTaxID {
			inserted.Status = "created"
		} else {
			inserted.Status = "created_no_tax_id"
		}
		results = append(results, inserted)
		importedCount++
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Same flat shape as BulkCreateSupplier above. handleBulkSubmit (the /supplier/bulk
	// caller in SupplierPage.tsx) reads `res.data?.data ?? res.data` so it tolerates either
	// nesting, plus `body.count`/`body.suppliers` — both covered here too.
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success":    true,
		"count":      len(results),
		"imported":   importedCount,
		"duplicates": duplicateCount,
		"suppliers":  results,
	})
}

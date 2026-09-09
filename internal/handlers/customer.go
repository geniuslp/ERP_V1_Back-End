package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

// CustomerHandler manages the customer master table (`customer`). customer_code
// is user-supplied at create time (required, unique) — not auto-generated.
// cus_id is the PK used for future joins (e.g. project table).
type CustomerHandler struct{ db *pgxpool.Pool }

func NewCustomerHandler(db *pgxpool.Pool) *CustomerHandler {
	return &CustomerHandler{db: db}
}

const customerCols = `cus_id, customer_code, customer_name, address, contact, credit_term,
	remarks, is_active, created_at, created_by, updated_at`

func scanCustomer(cu *models.Customer, row interface {
	Scan(dest ...any) error
}) error {
	return row.Scan(&cu.CusID, &cu.CustomerCode, &cu.CustomerName, &cu.Address, &cu.Contact,
		&cu.CreditTerm, &cu.Remarks, &cu.IsActive, &cu.CreatedAt, &cu.CreatedBy, &cu.UpdatedAt)
}

// validateCustomerFields is the single source of truth for required-field checks on a
// customer record — used by Create and Import so the two paths can't drift apart.
// customerCode is returned trimmed for the caller to use as-is.
func validateCustomerFields(customerCode, customerName string) (string, error) {
	customerCode = strings.TrimSpace(customerCode)
	if customerCode == "" {
		return "", fmt.Errorf("customer_code is required")
	}
	if strings.TrimSpace(customerName) == "" {
		return "", fmt.Errorf("customer_name is required")
	}
	return customerCode, nil
}

// List godoc
// @Summary      รายการลูกค้า
// @Description  List customers with pagination and search by name/code
// @Tags         Customer
// @Security     BearerAuth
// @Produce      json
// @Param        search     query  string  false  "Search by customer_name/customer_code"
// @Param        is_active  query  string  false  "Filter by active flag (default true)"
// @Param        page       query  int     false  "Page"
// @Param        page_size  query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /customer [get]
func (h *CustomerHandler) List(c *fiber.Ctx) error {
	search := c.Query("search")
	isActive := c.Query("is_active", "true")

	page, _ := strconv.Atoi(c.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	ctx := context.Background()

	where := []string{"1=1"}
	args := []any{}
	i := 1
	if isActive != "" {
		where = append(where, fmt.Sprintf("is_active = $%d", i))
		args = append(args, isActive == "true")
		i++
	}
	if search != "" {
		where = append(where, fmt.Sprintf("(customer_name ILIKE $%d OR customer_code ILIKE $%d)", i, i))
		args = append(args, "%"+search+"%")
		i++
	}
	whereClause := strings.Join(where, " AND ")

	var total int64
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM customer WHERE %s`, whereClause), args...).Scan(&total); err != nil {
		return err
	}

	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT `+customerCols+`
		FROM customer
		WHERE %s
		ORDER BY cus_id DESC
		LIMIT $%d OFFSET $%d`, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []models.Customer{}
	for rows.Next() {
		var cu models.Customer
		if err := scanCustomer(&cu, rows); err != nil {
			return err
		}
		items = append(items, cu)
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages,
		},
	})
}

// Get godoc
// @Summary      ข้อมูลลูกค้ารายตัว
// @Tags         Customer
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Customer ID"
// @Success      200  {object}  models.Customer
// @Failure      404  {object}  fiber.Map
// @Router       /customer/{id} [get]
func (h *CustomerHandler) Get(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var cu models.Customer
	if err := scanCustomer(&cu, h.db.QueryRow(context.Background(),
		`SELECT `+customerCols+` FROM customer WHERE cus_id = $1`, id)); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "customer not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": cu})
}

// Create godoc
// @Summary      สร้างลูกค้าใหม่
// @Description  customer_code is user-supplied and must be unique
// @Tags         Customer
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateCustomerRequest  true  "Customer data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /customer [post]
func (h *CustomerHandler) Create(c *fiber.Ctx) error {
	var req models.CreateCustomerRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	code, verr := validateCustomerFields(req.CustomerCode, req.CustomerName)
	if verr != nil {
		return fiber.NewError(fiber.StatusBadRequest, verr.Error())
	}
	req.CustomerCode = code

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer WHERE customer_code = $1)`, req.CustomerCode).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fiber.NewError(fiber.StatusConflict, "customer_code already exists")
	}

	var cu models.Customer
	err = scanCustomer(&cu, tx.QueryRow(ctx, `
		INSERT INTO customer (customer_code, customer_name, address, contact, credit_term, remarks, is_active, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,true,$7)
		RETURNING `+customerCols,
		req.CustomerCode, req.CustomerName, req.Address, req.Contact, req.CreditTerm, req.Remarks, claims.UserID,
	))
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusConflict, "customer_code already exists")
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": cu})
}

// Update godoc
// @Summary      แก้ไขข้อมูลลูกค้า
// @Tags         Customer
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                           true  "Customer ID"
// @Param        body  body  models.UpdateCustomerRequest  true  "Fields to update"
// @Success      200   {object}  models.Customer
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /customer/{id} [put]
func (h *CustomerHandler) Update(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var req models.UpdateCustomerRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	ctx := context.Background()

	if req.CustomerCode != nil {
		code := strings.TrimSpace(*req.CustomerCode)
		if code == "" {
			return fiber.NewError(fiber.StatusBadRequest, "customer_code cannot be empty")
		}
		req.CustomerCode = &code

		var conflictID int64
		err := h.db.QueryRow(ctx, `SELECT cus_id FROM customer WHERE customer_code = $1 AND cus_id != $2`, code, id).Scan(&conflictID)
		if err == nil {
			return fiber.NewError(fiber.StatusConflict, "customer_code already exists")
		}
	}

	var cu models.Customer
	err = scanCustomer(&cu, h.db.QueryRow(ctx, `
		UPDATE customer SET
			customer_code = COALESCE($1, customer_code),
			customer_name = COALESCE($2, customer_name),
			address       = COALESCE($3, address),
			contact       = COALESCE($4, contact),
			credit_term   = COALESCE($5, credit_term),
			remarks       = COALESCE($6, remarks),
			is_active     = COALESCE($7, is_active),
			updated_at    = NOW()
		WHERE cus_id = $8
		RETURNING `+customerCols,
		req.CustomerCode, req.CustomerName, req.Address, req.Contact, req.CreditTerm, req.Remarks, req.IsActive, id,
	))
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusConflict, "customer_code already exists")
		}
		return fiber.NewError(fiber.StatusNotFound, "customer not found")
	}

	return c.JSON(fiber.Map{"success": true, "data": cu})
}

// Delete godoc
// @Summary      ลบลูกค้า (soft delete)
// @Description  Sets is_active=false; does not hard delete since customer will be referenced by projects/PR/PO later
// @Tags         Customer
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Customer ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /customer/{id} [delete]
func (h *CustomerHandler) Delete(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	tag, err := h.db.Exec(context.Background(),
		`UPDATE customer SET is_active = false, updated_at = NOW() WHERE cus_id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "customer not found")
	}

	return c.JSON(fiber.Map{"success": true, "message": "customer deactivated"})
}

// ─── Import ──────────────────────────────────────────────────────────────────

// nullableCell converts a trimmed-empty Excel cell to nil, otherwise returns a pointer to
// the trimmed value — for optional VARCHAR/TEXT columns bound via COALESCE-free plain INSERT.
func nullableCell(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func cellAt(row []string, idx int) string {
	if idx < len(row) {
		return row[idx]
	}
	return ""
}

// Import godoc
// @Summary      Bulk import customers จาก Excel
// @Description  Column layout (row 1 = header, data starts row 2): customer_code*, customer_name*, address, contact, credit_term, remarks. Validates every row (required fields, customer_code uniqueness — case-sensitive exact match, credit_term against the fixed option list excluding เงินสด) and processes in partial-success mode: valid rows are inserted, invalid rows are skipped and reported. Re-upload only the failed rows after fixing them.
// @Tags         Customer
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        file  formData  file  true  "Excel file (.xlsx)"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /customer/import [post]
func (h *CustomerHandler) Import(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file is required")
	}
	f, err := file.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cannot open file")
	}
	defer f.Close()

	xlsx, err := excelize.OpenReader(f)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid excel file")
	}
	sheetRows, err := xlsx.GetRows(xlsx.GetSheetName(0))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cannot read sheet")
	}
	if len(sheetRows) < 2 {
		return fiber.NewError(fiber.StatusBadRequest, "no data rows found")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	creditTermSet := toStringSet(creditTermOptionsExcludingCash())

	type rowError struct {
		Row    int    `json:"row"`
		Reason string `json:"reason"`
	}
	type rowSuccess struct {
		Row          int    `json:"row"`
		CusID        int64  `json:"cus_id"`
		CustomerCode string `json:"customer_code"`
	}
	var errs []rowError
	var oks []rowSuccess

	for i, row := range sheetRows[1:] {
		rowNo := i + 2 // account for header row
		code, name := cellAt(row, 0), cellAt(row, 1)

		// An empty customer_code marks the end of the data grid — the template's notes/legend
		// text (and any trailing blank rows) live below this point and must never be read as
		// data, so stop entirely rather than reporting them as failed rows.
		if strings.TrimSpace(code) == "" {
			break
		}

		code, verr := validateCustomerFields(code, name)
		if verr != nil {
			errs = append(errs, rowError{Row: rowNo, Reason: verr.Error()})
			continue
		}

		creditTerm := strings.TrimSpace(cellAt(row, 4))
		if creditTerm != "" && !creditTermSet[creditTerm] {
			errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("credit_term %q is not a valid option", creditTerm)})
			continue
		}

		var exists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer WHERE customer_code = $1)`, code).Scan(&exists); err != nil {
			errs = append(errs, rowError{Row: rowNo, Reason: err.Error()})
			continue
		}
		if exists {
			errs = append(errs, rowError{Row: rowNo, Reason: "customer_code already exists"})
			continue
		}

		var cusID int64
		err := h.db.QueryRow(ctx, `
			INSERT INTO customer (customer_code, customer_name, address, contact, credit_term, remarks, is_active, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,true,$7)
			RETURNING cus_id`,
			code, strings.TrimSpace(name),
			nullableCell(cellAt(row, 2)), nullableCell(cellAt(row, 3)), nullableCell(cellAt(row, 4)), nullableCell(cellAt(row, 5)),
			claims.UserID,
		).Scan(&cusID)
		if err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
				errs = append(errs, rowError{Row: rowNo, Reason: "customer_code already exists"})
				continue
			}
			errs = append(errs, rowError{Row: rowNo, Reason: err.Error()})
			continue
		}

		oks = append(oks, rowSuccess{Row: rowNo, CusID: cusID, CustomerCode: code})
	}

	if errs == nil {
		errs = []rowError{}
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"imported": len(oks),
			"failed":   len(errs),
			"rows":     oks,
			"errors":   errs,
		},
	})
}

// ImportTemplate godoc
// @Summary      ดาวน์โหลด template Excel สำหรับ import ลูกค้า
// @Description  Generates the .xlsx on the fly: sheet "Customer" with header row (customer_code*, customer_name*, address, contact, credit_term, remarks), one example row, and a notes row. A hidden "Lookup" sheet lists the fixed credit_term options (same hardcoded list as the Supplier page's dropdown, excluding เงินสด for Customer), bound to the credit_term column as an Excel dropdown. No database query is involved for this list.
// @Tags         Customer
// @Security     BearerAuth
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200  {file}  file
// @Failure      500  {object}  fiber.Map
// @Router       /customer/import/template [get]
func (h *CustomerHandler) ImportTemplate(c *fiber.Ctx) error {
	creditTerms := creditTermOptionsExcludingCash()

	f := excelize.NewFile()
	defer f.Close()

	const sheet = "Customer"
	f.SetSheetName("Sheet1", sheet)

	boldStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return err
	}
	// requiredStyle marks required columns visually (bold + a distinct fill) WITHOUT touching
	// the header cell's text — the import parser matches header text exactly against expected
	// field names, so appending a "*" to the string (as this template used to do) made
	// "customer_code*" fail to match "customer_code" and read as an unrecognized column.
	requiredStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FFF2CC"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	headers := []string{"customer_code", "customer_name", "address", "contact", "credit_term", "remarks"}
	required := []bool{true, true, false, false, false, false}
	colWidths := []float64{18, 30, 30, 24, 14, 30}
	for i, hdr := range headers {
		col := string(rune('A' + i))
		cell := col + "1"
		f.SetCellValue(sheet, cell, hdr)
		style := boldStyle
		if required[i] {
			style = requiredStyle
		}
		f.SetCellStyle(sheet, cell, cell, style)
		f.SetColWidth(sheet, col, col, colWidths[i])
	}

	example := []string{"CUS-000001", "บริษัท ตัวอย่าง จำกัด", "123 ถนนตัวอย่าง กรุงเทพฯ", "คุณสมชาย 08x-xxx-xxxx", "30 วัน", ""}
	for i, v := range example {
		f.SetCellValue(sheet, fmt.Sprintf("%s2", string(rune('A'+i))), v)
	}

	_, _, creditRange, err := writeLookupSheet(f, nil, nil, creditTerms)
	if err != nil {
		return err
	}
	if err := addDropdown(f, sheet, "E2:E1000", creditRange); err != nil {
		return err
	}

	f.SetActiveSheet(0)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename=customer_import_template.xlsx")
	return c.Send(buf.Bytes())
}

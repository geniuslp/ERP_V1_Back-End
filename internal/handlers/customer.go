package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
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

const customerCols = `cus_id, customer_code, customer_name, address, contact, contact_phone, tax_id, credit_term,
	remarks, tel, is_active, created_at, created_by, updated_at, updated_by`

func scanCustomer(cu *models.Customer, row interface {
	Scan(dest ...any) error
}) error {
	return row.Scan(&cu.CusID, &cu.CustomerCode, &cu.CustomerName, &cu.Address, &cu.Contact,
		&cu.ContactPhone, &cu.TaxID, &cu.CreditTerm, &cu.Remarks, &cu.Tel, &cu.IsActive, &cu.CreatedAt, &cu.CreatedBy, &cu.UpdatedAt, &cu.UpdatedBy)
}

// validateCustomerFields is the single source of truth for required-field checks on a
// customer record — used by Import so the Excel path can't drift apart from the API path.
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

// customerOptional trims an optional text input; empty becomes nil so the column is stored as NULL, never an empty string.
func customerOptional(v *string) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil
	}
	return &t
}

// checkCustomerFields validates already-trimmed values with plain-language errors (before any SQL).
func checkCustomerFields(code, name string, tel, contactPhone *string) error {
	if code == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณากรอกรหัสลูกค้า")
	}
	if utf8.RuneCountInString(code) > 20 {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสลูกค้ายาวเกินกำหนด (สูงสุด 20 ตัวอักษร)")
	}
	if name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณากรอกชื่อลูกค้า")
	}
	if utf8.RuneCountInString(name) > 255 {
		return fiber.NewError(fiber.StatusBadRequest, "ชื่อลูกค้ายาวเกินกำหนด (สูงสุด 255 ตัวอักษร)")
	}
	if tel != nil && utf8.RuneCountInString(*tel) > 50 {
		return fiber.NewError(fiber.StatusBadRequest, "เบอร์โทรยาวเกินกำหนด (สูงสุด 50 ตัวอักษร)")
	}
	if contactPhone != nil && utf8.RuneCountInString(*contactPhone) > 50 {
		return fiber.NewError(fiber.StatusBadRequest, "เบอร์โทรผู้ติดต่อยาวเกินกำหนด (สูงสุด 50 ตัวอักษร)")
	}
	return nil
}

// normalizeCustomerTaxID trims the value, treats empty as NULL, and — when present — strips
// spaces/dashes (Excel often formats it as 1-2345-67890-12-3) and requires exactly 13 digits.
// The returned *string is the value to store. Errors are plain Thai, raised before any SQL.
func normalizeCustomerTaxID(v *string) (*string, error) {
	t := customerOptional(v)
	if t == nil {
		return nil, nil
	}
	if utf8.RuneCountInString(*t) > 20 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "เลขประจำตัวผู้เสียภาษียาวเกินกำหนด (สูงสุด 20 ตัวอักษร)")
	}
	digits := strings.NewReplacer(" ", "", "-", "").Replace(*t)
	if utf8.RuneCountInString(digits) != 13 || !isASCIIDigits(digits) {
		return nil, fiber.NewError(fiber.StatusBadRequest, "เลขประจำตัวผู้เสียภาษีต้องมี 13 หลัก")
	}
	return &digits, nil
}

// customerCodeTaken reports whether another customer already uses this code (case-insensitive,
// surrounding spaces ignored). excludeID = 0 means "no row to exclude" (create).
func (h *CustomerHandler) customerCodeTaken(ctx context.Context, code string, excludeID int64) (bool, error) {
	var taken bool
	err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM customer WHERE lower(btrim(customer_code)) = lower($1) AND cus_id <> $2)`,
		code, excludeID).Scan(&taken)
	return taken, err
}

// customerListMaxPageSize caps GET /customer page_size/limit (default stays 20).
const customerListMaxPageSize = 500

const customerDupMsg = "รหัสลูกค้านี้มีอยู่แล้ว"

// List godoc
// @Summary      รายการลูกค้า
// @Description  Paginated list sorted by customer_code. q (alias search) matches customer_code, customer_name, contact, contact_phone, tel (case-insensitive). is_active defaults to true when omitted; pass is_active=false for inactive only or is_active=all for both.
// @Tags         Customer
// @Security     BearerAuth
// @Produce      json
// @Param        q          query  string  false  "Search customer_code/customer_name/contact/tel"
// @Param        search     query  string  false  "Alias of q"
// @Param        is_active  query  string  false  "true (default) | false | all"
// @Param        page       query  int     false  "Page"
// @Param        page_size  query  int     false  "Page size (default 20, max 500; alias limit)"
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /customer [get]
// @Router       /master/customer [get]
func (h *CustomerHandler) List(c *fiber.Ctx) error {
	search := strings.TrimSpace(c.Query("q"))
	if search == "" {
		search = strings.TrimSpace(c.Query("search"))
	}
	isActive := c.Query("is_active", "true")

	page, _ := strconv.Atoi(c.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	// page_size (alias limit): default 20; values above customerListMaxPageSize are clamped to it
	// (not reset to the default), so a dropdown asking for everything gets the max, not 20.
	sizeStr := c.Query("page_size")
	if sizeStr == "" {
		sizeStr = c.Query("limit", "20")
	}
	pageSize, _ := strconv.Atoi(sizeStr)
	if pageSize < 1 {
		pageSize = 20
	} else if pageSize > customerListMaxPageSize {
		pageSize = customerListMaxPageSize
	}

	ctx := context.Background()

	where := []string{"1=1"}
	args := []any{}
	i := 1
	if isActive == "true" || isActive == "false" {
		where = append(where, fmt.Sprintf("is_active = $%d", i))
		args = append(args, isActive == "true")
		i++
	}
	if search != "" {
		where = append(where, fmt.Sprintf("(customer_code ILIKE $%d OR customer_name ILIKE $%d OR contact ILIKE $%d OR contact_phone ILIKE $%d OR tel ILIKE $%d)", i, i, i, i, i))
		args = append(args, "%"+search+"%")
		i++
	}
	whereClause := strings.Join(where, " AND ")

	// COUNT uses args as-is; the SELECT appends limit/offset, so its LIMIT/OFFSET placeholders are $i/$i+1.
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
		ORDER BY customer_code ASC, cus_id ASC
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
	if err := rows.Err(); err != nil {
		return err
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
// @Param        id  path  int  true  "Customer ID (cus_id)"
// @Success      200  {object}  models.Customer
// @Failure      404  {object}  fiber.Map
// @Router       /customer/{id} [get]
// @Router       /master/customer/{id} [get]
func (h *CustomerHandler) Get(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสอ้างอิงลูกค้าไม่ถูกต้อง")
	}

	var cu models.Customer
	if err := scanCustomer(&cu, h.db.QueryRow(context.Background(),
		`SELECT `+customerCols+` FROM customer WHERE cus_id = $1`, id)); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบข้อมูลลูกค้า")
	}
	return c.JSON(fiber.Map{"success": true, "data": cu})
}

// Create godoc
// @Summary      สร้างลูกค้าใหม่
// @Description  customer_code (max 20, unique ignoring case/surrounding spaces) and customer_name (max 255) are required; tel max 50. Empty optional fields are stored as NULL.
// @Tags         Customer
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateCustomerRequest  true  "Customer data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /customer [post]
// @Router       /master/customer [post]
func (h *CustomerHandler) Create(c *fiber.Ctx) error {
	var req models.CreateCustomerRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
	}
	code := strings.TrimSpace(req.CustomerCode)
	name := strings.TrimSpace(req.CustomerName)
	tel, contactPhone := customerOptional(req.Tel), customerOptional(req.ContactPhone)
	taxID, err := normalizeCustomerTaxID(req.TaxID)
	if err != nil {
		return err
	}
	if err := checkCustomerFields(code, name, tel, contactPhone); err != nil {
		return err
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	taken, err := h.customerCodeTaken(ctx, code, 0)
	if err != nil {
		return err
	}
	if taken {
		return fiber.NewError(fiber.StatusConflict, customerDupMsg)
	}

	var cu models.Customer
	err = scanCustomer(&cu, h.db.QueryRow(ctx, `
		INSERT INTO customer (customer_code, customer_name, address, contact, contact_phone, tax_id, credit_term, remarks, tel, is_active, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,true,$10)
		RETURNING `+customerCols,
		code, name, customerOptional(req.Address), customerOptional(req.Contact), contactPhone, taxID, customerOptional(req.CreditTerm),
		customerOptional(req.Remarks), tel, claims.UserID,
	))
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" { // lost a race with a concurrent create
			return fiber.NewError(fiber.StatusConflict, customerDupMsg)
		}
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": cu})
}

// Update godoc
// @Summary      แก้ไขข้อมูลลูกค้า
// @Description  Omitted fields keep their current value; an optional field sent as an empty string is cleared to NULL. updated_by/updated_at are set from the JWT / now().
// @Tags         Customer
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                           true  "Customer ID (cus_id)"
// @Param        body  body  models.UpdateCustomerRequest  true  "Fields to update"
// @Success      200   {object}  models.Customer
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /customer/{id} [put]
// @Router       /master/customer/{id} [put]
func (h *CustomerHandler) Update(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสอ้างอิงลูกค้าไม่ถูกต้อง")
	}

	var req models.UpdateCustomerRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var cur models.Customer
	if err := scanCustomer(&cur, h.db.QueryRow(ctx,
		`SELECT `+customerCols+` FROM customer WHERE cus_id = $1`, id)); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบข้อมูลลูกค้า")
	}

	// merge: required fields change only when sent; optional fields change when sent (empty -> NULL)
	code, name := cur.CustomerCode, cur.CustomerName
	if req.CustomerCode != nil {
		code = strings.TrimSpace(*req.CustomerCode)
	}
	if req.CustomerName != nil {
		name = strings.TrimSpace(*req.CustomerName)
	}
	pick := func(sent, current *string) *string {
		if sent == nil {
			return current
		}
		return customerOptional(sent)
	}
	address, contact := pick(req.Address, cur.Address), pick(req.Contact, cur.Contact)
	contactPhone := pick(req.ContactPhone, cur.ContactPhone)
	taxID := cur.TaxID
	if req.TaxID != nil {
		var terr error
		if taxID, terr = normalizeCustomerTaxID(req.TaxID); terr != nil {
			return terr
		}
	}
	creditTerm, remarks, tel := pick(req.CreditTerm, cur.CreditTerm), pick(req.Remarks, cur.Remarks), pick(req.Tel, cur.Tel)
	isActive := cur.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	if err := checkCustomerFields(code, name, tel, contactPhone); err != nil {
		return err
	}
	if req.CustomerCode != nil {
		taken, err := h.customerCodeTaken(ctx, code, int64(id))
		if err != nil {
			return err
		}
		if taken {
			return fiber.NewError(fiber.StatusConflict, customerDupMsg)
		}
	}

	var cu models.Customer
	err = scanCustomer(&cu, h.db.QueryRow(ctx, `
		UPDATE customer SET
			customer_code=$1, customer_name=$2, address=$3, contact=$4, contact_phone=$5, tax_id=$6, credit_term=$7,
			remarks=$8, tel=$9, is_active=$10, updated_at=NOW(), updated_by=$11
		WHERE cus_id=$12
		RETURNING `+customerCols,
		code, name, address, contact, contactPhone, taxID, creditTerm, remarks, tel, isActive, claims.UserID, id,
	))
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusConflict, customerDupMsg)
		}
		return err
	}

	return c.JSON(fiber.Map{"success": true, "data": cu})
}

// customerInUse reports whether any row anywhere still references this customer. It reads every
// single-column foreign key that points at public.customer from pg_constraint (so a future FK is
// covered automatically; today that is project.customer_id) and checks each referencing table.
func (h *CustomerHandler) customerInUse(ctx context.Context, id int64) (bool, error) {
	rows, err := h.db.Query(ctx, `
		SELECT cl.relname, att.attname, ratt.attname
		FROM pg_constraint con
		JOIN pg_class cl ON cl.oid = con.conrelid
		JOIN pg_namespace ns ON ns.oid = cl.relnamespace
		JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = con.conkey[1]
		JOIN pg_attribute ratt ON ratt.attrelid = con.confrelid AND ratt.attnum = con.confkey[1]
		WHERE con.contype = 'f'
		  AND con.confrelid = 'public.customer'::regclass
		  AND array_length(con.conkey, 1) = 1
		  AND ns.nspname = 'public'`)
	if err != nil {
		return false, err
	}
	type ref struct{ table, col, refCol string }
	var refs []ref
	for rows.Next() {
		var r ref
		if err := rows.Scan(&r.table, &r.col, &r.refCol); err != nil {
			rows.Close()
			return false, err
		}
		refs = append(refs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}
	for _, r := range refs {
		q := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE %s = (SELECT %s FROM customer WHERE cus_id = $1))`,
			pgx.Identifier{"public", r.table}.Sanitize(), pgx.Identifier{r.col}.Sanitize(), pgx.Identifier{r.refCol}.Sanitize())
		var used bool
		if err := h.db.QueryRow(ctx, q, id).Scan(&used); err != nil {
			return false, err
		}
		if used {
			return true, nil
		}
	}
	return false, nil
}

const customerInUseMsg = "ลูกค้ารายนี้ถูกใช้งานแล้ว ไม่สามารถลบได้ กรุณาปิดการใช้งานแทน"

// Delete godoc
// @Summary      ลบลูกค้า (hard delete เมื่อยังไม่ถูกใช้งาน)
// @Description  Deletes the row only when nothing references it (every FK to customer is checked, today project.customer_id; inactive projects count too). Otherwise 409 and the customer should be deactivated via PUT is_active=false instead. 404 when the customer does not exist.
// @Tags         Customer
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Customer ID (cus_id)"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Router       /customer/{id} [delete]
// @Router       /master/customer/{id} [delete]
func (h *CustomerHandler) Delete(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสอ้างอิงลูกค้าไม่ถูกต้อง")
	}
	ctx := context.Background()

	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer WHERE cus_id = $1)`, id).Scan(&exists); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถลบข้อมูลลูกค้าได้ กรุณาลองใหม่อีกครั้ง")
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบข้อมูลลูกค้า")
	}

	inUse, err := h.customerInUse(ctx, int64(id))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบการใช้งานของลูกค้าได้ กรุณาลองใหม่อีกครั้ง")
	}
	if inUse {
		return fiber.NewError(fiber.StatusConflict, customerInUseMsg)
	}

	tag, err := h.db.Exec(ctx, `DELETE FROM customer WHERE cus_id = $1`, id)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" { // referenced between the check and the delete
			return fiber.NewError(fiber.StatusConflict, customerInUseMsg)
		}
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถลบข้อมูลลูกค้าได้ กรุณาลองใหม่อีกครั้ง")
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบข้อมูลลูกค้า")
	}

	return c.JSON(fiber.Map{"success": true, "message": "ลบลูกค้าเรียบร้อยแล้ว"})
}

// ─── JSON import (frontend parses the Excel file and posts the rows) ─────────

const customerImportMaxRows = 2000

// customerImportCell accepts a JSON string, number or null — Excel numeric cells (e.g. a
// credit_term of 30) often arrive as numbers, and must not fail the whole request body.
type customerImportCell string

func (c *customerImportCell) UnmarshalJSON(b []byte) error {
	t := strings.TrimSpace(string(b))
	if t == "null" {
		*c = ""
		return nil
	}
	if strings.HasPrefix(t, `"`) {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*c = customerImportCell(s)
		return nil
	}
	*c = customerImportCell(t) // number / bool literal, validated later like any other text
	return nil
}

type customerImportRow struct {
	// Row is the optional spreadsheet row number to show in error messages (e.g. 5 for the
	// 4th data row under a header). Falls back to the 1-based position in the array.
	Row          int                `json:"row,omitempty"`
	CustomerCode customerImportCell `json:"customer_code"`
	CustomerName customerImportCell `json:"customer_name"`
	Contact      customerImportCell `json:"contact"`
	ContactPhone customerImportCell `json:"contact_phone"`
	TaxID        customerImportCell `json:"tax_id"`
	Address      customerImportCell `json:"address"`
	CreditTerm   customerImportCell `json:"credit_term"`
	Remarks      customerImportCell `json:"remarks"`
}

type customerImportRequest struct {
	Rows []customerImportRow `json:"rows"`
}

type customerImportError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ImportRows godoc
// @Summary      Import ลูกค้าจาก JSON (frontend อ่านไฟล์ Excel แล้วส่งเป็น rows)
// @Description  Body {rows:[{row?, customer_code*, customer_name*, contact, contact_phone, tax_id, address, credit_term, remarks}]}, max 2000 rows, fully empty rows ignored. row (optional) is the spreadsheet row number used in error messages; default is the 1-based array position. Values are trimmed and empty optional fields become NULL. All rows are validated first (code required/max 20, name required/max 255, contact_phone max 50, tax_id (spaces/dashes stripped, must be 13 digits, raw max 20), credit_term free text max 50 (trimmed, empty = NULL, never parsed); duplicate customer_code inside the file, compared by lower(trim), is rejected and every involved row is listed); any error gives 400 {errors:[{row,field,message}]} with Thai messages "แถวที่ N: ..." and nothing is written. Otherwise ONE transaction: match an existing customer by lower(trim(customer_code)); no match then INSERT (is_active=true, created_by); match then UPDATE all imported fields (a blank optional cell clears the stored value; is_active is never touched). Response data: {inserted, updated, total} (created = inserted, kept for older clients).
// @Tags         Customer
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  customerImportRequest  true  "rows"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /master/customer/import [post]
func (h *CustomerHandler) ImportRows(c *fiber.Ctx) error {
	var req customerImportRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
	}
	if len(req.Rows) > customerImportMaxRows {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("นำเข้าได้สูงสุด %d แถวต่อครั้ง (ส่งมา %d แถว)", customerImportMaxRows, len(req.Rows)))
	}

	type item struct {
		row                   int
		code, name, key       string
		contact, contactPhone *string
		taxID                 *string
		address, creditTerm   *string
		remarks               *string
	}
	var items []item
	var errs []customerImportError
	addErr := func(row int, field, msg string) {
		errs = append(errs, customerImportError{row, field, fmt.Sprintf("แถวที่ %d: %s", row, msg)})
	}
	opt := func(v customerImportCell) *string { s := string(v); return customerOptional(&s) }
	rowsByKey := map[string][]int{}
	for i, r := range req.Rows {
		code, name := strings.TrimSpace(string(r.CustomerCode)), strings.TrimSpace(string(r.CustomerName))
		contact, contactPhone := opt(r.Contact), opt(r.ContactPhone)
		rawTax := opt(r.TaxID)
		address, creditTerm, remarks := opt(r.Address), opt(r.CreditTerm), opt(r.Remarks)
		if code == "" && name == "" && rawTax == nil && contact == nil && contactPhone == nil && address == nil && creditTerm == nil && remarks == nil {
			continue // fully empty row
		}
		rowNo := i + 1
		if r.Row > 0 {
			rowNo = r.Row
		}
		bad := false
		switch {
		case code == "":
			addErr(rowNo, "customer_code", "ต้องระบุรหัสลูกค้า")
			bad = true
		case utf8.RuneCountInString(code) > 20:
			addErr(rowNo, "customer_code", "รหัสลูกค้ายาวเกินกำหนด (สูงสุด 20 ตัวอักษร)")
			bad = true
		}
		switch {
		case name == "":
			addErr(rowNo, "customer_name", "ต้องระบุชื่อลูกค้า")
			bad = true
		case utf8.RuneCountInString(name) > 255:
			addErr(rowNo, "customer_name", "ชื่อลูกค้ายาวเกินกำหนด (สูงสุด 255 ตัวอักษร)")
			bad = true
		}
		if contactPhone != nil && utf8.RuneCountInString(*contactPhone) > 50 {
			addErr(rowNo, "contact_phone", "เบอร์โทรผู้ติดต่อยาวเกินกำหนด (สูงสุด 50 ตัวอักษร)")
			bad = true
		}
		taxID, terr := normalizeCustomerTaxID(rawTax)
		if terr != nil {
			addErr(rowNo, "tax_id", terr.(*fiber.Error).Message)
			bad = true
		}
		if creditTerm != nil && utf8.RuneCountInString(*creditTerm) > 50 {
			addErr(rowNo, "credit_term", "เครดิตยาวเกิน 50 ตัวอักษร")
			bad = true
		}
		key := strings.ToLower(code)
		if code != "" {
			rowsByKey[key] = append(rowsByKey[key], rowNo)
		}
		if !bad {
			items = append(items, item{rowNo, code, name, key, contact, contactPhone, taxID, address, creditTerm, remarks})
		}
	}
	// Duplicate codes inside the file: report every involved row, once per row.
	for _, dupRows := range rowsByKey {
		if len(dupRows) < 2 {
			continue
		}
		list := make([]string, len(dupRows))
		for i, n := range dupRows {
			list[i] = strconv.Itoa(n)
		}
		for _, n := range dupRows {
			addErr(n, "customer_code", fmt.Sprintf("รหัสลูกค้าซ้ำในไฟล์เดียวกัน (แถวที่ %s)", strings.Join(list, ", ")))
		}
	}
	if len(errs) > 0 {
		sort.SliceStable(errs, func(a, b int) bool { return errs[a].Row < errs[b].Row })
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false, "error": errs[0].Message, "errors": errs,
		})
	}
	if len(items) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่พบข้อมูลที่จะนำเข้า")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)
	fail := fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถนำเข้าข้อมูลลูกค้าได้ กรุณาลองใหม่อีกครั้ง")

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fail
	}
	defer tx.Rollback(ctx)

	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.key
	}
	existing := map[string]int64{}
	rows, err := tx.Query(ctx,
		`SELECT cus_id, lower(btrim(customer_code)) FROM customer
		 WHERE lower(btrim(customer_code)) = ANY($1) ORDER BY cus_id DESC`, keys)
	if err != nil {
		return fail
	}
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			rows.Close()
			return fail
		}
		existing[key] = id
	}
	rows.Close()
	if rows.Err() != nil {
		return fail
	}

	inserted, updated := 0, 0
	for _, it := range items {
		if id, found := existing[it.key]; found {
			if _, err := tx.Exec(ctx, `
				UPDATE customer SET customer_name=$1, contact=$2, contact_phone=$3, tax_id=$4, address=$5,
					credit_term=$6, remarks=$7, updated_by=$8, updated_at=NOW()
				WHERE cus_id=$9`,
				it.name, it.contact, it.contactPhone, it.taxID, it.address, it.creditTerm, it.remarks, claims.UserID, id); err != nil {
				return fail
			}
			updated++
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO customer (customer_code, customer_name, contact, contact_phone, tax_id, address, credit_term, remarks, is_active, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,true,$9)`,
			it.code, it.name, it.contact, it.contactPhone, it.taxID, it.address, it.creditTerm, it.remarks, claims.UserID); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
				return fiber.NewError(fiber.StatusConflict, fmt.Sprintf("แถวที่ %d: %s", it.row, customerDupMsg))
			}
			return fail
		}
		inserted++
	}
	if err := tx.Commit(ctx); err != nil {
		return fail
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"inserted": inserted, "updated": updated, "total": inserted + updated,
		"created": inserted, // alias of inserted for older clients
	}})
}

// isASCIIDigits reports whether s is non-empty and made only of ASCII digits (used for tax_id).
func isASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
// @Description  Column layout (row 1 = header, data starts row 2): customer_code*, customer_name*, address, contact, credit_term, remarks, contact_phone (optional, max 50), tax_id (optional, 13 digits). Validates every row (required fields, customer_code uniqueness — case-sensitive exact match, credit_term against the fixed option list excluding เงินสด) and processes in partial-success mode: valid rows are inserted, invalid rows are skipped and reported. Re-upload only the failed rows after fixing them.
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

		if utf8.RuneCountInString(strings.TrimSpace(cellAt(row, 6))) > 50 {
			errs = append(errs, rowError{Row: rowNo, Reason: "เบอร์โทรผู้ติดต่อยาวเกินกำหนด (สูงสุด 50 ตัวอักษร)"})
			continue
		}

		xlsxTaxID, terr := normalizeCustomerTaxID(nullableCell(cellAt(row, 7)))
		if terr != nil {
			errs = append(errs, rowError{Row: rowNo, Reason: terr.(*fiber.Error).Message})
			continue
		}

		var exists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer WHERE customer_code = $1)`, code).Scan(&exists); err != nil {
			errs = append(errs, rowError{Row: rowNo, Reason: "ไม่สามารถตรวจสอบรหัสลูกค้าได้"})
			continue
		}
		if exists {
			errs = append(errs, rowError{Row: rowNo, Reason: "customer_code already exists"})
			continue
		}

		var cusID int64
		err := h.db.QueryRow(ctx, `
			INSERT INTO customer (customer_code, customer_name, address, contact, credit_term, remarks, contact_phone, tax_id, is_active, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,true,$9)
			RETURNING cus_id`,
			code, strings.TrimSpace(name),
			nullableCell(cellAt(row, 2)), nullableCell(cellAt(row, 3)), nullableCell(cellAt(row, 4)), nullableCell(cellAt(row, 5)),
			nullableCell(cellAt(row, 6)), xlsxTaxID, claims.UserID,
		).Scan(&cusID)
		if err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
				errs = append(errs, rowError{Row: rowNo, Reason: "customer_code already exists"})
				continue
			}
			errs = append(errs, rowError{Row: rowNo, Reason: "ไม่สามารถบันทึกแถวนี้ได้"})
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
	headers := []string{"customer_code", "customer_name", "address", "contact", "credit_term", "remarks", "contact_phone", "tax_id"}
	required := []bool{true, true, false, false, false, false, false, false}
	colWidths := []float64{18, 30, 30, 24, 14, 30, 18, 18}
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

	example := []string{"CUS-000001", "บริษัท ตัวอย่าง จำกัด", "123 ถนนตัวอย่าง กรุงเทพฯ", "คุณสมชาย 08x-xxx-xxxx", "30 วัน", "", "08x-xxx-xxxx", "0-1055-12345-67-8"}
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

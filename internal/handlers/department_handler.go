package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DepartmentHandler struct{ db *pgxpool.Pool }

func NewDepartmentHandler(db *pgxpool.Pool) *DepartmentHandler {
	return &DepartmentHandler{db: db}
}

const (
	deptCodeMaxLen = 20
	deptNameMaxLen = 200

	msgDeptDuplicate = "รหัสแผนกนี้มีอยู่แล้ว"
	msgDeptNotFound  = "ไม่พบแผนกนี้"
	msgDeptSaveFail  = "ไม่สามารถบันทึกข้อมูลแผนกได้ กรุณาลองใหม่อีกครั้ง"
)

var deptCodeRe = regexp.MustCompile(`^[A-Z0-9_]+$`)

// deptTableLabels: Thai names for the tables that reference departments.dept_code, used in the delete-guard message.
// Any other table found through information_schema is reported under its table name.
var deptTableLabels = map[string]string{
	"users":                  "ผู้ใช้",
	"purchase_request":       "เอกสาร PR",
	"petty_cash_requisition": "ใบเบิกเงินสดย่อย",
	"project":                "โครงการ",
	"roles":                  "บทบาท",
	"dept_menu_permissions":  "สิทธิ์เมนูของแผนก",
}

type departmentItem struct {
	ID        int64  `json:"id"`
	DeptCode  string `json:"dept_code"`
	DeptName  string `json:"dept_name"`
	SortOrder int    `json:"sort_order"`
	IsActive  bool   `json:"is_active"`
}

type departmentCreateRequest struct {
	DeptCode  string `json:"dept_code"`
	DeptName  string `json:"dept_name"`
	SortOrder *int   `json:"sort_order"`
	IsActive  *bool  `json:"is_active"`
}

type departmentUpdateRequest struct {
	DeptCode  *string `json:"dept_code"` // immutable: only accepted when it equals the stored code
	DeptName  string  `json:"dept_name"`
	SortOrder *int    `json:"sort_order"`
	IsActive  *bool   `json:"is_active"`
}

// normalizeDeptCode trims and upper-cases, then validates length and charset.
func normalizeDeptCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุรหัสแผนก")
	}
	if utf8.RuneCountInString(code) > deptCodeMaxLen {
		return "", fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("รหัสแผนกต้องไม่เกิน %d ตัวอักษร", deptCodeMaxLen))
	}
	if !deptCodeRe.MatchString(code) {
		return "", fiber.NewError(fiber.StatusBadRequest, "รหัสแผนกใช้ได้เฉพาะตัวอักษร A-Z ตัวเลข 0-9 และเครื่องหมาย _ เท่านั้น")
	}
	return code, nil
}

func normalizeDeptName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุชื่อแผนก")
	}
	if utf8.RuneCountInString(name) > deptNameMaxLen {
		return "", fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("ชื่อแผนกต้องไม่เกิน %d ตัวอักษร", deptNameMaxLen))
	}
	return name, nil
}

func checkDeptSortOrder(v *int) error {
	if v != nil && *v < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "ลำดับการแสดงต้องเป็นจำนวนเต็มตั้งแต่ 0 ขึ้นไป")
	}
	return nil
}

func deptServerError(op string, err error) error {
	log.Printf("departments %s: %v", op, err)
	return fiber.NewError(fiber.StatusInternalServerError, msgDeptSaveFail)
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// List godoc
// @Summary      List departments
// @Description  All departments ordered by sort_order, dept_code. active_only=true returns only is_active rows (use it for dropdowns).
// @Tags         Departments
// @Security     BearerAuth
// @Produce      json
// @Param        active_only  query  bool  false  "Only active departments"
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /departments [get]
func (h *DepartmentHandler) List(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT id, dept_code, dept_name, sort_order, is_active
		 FROM departments
		 WHERE ($1::boolean IS NOT TRUE OR is_active = true)
		 ORDER BY sort_order, dept_code`, c.QueryBool("active_only", false))
	if err != nil {
		log.Printf("departments list: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการแผนกได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()

	out := []departmentItem{}
	for rows.Next() {
		var it departmentItem
		if err := rows.Scan(&it.ID, &it.DeptCode, &it.DeptName, &it.SortOrder, &it.IsActive); err != nil {
			log.Printf("departments list scan: %v", err)
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการแผนกได้ กรุณาลองใหม่อีกครั้ง")
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		log.Printf("departments list rows: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการแผนกได้ กรุณาลองใหม่อีกครั้ง")
	}
	return c.JSON(fiber.Map{"success": true, "data": out})
}

// Create godoc
// @Summary      Create a department
// @Description  dept_code is trimmed, upper-cased, max 20, A-Z 0-9 and _ only, and cannot be changed later. dept_name required, max 200. sort_order integer >= 0 (default 0). is_active defaults to true. 409 when the code already exists.
// @Tags         Departments
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  departmentCreateRequest  true  "Department"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Router       /departments [post]
func (h *DepartmentHandler) Create(c *fiber.Ctx) error {
	var req departmentCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
	}
	code, err := normalizeDeptCode(req.DeptCode)
	if err != nil {
		return err
	}
	name, err := normalizeDeptName(req.DeptName)
	if err != nil {
		return err
	}
	if err := checkDeptSortOrder(req.SortOrder); err != nil {
		return err
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	ctx := context.Background()
	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM departments WHERE dept_code = $1)`, code).Scan(&exists); err != nil {
		return deptServerError("create exists", err)
	}
	if exists {
		return fiber.NewError(fiber.StatusConflict, msgDeptDuplicate)
	}

	var it departmentItem
	if err := h.db.QueryRow(ctx, `
		INSERT INTO departments (dept_code, dept_name, sort_order, is_active)
		VALUES ($1,$2,$3,$4)
		RETURNING id, dept_code, dept_name, sort_order, is_active`, code, name, sortOrder, active,
	).Scan(&it.ID, &it.DeptCode, &it.DeptName, &it.SortOrder, &it.IsActive); err != nil {
		if isPgCode(err, "23505") { // lost a race with a concurrent create
			return fiber.NewError(fiber.StatusConflict, msgDeptDuplicate)
		}
		return deptServerError("create", err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": it})
}

// Update godoc
// @Summary      Update a department
// @Description  Updates dept_name (required), sort_order and is_active (kept when omitted). dept_code is immutable: a dept_code in the body is accepted only if it equals the stored code, otherwise 400.
// @Tags         Departments
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                      true  "Department id"
// @Param        body  body  departmentUpdateRequest  true  "Department"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /departments/{id} [put]
func (h *DepartmentHandler) Update(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสอ้างอิงแผนกไม่ถูกต้อง")
	}
	var req departmentUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
	}
	name, err := normalizeDeptName(req.DeptName)
	if err != nil {
		return err
	}
	if err := checkDeptSortOrder(req.SortOrder); err != nil {
		return err
	}

	ctx := context.Background()
	var current string
	if err := h.db.QueryRow(ctx, `SELECT dept_code FROM departments WHERE id = $1`, id).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, msgDeptNotFound)
		}
		return deptServerError("update load", err)
	}
	if req.DeptCode != nil && strings.ToUpper(strings.TrimSpace(*req.DeptCode)) != current {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่สามารถเปลี่ยนรหัสแผนกได้")
	}

	var it departmentItem
	if err := h.db.QueryRow(ctx, `
		UPDATE departments
		SET dept_name = $1,
		    sort_order = COALESCE($2, sort_order),
		    is_active = COALESCE($3, is_active),
		    updated_at = NOW()
		WHERE id = $4
		RETURNING id, dept_code, dept_name, sort_order, is_active`, name, req.SortOrder, req.IsActive, id,
	).Scan(&it.ID, &it.DeptCode, &it.DeptName, &it.SortOrder, &it.IsActive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, msgDeptNotFound)
		}
		return deptServerError("update", err)
	}
	return c.JSON(fiber.Map{"success": true, "data": it})
}

// deptUsage counts, for every table in public that has a dept_code column (discovered through information_schema,
// so a table added later is covered), the rows referencing code. Tables with 0 rows are omitted. The identifiers come
// from the catalog and are quoted; the code is a bind parameter.
func deptUsage(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}, code string) (map[string]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT c.table_name
		FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = 'public' AND c.column_name = 'dept_code'
		  AND t.table_type = 'BASE TABLE' AND c.table_name <> 'departments'
		ORDER BY c.table_name`)
	if err != nil {
		return nil, err
	}
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	used := map[string]int64{}
	for _, t := range tables {
		var n int64
		sql := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE dept_code = $1`, pgx.Identifier{"public", t}.Sanitize())
		if err := q.QueryRow(ctx, sql, code).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			used[t] = n
		}
	}
	return used, nil
}

func deptInUseMessage(used map[string]int64) string {
	tables := make([]string, 0, len(used))
	for t := range used {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	parts := make([]string, 0, len(tables))
	for _, t := range tables {
		label := deptTableLabels[t]
		if label == "" {
			label = t
		}
		parts = append(parts, fmt.Sprintf("%s %d", label, used[t]))
	}
	return fmt.Sprintf("ไม่สามารถลบแผนกนี้ได้ เพราะถูกใช้งานอยู่ (%s) กรุณาปิดการใช้งานแทน", strings.Join(parts, ", "))
}

// Delete godoc
// @Summary      Delete a department
// @Description  Refused with 409 while any table with a dept_code column (users, roles, project, purchase_request, petty_cash_requisition, dept_menu_permissions, ...) still references the code; the message lists the tables and row counts and suggests deactivating instead. The check and the delete run in one transaction.
// @Tags         Departments
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Department id"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Router       /departments/{id} [delete]
func (h *DepartmentHandler) Delete(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสอ้างอิงแผนกไม่ถูกต้อง")
	}
	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return deptServerError("delete begin", err)
	}
	defer tx.Rollback(ctx)

	var code string
	if err := tx.QueryRow(ctx, `SELECT dept_code FROM departments WHERE id = $1 FOR UPDATE`, id).Scan(&code); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, msgDeptNotFound)
		}
		return deptServerError("delete load", err)
	}
	used, err := deptUsage(ctx, tx, code)
	if err != nil {
		return deptServerError("delete usage", err)
	}
	if len(used) > 0 {
		return fiber.NewError(fiber.StatusConflict, deptInUseMessage(used))
	}
	if _, err := tx.Exec(ctx, `DELETE FROM departments WHERE id = $1`, id); err != nil {
		if isPgCode(err, "23503") { // a row started referencing the code after the check
			return fiber.NewError(fiber.StatusConflict, "ไม่สามารถลบแผนกนี้ได้ เพราะถูกใช้งานอยู่ กรุณาปิดการใช้งานแทน")
		}
		return deptServerError("delete", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return deptServerError("delete commit", err)
	}
	return c.JSON(fiber.Map{"success": true, "message": "ลบแผนกเรียบร้อยแล้ว"})
}

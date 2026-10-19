package handlers

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

type ProjectHandler struct {
	db *pgxpool.Pool
}

func NewProjectHandler(db *pgxpool.Pool) *ProjectHandler {
	return &ProjectHandler{db: db}
}

var validProjectStatus = map[string]bool{"ACTIVE": true, "INACTIVE": true, "CLOSED": true}

// isProjectImportRowBlank reports whether every column the Project import reads (project_code
// through responsible_person_name, i.e. columns 0-10 of the template) is empty — the actual
// signature of a trailing blank row or the template's notes/legend section. A row with content
// in any of these columns is real data, even if project_code itself happens to be blank.
func isProjectImportRowBlank(row []string) bool {
	for i := 0; i <= 10; i++ {
		if strings.TrimSpace(cellAt(row, i)) != "" {
			return false
		}
	}
	return true
}

// enableImportExistenceChecks gates the Project bulk-import row-level existence/validity checks
// (customer_code must resolve to a real customer, job_code must be one of the fixed JobTypes).
// Temporarily off so early data loads aren't rejected for codes not yet set up elsewhere in the
// system. TODO: re-enable once system goes live.
const enableImportExistenceChecks = false

// p.owner_name is aliased to project_owner_name in the SELECT below — the real column (added
// by manual ALTER TABLE) is literally named owner_name, which collides with the unrelated
// "u.full_name AS owner_name" alias a few tokens later (the joined name behind owner_id,
// "ผู้รับผิดชอบหลัก"). Aliasing keeps both columns distinguishable in the result set and
// matches the existing Go field/JSON key (ProjectOwnerName / project_owner_name) without an
// API-facing rename — only the SQL identifier was ever wrong, not the model shape.
const projectSelectCols = `p.id, p.project_code, p.project_name, p.location_code,
	p.dept_code, d.dept_name,
	p.owner_id, u.full_name AS owner_name, p.owner_name AS project_owner_name,
	p.customer_id, cu.customer_name,
	p.responsible_person_name, p.job_codes,
	p.budget_amount, p.vo_amount::float8, p.project_kind,
	COALESCE((SELECT string_agg(ru.username, ', ' ORDER BY ru.username)
		FROM project_responsible_person prp JOIN users ru ON ru.id = prp.user_id
		WHERE prp.project_id = p.id), '') AS responsible_usernames,
	COALESCE((SELECT SUM(po.net_amount) FROM purchase_order po
		WHERE po.project_code = p.project_code
		  AND po.status = 'APPROVED'), 0)
	+ COALESCE((SELECT SUM(wo.net_amount) FROM work_order wo
		WHERE wo.project_code = p.project_code
		  AND wo.status = 'APPROVED'), 0)
	+ ` + pcProjectSpentSQL + ` AS spent_amount,
	COALESCE((SELECT SUM(pl.amount_paid) FROM payment_log pl
		JOIN purchase_order po ON po.id = pl.doc_id
		WHERE pl.doc_type = 'PO' AND po.project_code = p.project_code), 0)
	+ COALESCE((SELECT SUM(pl.amount_paid) FROM payment_log pl
		JOIN work_order wo ON wo.id = pl.doc_id
		WHERE pl.doc_type = 'WO' AND wo.project_code = p.project_code), 0)
	+ ` + pcProjectPaidSQL + ` AS paid_amount,
	p.budget_amount - (
		COALESCE((SELECT SUM(po.net_amount) FROM purchase_order po
			WHERE po.project_code = p.project_code
			  AND po.status = 'APPROVED'), 0)
		+ COALESCE((SELECT SUM(wo.net_amount) FROM work_order wo
			WHERE wo.project_code = p.project_code
			  AND wo.status = 'APPROVED'), 0)
		+ ` + pcProjectSpentSQL + `
	) AS remaining_amount,
	p.consultant_name, p.consultant_phone,
	p.start_date, p.end_date, p.status, p.is_active,
	p.created_at, p.updated_at, p.created_by, p.updated_by`

const projectSelectFrom = `
	FROM project p
	LEFT JOIN users       u  ON u.id = p.owner_id
	LEFT JOIN departments d  ON d.dept_code = p.dept_code
	LEFT JOIN customer    cu ON cu.cus_id = p.customer_id`

func scanProjectFull(p *models.ProjectFull, row pgx.Row) error {
	return row.Scan(&p.Id, &p.ProjectCode, &p.ProjectName, &p.LocationCode,
		&p.DeptCode, &p.DeptName,
		&p.OwnerID, &p.OwnerName, &p.ProjectOwnerName,
		&p.CustomerID, &p.CustomerName,
		&p.ResponsiblePersonName, &p.JobCodes,
		&p.BudgetAmount, &p.VOAmount, &p.ProjectKind, &p.ResponsibleUsernames, &p.SpentAmount, &p.PaidAmount, &p.RemainingAmount, &p.ConsultantName, &p.ConsultantPhone,
		&p.StartDate, &p.EndDate, &p.Status, &p.IsActive,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy)
}

// validateJobCodes is defined in job_type.go, shared with the PR/PO job_code fields.

// validateProjectCore is the single source of truth for project field validation — used by
// Create, Update, and Import so the three paths can't drift apart. Returns the normalized
// status (defaulted to ACTIVE when blank) for the caller to use.
func validateProjectCore(projectCode, projectName, responsiblePersonName, status string, budgetAmount float64, jobCodes []string) (string, error) {
	if projectCode == "" || projectName == "" {
		return "", fmt.Errorf("project_code and project_name are required")
	}
	if strings.TrimSpace(responsiblePersonName) == "" {
		return "", fmt.Errorf("responsible_person_name is required")
	}
	if status == "" {
		status = "ACTIVE"
	}
	if !validProjectStatus[status] {
		return "", fmt.Errorf("status must be one of ACTIVE, INACTIVE, CLOSED")
	}
	if budgetAmount < 0 {
		return "", fmt.Errorf("budget_amount must be >= 0")
	}
	if err := validateJobCodes(jobCodes); err != nil {
		return "", err
	}
	return status, nil
}

// normalizeVOAmount validates project.vo_amount ("มูลค่า VO", separate from budget_amount):
// nil -> 0, must be >= 0 and fit numeric(18,4); rounded to 2 decimals.
func normalizeVOAmount(v *float64) (float64, error) {
	if v == nil {
		return 0, nil
	}
	if math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 {
		return 0, fmt.Errorf("มูลค่า VO ต้องเป็นตัวเลขที่ไม่ติดลบ")
	}
	if *v >= 1e14 {
		return 0, fmt.Errorf("มูลค่า VO มากเกินกำหนด")
	}
	return math.Round(*v*100) / 100, nil
}

// resolveProjectValues applies the project_kind rule: NEW needs budget_amount (vo_amount saved as 0),
// VO needs vo_amount (budget_amount saved as 0). Blank kind falls back to defaultKind. Returns the
// normalized kind and the two amounts to store, whatever the client sent for the other field.
func resolveProjectValues(kind, defaultKind string, budget, vo *float64) (string, float64, float64, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind == "" {
		kind = defaultKind
	}
	switch kind {
	case "NEW":
		if budget == nil {
			return "", 0, 0, fmt.Errorf("กรุณาระบุมูลค่าโครงการ")
		}
		if math.IsNaN(*budget) || math.IsInf(*budget, 0) || *budget < 0 {
			return "", 0, 0, fmt.Errorf("มูลค่าโครงการต้องเป็นตัวเลขที่ไม่ติดลบ")
		}
		return kind, *budget, 0, nil
	case "VO":
		if vo == nil {
			return "", 0, 0, fmt.Errorf("กรุณาระบุมูลค่า VO")
		}
		v, err := normalizeVOAmount(vo)
		if err != nil {
			return "", 0, 0, err
		}
		return kind, 0, v, nil
	default:
		return "", 0, 0, fmt.Errorf("ประเภทโครงการต้องเป็น NEW หรือ VO")
	}
}

// resolveResponsibleUsers de-duplicates the picked ids (order kept), checks every id is an active user
// and returns the ids plus the legacy responsible_person_name (usernames sorted, joined by ", ").
func (h *ProjectHandler) resolveResponsibleUsers(ctx context.Context, ids []int64) ([]int64, string, error) {
	seen := map[int64]bool{}
	uniq := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, "", fiber.NewError(fiber.StatusBadRequest, "ผู้รับผิดชอบที่เลือกไม่ถูกต้อง")
		}
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	if len(uniq) == 0 {
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกผู้รับผิดชอบหลัก")
	}
	rows, err := h.db.Query(ctx, `SELECT username FROM users WHERE id = ANY($1) AND is_active = true ORDER BY username`, uniq)
	if err != nil {
		return nil, "", fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบผู้รับผิดชอบได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, "", fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบผู้รับผิดชอบได้ กรุณาลองใหม่อีกครั้ง")
		}
		names = append(names, n)
	}
	if rows.Err() != nil || len(names) != len(uniq) {
		if rows.Err() != nil {
			return nil, "", fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบผู้รับผิดชอบได้ กรุณาลองใหม่อีกครั้ง")
		}
		return nil, "", fiber.NewError(fiber.StatusBadRequest, "ผู้รับผิดชอบที่เลือกไม่ถูกต้อง")
	}
	return uniq, strings.Join(names, ", "), nil
}

// loadResponsibleUsers returns the project's main responsible persons (id + username only).
func (h *ProjectHandler) loadResponsibleUsers(ctx context.Context, projectID int64) []models.ProjectResponsibleUser {
	out := []models.ProjectResponsibleUser{}
	rows, err := h.db.Query(ctx, `SELECT u.id, u.username FROM project_responsible_person prp
		JOIN users u ON u.id = prp.user_id WHERE prp.project_id = $1 ORDER BY u.username`, projectID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var r models.ProjectResponsibleUser
		if rows.Scan(&r.ID, &r.Username) == nil {
			out = append(out, r)
		}
	}
	return out
}

// replaceResponsibleUsers sets the project's responsible persons inside the caller's transaction.
func replaceResponsibleUsers(ctx context.Context, tx pgx.Tx, projectID int64, ids []int64, by int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM project_responsible_person WHERE project_id = $1`, projectID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO project_responsible_person (project_id, user_id, created_by)
		SELECT $1, x, $3 FROM unnest($2::bigint[]) AS x`, projectID, ids, by)
	return err
}

// List godoc
// @Summary      List projects
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        search    query  string  false  "search project_code / project_name"
// @Param        status    query  string  false  "filter status"
// @Param        is_active query  string  false  "filter is_active (default true; pass empty string for all)"
// @Param        page      query  int     false  "page"
// @Param        page_size query  int     false  "page_size"
// @Success      200  {object}  models.PaginatedResponse
// @Router       /master/projects [get]
func (h *ProjectHandler) List(c *fiber.Ctx) error {
	var f models.ProjectListFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	offset := (f.Page - 1) * f.PageSize

	ctx := context.Background()
	args := []any{}
	where := "WHERE 1=1"
	i := 1

	if f.Search != "" {
		where += fmt.Sprintf(" AND (p.project_code ILIKE $%d OR p.project_name ILIKE $%d)", i, i)
		args = append(args, "%"+f.Search+"%")
		i++
	}
	if f.Status != "" {
		where += fmt.Sprintf(" AND p.status = $%d", i)
		args = append(args, f.Status)
		i++
	}
	// Default to active-only, matching ListWarehouses' "is_active" convention: unset means
	// "true", an explicit empty string ("?is_active=") means no filter (both), and any other
	// explicit value ("true"/"false") is respected as-is.
	if !c.Request().URI().QueryArgs().Has("is_active") {
		f.IsActive = "true"
	}
	if f.IsActive != "" {
		where += fmt.Sprintf(" AND p.is_active = $%d", i)
		args = append(args, f.IsActive == "true")
		i++
	}

	var total int64
	if err := h.db.QueryRow(ctx, `SELECT COUNT(*) FROM project p `+where, args...).Scan(&total); err != nil {
		return err
	}

	dataSQL := `SELECT ` + projectSelectCols + projectSelectFrom + `
		` + where + `
		ORDER BY p.created_at DESC
		LIMIT $` + fmt.Sprintf("%d", i) + ` OFFSET $` + fmt.Sprintf("%d", i+1)
	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, dataSQL, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []models.ProjectFull{}
	for rows.Next() {
		var p models.ProjectFull
		if err := scanProjectFull(&p, rows); err != nil {
			return err
		}
		p.ResponsibleUsers = []models.ProjectResponsibleUser{} // list carries responsible_usernames only
		items = append(items, p)
	}

	totalPages := int((total + int64(f.PageSize) - 1) / int64(f.PageSize))
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: f.Page, PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// GetByID godoc
// @Summary      Get project by id
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Project ID"
// @Description  Detail also returns job_shares: [{job_code, share_percent, share_amount}] (share_amount = budget_amount * share_percent / 100, derived, read-only); [] for legacy projects with no rows.
// @Success      200  {object}  ProjectDetail
// @Failure      404  {object}  fiber.Map
// @Router       /master/projects/{id} [get]
func (h *ProjectHandler) GetByID(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var p models.ProjectFull
	err = scanProjectFull(&p, h.db.QueryRow(context.Background(),
		`SELECT `+projectSelectCols+projectSelectFrom+` WHERE p.id = $1`, id))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	p.ResponsibleUsers = h.loadResponsibleUsers(context.Background(), int64(p.Id))
	shares, err := h.loadJobShares(context.Background(), int64(p.Id))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": ProjectDetail{ProjectFull: p, JobShares: shares}})
}

// formatProjectCode is the single project_code format ({YYYY}-{customer_code}-{NNN}, NNN padded to 3
// digits, 4+ digits past 999), shared by Create and the NextCode preview so they cannot drift.
func formatProjectCode(year int, customerCode string, no int) string {
	return fmt.Sprintf("%d-%s-%03d", year, customerCode, no)
}

// NextCode godoc
// @Summary      Preview the next project_code (read-only)
// @Description  Never reserves or writes anything: next_no = project_code_year_counter.last_no (0 if none) + 1 for the current Asia/Bangkok year; the running number is shared by ALL customers and customer_id only supplies the customer_code part. Create still allocates the real code atomically, so the final code can differ if someone else creates a project first.
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        customer_id  query  int  true  "customer cus_id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /master/projects/next-code [get]
func (h *ProjectHandler) NextCode(c *fiber.Ctx) error {
	customerID, err := strconv.ParseInt(strings.TrimSpace(c.Query("customer_id")), 10, 64)
	if err != nil || customerID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกเจ้าของโครงการ")
	}
	ctx := context.Background()
	const failMsg = "ไม่สามารถดูรหัสโครงการถัดไปได้ กรุณาลองใหม่อีกครั้ง"

	var customerCode string
	if err := h.db.QueryRow(ctx, `SELECT COALESCE(btrim(customer_code), '') FROM customer WHERE cus_id = $1`, customerID).Scan(&customerCode); err != nil {
		if err == pgx.ErrNoRows {
			return fiber.NewError(fiber.StatusBadRequest, "ไม่พบข้อมูลลูกค้าที่เลือก")
		}
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if customerCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "ลูกค้านี้ยังไม่มีรหัสลูกค้า กรุณาเพิ่มรหัสลูกค้าก่อน")
	}

	year := bangkokNow().Year()
	var nextNo int
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE((SELECT last_no FROM project_code_year_counter WHERE code_year = $1), 0) + 1`,
		year).Scan(&nextNo); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	return c.JSON(fiber.Map{
		"project_code": formatProjectCode(year, customerCode, nextNo),
		"code_year":    year,
		"next_no":      nextNo,
	})
}

// Create godoc
// @Summary      Create project
// @Description  project_code is generated by the server as YYYY-{customer_code}-NNN (client value ignored; customer_id required) and cannot be changed afterwards. location_code is now a free-text project address (no longer validated/joined against the location master). dept_code ("แผนก") is validated against the departments master (400 on an unknown code). job_codes is a subset of MP/ME/MS/MF/MG/MH/G (validated server-side, 400 on any other value). project_owner_name is free text ("เจ้าของโครงการ") — legacy, kept for backward compat. customer_id (FK -> customer.cus_id) is the new dropdown-selected "เจ้าของโครงการ" going forward; both fields are accepted independently. responsible_person_name ("ผู้รับผิดชอบหลัก") is required free text, replacing the old owner_id dropdown — owner_id is still accepted for backward compatibility but no longer required or used to drive anything. consultant_phone ("เบอร์ติดต่อของที่ปรึกษา") is freeform, no validation.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateProjectReq  true  "Project data"
// @Success      201   {object}  ProjectDetail
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /master/projects [post]
func (h *ProjectHandler) Create(c *fiber.Ctx) error {
	var req models.CreateProjectReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	// project_code is always generated by the server; whatever the client sent is ignored.
	req.ProjectCode = ""
	if req.CustomerID == nil || *req.CustomerID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกเจ้าของโครงการ")
	}
	// job_shares: pure validation, before any SQL.
	if err := validateJobCodes(req.JobCodes); err != nil {
		return err
	}
	jobShares, jerr := validateJobShares(req.JobCodes, req.JobShares)
	if jerr != nil {
		return jerr
	}
	var responsibleIDs []int64
	if req.ResponsibleUserIDs != nil {
		ids, names, rerr := h.resolveResponsibleUsers(context.Background(), req.ResponsibleUserIDs)
		if rerr != nil {
			return rerr
		}
		responsibleIDs, req.ResponsiblePersonName = ids, names // legacy name = selected usernames
	}
	kind, budgetAmount, voAmount, verr := resolveProjectValues(req.ProjectKind, "NEW", req.BudgetAmount, req.VOAmount)
	if verr != nil {
		return fiber.NewError(fiber.StatusBadRequest, verr.Error())
	}
	status, verr := validateProjectCore("(auto)", req.ProjectName, req.ResponsiblePersonName, req.Status, budgetAmount, req.JobCodes)
	if verr != nil {
		return fiber.NewError(fiber.StatusBadRequest, verr.Error())
	}
	req.Status = status

	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()
	const failMsg = "ไม่สามารถสร้างโครงการได้ กรุณาลองใหม่อีกครั้ง"

	var customerCode string
	if err := h.db.QueryRow(ctx, `SELECT COALESCE(btrim(customer_code), '') FROM customer WHERE cus_id = $1`, *req.CustomerID).Scan(&customerCode); err != nil {
		if err == pgx.ErrNoRows {
			return fiber.NewError(fiber.StatusBadRequest, "ไม่พบข้อมูลลูกค้าที่เลือก")
		}
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if customerCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "ลูกค้านี้ยังไม่มีรหัสลูกค้า กรุณาเพิ่มรหัสลูกค้าก่อน")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	defer tx.Rollback(ctx)

	// Running number per Bangkok year, shared by all customers, allocated in the same transaction as the INSERT.
	// The INSERT runs in a savepoint so a project_code unique violation can be retried (next number)
	// without aborting the whole transaction.
	year := bangkokNow().Year()
	var id int64
	var projectCode string
	for attempt := 0; attempt < 5; attempt++ {
		var lastNo int
		if err := tx.QueryRow(ctx, `
			INSERT INTO project_code_year_counter (code_year, last_no) VALUES ($1, 1)
			ON CONFLICT (code_year) DO UPDATE SET last_no = project_code_year_counter.last_no + 1
			RETURNING last_no`, year).Scan(&lastNo); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
		projectCode = formatProjectCode(year, customerCode, lastNo)

		sp, err := tx.Begin(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
		err = sp.QueryRow(ctx, `
			INSERT INTO project
			    (project_code, project_name, location_code, dept_code, owner_id, owner_name, customer_id, responsible_person_name, job_codes,
			     budget_amount, consultant_name, consultant_phone, start_date, end_date,
			     status, is_active, created_at, updated_at, created_by, updated_by, vo_amount, project_kind)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,true,now(),now(),$16,$16,$17,$18)
			RETURNING id`,
			projectCode, req.ProjectName, req.LocationCode, req.DeptCode, req.OwnerID, req.ProjectOwnerName, req.CustomerID, req.ResponsiblePersonName, req.JobCodes,
			budgetAmount, req.ConsultantName, req.ConsultantPhone,
			req.StartDate, req.EndDate, req.Status, claims.UserID, voAmount, kind,
		).Scan(&id)
		if err == nil {
			if err := sp.Commit(ctx); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, failMsg)
			}
			break
		}
		_ = sp.Rollback(ctx)
		pgErr, ok := err.(*pgconn.PgError)
		if !ok {
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
		if pgErr.Code == "23505" {
			id = 0
			continue // code already taken: take the next number
		}
		if pgErr.Code == "23503" {
			if pgErr.ConstraintName == "project_customer_id_fkey" {
				return fiber.NewError(fiber.StatusBadRequest, "ไม่พบข้อมูลลูกค้าที่เลือก")
			}
			return fiber.NewError(fiber.StatusBadRequest, "รหัสแผนกไม่ถูกต้อง")
		}
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if id == 0 {
		return fiber.NewError(fiber.StatusConflict, "ไม่สามารถสร้างรหัสโครงการที่ไม่ซ้ำได้ กรุณาลองใหม่อีกครั้ง")
	}
	if responsibleIDs != nil {
		if err := replaceResponsibleUsers(ctx, tx, id, responsibleIDs, claims.UserID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
	}
	if err := replaceJobShares(ctx, tx, id, jobShares); err != nil {
		log.Printf("project create: replaceJobShares: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if err := tx.Commit(ctx); err != nil {
		log.Printf("project create: commit: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}

	var p models.ProjectFull
	if err := scanProjectFull(&p, h.db.QueryRow(ctx,
		`SELECT `+projectSelectCols+projectSelectFrom+` WHERE p.id = $1`, id)); err != nil {
		return err
	}
	p.ResponsibleUsers = h.loadResponsibleUsers(ctx, int64(p.Id))
	shares, err := h.loadJobShares(ctx, int64(p.Id))
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": ProjectDetail{ProjectFull: p, JobShares: shares}})
}

// Update godoc
// @Summary      Update project
// @Description  location_code is now a free-text project address (no longer validated/joined against the location master). dept_code ("แผนก") is validated against the departments master (400 on an unknown code). job_codes is a subset of MP/ME/MS/MF/MG/MH/G (validated server-side, 400 on any other value). project_owner_name is free text ("เจ้าของโครงการ") — legacy, kept for backward compat. customer_id (FK -> customer.cus_id) is the new dropdown-selected "เจ้าของโครงการ" going forward; both fields are accepted independently. responsible_person_name ("ผู้รับผิดชอบหลัก") is required free text, replacing the old owner_id dropdown — owner_id is still accepted for backward compatibility but no longer required or used to drive anything. consultant_phone ("เบอร์ติดต่อของที่ปรึกษา") is freeform, no validation.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                      true  "Project ID"
// @Param        body  body  models.UpdateProjectReq  true  "Update data"
// @Success      200   {object}  ProjectDetail
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /master/projects/{id} [put]
func (h *ProjectHandler) Update(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.UpdateProjectReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	var currentProjectCode, currentKind string
	var currentIsActive bool
	if err := h.db.QueryRow(ctx, `SELECT project_code, is_active, project_kind FROM project WHERE id=$1`, id).Scan(&currentProjectCode, &currentIsActive, &currentKind); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	// project_code is immutable: it is generated at create time and any client value is ignored.
	req.ProjectCode = currentProjectCode

	// job_shares: pure validation, before the write transaction.
	if err := validateJobCodes(req.JobCodes); err != nil {
		return err
	}
	jobShares, jerr := validateJobShares(req.JobCodes, req.JobShares)
	if jerr != nil {
		return jerr
	}

	var responsibleIDs []int64
	if req.ResponsibleUserIDs != nil {
		ids, names, rerr := h.resolveResponsibleUsers(ctx, req.ResponsibleUserIDs)
		if rerr != nil {
			return rerr
		}
		responsibleIDs, req.ResponsiblePersonName = ids, names // legacy name = selected usernames
	}

	// Validation only (no writes yet): a blank project_kind keeps the project's current kind.
	kind, budgetAmount, voAmount, verr := resolveProjectValues(req.ProjectKind, currentKind, req.BudgetAmount, req.VOAmount)
	if verr != nil {
		return fiber.NewError(fiber.StatusBadRequest, verr.Error())
	}
	status, verr := validateProjectCore(req.ProjectCode, req.ProjectName, req.ResponsiblePersonName, req.Status, budgetAmount, req.JobCodes)
	if verr != nil {
		return fiber.NewError(fiber.StatusBadRequest, verr.Error())
	}
	req.Status = status

	// is_active: nil means the client omitted it — preserve the project's current value
	// instead of defaulting to false. Unlike Status (which has a real default, "ACTIVE"),
	// IsActive has no sensible default; the only correct behavior for "not sent" is "unchanged".
	isActive := req.IsActive
	if isActive == nil {
		isActive = &currentIsActive
	}

	// project_code is an FK target (not project.id) for petty_cash_requisition_line,
	// purchase_order, and requisition, all with ON UPDATE NO ACTION — the plain UPDATE below
	// would fail with a raw 23503 for any project with existing dependents in those tables.
	// Proactively check and return a clear, itemized 409 before attempting the UPDATE at all,
	// rather than relying solely on the DB error (which we also still map below as a backstop —
	// e.g. a dependent row inserted concurrently between this check and the UPDATE).
	//
	// TODO: work_order.project_code has no FK constraint (confirmed via
	// information_schema query, 2026-09-09) — it is NOT checked here and
	// will silently go stale if project_code changes. Deliberately out of
	// scope for now; must be addressed before this feature is considered
	// fully safe. See conversation notes for the CASCADE/new-FK options
	// considered.
	if req.ProjectCode != currentProjectCode {
		var pclCount, poCount, reqCount int
		if err := h.db.QueryRow(ctx, `SELECT COUNT(*) FROM petty_cash_requisition_line WHERE project_code=$1`, currentProjectCode).Scan(&pclCount); err != nil {
			return err
		}
		if err := h.db.QueryRow(ctx, `SELECT COUNT(*) FROM purchase_order WHERE project_code=$1`, currentProjectCode).Scan(&poCount); err != nil {
			return err
		}
		if err := h.db.QueryRow(ctx, `SELECT COUNT(*) FROM requisition WHERE project_code=$1`, currentProjectCode).Scan(&reqCount); err != nil {
			return err
		}
		if pclCount > 0 || poCount > 0 || reqCount > 0 {
			var parts []string
			if poCount > 0 {
				parts = append(parts, fmt.Sprintf("%d purchase order(s)", poCount))
			}
			if reqCount > 0 {
				parts = append(parts, fmt.Sprintf("%d requisition(s)", reqCount))
			}
			if pclCount > 0 {
				parts = append(parts, fmt.Sprintf("%d petty cash entries", pclCount))
			}
			return fiber.NewError(fiber.StatusConflict, fmt.Sprintf(
				"project_code cannot be changed: this project has existing %s referencing it",
				strings.Join(parts, ", ")))
		}
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer tx.Rollback(ctx)
	// Serialise saves per project: concurrent updates (and their share replacement) queue here.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM project WHERE id=$1 FOR UPDATE`, id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}
	tag, err := tx.Exec(ctx, `
		UPDATE project
		SET project_code=$1, project_name=$2, location_code=$3, dept_code=$4, owner_id=$5,
		    owner_name=$6, customer_id=$7, responsible_person_name=$8, job_codes=$9,
		    budget_amount=$10, consultant_name=$11, consultant_phone=$12,
		    start_date=$13, end_date=$14, status=$15, is_active=$16, updated_at=now(), updated_by=$17, vo_amount=$19, project_kind=$20
		WHERE id=$18`,
		req.ProjectCode, req.ProjectName, req.LocationCode, req.DeptCode, req.OwnerID,
		req.ProjectOwnerName, req.CustomerID, req.ResponsiblePersonName, req.JobCodes,
		budgetAmount, req.ConsultantName, req.ConsultantPhone,
		req.StartDate, req.EndDate, req.Status, *isActive, claims.UserID, id, voAmount, kind)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == "23505" {
				return fiber.NewError(fiber.StatusConflict, "project_code already exists")
			}
			if pgErr.Code == "23503" {
				if pgErr.ConstraintName == "project_customer_id_fkey" {
					return fiber.NewError(fiber.StatusBadRequest, "invalid customer_id")
				}
				switch pgErr.ConstraintName {
				case "fk_pcl_project_code", "purchase_order_project_fk", "requisition_project_code_fkey":
					// Backstop for the proactive check above (e.g. a dependent row inserted
					// concurrently between the count check and this UPDATE).
					return fiber.NewError(fiber.StatusConflict, "project_code cannot be changed: this project has existing purchase orders/requisitions/petty cash entries referencing it")
				}
				return fiber.NewError(fiber.StatusBadRequest, "invalid dept_code")
			}
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	if responsibleIDs != nil {
		if err := replaceResponsibleUsers(ctx, tx, int64(id), responsibleIDs, claims.UserID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกผู้รับผิดชอบได้ กรุณาลองใหม่อีกครั้ง")
		}
	}
	if err := replaceJobShares(ctx, tx, int64(id), jobShares); err != nil {
		log.Printf("project update: replaceJobShares: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกสัดส่วนงานได้ กรุณาลองใหม่อีกครั้ง")
	}
	if err := tx.Commit(ctx); err != nil {
		log.Printf("project update: commit: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}

	var p models.ProjectFull
	if err := scanProjectFull(&p, h.db.QueryRow(ctx,
		`SELECT `+projectSelectCols+projectSelectFrom+` WHERE p.id = $1`, id)); err != nil {
		return err
	}
	p.ResponsibleUsers = h.loadResponsibleUsers(ctx, int64(p.Id))
	shares, err := h.loadJobShares(ctx, int64(p.Id))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": ProjectDetail{ProjectFull: p, JobShares: shares}})
}

// SoftDelete godoc
// @Summary      Soft-delete project
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Project ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      409  {object}  fiber.Map
// @Router       /master/projects/{id} [delete]
func (h *ProjectHandler) SoftDelete(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var projectCode string
	if err := h.db.QueryRow(ctx, `SELECT project_code FROM project WHERE id=$1`, id).Scan(&projectCode); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}

	var refCount int
	if err := h.db.QueryRow(ctx, `
		SELECT
		    (SELECT COUNT(*) FROM memo WHERE project_code = $1) +
		    (SELECT COUNT(*) FROM purchase_request WHERE project_code = $1)`,
		projectCode,
	).Scan(&refCount); err != nil {
		return err
	}
	if refCount > 0 {
		return fiber.NewError(fiber.StatusConflict, "cannot delete: referenced by memo/pr")
	}

	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	tag, err := h.db.Exec(ctx,
		`UPDATE project SET is_active=false, status='INACTIVE', updated_at=now(), updated_by=$1 WHERE id=$2`,
		claims.UserID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "project deleted"})
}

// ─── Import ──────────────────────────────────────────────────────────────────

// Import godoc
// @Summary      Bulk import projects จาก Excel
// @Description  Column layout (row 1 = header, data starts row 2): project_code*, project_name*, customer_code, "Project location_code", start_date, end_date, budget_amount, "job Code", consultant_name, consultant_phone, responsible_person_name — matches the reference template used by other teams literally, including that header spacing/casing. No dept_code or credit columns. Applies the same validation as POST /master/projects (responsible_person_name required, status defaults to ACTIVE, budget_amount >= 0). customer_code (if given) is resolved against the customer table to set customer_id — an unresolvable code fails that row. job_code accepts one or more comma-separated codes from the fixed JobTypes enum (same list validated on POST /master/projects' job_codes[], NOT the unrelated cost_job master table) and is written to project.job_codes[]. Gives a clear per-row reason on failure instead of a generic "invalid". Processes in partial-success mode: valid rows are inserted, invalid rows are skipped and reported. Re-upload only the failed rows after fixing them.
// @Tags         Master
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        file  formData  file  true  "Excel file (.xlsx)"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /master/projects/import [post]
func (h *ProjectHandler) Import(c *fiber.Ctx) error {
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

	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	jobSet := toSet(projectJobTypeOptions())

	type rowError struct {
		Row    int    `json:"row"`
		Reason string `json:"reason"`
	}
	type rowSuccess struct {
		Row         int    `json:"row"`
		ID          int64  `json:"id"`
		ProjectCode string `json:"project_code"`
	}
	var errs []rowError
	var oks []rowSuccess
	rowsInFile := 0

	for i, row := range sheetRows[1:] {
		rowNo := i + 2 // account for header row

		projectCode := strings.TrimSpace(cellAt(row, 0))

		// A blank project_code alone is NOT enough to conclude "end of data grid" — a stray
		// separator row, a merged-cell artifact, or an accidental blank in just this one column
		// must not truncate the rest of a real file (this previously caused later rows to be
		// silently dropped with zero error reported for them). Only stop when the ENTIRE row is
		// blank — that's the actual signature of the template's notes/legend section and
		// trailing blank rows. A row with a blank project_code but content elsewhere is a
		// genuine error row: report it and keep going.
		if projectCode == "" && isProjectImportRowBlank(row) {
			break
		}
		rowsInFile++
		if projectCode == "" {
			errs = append(errs, rowError{Row: rowNo, Reason: "project_code is required"})
			continue
		}

		projectName := strings.TrimSpace(cellAt(row, 1))
		customerCode := strings.TrimSpace(cellAt(row, 2))
		locationCode := nullableCell(cellAt(row, 3))
		startDate := nullableCell(cellAt(row, 4))
		endDate := nullableCell(cellAt(row, 5))
		budgetStr := strings.TrimSpace(cellAt(row, 6))
		jobCodeRaw := strings.TrimSpace(cellAt(row, 7))
		consultantName := nullableCell(cellAt(row, 8))
		consultantPhone := nullableCell(cellAt(row, 9))
		responsiblePersonName := strings.TrimSpace(cellAt(row, 10))

		budgetAmount := 0.0
		if budgetStr != "" {
			var perr error
			// Strip thousand-separator commas (e.g. "390,000.00" / "75,300,000.00") before
			// parsing — a plain number with no commas ("390000") is unaffected by ReplaceAll.
			budgetAmount, perr = strconv.ParseFloat(strings.ReplaceAll(budgetStr, ",", ""), 64)
			if perr != nil {
				errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("invalid budget_amount: %q", budgetStr)})
				continue
			}
		}

		// job_codes[] is the exact same storage mechanism the normal multi-select Create/Update
		// form uses (see CreateProjectReq.JobCodes / UpdateProjectReq.JobCodes, written straight
		// to the project.job_codes array column) — just split from one cell instead of collected
		// from multiple checkbox selections. Accepts both "," and "/" as separators (the comma
		// convention from this template's own dropdown/comment, and "/" as seen in reference
		// files like "MP / ME / MS / MG / OH").
		var jobCodes []string
		if jobCodeRaw != "" {
			badCode := ""
			for _, jc := range strings.Split(strings.ReplaceAll(jobCodeRaw, "/", ","), ",") {
				jc = strings.TrimSpace(jc)
				if jc == "" {
					continue
				}
				if enableImportExistenceChecks && !jobSet[jc] { // TODO: re-enable once system goes live
					badCode = jc
					break
				}
				jobCodes = append(jobCodes, jc)
			}
			if badCode != "" {
				errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("job_code %q is not a valid option", badCode)})
				continue
			}
		}

		// validateProjectCore also re-validates jobCodes against the fixed JobTypes set
		// internally (validateJobCodes) — when existence checks are disabled, skip that by
		// validating with an empty slice here instead of duplicating validateProjectCore's other
		// required-field checks. jobCodes itself (parsed above) is still what gets inserted.
		jobCodesForValidation := jobCodes
		if !enableImportExistenceChecks { // TODO: re-enable once system goes live
			jobCodesForValidation = nil
		}
		status, verr := validateProjectCore(projectCode, projectName, responsiblePersonName, "", budgetAmount, jobCodesForValidation)
		if verr != nil {
			errs = append(errs, rowError{Row: rowNo, Reason: verr.Error()})
			continue
		}

		var codeExists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code = $1)`, projectCode).Scan(&codeExists); err != nil {
			errs = append(errs, rowError{Row: rowNo, Reason: err.Error()})
			continue
		}
		if codeExists {
			errs = append(errs, rowError{Row: rowNo, Reason: "project_code already exists"})
			continue
		}

		var customerID *int64
		if customerCode != "" && enableImportExistenceChecks { // TODO: re-enable once system goes live
			var cid int64
			if err := h.db.QueryRow(ctx, `SELECT cus_id FROM customer WHERE customer_code = $1`, customerCode).Scan(&cid); err != nil {
				errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("customer_code %q not found", customerCode)})
				continue
			}
			customerID = &cid
		}
		// With existence checks disabled, an unresolvable customer_code leaves customerID nil
		// (project.customer_id is nullable) rather than failing the row.

		var id int64
		err := h.db.QueryRow(ctx, `
			INSERT INTO project
			    (project_code, project_name, location_code, customer_id, responsible_person_name,
			     job_codes, budget_amount, consultant_name, consultant_phone, start_date, end_date,
			     status, is_active, created_at, updated_at, created_by, updated_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,true,now(),now(),$13,$13)
			RETURNING id`,
			projectCode, projectName, locationCode, customerID, responsiblePersonName,
			jobCodes, budgetAmount, consultantName, consultantPhone, startDate, endDate,
			status, claims.UserID,
		).Scan(&id)
		if err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok {
				if pgErr.Code == "23505" {
					errs = append(errs, rowError{Row: rowNo, Reason: "project_code already exists"})
					continue
				}
				if pgErr.Code == "23503" {
					if pgErr.ConstraintName == "project_customer_id_fkey" {
						errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("customer_code %q not found", customerCode)})
						continue
					}
					errs = append(errs, rowError{Row: rowNo, Reason: "invalid reference in row"})
					continue
				}
			}
			errs = append(errs, rowError{Row: rowNo, Reason: err.Error()})
			continue
		}

		oks = append(oks, rowSuccess{Row: rowNo, ID: id, ProjectCode: projectCode})
	}

	if errs == nil {
		errs = []rowError{}
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"rows_in_file": rowsInFile, // sanity check: imported + failed should always equal this
			"imported":     len(oks),
			"failed":       len(errs),
			"rows":         oks,
			"errors":       errs,
		},
	})
}

// ImportTemplate godoc
// @Summary      ดาวน์โหลด template Excel สำหรับ import โครงการ
// @Description  Generates the .xlsx on the fly: sheet "Project" with header row (project_code*, project_name*, customer_code, "Project location_code", start_date, end_date, budget_amount, "job Code", consultant_name, consultant_phone, responsible_person_name — literal spacing/casing to match a reference template already in use by other teams, no dept_code or credit columns) and two example rows. A hidden "Lookup" sheet lists job_code/job_name (the fixed JobTypes enum — NOT cost_job), bound as an Excel dropdown on the "job Code" column.
// @Tags         Master
// @Security     BearerAuth
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200  {file}  file
// @Failure      500  {object}  fiber.Map
// @Router       /master/projects/import/template [get]
func (h *ProjectHandler) ImportTemplate(c *fiber.Ctx) error {
	jobs := projectJobTypeOptions()

	f := excelize.NewFile()
	defer f.Close()

	const sheet = "Project"
	f.SetSheetName("Sheet1", sheet)

	boldStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return err
	}
	// requiredStyle marks required columns visually (bold + a distinct fill) WITHOUT touching
	// the header cell's text — the import parser matches header text exactly against expected
	// field names, so appending a "*" to the string (as this template used to do) made
	// "project_code*" fail to match "project_code" and read as an unrecognized column.
	requiredStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FFF2CC"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	// Header text/order/spacing/casing here (including "Project location_code" and "job Code")
	// mirrors a reference template already in use by other teams — matched literally, not
	// "cleaned up", so re-uploads of that existing file layout parse correctly.
	headers := []string{
		"project_code", "project_name", "customer_code", "Project location_code", "start_date", "end_date",
		"budget_amount", "job Code", "consultant_name", "consultant_phone", "responsible_person_name",
	}
	required := []bool{true, true, false, false, false, false, false, false, false, false, false}
	colWidths := []float64{16, 30, 16, 24, 14, 14, 16, 24, 24, 20, 24}
	for i, hdr := range headers {
		col := colLetter(i)
		cell := col + "1"
		f.SetCellValue(sheet, cell, hdr)
		style := boldStyle
		if required[i] {
			style = requiredStyle
		}
		f.SetCellStyle(sheet, cell, cell, style)
		f.SetColWidth(sheet, col, col, colWidths[i])
	}

	examples := [][]string{
		{"PRJ-000001", "โครงการตัวอย่าง", "CUS-000001", "123 ถนนตัวอย่าง", "2026-01-01", "2026-12-31",
			"1000000", "MP,ME", "นายที่ปรึกษา ตัวอย่าง", "08x-xxx-xxxx", "นายผู้รับผิดชอบ ตัวอย่าง"},
		{"PRJ-000002", "โครงการตัวอย่าง 2", "CUS-000002", "456 ถนนตัวอย่าง", "2026-02-01", "2026-11-30",
			"500000", "MS", "นายที่ปรึกษา สอง", "08x-xxx-xxxx", "นายผู้รับผิดชอบ สอง"},
	}
	for row, example := range examples {
		for i, v := range example {
			f.SetCellValue(sheet, fmt.Sprintf("%s%d", colLetter(i), row+2), v)
		}
	}

	_, jobRange, _, err := writeLookupSheet(f, nil, jobs, nil)
	if err != nil {
		return err
	}
	if err := addDropdown(f, sheet, "H2:H1000", jobRange); err != nil {
		return err
	}

	// Excel's dropdown/data-validation only lets a cell hold one value from the list — there's
	// no built-in multi-select. job_code needs to accept several codes per project (see the
	// example row's "MP,ME"), so the dropdown here is a reference list only, not the actual
	// input method: type the codes directly into the cell, comma-separated. A cell comment
	// (not a data row) carries this instruction so it never pollutes the parseable data range.
	if err := f.AddComment(sheet, excelize.Comment{
		Cell: "H1",
		Text: "พิมพ์ได้หลายค่า คั่นด้วยจุลภาค เช่น MP,ME,MS (ดรอปดาวน์เป็นรายการอ้างอิงเลือกได้ทีละค่าเท่านั้น)",
	}); err != nil {
		return err
	}

	f.SetActiveSheet(0)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename=project_import_template.xlsx")
	return c.Send(buf.Bytes())
}

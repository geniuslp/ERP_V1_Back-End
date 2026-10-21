package handlers

import (
	"context"
	"fmt"
	"log"
	"regexp"
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
	p.budget_amount, p.vo_amount::float8, p.cost_amount::float8, p.project_kind,
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
		&p.BudgetAmount, &p.VOAmount, &p.CostAmount, &p.ProjectKind, &p.ResponsibleUsernames, &p.SpentAmount, &p.PaidAmount, &p.RemainingAmount, &p.ConsultantName, &p.ConsultantPhone,
		&p.StartDate, &p.EndDate, &p.Status, &p.IsActive,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy)
}

var projectCodeRe = regexp.MustCompile(`^(\d{4})-(.*)-(\d+)$`)

// recodeProjectCode swaps the customer segment of {year}-{customer_code}-{NNN}, keeping year and
// number. The middle group is greedy (customer_code may contain '-'). ok=false when the current
// code is not in that shape or the new customer has no code.
func recodeProjectCode(current, newCustomerCode string) (string, bool) {
	newCustomerCode = strings.TrimSpace(newCustomerCode)
	m := projectCodeRe.FindStringSubmatch(current)
	if m == nil || newCustomerCode == "" {
		return "", false
	}
	return m[1] + "-" + newCustomerCode + "-" + m[3], true
}

func sameInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// validateJobCodes is defined in job_type.go, shared with the PR/PO job_code fields.

// validateProjectCore is the single source of truth for project field validation — used by
// Create, Update, and Import so the three paths can't drift apart. Returns the normalized
// status (defaulted to ACTIVE when blank) for the caller to use.
func validateProjectCore(projectCode, projectName, responsiblePersonName, status string, jobCodes []string) (string, error) {
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
	if err := validateJobCodes(jobCodes); err != nil {
		return "", err
	}
	return status, nil
}

// amountOrZero maps an omitted / null / "" amount to 0. No range validation (all three amounts are optional).
func amountOrZero(v *models.AmountInput) float64 {
	if v == nil {
		return 0
	}
	return float64(*v)
}

// resolveProjectValues applies the project_kind rule: NEW stores budget_amount (vo_amount saved as 0),
// VO stores vo_amount (budget_amount saved as 0). Both are optional: missing/null/"" -> 0. Blank kind
// falls back to defaultKind. Returns the normalized kind and the two amounts to store.
func resolveProjectValues(kind, defaultKind string, budget, vo *models.AmountInput) (string, float64, float64, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind == "" {
		kind = defaultKind
	}
	switch kind {
	case "NEW":
		return kind, amountOrZero(budget), 0, nil
	case "VO":
		return kind, 0, amountOrZero(vo), nil
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
// @Description  Every item also carries in_use (bool) and in_use_by (Thai doc names, e.g. "ใบขอซื้อ (PR)", "ใบสั่งซื้อ (PO)", "หนังสือสั่งจ้าง (WO)", "Memo"; [] when unused) — true once the project_code is referenced by any work document (see projectUsageSources in project_usage.go). Computed for the whole page with one query.
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
	rows.Close()
	ptrs := make([]*models.ProjectFull, len(items))
	for k := range items {
		ptrs[k] = &items[k]
	}
	h.fillProjectUsage(ctx, ptrs) // one query for the whole page

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
// @Description  Detail also returns job_shares: [{job_code, share_percent, share_amount}] (share_amount = budget_amount * share_percent / 100, derived, read-only); [] for legacy projects with no rows. Also returns in_use / in_use_by (see List).
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
	h.fillProjectUsage(context.Background(), []*models.ProjectFull{&p})
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

// nextProjectCode takes the next running number of the Asia/Bangkok year from
// project_code_year_counter (inside the caller's transaction, so a rollback gives the number
// back) and returns {year}-{customer_code}-{NNN}. Shared by Create and the Excel import so the
// two cannot drift. customerCode must be the customer's STORED customer_code.
func nextProjectCode(ctx context.Context, tx pgx.Tx, year int, customerCode string) (string, error) {
	var lastNo int
	if err := tx.QueryRow(ctx, `
		INSERT INTO project_code_year_counter (code_year, last_no) VALUES ($1, 1)
		ON CONFLICT (code_year) DO UPDATE SET last_no = project_code_year_counter.last_no + 1
		RETURNING last_no`, year).Scan(&lastNo); err != nil {
		return "", err
	}
	return formatProjectCode(year, customerCode, lastNo), nil
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
// @Description  project_code is generated by the server as YYYY-{customer_code}-NNN (client value ignored; customer_id required) and cannot be changed afterwards. location_code is now a free-text project address (no longer validated/joined against the location master). dept_code ("แผนก") is validated against the departments master (400 on an unknown code). job_codes is a subset of MP/ME/MS/MF/MG/MH/G (validated server-side, 400 on any other value). project_owner_name is free text ("เจ้าของโครงการ") — legacy, kept for backward compat. customer_id (FK -> customer.cus_id) is the new dropdown-selected "เจ้าของโครงการ" going forward; both fields are accepted independently. responsible_person_name ("ผู้รับผิดชอบหลัก") is required free text, replacing the old owner_id dropdown — owner_id is still accepted for backward compatibility but no longer required or used to drive anything. consultant_phone ("เบอร์ติดต่อของที่ปรึกษา") is freeform, no validation. job_shares: a needs-share job may be sent without a percent (missing / null / "" / 0) — it stays in job_codes and no share row is written; the total is not capped by the app, but a total above 100 (or one share above 100) is rejected with a plain Thai 400 from the DB guards (trg_project_job_share_total / share_percent CHECK).
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
	status, verr := validateProjectCore("(auto)", req.ProjectName, req.ResponsiblePersonName, req.Status, req.JobCodes)
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
		var cerr error
		if projectCode, cerr = nextProjectCode(ctx, tx, year, customerCode); cerr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}

		sp, err := tx.Begin(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
		err = sp.QueryRow(ctx, `
			INSERT INTO project
			    (project_code, project_name, location_code, dept_code, owner_id, owner_name, customer_id, responsible_person_name, job_codes,
			     budget_amount, consultant_name, consultant_phone, start_date, end_date,
			     status, is_active, created_at, updated_at, created_by, updated_by, vo_amount, project_kind, cost_amount)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,true,now(),now(),$16,$16,$17,$18,$19)
			RETURNING id`,
			projectCode, req.ProjectName, req.LocationCode, req.DeptCode, req.OwnerID, req.ProjectOwnerName, req.CustomerID, req.ResponsiblePersonName, req.JobCodes,
			budgetAmount, req.ConsultantName, req.ConsultantPhone,
			req.StartDate, req.EndDate, req.Status, claims.UserID, voAmount, kind, amountOrZero(req.CostAmount),
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
			log.Printf("project create: insert: %v", err)
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
		log.Printf("project create: insert: %v", err)
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
		if mapped := jobShareDBError(err, jobShares); mapped != nil {
			return mapped
		}
		log.Printf("project create: replaceJobShares: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if err := tx.Commit(ctx); err != nil {
		if mapped := jobShareDBError(err, jobShares); mapped != nil {
			return mapped
		}
		log.Printf("project create: commit: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}

	var p models.ProjectFull
	if err := scanProjectFull(&p, h.db.QueryRow(ctx,
		`SELECT `+projectSelectCols+projectSelectFrom+` WHERE p.id = $1`, id)); err != nil {
		return err
	}
	p.ResponsibleUsers = h.loadResponsibleUsers(ctx, int64(p.Id))
	h.fillProjectUsage(ctx, []*models.ProjectFull{&p})
	shares, err := h.loadJobShares(ctx, int64(p.Id))
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": ProjectDetail{ProjectFull: p, JobShares: shares}})
}

// Update godoc
// @Summary      Update project
// @Description  location_code is now a free-text project address (no longer validated/joined against the location master). dept_code ("แผนก") is validated against the departments master (400 on an unknown code). job_codes is a subset of MP/ME/MS/MF/MG/MH/G (validated server-side, 400 on any other value). project_owner_name is free text ("เจ้าของโครงการ") — legacy, kept for backward compat. customer_id (FK -> customer.cus_id) is the new dropdown-selected "เจ้าของโครงการ" going forward; both fields are accepted independently. responsible_person_name ("ผู้รับผิดชอบหลัก") is required free text, replacing the old owner_id dropdown — owner_id is still accepted for backward compatibility but no longer required or used to drive anything. consultant_phone ("เบอร์ติดต่อของที่ปรึกษา") is freeform, no validation. job_shares: a needs-share job may be sent without a percent (missing / null / "" / 0) — it stays in job_codes and no share row is written; the total is not capped by the app, but a total above 100 (or one share above 100) is rejected with a plain Thai 400 from the DB guards (trg_project_job_share_total / share_percent CHECK). OWNER CHANGE: customer_id, owner_id and project_owner_name are the owner fields; an omitted owner field keeps its current value, and a sent value that differs from the stored one is saved only while the project_code is NOT used by any work document — otherwise 409 "ไม่สามารถเปลี่ยนเจ้าของโครงการได้ เนื่องจากรหัสโครงการนี้ถูกใช้งานแล้วใน: <list>" (the check runs inside the UPDATE transaction). Other fields are never blocked by usage. See in_use / in_use_by on GET. PROJECT CODE FOLLOWS THE CUSTOMER: when customer_id changes on an unused project, project_code is recomputed as {year}-{new customer_code}-{same NNN} (year and number kept, project_code_year_counter untouched); 409 "รหัสโครงการ <code> ซ้ำกับโครงการอื่น" if that code exists. A legacy code that does not match YYYY-*-N, or a customer without customer_code, keeps the code and still saves the owner. A project_code in the body is always ignored; the response returns the saved project_code.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                      true  "Project ID"
// @Param        body  body  models.UpdateProjectReq  true  "Update data"
// @Success      200   {object}  ProjectDetail
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
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
	status, verr := validateProjectCore(req.ProjectCode, req.ProjectName, req.ResponsiblePersonName, req.Status, req.JobCodes)
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

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer tx.Rollback(ctx)
	// Serialise saves per project: concurrent updates (and their share replacement) queue here.
	var curOwnerID, curCustomerID *int64
	var curOwnerName *string
	if err := tx.QueryRow(ctx, `SELECT owner_id, owner_name, customer_id FROM project WHERE id=$1 FOR UPDATE`, id).
		Scan(&curOwnerID, &curOwnerName, &curCustomerID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}
	// Owner fields (customer_id, owner_id, project_owner_name): an omitted field means "keep
	// current" (the frontend omits customer_id while locked and never sends owner_id — writing
	// the omitted nil would wipe them). A sent value that differs from the stored one is an owner
	// change, allowed only while the project_code is not used by any work (checked here, in the
	// same transaction as the UPDATE, after the project row is locked).
	ownerChanged := false
	if req.CustomerID == nil {
		req.CustomerID = curCustomerID
	} else if !sameInt64Ptr(req.CustomerID, curCustomerID) {
		ownerChanged = true
	}
	if req.OwnerID == nil {
		req.OwnerID = curOwnerID
	} else if !sameInt64Ptr(req.OwnerID, curOwnerID) {
		ownerChanged = true
	}
	if req.ProjectOwnerName == nil {
		req.ProjectOwnerName = curOwnerName
	} else if strings.TrimSpace(*req.ProjectOwnerName) != strings.TrimSpace(derefStr(curOwnerName)) {
		ownerChanged = true
	}
	if ownerChanged {
		usage, uerr := projectUsage(ctx, tx, []string{currentProjectCode})
		if uerr != nil {
			log.Printf("project update: projectUsage: %v", uerr)
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบการใช้งานรหัสโครงการได้ กรุณาลองใหม่อีกครั้ง")
		}
		if by := usage[currentProjectCode]; len(by) > 0 {
			return fiber.NewError(fiber.StatusConflict,
				"ไม่สามารถเปลี่ยนเจ้าของโครงการได้ เนื่องจากรหัสโครงการนี้ถูกใช้งานแล้วใน: "+strings.Join(by, ", "))
		}
		// Not in use: when the CUSTOMER changed, project_code follows it — only the customer segment is
		// replaced; year and running number stay (no new number, project_code_year_counter untouched).
		if req.CustomerID != nil && !sameInt64Ptr(req.CustomerID, curCustomerID) {
			var newCustomerCode string
			if err := tx.QueryRow(ctx, `SELECT COALESCE(btrim(customer_code), '') FROM customer WHERE cus_id = $1`, *req.CustomerID).Scan(&newCustomerCode); err != nil {
				if err == pgx.ErrNoRows {
					return fiber.NewError(fiber.StatusBadRequest, "ไม่พบข้อมูลลูกค้าที่เลือก")
				}
				log.Printf("project update: customer_code lookup: %v", err)
				return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลโครงการได้ กรุณาลองใหม่อีกครั้ง")
			}
			if newCode, ok := recodeProjectCode(currentProjectCode, newCustomerCode); !ok {
				// legacy code shape or customer without a code: owner is saved, code stays.
				log.Printf("project update: id=%d code %q kept on owner change (customer_code=%q)", id, currentProjectCode, newCustomerCode)
			} else if newCode != currentProjectCode {
				var taken bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code = $1 AND id <> $2)`, newCode, id).Scan(&taken); err != nil {
					log.Printf("project update: code exists check: %v", err)
					return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลโครงการได้ กรุณาลองใหม่อีกครั้ง")
				}
				if taken {
					return fiber.NewError(fiber.StatusConflict, "รหัสโครงการ "+newCode+" ซ้ำกับโครงการอื่น")
				}
				req.ProjectCode = newCode
			}
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE project
		SET project_code=$1, project_name=$2, location_code=$3, dept_code=$4, owner_id=$5,
		    owner_name=$6, customer_id=$7, responsible_person_name=$8, job_codes=$9,
		    budget_amount=$10, consultant_name=$11, consultant_phone=$12,
		    start_date=$13, end_date=$14, status=$15, is_active=$16, updated_at=now(), updated_by=$17, vo_amount=$19, project_kind=$20, cost_amount=$21
		WHERE id=$18`,
		req.ProjectCode, req.ProjectName, req.LocationCode, req.DeptCode, req.OwnerID,
		req.ProjectOwnerName, req.CustomerID, req.ResponsiblePersonName, req.JobCodes,
		budgetAmount, req.ConsultantName, req.ConsultantPhone,
		req.StartDate, req.EndDate, req.Status, *isActive, claims.UserID, id, voAmount, kind, amountOrZero(req.CostAmount))
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == "23505" {
				return fiber.NewError(fiber.StatusConflict, "รหัสโครงการ "+req.ProjectCode+" ซ้ำกับโครงการอื่น")
			}
			if pgErr.Code == "23503" {
				if pgErr.ConstraintName == "project_customer_id_fkey" {
					return fiber.NewError(fiber.StatusBadRequest, "invalid customer_id")
				}
				if strings.Contains(pgErr.Message, "update or delete on table") {
					// Backstop: a table with an FK on project(project_code) (NO ACTION) holds a row the
					// usage check did not see (e.g. inserted concurrently) — nothing is changed.
					return fiber.NewError(fiber.StatusConflict, "ไม่สามารถเปลี่ยนรหัสโครงการได้ เนื่องจากมีเอกสารอ้างอิงรหัสโครงการนี้อยู่")
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
		if mapped := jobShareDBError(err, jobShares); mapped != nil {
			return mapped
		}
		log.Printf("project update: replaceJobShares: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกสัดส่วนงานได้ กรุณาลองใหม่อีกครั้ง")
	}
	if err := tx.Commit(ctx); err != nil {
		if mapped := jobShareDBError(err, jobShares); mapped != nil {
			return mapped
		}
		log.Printf("project update: commit: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}

	var p models.ProjectFull
	if err := scanProjectFull(&p, h.db.QueryRow(ctx,
		`SELECT `+projectSelectCols+projectSelectFrom+` WHERE p.id = $1`, id)); err != nil {
		return err
	}
	p.ResponsibleUsers = h.loadResponsibleUsers(ctx, int64(p.Id))
	h.fillProjectUsage(ctx, []*models.ProjectFull{&p})
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

// ImportTemplate godoc
// @Summary      Download the Excel template for project import
// @Description  Generated on the fly. Sheet "Project", row 1 = header: project_name, customer_code, "Project location_code", start_date, end_date, budget_amount, "job Code" (project_code, consultant_name, consultant_phone and responsible_person_name were removed — the code is generated on import). Two sample rows use real customer_code values from the customer table (first two active customers), job codes "MP,ME" and "MS". The "job Code" column is free text: no dropdown/data validation and no Lookup sheet (the template has the single sheet "Project"); a header comment says multiple codes are typed comma-separated.
// @Tags         Master
// @Security     BearerAuth
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200  {file}  file
// @Failure      500  {object}  fiber.Map
// @Router       /master/projects/import/template [get]
func (h *ProjectHandler) ImportTemplate(c *fiber.Ctx) error {
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
		"project_name", "customer_code", "Project location_code", "start_date", "end_date",
		"budget_amount", "job Code",
	}
	required := []bool{true, true, false, false, false, false, false}
	colWidths := []float64{30, 16, 24, 14, 14, 16, 24}
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

	// Sample rows use REAL customer codes (first two active customers) so the example imports as is.
	sampleCodes := []string{"", ""}
	if crows, qerr := h.db.Query(context.Background(), `SELECT btrim(customer_code) FROM customer WHERE is_active AND btrim(customer_code) <> '' ORDER BY cus_id LIMIT 2`); qerr == nil {
		for i := 0; crows.Next() && i < 2; i++ {
			_ = crows.Scan(&sampleCodes[i])
		}
		crows.Close()
	}
	if sampleCodes[1] == "" {
		sampleCodes[1] = sampleCodes[0]
	}
	examples := [][]string{
		{"โครงการตัวอย่าง", sampleCodes[0], "123 ถนนตัวอย่าง", "2026-01-01", "2026-12-31", "1000000", "MP,ME"},
		{"โครงการตัวอย่าง 2", sampleCodes[1], "456 ถนนตัวอย่าง", "2026-02-01", "2026-11-30", "500000", "MS"},
	}
	for row, example := range examples {
		for i, v := range example {
			f.SetCellValue(sheet, fmt.Sprintf("%s%d", colLetter(i), row+2), v)
		}
	}

	// start_date / end_date columns are formatted as Text ("@") so Excel keeps exactly what the user
	// types ("2026-01-01") instead of converting it; real date cells are still accepted on import.
	textStyle, err := f.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return err
	}
	if err := f.SetColStyle(sheet, "D:E", textStyle); err != nil {
		return err
	}
	for _, cell := range []string{"D1", "E1"} {
		if err := f.AddComment(sheet, excelize.Comment{Cell: cell, Text: "รูปแบบ 2026-01-01 หรือ 2026/01/01"}); err != nil {
			return err
		}
	}

	// "job Code" is free text (no dropdown, no validation, no Lookup sheet): several values are typed
	// comma-separated; a cell comment on the header (not a data row) carries the instruction.
	if err := f.AddComment(sheet, excelize.Comment{
		Cell: "G1",
		Text: "พิมพ์ได้หลายค่า คั่นด้วยจุลภาค เช่น MP,ME,MS",
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

package handlers

import (
	"context"
	"fmt"
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
	p.budget_amount,
	COALESCE((SELECT SUM(po.net_amount) FROM purchase_order po
		WHERE po.project_code = p.project_code
		  AND po.status = 'APPROVED'), 0)
	+ COALESCE((SELECT SUM(wo.net_amount) FROM work_order wo
		WHERE wo.project_code = p.project_code
		  AND wo.status = 'APPROVED'), 0) AS spent_amount,
	COALESCE((SELECT SUM(pl.amount_paid) FROM payment_log pl
		JOIN purchase_order po ON po.id = pl.doc_id
		WHERE pl.doc_type = 'PO' AND po.project_code = p.project_code), 0)
	+ COALESCE((SELECT SUM(pl.amount_paid) FROM payment_log pl
		JOIN work_order wo ON wo.id = pl.doc_id
		WHERE pl.doc_type = 'WO' AND wo.project_code = p.project_code), 0) AS paid_amount,
	p.budget_amount - (
		COALESCE((SELECT SUM(po.net_amount) FROM purchase_order po
			WHERE po.project_code = p.project_code
			  AND po.status = 'APPROVED'), 0)
		+ COALESCE((SELECT SUM(wo.net_amount) FROM work_order wo
			WHERE wo.project_code = p.project_code
			  AND wo.status = 'APPROVED'), 0)
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
		&p.BudgetAmount, &p.SpentAmount, &p.PaidAmount, &p.RemainingAmount, &p.ConsultantName, &p.ConsultantPhone,
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
// @Success      200  {object}  models.ProjectFull
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
	return c.JSON(fiber.Map{"success": true, "data": p})
}

// Create godoc
// @Summary      Create project
// @Description  location_code is now a free-text project address (no longer validated/joined against the location master). dept_code ("แผนก") is validated against the departments master (400 on an unknown code). job_codes is a subset of MP/ME/MS/MF/MG/MH/G (validated server-side, 400 on any other value). project_owner_name is free text ("เจ้าของโครงการ") — legacy, kept for backward compat. customer_id (FK -> customer.cus_id) is the new dropdown-selected "เจ้าของโครงการ" going forward; both fields are accepted independently. responsible_person_name ("ผู้รับผิดชอบหลัก") is required free text, replacing the old owner_id dropdown — owner_id is still accepted for backward compatibility but no longer required or used to drive anything. consultant_phone ("เบอร์ติดต่อของที่ปรึกษา") is freeform, no validation.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateProjectReq  true  "Project data"
// @Success      201   {object}  models.ProjectFull
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /master/projects [post]
func (h *ProjectHandler) Create(c *fiber.Ctx) error {
	var req models.CreateProjectReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	status, verr := validateProjectCore(req.ProjectCode, req.ProjectName, req.ResponsiblePersonName, req.Status, req.BudgetAmount, req.JobCodes)
	if verr != nil {
		return fiber.NewError(fiber.StatusBadRequest, verr.Error())
	}
	req.Status = status

	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	var id int64
	err := h.db.QueryRow(ctx, `
		INSERT INTO project
		    (project_code, project_name, location_code, dept_code, owner_id, owner_name, customer_id, responsible_person_name, job_codes,
		     budget_amount, consultant_name, consultant_phone, start_date, end_date,
		     status, is_active, created_at, updated_at, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,true,now(),now(),$16,$16)
		RETURNING id`,
		req.ProjectCode, req.ProjectName, req.LocationCode, req.DeptCode, req.OwnerID, req.ProjectOwnerName, req.CustomerID, req.ResponsiblePersonName, req.JobCodes,
		req.BudgetAmount, req.ConsultantName, req.ConsultantPhone,
		req.StartDate, req.EndDate, req.Status, claims.UserID,
	).Scan(&id)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == "23505" {
				return fiber.NewError(fiber.StatusConflict, "project_code already exists")
			}
			if pgErr.Code == "23503" {
				if pgErr.ConstraintName == "project_customer_id_fkey" {
					return fiber.NewError(fiber.StatusBadRequest, "invalid customer_id")
				}
				return fiber.NewError(fiber.StatusBadRequest, "invalid dept_code")
			}
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	var p models.ProjectFull
	if err := scanProjectFull(&p, h.db.QueryRow(ctx,
		`SELECT `+projectSelectCols+projectSelectFrom+` WHERE p.id = $1`, id)); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": p})
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
// @Success      200   {object}  models.ProjectFull
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
	status, verr := validateProjectCore(req.ProjectCode, req.ProjectName, req.ResponsiblePersonName, req.Status, req.BudgetAmount, req.JobCodes)
	if verr != nil {
		return fiber.NewError(fiber.StatusBadRequest, verr.Error())
	}
	req.Status = status

	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	// is_active: nil means the client omitted it — preserve the project's current value
	// instead of defaulting to false. Unlike Status (which has a real default, "ACTIVE"),
	// IsActive has no sensible default; the only correct behavior for "not sent" is "unchanged".
	isActive := req.IsActive
	if isActive == nil {
		var current bool
		if err := h.db.QueryRow(ctx, `SELECT is_active FROM project WHERE id=$1`, id).Scan(&current); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "project not found")
		}
		isActive = &current
	}

	tag, err := h.db.Exec(ctx, `
		UPDATE project
		SET project_code=$1, project_name=$2, location_code=$3, dept_code=$4, owner_id=$5,
		    owner_name=$6, customer_id=$7, responsible_person_name=$8, job_codes=$9,
		    budget_amount=$10, consultant_name=$11, consultant_phone=$12,
		    start_date=$13, end_date=$14, status=$15, is_active=$16, updated_at=now(), updated_by=$17
		WHERE id=$18`,
		req.ProjectCode, req.ProjectName, req.LocationCode, req.DeptCode, req.OwnerID,
		req.ProjectOwnerName, req.CustomerID, req.ResponsiblePersonName, req.JobCodes,
		req.BudgetAmount, req.ConsultantName, req.ConsultantPhone,
		req.StartDate, req.EndDate, req.Status, *isActive, claims.UserID, id)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == "23505" {
				return fiber.NewError(fiber.StatusConflict, "project_code already exists")
			}
			if pgErr.Code == "23503" {
				if pgErr.ConstraintName == "project_customer_id_fkey" {
					return fiber.NewError(fiber.StatusBadRequest, "invalid customer_id")
				}
				return fiber.NewError(fiber.StatusBadRequest, "invalid dept_code")
			}
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}

	var p models.ProjectFull
	if err := scanProjectFull(&p, h.db.QueryRow(ctx,
		`SELECT `+projectSelectCols+projectSelectFrom+` WHERE p.id = $1`, id)); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": p})
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
// @Description  Column layout (row 1 = header, data starts row 2): project_code*, project_name*, customer_code, location_code, start_date, end_date, budget_amount, dept_code, job_code, consultant_name, consultant_phone, responsible_person_name, credit. Applies the same validation as POST /master/projects (responsible_person_name required, status defaults to ACTIVE, budget_amount >= 0). customer_code (if given) is resolved against the customer table to set customer_id — an unresolvable code fails that row. dept_code is pre-validated against the live departments table (is_active=true). job_code accepts one or more comma-separated codes from the fixed JobTypes enum (same list validated on POST /master/projects' job_codes[], NOT the unrelated cost_job master table) and is written to project.job_codes[]. credit is pre-validated against the fixed credit_term option list (same as the Supplier page's dropdown, not DB-backed). All three give a clear per-row reason on failure instead of a generic "invalid". Processes in partial-success mode: valid rows are inserted, invalid rows are skipped and reported. Re-upload only the failed rows after fixing them.
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

	depts, err := fetchActiveDepartments(ctx, h.db)
	if err != nil {
		return err
	}
	deptSet := toSet(depts)
	jobSet := toSet(projectJobTypeOptions())
	creditTermSet := toStringSet(creditTermOptions)

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

	for i, row := range sheetRows[1:] {
		rowNo := i + 2 // account for header row

		projectCode := strings.TrimSpace(cellAt(row, 0))

		// An empty project_code marks the end of the data grid — the template's notes/legend
		// text (and any trailing blank rows) live below this point and must never be read as
		// data, so stop entirely rather than reporting them as failed rows.
		if projectCode == "" {
			break
		}

		projectName := strings.TrimSpace(cellAt(row, 1))
		customerCode := strings.TrimSpace(cellAt(row, 2))
		locationCode := nullableCell(cellAt(row, 3))
		startDate := nullableCell(cellAt(row, 4))
		endDate := nullableCell(cellAt(row, 5))
		budgetStr := strings.TrimSpace(cellAt(row, 6))
		deptCodeRaw := strings.TrimSpace(cellAt(row, 7))
		jobCodeRaw := strings.TrimSpace(cellAt(row, 8))
		consultantName := nullableCell(cellAt(row, 9))
		consultantPhone := nullableCell(cellAt(row, 10))
		responsiblePersonName := strings.TrimSpace(cellAt(row, 11))
		creditRaw := strings.TrimSpace(cellAt(row, 12))

		budgetAmount := 0.0
		if budgetStr != "" {
			var perr error
			budgetAmount, perr = strconv.ParseFloat(budgetStr, 64)
			if perr != nil {
				errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("invalid budget_amount: %q", budgetStr)})
				continue
			}
		}

		if deptCodeRaw != "" && !deptSet[deptCodeRaw] {
			errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("dept_code %q not found in departments", deptCodeRaw)})
			continue
		}
		deptCode := nullableCell(deptCodeRaw)

		var jobCodes []string
		if jobCodeRaw != "" {
			badCode := ""
			for _, jc := range strings.Split(jobCodeRaw, ",") {
				jc = strings.TrimSpace(jc)
				if jc == "" {
					continue
				}
				if !jobSet[jc] {
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

		if creditRaw != "" && !creditTermSet[creditRaw] {
			errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("credit %q is not a valid option", creditRaw)})
			continue
		}
		credit := nullableCell(creditRaw)

		status, verr := validateProjectCore(projectCode, projectName, responsiblePersonName, "", budgetAmount, jobCodes)
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
		if customerCode != "" {
			var cid int64
			if err := h.db.QueryRow(ctx, `SELECT cus_id FROM customer WHERE customer_code = $1`, customerCode).Scan(&cid); err != nil {
				errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("customer_code %q not found", customerCode)})
				continue
			}
			customerID = &cid
		}

		var id int64
		err := h.db.QueryRow(ctx, `
			INSERT INTO project
			    (project_code, project_name, location_code, dept_code, customer_id, responsible_person_name,
			     job_codes, budget_amount, consultant_name, consultant_phone, start_date, end_date, credit,
			     status, is_active, created_at, updated_at, created_by, updated_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,true,now(),now(),$15,$15)
			RETURNING id`,
			projectCode, projectName, locationCode, deptCode, customerID, responsiblePersonName,
			jobCodes, budgetAmount, consultantName, consultantPhone, startDate, endDate, credit,
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
					errs = append(errs, rowError{Row: rowNo, Reason: fmt.Sprintf("invalid dept_code %q", deptCodeRaw)})
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
			"imported": len(oks),
			"failed":   len(errs),
			"rows":     oks,
			"errors":   errs,
		},
	})
}

// ImportTemplate godoc
// @Summary      ดาวน์โหลด template Excel สำหรับ import โครงการ
// @Description  Generates the .xlsx on the fly: sheet "Project" with header row (project_code*, project_name*, customer_code, location_code, start_date, end_date, budget_amount, dept_code, job_code, consultant_name, consultant_phone, responsible_person_name, credit), one example row, and a notes row. A hidden "Lookup" sheet lists current active dept_code/dept_name (departments, DB-backed), job_code/job_name (the fixed JobTypes enum — NOT cost_job), and credit_term options (fixed hardcoded list, same as the Supplier page's dropdown — not DB-backed), bound as Excel dropdowns on the dept_code, job_code, and credit columns.
// @Tags         Master
// @Security     BearerAuth
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200  {file}  file
// @Failure      500  {object}  fiber.Map
// @Router       /master/projects/import/template [get]
func (h *ProjectHandler) ImportTemplate(c *fiber.Ctx) error {
	ctx := context.Background()

	depts, err := fetchActiveDepartments(ctx, h.db)
	if err != nil {
		return err
	}
	jobs := projectJobTypeOptions()
	creditTerms := creditTermOptions

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
	headers := []string{
		"project_code", "project_name", "customer_code", "location_code", "start_date", "end_date",
		"budget_amount", "dept_code", "job_code", "consultant_name", "consultant_phone",
		"responsible_person_name", "credit",
	}
	required := []bool{true, true, false, false, false, false, false, false, false, false, false, false, false}
	colWidths := []float64{16, 30, 16, 24, 14, 14, 16, 12, 24, 24, 20, 24, 14}
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

	example := []string{
		"PRJ-000001", "โครงการตัวอย่าง", "CUS-000001", "123 ถนนตัวอย่าง", "2026-01-01", "2026-12-31",
		"1000000", "IT", "MP,ME", "นายที่ปรึกษา ตัวอย่าง", "08x-xxx-xxxx", "นายผู้รับผิดชอบ ตัวอย่าง", "30 วัน",
	}
	for i, v := range example {
		f.SetCellValue(sheet, colLetter(i)+"2", v)
	}

	deptRange, jobRange, creditRange, err := writeLookupSheet(f, depts, jobs, creditTerms)
	if err != nil {
		return err
	}
	if err := addDropdown(f, sheet, "H2:H1000", deptRange); err != nil {
		return err
	}
	if err := addDropdown(f, sheet, "I2:I1000", jobRange); err != nil {
		return err
	}
	if err := addDropdown(f, sheet, "M2:M1000", creditRange); err != nil {
		return err
	}

	// Excel's dropdown/data-validation only lets a cell hold one value from the list — there's
	// no built-in multi-select. job_code needs to accept several codes per project (see the
	// example row's "MP,ME"), so the dropdown here is a reference list only, not the actual
	// input method: type the codes directly into the cell, comma-separated. A cell comment
	// (not a data row) carries this instruction so it never pollutes the parseable data range.
	if err := f.AddComment(sheet, excelize.Comment{
		Cell: "I1",
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

package handlers

import (
	"context"
	"fmt"
	"strings"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
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
	p.owner_id, u.full_name AS owner_name, p.owner_name AS project_owner_name, p.responsible_person_name, p.job_codes,
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
	LEFT JOIN users       u ON u.id = p.owner_id
	LEFT JOIN departments d ON d.dept_code = p.dept_code`

func scanProjectFull(p *models.ProjectFull, row pgx.Row) error {
	return row.Scan(&p.Id, &p.ProjectCode, &p.ProjectName, &p.LocationCode,
		&p.DeptCode, &p.DeptName,
		&p.OwnerID, &p.OwnerName, &p.ProjectOwnerName, &p.ResponsiblePersonName, &p.JobCodes,
		&p.BudgetAmount, &p.SpentAmount, &p.PaidAmount, &p.RemainingAmount, &p.ConsultantName, &p.ConsultantPhone,
		&p.StartDate, &p.EndDate, &p.Status, &p.IsActive,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy)
}

// validateJobCodes is defined in job_type.go, shared with the PR/PO job_code fields.

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
// @Description  location_code is now a free-text project address (no longer validated/joined against the location master). dept_code ("แผนก") is validated against the departments master (400 on an unknown code). job_codes is a subset of MP/ME/MS/MF/MG/MH/G (validated server-side, 400 on any other value). project_owner_name is free text ("เจ้าของโครงการ"). responsible_person_name ("ผู้รับผิดชอบหลัก") is required free text, replacing the old owner_id dropdown — owner_id is still accepted for backward compatibility but no longer required or used to drive anything. consultant_phone ("เบอร์ติดต่อของที่ปรึกษา") is freeform, no validation.
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
	if req.ProjectCode == "" || req.ProjectName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "project_code and project_name are required")
	}
	if strings.TrimSpace(req.ResponsiblePersonName) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "responsible_person_name is required")
	}
	if req.Status == "" {
		req.Status = "ACTIVE"
	}
	if !validProjectStatus[req.Status] {
		return fiber.NewError(fiber.StatusBadRequest, "status must be one of ACTIVE, INACTIVE, CLOSED")
	}
	if req.BudgetAmount < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "budget_amount must be >= 0")
	}
	if err := validateJobCodes(req.JobCodes); err != nil {
		return err
	}

	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	var id int64
	err := h.db.QueryRow(ctx, `
		INSERT INTO project
		    (project_code, project_name, location_code, dept_code, owner_id, owner_name, responsible_person_name, job_codes,
		     budget_amount, consultant_name, consultant_phone, start_date, end_date,
		     status, is_active, created_at, updated_at, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,true,now(),now(),$15,$15)
		RETURNING id`,
		req.ProjectCode, req.ProjectName, req.LocationCode, req.DeptCode, req.OwnerID, req.ProjectOwnerName, req.ResponsiblePersonName, req.JobCodes,
		req.BudgetAmount, req.ConsultantName, req.ConsultantPhone,
		req.StartDate, req.EndDate, req.Status, claims.UserID,
	).Scan(&id)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == "23505" {
				return fiber.NewError(fiber.StatusConflict, "project_code already exists")
			}
			if pgErr.Code == "23503" {
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
// @Description  location_code is now a free-text project address (no longer validated/joined against the location master). dept_code ("แผนก") is validated against the departments master (400 on an unknown code). job_codes is a subset of MP/ME/MS/MF/MG/MH/G (validated server-side, 400 on any other value). project_owner_name is free text ("เจ้าของโครงการ"). responsible_person_name ("ผู้รับผิดชอบหลัก") is required free text, replacing the old owner_id dropdown — owner_id is still accepted for backward compatibility but no longer required or used to drive anything. consultant_phone ("เบอร์ติดต่อของที่ปรึกษา") is freeform, no validation.
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
	if req.ProjectCode == "" || req.ProjectName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "project_code and project_name are required")
	}
	if strings.TrimSpace(req.ResponsiblePersonName) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "responsible_person_name is required")
	}
	if req.Status == "" {
		req.Status = "ACTIVE"
	}
	if !validProjectStatus[req.Status] {
		return fiber.NewError(fiber.StatusBadRequest, "status must be one of ACTIVE, INACTIVE, CLOSED")
	}
	if req.BudgetAmount < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "budget_amount must be >= 0")
	}
	if err := validateJobCodes(req.JobCodes); err != nil {
		return err
	}

	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	tag, err := h.db.Exec(ctx, `
		UPDATE project
		SET project_code=$1, project_name=$2, location_code=$3, dept_code=$4, owner_id=$5,
		    owner_name=$6, responsible_person_name=$7, job_codes=$8,
		    budget_amount=$9, consultant_name=$10, consultant_phone=$11,
		    start_date=$12, end_date=$13, status=$14, is_active=$15, updated_at=now(), updated_by=$16
		WHERE id=$17`,
		req.ProjectCode, req.ProjectName, req.LocationCode, req.DeptCode, req.OwnerID,
		req.ProjectOwnerName, req.ResponsiblePersonName, req.JobCodes,
		req.BudgetAmount, req.ConsultantName, req.ConsultantPhone,
		req.StartDate, req.EndDate, req.Status, req.IsActive, claims.UserID, id)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == "23505" {
				return fiber.NewError(fiber.StatusConflict, "project_code already exists")
			}
			if pgErr.Code == "23503" {
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

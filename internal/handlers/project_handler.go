package handlers

import (
	"context"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectHandler struct {
	db *pgxpool.Pool
}

func NewProjectHandler(db *pgxpool.Pool) *ProjectHandler {
	return &ProjectHandler{db: db}
}

const projectCols = `id, project_code, project_name, location_code, start_date, end_date,
	status, is_active, created_at, updated_at, created_by, updated_by`

func scanProject(p *models.ProjectFull, row interface {
	Scan(dest ...any) error
}) error {
	return row.Scan(&p.Id, &p.ProjectCode, &p.ProjectName, &p.LocationCode,
		&p.StartDate, &p.EndDate, &p.Status, &p.IsActive,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy)
}

// ListProjects godoc
// @Summary      List all active projects
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        status  query  string  false  "Filter by status (ACTIVE|INACTIVE|CLOSED)"
// @Param        q       query  string  false  "Search project_code or project_name"
// @Success      200     {object}  fiber.Map
// @Failure      500     {object}  fiber.Map
// @Router       /master/projects [get]
func (h *ProjectHandler) ListProjects(c *fiber.Ctx) error {
	status := c.Query("status")
	q := c.Query("q")

	rows, err := h.db.Query(context.Background(), `
		SELECT `+projectCols+`
		FROM project
		WHERE is_active = true
		  AND ($1 = '' OR status = $1)
		  AND ($2 = '' OR project_code ILIKE '%'||$2||'%' OR project_name ILIKE '%'||$2||'%')
		ORDER BY project_code`, status, q)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.ProjectFull
	for rows.Next() {
		var p models.ProjectFull
		if err := scanProject(&p, rows); err != nil {
			return err
		}
		items = append(items, p)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// CreateProject godoc
// @Summary      Create project
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateProjectReq  true  "Project data"
// @Success      201   {object}  models.ProjectFull
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /master/projects [post]
func (h *ProjectHandler) CreateProject(c *fiber.Ctx) error {
	var req models.CreateProjectReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.ProjectCode == "" || req.ProjectName == "" || req.Status == "" {
		return fiber.NewError(fiber.StatusBadRequest, "project_code, project_name and status are required")
	}

	claims := middleware.GetClaims(c)

	var p models.ProjectFull
	err := scanProject(&p, h.db.QueryRow(context.Background(), `
		INSERT INTO project (project_code, project_name, location_code, start_date, end_date,
		    status, is_active, created_at, updated_at, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,true,now(),now(),$7,$7)
		RETURNING `+projectCols,
		req.ProjectCode, req.ProjectName, req.LocationCode,
		req.StartDate, req.EndDate, req.Status, claims.UserID))
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusConflict, "project_code already exists")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": p})
}

// UpdateProject godoc
// @Summary      Update project
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
func (h *ProjectHandler) UpdateProject(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.UpdateProjectReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.ProjectCode == "" || req.ProjectName == "" || req.Status == "" {
		return fiber.NewError(fiber.StatusBadRequest, "project_code, project_name and status are required")
	}

	claims := middleware.GetClaims(c)

	var p models.ProjectFull
	err = scanProject(&p, h.db.QueryRow(context.Background(), `
		UPDATE project
		SET project_code=$1, project_name=$2, location_code=$3, start_date=$4, end_date=$5,
		    status=$6, is_active=$7, updated_at=now(), updated_by=$8
		WHERE id=$9
		RETURNING `+projectCols,
		req.ProjectCode, req.ProjectName, req.LocationCode,
		req.StartDate, req.EndDate, req.Status, req.IsActive,
		claims.UserID, id))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": p})
}

// DeleteProject godoc
// @Summary      Soft-delete project
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Project ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/projects/{id} [delete]
func (h *ProjectHandler) DeleteProject(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	claims := middleware.GetClaims(c)

	tag, err := h.db.Exec(context.Background(),
		`UPDATE project SET is_active=false, updated_at=now(), updated_by=$1 WHERE id=$2`,
		claims.UserID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "project deleted"})
}

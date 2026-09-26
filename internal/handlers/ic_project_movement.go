package handlers

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

// IcProjectMovementHandler serves the IC Project Movement (Issue/Transfer) module.
type IcProjectMovementHandler struct {
	db *pgxpool.Pool
}

func NewIcProjectMovementHandler(db *pgxpool.Pool) *IcProjectMovementHandler {
	return &IcProjectMovementHandler{db: db}
}

// GetProjectByCode godoc
// @Summary      Get a single project's basic info by project_code (IC Project Movement context header)
// @Description  Returns id, project_code, project_name, customer_name for a project by project_code — the string identifier this movement feature is keyed on throughout (route param, job-codes lookup, movement create), unlike GET /ic/projects/{projectId} which takes the numeric id.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path  string  true  "project.project_code"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /ic/projects/by-code/{projectCode} [get]
func (h *IcProjectMovementHandler) GetProjectByCode(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "projectCode is required")
	}

	var p ICProject
	err := h.db.QueryRow(context.Background(), `
		SELECT p.id, p.project_code, p.project_name, c.customer_name
		FROM project p
		LEFT JOIN customer c ON p.customer_id::integer = c.cus_id
		WHERE p.project_code = $1`, projectCode,
	).Scan(&p.ID, &p.ProjectCode, &p.ProjectName, &p.CustomerName)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}

	return c.JSON(fiber.Map{"success": true, "data": p})
}

type IcProjectJobCode struct {
	JobCode string `json:"job_code"`
	JobName string `json:"job_name"`
}

// ListProjectJobCodes godoc
// @Summary      List job codes assigned to a project
// @Description  Reads project.job_codes (text[]) for this project_code and resolves each against the fixed JobTypes list (internal/handlers/job_type.go) — NOT the unrelated cost_job master table, which uses a different single-letter code space (P/E/S/F/G/H/B) scoped per cost subject and shares the "job_code" column name only by coincidence. Returns an empty array if the project has no job_codes assigned.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path  string  true  "project.project_code"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/job-codes [get]
func (h *IcProjectMovementHandler) ListProjectJobCodes(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "projectCode is required")
	}

	ctx := context.Background()

	jobCodes, err := fetchProjectJobCodes(ctx, h.db, projectCode)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	result := []IcProjectJobCode{}
	for _, jc := range jobCodes {
		for _, jt := range JobTypes {
			if jt.Code == jc {
				result = append(result, IcProjectJobCode{JobCode: jt.Code, JobName: jt.Label})
				break
			}
		}
	}

	return c.JSON(fiber.Map{"success": true, "data": result})
}

// fetchProjectJobCodes reads project.job_codes (text[]) for the given project_code. Returns
// an empty (non-nil) slice if the project doesn't exist or has NULL/empty job_codes.
func fetchProjectJobCodes(ctx context.Context, db *pgxpool.Pool, projectCode string) ([]string, error) {
	var jobCodes []string
	err := db.QueryRow(ctx, `SELECT job_codes FROM project WHERE project_code = $1`, projectCode).Scan(&jobCodes)
	if err == pgx.ErrNoRows {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if jobCodes == nil {
		return []string{}, nil
	}
	return jobCodes, nil
}

// CreateMovement godoc
// @Summary      Create an IC Project Movement (Issue or Transfer) header
// @Description  Validates doc_type and that job_code is assigned to this project (server-side re-check against project.job_codes), generates doc_no (PIS-YYYYMM-NNNN for ISSUE, PTR-YYYYMM-NNNN for TRANSFER, monthly-reset counter shared mechanism with ic_po_receive_document.receive_no), and inserts the row with status=DRAFT.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        projectCode  path  string                                  true  "project.project_code"
// @Param        body         body  models.CreateIcProjectMovementRequest  true  "Movement header"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/movements [post]
func (h *IcProjectMovementHandler) CreateMovement(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "projectCode is required")
	}

	var req models.CreateIcProjectMovementRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.ProjectCode = projectCode

	if req.DocType != "ISSUE" && req.DocType != "TRANSFER" {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type must be ISSUE or TRANSFER")
	}
	if req.JobCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "job_code is required")
	}
	if req.RequestedBy == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "requested_by is required")
	}

	docDate := req.DocDate
	if docDate == "" {
		docDate = time.Now().Format("2006-01-02")
	}

	ctx := context.Background()

	// Re-fetch project.job_codes server-side — never trust the frontend's list alone.
	assignedJobCodes, err := fetchProjectJobCodes(ctx, h.db, projectCode)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	assigned := false
	for _, jc := range assignedJobCodes {
		if jc == req.JobCode {
			assigned = true
			break
		}
	}
	if !assigned {
		return fiber.NewError(fiber.StatusBadRequest, "job_code not assigned to this project")
	}

	claims := middleware.GetClaims(c)

	prefix := "PIS"
	if req.DocType == "TRANSFER" {
		prefix = "PTR"
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	defer tx.Rollback(ctx)

	ym := time.Now().Format("200601")
	var seq int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO por_number_counter (year_month, last_seq) VALUES ($1, 1)
		ON CONFLICT (year_month) DO UPDATE SET last_seq = por_number_counter.last_seq + 1
		RETURNING last_seq`, ym,
	).Scan(&seq); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	docNo := fmt.Sprintf("%s-%s-%04d", prefix, ym, seq)

	var newID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO ic_project_movement
		    (doc_no, doc_type, doc_date, project_code, job_code, requested_by, remarks, status, created_by, created_at, updated_at)
		VALUES
		    ($1, $2, $3, $4, $5, $6, $7, 'DRAFT', $8, NOW(), NOW())
		RETURNING id`,
		docNo, req.DocType, docDate, req.ProjectCode, req.JobCode, req.RequestedBy, req.Remarks, claims.UserID,
	).Scan(&newID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": newID, "doc_no": docNo}})
}

// jobNameForCode resolves a display label for a project-movement job_code against the fixed
// JobTypes list (job_type.go) — NOT the unrelated cost_job table, which uses a different
// single-letter code space (see ListProjectJobCodes above for the same caveat).
func jobNameForCode(code string) string {
	for _, jt := range JobTypes {
		if jt.Code == code {
			return jt.Label
		}
	}
	return code
}

// GetMovement godoc
// @Summary      Get one IC Project Movement header, joined for display
// @Description  Returns doc_no, doc_type, doc_date, project_code/name, job_code/name, requested_by (id + full_name), status, remarks, created_at for one movement, scoped to the given project_code. job_name is resolved from the fixed JobTypes list (job_type.go), not the unrelated cost_job table.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path  string  true  "project.project_code"
// @Param        id           path  int     true  "ic_project_movement.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/movements/{id} [get]
func (h *IcProjectMovementHandler) GetMovement(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "projectCode is required")
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var d models.IcProjectMovementDetail
	err = h.db.QueryRow(context.Background(), `
		SELECT m.id, m.doc_no, m.doc_type, m.doc_date::text, m.project_code, p.project_name,
		       m.job_code, m.requested_by, u.full_name, m.status, m.remarks, m.created_at
		FROM ic_project_movement m
		JOIN project p ON p.project_code = m.project_code
		JOIN users u ON u.id = m.requested_by
		WHERE m.id = $1 AND m.project_code = $2`, id, projectCode,
	).Scan(&d.ID, &d.DocNo, &d.DocType, &d.DocDate, &d.ProjectCode, &d.ProjectName,
		&d.JobCode, &d.RequestedBy, &d.RequestedByName, &d.Status, &d.Remarks, &d.CreatedAt)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "movement not found")
	}
	d.JobName = jobNameForCode(d.JobCode)

	return c.JSON(fiber.Map{"success": true, "data": d})
}

// ListMovements godoc
// @Summary      List IC Project Movement documents for a project, paginated
// @Description  Newest first. job_name is resolved from the fixed JobTypes list (job_type.go), NOT joined against cost_job — cost_job.job_code is a different, single-letter code space (see ListAvailableMaterials/ListProjectCostCodes for the same caveat) and a direct join on it would silently exclude every row. requested_by is displayed via users.full_name, matching GetMovement and the existing pr.go pattern.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path   string  true  "project.project_code"
// @Param        page         query  int     false  "page number"  default(1)
// @Param        page_size    query  int     false  "page size"    default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/movements [get]
func (h *IcProjectMovementHandler) ListMovements(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "projectCode is required")
	}
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	ctx := context.Background()

	var total int64
	if err := h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM ic_project_movement WHERE project_code = $1`, projectCode,
	).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to count movements: "+err.Error())
	}

	rows, err := h.db.Query(ctx, `
		SELECT m.id, m.doc_no, m.doc_type, m.doc_date::text, m.status, m.job_code, u.full_name, m.created_at
		FROM ic_project_movement m
		JOIN users u ON u.id = m.requested_by
		WHERE m.project_code = $1
		ORDER BY m.created_at DESC
		LIMIT $2 OFFSET $3`, projectCode, size, offset)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list movements: "+err.Error())
	}
	defer rows.Close()

	items := []models.IcProjectMovementListItem{}
	for rows.Next() {
		var m models.IcProjectMovementListItem
		if err := rows.Scan(&m.ID, &m.DocNo, &m.DocType, &m.DocDate, &m.Status, &m.JobCode, &m.RequestedByName, &m.CreatedAt); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan movement: "+err.Error())
		}
		m.JobName = jobNameForCode(m.JobCode)
		items = append(items, m)
	}

	totalPages := int(total) / size
	if int(total)%size != 0 || total == 0 {
		totalPages++
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages,
		},
	})
}

// ─── Line items ─────────────────────────────────────────────────────────────

type linkedStockRow struct {
	ItemID    int64
	MatCode   string
	MatName   string
	Unit      *string
	Qty       float64
	UnitCost  float64
	UpdatedAt time.Time
	// Cost hierarchy resolved from material_code.cost_subgroup_id (same chain as GET /stock/items).
	CostSubgroupID int64
	CostCode       string
	CostName       string // consumed by the picker only; the project stock page deliberately omits cost_name
}

// queryLinkedWarehouseStock returns flat per-item stock straight from stock_item (one row per
// mat_code, global qty — stock_inventory is deliberately NOT read: it has no verified sync with
// stock_item's other writers). Shared by the TRANSFER picker and the project stock
// ("คงเหลือวัสดุ") page for warehouse-linked projects. matNameID/search are optional filters;
// onlyPositive drops items with qty = 0.
func queryLinkedWarehouseStock(ctx context.Context, db *pgxpool.Pool, matNameID int, search string, onlyPositive bool) ([]linkedStockRow, error) {
	args := []interface{}{}
	where := "WHERE 1=1"
	if onlyPositive {
		where += " AND si.qty > 0"
	}
	if matNameID > 0 {
		args = append(args, matNameID)
		where += fmt.Sprintf(" AND mc.mat_name_id = $%d", len(args))
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		where += fmt.Sprintf(" AND (si.mat_code ILIKE $%d OR mn.mat_name ILIKE $%d OR sp.spec_description ILIKE $%d OR si.item_name ILIKE $%d)", len(args), len(args), len(args), len(args))
	}
	rows, err := db.Query(ctx, `
		SELECT si.id, si.mat_code,
		       COALESCE(NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), ''), si.item_name, si.mat_code),
		       COALESCE(u.unit_name, si.unit),
		       si.qty, si.unit_cost, si.updated_at,
		       COALESCE(mc.cost_subgroup_id, 0),
		       COALESCE(csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code, ''),
		       COALESCE(csg.subgroup_name, '')
		FROM stock_item si
		LEFT JOIN material_code mc ON mc.mat_code = si.mat_code
		LEFT JOIN cost_subgroup csg ON csg.id = mc.cost_subgroup_id
		LEFT JOIN cost_group cg     ON cg.id = csg.group_id
		LEFT JOIN cost_job cj       ON cj.id = cg.job_id
		LEFT JOIN cost_subject csub ON csub.id = cj.subject_id
		LEFT JOIN mat_name mn  ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size sp ON sp.id = mc.spec_id
		LEFT JOIN unit u       ON u.id = mc.unit_id
		`+where+`
		ORDER BY si.mat_code`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []linkedStockRow{}
	for rows.Next() {
		var r linkedStockRow
		if err := rows.Scan(&r.ItemID, &r.MatCode, &r.MatName, &r.Unit, &r.Qty, &r.UnitCost, &r.UpdatedAt, &r.CostSubgroupID, &r.CostCode, &r.CostName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type icRowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// linkedWarehouseCode returns project.linked_warehouse_code ("" when NULL). A non-empty value
// means the project's stock lives in the general stock system (stock_item.qty), not in
// ic_project_receipt_pool / ic_project_cost_item.
func linkedWarehouseCode(ctx context.Context, q icRowQuerier, projectCode string) (string, error) {
	var code *string
	if err := q.QueryRow(ctx, `SELECT linked_warehouse_code FROM project WHERE project_code = $1`, projectCode).Scan(&code); err != nil {
		return "", err
	}
	if code == nil {
		return "", nil
	}
	return *code, nil
}

func nullableID(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

const errLinkedIssue = "this project has no receipt pool; use TRANSFER for warehouse-linked projects"

// ListAvailableMaterials godoc
// @Summary      List materials with on-hand cost-item balance for a project, filtered by job_code
// @Description  Populates the MatCode dropdown for adding a line item — joins ic_project_receipt_pool (the source of truth for movements) to material_code/mat_name/spec_size for display name (same pattern as PR line items) and to the 4-level cost code hierarchy for the job_code filter. Only rows with remaining (qty_received - qty_issued) > 0 are returned; the response's qty_on_hand field carries that computed remaining. job_code (e.g. "MP") is decomposed as cost_subject.subject_code=LEFT(job_code,1) + cost_job.job_code=SUBSTRING(job_code,2), the same convention pr_approval.go uses to resolve purchase_request.job_code — NOT a direct match against cost_job.job_code, which is a single-letter code in a different space (P/E/S/F/G/H/B) that never equals a 2-char JobTypes code on its own.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path   string  true  "movement's own project.project_code"
// @Param        id           path   int     true  "ic_project_movement.id (unused for filtering here, kept for route symmetry)"
// @Param        job_code     query  string  false "optional job_code filter (e.g. MP)"
// @Param        subgroup_id  query  int     false "cost_subgroup.id filter"
// @Param        mat_name_id  query  int     false "mat_name.id filter"
// @Param        q            query  string  false "search mat_code / mat_name / spec"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/movements/{id}/available-materials [get]
func (h *IcProjectMovementHandler) ListAvailableMaterials(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	movementID, err := c.ParamsInt("id")
	if err != nil || projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "projectCode and id are required")
	}

	var docType string
	if err := h.db.QueryRow(context.Background(),
		`SELECT doc_type FROM ic_project_movement WHERE id = $1 AND project_code = $2`,
		movementID, projectCode).Scan(&docType); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "movement not found")
	}

	linkedWH, err := linkedWarehouseCode(context.Background(), h.db, projectCode)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if linkedWH != "" {
		if docType == "ISSUE" {
			return fiber.NewError(fiber.StatusBadRequest, errLinkedIssue)
		}
		// Warehouse-linked TRANSFER source: flat stock_item.qty per mat_code (global, no
		// per-warehouse breakdown).
		items, err := queryLinkedWarehouseStock(context.Background(), h.db,
			c.QueryInt("mat_name_id", 0), strings.TrimSpace(c.Query("q")), true)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		result := []models.IcAvailableMaterial{}
		for _, it := range items {
			// cost_subgroup_id/cost_code/cost_name resolved from material_code -> cost_subgroup
			// (csg.subgroup_name, same source as the normal branch and ToCostCode's name).
			result = append(result, models.IcAvailableMaterial{MatCode: it.MatCode, MatName: it.MatName, Unit: it.Unit, QtyOnHand: it.Qty,
				CostSubgroupID: it.CostSubgroupID, CostCode: it.CostCode, CostName: it.CostName})
		}
		return c.JSON(fiber.Map{"success": true, "data": result})
	}

	// ISSUE draws from the receipt pool (received - issued); TRANSFER from cost-item on-hand.
	table, qtyExpr := "ic_project_receipt_pool", "(ici.qty_received - ici.qty_issued)"
	if docType == "TRANSFER" {
		table, qtyExpr = "ic_project_cost_item", "ici.qty_on_hand"
	}

	args := []interface{}{projectCode}
	where := " WHERE ici.project_code = $1 AND " + qtyExpr + " > 0"
	if jc := c.Query("job_code"); jc != "" {
		args = append(args, jc)
		where += fmt.Sprintf(" AND csub.subject_code = LEFT($%d, 1) AND cj.job_code = SUBSTRING($%d FROM 2)", len(args), len(args))
	}
	if v := c.QueryInt("subgroup_id", 0); v > 0 {
		args = append(args, v)
		where += fmt.Sprintf(" AND ici.cost_subgroup_id = $%d", len(args))
	}
	if v := c.QueryInt("mat_name_id", 0); v > 0 {
		args = append(args, v)
		where += fmt.Sprintf(" AND mc.mat_name_id = $%d", len(args))
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		args = append(args, "%"+q+"%")
		where += fmt.Sprintf(" AND (ici.mat_code ILIKE $%d OR mn.mat_name ILIKE $%d OR sp.spec_description ILIKE $%d)", len(args), len(args), len(args))
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT ici.mat_code,
		       COALESCE(NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), ''), ici.mat_code),
		       ici.cost_subgroup_id,
		       csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code,
		       csg.subgroup_name,
		       u.unit_name,
		       `+qtyExpr+`
		FROM `+table+` ici
		JOIN material_code mc      ON mc.mat_code = ici.mat_code
		LEFT JOIN mat_name mn      ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size sp     ON sp.id = mc.spec_id
		LEFT JOIN unit u           ON u.id = mc.unit_id
		JOIN cost_subgroup csg     ON csg.id = ici.cost_subgroup_id
		JOIN cost_group cg         ON cg.id = csg.group_id
		JOIN cost_job cj           ON cj.id = cg.job_id
		JOIN cost_subject csub     ON csub.id = cj.subject_id`+where+`
		ORDER BY ici.mat_code`, args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	result := []models.IcAvailableMaterial{}
	for rows.Next() {
		var m models.IcAvailableMaterial
		if err := rows.Scan(&m.MatCode, &m.MatName, &m.CostSubgroupID, &m.CostCode, &m.CostName, &m.Unit, &m.QtyOnHand); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		result = append(result, m)
	}

	return c.JSON(fiber.Map{"success": true, "data": result})
}

// ListProjectCostCodes godoc
// @Summary      List cost-subgroup codes for a project, across all of its job_codes
// @Description  For the ToCostCode dropdown — :projectCode here is the TARGET project (ToProject), re-called whenever the user changes it, not the movement's own project. Returns every cost_subgroup under any job_code assigned to this project. Each project.job_codes entry (e.g. "MP") is matched by reconstructing it as cost_subject.subject_code || cost_job.job_code (subject_code is 1 char, cost_job.job_code is 1 char) — the same decomposition ListAvailableMaterials uses, since cost_job.job_code alone is a different, single-letter code space.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path  string  true  "target project.project_code (ToProject)"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/cost-codes [get]
func (h *IcProjectMovementHandler) ListProjectCostCodes(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "projectCode is required")
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT csg.id,
		       csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code,
		       csg.subgroup_name
		FROM cost_subgroup csg
		JOIN cost_group cg  ON cg.id = csg.group_id
		JOIN cost_job cj    ON cj.id = cg.job_id
		JOIN cost_subject csub ON csub.id = cj.subject_id
		JOIN project p      ON p.project_code = $1
		WHERE (csub.subject_code || cj.job_code) = ANY(p.job_codes)
		ORDER BY 2`, projectCode)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	result := []models.IcCostCodeOption{}
	for rows.Next() {
		var o models.IcCostCodeOption
		if err := rows.Scan(&o.CostSubgroupID, &o.CostCode, &o.SubgroupName); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		result = append(result, o)
	}

	return c.JSON(fiber.Map{"success": true, "data": result})
}

// AddMovementLine godoc
// @Summary      Add a line item to an IC Project Movement (staged, not yet deducted)
// @Description  Validates qty against ic_project_receipt_pool remaining = qty_received - qty_issued (locked FOR UPDATE) for (project_code, mat_code, cost_subgroup_id) and rejects with 400 if insufficient. For doc_type='ISSUE' the destination is forced to (movement.project_code, the line's own cost_subgroup_id) and any to_project_code/to_cost_subgroup_id in the body is ignored; for 'TRANSFER' both are required. Only DRAFT movements accept new lines. Does NOT deduct qty_on_hand here — deduction happens only when the whole movement document is confirmed/submitted (a separate, not-yet-built step). Inserts with the next line_no for this movement.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        projectCode  path  string                                       true  "movement's own project.project_code"
// @Param        id           path  int                                          true  "ic_project_movement.id"
// @Param        body         body  models.CreateIcProjectMovementLineRequest   true  "Line item"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/movements/{id}/lines [post]
func (h *IcProjectMovementHandler) AddMovementLine(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	movementID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var req models.CreateIcProjectMovementLineRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.MatCode == "" || req.Qty <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "mat_code and qty are required")
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	defer tx.Rollback(ctx)

	var movementProjectCode, docType, movementStatus string
	err = tx.QueryRow(ctx, `
		SELECT project_code, doc_type, status FROM ic_project_movement WHERE id = $1`, movementID,
	).Scan(&movementProjectCode, &docType, &movementStatus)
	if err == pgx.ErrNoRows || (err == nil && movementProjectCode != projectCode) {
		return fiber.NewError(fiber.StatusNotFound, "movement not found")
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if movementStatus != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest, "movement is not DRAFT — lines can no longer be added")
	}

	linkedWH, err := linkedWarehouseCode(ctx, tx, projectCode)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if linkedWH != "" && docType == "ISSUE" {
		return fiber.NewError(fiber.StatusBadRequest, errLinkedIssue)
	}
	if linkedWH == "" && req.CostSubgroupID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "cost_subgroup_id is required")
	}

	dstLinked := ""
	if docType == "ISSUE" {
		// ISSUE always posts back into the exact same (project_code, cost_subgroup_id) it came
		// from — whatever the client sent for to_project_code/to_cost_subgroup_id is ignored.
		req.ToProjectCode = movementProjectCode
		req.ToCostSubgroupID = req.CostSubgroupID
	} else {
		if req.ToProjectCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, "to_project_code is required for a TRANSFER line")
		}
		dl, derr := linkedWarehouseCode(ctx, tx, req.ToProjectCode)
		if derr == pgx.ErrNoRows {
			return fiber.NewError(fiber.StatusBadRequest, "destination project not found")
		} else if derr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, derr.Error())
		}
		dstLinked = dl
		if dstLinked == "" && req.ToCostSubgroupID == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "to_cost_subgroup_id is required for a TRANSFER line")
		}
		if linkedWH != "" && dstLinked != "" {
			return fiber.NewError(fiber.StatusBadRequest, "both source and destination are warehouse-linked; use Stock Transfer instead")
		}
	}

	if docType == "TRANSFER" && linkedWH == "" && dstLinked == "" && req.ToProjectCode == projectCode && req.ToCostSubgroupID == req.CostSubgroupID {
		return fiber.NewError(fiber.StatusBadRequest, "TRANSFER source and destination are identical")
	}

	// ISSUE draws from the receipt pool; TRANSFER from the project's cost-item balance.
	availSQL := `
		SELECT qty_received - qty_issued FROM ic_project_receipt_pool
		WHERE project_code = $1 AND mat_code = $2 AND cost_subgroup_id = $3
		FOR UPDATE`
	if docType == "TRANSFER" {
		availSQL = `
		SELECT qty_on_hand FROM ic_project_cost_item
		WHERE project_code = $1 AND mat_code = $2 AND cost_subgroup_id = $3
		FOR UPDATE`
	}
	var qtyOnHand float64
	if linkedWH != "" {
		err = tx.QueryRow(ctx, `SELECT qty FROM stock_item WHERE mat_code = $1 FOR UPDATE`, req.MatCode).Scan(&qtyOnHand)
	} else {
		err = tx.QueryRow(ctx, availSQL, projectCode, req.MatCode, req.CostSubgroupID).Scan(&qtyOnHand)
	}
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusBadRequest, "qty exceeds available (0.0000)")
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if qtyOnHand < req.Qty {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("qty exceeds available (%.4f)", qtyOnHand))
	}

	var nextLineNo int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(line_no), 0) + 1 FROM ic_project_movement_line WHERE movement_id = $1`, movementID,
	).Scan(&nextLineNo); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	claims := middleware.GetClaims(c)

	var newLineID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO ic_project_movement_line
		    (movement_id, line_no, mat_code, cost_subgroup_id, qty, to_project_code, to_cost_subgroup_id, remarks, created_at, created_by)
		VALUES
		    ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9)
		RETURNING id`,
		movementID, nextLineNo, req.MatCode, nullableID(req.CostSubgroupID), req.Qty, req.ToProjectCode, nullableID(req.ToCostSubgroupID), req.Remarks, claims.UserID,
	).Scan(&newLineID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": newLineID, "line_no": nextLineNo}})
}

// ListMovementLines godoc
// @Summary      List line items for an IC Project Movement (detail page table)
// @Description  Joins back to material_code/cost_subgroup hierarchy (source and target) and project for display, same field shape as available-materials/cost-codes plus to_project_name.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path  string  true  "movement's own project.project_code"
// @Param        id           path  int     true  "ic_project_movement.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/movements/{id}/lines [get]
func (h *IcProjectMovementHandler) ListMovementLines(c *fiber.Ctx) error {
	movementID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT l.id, l.line_no, l.mat_code,
		       NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), l.mat_code),
		       COALESCE(l.cost_subgroup_id, 0),
		       COALESCE(csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code, ''),
		       COALESCE(csg.subgroup_name, ''),
		       u.unit_name,
		       l.qty,
		       l.to_project_code, tp.project_name,
		       COALESCE(l.to_cost_subgroup_id, 0),
		       COALESCE(tcsub.subject_code || tcj.job_code || tcg.group_code || tcsg.subgroup_code, ''),
		       COALESCE(tcsg.subgroup_name, ''),
		       l.remarks
		FROM ic_project_movement_line l
		JOIN material_code mc      ON mc.mat_code = l.mat_code
		LEFT JOIN mat_name mn      ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size sp     ON sp.id = mc.spec_id
		LEFT JOIN unit u           ON u.id = mc.unit_id
		LEFT JOIN cost_subgroup csg ON csg.id = l.cost_subgroup_id
		LEFT JOIN cost_group cg    ON cg.id = csg.group_id
		LEFT JOIN cost_job cj      ON cj.id = cg.job_id
		LEFT JOIN cost_subject csub ON csub.id = cj.subject_id
		JOIN project tp            ON tp.project_code = l.to_project_code
		LEFT JOIN cost_subgroup tcsg ON tcsg.id = l.to_cost_subgroup_id
		LEFT JOIN cost_group tcg   ON tcg.id = tcsg.group_id
		LEFT JOIN cost_job tcj     ON tcj.id = tcg.job_id
		LEFT JOIN cost_subject tcsub ON tcsub.id = tcj.subject_id
		WHERE l.movement_id = $1
		ORDER BY l.line_no`, movementID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	result := []models.IcProjectMovementLine{}
	for rows.Next() {
		var l models.IcProjectMovementLine
		if err := rows.Scan(&l.ID, &l.LineNo, &l.MatCode, &l.MatName, &l.CostSubgroupID, &l.CostCode, &l.CostName,
			&l.Unit, &l.Qty, &l.ToProjectCode, &l.ToProjectName, &l.ToCostSubgroupID, &l.ToCostCode, &l.ToCostName, &l.Remarks); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		result = append(result, l)
	}

	return c.JSON(fiber.Map{"success": true, "data": result})
}

// ─── Submit (confirm) ───────────────────────────────────────────────────────

type icCostItemKey struct {
	ProjectCode    string
	MatCode        string
	CostSubgroupID int64
}

type icCostItemState struct {
	ID        int64
	QtyOnHand float64
	UnitCost  float64
}

// SubmitMovement godoc
// @Summary      Submit (confirm/post) an IC Project Movement
// @Description  One all-or-nothing transaction. Locks every touched source ic_project_receipt_pool row and destination ic_project_cost_item row (each set deduped and sorted by id ascending, pools first then cost items, to avoid deadlocks with concurrent movements), re-validates remaining (qty_received - qty_issued) under lock, then for each line adds qty to the pool's qty_issued (logged as ISSUE_OUT / ref_type MOVEMENT in ic_project_receipt_pool_transaction) and credits the destination ic_project_cost_item (logged in ic_project_cost_item_transaction, ref_type MOVEMENT_ISSUE/MOVEMENT_TRANSFER). A missing destination ic_project_cost_item row is auto-created. Every line is posted, including ISSUE lines whose destination key equals the source key (source is the receipt pool, destination is the cost item — different tables). Insufficient source remaining aborts the whole submit with 400. Transitions ic_project_movement.status DRAFT -> POSTED; already-POSTED rejects with 400.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        projectCode  path  string  true  "movement's own project.project_code"
// @Param        id           path  int     true  "ic_project_movement.id"
// @Param        body         body  object  false "optional {\"lines\": [CreateIcProjectMovementLineRequest]} — replaces staged DRAFT lines in the same tx"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/movements/{id}/submit [post]
func (h *IcProjectMovementHandler) SubmitMovement(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	movementID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	defer tx.Rollback(ctx)

	var movementProjectCode, docType, status string
	err = tx.QueryRow(ctx, `
		SELECT project_code, doc_type, status FROM ic_project_movement WHERE id = $1 FOR UPDATE`, movementID,
	).Scan(&movementProjectCode, &docType, &status)
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusNotFound, "movement not found")
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if movementProjectCode != projectCode {
		return fiber.NewError(fiber.StatusNotFound, "movement not found")
	}
	if status == "POSTED" {
		return fiber.NewError(fiber.StatusBadRequest, "movement already submitted")
	}
	if status != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest, "movement status is not DRAFT")
	}

	// Optional bundled lines: when the body carries a "lines" array, it replaces whatever DRAFT
	// lines were staged before, inside this same transaction (source qty is re-validated under
	// lock further below, so nothing extra is checked here beyond shape/destination).
	var body struct {
		Lines []models.CreateIcProjectMovementLineRequest `json:"lines"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
	}
	linkedWH, err := linkedWarehouseCode(ctx, tx, movementProjectCode)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if linkedWH != "" && docType == "ISSUE" {
		return fiber.NewError(fiber.StatusBadRequest, errLinkedIssue)
	}
	if len(body.Lines) > 0 {
		claims := middleware.GetClaims(c)
		if _, err := tx.Exec(ctx, `DELETE FROM ic_project_movement_line WHERE movement_id = $1`, movementID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		for i, ln := range body.Lines {
			if ln.MatCode == "" || ln.Qty <= 0 || (linkedWH == "" && ln.CostSubgroupID == 0) {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: mat_code, cost_subgroup_id, qty are required", i+1))
			}
			if docType == "ISSUE" {
				ln.ToProjectCode = movementProjectCode
				ln.ToCostSubgroupID = ln.CostSubgroupID
			} else {
				if ln.ToProjectCode == "" {
					return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: to_project_code is required for TRANSFER", i+1))
				}
				dl, derr := linkedWarehouseCode(ctx, tx, ln.ToProjectCode)
				if derr == pgx.ErrNoRows {
					return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: destination project not found", i+1))
				} else if derr != nil {
					return fiber.NewError(fiber.StatusInternalServerError, derr.Error())
				}
				if dl == "" && ln.ToCostSubgroupID == 0 {
					return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: to_cost_subgroup_id is required for TRANSFER", i+1))
				}
				if linkedWH != "" && dl != "" {
					return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: both source and destination are warehouse-linked; use Stock Transfer instead", i+1))
				}
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code = $1)`, ln.ToProjectCode).Scan(&exists); err != nil {
					return fiber.NewError(fiber.StatusInternalServerError, err.Error())
				}
				if !exists {
					return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line[%d]: destination project not found", i+1))
				}
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO ic_project_movement_line
				    (movement_id, line_no, mat_code, cost_subgroup_id, qty, to_project_code, to_cost_subgroup_id, remarks, created_at, created_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9)`,
				movementID, i+1, ln.MatCode, nullableID(ln.CostSubgroupID), ln.Qty, ln.ToProjectCode, nullableID(ln.ToCostSubgroupID), ln.Remarks, claims.UserID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
	}

	type movementLine struct {
		ID               int64
		LineNo           int
		MatCode          string
		CostSubgroupID   int64
		Qty              float64
		ToProjectCode    string
		ToCostSubgroupID int64
		Remarks          *string
	}

	rows, err := tx.Query(ctx, `
		SELECT id, line_no, mat_code, COALESCE(cost_subgroup_id, 0), qty, to_project_code, COALESCE(to_cost_subgroup_id, 0), remarks
		FROM ic_project_movement_line WHERE movement_id = $1 ORDER BY line_no`, movementID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	var lines []movementLine
	for rows.Next() {
		var l movementLine
		if err := rows.Scan(&l.ID, &l.LineNo, &l.MatCode, &l.CostSubgroupID, &l.Qty, &l.ToProjectCode, &l.ToCostSubgroupID, &l.Remarks); err != nil {
			rows.Close()
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		lines = append(lines, l)
	}
	rows.Close()
	if len(lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "movement has no line items")
	}

	// Resolve the source ic_project_receipt_pool id and destination ic_project_cost_item id for
	// every line (destination row is auto-created if missing).
	type resolvedLine struct {
		line   movementLine
		srcKey icCostItemKey
		dstKey icCostItemKey
		dstWH  string // non-empty: destination project is warehouse-linked (credit general stock)
	}
	var resolved []resolvedLine
	poolIDByKey := map[icCostItemKey]int64{}
	itemIDByKey := map[icCostItemKey]int64{}

	resolvePoolID := func(key icCostItemKey) (int64, error) {
		if id, ok := poolIDByKey[key]; ok {
			return id, nil
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			SELECT id FROM ic_project_receipt_pool
			WHERE project_code = $1 AND mat_code = $2 AND cost_subgroup_id = $3`,
			key.ProjectCode, key.MatCode, key.CostSubgroupID,
		).Scan(&id); err != nil {
			return 0, err
		}
		poolIDByKey[key] = id
		return id, nil
	}
	resolveOrCreateItemID := func(key icCostItemKey) (int64, error) {
		if id, ok := itemIDByKey[key]; ok {
			return id, nil
		}
		// item_name/unit mirror icReceiveToProjectCost's material_code join.
		if _, err := tx.Exec(ctx, `
			INSERT INTO ic_project_cost_item (project_code, mat_code, cost_subgroup_id, item_name, unit, qty_on_hand, last_unit_cost)
			SELECT $1, mc.mat_code, $3,
			       COALESCE(NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), ''), mc.mat_code),
			       u.unit_name, 0, 0
			FROM material_code mc
			LEFT JOIN mat_name mn  ON mc.mat_name_id = mn.id
			LEFT JOIN spec_size sp ON mc.spec_id = sp.id
			LEFT JOIN unit u       ON mc.unit_id = u.id
			WHERE mc.mat_code = $2
			ON CONFLICT (project_code, mat_code, cost_subgroup_id) DO NOTHING`,
			key.ProjectCode, key.MatCode, key.CostSubgroupID,
		); err != nil {
			return 0, err
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			SELECT id FROM ic_project_cost_item
			WHERE project_code = $1 AND mat_code = $2 AND cost_subgroup_id = $3`,
			key.ProjectCode, key.MatCode, key.CostSubgroupID,
		).Scan(&id); err != nil {
			return 0, err
		}
		itemIDByKey[key] = id
		return id, nil
	}

	for _, l := range lines {
		srcKey := icCostItemKey{ProjectCode: projectCode, MatCode: l.MatCode, CostSubgroupID: l.CostSubgroupID}
		dstKey := icCostItemKey{ProjectCode: l.ToProjectCode, MatCode: l.MatCode, CostSubgroupID: l.ToCostSubgroupID}
		// No skip-if-identical: the source is the receipt POOL and the destination is the cost
		// ITEM — different tables — so the same (project, mat_code, cost_subgroup) key on both
		// sides is the normal ISSUE case (pool -> the project's own cost item), not a no-op.

		if linkedWH != "" {
			// Source is the general stock system: nothing to resolve here, it is locked and
			// deducted per line below.
		} else if docType == "TRANSFER" {
			if srcKey == dstKey {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: TRANSFER source and destination are identical", l.MatCode))
			}
			var srcItemID int64
			if err := tx.QueryRow(ctx, `
				SELECT id FROM ic_project_cost_item
				WHERE project_code = $1 AND mat_code = $2 AND cost_subgroup_id = $3`,
				srcKey.ProjectCode, srcKey.MatCode, srcKey.CostSubgroupID,
			).Scan(&srcItemID); err == pgx.ErrNoRows {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: source cost item not found", l.MatCode))
			} else if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			itemIDByKey[srcKey] = srcItemID
		} else if _, err := resolvePoolID(srcKey); err == pgx.ErrNoRows {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: source receipt pool row not found", l.MatCode))
		} else if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		dstWH := ""
		if docType == "TRANSFER" {
			w, err := linkedWarehouseCode(ctx, tx, l.ToProjectCode)
			if err == pgx.ErrNoRows {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: destination project not found", l.MatCode))
			} else if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			dstWH = w
			if linkedWH != "" && dstWH != "" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: both source and destination are warehouse-linked; use Stock Transfer instead", l.MatCode))
			}
		}
		if dstWH == "" {
			if _, err := resolveOrCreateItemID(dstKey); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("%s: failed to resolve destination cost item: %s", l.MatCode, err.Error()))
			}
		}

		resolved = append(resolved, resolvedLine{line: l, srcKey: srcKey, dstKey: dstKey, dstWH: dstWH})
	}

	// Lock every distinct touched row before validating/mutating any of them. Pools and cost
	// items are different tables, so each set is sorted by id ascending and pools are always
	// locked before cost items — a consistent global order across concurrent submits.
	poolIDSet := map[int64]bool{}
	itemIDSet := map[int64]bool{}
	for _, r := range resolved {
		if linkedWH != "" {
			// no pool / source cost item to lock
		} else if docType == "TRANSFER" {
			itemIDSet[itemIDByKey[r.srcKey]] = true
		} else {
			poolIDSet[poolIDByKey[r.srcKey]] = true
		}
		if r.dstWH == "" {
			itemIDSet[itemIDByKey[r.dstKey]] = true
		}
	}
	sortedIDs := func(set map[int64]bool) []int64 {
		out := make([]int64, 0, len(set))
		for id := range set {
			out = append(out, id)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}

	type poolState struct {
		Remaining float64
		UnitCost  float64
	}
	poolStateByID := map[int64]*poolState{}
	for _, id := range sortedIDs(poolIDSet) {
		var st poolState
		if err := tx.QueryRow(ctx, `
			SELECT qty_received - qty_issued, COALESCE(last_unit_cost, 0)
			FROM ic_project_receipt_pool WHERE id = $1 FOR UPDATE`, id,
		).Scan(&st.Remaining, &st.UnitCost); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		poolStateByID[id] = &st
	}
	itemStateByID := map[int64]*icCostItemState{}
	for _, id := range sortedIDs(itemIDSet) {
		var st icCostItemState
		st.ID = id
		if err := tx.QueryRow(ctx, `
			SELECT qty_on_hand, last_unit_cost FROM ic_project_cost_item WHERE id = $1 FOR UPDATE`, id,
		).Scan(&st.QtyOnHand, &st.UnitCost); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		itemStateByID[id] = &st
	}

	claims := middleware.GetClaims(c)
	refType := "MOVEMENT_TRANSFER"
	if docType == "ISSUE" {
		refType = "MOVEMENT_ISSUE"
	}

	for _, r := range resolved {
		dstID := itemIDByKey[r.dstKey]
		dstState := itemStateByID[dstID]
		var unitCost float64

		if linkedWH != "" {
			// Warehouse-linked TRANSFER: deduct stock_item.qty directly and log
			// in stock_transaction (the real stock ledger); the destination cost-item credit
			// below is shared with the normal path. Deliberate, confirmed exception.
			var itemID int64
			var itemQty float64
			var itemLoc string
			if err := tx.QueryRow(ctx, `SELECT id, qty, location_code FROM stock_item WHERE mat_code = $1 FOR UPDATE`, r.line.MatCode).Scan(&itemID, &itemQty, &itemLoc); err == pgx.ErrNoRows {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: not found in stock_item", r.line.MatCode))
			} else if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			if itemQty < r.line.Qty {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: qty exceeds available (%.4f)", r.line.MatCode, itemQty))
			}
			if _, err := tx.Exec(ctx, `UPDATE stock_item SET qty = qty - $1, updated_at = NOW() WHERE id = $2`, r.line.Qty, itemID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			var seq int64
			if err := tx.QueryRow(ctx, `SELECT nextval('stock_txn_seq')`).Scan(&seq); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO stock_transaction
				    (txn_no, txn_type, item_id, from_location, qty, ref_doc_type, ref_doc_id, remarks, qty_before, qty_after, created_by)
				VALUES ($1, 'OUT', $2, $3, $4, 'IC_MOVEMENT', $5, $6, $7, $8, $9)`,
				fmt.Sprintf("TXN-%s-%04d", timeYYMM(), seq), itemID, itemLoc, r.line.Qty, movementID, r.line.Remarks,
				itemQty, itemQty-r.line.Qty, claims.UserID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			unitCost = dstState.UnitCost
		} else if docType == "TRANSFER" {
			srcID := itemIDByKey[r.srcKey]
			src := itemStateByID[srcID]
			if src.QtyOnHand < r.line.Qty {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: qty exceeds available (%.4f)", r.line.MatCode, src.QtyOnHand))
			}
			qtyBeforeSrc := src.QtyOnHand
			src.QtyOnHand -= r.line.Qty
			unitCost = src.UnitCost
			if _, err := tx.Exec(ctx, `UPDATE ic_project_cost_item SET qty_on_hand = $1, updated_at = NOW() WHERE id = $2`, src.QtyOnHand, srcID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO ic_project_cost_item_transaction
				    (project_cost_item_id, qty, qty_before, qty_after, unit_cost, remarks, txn_date, movement_id, movement_line_id, ref_type, created_by)
				VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7, $8, $9, $10)`,
				srcID, -r.line.Qty, qtyBeforeSrc, src.QtyOnHand, unitCost, r.line.Remarks, movementID, r.line.ID, refType, claims.UserID,
			); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		} else {
			poolID := poolIDByKey[r.srcKey]
			pool := poolStateByID[poolID]
			unitCost = pool.UnitCost
			if pool.Remaining < r.line.Qty {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: qty exceeds available (%.4f)", r.line.MatCode, pool.Remaining))
			}
			pool.Remaining -= r.line.Qty

			if _, err := tx.Exec(ctx, `
			UPDATE ic_project_receipt_pool SET qty_issued = qty_issued + $1, updated_at = NOW() WHERE id = $2`,
				r.line.Qty, poolID,
			); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			if _, err := tx.Exec(ctx, `
			INSERT INTO ic_project_receipt_pool_transaction
			    (receipt_pool_id, txn_type, qty, ref_type, movement_id, movement_line_id, created_by)
			VALUES ($1, 'ISSUE_OUT', $2, 'MOVEMENT', $3, $4, $5)`,
				poolID, r.line.Qty, movementID, r.line.ID, claims.UserID,
			); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}

		if r.dstWH != "" {
			// Project -> Stock: credit stock_item.qty (flat, global) instead of a
			// destination cost item; the source cost-item deduction above already ran.
			// Same auto-create pattern as CreateMaterial: stock_item must exist for the mat_code.
			if _, err := tx.Exec(ctx, `
				INSERT INTO stock_item (mat_code, item_name, unit, qty)
				SELECT mc.mat_code,
				       COALESCE(NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), ''), mc.mat_code),
				       COALESCE(u.unit_name, '-'), 0
				FROM material_code mc
				LEFT JOIN mat_name mn  ON mc.mat_name_id = mn.id
				LEFT JOIN spec_size sp ON mc.spec_id = sp.id
				LEFT JOIN unit u       ON mc.unit_id = u.id
				WHERE mc.mat_code = $1
				ON CONFLICT (mat_code) DO NOTHING`, r.line.MatCode); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			var itemID int64
			var itemBefore float64
			var itemLoc string
			if err := tx.QueryRow(ctx, `SELECT id, qty, location_code FROM stock_item WHERE mat_code = $1 FOR UPDATE`, r.line.MatCode).Scan(&itemID, &itemBefore, &itemLoc); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: not found in stock_item / material_code", r.line.MatCode))
			}
			if _, err := tx.Exec(ctx, `UPDATE stock_item SET qty = qty + $1, updated_at = NOW() WHERE id = $2`, r.line.Qty, itemID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			var seq int64
			if err := tx.QueryRow(ctx, `SELECT nextval('stock_txn_seq')`).Scan(&seq); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO stock_transaction
				    (txn_no, txn_type, item_id, to_location, qty, ref_doc_type, ref_doc_id, remarks, qty_before, qty_after, created_by)
				VALUES ($1, 'IN', $2, $3, $4, 'IC_MOVEMENT', $5, $6, $7, $8, $9)`,
				fmt.Sprintf("TXN-%s-%04d", timeYYMM(), seq), itemID, itemLoc, r.line.Qty, movementID, r.line.Remarks,
				itemBefore, itemBefore+r.line.Qty, claims.UserID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			continue
		}

		qtyBeforeDst := dstState.QtyOnHand
		qtyAfterDst := qtyBeforeDst + r.line.Qty
		dstState.QtyOnHand = qtyAfterDst

		if _, err := tx.Exec(ctx, `UPDATE ic_project_cost_item SET qty_on_hand = $1, updated_at = NOW() WHERE id = $2`, qtyAfterDst, dstID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ic_project_cost_item_transaction
			    (project_cost_item_id, qty, qty_before, qty_after, unit_cost, remarks, txn_date, movement_id, movement_line_id, ref_type, created_by)
			VALUES
			    ($1, $2, $3, $4, $5, $6, NOW(), $7, $8, $9, $10)`,
			dstID, r.line.Qty, qtyBeforeDst, qtyAfterDst, unitCost, r.line.Remarks, movementID, r.line.ID, refType, claims.UserID,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE ic_project_movement SET status = 'POSTED', updated_at = NOW() WHERE id = $1`, movementID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": movementID, "status": "POSTED"}})
}

// ─── Cost transaction history ───────────────────────────────────────────────

// ListCostTransactions godoc
// @Summary      List IC Project cost-item transaction history for a project
// @Description  Parallel to the Stock Transaction page, but scoped to one project and reading from ic_project_cost_item_transaction. created_by_name is resolved via users.full_name (matching this project's established display convention everywhere else — e.g. GetMovement, ListMovements — not username).
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path   string  true   "project.project_code"
// @Param        ref_type     query  string  false  "Filter: PO, MOVEMENT_ISSUE, or MOVEMENT_TRANSFER"
// @Param        search       query  string  false  "Search mat_code/mat_name"
// @Param        date_from    query  string  false  "Date from (YYYY-MM-DD)"
// @Param        date_to      query  string  false  "Date to (YYYY-MM-DD)"
// @Param        page         query  int     false  "page number"  default(1)
// @Param        page_size    query  int     false  "page size"    default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/cost-transactions [get]
func (h *IcProjectMovementHandler) ListCostTransactions(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "projectCode is required")
	}

	var f models.IcProjectCostTransactionFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	page := max(f.Page, 1)
	size := f.PageSize
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	offset := (page - 1) * size

	where := []string{"ici.project_code = $1"}
	args := []interface{}{projectCode}
	i := 2

	if f.RefType != "" {
		where = append(where, fmt.Sprintf("t.ref_type = $%d", i))
		args = append(args, f.RefType)
		i++
	}
	if f.DateFrom != "" {
		where = append(where, fmt.Sprintf("t.txn_date >= $%d", i))
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where = append(where, fmt.Sprintf("t.txn_date <= $%d", i))
		args = append(args, f.DateTo)
		i++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf("(ici.mat_code ILIKE $%d OR mn.mat_name ILIKE $%d)", i, i))
		args = append(args, "%"+f.Search+"%")
		i++
	}
	whereClause := strings.Join(where, " AND ")

	joinClause := `
		FROM ic_project_cost_item_transaction t
		JOIN ic_project_cost_item ici ON ici.id = t.project_cost_item_id
		JOIN material_code mc         ON mc.mat_code = ici.mat_code
		LEFT JOIN mat_name mn         ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size sp        ON sp.id = mc.spec_id
		JOIN cost_subgroup csg        ON csg.id = ici.cost_subgroup_id
		JOIN cost_group cg            ON cg.id = csg.group_id
		JOIN cost_job cj              ON cj.id = cg.job_id
		JOIN cost_subject csub        ON csub.id = cj.subject_id
		LEFT JOIN purchase_order po   ON po.id = t.po_id
		LEFT JOIN ic_project_movement m ON m.id = t.movement_id
		LEFT JOIN users u             ON u.id = t.created_by`

	var total int64
	if err := h.db.QueryRow(context.Background(), fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, joinClause, whereClause), args...).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to count cost transactions: "+err.Error())
	}

	args = append(args, size, offset)
	rows, err := h.db.Query(context.Background(), fmt.Sprintf(`
		SELECT t.id, t.qty, t.qty_before, t.qty_after, t.unit_cost, t.remarks, t.txn_date::text, t.ref_type,
		       ici.mat_code,
		       NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), ici.mat_code),
		       ici.cost_subgroup_id,
		       csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code,
		       csg.subgroup_name,
		       CASE WHEN t.ref_type = 'PO' THEN po.po_no ELSE m.doc_no END,
		       COALESCE(u.full_name, ''),
		       t.created_at
		%s
		WHERE %s
		ORDER BY t.txn_date DESC, t.id DESC
		LIMIT $%d OFFSET $%d`, joinClause, whereClause, i, i+1), args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list cost transactions: "+err.Error())
	}
	defer rows.Close()

	items := []models.IcProjectCostTransaction{}
	for rows.Next() {
		var t models.IcProjectCostTransaction
		if err := rows.Scan(&t.ID, &t.Qty, &t.QtyBefore, &t.QtyAfter, &t.UnitCost, &t.Remarks, &t.TxnDate, &t.RefType,
			&t.MatCode, &t.MatName, &t.CostSubgroupID, &t.CostCode, &t.CostName, &t.RefNo, &t.CreatedByName, &t.CreatedAt); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan cost transaction: "+err.Error())
		}
		items = append(items, t)
	}

	totalPages := int(total) / size
	if int(total)%size != 0 || total == 0 {
		totalPages++
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages,
		},
	})
}

// ─── Project stock balance ──────────────────────────────────────────────────

// ListProjectStock godoc
// @Summary      Current cost-item balance list for a project
// @Description  Reads ic_project_cost_item (the destination-side balance table) for this project, joined to material_code for the display name (mat_name + spec_description, same pattern as available-materials) and to the 4-level cost code hierarchy. only_with_stock defaults to true (qty_on_hand > 0); pass only_with_stock=false to also see zero-balance rows. job_code (e.g. "MP") uses the same decomposition as available-materials (cost_subject.subject_code = first character, cost_job.job_code = the rest). search matches mat_code or the composed mat_name (case-insensitive). Note this is the cost-item balance only — received-but-not-yet-issued quantity lives in ic_project_receipt_pool and is not included here.
// @Tags         IC Project Movement
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode     path   string  true   "project.project_code"
// @Param        job_code        query  string  false  "Filter by job code (e.g. MP)"
// @Param        search          query  string  false  "Match mat_code or mat_name"
// @Param        only_with_stock query  bool    false  "Only rows with qty_on_hand > 0 (default true)"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /ic/projects/{projectCode}/stock [get]
func (h *IcProjectMovementHandler) ListProjectStock(c *fiber.Ctx) error {
	projectCode := c.Params("projectCode")
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "projectCode is required")
	}
	jobCode := strings.TrimSpace(c.Query("job_code"))
	search := strings.TrimSpace(c.Query("search"))
	onlyWithStock := c.QueryBool("only_with_stock", true)

	linkedWH, err := linkedWarehouseCode(context.Background(), h.db, projectCode)
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	} else if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if linkedWH != "" {
		// Warehouse-linked project: its balance lives in the general stock system, not in
		// ic_project_cost_item. job_code is ignored (warehouse stock has no cost subgroup).
		items, err := queryLinkedWarehouseStock(context.Background(), h.db, 0, search, onlyWithStock)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		result := make([]fiber.Map, 0, len(items))
		for _, it := range items {
			result = append(result, fiber.Map{
				"id": it.ItemID, "mat_code": it.MatCode, "mat_name": it.MatName,
				"cost_subgroup_id": it.CostSubgroupID, "cost_code": it.CostCode,
				"unit": it.Unit, "qty_on_hand": it.Qty, "last_unit_cost": it.UnitCost, "updated_at": it.UpdatedAt,
			})
		}
		return c.JSON(fiber.Map{"success": true, "data": result})
	}

	where := []string{"ici.project_code = $1"}
	args := []interface{}{projectCode}
	if onlyWithStock {
		where = append(where, "ici.qty_on_hand > 0")
	}
	if jobCode != "" {
		args = append(args, jobCode)
		where = append(where, fmt.Sprintf("csub.subject_code = LEFT($%[1]d, 1) AND cj.job_code = SUBSTRING($%[1]d FROM 2)", len(args)))
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		where = append(where, fmt.Sprintf(
			"(ici.mat_code ILIKE $%[1]d OR NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), '') ILIKE $%[1]d)",
			len(args)))
	}

	rows, err := h.db.Query(context.Background(), fmt.Sprintf(`
		SELECT ici.id, ici.mat_code,
		       NULLIF(TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(sp.spec_description, '')), ''),
		       ici.cost_subgroup_id,
		       csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code,
		       COALESCE(u.unit_name, ici.unit),
		       ici.qty_on_hand, ici.last_unit_cost, ici.updated_at
		FROM ic_project_cost_item ici
		JOIN material_code mc      ON mc.mat_code = ici.mat_code
		LEFT JOIN mat_name mn      ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size sp     ON sp.id = mc.spec_id
		LEFT JOIN unit u           ON u.id = mc.unit_id
		JOIN cost_subgroup csg     ON csg.id = ici.cost_subgroup_id
		JOIN cost_group cg         ON cg.id = csg.group_id
		JOIN cost_job cj           ON cj.id = cg.job_id
		JOIN cost_subject csub     ON csub.id = cj.subject_id
		WHERE %s
		ORDER BY mc.mat_code, ici.cost_subgroup_id`, strings.Join(where, " AND ")), args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	type stockRow struct {
		ID             int64     `json:"id"`
		MatCode        string    `json:"mat_code"`
		MatName        *string   `json:"mat_name"`
		CostSubgroupID int64     `json:"cost_subgroup_id"`
		CostCode       string    `json:"cost_code"`
		Unit           *string   `json:"unit"`
		QtyOnHand      float64   `json:"qty_on_hand"`
		LastUnitCost   float64   `json:"last_unit_cost"`
		UpdatedAt      time.Time `json:"updated_at"`
	}
	result := []stockRow{}
	for rows.Next() {
		var r stockRow
		if err := rows.Scan(&r.ID, &r.MatCode, &r.MatName, &r.CostSubgroupID, &r.CostCode,
			&r.Unit, &r.QtyOnHand, &r.LastUnitCost, &r.UpdatedAt); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{"success": true, "data": result})
}

package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

type CostCodeHandler struct {
	db *pgxpool.Pool
}

func NewCostCodeHandler(db *pgxpool.Pool) *CostCodeHandler {
	return &CostCodeHandler{db: db}
}

// ─── Cost Subject ───────────────────────────────────────────────────────────

// ListSubjects godoc
// @Summary      List cost subjects
// @Tags         CostCode
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /master/cost-code/subjects [get]
func (h *CostCodeHandler) ListSubjects(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT id, subject_code, subject_name, is_active, created_at, updated_at
		 FROM cost_subject ORDER BY subject_code`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type subject struct {
		ID          int64  `json:"id"`
		SubjectCode string `json:"subject_code"`
		SubjectName string `json:"subject_name"`
		IsActive    bool   `json:"is_active"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	}
	var items []subject
	for rows.Next() {
		var s subject
		if err := rows.Scan(&s.ID, &s.SubjectCode, &s.SubjectName, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return err
		}
		items = append(items, s)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// ─── Cost Job ───────────────────────────────────────────────────────────────

// ListJobs godoc
// @Summary      List cost jobs filtered by subject_id
// @Tags         CostCode
// @Security     BearerAuth
// @Produce      json
// @Param        subject_id  query  int  false  "subject_id filter"
// @Success      200  {object}  fiber.Map
// @Router       /master/cost-code/jobs [get]
func (h *CostCodeHandler) ListJobs(c *fiber.Ctx) error {
	subjectID := c.QueryInt("subject_id", 0)

	var rows pgx.Rows
	var err error
	if subjectID > 0 {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, subject_id, job_code, job_name, is_active, created_at, updated_at
			 FROM cost_job WHERE subject_id = $1 ORDER BY job_code`, subjectID)
	} else {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, subject_id, job_code, job_name, is_active, created_at, updated_at
			 FROM cost_job ORDER BY job_code`)
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	type job struct {
		ID        int64  `json:"id"`
		SubjectID int64  `json:"subject_id"`
		JobCode   string `json:"job_code"`
		JobName   string `json:"job_name"`
		IsActive  bool   `json:"is_active"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	var items []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.ID, &j.SubjectID, &j.JobCode, &j.JobName, &j.IsActive, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return err
		}
		items = append(items, j)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// ─── Cost Group ─────────────────────────────────────────────────────────────

// ListGroupsByJob godoc
// @Summary      List cost groups filtered by job_id
// @Tags         CostCode
// @Security     BearerAuth
// @Produce      json
// @Param        job_id  query  int  false  "job_id filter"
// @Success      200  {object}  fiber.Map
// @Router       /master/cost-code/groups [get]
func (h *CostCodeHandler) ListGroupsByJob(c *fiber.Ctx) error {
	jobID := c.QueryInt("job_id", 0)

	var rows pgx.Rows
	var err error
	if jobID > 0 {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, job_id, group_code, group_name, is_active, created_at, updated_at
			 FROM cost_group WHERE job_id = $1 ORDER BY group_code`, jobID)
	} else {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, job_id, group_code, group_name, is_active, created_at, updated_at
			 FROM cost_group ORDER BY group_code`)
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	type group struct {
		ID        int64  `json:"id"`
		JobID     int64  `json:"job_id"`
		GroupCode string `json:"group_code"`
		GroupName string `json:"group_name"`
		IsActive  bool   `json:"is_active"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	var items []group
	for rows.Next() {
		var g group
		if err := rows.Scan(&g.ID, &g.JobID, &g.GroupCode, &g.GroupName, &g.IsActive, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return err
		}
		items = append(items, g)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// ─── Cost Subgroup ──────────────────────────────────────────────────────────

// ListSubgroupsByGroup godoc
// @Summary      List cost subgroups filtered by group_id
// @Tags         CostCode
// @Security     BearerAuth
// @Produce      json
// @Param        group_id  query  int  false  "group_id filter"
// @Success      200  {object}  fiber.Map
// @Router       /master/cost-code/subgroups [get]
func (h *CostCodeHandler) ListSubgroupsByGroup(c *fiber.Ctx) error {
	groupID := c.QueryInt("group_id", 0)

	var rows pgx.Rows
	var err error
	if groupID > 0 {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, group_id, subgroup_code, subgroup_name, is_active, created_at, updated_at
			 FROM cost_subgroup WHERE group_id = $1 ORDER BY subgroup_code`, groupID)
	} else {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, group_id, subgroup_code, subgroup_name, is_active, created_at, updated_at
			 FROM cost_subgroup ORDER BY subgroup_code`)
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	type subgroup struct {
		ID           int64  `json:"id"`
		GroupID      int64  `json:"group_id"`
		SubgroupCode string `json:"subgroup_code"`
		SubgroupName string `json:"subgroup_name"`
		IsActive     bool   `json:"is_active"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
	}
	var items []subgroup
	for rows.Next() {
		var s subgroup
		if err := rows.Scan(&s.ID, &s.GroupID, &s.SubgroupCode, &s.SubgroupName, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return err
		}
		items = append(items, s)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// ─── Full joined list ───────────────────────────────────────────────────────

type CostCodeFull struct {
	SubgroupID   int64  `json:"subgroup_id"`
	CostCode     string `json:"cost_code"`
	SubjectCode  string `json:"subject_code"`
	SubjectName  string `json:"subject_name"`
	JobCode      string `json:"job_code"`
	JobName      string `json:"job_name"`
	GroupCode    string `json:"group_code"`
	GroupName    string `json:"group_name"`
	SubgroupCode string `json:"subgroup_code"`
	SubgroupName string `json:"subgroup_name"`
	IsActive     bool   `json:"is_active"`
}

const costCodeFullQuery = `
	SELECT
		sg.id,
		sub.subject_code || j.job_code || g.group_code || sg.subgroup_code AS cost_code,
		sub.subject_code, sub.subject_name,
		j.job_code, j.job_name,
		g.group_code, g.group_name,
		sg.subgroup_code, sg.subgroup_name,
		sg.is_active
	FROM cost_subgroup sg
	JOIN cost_group g ON g.id = sg.group_id
	JOIN cost_job j ON j.id = g.job_id
	JOIN cost_subject sub ON sub.id = j.subject_id
`

// ListFull godoc
// @Summary      Full joined cost-code list with computed cost_code string
// @Tags         CostCode
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /master/cost-code/full [get]
func (h *CostCodeHandler) ListFull(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(), costCodeFullQuery+` ORDER BY cost_code`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []CostCodeFull
	for rows.Next() {
		var f CostCodeFull
		if err := rows.Scan(&f.SubgroupID, &f.CostCode, &f.SubjectCode, &f.SubjectName,
			&f.JobCode, &f.JobName, &f.GroupCode, &f.GroupName,
			&f.SubgroupCode, &f.SubgroupName, &f.IsActive); err != nil {
			return err
		}
		items = append(items, f)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// ExportCostCode godoc
// @Summary      Export the full cost-code hierarchy (subject/job/group/subgroup) to Excel
// @Description  Rows are sorted by cost_subgroup.id ASC, preserving the exact row order of the original import file (subgroup, group, job, and subject tables were TRUNCATE...RESTART IDENTITY'd and re-imported in file order this session, so id order == file order). รหัสรวม (combined code) uses the same subject_code+job_code+group_code+subgroup_code concatenation as costCodeFullQuery / ListFull / ListCostCode — no separator, same rule everywhere in this codebase.
// @Tags         CostCode
// @Security     BearerAuth
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200  {file}  file
// @Failure      500  {object}  fiber.Map
// @Router       /master/cost-code/export [get]
func (h *CostCodeHandler) ExportCostCode(c *fiber.Ctx) error {
	ctx := context.Background()

	rows, err := h.db.Query(ctx, costCodeFullQuery+`
		ORDER BY sg.id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	f := excelize.NewFile()
	defer f.Close()

	const sheet = "CostCode"
	f.SetSheetName("Sheet1", sheet)

	boldStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return err
	}

	headers := []string{"Subject Code", "Subject Name", "Job Code", "Job Name", "Group Code", "Group Name", "Subgroup Code", "Subgroup Name", "รหัสรวม"}
	colWidths := []float64{14, 30, 12, 30, 14, 30, 16, 30, 16}
	for i, hdr := range headers {
		col := string(rune('A' + i))
		cell := col + "1"
		f.SetCellValue(sheet, cell, hdr)
		f.SetCellStyle(sheet, cell, cell, boldStyle)
		f.SetColWidth(sheet, col, col, colWidths[i])
	}

	rowIdx := 2
	for rows.Next() {
		var fr CostCodeFull
		if err := rows.Scan(&fr.SubgroupID, &fr.CostCode, &fr.SubjectCode, &fr.SubjectName,
			&fr.JobCode, &fr.JobName, &fr.GroupCode, &fr.GroupName,
			&fr.SubgroupCode, &fr.SubgroupName, &fr.IsActive); err != nil {
			return err
		}
		f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), fr.SubjectCode)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIdx), fr.SubjectName)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIdx), fr.JobCode)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIdx), fr.JobName)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIdx), fr.GroupCode)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", rowIdx), fr.GroupName)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", rowIdx), fr.SubgroupCode)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", rowIdx), fr.SubgroupName)
		f.SetCellValue(sheet, fmt.Sprintf("I%d", rowIdx), fr.CostCode)
		rowIdx++
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}

	filename := "cost_code_" + time.Now().Format("20060102") + ".xlsx"
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename="+filename)
	return c.Send(buf.Bytes())
}

// ─── Import ──────────────────────────────────────────────────────────────────

type CostCodeImportRow struct {
	SubjectCode  string `json:"subject_code"`
	SubjectName  string `json:"subject_name"`
	JobCode      string `json:"job_code"`
	JobName      string `json:"job_name"`
	GroupCode    string `json:"group_code"`
	GroupName    string `json:"group_name"`
	SubgroupCode string `json:"subgroup_code"`
	SubgroupName string `json:"subgroup_name"`
}

// ImportCostCode godoc
// @Summary      Bulk import cost-code hierarchy (subject/job/group/subgroup)
// @Description  Accepts an array of rows. Uses in-memory cache to skip duplicate FK lookups, then upserts each level of the hierarchy. subgroup rows with empty subgroup_name are skipped.
// @Tags         CostCode
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      []CostCodeImportRow  true  "Array of cost-code rows"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /master/cost-code/import [post]
func (h *CostCodeHandler) ImportCostCode(c *fiber.Ctx) error {
	var reqs []CostCodeImportRow
	if err := c.BodyParser(&reqs); err != nil {
		log.Println("❌ BodyParser:", err)
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(reqs) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "request body is empty")
	}

	for i, req := range reqs {
		if req.SubjectCode == "" || req.SubjectName == "" ||
			req.JobCode == "" || req.JobName == "" ||
			req.GroupCode == "" || req.GroupName == "" ||
			req.SubgroupCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("item[%d]: subject/job/group codes+names and subgroup_code are required", i))
		}
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		log.Println("❌ Begin tx:", err)
		return err
	}
	defer tx.Rollback(ctx)

	subjectCache := map[string]int64{}
	jobCache := map[string]int64{}
	groupCache := map[string]int64{}
	subgroupCache := map[string]int64{}
	// subgroupFirstRow records the Excel row (data row index + 2, accounting for the header
	// row) where each (group_id, subgroup_code) key was first seen in this upload, so a later
	// in-file duplicate can be logged against the row it duplicates.
	subgroupFirstRow := map[string]int{}

	var subjectCreated, subjectUpdated int
	var jobCreated, jobUpdated int
	var groupCreated, groupUpdated int
	var subgroupCreated, subgroupUpdated, subgroupSkipped, subgroupSkippedDup int

	for i, req := range reqs {
		// subject
		subjectID, ok := subjectCache[req.SubjectCode]
		if !ok {
			var inserted bool
			if err = tx.QueryRow(ctx,
				`INSERT INTO cost_subject (subject_code, subject_name, is_active, created_at, updated_at)
				 VALUES ($1, $2, true, NOW(), NOW())
				 ON CONFLICT (subject_code) DO UPDATE SET subject_name = EXCLUDED.subject_name, updated_at = NOW()
				 RETURNING id, (xmax = 0)`,
				req.SubjectCode, req.SubjectName,
			).Scan(&subjectID, &inserted); err != nil {
				log.Printf("❌ subject[%d]: %v", i, err)
				return err
			}
			subjectCache[req.SubjectCode] = subjectID
			if inserted {
				subjectCreated++
			} else {
				subjectUpdated++
			}
		}

		// job
		jobKey := fmt.Sprintf("%d:%s", subjectID, req.JobCode)
		jobID, ok := jobCache[jobKey]
		if !ok {
			var inserted bool
			if err = tx.QueryRow(ctx,
				`INSERT INTO cost_job (subject_id, job_code, job_name, is_active, created_at, updated_at)
				 VALUES ($1, $2, $3, true, NOW(), NOW())
				 ON CONFLICT (subject_id, job_code) DO UPDATE SET job_name = EXCLUDED.job_name, updated_at = NOW()
				 RETURNING id, (xmax = 0)`,
				subjectID, req.JobCode, req.JobName,
			).Scan(&jobID, &inserted); err != nil {
				log.Printf("❌ job[%d]: %v", i, err)
				return err
			}
			jobCache[jobKey] = jobID
			if inserted {
				jobCreated++
			} else {
				jobUpdated++
			}
		}

		// group
		groupKey := fmt.Sprintf("%d:%s", jobID, req.GroupCode)
		groupID, ok := groupCache[groupKey]
		if !ok {
			var inserted bool
			if err = tx.QueryRow(ctx,
				`INSERT INTO cost_group (job_id, group_code, group_name, is_active, created_at, updated_at)
				 VALUES ($1, $2, $3, true, NOW(), NOW())
				 ON CONFLICT (job_id, group_code) DO UPDATE SET group_name = EXCLUDED.group_name, updated_at = NOW()
				 RETURNING id, (xmax = 0)`,
				jobID, req.GroupCode, req.GroupName,
			).Scan(&groupID, &inserted); err != nil {
				log.Printf("❌ group[%d]: %v", i, err)
				return err
			}
			groupCache[groupKey] = groupID
			if inserted {
				groupCreated++
			} else {
				groupUpdated++
			}
		}

		// subgroup — skip if subgroup_name empty
		excelRow := i + 2 // +2: 1-based Excel rows, plus the header row consumed before data
		if req.SubgroupName == "" {
			subgroupSkipped++
			continue
		}
		subgroupKey := fmt.Sprintf("%d:%s", groupID, req.SubgroupCode)
		if firstRow, ok := subgroupFirstRow[subgroupKey]; ok {
			subgroupSkippedDup++
			log.Printf("[cost-code-import] Skipped duplicate row %d: subject=%s job=%s group=%s subgroup=%s (already seen at row %d)",
				excelRow, req.SubjectCode, req.JobCode, req.GroupCode, req.SubgroupCode, firstRow)
			continue
		}
		var subgroupID int64
		var inserted bool
		var existingName string
		if err = tx.QueryRow(ctx,
			`SELECT subgroup_name FROM cost_subgroup WHERE group_id = $1 AND subgroup_code = $2`,
			groupID, req.SubgroupCode,
		).Scan(&existingName); err != nil && err != pgx.ErrNoRows {
			log.Printf("❌ subgroup lookup[%d]: %v", i, err)
			return err
		}
		if err = tx.QueryRow(ctx,
			`INSERT INTO cost_subgroup (group_id, subgroup_code, subgroup_name, is_active, created_at, updated_at)
			 VALUES ($1, $2, $3, true, NOW(), NOW())
			 ON CONFLICT (group_id, subgroup_code) DO UPDATE SET subgroup_name = EXCLUDED.subgroup_name, updated_at = NOW()
			 RETURNING id, (xmax = 0)`,
			groupID, req.SubgroupCode, req.SubgroupName,
		).Scan(&subgroupID, &inserted); err != nil {
			log.Printf("❌ subgroup[%d]: %v", i, err)
			return err
		}
		subgroupFirstRow[subgroupKey] = excelRow
		subgroupCache[subgroupKey] = subgroupID
		if inserted {
			subgroupCreated++
		} else {
			subgroupUpdated++
			if existingName != req.SubgroupName {
				log.Printf("[cost-code-import] Row %d overwrote existing subgroup name for subject=%s job=%s group=%s subgroup=%s: %q -> %q",
					excelRow, req.SubjectCode, req.JobCode, req.GroupCode, req.SubgroupCode, existingName, req.SubgroupName)
			}
		}
	}

	log.Printf("[cost-code-import] Done: %d rows read, %d inserted, %d skipped as duplicates (%d overwritten, %d skipped for empty subgroup_name)",
		len(reqs), subgroupCreated, subgroupSkippedDup, subgroupUpdated, subgroupSkipped)

	if err = tx.Commit(ctx); err != nil {
		log.Println("❌ commit:", err)
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"subjects":  fiber.Map{"created": subjectCreated, "updated": subjectUpdated},
			"jobs":      fiber.Map{"created": jobCreated, "updated": jobUpdated},
			"groups":    fiber.Map{"created": groupCreated, "updated": groupUpdated},
			"subgroups": fiber.Map{"created": subgroupCreated, "updated": subgroupUpdated, "skipped": subgroupSkipped},
		},
		"message": "cost code hierarchy imported",
	})
}

// ─── List (paginated) ────────────────────────────────────────────────────────

// ListCostCode godoc
// @Summary      Get all cost codes with detail (paginated)
// @Tags         CostCode
// @Security     BearerAuth
// @Produce      json
// @Param        q     query  string  false  "Search keyword (any code/name field)"
// @Param        page  query  int     false  "Page number (default 1)"
// @Success      200   {object}  fiber.Map
// @Failure      500   {object}  fiber.Map
// @Router       /master/cost-code/list [get]
func (h *CostCodeHandler) ListCostCode(c *fiber.Ctx) error {
	q := c.Query("q")
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	const pageSize = 10
	offset := (page - 1) * pageSize

	conditions := []string{"1=1"}
	args := []interface{}{}
	idx := 1

	if q != "" {
		conditions = append(conditions, fmt.Sprintf(
			`(sub.subject_code ILIKE '%%'||$%[1]d||'%%' OR sub.subject_name ILIKE '%%'||$%[1]d||'%%'
			  OR j.job_code ILIKE '%%'||$%[1]d||'%%' OR j.job_name ILIKE '%%'||$%[1]d||'%%'
			  OR g.group_code ILIKE '%%'||$%[1]d||'%%' OR g.group_name ILIKE '%%'||$%[1]d||'%%'
			  OR sg.subgroup_code ILIKE '%%'||$%[1]d||'%%' OR sg.subgroup_name ILIKE '%%'||$%[1]d||'%%')`,
			idx))
		args = append(args, q)
		idx++
	}
	whereStr := strings.Join(conditions, " AND ")

	const joins = `
		FROM cost_subgroup sg
		JOIN cost_group g ON g.id = sg.group_id
		JOIN cost_job j ON j.id = g.job_id
		JOIN cost_subject sub ON sub.id = j.subject_id`

	var total int
	if err := h.db.QueryRow(context.Background(),
		`SELECT COUNT(*) `+joins+` WHERE `+whereStr, args...,
	).Scan(&total); err != nil {
		return err
	}

	selectArgs := append(append([]interface{}{}, args...), pageSize, offset)
	rows, err := h.db.Query(context.Background(),
		fmt.Sprintf(`SELECT
			sub.subject_code || j.job_code || g.group_code || sg.subgroup_code AS cost_code,
			sub.subject_name, j.job_name, g.group_name, sg.subgroup_name, sg.is_active
		%s WHERE %s
		ORDER BY cost_code LIMIT $%d OFFSET $%d`,
			joins, whereStr, idx, idx+1),
		selectArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type costCodeRow struct {
		CostCode     string `json:"cost_code"`
		SubjectName  string `json:"subject_name"`
		JobName      string `json:"job_name"`
		GroupName    string `json:"group_name"`
		SubgroupName string `json:"subgroup_name"`
		IsActive     bool   `json:"is_active"`
	}
	var items []costCodeRow
	for rows.Next() {
		var r costCodeRow
		if err := rows.Scan(&r.CostCode, &r.SubjectName, &r.JobName, &r.GroupName, &r.SubgroupName, &r.IsActive); err != nil {
			return err
		}
		items = append(items, r)
	}

	totalPages := (total + pageSize - 1) / pageSize
	return c.JSON(fiber.Map{
		"success":    true,
		"data":       items,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
		"totalPages": totalPages,
	})
}

// ─── Stats ───────────────────────────────────────────────────────────────────

// GetCostCodeStats godoc
// @Summary      Cost-code stats (total, active)
// @Tags         CostCode
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /master/cost-code/stats [get]
func (h *CostCodeHandler) GetCostCodeStats(c *fiber.Ctx) error {
	ctx := context.Background()

	var total, active int64
	if err := h.db.QueryRow(ctx, `SELECT COUNT(*) FROM cost_subgroup`).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to count cost codes: "+err.Error())
	}
	if err := h.db.QueryRow(ctx, `SELECT COUNT(*) FROM cost_subgroup WHERE is_active = true`).Scan(&active); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to count active cost codes: "+err.Error())
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"total":  total,
			"active": active,
		},
	})
}

// ─── Options (for PR line Cost Code dropdown) ───────────────────────────────

// CostCodeOption is a compact { id, code, name } shape for populating a
// searchable Cost Code Select when building a PR line item.
type CostCodeOption struct {
	ID           int64  `json:"id"`
	CostCode     string `json:"cost_code"`
	SubgroupName string `json:"subgroup_name"`
}

// GetCostCodeOptions godoc
// @Summary      Flat list of active cost codes for a PR-line Cost Code dropdown
// @Description  Returns id + combined code (subject+job+group+subgroup) + subgroup_name for every active cost_subgroup.
// @Tags         CostCode
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /master/cost-code/options [get]
func (h *CostCodeHandler) GetCostCodeOptions(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(), costCodeFullQuery+`
		WHERE sg.is_active = true
		ORDER BY cost_code`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var opts []CostCodeOption
	for rows.Next() {
		var f CostCodeFull
		if err := rows.Scan(&f.SubgroupID, &f.CostCode, &f.SubjectCode, &f.SubjectName,
			&f.JobCode, &f.JobName, &f.GroupCode, &f.GroupName,
			&f.SubgroupCode, &f.SubgroupName, &f.IsActive); err != nil {
			return err
		}
		opts = append(opts, CostCodeOption{ID: f.SubgroupID, CostCode: f.CostCode, SubgroupName: f.SubgroupName})
	}
	if opts == nil {
		opts = []CostCodeOption{}
	}
	return c.JSON(fiber.Map{"success": true, "data": opts})
}

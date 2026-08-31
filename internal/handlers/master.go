package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

type MasterHandler struct {
	db *pgxpool.Pool
}

func NewMasterHandler(db *pgxpool.Pool) *MasterHandler {
	return &MasterHandler{db: db}
}

// ─── Material Groups ──────────────────────────────────────────────────────────

// ListGroups godoc
// @Summary      List material groups
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  models.MatGroup
// @Router       /master/groups [get]
func (h *MasterHandler) ListGroups(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(), `SELECT id, group_code, group_name FROM mat_group ORDER BY group_code`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []models.MatGroup
	for rows.Next() {
		var m models.MatGroup
		rows.Scan(&m.Id, &m.GroupCode, &m.GroupName)
		items = append(items, m)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// UpdateMatGroup godoc
// @Summary      Update a material group's display name
// @Description  Updates mat_group.group_name only — group_code (the stable identifier used in mat_code composition) is never changed here. Affects every material_code row linked to this group.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int  true  "Group ID"
// @Param        body  body  object{group_name=string}  true  "New group_name"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/groups/{id} [put]
func (h *MasterHandler) UpdateMatGroup(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		GroupName string `json:"group_name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.GroupName = strings.TrimSpace(req.GroupName)
	if req.GroupName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "group_name is required")
	}

	var m models.MatGroup
	err = h.db.QueryRow(context.Background(),
		`UPDATE mat_group SET group_name = $1, updated_at = NOW() WHERE id = $2 RETURNING id, group_code, group_name`,
		req.GroupName, id,
	).Scan(&m.Id, &m.GroupCode, &m.GroupName)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("mat_group %d not found", id))
	}
	return c.JSON(fiber.Map{"success": true, "data": m})
}

// ─── Materials ────────────────────────────────────────────────────────────────

// ListMaterials godoc
// @Summary      List materials (full view)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        q         query  string  false  "search keyword (mat_name/mat_code)"
// @Param        search    query  string  false  "search keyword across mat_code, mat_name_th, spec_description"
// @Param        group     query  string  false  "group_code filter"
// @Param        active    query  bool    false  "active only"
// @Param        page      query  int     false  "page number"  default(1)
// @Param        page_size query  int     false  "page size"    default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /master/materials [get]
func (h *MasterHandler) ListMaterials(c *fiber.Ctx) error {
	q := c.Query("q")
	search := c.Query("search")
	group := c.Query("group")
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	where := []string{"1=1"}
	args := []interface{}{}
	idx := 1

	if q != "" {
		where = append(where, fmt.Sprintf("(mn.mat_name ILIKE $%d OR mc.mat_code ILIKE $%d)", idx, idx))
		args = append(args, "%"+q+"%")
		idx++
	}
	if search != "" {
		where = append(where, fmt.Sprintf("(mc.mat_code ILIKE $%d OR mn.mat_name ILIKE $%d OR ss.spec_description ILIKE $%d)", idx, idx, idx))
		args = append(args, "%"+search+"%")
		idx++
	}
	if group != "" {
		where = append(where, fmt.Sprintf("mg.group_code = $%d", idx))
		args = append(args, group)
		idx++
	}

	whereStr := strings.Join(where, " AND ")

	fromJoin := `FROM material_code mc
			JOIN mat_group mg ON mg.id = mc.group_id
			JOIN subgroup sg  ON sg.id = mc.subgroup_id
			JOIN mat_name mn  ON mn.id = mc.mat_name_id
			JOIN spec_size ss ON ss.id = mc.spec_id
			JOIN brand br     ON br.id = mc.brand_id
			JOIN unit u       ON u.id = mc.unit_id`

	var total int64
	h.db.QueryRow(context.Background(), fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, fromJoin, whereStr), args...).Scan(&total)

	args = append(args, size, offset)
	rows, err := h.db.Query(context.Background(),
		fmt.Sprintf(`SELECT mc.mat_code, mg.group_name, sg.subgroup_name, mn.mat_name, NULL::text,
			ss.spec_description, br.brand_name, u.unit_name, mc.is_active
			%s WHERE %s
			ORDER BY mc.mat_code LIMIT $%d OFFSET $%d`, fromJoin, whereStr, idx, idx+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.MaterialFull
	for rows.Next() {
		var m models.MaterialFull
		rows.Scan(&m.MatCode, &m.GroupName, &m.SubgroupName, &m.MatNameTH, &m.MatNameEN,
			&m.SpecDescription, &m.BrandName, &m.UnitName, &m.IsActive)
		items = append(items, m)
	}

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages,
		},
	})
}

// GetMaterialStats godoc
// @Summary      Material code stats (total, active, duplicate combinations)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /master/materials/stats [get]
func (h *MasterHandler) GetMaterialStats(c *fiber.Ctx) error {
	ctx := context.Background()

	var total, active, duplicates int64

	if err := h.db.QueryRow(ctx, `SELECT COUNT(*) FROM material_code WHERE is_active = true`).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to count materials: "+err.Error())
	}
	active = total

	if err := h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT group_id, subgroup_id, mat_name_id, spec_id, brand_id, unit_id
			FROM material_code
			WHERE is_active = true
			GROUP BY group_id, subgroup_id, mat_name_id, spec_id, brand_id, unit_id
			HAVING COUNT(*) > 1
		) sub`,
	).Scan(&duplicates); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to count duplicates: "+err.Error())
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"total":      total,
			"active":     active,
			"duplicates": duplicates,
		},
	})
}

// GetMaterial godoc
// @Summary      Get material detail by code (id+name for every FK, for the edit page)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "material code"
// @Success      200   {object}  models.MaterialDetailFull
// @Failure      404   {object}  fiber.Map
// @Router       /master/materials/{code} [get]
func (h *MasterHandler) GetMaterial(c *fiber.Ctx) error {
	code := c.Params("code")
	row := h.db.QueryRow(context.Background(), `
		SELECT mc.id, mc.mat_code, mc.is_active,
		       mg.id, mg.group_name,
		       sg.id, sg.subgroup_name,
		       mn.id, mn.mat_name,
		       ss.id, ss.spec_description,
		       br.id, br.brand_name,
		       u.id,  u.unit_name
		FROM material_code mc
		JOIN mat_group mg ON mg.id = mc.group_id
		JOIN subgroup  sg ON sg.id = mc.subgroup_id
		JOIN mat_name  mn ON mn.id = mc.mat_name_id
		JOIN spec_size ss ON ss.id = mc.spec_id
		JOIN brand     br ON br.id = mc.brand_id
		JOIN unit      u  ON u.id  = mc.unit_id
		WHERE mc.mat_code = $1`, code)

	var m models.MaterialDetailFull
	if err := row.Scan(&m.ID, &m.MatCode, &m.IsActive,
		&m.GroupID, &m.GroupName,
		&m.SubgroupID, &m.SubgroupName,
		&m.MatNameID, &m.MatName,
		&m.SpecID, &m.SpecDescription,
		&m.BrandID, &m.BrandName,
		&m.UnitID, &m.UnitName,
	); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "material not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": m})
}

// SearchMaterials godoc
// @Summary      Type-ahead material search
// @Description  Search active materials by mat_code or mat_name for the Create PO combobox. Returns mat_code, mat_name, unit, and last purchase price (nullable). Prefix matches on mat_code rank first. When warehouse_code is passed (e.g. Requisition item picker), results are additionally scoped to materials that have a stock_item row at that warehouse, and each result includes qty_on_hand from that warehouse.
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        q               query  string  true   "search term (matches mat_code or mat_name)"
// @Param        limit           query  int     false  "max results, default 20, cap 50"
// @Param        warehouse_code  query  string  false  "scope results to materials with stock at this warehouse; adds qty_on_hand to each result"
// @Success      200    {object}  fiber.Map
// @Failure      500    {object}  fiber.Map
// @Router       /materials/search [get]
func (h *MasterHandler) SearchMaterials(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return c.JSON(fiber.Map{"success": true, "data": []models.MaterialSearchItem{}})
	}

	limit := c.QueryInt("limit", 20)
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	warehouseCode := strings.TrimSpace(c.Query("warehouse_code"))

	baseQuery := `
    SELECT
        mc.mat_code,
        mn.mat_name,
        u.unit_name AS unit,
        lp.last_price,
        csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code AS cost_code
        %s
    FROM material_code mc
    JOIN mat_name mn ON mn.id = mc.mat_name_id
    JOIN unit u      ON u.id  = mc.unit_id
    LEFT JOIN LATERAL (
        SELECT pol.unit_price AS last_price
        FROM purchase_order_line pol
        JOIN purchase_order po ON po.id = pol.po_id
        WHERE pol.mat_code = mc.mat_code
          AND pol.status != 'CANCELLED'
          AND po.status IN ('APPROVED','SENT','PARTIALLY_RECEIVED','RECEIVED')
          AND pol.unit_price > 0
        ORDER BY po.po_date DESC
        LIMIT 1
    ) lp ON TRUE
    LEFT JOIN cost_subgroup csg ON csg.id = mc.cost_subgroup_id
    LEFT JOIN cost_group    cg  ON cg.id  = csg.group_id
    LEFT JOIN cost_job      cj  ON cj.id  = cg.job_id
    LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id
    %s
    WHERE mc.is_active = true
      AND (mc.mat_code ILIKE '%%' || $1 || '%%'
           OR mn.mat_name ILIKE '%%' || $1 || '%%')
      %s
    ORDER BY
        (mc.mat_code ILIKE $1 || '%%') DESC,
        mn.mat_name ASC
    LIMIT $2`

	var (
		rows pgx.Rows
		err  error
	)
	if warehouseCode == "" {
		query := fmt.Sprintf(baseQuery, "", "", "")
		rows, err = h.db.Query(context.Background(), query, q, limit)
	} else {
		// INNER JOIN stock_item -> stock_inventory scoped to warehouse_code — only materials
		// with actual stock AT THAT WAREHOUSE are returned, per requisition's hard requirement
		// that a requisition draws from exactly one warehouse. Deliberately NOT using
		// stock_item.qty (the system-wide rollup total) here: unlike the PO detail page, this
		// picker's whole purpose is to show/filter by one specific warehouse's real quantity,
		// so it needs stock_inventory's per-location qty_on_hand, not the cross-warehouse sum.
		query := fmt.Sprintf(baseQuery,
			", si.qty_on_hand AS qty_on_hand",
			"JOIN stock_item sitem ON sitem.mat_code = mc.mat_code JOIN stock_inventory si ON si.item_id = sitem.id AND si.warehouse_code = $3",
			"")
		rows, err = h.db.Query(context.Background(), query, q, limit, warehouseCode)
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "query error: "+err.Error())
	}
	defer rows.Close()

	items := make([]models.MaterialSearchItem, 0, limit)
	for rows.Next() {
		var m models.MaterialSearchItem
		if warehouseCode == "" {
			if err := rows.Scan(&m.MatCode, &m.MatName, &m.Unit, &m.LastPrice, &m.CostCode); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "scan error: "+err.Error())
			}
		} else {
			if err := rows.Scan(&m.MatCode, &m.MatName, &m.Unit, &m.LastPrice, &m.CostCode, &m.QtyOnHand); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "scan error: "+err.Error())
			}
		}
		items = append(items, m)
	}

	return c.JSON(fiber.Map{"success": true, "data": items})
}

// GetAllMaterial godoc
// @Summary      Get all active materials with detail (paginated)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        q            query  string  false  "Search keyword (mat_code, mat_name, spec_description, brand_name)"
// @Param        subgroup_id   query  int     false  "Filter by subgroup ID"
// @Param        mat_name_id   query  int     false  "Filter by mat_name ID"
// @Param        project_code  query  string  false  "When set, adds stock_on_hand per material from project_stock for that project (read-only reference, e.g. Petty Cash line picker)"
// @Param        page          query  int     false  "Page number (default 1)"
// @Success      200   {object}  models.PaginatedResponse
// @Failure      500   {object}  fiber.Map
// @Router       /master/allMaterial [get]
func (h *MasterHandler) GetAllMaterial(c *fiber.Ctx) error {
	q := c.Query("q")
	if q == "" {
		q = c.Query("search")
	}
	subgroupID := c.QueryInt("subgroup_id", 0)
	matNameID := c.QueryInt("mat_name_id", 0)
	projectCode := strings.TrimSpace(c.Query("project_code"))
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	const pageSize = 10
	offset := (page - 1) * pageSize

	joins := `
		FROM material_code mc
		JOIN mat_group  mg ON mg.id = mc.group_id
		JOIN subgroup   sg ON sg.id = mc.subgroup_id
		JOIN mat_name   mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size ss ON ss.id = mc.spec_id
		LEFT JOIN brand     b  ON b.id  = mc.brand_id
		JOIN unit           u  ON u.id  = mc.unit_id
		LEFT JOIN cost_subgroup csg ON csg.id = mc.cost_subgroup_id
		LEFT JOIN cost_group    cg  ON cg.id  = csg.group_id
		LEFT JOIN cost_job      cj  ON cj.id  = cg.job_id
		LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id`

	conditions := []string{"mc.is_active = true"}
	args := []interface{}{}
	idx := 1

	if q != "" {
		conditions = append(conditions, fmt.Sprintf(
			`(mc.mat_code ILIKE '%%'||$%[1]d||'%%' OR mn.mat_name ILIKE '%%'||$%[1]d||'%%' OR ss.spec_description ILIKE '%%'||$%[1]d||'%%' OR b.brand_name ILIKE '%%'||$%[1]d||'%%')`,
			idx))
		args = append(args, q)
		idx++
	}
	if subgroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("mc.subgroup_id = $%d", idx))
		args = append(args, subgroupID)
		idx++
	}
	if matNameID > 0 {
		conditions = append(conditions, fmt.Sprintf("mc.mat_name_id = $%d", idx))
		args = append(args, matNameID)
		idx++
	}

	// project_code is only used in the SELECT-time JOIN below (for stock_on_hand), not a
	// filter condition — a material with no project_stock row for this project is still a
	// valid pick, just with stock_on_hand=0 (LEFT JOIN, never INNER, per CLAUDE.md's
	// nullable-FK rule).
	projectArgIdx := 0
	if projectCode != "" {
		projectArgIdx = idx
		args = append(args, projectCode)
		idx++
		// project_stock has no unique constraint on (project_code, mat_code) in the live
		// schema — LATERAL + LIMIT 1 avoids fanning out material_code rows if duplicates
		// ever exist, unlike a plain LEFT JOIN.
		joins += fmt.Sprintf(` LEFT JOIN LATERAL (
			SELECT qty_on_hand FROM project_stock
			WHERE project_code = $%d AND mat_code = mc.mat_code
			ORDER BY updated_at DESC LIMIT 1
		) ps ON TRUE`, projectArgIdx)
	}

	whereStr := strings.Join(conditions, " AND ")

	var total int
	if err := h.db.QueryRow(context.Background(),
		`SELECT COUNT(*) `+joins+` WHERE `+whereStr, args...,
	).Scan(&total); err != nil {
		return err
	}

	stockSelect := "0"
	if projectCode != "" {
		stockSelect = "COALESCE(ps.qty_on_hand, 0)"
	}

	selectArgs := append(append([]interface{}{}, args...), pageSize, offset)
	rows, err := h.db.Query(context.Background(),
		fmt.Sprintf(`SELECT
			mg.group_code, sg.subgroup_code, mc.mat_code,
			mn.mat_name_code, mn.mat_name,
			ss.spec_description, ss.spec_code,
			b.brand_code, b.brand_name,
			u.unit_code, u.unit_name, mc.is_active,
			csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code AS cost_code,
			csg.subgroup_name AS cost_subgroup_name,
			mc.cost_subgroup_id,
			%s AS stock_on_hand
		%s WHERE %s
		ORDER BY mc.mat_code LIMIT $%d OFFSET $%d`,
			stockSelect, joins, whereStr, idx, idx+1),
		selectArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.MaterialDetail
	for rows.Next() {
		var m models.MaterialDetail
		rows.Scan(&m.GroupCode, &m.SubgroupCode, &m.MatCode, &m.MatNameCode, &m.MatNameTH,
			&m.SpecDescription, &m.SpecCode, &m.BrandCode, &m.BrandName,
			&m.UnitCode, &m.UnitName, &m.IsActive,
			&m.CostCode, &m.CostSubgroupName, &m.CostSubgroupID, &m.StockOnHand)
		items = append(items, m)
	}
	log.Println("checkdata", items)
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

// ListSubgroups godoc
// @Summary      List active subgroups (optionally filtered by group_id)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        group_id  query  int  false  "Group ID (filter)"
// @Success      200  {array}   fiber.Map
// @Router       /master/subgroups [get]
func (h *MasterHandler) ListSubgroups(c *fiber.Ctx) error {
	type subgroupItem struct {
		ID           int    `json:"id"`
		SubgroupCode string `json:"subgroup_code"`
		SubgroupName string `json:"subgroup_name"`
	}
	groupID := c.QueryInt("group_id", 0)
	var rows pgx.Rows
	var err error
	if groupID > 0 {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, subgroup_code, subgroup_name FROM subgroup WHERE is_active = true AND group_id = $1 ORDER BY subgroup_code`,
			groupID)
	} else {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, subgroup_code, subgroup_name FROM subgroup WHERE is_active = true ORDER BY subgroup_code`)
	}
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []subgroupItem
	for rows.Next() {
		var s subgroupItem
		rows.Scan(&s.ID, &s.SubgroupCode, &s.SubgroupName)
		items = append(items, s)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// UpdateSubgroup godoc
// @Summary      Update a subgroup's display name and/or reassign its parent group
// @Description  Updates subgroup.subgroup_name and, if provided, subgroup.group_id (reassigns the parent mat_group). subgroup_code is never changed here. Note: material_code rows carry their own independent group_id — reassigning a subgroup's parent does not retroactively change group_id on any material_code row that already uses this subgroup, so it can leave a subgroup pointing at a different group than some of its materials' own group_id. This mirrors the existing schema (no enforced consistency between the two), not a new limitation introduced here.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int  true  "Subgroup ID"
// @Param        body  body  object{subgroup_name=string,group_id=int}  true  "New subgroup_name (required) and optional group_id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/subgroups/{id} [put]
func (h *MasterHandler) UpdateSubgroup(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		SubgroupName string `json:"subgroup_name"`
		GroupID      *int64 `json:"group_id,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.SubgroupName = strings.TrimSpace(req.SubgroupName)
	if req.SubgroupName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "subgroup_name is required")
	}

	ctx := context.Background()
	if req.GroupID != nil {
		var exists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mat_group WHERE id = $1)`, *req.GroupID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("group_id %d not found", *req.GroupID))
		}
	}

	type subgroupItem struct {
		ID           int64  `json:"id"`
		SubgroupCode string `json:"subgroup_code"`
		SubgroupName string `json:"subgroup_name"`
		GroupID      int64  `json:"group_id"`
	}
	var s subgroupItem
	err = h.db.QueryRow(ctx, `
		UPDATE subgroup SET
		    subgroup_name = $1,
		    group_id      = COALESCE($2, group_id),
		    updated_at    = NOW()
		WHERE id = $3
		RETURNING id, subgroup_code, subgroup_name, group_id`,
		req.SubgroupName, req.GroupID, id,
	).Scan(&s.ID, &s.SubgroupCode, &s.SubgroupName, &s.GroupID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("subgroup %d not found", id))
	}
	return c.JSON(fiber.Map{"success": true, "data": s})
}

// ListMatNames godoc
// @Summary      List mat_names by subgroup_id
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        subgroup_id  query  int  true  "Subgroup ID"
// @Success      200  {array}   fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /master/mat-names [get]
func (h *MasterHandler) ListMatNames(c *fiber.Ctx) error {
	type matNameItem struct {
		ID          int    `json:"id"`
		MatNameCode string `json:"mat_name_code"`
		MatName     string `json:"mat_name"`
	}
	subgroupID := c.QueryInt("subgroup_id", 0)
	if subgroupID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "subgroup_id is required")
	}
	rows, err := h.db.Query(context.Background(),
		`SELECT id, mat_name_code, mat_name FROM mat_name WHERE subgroup_id = $1 AND is_active = true ORDER BY mat_name`,
		subgroupID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []matNameItem
	for rows.Next() {
		var m matNameItem
		rows.Scan(&m.ID, &m.MatNameCode, &m.MatName)
		items = append(items, m)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// UpdateMatName godoc
// @Summary      Update a mat_name's display name
// @Description  Updates mat_name.mat_name only — mat_name_code (the stable identifier used in mat_code composition) is never changed here. Affects every material_code row linked to this mat_name.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int  true  "Mat_name ID"
// @Param        body  body  object{mat_name=string}  true  "New mat_name"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/mat-names/{id} [put]
func (h *MasterHandler) UpdateMatName(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		MatName string `json:"mat_name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.MatName = strings.TrimSpace(req.MatName)
	if req.MatName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "mat_name is required")
	}

	type matNameItem struct {
		ID          int64  `json:"id"`
		MatNameCode string `json:"mat_name_code"`
		MatName     string `json:"mat_name"`
	}
	var m matNameItem
	err = h.db.QueryRow(context.Background(),
		`UPDATE mat_name SET mat_name = $1, updated_at = NOW() WHERE id = $2 RETURNING id, mat_name_code, mat_name`,
		req.MatName, id,
	).Scan(&m.ID, &m.MatNameCode, &m.MatName)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("mat_name %d not found", id))
	}
	return c.JSON(fiber.Map{"success": true, "data": m})
}

// ListSpecs godoc
// @Summary      List spec_size rows by mat_name_id
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        mat_name_id  query  int  true  "Material Name ID"
// @Success      200  {array}   fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /master/specs [get]
func (h *MasterHandler) ListSpecs(c *fiber.Ctx) error {
	type specItem struct {
		ID              int    `json:"id"`
		SpecCode        string `json:"spec_code"`
		MatNameID       int    `json:"mat_name_id"`
		SpecDescription string `json:"spec_description"`
		IsActive        bool   `json:"is_active"`
	}
	matNameID := c.QueryInt("mat_name_id", 0)
	if matNameID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "mat_name_id is required")
	}
	rows, err := h.db.Query(context.Background(),
		`SELECT id, spec_code, mat_name_id, spec_description, is_active FROM spec_size WHERE mat_name_id = $1 AND is_active = true ORDER BY spec_code`,
		matNameID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []specItem
	for rows.Next() {
		var s specItem
		rows.Scan(&s.ID, &s.SpecCode, &s.MatNameID, &s.SpecDescription, &s.IsActive)
		items = append(items, s)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// UpdateSpecSize godoc
// @Summary      Update a spec_size's display description
// @Description  Updates spec_size.spec_description only — spec_code (the stable identifier used in mat_code composition) is never changed here. Affects every material_code row linked to this spec.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int  true  "Spec ID"
// @Param        body  body  object{spec_description=string}  true  "New spec_description"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/specs/{id} [put]
func (h *MasterHandler) UpdateSpecSize(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		SpecDescription string `json:"spec_description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.SpecDescription = strings.TrimSpace(req.SpecDescription)
	if req.SpecDescription == "" {
		return fiber.NewError(fiber.StatusBadRequest, "spec_description is required")
	}

	type specItem struct {
		ID              int64  `json:"id"`
		SpecCode        string `json:"spec_code"`
		SpecDescription string `json:"spec_description"`
	}
	var s specItem
	err = h.db.QueryRow(context.Background(),
		`UPDATE spec_size SET spec_description = $1, updated_at = NOW() WHERE id = $2 RETURNING id, spec_code, spec_description`,
		req.SpecDescription, id,
	).Scan(&s.ID, &s.SpecCode, &s.SpecDescription)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("spec_size %d not found", id))
	}
	return c.JSON(fiber.Map{"success": true, "data": s})
}

// ListBrands godoc
// @Summary      List brand rows by spec_id
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        spec_id  query  int  true  "Spec ID"
// @Success      200  {array}   fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /master/brands [get]
func (h *MasterHandler) ListBrands(c *fiber.Ctx) error {
	type brandItem struct {
		ID        int    `json:"id"`
		BrandCode string `json:"brand_code"`
		SpecID    int    `json:"spec_id"`
		BrandName string `json:"brand_name"`
		IsActive  bool   `json:"is_active"`
	}
	specID := c.QueryInt("spec_id", 0)
	if specID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "spec_id is required")
	}
	rows, err := h.db.Query(context.Background(),
		`SELECT id, brand_code, spec_id, brand_name, is_active FROM brand WHERE spec_id = $1 AND is_active = true ORDER BY brand_code`,
		specID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []brandItem
	for rows.Next() {
		var b brandItem
		rows.Scan(&b.ID, &b.BrandCode, &b.SpecID, &b.BrandName, &b.IsActive)
		items = append(items, b)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// UpdateBrand godoc
// @Summary      Update a brand's display name
// @Description  Updates brand.brand_name only — brand_code (the stable identifier used in mat_code composition) is never changed here. Affects every material_code row linked to this brand.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int  true  "Brand ID"
// @Param        body  body  object{brand_name=string}  true  "New brand_name"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/brands/{id} [put]
func (h *MasterHandler) UpdateBrand(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		BrandName string `json:"brand_name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.BrandName = strings.TrimSpace(req.BrandName)
	if req.BrandName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "brand_name is required")
	}

	type brandDetail struct {
		ID        int64  `json:"id"`
		BrandCode string `json:"brand_code"`
		BrandName string `json:"brand_name"`
	}
	var b brandDetail
	err = h.db.QueryRow(context.Background(),
		`UPDATE brand SET brand_name = $1, updated_at = NOW() WHERE id = $2 RETURNING id, brand_code, brand_name`,
		req.BrandName, id,
	).Scan(&b.ID, &b.BrandCode, &b.BrandName)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("brand %d not found", id))
	}
	return c.JSON(fiber.Map{"success": true, "data": b})
}

// ─── Units ────────────────────────────────────────────────────────────────────

// ListUnits godoc
// @Summary      List units of measure
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  fiber.Map
// @Router       /master/units [get]
func (h *MasterHandler) ListUnits(c *fiber.Ctx) error {
	type unitItem struct {
		ID       int    `json:"id"`
		UnitCode string `json:"unit_code"`
		UnitName string `json:"unit_name"`
	}
	rows, _ := h.db.Query(context.Background(), `SELECT id, unit_code, unit_name FROM unit ORDER BY unit_code`)
	defer rows.Close()
	var items []unitItem
	for rows.Next() {
		var u unitItem
		rows.Scan(&u.ID, &u.UnitCode, &u.UnitName)
		items = append(items, u)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// UpdateUnit godoc
// @Summary      Update a unit's display name
// @Description  Updates unit.unit_name only — unit_code (the stable identifier used in mat_code composition) is never changed here. Affects every material_code row linked to this unit.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int  true  "Unit ID"
// @Param        body  body  object{unit_name=string}  true  "New unit_name"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /master/units/{id} [put]
func (h *MasterHandler) UpdateUnit(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		UnitName string `json:"unit_name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req.UnitName = strings.TrimSpace(req.UnitName)
	if req.UnitName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "unit_name is required")
	}

	type unitDetail struct {
		ID       int64  `json:"id"`
		UnitCode string `json:"unit_code"`
		UnitName string `json:"unit_name"`
	}
	var u unitDetail
	err = h.db.QueryRow(context.Background(),
		`UPDATE unit SET unit_name = $1, updated_at = NOW() WHERE id = $2 RETURNING id, unit_code, unit_name`,
		req.UnitName, id,
	).Scan(&u.ID, &u.UnitCode, &u.UnitName)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("unit %d not found", id))
	}
	return c.JSON(fiber.Map{"success": true, "data": u})
}

// ─── Roles ───────────────────────────────────────────────────────────────────

// ListRoles godoc
// @Summary      List roles (optionally filtered by department)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        dept_code  query  string  false  "filter roles by department"
// @Success      200  {object}  fiber.Map
// @Router       /master/roles [get]
func (h *MasterHandler) ListRoles(c *fiber.Ctx) error {
	deptCode := c.Query("dept_code")

	var rows pgx.Rows
	var err error
	if deptCode != "" {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, role_code, role_name, dept_code FROM roles WHERE is_active = true AND dept_code = $1 ORDER BY role_name`,
			deptCode)
	} else {
		rows, err = h.db.Query(context.Background(),
			`SELECT id, role_code, role_name, dept_code FROM roles WHERE is_active = true ORDER BY role_name`)
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	type roleItem struct {
		ID       int64   `json:"role_id"`
		RoleCode string  `json:"role_code"`
		RoleName string  `json:"role_name"`
		DeptCode *string `json:"dept_code"`
	}
	items := []roleItem{}
	for rows.Next() {
		var it roleItem
		if err := rows.Scan(&it.ID, &it.RoleCode, &it.RoleName, &it.DeptCode); err != nil {
			return err
		}
		items = append(items, it)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// ─── Eligible Approvers ────────────────────────────────────────────────────────

// ListEligibleApprovers godoc
// @Summary      List users eligible to approve a given doc_type
// @Description  Union of role-based approvers (approval_config.approver_role_id, joined via user_roles) and extra approvers (approval_delegation, doc_type match or NULL = applies to all), deduplicated. Used to populate approver-selection dropdowns (e.g. Memo's "ผู้อนุมัติ" field) so only users actually eligible to approve that doc_type are offered.
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        doc_type  query  string  true  "Document type, e.g. MEMO"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /master/eligible-approvers [get]
func (h *MasterHandler) ListEligibleApprovers(c *fiber.Ctx) error {
	docType := c.Query("doc_type")
	if docType == "" {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type is required")
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT DISTINCT u.id, u.full_name, u.dept_code, d.dept_name
		FROM users u
		LEFT JOIN departments d ON d.dept_code = u.dept_code
		WHERE u.is_active = true
		  AND (
			u.id IN (
				SELECT ur.user_id
				FROM approval_config ac
				JOIN user_roles ur ON ur.role_id = ac.approver_role_id
				JOIN roles r ON r.id = ac.approver_role_id AND r.is_active = true
				WHERE ac.doc_type = $1 AND ac.is_active = true
			)
			OR u.id IN (
				SELECT ad.user_id FROM approval_delegation ad
				WHERE ad.doc_type = $1 OR ad.doc_type IS NULL
			)
		  )
		ORDER BY u.full_name`, docType)
	if err != nil {
		return err
	}
	defer rows.Close()

	type approverItem struct {
		Value int64   `json:"value"`
		Label string  `json:"label"`
		Dept  *string `json:"dept"`
	}
	items := []approverItem{}
	for rows.Next() {
		var it approverItem
		var deptCode, deptName *string
		if err := rows.Scan(&it.Value, &it.Label, &deptCode, &deptName); err != nil {
			return err
		}
		if deptName != nil {
			it.Dept = deptName
		} else {
			it.Dept = deptCode
		}
		items = append(items, it)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// ─── Location ────────────────────────────────────────────────────────────────

// ListLocations godoc
// @Summary      List locations (departments/projects)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        type  query  string  false  "location_type filter"
// @Success      200   {array}  models.Location
// @Router       /master/locations [get]
func (h *MasterHandler) ListLocations(c *fiber.Ctx) error {
	locType := c.Query("type")
	var rows interface{ Close() }
	var err error
	if locType != "" {
		r, e := h.db.Query(context.Background(),
			`SELECT location_code, location_name, location_type, parent_code, is_active, created_at
			 FROM location WHERE location_type=$1 AND is_active=true ORDER BY location_name`, locType)
		rows, err = r, e
	} else {
		r, e := h.db.Query(context.Background(),
			`SELECT location_code, location_name, location_type, parent_code, is_active, created_at
			 FROM location WHERE is_active=true ORDER BY location_name`)
		rows, err = r, e
	}
	if err != nil {
		return err
	}
	_ = rows
	return c.JSON(fiber.Map{"success": true, "data": []models.Location{}})
}

// CreateLocation godoc
// @Summary      Create location
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateLocationRequest  true  "Location data"
// @Success      201   {object}  models.Location
// @Failure      400   {object}  fiber.Map
// @Router       /master/locations [post]
func (h *MasterHandler) CreateLocation(c *fiber.Ctx) error {
	var req models.CreateLocationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	_, err := h.db.Exec(context.Background(), `
		INSERT INTO location (location_code, location_name, location_type, parent_code)
		VALUES ($1,$2,$3,$4)`,
		req.LocationCode, req.LocationName, req.LocationType, req.ParentCode)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, "location_code already exists or constraint error")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "message": "location created"})
}

// ─── Warehouse ───────────────────────────────────────────────────────────────

// ListWarehouses godoc
// @Summary      List warehouses (dropdown/autocomplete)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        search     query  string  false  "Search by warehouse_name/warehouse_code"
// @Param        is_active  query  string  false  "Filter by active flag (default true)"
// @Success      200  {object}  fiber.Map
// @Router       /master/warehouses [get]
func (h *MasterHandler) ListWarehouses(c *fiber.Ctx) error {
	search := c.Query("search")
	isActive := c.Query("is_active", "true")

	rows, err := h.db.Query(context.Background(), `
		SELECT warehouse_code, warehouse_name, address, is_active, created_at
		FROM warehouse
		WHERE ($1 = '' OR is_active = $1::bool)
		  AND ($2 = '' OR warehouse_name ILIKE '%'||$2||'%' OR warehouse_code ILIKE '%'||$2||'%')
		ORDER BY warehouse_name`, isActive, search)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []models.Warehouse
	for rows.Next() {
		var w models.Warehouse
		rows.Scan(&w.WarehouseCode, &w.WarehouseName, &w.Address, &w.IsActive, &w.CreatedAt)
		items = append(items, w)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// CreateWarehouse godoc
// @Summary      Create warehouse
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateWarehouseRequest  true  "Warehouse"
// @Success      201   {object}  fiber.Map
// @Router       /master/warehouses [post]
func (h *MasterHandler) CreateWarehouse(c *fiber.Ctx) error {
	var req models.CreateWarehouseRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	_, err := h.db.Exec(context.Background(),
		`INSERT INTO warehouse (warehouse_code, warehouse_name, address) VALUES ($1,$2,$3)`,
		req.WarehouseCode, req.WarehouseName, req.Address)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, "warehouse code already exists")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "message": "warehouse created"})
}

// ListZones godoc
// @Summary      List storage zones for a warehouse
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "warehouse_code"
// @Success      200   {array}  models.StorageZone
// @Router       /master/warehouses/{code}/zones [get]
func (h *MasterHandler) ListZones(c *fiber.Ctx) error {
	code := c.Params("code")
	rows, err := h.db.Query(context.Background(),
		`SELECT zone_id, warehouse_code, zone_code, zone_name, zone_type FROM storage_zone WHERE warehouse_code=$1 ORDER BY zone_code`, code)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []models.StorageZone
	for rows.Next() {
		var z models.StorageZone
		rows.Scan(&z.ZoneID, &z.WarehouseCode, &z.ZoneCode, &z.ZoneName, &z.ZoneType)
		items = append(items, z)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// ─── Supplier ────────────────────────────────────────────────────────────────

// ListSuppliers godoc
// @Summary      List suppliers
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        q  query  string  false  "search"
// @Success      200  {array}  models.Supplier
// @Router       /master/suppliers [get]
func (h *MasterHandler) ListSuppliers(c *fiber.Ctx) error {
	q := "%" + c.Query("q") + "%"
	rows, err := h.db.Query(context.Background(), `
		SELECT supplier_name, tax_id, address, contact_name, contact_phone, contact_email, payment_terms, is_active, created_at
		FROM supplier
		WHERE is_active=true AND supplier_name ILIKE $1
		ORDER BY supplier_name`, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []models.Supplier
	for rows.Next() {
		var s models.Supplier
		rows.Scan(&s.SupplierName, &s.TaxID, &s.Address,
			&s.ContactName, &s.ContactPhone, &s.ContactEmail, &s.PaymentTerms, &s.IsActive, &s.CreatedAt)
		items = append(items, s)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// CreateSupplier godoc
// @Summary      Create supplier
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateSupplierRequest  true  "Supplier data"
// @Success      201   {object}  fiber.Map
// @Router       /master/suppliers [post]
func (h *MasterHandler) CreateSupplier(c *fiber.Ctx) error {
	var req models.CreateSupplierRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	_, err := h.db.Exec(context.Background(), `
		INSERT INTO supplier (supplier_name, tax_id, address, contact_name, contact_phone, contact_email, payment_terms)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		req.SupplierName, req.TaxID, req.Address,
		req.ContactName, req.ContactPhone, req.ContactEmail, req.PaymentTerms)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "message": "supplier created"})
}

// CreateMaterial godoc
// @Summary      Create a single material (full transaction)
// @Description  INSERT order (FK parent-first): unit → group → subgroup → mat_name → spec_size → brand → material_code
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateMaterialRequest  true  "Material data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /master/materials [post]
func (h *MasterHandler) CreateMaterial(c *fiber.Ctx) error {
	var req models.CreateMaterialRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.GroupCode == "" || req.SubgroupCode == "" || req.SubgroupName == "" ||
		req.MatNameCode == "" || req.MatNameTH == "" ||
		req.SpecCode == "" || req.SpecDescription == "" ||
		req.BrandCode == "" || req.BrandName == "" ||
		req.UnitCode == "" || req.UnitName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "all fields are required")
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var unitID int
	if err = tx.QueryRow(ctx,
		`INSERT INTO unit (unit_code, unit_name) VALUES ($1, $2)
		 ON CONFLICT (unit_code) DO UPDATE SET unit_name = EXCLUDED.unit_name
		 RETURNING id`,
		req.UnitCode, req.UnitName,
	).Scan(&unitID); err != nil {
		return err
	}

	var groupID int
	if err = tx.QueryRow(ctx,
		`SELECT id FROM mat_group WHERE group_code = $1`, req.GroupCode,
	).Scan(&groupID); err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "group_code not found: "+req.GroupCode)
	}

	var subgroupID int
	if err = tx.QueryRow(ctx,
		`INSERT INTO subgroup (subgroup_code, group_id, subgroup_name) VALUES ($1, $2, $3)
		 ON CONFLICT (subgroup_code) DO UPDATE SET subgroup_name = EXCLUDED.subgroup_name
		 RETURNING id`,
		req.SubgroupCode, groupID, req.SubgroupName,
	).Scan(&subgroupID); err != nil {
		return err
	}

	var matNameID int
	if err = tx.QueryRow(ctx,
		`INSERT INTO mat_name (mat_name_code, subgroup_id, mat_name) VALUES ($1, $2, $3)
		 ON CONFLICT (subgroup_id, mat_name_code) DO UPDATE SET mat_name = EXCLUDED.mat_name
		 RETURNING id`,
		req.MatNameCode, subgroupID, req.MatNameTH,
	).Scan(&matNameID); err != nil {
		return err
	}

	var specID int
	if err = tx.QueryRow(ctx,
		`INSERT INTO spec_size (spec_code, mat_name_id, spec_description, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, true, NOW(), NOW())
		 ON CONFLICT (spec_code, mat_name_id) DO UPDATE SET spec_description = EXCLUDED.spec_description
		 RETURNING id`,
		req.SpecCode, matNameID, req.SpecDescription,
	).Scan(&specID); err != nil {
		return err
	}

	var brandID int
	if err = tx.QueryRow(ctx,
		`INSERT INTO brand (brand_code, brand_name, spec_id, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, true, NOW(), NOW())
		 ON CONFLICT (brand_code, spec_id) DO UPDATE SET brand_name = EXCLUDED.brand_name
		 RETURNING id`,
		req.BrandCode, req.BrandName, specID,
	).Scan(&brandID); err != nil {
		return err
	}

	matCode := req.GroupCode + req.SubgroupCode + req.MatNameCode + req.SpecCode + req.BrandCode + req.UnitCode

	var dupCount int
	if err = tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM material_code WHERE mat_code = $1`, matCode,
	).Scan(&dupCount); err != nil {
		return err
	}
	if dupCount > 0 {
		return fiber.NewError(fiber.StatusConflict, "material already exists: "+matCode)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO material_code
		   (mat_code, group_id, subgroup_id, mat_name_id, spec_id, brand_id, unit_id, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, true, NOW(), NOW())`,
		matCode, groupID, subgroupID, matNameID, specID, brandID, unitID,
	); err != nil {
		return err
	}

	// Every material must have a matching stock_item so GRN receiving can
	// always find one — see CLAUDE.md "GRN receiving / stock link" note.
	var stockItemExists bool
	if err = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM stock_item WHERE mat_code = $1)`, matCode,
	).Scan(&stockItemExists); err != nil {
		return err
	}
	if !stockItemExists {
		if _, err = tx.Exec(ctx,
			`INSERT INTO stock_item (mat_code, item_name, unit, qty, created_at, updated_at)
			 VALUES ($1, $2, $3, 0, NOW(), NOW())`,
			matCode, req.MatNameTH, req.UnitName,
		); err != nil {
			return err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success":  true,
		"mat_code": matCode,
		"message":  "material created",
	})
}

// BulkCreateMaterial godoc
// @Summary      Bulk create materials (cache + UNNEST optimized)
// @Description  Accepts an array of materials. Uses in-memory cache to skip duplicate FK lookups, then inserts all material_code rows in a single UNNEST statement.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      []models.CreateMaterialRequest  true  "Array of material data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /master/materials/bulk [post]
func (h *MasterHandler) BulkCreateMaterial(c *fiber.Ctx) error {
	var reqs []models.CreateMaterialRequest
	if err := c.BodyParser(&reqs); err != nil {
		log.Println("❌ BodyParser:", err)
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(reqs) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "request body is empty")
	}

	for i, req := range reqs {
		if req.GroupCode == "" || req.SubgroupCode == "" || req.SubgroupName == "" ||
			req.MatNameCode == "" || req.MatNameTH == "" ||
			req.SpecCode == "" || req.SpecDescription == "" ||
			req.BrandCode == "" || req.UnitCode == "" {
			// ลบ req.BrandName == "" ออก
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("item[%d]: all fields are required", i))
		}
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		log.Println("❌ Begin tx:", err)
		return err
	}
	defer tx.Rollback(ctx)

	unitCache := map[string]int{}
	groupCache := map[string]int{}
	subgroupCache := map[string]int{}
	matNameCache := map[string]int{}
	specCache := map[string]int{}
	brandCache := map[string]int{}

	type matRow struct {
		matCode     string
		groupID     int
		subgroupID  int
		matNameID   int
		specID      int
		brandID     int
		unitID      int
		groupCode   string
		subCode     string
		matNameCode string
		specCode    string
		brandCode   string
		unitCode    string
		rowIndex    int
	}
	rows := make([]matRow, 0, len(reqs))

	for i, req := range reqs {
		// unit
		unitID, ok := unitCache[req.UnitCode]
		if !ok {
			if err = tx.QueryRow(ctx,
				`INSERT INTO unit (unit_code, unit_name) VALUES ($1, $2)
				 ON CONFLICT (unit_code) DO UPDATE SET unit_name = EXCLUDED.unit_name
				 RETURNING id`,
				req.UnitCode, req.UnitName,
			).Scan(&unitID); err != nil {
				log.Printf("❌ unit[%d]: %v", i, err)
				return err
			}
			unitCache[req.UnitCode] = unitID
		}

		// group
		groupID, ok := groupCache[req.GroupCode]
		if !ok {
			if err = tx.QueryRow(ctx,
				`SELECT id FROM mat_group WHERE group_code = $1`, req.GroupCode,
			).Scan(&groupID); err != nil {
				log.Printf("❌ group[%d]: %v", i, err)
				return fiber.NewError(fiber.StatusUnprocessableEntity, "group_code not found: "+req.GroupCode)
			}
			groupCache[req.GroupCode] = groupID
		}

		// subgroup
		subgroupID, ok := subgroupCache[req.SubgroupCode]
		if !ok {
			if err = tx.QueryRow(ctx,
				`INSERT INTO subgroup (subgroup_code, group_id, subgroup_name) VALUES ($1, $2, $3)
				 ON CONFLICT (subgroup_code) DO UPDATE SET subgroup_name = EXCLUDED.subgroup_name
				 RETURNING id`,
				req.SubgroupCode, groupID, req.SubgroupName,
			).Scan(&subgroupID); err != nil {
				log.Printf("❌ subgroup[%d]: %v", i, err)
				return err
			}
			subgroupCache[req.SubgroupCode] = subgroupID
		}

		// mat_name
		matNameKey := req.SubgroupCode + ":" + req.MatNameCode
		matNameID, ok := matNameCache[matNameKey]
		if !ok {
			if err = tx.QueryRow(ctx,
				`INSERT INTO mat_name (mat_name_code, subgroup_id, mat_name) VALUES ($1, $2, $3)
				 ON CONFLICT (subgroup_id, mat_name_code) DO UPDATE SET mat_name = EXCLUDED.mat_name
				 RETURNING id`,
				req.MatNameCode, subgroupID, req.MatNameTH,
			).Scan(&matNameID); err != nil {
				log.Printf("❌ mat_name[%d]: %v", i, err)
				return err
			}
			matNameCache[matNameKey] = matNameID
		}

		// spec_size
		specKey := fmt.Sprintf("%d:%s", matNameID, req.SpecCode)
		specID, ok := specCache[specKey]
		if !ok {
			if err = tx.QueryRow(ctx,
				`INSERT INTO spec_size (spec_code, mat_name_id, spec_description, is_active, created_at, updated_at)
				 VALUES ($1, $2, $3, true, NOW(), NOW())
				 ON CONFLICT (spec_code, mat_name_id) DO UPDATE SET spec_description = EXCLUDED.spec_description
				 RETURNING id`,
				req.SpecCode, matNameID, req.SpecDescription,
			).Scan(&specID); err != nil {
				log.Printf("❌ spec_size[%d]: %v", i, err)
				return err
			}
			specCache[specKey] = specID
		}

		// brand
		brandKey := fmt.Sprintf("%d:%s", specID, req.BrandCode)
		brandID, ok := brandCache[brandKey]
		if !ok {
			if err = tx.QueryRow(ctx,
				`INSERT INTO brand (brand_code, brand_name, spec_id, is_active, created_at, updated_at)
				 VALUES ($1, $2, $3, true, NOW(), NOW())
				 ON CONFLICT (brand_code, spec_id) DO UPDATE SET brand_name = EXCLUDED.brand_name
				 RETURNING id`,
				req.BrandCode, req.BrandName, specID,
			).Scan(&brandID); err != nil {
				log.Printf("❌ brand[%d]: %v", i, err)
				return err
			}
			brandCache[brandKey] = brandID
		}

		matCode := req.GroupCode + req.SubgroupCode + req.MatNameCode + req.SpecCode + req.BrandCode + req.UnitCode

		// ✅ log ตรงนี้เพื่อดูว่า mat_code ไหนซ้ำ
		log.Printf("row[%d] mat_code=%s spec=%s brand=%s unit=%s",
			i, matCode, req.SpecCode, req.BrandCode, req.UnitCode)

		rows = append(rows, matRow{matCode, groupID, subgroupID, matNameID, specID, brandID, unitID,
			req.GroupCode, req.SubgroupCode, req.MatNameCode, req.SpecCode, req.BrandCode, req.UnitCode, i})
	}

	log.Printf("✅ total rows before dedup: %d", len(rows))

	// ── Build slices + dedup ──────────────────────────────────────────────
	seen := map[string]bool{}
	matCodes := make([]string, 0, len(rows))
	groupIDs := make([]int, 0, len(rows))
	subgroupIDs := make([]int, 0, len(rows))
	matNameIDs := make([]int, 0, len(rows))
	specIDs := make([]int, 0, len(rows))
	brandIDs := make([]int, 0, len(rows))
	unitIDs := make([]int, 0, len(rows))

	for _, r := range rows {
		if seen[r.matCode] {
			// ✅ log mat_code ที่ซ้ำ
			log.Printf("⚠️ duplicate skipped: Excel row %d | mat_code=%s | group=%s sub=%s matCode=%s spec=%s brand=%s unit=%s",
				r.rowIndex+2, r.matCode, r.groupCode, r.subCode, r.matNameCode, r.specCode, r.brandCode, r.unitCode)
			continue
		}
		seen[r.matCode] = true
		matCodes = append(matCodes, r.matCode)
		groupIDs = append(groupIDs, r.groupID)
		subgroupIDs = append(subgroupIDs, r.subgroupID)
		matNameIDs = append(matNameIDs, r.matNameID)
		specIDs = append(specIDs, r.specID)
		brandIDs = append(brandIDs, r.brandID)
		unitIDs = append(unitIDs, r.unitID)
	}

	log.Printf("✅ total rows after dedup: %d", len(matCodes))

	// ── Bulk upsert ───────────────────────────────────────────────────────
	if _, err = tx.Exec(ctx, `
		INSERT INTO material_code
		  (mat_code, group_id, subgroup_id, mat_name_id, spec_id, brand_id, unit_id, is_active, created_at, updated_at)
		SELECT UNNEST($1::text[]),
		       UNNEST($2::bigint[]), UNNEST($3::bigint[]), UNNEST($4::bigint[]),
		       UNNEST($5::bigint[]), UNNEST($6::bigint[]), UNNEST($7::bigint[]),
		       true, NOW(), NOW()
		ON CONFLICT (mat_code) DO UPDATE SET
		    group_id    = EXCLUDED.group_id,
		    subgroup_id = EXCLUDED.subgroup_id,
		    mat_name_id = EXCLUDED.mat_name_id,
		    spec_id     = EXCLUDED.spec_id,
		    brand_id    = EXCLUDED.brand_id,
		    unit_id     = EXCLUDED.unit_id,
		    updated_at  = NOW()`,
		matCodes, groupIDs, subgroupIDs, matNameIDs, specIDs, brandIDs, unitIDs,
	); err != nil {
		log.Println("❌ bulk upsert:", err)
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		log.Println("❌ commit:", err)
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success":   true,
		"mat_codes": matCodes,
		"count":     len(matCodes),
		"message":   "materials created or updated",
	})
}

// UpdateMaterial godoc
// materialCodeCascadeTables lists every table with a (non-deferrable-by-default, now made
// DEFERRABLE INITIALLY IMMEDIATE — see the one-time ALTER TABLE run alongside this handler)
// FK to material_code(mat_code), plus stock_item which has no FK at all. All are cascaded
// to the new mat_code, inside the same transaction as the material_code update itself, using
// SET CONSTRAINTS ... DEFERRED so the intermediate (old-code-missing) state within the
// transaction doesn't trip the FK checks before COMMIT.
var materialCodeCascadeTables = []string{
	"stock_item", "borrow_line", "grn_line", "inventory", "inventory_transaction",
	"purchase_order_line", "purchase_request_line", "rfq_line", "stock_count_line",
}

// materialCodeCascadeConstraints are the FK constraint names deferred for the cascade —
// only material_code's own dependents, not every constraint in the database.
var materialCodeCascadeConstraints = []string{
	"borrow_line_mat_code_fkey", "grn_line_mat_code_fkey", "inventory_mat_code_fkey",
	"inventory_transaction_mat_code_fkey", "purchase_order_line_mat_code_fkey",
	"purchase_request_line_mat_code_fkey", "rfq_line_mat_code_fkey", "stock_count_line_mat_code_fkey",
}

// UpdateMaterial godoc
// @Summary      Update a material's group/subgroup/mat_name/spec/brand/unit/is_active
// @Description  Partial update — only fields present in the body are changed. Reassigns which
// @Description  existing group/subgroup/mat_name/spec/brand/unit row this material points to
// @Description  (by id); it does not rename shared master rows, and mat_code is never accepted
// @Description  from the client. If any of group_id/subgroup_id/mat_name_id/spec_id/brand_id/
// @Description  unit_id changes, mat_code is auto-regenerated server-side as
// @Description  group_code+subgroup_code+mat_name_code+spec_code+brand_code+unit_code (the
// @Description  confirmed composition rule), and the change is cascaded in the same
// @Description  transaction to every table that references mat_code — including stock_item,
// @Description  which has no FK and would otherwise silently lose its link to this material.
// @Description  If the regenerated code would collide with a different existing material, the
// @Description  update is rejected with 409.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        code  path      string                          true  "mat_code"
// @Param        body  body      models.UpdateMaterialFullRequest true  "Update data"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Failure      409   {object}  fiber.Map
// @Router       /master/materials/{code} [put]
func (h *MasterHandler) UpdateMaterial(c *fiber.Ctx) error {
	matCode := c.Params("code")
	var req models.UpdateMaterialFullRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var current struct {
		id                                                      int64
		groupID, subgroupID, matNameID, specID, brandID, unitID int64
	}
	if err := h.db.QueryRow(ctx,
		`SELECT id, group_id, subgroup_id, mat_name_id, spec_id, brand_id, unit_id
		 FROM material_code WHERE mat_code = $1`, matCode,
	).Scan(&current.id, &current.groupID, &current.subgroupID, &current.matNameID,
		&current.specID, &current.brandID, &current.unitID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "material not found: "+matCode)
	}

	// Validate every provided FK id actually exists before touching anything.
	fkChecks := []struct {
		id    *int64
		table string
		name  string
	}{
		{req.GroupID, "mat_group", "group_id"},
		{req.SubgroupID, "subgroup", "subgroup_id"},
		{req.MatNameID, "mat_name", "mat_name_id"},
		{req.SpecID, "spec_size", "spec_id"},
		{req.BrandID, "brand", "brand_id"},
		{req.UnitID, "unit", "unit_id"},
	}
	for _, fk := range fkChecks {
		if fk.id == nil {
			continue
		}
		var exists bool
		if err := h.db.QueryRow(ctx,
			fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE id = $1)`, fk.table), *fk.id,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s %d not found", fk.name, *fk.id))
		}
	}

	effGroupID := current.groupID
	if req.GroupID != nil {
		effGroupID = *req.GroupID
	}
	effSubgroupID := current.subgroupID
	if req.SubgroupID != nil {
		effSubgroupID = *req.SubgroupID
	}
	effMatNameID := current.matNameID
	if req.MatNameID != nil {
		effMatNameID = *req.MatNameID
	}
	effSpecID := current.specID
	if req.SpecID != nil {
		effSpecID = *req.SpecID
	}
	effBrandID := current.brandID
	if req.BrandID != nil {
		effBrandID = *req.BrandID
	}
	effUnitID := current.unitID
	if req.UnitID != nil {
		effUnitID = *req.UnitID
	}

	codeChanged := effGroupID != current.groupID || effSubgroupID != current.subgroupID ||
		effMatNameID != current.matNameID || effSpecID != current.specID ||
		effBrandID != current.brandID || effUnitID != current.unitID

	newMatCode := matCode
	if codeChanged {
		var groupCode, subgroupCode, matNameCode, specCode, brandCode, unitCode string
		if err := h.db.QueryRow(ctx, `
			SELECT mg.group_code, sg.subgroup_code, mn.mat_name_code, ss.spec_code, br.brand_code, u.unit_code
			FROM mat_group mg, subgroup sg, mat_name mn, spec_size ss, brand br, unit u
			WHERE mg.id = $1 AND sg.id = $2 AND mn.id = $3 AND ss.id = $4 AND br.id = $5 AND u.id = $6`,
			effGroupID, effSubgroupID, effMatNameID, effSpecID, effBrandID, effUnitID,
		).Scan(&groupCode, &subgroupCode, &matNameCode, &specCode, &brandCode, &unitCode); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "one or more of the resolved group/subgroup/mat_name/spec/brand/unit ids do not exist")
		}
		newMatCode = groupCode + subgroupCode + matNameCode + specCode + brandCode + unitCode

		// Guard: reject if the regenerated code collides with a DIFFERENT existing material.
		var collidingID int64
		err := h.db.QueryRow(ctx, `SELECT id FROM material_code WHERE mat_code = $1 AND id != $2`, newMatCode, current.id).Scan(&collidingID)
		if err == nil {
			return fiber.NewError(fiber.StatusConflict, fmt.Sprintf(
				"the regenerated mat_code %q already belongs to another material (id=%d) — resulting combination is not unique", newMatCode, collidingID))
		}
	}

	var updatedBy *int64
	if claims != nil {
		updatedBy = &claims.UserID
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	cascaded := fiber.Map{}
	if codeChanged && newMatCode != matCode {
		if _, err := tx.Exec(ctx,
			fmt.Sprintf(`SET CONSTRAINTS %s DEFERRED`, strings.Join(materialCodeCascadeConstraints, ", ")),
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to defer FK constraints: "+err.Error())
		}
		for _, table := range materialCodeCascadeTables {
			tag, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET mat_code = $1 WHERE mat_code = $2`, table), newMatCode, matCode)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to cascade mat_code to %s: %s", table, err.Error()))
			}
			cascaded[table+"_rows_updated"] = tag.RowsAffected()
		}
		log.Printf("material_code id=%d mat_code changed %q -> %q by user %v; cascaded to %d table(s)",
			current.id, matCode, newMatCode, updatedBy, len(materialCodeCascadeTables))
	}

	var returnedMatCode string
	if err := tx.QueryRow(ctx, `
		UPDATE material_code SET
		    mat_code    = $1,
		    group_id    = COALESCE($2, group_id),
		    subgroup_id = COALESCE($3, subgroup_id),
		    mat_name_id = COALESCE($4, mat_name_id),
		    spec_id     = COALESCE($5, spec_id),
		    brand_id    = COALESCE($6, brand_id),
		    unit_id     = COALESCE($7, unit_id),
		    is_active   = COALESCE($8, is_active),
		    updated_at  = NOW(), updated_by = $9
		WHERE id = $10
		RETURNING mat_code`,
		newMatCode, req.GroupID, req.SubgroupID, req.MatNameID, req.SpecID, req.BrandID, req.UnitID, req.IsActive,
		updatedBy, current.id,
	).Scan(&returnedMatCode); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
			return fiber.NewError(fiber.StatusBadRequest, "invalid group_id, subgroup_id, mat_name_id, spec_id, brand_id or unit_id")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update material: "+err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
			return fiber.NewError(fiber.StatusInternalServerError, "cascade left an inconsistent reference — commit rejected: "+pgErr.Message)
		}
		return fiber.NewError(fiber.StatusInternalServerError, "failed to commit: "+err.Error())
	}

	resp := fiber.Map{
		"success": true,
		"data": fiber.Map{
			"mat_code": returnedMatCode,
		},
		"message": "material updated",
	}
	if len(cascaded) > 0 {
		resp["cascaded"] = cascaded
	}
	return c.JSON(resp)
}

// ExportMaterials godoc
// @Summary      Export all active materials as Excel
// @Description  Downloads an xlsx file with all active materials joined across group/subgroup/mat_name/spec/brand/unit
// @Tags         Master
// @Security     BearerAuth
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200  {file}    binary
// @Failure      500  {object}  fiber.Map
// @Router       /master/materials/export [get]
func (h *MasterHandler) ExportMaterials(c *fiber.Ctx) error {
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT
			mc.mat_code,
			mg.group_code,
			sg.subgroup_code,
			mn.mat_name,
			COALESCE(ss.spec_description, '') AS spec_description,
			COALESCE(b.brand_name, '')        AS brand_name,
			u.unit_code,
			u.unit_name,
			COALESCE(csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code, '') AS cost_code,
			COALESCE(csg.subgroup_name, '') AS cost_subgroup_name
		FROM material_code mc
		JOIN mat_group  mg ON mg.id = mc.group_id
		JOIN subgroup   sg ON sg.id = mc.subgroup_id
		JOIN mat_name   mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size ss ON ss.id = mc.spec_id
		LEFT JOIN brand     b  ON b.id  = mc.brand_id
		JOIN unit           u  ON u.id  = mc.unit_id
		LEFT JOIN cost_subgroup csg ON csg.id = mc.cost_subgroup_id
		LEFT JOIN cost_group    cg  ON cg.id  = csg.group_id
		LEFT JOIN cost_job      cj  ON cj.id  = cg.job_id
		LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id
		WHERE mc.is_active = true
		ORDER BY mc.mat_code`)
	if err != nil {
		return err
	}
	defer rows.Close()

	f := excelize.NewFile()
	defer f.Close()

	const sheet = "Materials"
	f.SetSheetName("Sheet1", sheet)

	boldStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return err
	}

	headers := []string{"mat_code", "group_code", "subgroup_code", "mat_name", "spec_description", "brand_name", "unit_code", "unit_name", "cost_code", "cost_subgroup_name"}
	colWidths := []float64{35, 15, 18, 40, 45, 25, 12, 20, 15, 30}
	for i, h := range headers {
		col := string(rune('A' + i))
		cell := col + "1"
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, boldStyle)
		f.SetColWidth(sheet, col, col, colWidths[i])
	}

	rowIdx := 2
	for rows.Next() {
		var matCode, groupCode, subgroupCode, matName, specDesc, brandName, unitCode, unitName, costCode, costSubgroupName string
		if err := rows.Scan(&matCode, &groupCode, &subgroupCode, &matName, &specDesc, &brandName, &unitCode, &unitName, &costCode, &costSubgroupName); err != nil {
			return err
		}
		f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), matCode)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIdx), groupCode)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIdx), subgroupCode)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIdx), matName)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIdx), specDesc)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", rowIdx), brandName)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", rowIdx), unitCode)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", rowIdx), unitName)
		f.SetCellValue(sheet, fmt.Sprintf("I%d", rowIdx), costCode)
		f.SetCellValue(sheet, fmt.Sprintf("J%d", rowIdx), costSubgroupName)
		rowIdx++
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}

	filename := "materials_" + time.Now().Format("20060102") + ".xlsx"
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename="+filename)
	return c.Send(buf.Bytes())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

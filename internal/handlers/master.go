package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
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
	rows, err := h.db.Query(context.Background(), `SELECT group_code, group_name FROM mat_group ORDER BY group_code`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []models.MatGroup
	for rows.Next() {
		var m models.MatGroup
		rows.Scan(&m.GroupCode, &m.GroupName)
		items = append(items, m)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// ─── Materials ────────────────────────────────────────────────────────────────

// ListMaterials godoc
// @Summary      List materials (full view)
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        q         query  string  false  "search keyword"
// @Param        group     query  string  false  "group_code filter"
// @Param        active    query  bool    false  "active only"
// @Param        page      query  int     false  "page number"  default(1)
// @Param        page_size query  int     false  "page size"    default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /master/materials [get]
func (h *MasterHandler) ListMaterials(c *fiber.Ctx) error {
	q := c.Query("q")
	group := c.Query("group")
	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	where := []string{"1=1"}
	args := []interface{}{}
	idx := 1

	if q != "" {
		where = append(where, fmt.Sprintf("(mat_name_th ILIKE $%d OR mat_name_en ILIKE $%d OR mat_code ILIKE $%d)", idx, idx, idx))
		args = append(args, "%"+q+"%")
		idx++
	}
	if group != "" {
		where = append(where, fmt.Sprintf("group_code = $%d", idx))
		args = append(args, group)
		idx++
	}

	whereStr := strings.Join(where, " AND ")

	var total int64
	h.db.QueryRow(context.Background(), fmt.Sprintf(`SELECT COUNT(*) FROM v_material_full WHERE %s`, whereStr), args...).Scan(&total)

	args = append(args, size, offset)
	rows, err := h.db.Query(context.Background(),
		fmt.Sprintf(`SELECT mat_code, group_name, subgroup_name, mat_name_th, mat_name_en,
			spec_description, brand_name, unit_name, is_active
			FROM v_material_full WHERE %s
			ORDER BY mat_code LIMIT $%d OFFSET $%d`, whereStr, idx, idx+1), args...)
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
// @Summary      Get material by code
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        code  path  string  true  "material code"
// @Success      200   {object}  models.MaterialFull
// @Failure      404   {object}  fiber.Map
// @Router       /master/materials/{code} [get]
func (h *MasterHandler) GetMaterial(c *fiber.Ctx) error {
	code := c.Params("code")
	row := h.db.QueryRow(context.Background(), `
		SELECT mat_code, group_name, subgroup_name, mat_name_th, mat_name_en,
		       spec_description, brand_name, unit_name, is_active
		FROM v_material_full WHERE mat_code = $1`, code)

	var m models.MaterialFull
	if err := row.Scan(&m.MatCode, &m.GroupName, &m.SubgroupName, &m.MatNameTH, &m.MatNameEN,
		&m.SpecDescription, &m.BrandName, &m.UnitName, &m.IsActive); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "material not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": m})
}

// SearchMaterials godoc
// @Summary      Type-ahead material search
// @Description  Search active materials by mat_code or mat_name for the Create PO combobox. Returns mat_code, mat_name, unit, and last purchase price (nullable). Prefix matches on mat_code rank first.
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Param        q      query  string  true   "search term (matches mat_code or mat_name)"
// @Param        limit  query  int     false  "max results, default 20, cap 50"
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

	rows, err := h.db.Query(context.Background(), `
    SELECT
        mc.mat_code,
        mn.mat_name,
        u.unit_name AS unit,
        lp.last_price,
        csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code AS cost_code
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
    WHERE mc.is_active = true
      AND (mc.mat_code ILIKE '%' || $1 || '%'
           OR mn.mat_name ILIKE '%' || $1 || '%')
    ORDER BY
        (mc.mat_code ILIKE $1 || '%') DESC,
        mn.mat_name ASC
    LIMIT $2`, q, limit)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "query error: "+err.Error())
	}
	defer rows.Close()

	items := make([]models.MaterialSearchItem, 0, limit)
	for rows.Next() {
		var m models.MaterialSearchItem
		if err := rows.Scan(&m.MatCode, &m.MatName, &m.Unit, &m.LastPrice, &m.CostCode); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "scan error: "+err.Error())
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
// @Param        subgroup_id  query  int     false  "Filter by subgroup ID"
// @Param        mat_name_id  query  int     false  "Filter by mat_name ID"
// @Param        page         query  int     false  "Page number (default 1)"
// @Success      200   {object}  models.PaginatedResponse
// @Failure      500   {object}  fiber.Map
// @Router       /master/allMaterial [get]
func (h *MasterHandler) GetAllMaterial(c *fiber.Ctx) error {
	q := c.Query("q")
	subgroupID := c.QueryInt("subgroup_id", 0)
	matNameID := c.QueryInt("mat_name_id", 0)
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	const pageSize = 10
	offset := (page - 1) * pageSize

	const joins = `
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
	whereStr := strings.Join(conditions, " AND ")

	var total int
	if err := h.db.QueryRow(context.Background(),
		`SELECT COUNT(*) `+joins+` WHERE `+whereStr, args...,
	).Scan(&total); err != nil {
		return err
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
			csg.subgroup_name AS cost_subgroup_name
		%s WHERE %s
		ORDER BY mc.mat_code LIMIT $%d OFFSET $%d`,
			joins, whereStr, idx, idx+1),
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
			&m.CostCode, &m.CostSubgroupName)
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
// @Summary      List active subgroups
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}   fiber.Map
// @Router       /master/subgroups [get]
func (h *MasterHandler) ListSubgroups(c *fiber.Ctx) error {
	type subgroupItem struct {
		ID           int    `json:"id"`
		SubgroupCode string `json:"subgroup_code"`
		SubgroupName string `json:"subgroup_name"`
	}
	rows, err := h.db.Query(context.Background(),
		`SELECT id, subgroup_code, subgroup_name FROM subgroup WHERE is_active = true ORDER BY subgroup_code`)
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

// ─── Units ────────────────────────────────────────────────────────────────────

// ListUnits godoc
// @Summary      List units of measure
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  models.Unit
// @Router       /master/units [get]
func (h *MasterHandler) ListUnits(c *fiber.Ctx) error {
	rows, _ := h.db.Query(context.Background(), `SELECT unit_code, unit_name FROM unit ORDER BY unit_code`)
	defer rows.Close()
	var items []models.Unit
	for rows.Next() {
		var u models.Unit
		rows.Scan(&u.UnitCode, &u.UnitName)
		items = append(items, u)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
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
// @Summary      List warehouses
// @Tags         Master
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  models.Warehouse
// @Router       /master/warehouses [get]
func (h *MasterHandler) ListWarehouses(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT warehouse_code, warehouse_name, address, is_active, created_at FROM warehouse WHERE is_active=true ORDER BY warehouse_name`)
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
		SELECT supplier_code, supplier_name, tax_id, address, contact_name, contact_phone, contact_email, payment_terms, is_active, created_at
		FROM supplier
		WHERE is_active=true AND (supplier_name ILIKE $1 OR supplier_code ILIKE $1)
		ORDER BY supplier_name`, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []models.Supplier
	for rows.Next() {
		var s models.Supplier
		rows.Scan(&s.SupplierCode, &s.SupplierName, &s.TaxID, &s.Address,
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
		INSERT INTO supplier (supplier_code, supplier_name, tax_id, address, contact_name, contact_phone, contact_email, payment_terms)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		req.SupplierCode, req.SupplierName, req.TaxID, req.Address,
		req.ContactName, req.ContactPhone, req.ContactEmail, req.PaymentTerms)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, "supplier code already exists")
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
// @Summary      Update material names (full transaction)
// @Description  Updates subgroup_name, mat_name_th, spec_description, brand_name, unit_name and is_active in one transaction. Codes (and mat_code) are not changed.
// @Tags         Master
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        code  path      string                       true  "mat_code"
// @Param        body  body      models.UpdateMaterialRequest true  "Update data"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /master/materials/{code} [put]
func (h *MasterHandler) UpdateMaterial(c *fiber.Ctx) error {
	matCode := c.Params("code")
	var req models.UpdateMaterialRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.SubgroupName == "" || req.MatNameTH == "" || req.SpecDescription == "" ||
		req.BrandName == "" || req.UnitName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "all name fields are required")
	}

	ctx := context.Background()

	// fetch the FK codes stored in material_code
	var groupCode, subgroupCode, matNameCode, specCode, brandCode, unitCode string
	err := h.db.QueryRow(ctx,
		`SELECT group_code, subgroup_code, mat_name_code, spec_code, brand_code, unit_code
		 FROM material_code WHERE mat_code = $1`, matCode).
		Scan(&groupCode, &subgroupCode, &matNameCode, &specCode, &brandCode, &unitCode)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "material not found: "+matCode)
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. unit — update name
	if _, err = tx.Exec(ctx,
		`UPDATE unit SET unit_name = $1 WHERE unit_code = $2`,
		req.UnitName, unitCode); err != nil {
		return err
	}

	// 2. brand — update name
	if _, err = tx.Exec(ctx,
		`UPDATE brand SET brand_name = $1 WHERE brand_code = $2`,
		req.BrandName, brandCode); err != nil {
		return err
	}

	// 3. mat_subgroup — update name
	if _, err = tx.Exec(ctx,
		`UPDATE subgroup SET subgroup_name = $1 WHERE subgroup_code = $2`,
		req.SubgroupName, subgroupCode); err != nil {
		return err
	}

	// 4. mat_name — update thai name
	if _, err = tx.Exec(ctx,
		`UPDATE mat_name SET mat_name_th = $1 WHERE subgroup_code = $2 AND mat_name_code = $3`,
		req.MatNameTH, subgroupCode, matNameCode); err != nil {
		return err
	}

	// 5. mat_spec — update description
	if _, err = tx.Exec(ctx,
		`UPDATE mat_spec SET spec_description = $1 WHERE mat_name_code = $2 AND spec_code = $3`,
		req.SpecDescription, matNameCode, specCode); err != nil {
		return err
	}

	// 6. material_code — update is_active
	if _, err = tx.Exec(ctx,
		`UPDATE material_code SET is_active = $1 WHERE mat_code = $2`,
		req.IsActive, matCode); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"mat_code": matCode,
		"message":  "material updated",
	})
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

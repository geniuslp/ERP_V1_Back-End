package handlers

import (
	"context"
	"fmt"
	"strings"

	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StockByProjectHandler serves the project-first "คลัง (Stock)" page (/master/stock).
//
// Two stock systems stay separate and are never UNIONed — every response carries `source`:
//   - WAREHOUSE: project is a warehouse project -> stock_item rows of that project's warehouse.
//   - PROJECT:   normal project -> ic_project_cost_item rows (+ ic_project_cost_item_transaction).
//
// For THIS page only a warehouse project is identified by COALESCE(linked_warehouse_code,
// warehouse_code). IC code still uses linked_warehouse_code alone (linkedWarehouseCode) — untouched.
type StockByProjectHandler struct{ db *pgxpool.Pool }

func NewStockByProjectHandler(db *pgxpool.Pool) *StockByProjectHandler {
	return &StockByProjectHandler{db: db}
}

const (
	stockSourceWarehouse = "WAREHOUSE"
	stockSourceProject   = "PROJECT"

	// SQL expression for the page's warehouse mapping (alias p = project).
	stockProjectWarehouseSQL = "COALESCE(NULLIF(p.linked_warehouse_code, ''), NULLIF(p.warehouse_code, ''))"
)

type stockProjectInfo struct {
	ID            int64
	ProjectCode   string
	ProjectName   string
	CustomerName  *string
	WarehouseCode string // "" for a normal project
}

func (s stockProjectInfo) source() string {
	if s.WarehouseCode != "" {
		return stockSourceWarehouse
	}
	return stockSourceProject
}

func (h *StockByProjectHandler) resolveProject(ctx context.Context, projectCode string) (*stockProjectInfo, error) {
	if strings.TrimSpace(projectCode) == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุรหัสโครงการ")
	}
	var info stockProjectInfo
	var wh *string
	err := h.db.QueryRow(ctx, `
		SELECT p.id, p.project_code, p.project_name, c.customer_name, `+stockProjectWarehouseSQL+`
		FROM project p LEFT JOIN customer c ON c.cus_id = p.customer_id
		WHERE p.project_code = $1`, projectCode).
		Scan(&info.ID, &info.ProjectCode, &info.ProjectName, &info.CustomerName, &wh)
	if err == pgx.ErrNoRows {
		return nil, fiber.NewError(fiber.StatusNotFound, "ไม่พบโครงการนี้")
	} else if err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดข้อมูลโครงการได้")
	}
	if wh != nil {
		info.WarehouseCode = *wh
	}
	return &info, nil
}

func stockPaging(c *fiber.Ctx) (page, size, offset int) {
	page = max(c.QueryInt("page", 1), 1)
	size = c.QueryInt("page_size", 20)
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size, (page - 1) * size
}

func stockTotalPages(total int64, size int) int {
	pages := int(total) / size
	if int(total)%size != 0 || total == 0 {
		pages++
	}
	return pages
}

// stockPageData is the shared envelope body: same keys as models.PaginatedResponse plus the
// `source` discriminator and project/warehouse context.
func stockPageData(info *stockProjectInfo, items interface{}, total int64, page, size int, extra fiber.Map) fiber.Map {
	m := fiber.Map{
		"source":         info.source(),
		"project_code":   info.ProjectCode,
		"project_name":   info.ProjectName,
		"warehouse_code": nullIfEmpty(info.WarehouseCode),
		"data":           items,
		"total":          total,
		"page":           page,
		"page_size":      size,
		"total_pages":    stockTotalPages(total, size),
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ─── Project list ───────────────────────────────────────────────────────────

type stockProjectRow struct {
	ID                 int64   `json:"id"`
	ProjectCode        string  `json:"project_code"`
	ProjectName        string  `json:"project_name"`
	CustomerName       *string `json:"customer_name"`
	IsWarehouseProject bool    `json:"is_warehouse_project"`
	WarehouseCode      *string `json:"warehouse_code"`
	WarehouseName      *string `json:"warehouse_name"`
	Source             string  `json:"source"`
	ItemCount          int64   `json:"item_count"`
	TotalQty           float64 `json:"total_qty"`
}

type stockProjectOption struct {
	ProjectCode string `json:"project_code"`
	ProjectName string `json:"project_name"`
	Source      string `json:"source"`
}

// ListProjects godoc
// @Summary      All projects (Stock page, step 1)
// @Description  Lists every project (active only by default), ordered by project_code, whether or not it has stock. Projects with no stock return item_count = 0 and total_qty = 0. Warehouse project = COALESCE(linked_warehouse_code, warehouse_code) is set. item_count / total_qty only count rows with qty > 0 (warehouse project: its warehouse's stock_item rows; normal project: ic_project_cost_item rows). source = WAREHOUSE | PROJECT. Same envelope as GET /ic/projects.
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        search            query  string  false  "project_code / project_name / customer_name"
// @Param        include_inactive  query  bool    false  "include inactive projects (default false)"
// @Param        all               query  bool    false  "dropdown mode: no pagination, data = [{project_code, project_name, source}] ordered by project_code"
// @Param        page              query  int     false  "page number"  default(1)
// @Param        page_size         query  int     false  "page size (max 100)"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Failure      500  {object}  fiber.Map
// @Router       /stock/by-project [get]
func (h *StockByProjectHandler) ListProjects(c *fiber.Ctx) error {
	ctx := context.Background()
	search := strings.TrimSpace(c.Query("search"))
	page, size, offset := stockPaging(c)

	where := []string{"TRUE"}
	args := []interface{}{}
	if !c.QueryBool("include_inactive", false) {
		where = append(where, "p.is_active = true")
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		where = append(where, fmt.Sprintf("(p.project_code ILIKE $%[1]d OR p.project_name ILIKE $%[1]d OR c.customer_name ILIKE $%[1]d)", len(args)))
	}
	whereStr := strings.Join(where, " AND ")
	from := `FROM project p LEFT JOIN customer c ON c.cus_id = p.customer_id`

	// Lightweight dropdown mode: no pagination, no stock aggregation.
	if c.QueryBool("all", false) {
		rows, err := h.db.Query(ctx, `
			SELECT p.project_code, p.project_name, (`+stockProjectWarehouseSQL+` IS NOT NULL)
			`+from+` WHERE `+whereStr+`
			ORDER BY p.project_code`, args...)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการโครงการได้")
		}
		defer rows.Close()
		opts := []stockProjectOption{}
		for rows.Next() {
			var o stockProjectOption
			var isWH bool
			if err := rows.Scan(&o.ProjectCode, &o.ProjectName, &isWH); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการโครงการได้")
			}
			o.Source = stockSourceProject
			if isWH {
				o.Source = stockSourceWarehouse
			}
			opts = append(opts, o)
		}
		if err := rows.Err(); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการโครงการได้")
		}
		return c.JSON(fiber.Map{"success": true, "data": opts})
	}

	var total int64
	if err := h.db.QueryRow(ctx, `SELECT COUNT(*) `+from+` WHERE `+whereStr, args...).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการโครงการได้")
	}

	args = append(args, size, offset)
	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT p.id, p.project_code, p.project_name, c.customer_name,
		       (wh.code IS NOT NULL), wh.code, w.warehouse_name,
		       COALESCE(sw.item_count, sp.item_count, 0), COALESCE(sw.total_qty, sp.total_qty, 0)
		%s
		LEFT JOIN LATERAL (SELECT `+stockProjectWarehouseSQL+` AS code) wh ON true
		LEFT JOIN warehouse w ON w.warehouse_code = wh.code
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS item_count, COALESCE(SUM(qty), 0) AS total_qty
			FROM stock_item WHERE warehouse_code = wh.code AND qty > 0
		) sw ON wh.code IS NOT NULL
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS item_count, COALESCE(SUM(qty_on_hand), 0) AS total_qty
			FROM ic_project_cost_item WHERE project_code = p.project_code AND qty_on_hand > 0
		) sp ON wh.code IS NULL
		WHERE %s
		ORDER BY p.project_code
		LIMIT $%d OFFSET $%d`, from, whereStr, len(args)-1, len(args)), args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการโครงการได้")
	}
	defer rows.Close()

	items := []stockProjectRow{}
	for rows.Next() {
		var r stockProjectRow
		if err := rows.Scan(&r.ID, &r.ProjectCode, &r.ProjectName, &r.CustomerName,
			&r.IsWarehouseProject, &r.WarehouseCode, &r.WarehouseName, &r.ItemCount, &r.TotalQty); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการโครงการได้")
		}
		r.Source = stockSourceProject
		if r.IsWarehouseProject {
			r.Source = stockSourceWarehouse
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการโครงการได้")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: page, PageSize: size, TotalPages: stockTotalPages(total, size),
		},
	})
}

// ─── Balance ────────────────────────────────────────────────────────────────

type stockBalanceRow struct {
	ID             int64   `json:"id"`
	MatCode        string  `json:"mat_code"`
	MatName        *string `json:"mat_name"`
	CostSubgroupID *int64  `json:"cost_subgroup_id"`
	CostCode       *string `json:"cost_code"`
	Unit           *string `json:"unit"`
	QtyOnHand      float64 `json:"qty_on_hand"`
	UnitCost       float64 `json:"unit_cost"`
	LocationCode   *string `json:"location_code,omitempty"`
	// StoreDescription is stock_item.description_store (the warehouse's own wording); WAREHOUSE source only.
	StoreDescription *string `json:"store_description,omitempty"`
	UpdatedAt        string  `json:"updated_at"`
}

// GetBalance godoc
// @Summary      Stock balance of one project (Stock page, step 2)
// @Description  data.source = WAREHOUSE -> stock_item rows of the project's warehouse (COALESCE(linked_warehouse_code, warehouse_code)), never the whole table; data.source = PROJECT -> ic_project_cost_item rows of the project. The two systems are never combined. only_with_stock defaults to true (qty > 0); pass only_with_stock=false to include zero-qty rows. data.no_stock = true when the list is empty (frontend shows "ยังไม่มีสินค้าในคลังนี้"). job_code (e.g. MS) applies to PROJECT only. Newest updated_at first.
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode      path   string  true   "project.project_code"
// @Param        search           query  string  false  "mat_code / material name"
// @Param        only_with_stock  query  bool    false  "only qty > 0 (default true)"
// @Param        job_code         query  string  false  "PROJECT source only, e.g. MS"
// @Param        page             query  int     false  "page number"  default(1)
// @Param        page_size        query  int     false  "page size (max 100)"  default(20)
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /stock/by-project/{projectCode}/balance [get]
func (h *StockByProjectHandler) GetBalance(c *fiber.Ctx) error {
	ctx := context.Background()
	info, err := h.resolveProject(ctx, c.Params("projectCode"))
	if err != nil {
		return err
	}
	search := strings.TrimSpace(c.Query("search"))
	onlyWithStock := c.QueryBool("only_with_stock", true)
	page, size, offset := stockPaging(c)

	var (
		items []stockBalanceRow
		total int64
	)
	if info.WarehouseCode != "" {
		items, total, err = h.warehouseBalance(ctx, info.WarehouseCode, search, onlyWithStock, false, size, offset)
	} else {
		items, total, err = h.projectBalance(ctx, info.ProjectCode, search, strings.TrimSpace(c.Query("job_code")), onlyWithStock, size, offset)
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": stockPageData(info, items, total, page, size,
		fiber.Map{"no_stock": total == 0, "only_with_stock": onlyWithStock})})
}

// orHasCostCode (with onlyWithStock) also keeps rows that have no stock but a Cost Code - the Master warehouse page rule:
// an item belongs in the warehouse when it has a quantity OR a Cost Code.
func (h *StockByProjectHandler) warehouseBalance(ctx context.Context, wh, search string, onlyWithStock, orHasCostCode bool, size, offset int) ([]stockBalanceRow, int64, error) {
	where := []string{"si.warehouse_code = $1"}
	args := []interface{}{wh}
	if onlyWithStock {
		if orHasCostCode {
			where = append(where, "(si.qty > 0 OR mc.cost_subgroup_id IS NOT NULL)")
		} else {
			where = append(where, "si.qty > 0")
		}
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		where = append(where, fmt.Sprintf("(si.mat_code ILIKE $%[1]d OR si.item_name ILIKE $%[1]d OR mn.mat_name ILIKE $%[1]d OR sp.spec_description ILIKE $%[1]d OR si.description_store ILIKE $%[1]d)", len(args)))
	}
	whereStr := strings.Join(where, " AND ")
	join := `FROM stock_item si
		LEFT JOIN material_code mc  ON mc.mat_code = si.mat_code
		LEFT JOIN mat_name mn       ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size sp      ON sp.id = mc.spec_id
		LEFT JOIN brand br ON br.id = mc.brand_id
		LEFT JOIN unit u            ON u.id = mc.unit_id
		LEFT JOIN cost_subgroup csg ON csg.id = mc.cost_subgroup_id
		LEFT JOIN cost_group cg     ON cg.id = csg.group_id
		LEFT JOIN cost_job cj       ON cj.id = cg.job_id
		LEFT JOIN cost_subject csub ON csub.id = cj.subject_id`

	var total int64
	if err := h.db.QueryRow(ctx, `SELECT COUNT(*) `+join+` WHERE `+whereStr, args...).Scan(&total); err != nil {
		return nil, 0, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดยอดคงเหลือได้")
	}
	args = append(args, size, offset)
	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT si.id, si.mat_code,
		       COALESCE(NULLIF(TRIM(CONCAT_WS(' ', NULLIF(TRIM(mn.mat_name), ''), NULLIF(TRIM(sp.spec_description), ''), NULLIF(REGEXP_REPLACE(TRIM(br.brand_name), '^-+$', ''), ''))), ''), si.item_name),
		       mc.cost_subgroup_id, `+costCodeSQL("csub", "cj", "cg", "csg")+`,
		       COALESCE(u.unit_name, si.unit), si.qty, si.unit_cost, si.location_code,
		       NULLIF(TRIM(si.description_store), ''),
		       TO_CHAR(si.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"+07:00"')
		%s WHERE %s
		ORDER BY si.updated_at DESC, si.id DESC
		LIMIT $%d OFFSET $%d`, join, whereStr, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดยอดคงเหลือได้")
	}
	defer rows.Close()
	out := []stockBalanceRow{}
	for rows.Next() {
		var r stockBalanceRow
		if err := rows.Scan(&r.ID, &r.MatCode, &r.MatName, &r.CostSubgroupID, &r.CostCode,
			&r.Unit, &r.QtyOnHand, &r.UnitCost, &r.LocationCode, &r.StoreDescription, &r.UpdatedAt); err != nil {
			return nil, 0, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดยอดคงเหลือได้")
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดยอดคงเหลือได้")
	}
	return out, total, nil
}

func (h *StockByProjectHandler) projectBalance(ctx context.Context, projectCode, search, jobCode string, onlyWithStock bool, size, offset int) ([]stockBalanceRow, int64, error) {
	where := []string{"ici.project_code = $1"}
	args := []interface{}{projectCode}
	if onlyWithStock {
		where = append(where, "ici.qty_on_hand > 0")
	}
	if jobCode != "" {
		args = append(args, jobCode)
		where = append(where, fmt.Sprintf("csub.subject_code = LEFT(job_cost_key($%[1]d::text), 1) AND cj.job_code = SUBSTRING(job_cost_key($%[1]d::text) FROM 2)", len(args)))
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		where = append(where, fmt.Sprintf(
			"(ici.mat_code ILIKE $%[1]d OR NULLIF(TRIM(CONCAT_WS(' ', NULLIF(TRIM(mn.mat_name), ''), NULLIF(TRIM(sp.spec_description), ''), NULLIF(REGEXP_REPLACE(TRIM(br.brand_name), '^-+$', ''), ''))), '') ILIKE $%[1]d)", len(args)))
	}
	whereStr := strings.Join(where, " AND ")
	join := `FROM ic_project_cost_item ici
		JOIN material_code mc      ON mc.mat_code = ici.mat_code
		LEFT JOIN mat_name mn      ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size sp     ON sp.id = mc.spec_id
		LEFT JOIN brand br ON br.id = mc.brand_id
		LEFT JOIN unit u           ON u.id = mc.unit_id
		JOIN cost_subgroup csg     ON csg.id = ici.cost_subgroup_id
		JOIN cost_group cg         ON cg.id = csg.group_id
		JOIN cost_job cj           ON cj.id = cg.job_id
		JOIN cost_subject csub     ON csub.id = cj.subject_id`

	var total int64
	if err := h.db.QueryRow(ctx, `SELECT COUNT(*) `+join+` WHERE `+whereStr, args...).Scan(&total); err != nil {
		return nil, 0, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดยอดคงเหลือได้")
	}
	args = append(args, size, offset)
	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT ici.id, ici.mat_code,
		       NULLIF(TRIM(CONCAT_WS(' ', NULLIF(TRIM(mn.mat_name), ''), NULLIF(TRIM(sp.spec_description), ''), NULLIF(REGEXP_REPLACE(TRIM(br.brand_name), '^-+$', ''), ''))), ''),
		       ici.cost_subgroup_id, `+costCodeSQL("csub", "cj", "cg", "csg")+`,
		       COALESCE(u.unit_name, ici.unit), ici.qty_on_hand, ici.last_unit_cost,
		       TO_CHAR(ici.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"+07:00"')
		%s WHERE %s
		ORDER BY ici.updated_at DESC, ici.id DESC
		LIMIT $%d OFFSET $%d`, join, whereStr, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดยอดคงเหลือได้")
	}
	defer rows.Close()
	out := []stockBalanceRow{}
	for rows.Next() {
		var r stockBalanceRow
		if err := rows.Scan(&r.ID, &r.MatCode, &r.MatName, &r.CostSubgroupID, &r.CostCode,
			&r.Unit, &r.QtyOnHand, &r.UnitCost, &r.UpdatedAt); err != nil {
			return nil, 0, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดยอดคงเหลือได้")
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดยอดคงเหลือได้")
	}
	return out, total, nil
}

// ─── Movement history ───────────────────────────────────────────────────────

type stockWarehouseTxnRow struct {
	ID            int64    `json:"id"`
	TxnNo         string   `json:"txn_no"`
	TxnType       string   `json:"txn_type"`
	MatCode       string   `json:"mat_code"`
	MatName       string   `json:"mat_name"`
	Qty           float64  `json:"qty"`
	QtyBefore     *float64 `json:"qty_before"`
	QtyAfter      *float64 `json:"qty_after"`
	FromLocation  *string  `json:"from_location"`
	ToLocation    *string  `json:"to_location"`
	RefDocType    *string  `json:"ref_doc_type"`
	RefDocID      *int64   `json:"ref_doc_id"`
	Remarks       *string  `json:"remarks"`
	TxnDate       string   `json:"txn_date"`
	CreatedByName string   `json:"created_by_name"`
	CreatedAt     string   `json:"created_at"`
}

// ListTransactions godoc
// @Summary      Movement history of one project (Stock page, step 3)
// @Description  data.source = PROJECT -> ic_project_cost_item_transaction rows (same query and row shape as GET /ic/projects/{projectCode}/cost-transactions; supports ref_type). data.source = WAREHOUSE -> stock_transaction rows of stock_item in the project's warehouse (the row shape differs: txn_no, txn_type, from/to_location, ref_doc_type...). Never combined.
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path   string  true   "project.project_code"
// @Param        ref_type     query  string  false  "PROJECT source only: PO, MOVEMENT_ISSUE or MOVEMENT_TRANSFER"
// @Param        search       query  string  false  "mat_code / material name"
// @Param        date_from    query  string  false  "YYYY-MM-DD"
// @Param        date_to      query  string  false  "YYYY-MM-DD"
// @Param        page         query  int     false  "page number"  default(1)
// @Param        page_size    query  int     false  "page size (max 100)"  default(20)
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /stock/by-project/{projectCode}/transactions [get]
func (h *StockByProjectHandler) ListTransactions(c *fiber.Ctx) error {
	ctx := context.Background()
	info, err := h.resolveProject(ctx, c.Params("projectCode"))
	if err != nil {
		return err
	}
	var f models.IcProjectCostTransactionFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "พารามิเตอร์ไม่ถูกต้อง")
	}

	if info.WarehouseCode == "" {
		items, total, page, size, err := queryProjectCostTransactions(ctx, h.db, info.ProjectCode, f)
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"success": true, "data": stockPageData(info, items, total, page, size, nil)})
	}

	page, size, offset := stockPaging(c)
	where := []string{"si.warehouse_code = $1"}
	args := []interface{}{info.WarehouseCode}
	if f.DateFrom != "" {
		args = append(args, f.DateFrom)
		where = append(where, fmt.Sprintf("t.txn_date >= $%d", len(args)))
	}
	if f.DateTo != "" {
		args = append(args, f.DateTo)
		where = append(where, fmt.Sprintf("t.txn_date <= $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		where = append(where, fmt.Sprintf("(si.mat_code ILIKE $%[1]d OR si.item_name ILIKE $%[1]d)", len(args)))
	}
	whereStr := strings.Join(where, " AND ")
	join := `FROM stock_transaction t JOIN stock_item si ON si.id = t.item_id LEFT JOIN users u ON u.id = t.created_by`

	var total int64
	if err := h.db.QueryRow(ctx, `SELECT COUNT(*) `+join+` WHERE `+whereStr, args...).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดประวัติการเคลื่อนไหวได้")
	}
	args = append(args, size, offset)
	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT t.id, t.txn_no, t.txn_type, si.mat_code, si.item_name, t.qty, t.qty_before, t.qty_after,
		       t.from_location, t.to_location, t.ref_doc_type, t.ref_doc_id, t.remarks, t.txn_date::text,
		       COALESCE(u.full_name, ''), TO_CHAR(t.created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"+07:00"')
		%s WHERE %s
		ORDER BY t.txn_date DESC, t.id DESC
		LIMIT $%d OFFSET $%d`, join, whereStr, len(args)-1, len(args)), args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดประวัติการเคลื่อนไหวได้")
	}
	defer rows.Close()
	items := []stockWarehouseTxnRow{}
	for rows.Next() {
		var r stockWarehouseTxnRow
		if err := rows.Scan(&r.ID, &r.TxnNo, &r.TxnType, &r.MatCode, &r.MatName, &r.Qty, &r.QtyBefore, &r.QtyAfter,
			&r.FromLocation, &r.ToLocation, &r.RefDocType, &r.RefDocID, &r.Remarks, &r.TxnDate, &r.CreatedByName, &r.CreatedAt); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดประวัติการเคลื่อนไหวได้")
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดประวัติการเคลื่อนไหวได้")
	}
	return c.JSON(fiber.Map{"success": true, "data": stockPageData(info, items, total, page, size, nil)})
}

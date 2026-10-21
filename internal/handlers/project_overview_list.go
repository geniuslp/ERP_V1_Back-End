package handlers

import (
	"context"
	"log"
	"strings"

	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
)

// Project Overview money columns: Contract Budget / Pu.Cost / Pu.Balance / Ac.Cost / Ac.Balance / Status.
//
// Bases (see ProjectOverviewHandler.List):
//   - Contract Budget = project_kind='VO' ? vo_amount : budget_amount.
//   - Pu.Cost (committed) = PO lines (amount - line_discount, ex-VAT/ex-WHT) + WO total_amount (ex-VAT, after
//     line discounts) + approved petty cash line_net. Memo excluded.
//   - Ac.Cost (actual)    = PO received value (qty_received x unit_price less the received share of line_discount,
//     ex-VAT, returns already netted in qty_received) + WO payments (payment_log, net of reversals) + approved
//     petty cash line_net. The IC ledgers are not used.

// Statuses of a PO / WO that count as committed. PO also lists the legacy receiving statuses that older rows may
// still hold in purchase_order.status.
const (
	overviewPOStatuses = `'APPROVED','PENDING_REAPPROVAL','SENT','PARTIALLY_RECEIVED','RECEIVED'`
	overviewWOStatuses = `'APPROVED','PENDING_REAPPROVAL'`
)

var overviewStatusLabels = map[string]string{
	"NORMAL":      "ปกติ",
	"NEAR_LIMIT":  "ใกล้เต็มงบ",
	"OVER_BUDGET": "เกินงบ",
	"NO_BUDGET":   "ไม่มีงบ",
	"CLOSED":      "ปิดโครงการ",
	"INACTIVE":    "ไม่ใช้งาน",
}

// overviewMaxPageSize caps page_size (the page shows a whole portfolio).
const overviewMaxPageSize = 200

const overviewStatusList = "NORMAL, NEAR_LIMIT, OVER_BUDGET, NO_BUDGET, CLOSED, INACTIVE"

// overviewListSQL is one set-based query: every source is aggregated once per project_code in its own CTE and
// joined to project; status is derived in SQL so the status filter applies before pagination.
// $1 search (nullable), $2 status filter (nullable), $3 limit, $4 offset.
const overviewListSQL = `
WITH po AS (
	SELECT o.project_code,
	       SUM(COALESCE(l.amount, l.qty_ordered * l.unit_price) - COALESCE(l.line_discount, 0)) AS pu,
	       SUM(CASE WHEN l.qty_ordered > 0
	                THEN l.qty_received * l.unit_price - COALESCE(l.line_discount, 0) * l.qty_received / l.qty_ordered
	                ELSE 0 END) AS ac
	FROM purchase_order_line l
	JOIN purchase_order o ON o.id = l.po_id
	WHERE o.deleted_at IS NULL
	  AND o.project_code IS NOT NULL
	  AND o.status IN (` + overviewPOStatuses + `)
	  AND l.status <> 'CANCELLED'
	GROUP BY o.project_code
), wo AS (
	SELECT project_code, SUM(total_amount) AS pu
	FROM work_order
	WHERE project_code IS NOT NULL AND status IN (` + overviewWOStatuses + `)
	GROUP BY project_code
), wo_paid AS (
	SELECT w.project_code, SUM(pl.amount_paid) AS ac
	FROM payment_log pl
	JOIN work_order w ON w.id = pl.doc_id
	WHERE pl.doc_type = 'WO' AND w.project_code IS NOT NULL
	GROUP BY w.project_code
), pc AS (
	SELECT l.project_code, SUM(COALESCE(l.line_net, l.amount)) AS amt
	FROM petty_cash_requisition_line l
	JOIN petty_cash_requisition h ON h.id = l.pc_id
	WHERE h.status = 'APPROVED' AND l.project_code IS NOT NULL
	GROUP BY l.project_code
), base AS (
	SELECT p.id, p.customer_id, p.project_code, p.project_name, cu.customer_name,
	       p.status AS project_status, p.is_active, p.project_kind,
	       (CASE WHEN p.project_kind = 'VO' THEN p.vo_amount ELSE p.budget_amount END) AS contract_budget,
	       COALESCE(po.pu, 0) AS pu_po, COALESCE(wo.pu, 0) AS pu_wo, COALESCE(pc.amt, 0) AS pu_pc,
	       COALESCE(po.ac, 0) AS ac_po, COALESCE(wo_paid.ac, 0) AS ac_wo, COALESCE(pc.amt, 0) AS ac_pc
	FROM project p
	LEFT JOIN customer cu ON cu.cus_id = p.customer_id
	LEFT JOIN po ON po.project_code = p.project_code
	LEFT JOIN wo ON wo.project_code = p.project_code
	LEFT JOIN wo_paid ON wo_paid.project_code = p.project_code
	LEFT JOIN pc ON pc.project_code = p.project_code
	WHERE ($1::text IS NULL
	       OR p.project_code ILIKE $1 OR p.project_name ILIKE $1 OR cu.customer_name ILIKE $1)
), calc AS (
	SELECT b.*,
	       b.pu_po + b.pu_wo + b.pu_pc AS pu_cost,
	       b.ac_po + b.ac_wo + b.ac_pc AS ac_cost,
	       (CASE
	          WHEN b.project_status = 'CLOSED' THEN 'CLOSED'
	          WHEN b.project_status = 'INACTIVE' OR NOT b.is_active THEN 'INACTIVE'
	          WHEN b.contract_budget <= 0 THEN 'NO_BUDGET'
	          WHEN b.pu_po + b.pu_wo + b.pu_pc > b.contract_budget THEN 'OVER_BUDGET'
	          WHEN b.pu_po + b.pu_wo + b.pu_pc >= b.contract_budget * 0.9 THEN 'NEAR_LIMIT'
	          ELSE 'NORMAL' END) AS overview_status
	FROM base b
)
SELECT c.id, c.customer_id, c.project_code, c.project_name, c.customer_name, c.project_status, c.is_active, c.project_kind,
       c.contract_budget::float8,
       c.pu_cost::float8, (c.contract_budget - c.pu_cost)::float8,
       c.ac_cost::float8, (c.contract_budget - c.ac_cost)::float8,
       (CASE WHEN c.contract_budget > 0 THEN c.pu_cost * 100 / c.contract_budget END)::float8,
       c.overview_status,
       c.pu_po::float8, c.pu_wo::float8, c.pu_pc::float8,
       c.ac_po::float8, c.ac_wo::float8, c.ac_pc::float8,
       COUNT(*) OVER ()
FROM calc c
WHERE (($2::text IS NULL AND c.overview_status <> 'INACTIVE') OR c.overview_status = $2::text)
ORDER BY c.project_code
LIMIT $3 OFFSET $4`

type overviewBucket struct {
	PO float64 `json:"po"`
	WO float64 `json:"wo"`
	PC float64 `json:"petty_cash"`
}

type overviewRow struct {
	ID            int64    `json:"id"`
	CustomerID    *int64   `json:"customer_id"`
	ProjectCode   string   `json:"project_code"`
	ProjectName   string   `json:"project_name"`
	CustomerName  *string  `json:"customer_name"`
	ProjectStatus string   `json:"project_status"`
	IsActive      bool     `json:"is_active"`
	ProjectKind   string   `json:"project_kind"`
	Contract      float64  `json:"contract_budget"`
	PuCost        float64  `json:"pu_cost"`
	PuBalance     float64  `json:"pu_balance"`
	AcCost        float64  `json:"ac_cost"`
	AcBalance     float64  `json:"ac_balance"`
	UsagePct      *float64 `json:"usage_pct"` // Pu.Cost / Contract Budget x 100; null when the budget is 0
	Status        string   `json:"status"`
	StatusLabel   string   `json:"status_label"`
	// Per-source split of pu_cost / ac_cost. ac.wo is the amount paid (payable basis), not ex-VAT.
	PuBreakdown overviewBucket `json:"pu_breakdown"`
	AcBreakdown overviewBucket `json:"ac_breakdown"`
}

// List godoc
// @Summary      Project Overview — Contract Budget / Pu.Cost / Pu.Balance / Ac.Cost / Ac.Balance / Status
// @Description  One row per project, computed in a single set-based query.
// @Description  contract_budget = vo_amount for project_kind VO, else budget_amount.
// @Description  pu_cost (committed, ex-VAT/ex-WHT) = PO lines (amount - line_discount; PO status APPROVED / PENDING_REAPPROVAL / legacy SENT, PARTIALLY_RECEIVED, RECEIVED; not deleted; line not CANCELLED) + WO total_amount (status APPROVED / PENDING_REAPPROVAL) + approved petty cash line_net. Memo is excluded.
// @Description  ac_cost (actual) = PO received value (qty_received x unit_price less the received share of line_discount, ex-VAT; returns already netted in qty_received) + WO amount paid (payment_log, net of reversals; this is on the PAYABLE basis, i.e. incl. VAT and net of WHT, NOT ex-VAT) + approved petty cash line_net (payable basis too). IC ledgers are not used.
// @Description  pu_balance = contract_budget - pu_cost; ac_balance = contract_budget - ac_cost (NULL sources count as 0, no rounding).
// @Description  status (from pu_cost / contract_budget): CLOSED (project.status) first, then INACTIVE (project.status INACTIVE or is_active=false), NO_BUDGET (budget <= 0), OVER_BUDGET (> 100%), NEAR_LIMIT (>= 90%), NORMAL. usage_pct is that ratio in percent (null when the budget is 0).
// @Description  Default list hides INACTIVE projects (only ACTIVE and CLOSED ones are listed); pass status=INACTIVE to see them. Ordered by project_code; page_size max 200.
// @Tags         Project Overview
// @Security     BearerAuth
// @Produce      json
// @Param        search     query  string  false  "project_code / project_name / customer_name"
// @Param        status     query  string  false  "NORMAL | NEAR_LIMIT | OVER_BUDGET | NO_BUDGET | CLOSED | INACTIVE"
// @Param        page       query  int     false  "page number"  default(1)
// @Param        page_size  query  int     false  "page size (max 200)"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Failure      400  {object}  fiber.Map
// @Router       /project-overview [get]
func (h *ProjectOverviewHandler) List(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	size := c.QueryInt("page_size", 20)
	if page < 1 {
		return fiber.NewError(fiber.StatusBadRequest, "หมายเลขหน้าต้องมากกว่า 0")
	}
	if size < 1 {
		return fiber.NewError(fiber.StatusBadRequest, "จำนวนต่อหน้าต้องมากกว่า 0")
	}
	if size > overviewMaxPageSize {
		size = overviewMaxPageSize
	}

	var search, statusFilter interface{}
	if s := strings.TrimSpace(c.Query("search")); s != "" {
		search = "%" + s + "%"
	}
	if st := strings.ToUpper(strings.TrimSpace(c.Query("status"))); st != "" {
		if _, ok := overviewStatusLabels[st]; !ok {
			return fiber.NewError(fiber.StatusBadRequest, "สถานะไม่ถูกต้อง (ใช้ได้: "+overviewStatusList+")")
		}
		statusFilter = st
	}

	rows, err := h.db.Query(context.Background(), overviewListSQL, search, statusFilter, size, (page-1)*size)
	if err != nil {
		log.Printf("project overview list: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดภาพรวมโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()

	items := []overviewRow{}
	var total int64
	for rows.Next() {
		var r overviewRow
		if err := rows.Scan(&r.ID, &r.CustomerID, &r.ProjectCode, &r.ProjectName, &r.CustomerName, &r.ProjectStatus, &r.IsActive, &r.ProjectKind,
			&r.Contract, &r.PuCost, &r.PuBalance, &r.AcCost, &r.AcBalance, &r.UsagePct, &r.Status,
			&r.PuBreakdown.PO, &r.PuBreakdown.WO, &r.PuBreakdown.PC,
			&r.AcBreakdown.PO, &r.AcBreakdown.WO, &r.AcBreakdown.PC, &total); err != nil {
			log.Printf("project overview list scan: %v", err)
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดภาพรวมโครงการได้ กรุณาลองใหม่อีกครั้ง")
		}
		r.StatusLabel = overviewStatusLabels[r.Status]
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		log.Printf("project overview list rows: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดภาพรวมโครงการได้ กรุณาลองใหม่อีกครั้ง")
	}

	pages := int(total) / size
	if int(total)%size != 0 || total == 0 {
		pages++
	}
	return c.JSON(fiber.Map{"success": true, "data": models.PaginatedResponse{
		Data: items, Total: total, Page: page, PageSize: size, TotalPages: pages,
	}})
}

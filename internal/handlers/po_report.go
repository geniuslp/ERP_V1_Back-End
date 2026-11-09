package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// POReportHandler serves the PO/WO > รายงานการสั่งซื้อ reports.
type POReportHandler struct{ db *pgxpool.Pool }

func NewPOReportHandler(db *pgxpool.Pool) *POReportHandler { return &POReportHandler{db: db} }

type poReportLine struct {
	LineNo      int     `json:"line_no"`
	JobCode     *string `json:"job_code"`
	Description string  `json:"description"`
	Qty         float64 `json:"qty"`
	UnitName    *string `json:"unit_name"`
	UnitPrice   float64 `json:"unit_price"`
	Amount      float64 `json:"amount"` // before VAT
	Remarks     *string `json:"remarks"`
}

type poReportGroup struct {
	POID         int64          `json:"po_id"`
	PONo         string         `json:"po_no"`
	PODate       *string        `json:"po_date"`
	Status       string         `json:"status"`
	SupplierName *string        `json:"supplier_name"`
	ProjectCode  *string        `json:"project_code"`
	ProjectName  *string        `json:"project_name"`
	Subtotal     float64        `json:"subtotal"`
	Lines        []poReportLine `json:"lines"`
}

// lineJobSQL: the job of a PO line. The job of its cost code (subject + job letters, mapped to the current job codes)
// when it has one, else the PO's own job_code (overhead cost codes carry no job of their own).
const poReportLineJobSQL = `(CASE
	WHEN pol.cost_subgroup_id IS NULL OR csub.subject_code = 'OH' THEN po.job_code
	ELSE (CASE csub.subject_code || cj.job_code
		WHEN 'MP' THEN 'MS' WHEN 'ME' THEN 'EE' WHEN 'MS' THEN 'SN' WHEN 'MF' THEN 'FF'
		WHEN 'MG' THEN 'GS' WHEN 'MH' THEN 'AC' WHEN 'MC' THEN 'CS' WHEN 'MA' THEN 'AR'
		ELSE csub.subject_code || cj.job_code END)
END)`

// History godoc
// @Summary      รายงานประวัติใบสั่ง (PO ทุกสถานะยกเว้น CANCELLED, รวม DRAFT) จัดกลุ่มตามใบสั่ง
// @Description  Each group = one PO (supplier + PO number) with its non-cancelled lines and the PO subtotal (before VAT);
// @Description  grand_total = sum of the subtotals. Filters: project_code, supplier_id, date_from / date_to (po_date, YYYY-MM-DD).
// @Tags         POReport
// @Security     BearerAuth
// @Produce      json
// @Router       /po-report/history [get]
func (h *POReportHandler) History(c *fiber.Ctx) error { return h.report(c, false) }

// PendingReceipt godoc
// @Summary      รายการค้างรับสินค้า (PO ที่อนุมัติแล้วแต่ยังไม่ผ่านกระบวนการรับของ IC) จัดกลุ่มตามใบสั่ง
// @Description  Same shape as /po-report/history. Only approved POs (APPROVED / SENT / PARTIALLY_RECEIVED) and only their
// @Description  lines that still have quantity to receive; qty and amount are the OUTSTANDING part (ordered - received).
// @Description  Overhead order types (asset / office equipment / tool) never go through IC receiving and are left out.
// @Description  Filters: project_code, job_code (the line's job), date_from / date_to (po_date).
// @Tags         POReport
// @Security     BearerAuth
// @Produce      json
// @Router       /po-report/pending-receipt [get]
func (h *POReportHandler) PendingReceipt(c *fiber.Ctx) error { return h.report(c, true) }

func (h *POReportHandler) report(c *fiber.Ctx, pending bool) error {
	where := []string{"po.status <> 'CANCELLED'", "po.deleted_at IS NULL", "pol.status <> 'CANCELLED'"}
	qtyExpr := "COALESCE(pol.qty_ordered, 0)::float8"
	amountExpr := "COALESCE(pol.amount, pol.qty_ordered * pol.unit_price, 0)::float8"
	if pending {
		where = []string{
			"po.status IN ('APPROVED', 'SENT', 'PARTIALLY_RECEIVED')", "po.deleted_at IS NULL",
			"pol.status IN ('OPEN', 'PARTIAL')",
			"COALESCE(po.order_type, '') NOT IN ('asset_equipment', 'office_equipment', 'asset_tool')",
			"COALESCE(pol.qty_ordered, 0) - COALESCE(pol.qty_received, 0) > 0",
		}
		qtyExpr = "(COALESCE(pol.qty_ordered, 0) - COALESCE(pol.qty_received, 0))::float8"
		amountExpr = "((COALESCE(pol.qty_ordered, 0) - COALESCE(pol.qty_received, 0)) * COALESCE(pol.unit_price, 0))::float8"
	}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.Replace(cond, "?", fmt.Sprintf("$%d", len(args)), 1))
	}
	if v := strings.TrimSpace(c.Query("project_code")); v != "" {
		add("po.project_code = ?", v)
	}
	if v := strings.TrimSpace(c.Query("supplier_id")); v != "" {
		add("po.supplier_id = ?::bigint", v)
	}
	if v := strings.TrimSpace(c.Query("job_code")); v != "" && pending {
		add(poReportLineJobSQL+" = ?", v)
	}
	if v := strings.TrimSpace(c.Query("date_from")); v != "" {
		add("po.po_date >= ?::date", v)
	}
	if v := strings.TrimSpace(c.Query("date_to")); v != "" {
		add("po.po_date <= ?::date", v)
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT po.id, po.po_no, to_char(po.po_date, 'YYYY-MM-DD'), po.status, s.supplier_name, po.project_code, p.project_name,
		       pol.line_no, `+poReportLineJobSQL+`,
		       COALESCE(NULLIF(btrim(COALESCE(mn.mat_name, '') || ' ' || COALESCE(ss.spec_description, '')), ''), pol.description, ''),
		       `+qtyExpr+`, u.unit_name, COALESCE(pol.unit_price, 0)::float8,
		       `+amountExpr+`, pol.remarks
		FROM purchase_order po
		JOIN purchase_order_line pol ON pol.po_id = po.id
		LEFT JOIN supplier s ON s.id = po.supplier_id
		LEFT JOIN project p ON p.project_code = po.project_code
		LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
		LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size ss ON ss.id = mc.spec_id
		LEFT JOIN unit u ON u.id = mc.unit_id
		LEFT JOIN cost_subgroup csg ON csg.id = pol.cost_subgroup_id
		LEFT JOIN cost_group cg ON cg.id = csg.group_id
		LEFT JOIN cost_job cj ON cj.id = cg.job_id
		LEFT JOIN cost_subject csub ON csub.id = cj.subject_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY s.supplier_name NULLS LAST, po.po_no, pol.line_no`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	groups := []*poReportGroup{}
	byID := map[int64]*poReportGroup{}
	grand := 0.0
	for rows.Next() {
		var g poReportGroup
		var l poReportLine
		if err := rows.Scan(&g.POID, &g.PONo, &g.PODate, &g.Status, &g.SupplierName, &g.ProjectCode, &g.ProjectName,
			&l.LineNo, &l.JobCode, &l.Description, &l.Qty, &l.UnitName, &l.UnitPrice, &l.Amount, &l.Remarks); err != nil {
			return err
		}
		cur, ok := byID[g.POID]
		if !ok {
			g.Lines = []poReportLine{}
			cur = &g
			byID[g.POID] = cur
			groups = append(groups, cur)
		}
		cur.Lines = append(cur.Lines, l)
		cur.Subtotal += l.Amount
		grand += l.Amount
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, g := range groups {
		g.Subtotal = woRound2(g.Subtotal)
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"groups": groups, "grand_total": woRound2(grand)}})
}

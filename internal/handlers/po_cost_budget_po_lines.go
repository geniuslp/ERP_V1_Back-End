package handlers

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type costBudgetPOLine struct {
	POID        int64   `json:"po_id"`
	PONo        string  `json:"po_no"`
	ProjectCode string  `json:"project_code"`
	ProjectName *string `json:"project_name"`
	LineNo      int     `json:"line_no"`
	MatCode     *string `json:"mat_code"`
	ItemName    *string `json:"item_name"`
	Quantity    float64 `json:"quantity"`
	Unit        *string `json:"unit"`
	Remarks     *string `json:"remarks"`
	POStatus    string  `json:"po_status"`
	Amount      float64 `json:"amount"` // qty_ordered * unit_price — the Pu.Cost basis, for reference only
}

// CostBudgetPOLines godoc
// @Summary      Cost Budget — PO lines charged to one cost code of a project
// @Description  Rows behind the Pu.Cost figure of one ITEM row of GET /po/{projectCode}/cost-budget-detail. Filtered by ids only (project_code + cost_subgroup_id, never the displayed cost code) with EXACTLY the Pu rule: po.deleted_at IS NULL, po.status NOT IN (DRAFT, REJECTED, CANCELLED), line status <> CANCELLED. So SUM(amount) = the row's pu_cost. quantity = qty_ordered; amount = qty_ordered * unit_price (reference only). item_name = material name + spec, falling back to the line description (same as the purchases drill-down). unit = material_code unit. Ordered by po_no, line_no; not paginated (one cost code in one project). 404 when the cost code is not an ITEM of this project's budget page (no budget row and no PO line).
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode      path  string  true  "Project code"
// @Param        costSubgroupId   path  int     true  "cost_subgroup.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/cost-budget/{projectCode}/cost-codes/{costSubgroupId}/po-lines [get]
func (h *POHandler) CostBudgetPOLines(c *fiber.Ctx) error {
	projectCode := strings.TrimSpace(c.Params("projectCode"))
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุรหัสโครงการ")
	}
	subgroupID, err := strconv.ParseInt(c.Params("costSubgroupId"), 10, 64)
	if err != nil || subgroupID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสต้นทุนไม่ถูกต้อง")
	}
	ctx := context.Background()
	const loadErr = "ไม่สามารถโหลดรายการ PO ได้ กรุณาลองใหม่อีกครั้ง"

	// The cost code must be an ITEM row of this project's page: a non-Head, non-Major subgroup that has a
	// project_cost_budget row or a counted PO line (the "keys" set of CostBudgetDetail).
	var belongs bool
	if err := h.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM cost_subgroup cs
			WHERE cs.id = $2 AND NOT cs.is_head AND NOT cs.is_major
			  AND (EXISTS (SELECT 1 FROM project_cost_budget b
			               JOIN project p ON p.id = b.project_id
			               WHERE p.project_code = $1 AND b.cost_subgroup_id = cs.id)
			    OR EXISTS (SELECT 1 FROM purchase_order_line pol
			               JOIN purchase_order po ON po.id = pol.po_id
			               WHERE po.project_code = $1 AND pol.cost_subgroup_id = cs.id
			                 AND po.deleted_at IS NULL
			                 AND po.status NOT IN ('DRAFT','REJECTED','CANCELLED')
			                 AND pol.status <> 'CANCELLED')))`,
		projectCode, subgroupID).Scan(&belongs); err != nil {
		log.Printf("cost-budget po-lines belongs project=%s sub=%d: %v", projectCode, subgroupID, err)
		return fiber.NewError(fiber.StatusInternalServerError, loadErr)
	}
	if !belongs {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบรหัสต้นทุนในโครงการนี้")
	}

	rows, err := h.db.Query(ctx, `
		SELECT po.id, po.po_no, po.project_code, pj.project_name, pol.line_no, pol.mat_code,
		       COALESCE(
		           NULLIF(TRIM(CONCAT_WS(' ', mn.mat_name, ss.spec_description)), ''),
		           NULLIF(TRIM(pol.description), '')
		       ) AS item_name,
		       pol.qty_ordered::float8, un.unit_name, NULLIF(TRIM(pol.remarks), ''), po.status,
		       (pol.qty_ordered * pol.unit_price)::float8
		FROM purchase_order_line pol
		JOIN purchase_order po ON po.id = pol.po_id
		LEFT JOIN project pj ON pj.project_code = po.project_code
		LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
		LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size ss ON ss.id = mc.spec_id
		LEFT JOIN unit un ON un.id = mc.unit_id
		WHERE po.project_code = $1 AND pol.cost_subgroup_id = $2
		  AND po.deleted_at IS NULL
		  AND po.status NOT IN ('DRAFT','REJECTED','CANCELLED')
		  AND pol.status <> 'CANCELLED'
		ORDER BY po.po_no, pol.line_no`, projectCode, subgroupID)
	if err != nil {
		log.Printf("cost-budget po-lines query project=%s sub=%d: %v", projectCode, subgroupID, err)
		return fiber.NewError(fiber.StatusInternalServerError, loadErr)
	}
	defer rows.Close()

	out := []costBudgetPOLine{}
	for rows.Next() {
		var l costBudgetPOLine
		if err := rows.Scan(&l.POID, &l.PONo, &l.ProjectCode, &l.ProjectName, &l.LineNo, &l.MatCode, &l.ItemName,
			&l.Quantity, &l.Unit, &l.Remarks, &l.POStatus, &l.Amount); err != nil {
			log.Printf("cost-budget po-lines scan: %v", err)
			return fiber.NewError(fiber.StatusInternalServerError, loadErr)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		log.Printf("cost-budget po-lines rows: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, loadErr)
	}
	return c.JSON(fiber.Map{"success": true, "data": out})
}

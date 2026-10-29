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
	Amount      float64 `json:"amount"` // PO: qty_ordered * unit_price (reference); PC: amount - line_discount (ex-VAT)
	Source      string  `json:"source"` // "PO", "PC" (petty cash) or "TR" (project transfer)
	// TR rows: po_id / po_no / po_status hold the movement id / TF doc_no / status, project_* is the OTHER
	// project of the transfer (the source for money coming in, the destination for money going out),
	// quantity and amount are negative when the goods leave this project.
	DocProjectCode *string `json:"doc_project_code,omitempty"` // project that owns the TR document (for its detail route)
}

// CostBudgetPOLines godoc
// @Summary      Cost Budget — PO and petty cash lines charged to one cost code of a project
// @Description  Rows behind the Pu.Cost figure of one ITEM row of GET /po/{projectCode}/cost-budget-detail. Filtered by ids only (project_code + cost_subgroup_id, never the displayed cost code) with EXACTLY the Pu rule: po.deleted_at IS NULL, po.status NOT IN (DRAFT, REJECTED, CANCELLED), line status <> CANCELLED. So SUM(amount) = the row's pu_cost. quantity = qty_ordered; amount = qty_ordered * unit_price (reference only). item_name = material name + spec, falling back to the line description (same as the purchases drill-down). unit = material_code unit. Ordered by po_no, line_no; not paginated (one cost code in one project). 404 when the cost code is not an ITEM of this project's budget page (no budget row, no PO line and no APPROVED petty cash line).
// @Description  Every row has source: "PO" (the rows above) or "PC" — APPROVED petty cash lines (header status APPROVED, the LINE's project_code = this project, cost_subgroup_id = this cost code) mapped as: po_id = petty cash header id, po_no = petty cash document number (PC-YYYYMM-NNNN), po_status = petty cash status, item_name = line description else material name + spec, quantity = line qty, amount = amount - line_discount (EX-VAT/ex-WHT; spent_amount in GET /master/projects uses line_net, VAT-inclusive). The PC amounts sum to the row's pc_cost on cost-budget-detail; PO amounts stay qty_ordered * unit_price as before. costSubgroupId may be the literal "none": returns only the PC rows of the project with no cost code (the unassigned row), 404 if there are none. Order: PO rows first, then PC, each by document number, line_no.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode      path  string  true  "Project code"
// @Param        costSubgroupId   path  string  true  "cost_subgroup.id, or none for petty cash lines without a cost code"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/cost-budget/{projectCode}/cost-codes/{costSubgroupId}/po-lines [get]
func (h *POHandler) CostBudgetPOLines(c *fiber.Ctx) error {
	projectCode := strings.TrimSpace(c.Params("projectCode"))
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุรหัสโครงการ")
	}
	// nil = the "none" row: petty cash lines of the project that carry no cost code.
	var subgroupID *int64
	if raw := strings.TrimSpace(c.Params("costSubgroupId")); !strings.EqualFold(raw, "none") {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, "รหัสต้นทุนไม่ถูกต้อง")
		}
		subgroupID = &id
	}
	ctx := context.Background()
	const loadErr = "ไม่สามารถโหลดรายการ PO ได้ กรุณาลองใหม่อีกครั้ง"

	// The cost code must be an ITEM row of this project's page: a non-Head, non-Major subgroup that has a
	// project_cost_budget row, a counted PO line or an APPROVED petty cash line (the "keys" set of
	// CostBudgetDetail). "none" belongs when the project has an APPROVED petty cash line with no cost code.
	var belongs bool
	var err error
	if subgroupID == nil {
		err = h.db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM petty_cash_requisition_line l
				JOIN petty_cash_requisition h ON h.id = l.pc_id
				WHERE h.status = 'APPROVED' AND l.project_code = $1 AND l.cost_subgroup_id IS NULL)`,
			projectCode).Scan(&belongs)
	} else {
		err = h.db.QueryRow(ctx, `
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
				                 AND pol.status <> 'CANCELLED')
				    OR EXISTS (SELECT 1 FROM petty_cash_requisition_line l
				               JOIN petty_cash_requisition h ON h.id = l.pc_id
				               WHERE h.status = 'APPROVED' AND l.project_code = $1
				                 AND l.cost_subgroup_id = cs.id)
				    OR EXISTS (SELECT 1 FROM ic_project_movement_line l
				               JOIN ic_project_movement m ON m.id = l.movement_id
				               WHERE m.doc_type = 'TRANSFER' AND m.status = 'POSTED'
				                 AND ((l.to_project_code = $1 AND l.to_cost_subgroup_id = cs.id)
				                   OR (m.project_code = $1 AND l.cost_subgroup_id = cs.id)))))`,
			projectCode, *subgroupID).Scan(&belongs)
	}
	if err != nil {
		log.Printf("cost-budget po-lines belongs project=%s sub=%v: %v", projectCode, subgroupID, err)
		return fiber.NewError(fiber.StatusInternalServerError, loadErr)
	}
	if !belongs {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบรหัสต้นทุนในโครงการนี้")
	}

	// PO rows (none when $2 is NULL: PO lines without a cost code are not on the page) UNION ALL petty cash
	// rows, one query. src_ord keeps PO first.
	rows, err := h.db.Query(ctx, `
		SELECT x.po_id, x.po_no, x.project_code, x.project_name, x.line_no, x.mat_code, x.item_name,
		       x.quantity, x.unit, x.remarks, x.po_status, x.amount, x.source, x.doc_project_code
		FROM (
			SELECT 0 AS src_ord, 'PO' AS source, NULL::text AS doc_project_code, po.id AS po_id, po.po_no, po.project_code, pj.project_name, pol.line_no, pol.mat_code,
			       COALESCE(
			           NULLIF(TRIM(CONCAT_WS(' ', mn.mat_name, ss.spec_description)), ''),
			           NULLIF(TRIM(pol.description), '')
			       ) AS item_name,
			       pol.qty_ordered::float8 AS quantity, un.unit_name AS unit, NULLIF(TRIM(pol.remarks), '') AS remarks,
			       po.status AS po_status,
			       (pol.qty_ordered * pol.unit_price)::float8 AS amount
			FROM purchase_order_line pol
			JOIN purchase_order po ON po.id = pol.po_id
			LEFT JOIN project pj ON pj.project_code = po.project_code
			LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
			LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
			LEFT JOIN spec_size ss ON ss.id = mc.spec_id
			LEFT JOIN unit un ON un.id = mc.unit_id
			WHERE po.project_code = $1 AND pol.cost_subgroup_id = $2::bigint
			  AND po.deleted_at IS NULL
			  AND po.status NOT IN ('DRAFT','REJECTED','CANCELLED')
			  AND pol.status <> 'CANCELLED'
			UNION ALL
			SELECT 1, 'PC', NULL::text, h.id, h.pc_no, l.project_code, pj.project_name, l.line_no, l.mat_code,
			       COALESCE(
			           NULLIF(TRIM(l.description), ''),
			           NULLIF(TRIM(CONCAT_WS(' ', mn.mat_name, ss.spec_description)), '')
			       ),
			       l.qty::float8, un.unit_name, NULLIF(TRIM(l.remarks), ''),
			       h.status,
			       `+pcExVatSQL+`::float8
			FROM petty_cash_requisition_line l
			JOIN petty_cash_requisition h ON h.id = l.pc_id
			LEFT JOIN project pj ON pj.project_code = l.project_code
			LEFT JOIN material_code mc ON mc.mat_code = l.mat_code
			LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
			LEFT JOIN spec_size ss ON ss.id = mc.spec_id
			LEFT JOIN unit un ON un.id = mc.unit_id
			WHERE h.status = 'APPROVED' AND l.project_code = $1
			  AND ((l.cost_subgroup_id IS NULL AND $2::bigint IS NULL) OR l.cost_subgroup_id = $2::bigint)
			UNION ALL
			-- Project transfers (POSTED) booked on this cost code. Coming in: counterpart = the source project;
			-- going out: counterpart = the destination project, quantity / amount negative.
			SELECT 2, 'TR', m.project_code, m.id, m.doc_no,
			       CASE WHEN l.to_project_code = $1 THEN m.project_code ELSE l.to_project_code END,
			       CASE WHEN l.to_project_code = $1 THEN sp.project_name ELSE dp.project_name END,
			       l.line_no, l.mat_code,
			       NULLIF(TRIM(CONCAT_WS(' ', NULLIF(TRIM(mn.mat_name), ''), NULLIF(TRIM(ss.spec_description), ''), NULLIF(REGEXP_REPLACE(TRIM(br.brand_name), '^-+$', ''), ''))), ''),
			       (CASE WHEN l.to_project_code = $1 THEN l.qty ELSE -l.qty END)::float8, un.unit_name,
			       NULLIF(TRIM(COALESCE(l.remarks, m.remarks)), ''), m.status,
			       (CASE WHEN l.to_project_code = $1 THEN l.qty ELSE -l.qty END * COALESCE(t.unit_cost, 0))::float8
			FROM ic_project_movement_line l
			JOIN ic_project_movement m ON m.id = l.movement_id AND m.doc_type = 'TRANSFER' AND m.status = 'POSTED'
			JOIN material_code mc ON mc.mat_code = l.mat_code
			LEFT JOIN project sp ON sp.project_code = m.project_code
			LEFT JOIN project dp ON dp.project_code = l.to_project_code
			LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
			LEFT JOIN spec_size ss ON ss.id = mc.spec_id
			LEFT JOIN brand br ON br.id = mc.brand_id
			LEFT JOIN unit un ON un.id = mc.unit_id
			LEFT JOIN LATERAL (SELECT unit_cost FROM ic_project_cost_item_transaction
			                    WHERE movement_line_id = l.id ORDER BY id LIMIT 1) t ON true
			WHERE $2::bigint IS NOT NULL
			  AND ((l.to_project_code = $1 AND l.to_cost_subgroup_id = $2::bigint)
			    OR (m.project_code = $1 AND COALESCE(NULLIF(l.cost_subgroup_id, 0), mc.cost_subgroup_id) = $2::bigint))
		) x
		ORDER BY x.src_ord, x.po_no, x.line_no`, projectCode, subgroupID)
	if err != nil {
		log.Printf("cost-budget po-lines query project=%s sub=%v: %v", projectCode, subgroupID, err)
		return fiber.NewError(fiber.StatusInternalServerError, loadErr)
	}
	defer rows.Close()

	out := []costBudgetPOLine{}
	for rows.Next() {
		var l costBudgetPOLine
		if err := rows.Scan(&l.POID, &l.PONo, &l.ProjectCode, &l.ProjectName, &l.LineNo, &l.MatCode, &l.ItemName,
			&l.Quantity, &l.Unit, &l.Remarks, &l.POStatus, &l.Amount, &l.Source, &l.DocProjectCode); err != nil {
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

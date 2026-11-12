package handlers

import (
	"context"
	"errors"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

type costBudgetDetailProject struct {
	ProjectCode  string  `json:"project_code"`
	ProjectName  string  `json:"project_name"`
	Status       string  `json:"status"`
	Contract     *string `json:"contract"`
	Contact      *string `json:"contact"`
	OwnerName    *string `json:"owner_name"`
	BudgetAmount float64 `json:"budget_amount"`
	StartDate    *string `json:"start_date"`
	EndDate      *string `json:"end_date"`
}

// pcExVatSQL is the petty cash amount used by the Cost Budget page and the project overview: the line
// base less its discount, ex-VAT and ex-WHT (amount is nullable in the schema, so it falls back to
// qty * unit_price). It expects the petty_cash_requisition_line alias `l`. GET /master/projects
// spent_amount (pcProjectSpentSQL) still uses line_net (VAT-inclusive, net of WHT) — a different basis.
const pcExVatSQL = `(COALESCE(l.amount, l.qty * l.unit_price) - l.line_discount)`

// woLineCostSubgroupJoin maps a work_order_line (alias `wol`) to its cost_subgroup as alias `wom`
// (wom.cost_subgroup_id). A WO line stores the COMBINED cost code text (e.g. LP30303), not an id, so the code is
// rebuilt from the cost tree (costCodeSQL) and matched; one line maps to exactly one non-Head, non-Major subgroup
// (lowest id if the text were ever ambiguous), so a line is never counted twice. A line whose code matches no
// subgroup is simply not charged to any cost code. Only APPROVED work orders are counted by the callers.
func woLineCostSubgroupJoin() string {
	return `JOIN LATERAL (
			SELECT cs2.id AS cost_subgroup_id
			FROM cost_subgroup cs2
			JOIN cost_group    cg2  ON cg2.id  = cs2.group_id
			JOIN cost_job      cj2  ON cj2.id  = cg2.job_id
			JOIN cost_subject  csb2 ON csb2.id = cj2.subject_id
			WHERE NOT cs2.is_head AND NOT cs2.is_major
			  AND ` + costCodeSQL("csb2", "cj2", "cg2", "cs2") + ` = wol.cost_code
			ORDER BY cs2.id LIMIT 1
		) wom ON true`
}

// costBudgetUnassignedLabel is the description of the synthetic row that holds APPROVED petty cash
// lines of the project that have no cost code.
const costBudgetUnassignedLabel = "ไม่ระบุรหัสต้นทุน"

type costBudgetDetailRow struct {
	RowType      string  `json:"row_type"`    // MAJOR | HEAD | ITEM
	SubgroupID   *int64  `json:"subgroup_id"` // null on the unassigned petty cash row
	IsUnassigned bool    `json:"is_unassigned"`
	CostCode     string  `json:"cost_code"`
	Description  *string `json:"description"`
	Budget       float64 `json:"budget"`
	PoCost       float64 `json:"po_cost"`
	PcCost       float64 `json:"pc_cost"`
	PuCost       float64 `json:"pu_cost"` // po_cost + pc_cost
	PuBal        float64 `json:"pu_bal"`
	AcCost       float64 `json:"ac_cost"`
	AcBal        float64 `json:"ac_bal"`
}

type costBudgetDetailTotals struct {
	Budget float64 `json:"budget"`
	PoCost float64 `json:"po_cost"`
	PcCost float64 `json:"pc_cost"`
	PuCost float64 `json:"pu_cost"`
	PuBal  float64 `json:"pu_bal"`
	AcCost float64 `json:"ac_cost"`
	AcBal  float64 `json:"ac_bal"`
}

type costBudgetPurchaseLine struct {
	PONo       string  `json:"po_no"`
	PRNo       *string `json:"pr_no"`
	MatCode    *string `json:"mat_code"`
	ItemName   *string `json:"item_name"`
	QtyOrdered float64 `json:"qty_ordered"`
	UnitPrice  float64 `json:"unit_price"`
}

// CostBudgetDetail godoc
// @Summary      Cost Budget modal — project header + per-cost-code rows
// @Description  Flat ordered list with row_type MAJOR | HEAD | ITEM. ITEM rows (NOT is_head AND NOT is_major) are the
// @Description  subgroups that have a PO line in the project (POs not DRAFT/REJECTED/CANCELLED/deleted; order_type is NOT
// @Description  filtered), a project_cost_budget row, OR an APPROVED petty cash line charged to that cost code. budget comes from
// @Description  project_cost_budget (LEFT JOIN, no row => 0). po_cost = SUM(qty_ordered * unit_price); pc_cost = SUM(amount -
// @Description  line_discount) of APPROVED petty cash lines (header status APPROVED, the LINE's project_code = this project) —
// @Description  EX-VAT/ex-WHT, unlike spent_amount in GET /master/projects which is line_net (VAT-inclusive). pu_cost = po_cost + pc_cost.
// @Description  ac_cost = what the finance page has paid (SAVED contractor-log bills of PO / WO / petty cash, before VAT / WHT, spread over the document's lines by value) plus the POSTED transfer amounts (cost moves with a transfer, like pu_cost). HEAD = sum of its group's ITEM rows, MAJOR = sum of all ITEM rows of its job
// @Description  (computed in the backend; a roll-up row appears only when it has at least one ITEM under it). Order per job: MAJOR,
// @Description  then each group's HEAD followed by its ITEMs. APPROVED petty cash lines with no cost code are summed into one last
// @Description  synthetic ITEM row (is_unassigned=true, subgroup_id null, description "ไม่ระบุรหัสต้นทุน", only when the amount is not 0);
// @Description  it has no Head/Major roll-up and no budget. totals (budget, po_cost, pc_cost, pu_cost, ...) = sum of all ITEM rows
// @Description  including the unassigned row, so pc_cost reconciles with the project-level petty cash total (ex-VAT).
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path  string  true  "Project code"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/{projectCode}/cost-budget-detail [get]
func (h *POHandler) CostBudgetDetail(c *fiber.Ctx) error {
	projectCode := strings.TrimSpace(c.Params("projectCode"))
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "project code is required")
	}
	if err := requireProjectAccess(c, h.db, projectCode); err != nil {
		return err
	}
	ctx := context.Background()

	var p costBudgetDetailProject
	var projectID int64
	err := h.db.QueryRow(ctx, `
		SELECT p.id, p.project_code, p.project_name, p.status,
		       p.responsible_person_name,
		       COALESCE(NULLIF(p.owner_name, ''), cu.customer_name),
		       p.budget_amount::float8, p.start_date::text, p.end_date::text
		FROM project p
		LEFT JOIN customer cu ON cu.cus_id = p.customer_id
		WHERE p.project_code = $1`, projectCode,
	).Scan(&projectID, &p.ProjectCode, &p.ProjectName, &p.Status, &p.Contact, &p.OwnerName,
		&p.BudgetAmount, &p.StartDate, &p.EndDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	if err != nil {
		return err
	}
	// p.Contract stays nil: no source field for contract yet.

	// ITEM rows only (NOT is_head AND NOT is_major): every subgroup that has a PO line in this
	// project OR a project_cost_budget row. Budget is a LEFT JOIN — no row => 0. The displayed
	// cost code is output only; every filter/join below uses ids.
	rows, err := h.db.Query(ctx, `
		WITH po_raw AS (
			SELECT pol.cost_subgroup_id,
			       SUM(pol.qty_ordered * pol.unit_price)::float8 AS po_cost
			FROM purchase_order_line pol
			JOIN purchase_order po ON po.id = pol.po_id
			WHERE po.project_code = $1
			  AND po.deleted_at IS NULL
			  AND po.status NOT IN ('DRAFT','REJECTED','CANCELLED')
			  AND pol.status <> 'CANCELLED'
			  AND pol.cost_subgroup_id IS NOT NULL
			GROUP BY pol.cost_subgroup_id
		), wo_raw AS (
			-- APPROVED work orders of the project: qty * unit_price per line (same basis as PO), charged to the
			-- cost code written on the line. Counted in po_cost together with PO and transfers.
			SELECT wom.cost_subgroup_id,
			       SUM(wol.qty * wol.unit_price)::float8 AS wo_cost
			FROM work_order_line wol
			JOIN work_order wo ON wo.id = wol.wo_id AND wo.project_code = $1 AND wo.status = 'APPROVED'
			`+woLineCostSubgroupJoin()+`
			GROUP BY wom.cost_subgroup_id
		), tr_amt AS (
			-- POSTED TRANSFER lines priced at the unit cost recorded when the document was posted:
			-- + on the destination project's cost code, - on the source project's cost code (a
			-- warehouse source with no cost code on the line falls back to the material's own).
			SELECT l.to_project_code AS project_code, l.to_cost_subgroup_id AS cost_subgroup_id,
			       (l.qty * t.unit_cost)::float8 AS amt
			FROM ic_project_movement_line l
			JOIN ic_project_movement m ON m.id = l.movement_id AND m.doc_type = 'TRANSFER' AND m.status = 'POSTED'
			JOIN LATERAL (SELECT unit_cost FROM ic_project_cost_item_transaction
			               WHERE movement_line_id = l.id ORDER BY id LIMIT 1) t ON true
			WHERE l.to_cost_subgroup_id IS NOT NULL AND l.to_cost_subgroup_id > 0
			UNION ALL
			SELECT m.project_code, COALESCE(NULLIF(l.cost_subgroup_id, 0), mc.cost_subgroup_id),
			       -(l.qty * t.unit_cost)::float8
			FROM ic_project_movement_line l
			JOIN ic_project_movement m ON m.id = l.movement_id AND m.doc_type = 'TRANSFER' AND m.status = 'POSTED'
			JOIN material_code mc ON mc.mat_code = l.mat_code
			JOIN LATERAL (SELECT unit_cost FROM ic_project_cost_item_transaction
			               WHERE movement_line_id = l.id ORDER BY id LIMIT 1) t ON true
		), po_c AS (
			SELECT cost_subgroup_id, SUM(po_cost)::float8 AS po_cost
			FROM (
				SELECT cost_subgroup_id, po_cost FROM po_raw
				UNION ALL
				SELECT cost_subgroup_id, wo_cost FROM wo_raw
				UNION ALL
				SELECT cost_subgroup_id, amt FROM tr_amt WHERE project_code = $1 AND cost_subgroup_id IS NOT NULL
			) x
			GROUP BY cost_subgroup_id
		), pc AS (
			SELECT l.cost_subgroup_id,
			       SUM(`+pcExVatSQL+`)::float8 AS pc_cost
			FROM petty_cash_requisition_line l
			JOIN petty_cash_requisition h ON h.id = l.pc_id
			WHERE h.status = 'APPROVED'
			  AND l.project_code = $1
			  AND l.cost_subgroup_id IS NOT NULL
			GROUP BY l.cost_subgroup_id
		), bp_po AS (
			-- Ac.Cost = what the finance page has paid: the SAVED contractor-log bills of each document
			-- (ADV + PROGRESS - ADV_DEDUCT - RETENTION held back, + RETENTION paid out later), before VAT and before withholding tax.
			SELECT lg.po_id AS doc_id, SUM(bl.amount * `+billSignSQL+`) AS paid
			FROM wo_contractor_log lg
			JOIN wo_contractor_log_bill b ON b.log_id = lg.id AND b.status = 'SAVED'
			JOIN wo_contractor_log_bill_line bl ON bl.bill_id = b.id AND bl.line_code IN ('ADV', 'PROGRESS', 'ADV_DEDUCT', 'RETENTION')
			WHERE lg.po_id IS NOT NULL
			GROUP BY lg.po_id
		), bp_wo AS (
			SELECT lg.wo_id AS doc_id, SUM(bl.amount * `+billSignSQL+`) AS paid
			FROM wo_contractor_log lg
			JOIN wo_contractor_log_bill b ON b.log_id = lg.id AND b.status = 'SAVED'
			JOIN wo_contractor_log_bill_line bl ON bl.bill_id = b.id AND bl.line_code IN ('ADV', 'PROGRESS', 'ADV_DEDUCT', 'RETENTION')
			WHERE lg.wo_id IS NOT NULL
			GROUP BY lg.wo_id
		), bp_pc AS (
			SELECT lg.pc_id AS doc_id, SUM(bl.amount * `+billSignSQL+`) AS paid
			FROM wo_contractor_log lg
			JOIN wo_contractor_log_bill b ON b.log_id = lg.id AND b.status = 'SAVED'
			JOIN wo_contractor_log_bill_line bl ON bl.bill_id = b.id AND bl.line_code IN ('ADV', 'PROGRESS', 'ADV_DEDUCT', 'RETENTION')
			WHERE lg.pc_id IS NOT NULL
			GROUP BY lg.pc_id
		), ac_po AS (
			-- a PO's payment is spread over its lines by each line's share of the PO's value
			SELECT x.cost_subgroup_id, SUM(x.share)::float8 AS ac
			FROM (
				SELECT pol.cost_subgroup_id,
				       CASE WHEN SUM(pol.qty_ordered * pol.unit_price) OVER (PARTITION BY po.id) > 0
				            THEN pol.qty_ordered * pol.unit_price * bp.paid / SUM(pol.qty_ordered * pol.unit_price) OVER (PARTITION BY po.id)
				            ELSE 0 END AS share
				FROM purchase_order po
				JOIN purchase_order_line pol ON pol.po_id = po.id AND pol.status <> 'CANCELLED'
				JOIN bp_po bp ON bp.doc_id = po.id
				WHERE po.project_code = $1 AND po.deleted_at IS NULL
				  AND po.status NOT IN ('DRAFT','REJECTED','CANCELLED')
			) x
			WHERE x.cost_subgroup_id IS NOT NULL
			GROUP BY x.cost_subgroup_id
		), ac_wo AS (
			SELECT wom.cost_subgroup_id,
			       SUM(wol.qty * wol.unit_price * bp.paid / NULLIF(t.tot, 0))::float8 AS ac
			FROM work_order wo
			JOIN bp_wo bp ON bp.doc_id = wo.id
			JOIN LATERAL (SELECT SUM(l2.qty * l2.unit_price) AS tot FROM work_order_line l2 WHERE l2.wo_id = wo.id) t ON true
			JOIN work_order_line wol ON wol.wo_id = wo.id
			`+woLineCostSubgroupJoin()+`
			WHERE wo.project_code = $1 AND wo.status = 'APPROVED'
			GROUP BY wom.cost_subgroup_id
		), ac_pc AS (
			SELECT y.cost_subgroup_id, SUM(y.share)::float8 AS ac
			FROM (
				SELECT l.project_code, l.cost_subgroup_id,
				       CASE WHEN SUM(`+pcExVatSQL+`) OVER (PARTITION BY h.id) > 0
				            THEN (`+pcExVatSQL+`) * bp.paid / SUM(`+pcExVatSQL+`) OVER (PARTITION BY h.id)
				            ELSE 0 END AS share
				FROM petty_cash_requisition h
				JOIN petty_cash_requisition_line l ON l.pc_id = h.id
				JOIN bp_pc bp ON bp.doc_id = h.id
				WHERE h.status = 'APPROVED'
			) y
			WHERE y.project_code = $1 AND y.cost_subgroup_id IS NOT NULL
			GROUP BY y.cost_subgroup_id
		), ac_c AS (
			-- Transfers move cost, so the same POSTED transfer amounts used for Pu.Cost (+ destination, - source)
			-- move Ac.Cost too.
			SELECT cost_subgroup_id, SUM(ac)::float8 AS ac_cost
			FROM (
				SELECT cost_subgroup_id, ac FROM ac_po
				UNION ALL
				SELECT cost_subgroup_id, ac FROM ac_wo
				UNION ALL
				SELECT cost_subgroup_id, ac FROM ac_pc
				UNION ALL
				SELECT cost_subgroup_id, amt FROM tr_amt WHERE project_code = $1 AND cost_subgroup_id IS NOT NULL
			) z
			GROUP BY cost_subgroup_id
		), keys AS (
			SELECT cost_subgroup_id FROM po_c
			UNION
			SELECT cost_subgroup_id FROM pc
			UNION
			SELECT cost_subgroup_id FROM ac_c
			UNION
			SELECT cost_subgroup_id FROM project_cost_budget WHERE project_id = $2
		)
		SELECT cs.id, cs.group_id, cg.job_id,
		       csb.subject_code, cj.job_code, cg.group_code, cs.subgroup_code,
		       `+costCodeSQL("csb", "cj", "cg", "cs")+` AS cost_code,
		       cs.subgroup_name,
		       COALESCE(b.budget_amount, 0)::float8,
		       COALESCE(po_c.po_cost, 0)::float8,
		       COALESCE(pc.pc_cost, 0)::float8,
		       COALESCE(ac_c.ac_cost, 0)::float8
		FROM keys k
		JOIN cost_subgroup cs  ON cs.id  = k.cost_subgroup_id
		JOIN cost_group    cg  ON cg.id  = cs.group_id
		JOIN cost_job      cj  ON cj.id  = cg.job_id
		JOIN cost_subject  csb ON csb.id = cj.subject_id
		LEFT JOIN po_c ON po_c.cost_subgroup_id = cs.id
		LEFT JOIN pc ON pc.cost_subgroup_id = cs.id
		LEFT JOIN ac_c ON ac_c.cost_subgroup_id = cs.id
		LEFT JOIN project_cost_budget b ON b.project_id = $2 AND b.cost_subgroup_id = cs.id
		WHERE NOT cs.is_head AND NOT cs.is_major`, projectCode, projectID)
	if err != nil {
		return err
	}
	defer rows.Close()

	groups := map[int64]*cbGroup{}
	jobs := map[int64]*cbJob{}
	var totals costBudgetDetailTotals
	for rows.Next() {
		var n cbNode
		var groupID, jobID int64
		if err := rows.Scan(&n.subgroupID, &groupID, &jobID,
			&n.subjectCode, &n.jobCode, &n.groupCode, &n.subgroupCode,
			&n.costCode, &n.name, &n.budget, &n.po, &n.pc, &n.ac); err != nil {
			return err
		}
		n.pu = n.po + n.pc
		g := groups[groupID]
		if g == nil {
			g = &cbGroup{groupCode: n.groupCode}
			groups[groupID] = g
		}
		g.items = append(g.items, n)
		j := jobs[jobID]
		if j == nil {
			j = &cbJob{subjectCode: n.subjectCode, jobCode: n.jobCode}
			jobs[jobID] = j
		}
		j.groupIDs = appendUniqueInt64(j.groupIDs, groupID)
		j.sum.add(n)
		g.sum.add(n)
		totals.Budget += n.budget
		totals.PoCost += n.po
		totals.PcCost += n.pc
		totals.PuCost += n.pu
		totals.AcCost += n.ac
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	// Head / Major nodes for the groups and jobs that have at least one ITEM row (so a roll-up
	// row never shows without an ITEM under it).
	groupIDs := make([]int64, 0, len(groups))
	for id := range groups {
		groupIDs = append(groupIDs, id)
	}
	jobIDs := make([]int64, 0, len(jobs))
	for id := range jobs {
		jobIDs = append(jobIDs, id)
	}
	hrows, err := h.db.Query(ctx, `
		SELECT cs.id, cs.group_id, cg.job_id, cs.is_head, cs.is_major,
		       csb.subject_code, cj.job_code, cg.group_code, cs.subgroup_code,
		       `+costCodeSQL("csb", "cj", "cg", "cs")+` AS cost_code,
		       cs.subgroup_name
		FROM cost_subgroup cs
		JOIN cost_group    cg  ON cg.id  = cs.group_id
		JOIN cost_job      cj  ON cj.id  = cg.job_id
		JOIN cost_subject  csb ON csb.id = cj.subject_id
		WHERE (cs.is_head  AND cs.group_id = ANY($1::bigint[]))
		   OR (cs.is_major AND cg.job_id   = ANY($2::bigint[]))`, groupIDs, jobIDs)
	if err != nil {
		return err
	}
	defer hrows.Close()
	for hrows.Next() {
		var n cbNode
		var groupID, jobID int64
		var isHead, isMajor bool
		if err := hrows.Scan(&n.subgroupID, &groupID, &jobID, &isHead, &isMajor,
			&n.subjectCode, &n.jobCode, &n.groupCode, &n.subgroupCode, &n.costCode, &n.name); err != nil {
			return err
		}
		if isMajor {
			if j := jobs[jobID]; j != nil {
				j.majors = append(j.majors, n)
			}
		} else if isHead {
			if g := groups[groupID]; g != nil {
				g.heads = append(g.heads, n)
			}
		}
	}
	if err := hrows.Err(); err != nil {
		return err
	}

	// Budget of a whole job = the project value * the job's share % (project_job_share, set on the project
	// form). It is shown on the job's MAJOR row; HEAD / ITEM rows keep their own (cost-code level) budget.
	jobBudgets, err := h.loadJobShareBudgets(ctx, projectCode)
	if err != nil {
		log.Printf("cost-budget-detail job shares project=%s: %v", projectCode, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดข้อมูลงบประมาณต้นทุนได้ กรุณาลองใหม่อีกครั้ง")
	}

	// Flat ordered list per job: MAJOR, then each group's HEAD followed by its ITEMs.
	jobOrder := make([]*cbJob, 0, len(jobs))
	for _, j := range jobs {
		jobOrder = append(jobOrder, j)
	}
	sort.Slice(jobOrder, func(a, b int) bool {
		if jobOrder[a].subjectCode != jobOrder[b].subjectCode {
			return jobOrder[a].subjectCode < jobOrder[b].subjectCode
		}
		return jobOrder[a].jobCode < jobOrder[b].jobCode
	})

	result := []costBudgetDetailRow{}
	for _, j := range jobOrder {
		majorSum := j.sum
		if b, ok := jobBudgets[j.subjectCode+j.jobCode]; ok {
			majorSum.budget = b // the page total stays the whole project value (below), not the sum of these
		}
		for _, m := range j.majors {
			result = append(result, cbRow("MAJOR", m, majorSum))
		}
		gs := make([]*cbGroup, 0, len(j.groupIDs))
		for _, gid := range j.groupIDs {
			gs = append(gs, groups[gid])
		}
		sort.Slice(gs, func(a, b int) bool { return gs[a].groupCode < gs[b].groupCode })
		for _, g := range gs {
			for _, hd := range g.heads {
				result = append(result, cbRow("HEAD", hd, g.sum))
			}
			sort.Slice(g.items, func(a, b int) bool { return g.items[a].subgroupCode < g.items[b].subgroupCode })
			for _, it := range g.items {
				result = append(result, cbRow("ITEM", it, cbSum{budget: it.budget, po: it.po, pc: it.pc, pu: it.pu, ac: it.ac}))
			}
		}
	}

	// Synthetic row for APPROVED petty cash lines of this project that carry no cost code. It is not a
	// cost_subgroup (no Head/Major roll-up, no budget), only shown when its amount is not 0, and counted
	// in the page total so the page reconciles with the project-level petty cash total.
	var unassigned float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(`+pcExVatSQL+`), 0)::float8
		FROM petty_cash_requisition_line l
		JOIN petty_cash_requisition h ON h.id = l.pc_id
		WHERE h.status = 'APPROVED'
		  AND l.project_code = $1
		  AND l.cost_subgroup_id IS NULL`, projectCode).Scan(&unassigned); err != nil {
		log.Printf("cost-budget-detail unassigned petty cash project=%s: %v", projectCode, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดข้อมูลงบประมาณต้นทุนได้ กรุณาลองใหม่อีกครั้ง")
	}
	// Paid share of those same petty cash lines (their document's SAVED bills spread by line value).
	var unassignedAc float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(y.share), 0)::float8 FROM (
			SELECT l.project_code, l.cost_subgroup_id,
			       CASE WHEN SUM(`+pcExVatSQL+`) OVER (PARTITION BY h.id) > 0
			            THEN (`+pcExVatSQL+`) * bp.paid / SUM(`+pcExVatSQL+`) OVER (PARTITION BY h.id)
			            ELSE 0 END AS share
			FROM petty_cash_requisition h
			JOIN petty_cash_requisition_line l ON l.pc_id = h.id
			JOIN (
				SELECT lg.pc_id AS doc_id, SUM(bl.amount * `+billSignSQL+`) AS paid
				FROM wo_contractor_log lg
				JOIN wo_contractor_log_bill b ON b.log_id = lg.id AND b.status = 'SAVED'
				JOIN wo_contractor_log_bill_line bl ON bl.bill_id = b.id AND bl.line_code IN ('ADV', 'PROGRESS', 'ADV_DEDUCT', 'RETENTION')
				WHERE lg.pc_id IS NOT NULL
				GROUP BY lg.pc_id
			) bp ON bp.doc_id = h.id
			WHERE h.status = 'APPROVED'
		) y
		WHERE y.project_code = $1 AND y.cost_subgroup_id IS NULL`, projectCode).Scan(&unassignedAc); err != nil {
		log.Printf("cost-budget-detail unassigned petty cash paid project=%s: %v", projectCode, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดข้อมูลงบประมาณต้นทุนได้ กรุณาลองใหม่อีกครั้ง")
	}
	unassignedAc = cbRound(unassignedAc)
	if unassigned = cbRound(unassigned); unassigned != 0 || unassignedAc != 0 {
		label := costBudgetUnassignedLabel
		result = append(result, costBudgetDetailRow{
			RowType: "ITEM", SubgroupID: nil, IsUnassigned: true, CostCode: "", Description: &label,
			PcCost: unassigned, PuCost: unassigned, PuBal: cbRound(-unassigned),
			AcCost: unassignedAc, AcBal: cbRound(-unassignedAc),
		})
		totals.PcCost += unassigned
		totals.PuCost += unassigned
		totals.AcCost += unassignedAc
	}

	// Grand total: costs = ITEM rows (incl. the unassigned row), never the MAJOR / HEAD roll-ups. Budget = the
	// project's whole value (vo_amount for VO projects, else budget_amount), not just the jobs shown here; when the
	// project has no value set it stays the sum of the item budgets.
	var projectValue float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(CASE WHEN project_kind = 'VO' THEN vo_amount ELSE budget_amount END, 0)::float8
		FROM project WHERE project_code = $1`, projectCode).Scan(&projectValue); err != nil {
		log.Printf("cost-budget-detail project value project=%s: %v", projectCode, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดข้อมูลงบประมาณต้นทุนได้ กรุณาลองใหม่อีกครั้ง")
	}
	if projectValue > 0 {
		totals.Budget = projectValue
	}
	totals.Budget = cbRound(totals.Budget)
	totals.PoCost = cbRound(totals.PoCost)
	totals.PcCost = cbRound(totals.PcCost)
	totals.PuCost = cbRound(totals.PuCost)
	totals.AcCost = cbRound(totals.AcCost)
	totals.PuBal = cbRound(totals.Budget - totals.PuCost)
	totals.AcBal = cbRound(totals.Budget - totals.AcCost)

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"project": p,
			"rows":    result,
			"totals":  totals,
		},
	})
}

// cbNode is one cost_subgroup row (ITEM / HEAD / MAJOR) with the sort keys used to order the list.
type cbNode struct {
	subgroupID                                    int64
	subjectCode, jobCode, groupCode, subgroupCode string
	costCode                                      string
	name                                          *string
	budget, po, pc, pu, ac                        float64
}

type cbSum struct{ budget, po, pc, pu, ac float64 }

func (s *cbSum) add(n cbNode) {
	s.budget += n.budget
	s.po += n.po
	s.pc += n.pc
	s.pu += n.pu
	s.ac += n.ac
}

type cbGroup struct {
	groupCode string
	items     []cbNode
	heads     []cbNode
	sum       cbSum
}

type cbJob struct {
	subjectCode, jobCode string
	groupIDs             []int64
	majors               []cbNode
	sum                  cbSum
}

func appendUniqueInt64(xs []int64, v int64) []int64 {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}

// cbRound trims float noise from the backend roll-up sums (budget is numeric(18,2)).
func cbRound(v float64) float64 { return math.Round(v*10000) / 10000 }

func cbRow(rowType string, n cbNode, s cbSum) costBudgetDetailRow {
	budget, po, pc, pu, ac := cbRound(s.budget), cbRound(s.po), cbRound(s.pc), cbRound(s.pu), cbRound(s.ac)
	id := n.subgroupID
	return costBudgetDetailRow{
		RowType:     rowType,
		SubgroupID:  &id,
		CostCode:    n.costCode,
		Description: n.name,
		Budget:      budget,
		PoCost:      po,
		PcCost:      pc,
		PuCost:      pu,
		PuBal:       cbRound(budget - pu),
		AcCost:      ac,
		AcBal:       cbRound(budget - ac),
	}
}

// CostBudgetDetailPurchases godoc
// @Summary      Cost Budget modal — purchase lines behind one cost code
// @Description  PO lines (with optional PR number) for one project + cost_subgroup, same status filters as the detail endpoint.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        projectCode  path  string  true  "Project code"
// @Param        subgroupId   path  int     true  "cost_subgroup.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /po/{projectCode}/cost-budget-detail/{subgroupId}/purchases [get]
func (h *POHandler) CostBudgetDetailPurchases(c *fiber.Ctx) error {
	projectCode := strings.TrimSpace(c.Params("projectCode"))
	if projectCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "project code is required")
	}
	if err := requireProjectAccess(c, h.db, projectCode); err != nil {
		return err
	}
	subgroupID, err := strconv.ParseInt(c.Params("subgroupId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "subgroupId must be a number")
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT po.po_no, pr.pr_no, pol.mat_code,
		       COALESCE(
		           NULLIF(TRIM(CONCAT_WS(' ', mn.mat_name, ss.spec_description)), ''),
		           NULLIF(TRIM(pol.description), '')
		       ) AS item_name,
		       pol.qty_ordered::float8, pol.unit_price::float8
		FROM purchase_order_line pol
		JOIN purchase_order po ON po.id = pol.po_id
		LEFT JOIN purchase_request_line prl ON prl.id = pol.pr_line_id
		LEFT JOIN purchase_request pr ON pr.id = prl.pr_id
		LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
		LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size ss ON ss.id = mc.spec_id
		WHERE po.project_code = $1 AND pol.cost_subgroup_id = $2
		  AND po.deleted_at IS NULL
		  AND po.status NOT IN ('DRAFT','REJECTED','CANCELLED')
		  AND pol.status <> 'CANCELLED'
		ORDER BY po.po_date DESC, po.id DESC, pol.line_no`, projectCode, subgroupID)
	if err != nil {
		return err
	}
	defer rows.Close()

	result := []costBudgetPurchaseLine{}
	for rows.Next() {
		var l costBudgetPurchaseLine
		if err := rows.Scan(&l.PONo, &l.PRNo, &l.MatCode, &l.ItemName, &l.QtyOrdered, &l.UnitPrice); err != nil {
			return err
		}
		result = append(result, l)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": result})
}

// jobShareCostKey maps a project job code (MS, EE, ..., G) to the cost_subject.subject_code || cost_job.job_code
// key of its MAJOR row (same mapping as the SQL function job_cost_key; General Code lives under subject OH).
func jobShareCostKey(code string) string {
	switch code {
	case "MS":
		return "MP"
	case "EE":
		return "ME"
	case "SN":
		return "MS"
	case "FF":
		return "MF"
	case "GS":
		return "MG"
	case "AC":
		return "MH"
	case "CS":
		return "MC"
	case "AR":
		return "MA"
	case "G":
		return "OHG"
	}
	return code
}

// loadJobShareBudgets returns, per subject||job key, project value (vo_amount for VO projects, else
// budget_amount) * share_percent / 100 for every job that has a share on the project.
func (h *POHandler) loadJobShareBudgets(ctx context.Context, projectCode string) (map[string]float64, error) {
	rows, err := h.db.Query(ctx, `
		SELECT s.job_code,
		       (CASE WHEN p.project_kind = 'VO' THEN p.vo_amount ELSE p.budget_amount END * s.share_percent / 100)::float8
		FROM project_job_share s
		JOIN project p ON p.id = s.project_id
		WHERE p.project_code = $1`, projectCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var code string
		var amount float64
		if err := rows.Scan(&code, &amount); err != nil {
			return nil, err
		}
		out[jobShareCostKey(code)] = cbRound(amount)
	}
	return out, rows.Err()
}

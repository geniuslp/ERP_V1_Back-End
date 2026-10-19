package handlers

import (
	"context"
	"errors"
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

type costBudgetDetailRow struct {
	RowType     string  `json:"row_type"` // MAJOR | HEAD | ITEM
	SubgroupID  int64   `json:"subgroup_id"`
	CostCode    string  `json:"cost_code"`
	Description *string `json:"description"`
	Budget      float64 `json:"budget"`
	PuCost      float64 `json:"pu_cost"`
	PuBal       float64 `json:"pu_bal"`
	AcCost      float64 `json:"ac_cost"`
	AcBal       float64 `json:"ac_bal"`
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
// @Description  filtered) OR a project_cost_budget row. budget comes from project_cost_budget (LEFT JOIN, no row => 0);
// @Description  pu_cost = SUM(qty_ordered * unit_price); ac_cost is still a 0 placeholder. HEAD = sum of its group's ITEM
// @Description  rows, MAJOR = sum of all ITEM rows of its job (computed in the backend; a roll-up row appears only when it has
// @Description  at least one ITEM under it). Order per job: MAJOR, then each group's HEAD followed by its ITEMs.
// @Description  totals = sum of ITEM rows only.
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
		WITH pu AS (
			SELECT pol.cost_subgroup_id,
			       SUM(pol.qty_ordered * pol.unit_price)::float8 AS pu_cost
			FROM purchase_order_line pol
			JOIN purchase_order po ON po.id = pol.po_id
			WHERE po.project_code = $1
			  AND po.deleted_at IS NULL
			  AND po.status NOT IN ('DRAFT','REJECTED','CANCELLED')
			  AND pol.status <> 'CANCELLED'
			  AND pol.cost_subgroup_id IS NOT NULL
			GROUP BY pol.cost_subgroup_id
		), keys AS (
			SELECT cost_subgroup_id FROM pu
			UNION
			SELECT cost_subgroup_id FROM project_cost_budget WHERE project_id = $2
		)
		SELECT cs.id, cs.group_id, cg.job_id,
		       csb.subject_code, cj.job_code, cg.group_code, cs.subgroup_code,
		       `+costCodeSQL("csb", "cj", "cg", "cs")+` AS cost_code,
		       cs.subgroup_name,
		       COALESCE(b.budget_amount, 0)::float8,
		       COALESCE(pu.pu_cost, 0)::float8
		FROM keys k
		JOIN cost_subgroup cs  ON cs.id  = k.cost_subgroup_id
		JOIN cost_group    cg  ON cg.id  = cs.group_id
		JOIN cost_job      cj  ON cj.id  = cg.job_id
		JOIN cost_subject  csb ON csb.id = cj.subject_id
		LEFT JOIN pu ON pu.cost_subgroup_id = cs.id
		LEFT JOIN project_cost_budget b ON b.project_id = $2 AND b.cost_subgroup_id = cs.id
		WHERE NOT cs.is_head AND NOT cs.is_major`, projectCode, projectID)
	if err != nil {
		return err
	}
	defer rows.Close()

	groups := map[int64]*cbGroup{}
	jobs := map[int64]*cbJob{}
	var totals costBudgetTotals
	for rows.Next() {
		var n cbNode
		var groupID, jobID int64
		if err := rows.Scan(&n.subgroupID, &groupID, &jobID,
			&n.subjectCode, &n.jobCode, &n.groupCode, &n.subgroupCode,
			&n.costCode, &n.name, &n.budget, &n.pu); err != nil {
			return err
		}
		// TODO: ac_cost definition not confirmed yet; stays 0.
		n.ac = 0
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
		for _, m := range j.majors {
			result = append(result, cbRow("MAJOR", m, j.sum))
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
				result = append(result, cbRow("ITEM", it, cbSum{budget: it.budget, pu: it.pu, ac: it.ac}))
			}
		}
	}

	// Grand total = ITEM rows only (never the MAJOR / HEAD roll-ups).
	totals.Budget = cbRound(totals.Budget)
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
	budget, pu, ac                                float64
}

type cbSum struct{ budget, pu, ac float64 }

func (s *cbSum) add(n cbNode) {
	s.budget += n.budget
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
	budget, pu, ac := cbRound(s.budget), cbRound(s.pu), cbRound(s.ac)
	return costBudgetDetailRow{
		RowType:     rowType,
		SubgroupID:  n.subgroupID,
		CostCode:    n.costCode,
		Description: n.name,
		Budget:      budget,
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

package handlers

import (
	"context"
	"errors"
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
// @Description  One row per purchase_order_line.cost_subgroup_id for POs of the project that are not
// @Description  DRAFT/REJECTED/CANCELLED/deleted (order_type is NOT filtered). budget and ac_* are
// @Description  placeholders (0) until real sources exist. pu_cost = SUM(qty_ordered * unit_price).
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
	err := h.db.QueryRow(ctx, `
		SELECT p.project_code, p.project_name, p.status,
		       p.responsible_person_name,
		       COALESCE(NULLIF(p.owner_name, ''), cu.customer_name),
		       p.budget_amount::float8, p.start_date::text, p.end_date::text
		FROM project p
		LEFT JOIN customer cu ON cu.cus_id = p.customer_id
		WHERE p.project_code = $1`, projectCode,
	).Scan(&p.ProjectCode, &p.ProjectName, &p.Status, &p.Contact, &p.OwnerName,
		&p.BudgetAmount, &p.StartDate, &p.EndDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	if err != nil {
		return err
	}
	// p.Contract stays nil: no source field for contract yet.

	rows, err := h.db.Query(ctx, `
		SELECT pol.cost_subgroup_id,
		       csb.subject_code || cj.job_code || cg.group_code || cs.subgroup_code AS cost_code,
		       cs.subgroup_name,
		       SUM(pol.qty_ordered * pol.unit_price)::float8 AS pu_cost
		FROM purchase_order_line pol
		JOIN purchase_order po  ON po.id  = pol.po_id
		JOIN cost_subgroup  cs  ON cs.id  = pol.cost_subgroup_id
		JOIN cost_group     cg  ON cg.id  = cs.group_id
		JOIN cost_job       cj  ON cj.id  = cg.job_id
		JOIN cost_subject   csb ON csb.id = cj.subject_id
		WHERE po.project_code = $1
		  AND po.deleted_at IS NULL
		  AND po.status NOT IN ('DRAFT','REJECTED','CANCELLED')
		  AND pol.status <> 'CANCELLED'
		GROUP BY pol.cost_subgroup_id, csb.subject_code, cj.job_code,
		         cg.group_code, cs.subgroup_code, cs.subgroup_name
		ORDER BY cost_code`, projectCode)
	if err != nil {
		return err
	}
	defer rows.Close()

	result := []costBudgetDetailRow{}
	var totals costBudgetTotals
	for rows.Next() {
		var r costBudgetDetailRow
		if err := rows.Scan(&r.SubgroupID, &r.CostCode, &r.Description, &r.PuCost); err != nil {
			return err
		}
		// TODO: plug in a real per-cost-code budget source here once one exists.
		r.Budget = 0
		r.PuBal = r.Budget - r.PuCost
		// TODO: ac_cost definition not confirmed yet; stays 0.
		r.AcCost = 0
		r.AcBal = r.Budget - r.AcCost
		totals.Budget += r.Budget
		totals.PuCost += r.PuCost
		totals.PuBal += r.PuBal
		totals.AcCost += r.AcCost
		totals.AcBal += r.AcBal
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"project": p,
			"rows":    result,
			"totals":  totals,
		},
	})
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

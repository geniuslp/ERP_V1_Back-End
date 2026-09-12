package handlers

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectOverviewHandler struct{ db *pgxpool.Pool }

func NewProjectOverviewHandler(db *pgxpool.Pool) *ProjectOverviewHandler {
	return &ProjectOverviewHandler{db: db}
}

// Search godoc
// @Summary      Cross-entity search for the Project Overview page
// @Description  Searches projects (project_code/project_name) and approved PO line items (mat_code/cost_code) in one call. Returns a unified array, each row tagged with "type": "project" or "po_line". Requires q to be at least 2 characters — shorter queries return an empty result without hitting the DB. Each match type is capped at 20 rows, most-recently-created first.
// @Tags         Project Overview
// @Security     BearerAuth
// @Produce      json
// @Param        q  query  string  true  "Search term (min 2 chars)"
// @Success      200  {object}  fiber.Map
// @Router       /project-overview/search [get]
func (h *ProjectOverviewHandler) Search(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		return c.JSON(fiber.Map{"success": true, "data": []fiber.Map{}})
	}
	like := "%" + q + "%"
	ctx := context.Background()

	results := []fiber.Map{}

	projRows, err := h.db.Query(ctx, `
		SELECT p.project_code, p.project_name,
		       p.budget_amount,
		       COALESCE((SELECT SUM(po.net_amount) FROM purchase_order po
		           WHERE po.project_code = p.project_code AND po.status = 'APPROVED'), 0)
		       + COALESCE((SELECT SUM(wo.net_amount) FROM work_order wo
		           WHERE wo.project_code = p.project_code AND wo.status = 'APPROVED'), 0) AS spent_amount,
		       COALESCE((SELECT SUM(pl.amount_paid) FROM payment_log pl
		           JOIN purchase_order po ON po.id = pl.doc_id
		           WHERE pl.doc_type = 'PO' AND po.project_code = p.project_code), 0)
		       + COALESCE((SELECT SUM(pl.amount_paid) FROM payment_log pl
		           JOIN work_order wo ON wo.id = pl.doc_id
		           WHERE pl.doc_type = 'WO' AND wo.project_code = p.project_code), 0) AS paid_amount
		FROM project p
		WHERE p.project_code ILIKE $1 OR p.project_name ILIKE $1
		ORDER BY p.created_at DESC
		LIMIT 20`, like)
	if err != nil {
		return err
	}
	for projRows.Next() {
		var projectCode, projectName string
		var budgetAmount, spentAmount, paidAmount float64
		if err := projRows.Scan(&projectCode, &projectName, &budgetAmount, &spentAmount, &paidAmount); err != nil {
			projRows.Close()
			return err
		}
		results = append(results, fiber.Map{
			"type":             "project",
			"project_code":     projectCode,
			"project_name":     projectName,
			"budget_amount":    budgetAmount,
			"spent_amount":     spentAmount,
			"paid_amount":      paidAmount,
			"remaining_amount": budgetAmount - spentAmount,
		})
	}
	projRows.Close()

	lineRows, err := h.db.Query(ctx, `
		SELECT po.id, po.po_no, po.project_code, pj.project_name,
		       pol.mat_code, mn.mat_name,
		       csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code AS cost_code,
		       pol.qty_ordered, pol.unit_price, pol.amount
		FROM purchase_order_line pol
		JOIN purchase_order po ON po.id = pol.po_id
		LEFT JOIN project      pj   ON pj.project_code = po.project_code
		LEFT JOIN material_code mc  ON mc.mat_code = pol.mat_code
		LEFT JOIN mat_name      mn  ON mn.id = mc.mat_name_id
		LEFT JOIN cost_subgroup csg ON csg.id = pol.cost_subgroup_id
		LEFT JOIN cost_group    cg  ON cg.id = csg.group_id
		LEFT JOIN cost_job      cj  ON cj.id = cg.job_id
		LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id
		WHERE po.status = 'APPROVED'
		  AND (
		    pol.mat_code ILIKE $1
		    OR (csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code) ILIKE $1
		  )
		ORDER BY po.created_at DESC
		LIMIT 20`, like)
	if err != nil {
		return err
	}
	defer lineRows.Close()
	for lineRows.Next() {
		var poID int64
		var poNo string
		var projectCode, projectName, costCode *string
		var matCode string
		var matName *string
		var qtyOrdered, unitPrice, amount float64
		if err := lineRows.Scan(&poID, &poNo, &projectCode, &projectName,
			&matCode, &matName, &costCode, &qtyOrdered, &unitPrice, &amount); err != nil {
			return err
		}
		results = append(results, fiber.Map{
			"type":         "po_line",
			"po_id":        poID,
			"po_no":        poNo,
			"project_code": projectCode,
			"project_name": projectName,
			"mat_code":     matCode,
			"mat_name":     matName,
			"cost_code":    costCode,
			"qty_ordered":  qtyOrdered,
			"unit_price":   unitPrice,
			"amount":       amount,
		})
	}

	return c.JSON(fiber.Map{"success": true, "data": results})
}

package handlers

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OFReportHandler serves the "รายงาน OF" pages. Only the PR reduction page (รายการการลดยอด PR) exists so far.
type OFReportHandler struct{ db *pgxpool.Pool }

func NewOFReportHandler(db *pgxpool.Pool) *OFReportHandler { return &OFReportHandler{db: db} }

// prReduceReasonMax is the longest reason accepted (the form shows a counter of this size).
const prReduceReasonMax = 300

// sqlPRReducibleStatuses: an approved PR (COMPLETED) or one a PO already took part of (PARTIALLY_FILLED).
const sqlPRReducibleStatuses = `pr.status IN ('COMPLETED','PARTIALLY_FILLED')`

// ListPRs  GET /reports/of/pr-reduce/prs
// Approved PRs that still have at least one line not fully ordered (nothing or only part opened as PO).
// show_all=true also returns the fully ordered ones. search_by = doc_no (default) | project.
func (h *OFReportHandler) ListPRs(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	page := max(c.QueryInt("page", 1), 1)
	limit := min(max(c.QueryInt("limit", 20), 1), 200)
	showAll := c.QueryBool("show_all", false)
	search := strings.TrimSpace(c.Query("search"))
	searchBy := c.Query("search_by", "doc_no")
	if searchBy != "doc_no" && searchBy != "project" {
		return fiber.NewError(fiber.StatusBadRequest, "search_by must be doc_no or project")
	}

	where := " WHERE pr.deleted_at IS NULL AND " + sqlPRReducibleStatuses
	args := []any{}
	if !showAll {
		where += " AND " + sqlPRHasOrderableLine
	}
	if search != "" {
		args = append(args, search)
		p := fmt.Sprintf("$%d", len(args))
		if searchBy == "doc_no" {
			where += " AND pr.pr_no ILIKE '%' || " + p + " || '%'"
		} else {
			where += " AND (pr.project_code ILIKE '%' || " + p + " || '%' OR pj.project_name ILIKE '%' || " + p + " || '%')"
		}
	}
	// Same visibility rule as GET /pr: a restricted project-department user only sees their projects (and own PRs).
	args = append(args, claims.UserID)
	up := fmt.Sprintf("$%d", len(args))
	where += " AND " + projectScopeSQL(up, "pr.project_code", "pr.requested_by = "+up)

	from := ` FROM purchase_request pr
		LEFT JOIN project pj ON pj.project_code = pr.project_code
		LEFT JOIN users u1 ON u1.id = pr.requested_by`

	var total int64
	if err := h.db.QueryRow(context.Background(), `SELECT COUNT(*)`+from+where, args...).Scan(&total); err != nil {
		return err
	}

	listArgs := append(append([]any{}, args...), limit, (page-1)*limit)
	rows, err := h.db.Query(context.Background(), `
		SELECT pr.id, pr.pr_no, pr.pr_date::text, pr.job_code, pr.order_type, pr.project_code, pj.project_name,
		       COALESCE(u1.full_name, ''), pr.remarks, `+sqlPRHasOrderableLine+` AS has_remaining`+from+where+`
		ORDER BY pr.pr_date DESC, pr.id DESC
		LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), listArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type item struct {
		ID           int64   `json:"id"`
		PRNo         string  `json:"pr_no"`
		PRDate       string  `json:"pr_date"`
		JobCode      *string `json:"job_code"`
		OrderType    *string `json:"order_type"`
		ProjectCode  *string `json:"project_code"`
		ProjectName  *string `json:"project_name"`
		RequestedBy  string  `json:"requested_by"`
		Remarks      *string `json:"remarks"`
		HasRemaining bool    `json:"has_remaining"`
	}
	items := make([]item, 0)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.PRNo, &it.PRDate, &it.JobCode, &it.OrderType, &it.ProjectCode, &it.ProjectName,
			&it.RequestedBy, &it.Remarks, &it.HasRemaining); err != nil {
			return err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": items, "total": total, "page": page, "limit": limit})
}

// ListLines  GET /reports/of/pr-reduce/prs/:id/lines
// Every line of the PR with its balance (qty_balance = qty_requested - qty_reserved - qty_ordered, the same
// "remaining orderable" rule as the PO pickers) and the latest real PO price of the material for reference.
func (h *OFReportHandler) ListLines(c *fiber.Ctx) error {
	prID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PR id")
	}
	var exists bool
	if err := h.db.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM purchase_request pr WHERE pr.id=$1 AND pr.deleted_at IS NULL AND `+sqlPRReducibleStatuses+`)`,
		prID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT prl.id, prl.line_no, `+costCodeSQL("csub", "cj", "cg", "csg")+` AS cost_code, prl.mat_code,
		       TRIM(COALESCE(mn.mat_name, '') || ' ' || COALESCE(ss.spec_description, '')) AS material,
		       u.unit_name, prl.qty_requested, COALESCE(prl.qty_reserved, 0), COALESCE(prl.qty_ordered, 0),
		       `+sqlPRLineRemainingExpr+` AS qty_balance,
		       (SELECT pol.unit_price
		          FROM purchase_order_line pol
		          JOIN purchase_order po ON po.id = pol.po_id AND po.deleted_at IS NULL
		         WHERE pol.mat_code = prl.mat_code AND pol.status != 'CANCELLED' AND po.status != 'CANCELLED'
		           AND pol.unit_price > 0
		         ORDER BY po.po_date DESC, pol.id DESC LIMIT 1) AS last_price
		FROM purchase_request_line prl
		JOIN material_code mc ON mc.mat_code = prl.mat_code
		JOIN mat_name mn ON mn.id = mc.mat_name_id
		LEFT JOIN unit u ON u.id = mc.unit_id
		LEFT JOIN spec_size ss ON ss.id = mc.spec_id
		LEFT JOIN cost_subgroup csg ON csg.id = prl.cost_subgroup_id
		LEFT JOIN cost_group cg ON cg.id = csg.group_id
		LEFT JOIN cost_job cj ON cj.id = cg.job_id
		LEFT JOIN cost_subject csub ON csub.id = cj.subject_id
		WHERE prl.pr_id = $1
		ORDER BY prl.line_no`, prID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type line struct {
		LineID       int64    `json:"line_id"`
		LineNo       int      `json:"line_no"`
		CostCode     *string  `json:"cost_code"`
		MatCode      string   `json:"mat_code"`
		Material     string   `json:"material"`
		UnitName     *string  `json:"unit_name"`
		QtyRequested float64  `json:"qty_requested"`
		QtyReserved  float64  `json:"qty_reserved"`
		QtyOrdered   float64  `json:"qty_ordered"`
		QtyBalance   float64  `json:"qty_balance"`
		LastPrice    *float64 `json:"last_price"`
	}
	lines := make([]line, 0)
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.LineID, &l.LineNo, &l.CostCode, &l.MatCode, &l.Material, &l.UnitName, &l.QtyRequested,
			&l.QtyReserved, &l.QtyOrdered, &l.QtyBalance, &l.LastPrice); err != nil {
			return err
		}
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": lines})
}

type prReduceItem struct {
	LineID       int64   `json:"line_id"`
	QtyDecrement float64 `json:"qty_decrement"`
}

type prReduceRequest struct {
	Reason string         `json:"reason"`
	Items  []prReduceItem `json:"items"`
}

// Reduce  POST /reports/of/pr-reduce/prs/:id/reduce
// Lowers qty_requested of the chosen lines and records each change in pr_line_reduction (with the mandatory reason).
// A line can only lose what is still orderable (qty_requested - qty_reserved - qty_ordered), so stock that is
// already reserved and quantities already on a PO are never touched; the line must keep a qty > 0.
func (h *OFReportHandler) Reduce(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	prID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PR id")
	}
	var req prReduceRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุหมายเหตุ")
	}
	if utf8.RuneCountInString(req.Reason) > prReduceReasonMax {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("หมายเหตุยาวเกิน %d ตัวอักษร", prReduceReasonMax))
	}
	if len(req.Items) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกรายการที่ต้องการลดยอด")
	}
	seen := map[int64]bool{}
	for _, it := range req.Items {
		if it.LineID <= 0 || seen[it.LineID] {
			return fiber.NewError(fiber.StatusBadRequest, "invalid or duplicate line_id")
		}
		seen[it.LineID] = true
		if !(it.QtyDecrement > 0) || math.IsInf(it.QtyDecrement, 0) {
			return fiber.NewError(fiber.StatusBadRequest, "จำนวนที่ลดต้องมากกว่า 0")
		}
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var prNo string
	if err := tx.QueryRow(ctx,
		`SELECT pr.pr_no FROM purchase_request pr WHERE pr.id=$1 AND pr.deleted_at IS NULL AND `+sqlPRReducibleStatuses+` FOR UPDATE`,
		prID).Scan(&prNo); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found or its status no longer allows a reduction")
	}

	for _, it := range req.Items {
		var matCode string
		var qtyReq, qtyReserved, qtyOrdered float64
		if err := tx.QueryRow(ctx, `
			SELECT mat_code, qty_requested, COALESCE(qty_reserved, 0), COALESCE(qty_ordered, 0)
			FROM purchase_request_line WHERE id=$1 AND pr_id=$2 FOR UPDATE`, it.LineID, prID,
		).Scan(&matCode, &qtyReq, &qtyReserved, &qtyOrdered); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("line %d does not belong to PR %s", it.LineID, prNo))
		}
		balance := qtyReq - qtyReserved - qtyOrdered
		if it.QtyDecrement-balance > 1e-9 {
			return fiber.NewError(fiber.StatusConflict, fmt.Sprintf(
				"%s: ลดได้ไม่เกินยอดคงเหลือ %.4f (ขอลด %.4f)", matCode, balance, it.QtyDecrement))
		}
		// Reducing a line to exactly 0 is allowed: the line stays on the PR (and prints with qty 0).
		after := qtyReq - it.QtyDecrement
		if after < 1e-9 {
			after = 0
		}
		// qty_to_order only exists on PRs approved after stock auto-split; it stays 0 on legacy ones.
		if _, err := tx.Exec(ctx, `
			UPDATE purchase_request_line
			SET qty_requested = $1, qty_to_order = GREATEST(qty_to_order - $2, 0)
			WHERE id = $3`, after, it.QtyDecrement, it.LineID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO pr_line_reduction (pr_id, pr_line_id, mat_code, qty_before, qty_decrement, qty_after, reason, reduced_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			prID, it.LineID, matCode, qtyReq, it.QtyDecrement, after, req.Reason, claims.UserID); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "บันทึกการลดยอดเรียบร้อย"})
}

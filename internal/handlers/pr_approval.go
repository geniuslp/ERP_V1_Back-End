package handlers

import (
	"context"
	"log"
	"strconv"

	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PRApprovalHandler struct {
	db *pgxpool.Pool
}

func NewPRApprovalHandler(db *pgxpool.Pool) *PRApprovalHandler {
	return &PRApprovalHandler{db: db}
}

// ListPRApproval godoc
// @Summary      List purchase requests
// @Description  available_for_po=true additionally restricts to status=COMPLETED PRs that still
// @Description  have at least one line not fully referenced by existing PO(s) — for the "select
// @Description  PR to create PO from" picker only. Omit it for any other PR listing (PR status/
// @Description  history pages), which must keep showing every PR regardless of reference status.
// @Description  Each item also carries po_conversion_status, computed from purchase_request_line
// @Description  qty_requested vs qty_ordered (aggregated across all lines, not the same as "status"):
// @Description  FULLY_CONVERTED (every line qty_ordered >= qty_requested), PARTIALLY_CONVERTED
// @Description  (at least one line qty_ordered > 0 but not all lines fully ordered), NOT_CONVERTED
// @Description  (every line qty_ordered = 0 or NULL).
// @Description  Each item also carries job_name (resolved from job_code via cost_subject+cost_job,
// @Description  same join as GET /pr/{id}) and project_name (resolved from project_code via the
// @Description  project table) — both null when the PR has no job_code/project_code or the code
// @Description  doesn't resolve to a live master row.
// @Description  Each item also carries memo_no (resolved from memo_id via the memo table),
// @Description  dept_name (resolved from dept_code via the departments table), required_date
// @Description  ("วันที่ส่งสินค้า" — when the requester needs the goods by, distinct from pr_date)
// @Description  and created_at ("วันที่เปิดเอกสาร" — the DB row insert timestamp). dept_name,
// @Description  memo_no, and required_date are null when the PR has no dept_code/memo_id/
// @Description  required_date or the code doesn't resolve to a live row.
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        status            query  string  false  "status filter"
// @Param        available_for_po  query  bool    false  "true = only COMPLETED PRs with remaining unreferenced qty on at least one line"
// @Param        page              query  int     false  "page"   default(1)
// @Param        limit             query  int     false  "limit"  default(20)
// @Success      200  {object}  fiber.Map
// @Router       /pr [get]
func (h *PRApprovalHandler) List(c *fiber.Ctx) error {
	status := c.Query("status")
	availableForPO := c.Query("available_for_po") == "true"
	page := max(c.QueryInt("page", 1), 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var statusFilter *string
	if status != "" {
		statusFilter = &status
	}

	// available_for_po forces status=COMPLETED (PR's only "usable" terminal status) and adds a
	// live EXISTS check: at least one line whose referenced-qty sum, from non-cancelled
	// purchase_order_line rows joined the same way LinesWithPOStatus computes qty_remaining, is
	// still below qty_requested. Uses qty_requested rather than qty_to_order because qty_to_order
	// is a one-time snapshot computed at PR-submit time against stock_item.qty and is never
	// resynced afterward — stock consumed elsewhere post-submit (or missing at submit time) left
	// stale qty_to_order values that could hide PRs which still genuinely need purchasing (same
	// fix already applied to GetAvailablePRs in po.go). A PR excluded here always shows
	// qty_remaining=0 on every line there, and vice versa.
	availableForPOFilter := "TRUE"
	if availableForPO {
		statusFilter = nil
		availableForPOFilter = `
			pr.status = 'COMPLETED'
			AND EXISTS (
				SELECT 1 FROM purchase_request_line prl
				WHERE prl.pr_id = pr.id
				AND prl.qty_requested > COALESCE((
					SELECT SUM(pol.qty_ordered)
					FROM purchase_order_line pol
					WHERE pol.pr_line_id = prl.id AND pol.status != 'CANCELLED'
				), 0)
			)`
	}

	var total int64
	h.db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM purchase_request pr WHERE ($1::text IS NULL OR pr.status = $1) AND (`+availableForPOFilter+`)`,
		statusFilter,
	).Scan(&total)

	type PRListItem struct {
		ID                 int64   `json:"id"`
		PRNo               string  `json:"pr_no"`
		Status             string  `json:"status"`
		RequestedBy        string  `json:"requested_by"`
		ApproverName       *string `json:"approver_name"`
		LocationText       string  `json:"location_text"`
		ProjectCode        *string `json:"project_code"`
		DeptCode           *string `json:"dept_code,omitempty"`
		Remarks            *string `json:"remarks"`
		PRDate             string  `json:"pr_date"`
		PRType             string  `json:"pr_type"`
		JobCode            *string `json:"job_code,omitempty"`
		JobName            *string `json:"job_name,omitempty"`
		ProjectName        *string `json:"project_name,omitempty"`
		DeptName           *string `json:"dept_name,omitempty"`
		MemoID             *int64  `json:"memo_id,omitempty"`
		MemoNo             *string `json:"memo_no,omitempty"`
		RequiredDate       *string `json:"required_date,omitempty"`
		CreatedAt          string  `json:"created_at"`
		PoConversionStatus string  `json:"po_conversion_status"`
	}

	// po_conversion_status is computed per PR from an aggregate over purchase_request_line,
	// not per line — one CTE (poconv) joined once, no N+1 queries:
	//   FULLY_CONVERTED     — every line has qty_ordered >= qty_requested
	//   PARTIALLY_CONVERTED — at least one line has qty_ordered > 0 but not all lines are fully ordered
	//   NOT_CONVERTED       — every line has qty_ordered = 0 (or NULL)
	rows, err := h.db.Query(context.Background(), `
WITH poconv AS (
	SELECT prl.pr_id,
	       BOOL_AND(COALESCE(prl.qty_ordered, 0) >= prl.qty_requested) AS all_full,
	       BOOL_OR(COALESCE(prl.qty_ordered, 0) > 0) AS any_ordered
	FROM purchase_request_line prl
	GROUP BY prl.pr_id
)
SELECT pr.id AS pr_id, pr.pr_no, pr.status,
       COALESCE(u1.full_name, '') AS requested_by,
       NULL::text                 AS approver_name,
       pr.location_text, pr.project_code, pr.dept_code, pr.remarks,
       pr.pr_date::text, pr.pr_type, pr.job_code, cj.job_name, pj.project_name,
       d.dept_name, pr.memo_id, m.memo_no, pr.required_date::text, pr.created_at::text,
       CASE
           WHEN COALESCE(pc.all_full, false) THEN 'FULLY_CONVERTED'
           WHEN COALESCE(pc.any_ordered, false) THEN 'PARTIALLY_CONVERTED'
           ELSE 'NOT_CONVERTED'
       END AS po_conversion_status
FROM purchase_request pr
LEFT JOIN users u1 ON u1.id = pr.requested_by
LEFT JOIN poconv pc ON pc.pr_id = pr.id
LEFT JOIN cost_subject cs ON cs.subject_code = LEFT(pr.job_code, 1)
LEFT JOIN cost_job cj ON cj.subject_id = cs.id AND cj.job_code = SUBSTRING(pr.job_code FROM 2)
LEFT JOIN project pj ON pj.project_code = pr.project_code
LEFT JOIN departments d ON d.dept_code = pr.dept_code
LEFT JOIN memo m ON m.id = pr.memo_id
WHERE ($1::text IS NULL OR pr.status = $1) AND (`+availableForPOFilter+`)
ORDER BY pr.created_at DESC
LIMIT $2 OFFSET $3`,
		statusFilter, limit, offset,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := make([]PRListItem, 0)
	for rows.Next() {
		var item PRListItem
		rows.Scan(&item.ID, &item.PRNo, &item.Status, &item.RequestedBy, &item.ApproverName,
			&item.LocationText, &item.ProjectCode, &item.DeptCode, &item.Remarks, &item.PRDate, &item.PRType, &item.JobCode,
			&item.JobName, &item.ProjectName, &item.DeptName, &item.MemoID, &item.MemoNo, &item.RequiredDate, &item.CreatedAt,
			&item.PoConversionStatus)
		items = append(items, item)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"items": items,
			"total": total,
			"page":  page,
			"limit": limit,
		},
	})
}

// GetPRDetail godoc
// @Summary      Get PR detail with lines and attachments
// @Description  po_conversion_status is computed from purchase_request_line qty_requested vs
// @Description  qty_ordered aggregated across all lines: FULLY_CONVERTED, PARTIALLY_CONVERTED,
// @Description  or NOT_CONVERTED (same 3 values as GET /pr's list-level field).
// @Description  project_name is resolved from project_code via the project table (null if the
// @Description  PR has no project_code or it doesn't match a live project row), same as job_name.
// @Description  dept_name is resolved from dept_code via the departments table, and memo_no is
// @Description  resolved from memo_id via the memo table — both null when the PR has no
// @Description  dept_code/memo_id or the code doesn't resolve to a live row. Also carries
// @Description  required_date and created_at.
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PR ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /pr/{id} [get]
func (h *PRApprovalHandler) GetDetail(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "invalid id"})
	}

	type PRLineItem struct {
		ID           int64   `json:"id"`
		LineNo       int     `json:"line_no"`
		MatCode      string  `json:"mat_code"`
		QtyRequested float64 `json:"qty_requested"`
		QtyReserved  float64 `json:"qty_reserved"`
		QtyToOrder   float64 `json:"qty_to_order"`
		Status       string  `json:"status"`
		Remarks      *string `json:"remarks,omitempty"`
		MatName      *string `json:"mat_name,omitempty"`
		UnitName     *string `json:"unit_name,omitempty"`
		BrandName    *string `json:"brand_name,omitempty"`
		SpecName     *string `json:"spec_name,omitempty"`
		GroupName    *string `json:"group_name,omitempty"`
		SubgroupName *string `json:"subgroup_name,omitempty"`

		// Cost Code — chosen per line item, not tied to the material.
		CostSubgroupID   *int64  `json:"cost_subgroup_id,omitempty"`
		CostCode         *string `json:"cost_code,omitempty"`          // resolved combined code, e.g. "LE30300"
		CostSubgroupName *string `json:"cost_subgroup_name,omitempty"` // resolved cost_subgroup.subgroup_name
	}

	type PRDetail struct {
		ID                 int64                `json:"id"`
		PRNo               string               `json:"pr_no"`
		Status             string               `json:"status"`
		RequestedBy        string               `json:"requested_by"`
		RequesterID        int64                `json:"requester_id"`
		ApproverID         *int64               `json:"approver_id"`
		ApproverName       *string              `json:"approver_name"`
		LocationText       string               `json:"location_text"`
		ProjectCode        *string              `json:"project_code"`
		DeptCode           *string              `json:"dept_code,omitempty"`
		DeptName           *string              `json:"dept_name,omitempty"` // resolved from dept_code via the departments table (dept_code UNIQUE, single-row join)
		WarehouseCode      *string              `json:"warehouse_code"`
		WarehouseName      *string              `json:"warehouse_name"`
		Remarks            *string              `json:"remarks"`
		PRDate             string               `json:"pr_date"`
		RequiredDate       *string              `json:"required_date,omitempty"`
		CreatedAt          string               `json:"created_at"`
		PRType             string               `json:"pr_type"`
		OrderType          string               `json:"order_type"`
		JobCode            *string              `json:"job_code,omitempty"`
		JobName            *string              `json:"job_name,omitempty"`     // resolved from cost_subject+cost_job — job_code is a plain varchar, not an FK, but conventionally composes as subject_code+cost_job.job_code (e.g. "MP" = Material + Metal Structure)
		ProjectName        *string              `json:"project_name,omitempty"` // resolved from project_code via the project table (project_code UNIQUE, single-row join)
		MemoID             *int64               `json:"memo_id,omitempty"`
		MemoNo             *string              `json:"memo_no,omitempty"` // resolved from memo_id via the memo table (memo id PK, single-row join)
		PoConversionStatus string               `json:"po_conversion_status"`
		Lines              []PRLineItem         `json:"lines"`
		Attachments        models.PRAttachments `json:"attachments"`
	}

	// ── Header ──────────────────────────────────────────────────────────────
	row := h.db.QueryRow(context.Background(), `
		SELECT pr.id, pr.pr_no, pr.status,
		       COALESCE(u1.full_name, '') AS requested_by, pr.requested_by AS requester_id,
		       NULL AS approver_id, NULL AS approver_name,
		       pr.location_text, pr.project_code, pr.dept_code, d.dept_name, pr.warehouse_code, w.warehouse_name, pr.remarks,
		       pr.pr_date::text, pr.required_date::text, pr.created_at::text, pr.pr_type, pr.order_type, pr.job_code, cj.job_name, pj.project_name, pr.memo_id, m.memo_no,
		       CASE
		           WHEN COALESCE(BOOL_AND(COALESCE(prl.qty_ordered, 0) >= prl.qty_requested), false) THEN 'FULLY_CONVERTED'
		           WHEN COALESCE(BOOL_OR(COALESCE(prl.qty_ordered, 0) > 0), false) THEN 'PARTIALLY_CONVERTED'
		           ELSE 'NOT_CONVERTED'
		       END AS po_conversion_status
		FROM purchase_request pr
		LEFT JOIN users u1 ON u1.id = pr.requested_by
		LEFT JOIN warehouse w ON w.warehouse_code = pr.warehouse_code
		LEFT JOIN cost_subject cs ON cs.subject_code = LEFT(pr.job_code, 1)
		LEFT JOIN cost_job cj ON cj.subject_id = cs.id AND cj.job_code = SUBSTRING(pr.job_code FROM 2)
		LEFT JOIN project pj ON pj.project_code = pr.project_code
		LEFT JOIN departments d ON d.dept_code = pr.dept_code
		LEFT JOIN memo m ON m.id = pr.memo_id
		LEFT JOIN purchase_request_line prl ON prl.pr_id = pr.id
		WHERE pr.id = $1
		GROUP BY pr.id, pr.pr_no, pr.status, u1.full_name, pr.requested_by, pr.location_text,
		         pr.project_code, pr.dept_code, d.dept_name, pr.warehouse_code, w.warehouse_name, pr.remarks,
		         pr.pr_date, pr.required_date, pr.created_at, pr.pr_type, pr.order_type, pr.job_code, cj.job_name, pj.project_name, pr.memo_id, m.memo_no`, id)

	var pr PRDetail
	pr.Lines = make([]PRLineItem, 0)
	pr.Attachments.PR = []models.PRAttachment{}

	if err := row.Scan(
		&pr.ID, &pr.PRNo, &pr.Status, &pr.RequestedBy, &pr.RequesterID,
		&pr.ApproverID, &pr.ApproverName, &pr.LocationText, &pr.ProjectCode, &pr.DeptCode, &pr.DeptName,
		&pr.WarehouseCode, &pr.WarehouseName, &pr.Remarks, &pr.PRDate, &pr.RequiredDate, &pr.CreatedAt, &pr.PRType, &pr.OrderType, &pr.JobCode, &pr.JobName, &pr.ProjectName, &pr.MemoID, &pr.MemoNo,
		&pr.PoConversionStatus,
	); err != nil {
		log.Printf("❌ header scan error: %v", err)
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}

	// ── Lines ────────────────────────────────────────────────────────────────
	lineRows, err := h.db.Query(context.Background(), `
		SELECT
			prl.id, prl.line_no, prl.mat_code, prl.qty_requested,
			prl.qty_reserved, prl.qty_to_order, prl.status, prl.remarks,
			mn.mat_name, u.unit_name, b.brand_name, ss.spec_description AS spec_name,
			mg.group_name, sg.subgroup_name,
			prl.cost_subgroup_id,
			csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code AS cost_code,
			csg.subgroup_name AS cost_subgroup_name
		FROM purchase_request_line prl
		LEFT JOIN material_code mc ON mc.mat_code = prl.mat_code
		LEFT JOIN mat_group     mg ON mg.id = mc.group_id
		LEFT JOIN subgroup      sg ON sg.id = mc.subgroup_id
		LEFT JOIN mat_name      mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size     ss ON ss.id = mc.spec_id
		LEFT JOIN brand         b  ON b.id  = mc.brand_id
		LEFT JOIN unit          u  ON u.id  = mc.unit_id
		LEFT JOIN cost_subgroup csg  ON csg.id = prl.cost_subgroup_id
		LEFT JOIN cost_group    cg   ON cg.id = csg.group_id
		LEFT JOIN cost_job      cj   ON cj.id = cg.job_id
		LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id
		WHERE prl.pr_id = $1
		ORDER BY prl.line_no`, id)
	if err != nil {
		log.Printf("❌ lines query error: %v", err)
	} else {
		defer lineRows.Close()
		for lineRows.Next() {
			var l PRLineItem
			if err := lineRows.Scan(
				&l.ID, &l.LineNo, &l.MatCode, &l.QtyRequested,
				&l.QtyReserved, &l.QtyToOrder, &l.Status, &l.Remarks,
				&l.MatName, &l.UnitName, &l.BrandName, &l.SpecName,
				&l.GroupName, &l.SubgroupName,
				&l.CostSubgroupID, &l.CostCode, &l.CostSubgroupName,
			); err != nil {
				log.Printf("❌ lines scan error: %v", err)
			}
			pr.Lines = append(pr.Lines, l)
		}
	}

	// ── Attachments ──────────────────────────────────────────────────────────
	prAttRows, err := h.db.Query(context.Background(), `
		SELECT id, pr_id, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at
		FROM pr_attachment WHERE pr_id=$1 ORDER BY uploaded_at`, id)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch pr attachments: "+err.Error())
	}
	prAtts := []models.PRAttachment{}
	for prAttRows.Next() {
		var a models.PRAttachment
		if err := prAttRows.Scan(&a.ID, &a.PRID, &a.FileName, &a.FilePath, &a.FileSize, &a.FileType, &a.UploadedBy, &a.UploadedAt); err != nil {
			prAttRows.Close()
			return fiber.NewError(fiber.StatusInternalServerError, "failed to scan pr attachments: "+err.Error())
		}
		a.FilePath = toAbsoluteFileURL(a.FilePath)
		prAtts = append(prAtts, a)
	}
	prAttRows.Close()
	pr.Attachments.PR = prAtts

	if pr.MemoID != nil {
		memoAttRows, err := h.db.Query(context.Background(), `
			SELECT id, memo_id, file_path, file_name, file_size, file_type, uploaded_by, uploaded_at
			FROM memo_attachment WHERE memo_id=$1 ORDER BY uploaded_at`, *pr.MemoID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch memo attachments: "+err.Error())
		}
		memoAtts := []models.MemoAttachment{}
		for memoAttRows.Next() {
			var a models.MemoAttachment
			if err := memoAttRows.Scan(&a.ID, &a.MemoID, &a.FilePath, &a.FileName, &a.FileSize, &a.FileType, &a.UploadedBy, &a.UploadedAt); err != nil {
				memoAttRows.Close()
				return fiber.NewError(fiber.StatusInternalServerError, "failed to scan memo attachments: "+err.Error())
			}
			a.FilePath = toAbsoluteFileURL(a.FilePath)
			memoAtts = append(memoAtts, a)
		}
		memoAttRows.Close()
		pr.Attachments.Memo = &memoAtts
	}

	return c.JSON(fiber.Map{"success": true, "data": pr})
}

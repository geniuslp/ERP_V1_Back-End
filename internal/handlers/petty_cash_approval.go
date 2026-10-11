package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// pcApprovalID reads the document id from either route shape:
// /petty-cash/:id/approve|reject or /approval/PETTY_CASH/:docId/approve|reject.
func pcApprovalID(c *fiber.Ctx) (int64, error) {
	raw := c.Params("id")
	if raw == "" {
		raw = c.Params("docId")
	}
	return strconv.ParseInt(raw, 10, 64)
}

// approvalDecide is the single Approve/Reject implementation for petty cash. It is reached by
// POST /petty-cash/:id/approve|reject and by PUT /approval/PETTY_CASH/:docId/approve|reject
// (registered ahead of the generic handler, which cannot write approval_log or the approver
// remark without changing PO/MEMO behaviour). Everything happens in ONE transaction with the
// document row locked, so two simultaneous decisions cannot both succeed.
func (h *PettyCashHandler) approvalDecide(c *fiber.Ctx, action string) error {
	id, err := pcApprovalID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสเอกสารไม่ถูกต้อง")
	}

	var req models.ApprovalActionRequest
	_ = c.BodyParser(&req)
	comments := ""
	if req.Comments != nil {
		comments = strings.TrimSpace(*req.Comments)
	}
	if action == "REJECT" && comments == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุเหตุผลที่ไม่อนุมัติ")
	}
	var commentsArg *string // NULL when empty
	if comments != "" {
		commentsArg = &comments
	}

	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "กรุณาเข้าสู่ระบบ")
	}
	ctx := context.Background()

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถดำเนินการได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer tx.Rollback(ctx)

	var currentStatus, pcNo string
	if err := tx.QueryRow(ctx,
		`SELECT status, pc_no FROM petty_cash_requisition WHERE id=$1 FOR UPDATE`, id,
	).Scan(&currentStatus, &pcNo); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "ไม่พบเอกสาร")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถดำเนินการได้ กรุณาลองใหม่อีกครั้ง")
	}
	if currentStatus == "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest, "เอกสารนี้ยังไม่ได้ส่งขออนุมัติ")
	}
	if currentStatus != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusConflict, "เอกสารนี้ถูกดำเนินการแล้ว")
	}

	var approvalReqID int64
	var stepNo int
	if err := tx.QueryRow(ctx, `
		SELECT id, step_no FROM approval_request
		WHERE doc_type='PETTY_CASH' AND doc_id=$1 AND status='PENDING'
		ORDER BY step_no DESC, id DESC LIMIT 1 FOR UPDATE`, id,
	).Scan(&approvalReqID, &stepNo); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusForbidden, "คุณไม่มีสิทธิ์อนุมัติเอกสารนี้")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถดำเนินการได้ กรุณาลองใหม่อีกครั้ง")
	}

	// Same eligibility rule the Approve/Reject button gating uses (my-status): the assigned
	// approver_id, or an extra approver from approval_delegation. No hardcoded roles.
	eligible, err := isEligibleApprover(ctx, h.db, "PETTY_CASH", stepNo, id, claims.UserID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบสิทธิ์ได้ กรุณาลองใหม่อีกครั้ง")
	}
	if !eligible {
		return fiber.NewError(fiber.StatusForbidden, "คุณไม่มีสิทธิ์อนุมัติเอกสารนี้")
	}

	newStatus := "APPROVED"
	if action == "REJECT" {
		newStatus = "REJECTED"
	}

	if _, err := tx.Exec(ctx, `
		UPDATE petty_cash_requisition
		SET status=$1, approver_remarks=$4, updated_at=NOW(), updated_by=$2
		WHERE id=$3`, newStatus, claims.UserID, id, commentsArg,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกผลการพิจารณาได้")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO petty_cash_status_log (pc_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,$3,$4,$5)`, id, currentStatus, newStatus, claims.UserID, commentsArg,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกประวัติสถานะได้")
	}
	if _, err := tx.Exec(ctx,
		`UPDATE approval_request SET status=$1, updated_at=NOW() WHERE id=$2`, newStatus, approvalReqID,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกผลการพิจารณาได้")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO approval_log (approval_id, doc_type, doc_id, doc_no, step_no, action, action_by,
		                          comments, old_status, new_status, ip_address, user_agent)
		VALUES ($1,'PETTY_CASH',$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::inet,NULLIF($11,''))`,
		approvalReqID, id, pcNo, stepNo, action, claims.UserID, commentsArg, currentStatus, newStatus,
		c.IP(), c.Get("User-Agent"),
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกประวัติการอนุมัติได้")
	}

	if err := tx.Commit(ctx); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกผลการพิจารณาได้")
	}

	msg := "อนุมัติเอกสารเรียบร้อย"
	if action == "REJECT" {
		msg = "ไม่อนุมัติเอกสารเรียบร้อย"
	}
	return c.JSON(fiber.Map{
		"success": true,
		"message": msg,
		"data":    fiber.Map{"id": id, "pc_no": pcNo, "status": newStatus},
	})
}

// PendingApproval godoc
// @Summary      Petty cash documents waiting for the current user's approval
// @Description  status = PENDING_APPROVAL with a PENDING approval_request assigned to the caller. Paginated; search matches pc_no. vendor_names is the distinct supplier names on the lines, comma-joined. submitted_at = latest SUBMIT approval_log, else latest PENDING_APPROVAL status-log entry, else the approval_request creation time.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        page       query  int     false  "default 1"
// @Param        page_size  query  int     false  "default 20, max 100"
// @Param        search     query  string  false  "pc_no contains"
// @Success      200  {object}  models.PaginatedResponse
// @Router       /petty-cash/pending-approval [get]
func (h *PettyCashHandler) PendingApproval(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "กรุณาเข้าสู่ระบบ")
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	size, _ := strconv.Atoi(c.Query("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	var search *string
	if s := strings.TrimSpace(c.Query("search")); s != "" {
		search = &s
	}
	ctx := context.Background()

	const baseFrom = `
		FROM petty_cash_requisition pc
		LEFT JOIN LATERAL (
			SELECT id, step_no, created_at FROM approval_request
			WHERE doc_type='PETTY_CASH' AND doc_id = pc.id AND status='PENDING' AND assigned_to = $1
			ORDER BY step_no DESC, id DESC LIMIT 1
		) ar ON TRUE`
	const baseWhere = `
		WHERE pc.status = 'PENDING_APPROVAL' AND ar.id IS NOT NULL
		  AND ($2::text IS NULL OR pc.pc_no ILIKE '%' || $2::text || '%')`

	var total int64
	if err := h.db.QueryRow(ctx, "SELECT COUNT(*) "+baseFrom+baseWhere, claims.UserID, search).Scan(&total); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการรออนุมัติได้")
	}

	rows, err := h.db.Query(ctx, `
		SELECT pc.id, pc.pc_no, pc.pc_date,
		       COALESCE(pc.project_code, lp.codes) AS project_code,
		       COALESCE(pc.project_name, lp.names) AS project_name,
		       pc.dept_code, pc.dept_name, u.full_name, pc.payee_name, lv.vendors,
		       pc.net_amount,
		       COALESCE(sl.action_at, ssl.changed_at, ar.created_at) AS submitted_at,
		       ar.id, ar.step_no
		`+baseFrom+`
		LEFT JOIN users u ON u.id = pc.requested_by
		LEFT JOIN LATERAL (
			SELECT string_agg(DISTINCT pcl.project_code, ', ') AS codes, string_agg(DISTINCT p.project_name, ', ') AS names
			FROM petty_cash_requisition_line pcl
			LEFT JOIN project p ON p.project_code = pcl.project_code
			WHERE pcl.pc_id = pc.id
		) lp ON TRUE
		LEFT JOIN LATERAL (
			SELECT string_agg(DISTINCT pcl.supplier_name, ', ') AS vendors
			FROM petty_cash_requisition_line pcl
			WHERE pcl.pc_id = pc.id AND pcl.supplier_name IS NOT NULL AND pcl.supplier_name <> ''
		) lv ON TRUE
		LEFT JOIN LATERAL (
			SELECT action_at FROM approval_log
			WHERE doc_type='PETTY_CASH' AND doc_id = pc.id AND action='SUBMIT'
			ORDER BY action_at DESC LIMIT 1
		) sl ON TRUE
		LEFT JOIN LATERAL (
			SELECT changed_at FROM petty_cash_status_log
			WHERE pc_id = pc.id AND to_status='PENDING_APPROVAL'
			ORDER BY changed_at DESC LIMIT 1
		) ssl ON TRUE`+baseWhere+`
		ORDER BY submitted_at DESC, pc.id DESC
		LIMIT $3 OFFSET $4`, claims.UserID, search, size, (page-1)*size)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการรออนุมัติได้")
	}
	defer rows.Close()

	type item struct {
		ID                int64    `json:"id"`
		PCNo              string   `json:"pc_no"`
		PCDate            any      `json:"pc_date"`
		ProjectCode       *string  `json:"project_code"`
		ProjectName       *string  `json:"project_name"`
		DeptCode          *string  `json:"dept_code"`
		DeptName          *string  `json:"dept_name"`
		RequestedByName   *string  `json:"requested_by_name"`
		PayeeName         *string  `json:"payee_name"`
		VendorNames       *string  `json:"vendor_names"`
		NetAmount         float64  `json:"net_amount"`
		SubmittedAt       any      `json:"submitted_at"`
		ApprovalRequestID int64    `json:"approval_request_id"`
		StepNo            int      `json:"step_no"`
	}
	items := []item{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.PCNo, &it.PCDate, &it.ProjectCode, &it.ProjectName,
			&it.DeptCode, &it.DeptName, &it.RequestedByName, &it.PayeeName, &it.VendorNames,
			&it.NetAmount, &it.SubmittedAt, &it.ApprovalRequestID, &it.StepNo); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านรายการรออนุมัติได้")
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการรออนุมัติได้")
	}

	totalPages := (int(total) + size - 1) / size
	return c.JSON(fiber.Map{"success": true, "data": models.PaginatedResponse{
		Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages,
	}})
}

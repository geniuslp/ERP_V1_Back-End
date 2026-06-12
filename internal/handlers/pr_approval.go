package handlers

import (
	"context"
	"log"
	"strconv"

	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PRApprovalHandler struct {
	db *pgxpool.Pool
}

func NewPRApprovalHandler(db *pgxpool.Pool) *PRApprovalHandler {
	return &PRApprovalHandler{db: db}
}

// requireSeniorPM checks that the calling user belongs to "Senior Project Mgr" department.
// Returns the caller's userID on success, or a fiber error on failure.
func (h *PRApprovalHandler) requireSeniorPM(c *fiber.Ctx) (int64, error) {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return 0, fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	var dept string
	if err := h.db.QueryRow(context.Background(),
		`SELECT COALESCE(department, '') FROM users WHERE id = $1`, claims.UserID,
	).Scan(&dept); err != nil {
		return 0, fiber.NewError(fiber.StatusUnauthorized, "user not found")
	}
	if dept != "Senior Project Mgr" {
		return 0, fiber.NewError(fiber.StatusForbidden, "only Senior Project Mgr can perform this action")
	}
	return claims.UserID, nil
}

// ListPRApproval godoc
// @Summary      List purchase requests
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        status  query  string  false  "status filter"
// @Param        page    query  int     false  "page"   default(1)
// @Param        limit   query  int     false  "limit"  default(20)
// @Success      200  {object}  fiber.Map
// @Router       /pr [get]
func (h *PRApprovalHandler) List(c *fiber.Ctx) error {
	status := c.Query("status")
	page := max(c.QueryInt("page", 1), 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var statusFilter *string
	if status != "" {
		statusFilter = &status
	}

	var total int64
	h.db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM purchase_request WHERE ($1::text IS NULL OR status = $1)`,
		statusFilter,
	).Scan(&total)

	type PRListItem struct {
		ID           int64   `json:"id"`
		PRNo         string  `json:"pr_no"`
		Status       string  `json:"status"`
		RequestedBy  string  `json:"requested_by"`
		ApproverName *string `json:"approver_name"`
		LocationCode string  `json:"location_code"`
		ProjectCode  *string `json:"project_code"`
		Remarks      *string `json:"remarks"`
		PRDate       string  `json:"pr_date"`
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT pr.id AS pr_id, pr.pr_no, pr.status,
       COALESCE(u1.full_name, '') AS requested_by,
       NULL::text                 AS approver_name,
       pr.location_code, pr.project_code, pr.remarks,
       pr.pr_date::text
FROM purchase_request pr
LEFT JOIN users u1 ON u1.id = pr.requested_by
WHERE ($1::text IS NULL OR pr.status = $1)
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
			&item.LocationCode, &item.ProjectCode, &item.Remarks, &item.PRDate)
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
	}

	type PRAttachItem struct {
		ID         int64  `json:"id"`
		FileName   string `json:"file_name"`
		FilePath   string `json:"file_path"`
		FileSize   int64  `json:"file_size"`
		FileType   string `json:"file_type"`
		UploadedAt string `json:"uploaded_at"`
		UploadedBy int64  `json:"uploaded_by"`
	}

	type PRDetail struct {
		ID           int64          `json:"id"`
		PRNo         string         `json:"pr_no"`
		Status       string         `json:"status"`
		RequestedBy  string         `json:"requested_by"`
		RequesterID  int64          `json:"requester_id"`
		ApproverID   *int64         `json:"approver_id"`
		ApproverName *string        `json:"approver_name"`
		LocationCode string         `json:"location_code"`
		ProjectCode  *string        `json:"project_code"`
		Remarks      *string        `json:"remarks"`
		PRDate       string         `json:"pr_date"`
		Lines        []PRLineItem   `json:"lines"`
		Attachments  []PRAttachItem `json:"attachments"`
	}

	// ── Header ──────────────────────────────────────────────────────────────
	row := h.db.QueryRow(context.Background(), `
		SELECT pr.id, pr.pr_no, pr.status,
		       COALESCE(u1.full_name, '') AS requested_by, pr.requested_by AS requester_id,
		       NULL AS approver_id, NULL AS approver_name,
		       pr.location_code, pr.project_code, pr.remarks,
		       pr.pr_date::text
		FROM purchase_request pr
		LEFT JOIN users u1 ON u1.id = pr.requested_by
		WHERE pr.id = $1`, id)

	var pr PRDetail
	pr.Lines = make([]PRLineItem, 0)
	pr.Attachments = make([]PRAttachItem, 0)

	if err := row.Scan(
		&pr.ID, &pr.PRNo, &pr.Status, &pr.RequestedBy, &pr.RequesterID,
		&pr.ApproverID, &pr.ApproverName, &pr.LocationCode, &pr.ProjectCode,
		&pr.Remarks, &pr.PRDate,
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
			mg.group_name, sg.subgroup_name
		FROM purchase_request_line prl
		LEFT JOIN material_code mc ON mc.mat_code = prl.mat_code
		LEFT JOIN mat_group     mg ON mg.id = mc.group_id
		LEFT JOIN subgroup      sg ON sg.id = mc.subgroup_id
		LEFT JOIN mat_name      mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size     ss ON ss.id = mc.spec_id
		LEFT JOIN brand         b  ON b.id  = mc.brand_id
		LEFT JOIN unit          u  ON u.id  = mc.unit_id
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
			); err != nil {
				log.Printf("❌ lines scan error: %v", err)
			}
			pr.Lines = append(pr.Lines, l)
		}
	}

	// ── Attachments ──────────────────────────────────────────────────────────
	attRows, err := h.db.Query(context.Background(), `
		SELECT id, file_name, file_path, file_size, file_type,
		       to_char(uploaded_at, 'YYYY-MM-DD HH24:MI'), COALESCE(uploaded_by, 0)
		FROM pr_attachment WHERE pr_id = $1 ORDER BY uploaded_at`, id)
	if err != nil {
		log.Printf("❌ attachments query error: %v", err)
	} else {
		defer attRows.Close()
		for attRows.Next() {
			var a PRAttachItem
			if err := attRows.Scan(
				&a.ID, &a.FileName, &a.FilePath, &a.FileSize,
				&a.FileType, &a.UploadedAt, &a.UploadedBy,
			); err != nil {
				log.Printf("❌ attachments scan error: %v", err)
			}
			pr.Attachments = append(pr.Attachments, a)
		}
	}

	return c.JSON(fiber.Map{"success": true, "data": pr})
}

// ApprovePR godoc
// @Summary      Approve a purchase request
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PR ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      403  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /pr/{id}/approve [put]
func (h *PRApprovalHandler) Approve(c *fiber.Ctx) error {
	userID, err := h.requireSeniorPM(c)
	if err != nil {
		return err
	}

	id := c.Params("id")

	var status string
	if err := h.db.QueryRow(context.Background(),
		`SELECT status FROM purchase_request WHERE pr_id = $1`, id,
	).Scan(&status); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}
	if status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "PR is not pending approval")
	}

	if _, err := h.db.Exec(context.Background(), `
		UPDATE purchase_request
		SET status = 'APPROVED', approver_id = $1, updated_at = NOW(), updated_by = $1
		WHERE pr_id = $2`, userID, id); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "PR approved successfully"})
}

// RejectPR godoc
// @Summary      Reject a purchase request
// @Tags         Purchase Request
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int     true  "PR ID"
// @Param        body  body  object  true  "Rejection reason"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      403  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /pr/{id}/reject [put]
func (h *PRApprovalHandler) Reject(c *fiber.Ctx) error {
	userID, err := h.requireSeniorPM(c)
	if err != nil {
		return err
	}

	id := c.Params("id")

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(req.Reason) < 5 {
		return fiber.NewError(fiber.StatusBadRequest, "reason must be at least 5 characters")
	}

	var status string
	if err := h.db.QueryRow(context.Background(),
		`SELECT status FROM purchase_request WHERE pr_id = $1`, id,
	).Scan(&status); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}
	if status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "PR is not pending approval")
	}

	if _, err := h.db.Exec(context.Background(), `
		UPDATE purchase_request
		SET status = 'REJECTED', remarks = $1, approver_id = $2, updated_at = NOW(), updated_by = $2
		WHERE pr_id = $3`, req.Reason, userID, id); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "PR rejected successfully"})
}

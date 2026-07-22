package handlers

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

type MemoHandler struct{ db *pgxpool.Pool }

func NewMemoHandler(db *pgxpool.Pool) *MemoHandler {
	return &MemoHandler{db: db}
}

// generateMemoNo สร้างเลข memo รูปแบบ MEM-2506-0001
func (h *MemoHandler) generateMemoNo(ctx context.Context) (string, error) {
	var seq int64
	if err := h.db.QueryRow(ctx, "SELECT nextval('memo_seq')").Scan(&seq); err != nil {
		return "", err
	}
	return fmt.Sprintf("MEM-%s-%04d", time.Now().Format("0601"), seq), nil
}

func (h *MemoHandler) getByID(ctx context.Context, id int64) (*models.Memo, error) {
	var m models.Memo
	err := h.db.QueryRow(ctx, `
		SELECT m.id, m.memo_no, m.title, m.project_code,
		       m.requested_by, m.approver_id, m.department, m.note, m.status,
		       m.created_at, m.updated_at,
		       u.full_name   AS requested_by_name,
		       au.full_name  AS approver_name,
		       p.project_name
		FROM public.memo m
		JOIN public.users    u  ON u.id  = m.requested_by
		LEFT JOIN public.users au ON au.id = m.approver_id
		LEFT JOIN public.project  p ON p.project_code  = m.project_code
		WHERE m.id = $1`, id,
	).Scan(
		&m.ID, &m.MemoNo, &m.Title, &m.ProjectCode,
		&m.RequestedBy, &m.ApproverID, &m.Department, &m.Note, &m.Status,
		&m.CreatedAt, &m.UpdatedAt,
		&m.RequestedByName, &m.ApproverName, &m.ProjectName,
	)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.Query(ctx, `
		SELECT id, memo_id, line_no, description, unit,
		       quantity, remark
		FROM public.memo_line
		WHERE memo_id = $1
		ORDER BY line_no`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var l models.MemoLine
		if err := rows.Scan(
			&l.ID, &l.MemoID, &l.LineNo, &l.Description, &l.Unit,
			&l.Quantity, &l.Remark,
		); err != nil {
			return nil, err
		}
		m.Lines = append(m.Lines, l)
	}

	attRows, err := h.db.Query(ctx, `
		SELECT id, memo_id, file_path, file_name, file_size, file_type, uploaded_by, uploaded_at
		FROM public.memo_attachment
		WHERE memo_id = $1
		ORDER BY uploaded_at`, id)
	if err != nil {
		return nil, err
	}
	defer attRows.Close()

	for attRows.Next() {
		var a models.MemoAttachment
		if err := attRows.Scan(
			&a.ID, &a.MemoID, &a.FilePath, &a.FileName, &a.FileSize,
			&a.FileType, &a.UploadedBy, &a.UploadedAt,
		); err != nil {
			return nil, err
		}
		m.Attachments = append(m.Attachments, a)
	}

	return &m, nil
}

// List godoc
// @Summary      รายการ Memo ทั้งหมด
// @Tags         Memo
// @Security     BearerAuth
// @Produce      json
// @Param        search       query  string  false  "ค้นหา memo_no / title"
// @Param        project_code query  string  false  "กรอง project"
// @Param        date_from    query  string  false  "วันที่เริ่ม (YYYY-MM-DD)"
// @Param        date_to      query  string  false  "วันที่สิ้นสุด (YYYY-MM-DD)"
// @Param        my_approvals query  string  false  "true = แสดงเฉพาะ memo ที่ approver คือผู้ใช้ปัจจุบัน (จาก JWT)"
// @Param        page         query  int     false  "หน้า (default 1)"
// @Param        page_size    query  int     false  "จำนวนต่อหน้า (default 20)"
// @Success      200  {object}  models.PaginatedResponse
// @Router       /memo [get]
func (h *MemoHandler) List(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	var f models.MemoListFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	offset := (f.Page - 1) * f.PageSize

	args := []any{}
	where := "WHERE 1=1"
	i := 1

	if f.Search != "" {
		where += fmt.Sprintf(" AND (m.memo_no ILIKE $%d OR m.title ILIKE $%d)", i, i)
		args = append(args, "%"+f.Search+"%")
		i++
	}
	if f.ProjectCode != "" {
		where += fmt.Sprintf(" AND m.project_code = $%d", i)
		args = append(args, f.ProjectCode)
		i++
	}
	if f.DateFrom != "" {
		where += fmt.Sprintf(" AND m.created_at >= $%d", i)
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where += fmt.Sprintf(" AND m.created_at < ($%d::date + interval '1 day')", i)
		args = append(args, f.DateTo)
		i++
	}
	if f.Status != "" {
		where += fmt.Sprintf(" AND m.status = $%d", i)
		args = append(args, f.Status)
		i++
	}
	if f.ExcludeUsedByPR == "true" {
		where += ` AND NOT EXISTS (
			SELECT 1 FROM public.purchase_request pr
			WHERE pr.memo_id = m.id AND pr.status = 'COMPLETED'
		)`
	}

	isAdmin := slices.Contains(claims.Roles, "ADMIN_CENTER")
	isMyApprovals := f.MyApprovals == "true"

	if isMyApprovals {
		// Approver's own queue: filter by approver_id derived from the JWT,
		// never from client input. Bypasses the owner-only filter below since
		// this is an intentional cross-user query (the approver is usually not
		// the requester).
		where += fmt.Sprintf(" AND m.approver_id = $%d", i)
		args = append(args, claims.UserID)
		i++
	} else if f.AllUsers != "true" && !isAdmin {
		where += fmt.Sprintf(" AND m.requested_by = $%d", i)
		args = append(args, claims.UserID)
		i++
	}

	ctx := context.Background()

	var total int64
	countSQL := `SELECT COUNT(*) FROM public.memo m ` + where
	if err := h.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return err
	}

	dataSQL := `
		SELECT m.id, m.memo_no, m.title, m.project_code,
		       m.requested_by, m.approver_id, m.department, m.note, m.status,
		       m.created_at, m.updated_at,
		       u.full_name   AS requested_by_name,
		       au.full_name  AS approver_name,
		       p.project_name
		FROM public.memo m
		JOIN public.users    u  ON u.id  = m.requested_by
		LEFT JOIN public.users au ON au.id = m.approver_id
		LEFT JOIN public.project  p ON p.project_code  = m.project_code
		` + where + `
		ORDER BY m.created_at DESC
		LIMIT $` + fmt.Sprintf("%d", i) + ` OFFSET $` + fmt.Sprintf("%d", i+1)

	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, dataSQL, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []models.Memo{}
	for rows.Next() {
		var m models.Memo
		if err := rows.Scan(
			&m.ID, &m.MemoNo, &m.Title, &m.ProjectCode,
			&m.RequestedBy, &m.ApproverID, &m.Department, &m.Note, &m.Status,
			&m.CreatedAt, &m.UpdatedAt,
			&m.RequestedByName, &m.ApproverName, &m.ProjectName,
		); err != nil {
			return err
		}
		items = append(items, m)
	}

	totalPages := int((total + int64(f.PageSize) - 1) / int64(f.PageSize))
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: f.Page,
			PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// GetByID godoc
// @Summary      ดูรายละเอียด Memo
// @Tags         Memo
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Memo ID"
// @Success      200  {object}  models.Memo
// @Router       /memo/{id} [get]
func (h *MemoHandler) GetByID(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	m, err := h.getByID(context.Background(), int64(id))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "memo not found")
	}

	return c.JSON(fiber.Map{"success": true, "data": m})
}

// Create godoc
// @Summary      สร้าง Memo ใหม่
// @Tags         Memo
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateMemoRequest  true  "ข้อมูล Memo"
// @Success      201   {object}  models.Memo
// @Router       /memo [post]
func (h *MemoHandler) Create(c *fiber.Ctx) error {
	var req models.CreateMemoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Title == "" {
		return fiber.NewError(fiber.StatusBadRequest, "title is required")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least 1 line is required")
	}
	for _, l := range req.Lines {
		if l.Description == "" || l.Unit == "" {
			return fiber.NewError(fiber.StatusBadRequest, "line description and unit are required")
		}
		if l.Quantity <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, "quantity must be > 0")
		}
	}
	if req.RequestedBy == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "requested_by is required")
	}
	if req.ApproverID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "approver_id is required")
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var requesterExists bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM public.users WHERE id=$1)`, req.RequestedBy,
	).Scan(&requesterExists); err != nil {
		return err
	}
	if !requesterExists {
		return fiber.NewError(fiber.StatusBadRequest, "requester not found")
	}

	var approverExists bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM public.users WHERE id=$1)`, *req.ApproverID,
	).Scan(&approverExists); err != nil {
		return err
	}
	if !approverExists {
		return fiber.NewError(fiber.StatusBadRequest, "approver not found")
	}

	status := req.Status
	if status == "" {
		status = "DRAFT"
	}
	if status != "DRAFT" && status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "status must be DRAFT or PENDING_APPROVAL")
	}

	memoNo, err := h.generateMemoNo(ctx)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var memoID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.memo
		    (memo_no, title, project_code, requested_by, approver_id,
		     department, note, status, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
		RETURNING id`,
		memoNo, req.Title, req.ProjectCode, req.RequestedBy, req.ApproverID,
		req.Department, req.Note, status, claims.UserID,
	).Scan(&memoID)
	if err != nil {
		return err
	}

	for _, l := range req.Lines {
		_, err = tx.Exec(ctx, `
			INSERT INTO public.memo_line
			    (memo_id, line_no, description, unit, quantity, remark)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			memoID, l.LineNo, l.Description, l.Unit, l.Quantity, l.Remark,
		)
		if err != nil {
			return err
		}
	}

	for _, att := range req.Attachments {
		if _, err := tx.Exec(ctx, `
			INSERT INTO public.memo_attachment
			    (memo_id, file_path, file_name, file_size, file_type, uploaded_by)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			memoID, att.FilePath, att.FileName, att.FileSize, att.FileType, claims.UserID,
		); err != nil {
			return err
		}
	}

	if status == "PENDING_APPROVAL" {
		var hasConfig bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM public.approval_config
				WHERE doc_type='MEMO' AND step_no=1 AND is_active=true
			)`,
		).Scan(&hasConfig); err != nil {
			return err
		}
		if hasConfig {
			if _, err := tx.Exec(ctx, `
				INSERT INTO public.approval_request
				    (doc_type, doc_id, doc_no, step_no, requested_by, status)
				VALUES ('MEMO',$1,$2,1,$3,'PENDING')`,
				memoID, memoNo, claims.UserID,
			); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	m, err := h.getByID(ctx, memoID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": m})
}

// Update godoc
// @Summary      แก้ไข Memo
// @Tags         Memo
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                       true  "Memo ID"
// @Param        body  body  models.UpdateMemoRequest  true  "ข้อมูล Memo"
// @Success      200   {object}  models.Memo
// @Router       /memo/{id} [put]
func (h *MemoHandler) Update(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.UpdateMemoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Title == "" {
		return fiber.NewError(fiber.StatusBadRequest, "title is required")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least 1 line is required")
	}
	if req.RequestedBy == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "requested_by is required")
	}
	if req.ApproverID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "approver_id is required")
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE public.memo
		SET title=$1, project_code=$2, requested_by=$3, approver_id=$4,
		    department=$5, note=$6, updated_by=$7, updated_at=NOW()
		WHERE id=$8`,
		req.Title, req.ProjectCode, req.RequestedBy, req.ApproverID,
		req.Department, req.Note, claims.UserID, id,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "memo not found")
	}

	if _, err = tx.Exec(ctx, `DELETE FROM public.memo_line WHERE memo_id=$1`, id); err != nil {
		return err
	}
	for _, l := range req.Lines {
		_, err = tx.Exec(ctx, `
			INSERT INTO public.memo_line
			    (memo_id, line_no, description, unit, quantity, remark)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			id, l.LineNo, l.Description, l.Unit, l.Quantity, l.Remark,
		)
		if err != nil {
			return err
		}
	}

	if _, err = tx.Exec(ctx, `DELETE FROM public.memo_attachment WHERE memo_id=$1`, id); err != nil {
		return err
	}
	for _, att := range req.Attachments {
		if _, err = tx.Exec(ctx, `
			INSERT INTO public.memo_attachment
			    (memo_id, file_path, file_name, file_size, file_type, uploaded_by)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			id, att.FilePath, att.FileName, att.FileSize, att.FileType, claims.UserID,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	m, err := h.getByID(ctx, int64(id))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": m})
}

// Delete godoc
// @Summary      ลบ Memo
// @Tags         Memo
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Memo ID"
// @Success      200  {object}  fiber.Map
// @Router       /memo/{id} [delete]
func (h *MemoHandler) Delete(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var refCount int
	if err := h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM public.purchase_request WHERE memo_id=$1`, id,
	).Scan(&refCount); err != nil {
		return err
	}
	if refCount > 0 {
		return fiber.NewError(fiber.StatusConflict, "cannot delete memo: referenced by purchase request")
	}

	result, err := h.db.Exec(ctx, `DELETE FROM public.memo WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "memo not found")
	}

	return c.JSON(fiber.Map{"success": true, "message": "memo deleted"})
}

// Submit godoc
// @Summary      ส่ง Memo ขออนุมัติ (DRAFT → PENDING_APPROVAL)
// @Tags         Memo
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Memo ID"
// @Success      200  {object}  fiber.Map
// @Router       /memo/{id}/submit [post]
func (h *MemoHandler) Submit(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	var requestedBy int64
	if err := h.db.QueryRow(ctx,
		`SELECT status, requested_by FROM public.memo WHERE id=$1`, id,
	).Scan(&currentStatus, &requestedBy); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "memo not found")
	}
	if currentStatus != "DRAFT" && currentStatus != "REJECTED" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot submit memo with status '%s' — must be DRAFT or REJECTED", currentStatus))
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE public.memo
		SET status='PENDING_APPROVAL', updated_at=NOW(), updated_by=$1
		WHERE id=$2`, claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO public.memo_status_log (memo_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'PENDING_APPROVAL',$3,'Submitted for approval')`,
		id, currentStatus, claims.UserID,
	); err != nil {
		return err
	}

	var hasConfig bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM public.approval_config
			WHERE doc_type='MEMO' AND step_no=1 AND is_active=true
		)`,
	).Scan(&hasConfig); err != nil {
		return err
	}

	if hasConfig {
		var memoNo string
		h.db.QueryRow(ctx, `SELECT memo_no FROM public.memo WHERE id=$1`, id).Scan(&memoNo)

		if _, err := tx.Exec(ctx, `
			INSERT INTO public.approval_request
			    (doc_type, doc_id, doc_no, step_no, requested_by, status)
			VALUES ('MEMO',$1,$2,1,$3,'PENDING')`,
			id, memoNo, claims.UserID,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Memo submitted for approval",
	})
}

// Cancel godoc
// @Summary      Cancel a memo
// @Description  Cancels a memo. Allowed when status is DRAFT, REJECTED, or PENDING_APPROVAL.
// @Description  DRAFT/REJECTED can be cancelled by the owner (requested_by). PENDING_APPROVAL
// @Description  can be cancelled by an eligible approver (same eligibility check as Approve/Reject).
// @Tags         Memo
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int     true  "Memo ID"
// @Param        body  body  models.CancelMemoRequest  false  "Optional cancel reason"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      403  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /memo/{id}/cancel [patch]
func (h *MemoHandler) Cancel(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	var req models.CancelMemoRequest
	c.BodyParser(&req) // optional body, ignore parse errors on empty body

	ctx := context.Background()

	var currentStatus string
	var requestedBy int64
	if err := h.db.QueryRow(ctx,
		`SELECT status, requested_by FROM public.memo WHERE id=$1`, id,
	).Scan(&currentStatus, &requestedBy); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "memo not found")
	}

	if currentStatus != "DRAFT" && currentStatus != "REJECTED" && currentStatus != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot cancel memo with status '%s'", currentStatus))
	}

	// Permission check: owner can cancel DRAFT/REJECTED; an eligible approver can
	// cancel PENDING_APPROVAL. Mirrors isEligibleApprover's MEMO rule (approver_id
	// match or active MEMO extra-approver) used by ApprovalHandler.MyApprovalStatus
	// and GenericApprovalHandler.decide.
	isOwner := claims.UserID == requestedBy
	if currentStatus == "PENDING_APPROVAL" {
		if !isOwner {
			isAssigned, err := isEligibleApprover(ctx, h.db, "MEMO", 1, int64(id), claims.UserID)
			if err != nil {
				return err
			}
			if !isAssigned {
				return fiber.NewError(fiber.StatusForbidden, "you are not eligible to cancel this memo")
			}
		}
	} else {
		// DRAFT / REJECTED — owner only
		if !isOwner {
			return fiber.NewError(fiber.StatusForbidden, "only the memo owner can cancel this memo")
		}
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE public.memo
		SET status='CANCELLED', updated_at=NOW(), updated_by=$1
		WHERE id=$2`, claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO public.memo_status_log (memo_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'CANCELLED',$3,$4)`,
		id, currentStatus, claims.UserID, req.Comments,
	); err != nil {
		return err
	}

	// If there was a pending approval_request for this memo, close it out too
	if currentStatus == "PENDING_APPROVAL" {
		if _, err := tx.Exec(ctx, `
			UPDATE public.approval_request
			SET status='CANCELLED'
			WHERE doc_type='MEMO' AND doc_id=$1 AND status='PENDING'`, id,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "Memo cancelled"})
}

// Approve godoc
// SUPERSEDED: use PUT /approval/MEMO/{id}/approve or /reject (generic_approval.go) instead.
// Note the eligibility rule differs: this handler required an exact match on memo.approver_id,
// while the generic path uses role-based approval_config + approval_delegation + assigned_to
// (matching approval_status.go's MyApprovalStatus, per this session's explicit instruction).
// Left in place, still routed at POST /memo/{id}/approve, only for rollback safety.
// @Summary      อนุมัติหรือปฏิเสธ Memo
// @Tags         Memo
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                          true  "Memo ID"
// @Param        body  body  models.ApprovalActionRequest true  "action: APPROVE or REJECT"
// @Success      200  {object}  fiber.Map
// @Router       /memo/{id}/approve [post]
func (h *MemoHandler) Approve(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var req models.ApprovalActionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Action != "APPROVE" && req.Action != "REJECT" {
		return fiber.NewError(fiber.StatusBadRequest, "action must be APPROVE or REJECT")
	}
	if req.Action == "REJECT" && (req.Comments == nil || strings.TrimSpace(*req.Comments) == "") {
		return fiber.NewError(fiber.StatusBadRequest, "comments are required when rejecting")
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	var approverID *int64
	if err := h.db.QueryRow(ctx,
		`SELECT status, approver_id FROM public.memo WHERE id=$1`, id,
	).Scan(&currentStatus, &approverID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "memo not found")
	}
	if currentStatus != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot approve memo with status '%s' — must be PENDING_APPROVAL", currentStatus))
	}
	// Eligible = the memo's assigned approver_id, or an active MEMO extra-approver
	// (approval_delegation) — matches isEligibleApprover's MEMO rule in approval_status.go.
	isAssigned, err := isEligibleApprover(ctx, h.db, "MEMO", 1, int64(id), claims.UserID)
	if err != nil {
		return err
	}
	if !isAssigned {
		return fiber.NewError(fiber.StatusForbidden, "you are not the assigned approver for this memo")
	}

	newStatus := "APPROVED"
	if req.Action == "REJECT" {
		newStatus = "REJECTED"
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE public.memo
		SET status=$1, updated_at=NOW(), updated_by=$2
		WHERE id=$3`, newStatus, claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO public.memo_status_log (memo_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,$3,$4,$5)`,
		id, currentStatus, newStatus, claims.UserID, req.Comments,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE public.approval_request
		SET status=$1, updated_at=NOW()
		WHERE doc_type='MEMO' AND doc_id=$2 AND status='PENDING'`,
		newStatus, id,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("Memo %s", newStatus),
	})
}

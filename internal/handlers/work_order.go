package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

// WorkOrderHandler handles Work Order (หนังสือสั่งจ้าง) — a main module, same tier as PR/PO.
// Approval is NOT owned here: once doc_type='WO' rows exist in approval_doc_types +
// approval_config, PUT /approval/WO/:id/approve|reject (GenericApprovalHandler, see
// generic_approval.go) drives the actual decision, keeping WO consistent with PO/Memo
// instead of duplicating approval logic. Submit only opens the step-1 approval_request,
// mirroring MemoHandler.Submit's hasConfig guard exactly.
type WorkOrderHandler struct{ db *pgxpool.Pool }

func NewWorkOrderHandler(db *pgxpool.Pool) *WorkOrderHandler {
	return &WorkOrderHandler{db: db}
}

// Create godoc
// @Summary      สร้างหนังสือสั่งจ้าง (DRAFT)
// @Tags         WorkOrder
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateWorkOrderRequest  true  "Work Order data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /work-order [post]
func (h *WorkOrderHandler) Create(c *fiber.Ctx) error {
	var req models.CreateWorkOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.EmployerName) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "employer_name is required")
	}
	if strings.TrimSpace(req.SupplierName) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "supplier_name is required")
	}
	if req.ContractType != "LABOR_ONLY" && req.ContractType != "LABOR_MATERIAL" {
		return fiber.NewError(fiber.StatusBadRequest, "contract_type must be LABOR_ONLY or LABOR_MATERIAL")
	}
	if req.WorkSystem != "P" && req.WorkSystem != "E" && req.WorkSystem != "S" {
		return fiber.NewError(fiber.StatusBadRequest, "work_system must be P, E, or S")
	}
	if req.ContractAmount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "contract_amount must be positive")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var seq int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(id), 0)+1 FROM work_order`).Scan(&seq); err != nil {
		return err
	}
	woNo := fmt.Sprintf("WO-%s-%06d", time.Now().Format("2006"), seq)

	woDate := time.Now().Format("2006-01-02")
	if req.WoDate != nil && *req.WoDate != "" {
		woDate = *req.WoDate
	}

	var woID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO work_order (
			wo_no, wo_date, employer_name, project_code, project_scope_text,
			supplier_code, supplier_name, contact_person, supplier_address, supplier_phone,
			contract_type, work_system, contract_description, contract_amount,
			vat_rate, wht_rate, advance_pct, advance_amount, progress_payment_note,
			retention_pct, advance_deduct_pct, other_deduction_note,
			start_date, duration_days, end_date, penalty_pct_per_day, warranty_years,
			ref_no, other_terms, cost_code, status,
			entered_by, entered_at, section_head_id, authorized_by, remarks,
			created_by, updated_by
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,$10,
			$11,$12,$13,$14,
			COALESCE($15,7.00), COALESCE($16,3.00), COALESCE($17,0), COALESCE($18,0), $19,
			COALESCE($20,5.00), COALESCE($21,0), $22,
			$23,$24,$25, COALESCE($26,0), COALESCE($27,1),
			$28,$29,$30,'DRAFT',
			$31,NOW(),$32,$33,$34,
			$31,$31
		) RETURNING id`,
		woNo, woDate, req.EmployerName, req.ProjectCode, req.ProjectScopeText,
		req.SupplierCode, req.SupplierName, req.ContactPerson, req.SupplierAddress, req.SupplierPhone,
		req.ContractType, req.WorkSystem, req.ContractDescription, req.ContractAmount,
		req.VatRate, req.WhtRate, req.AdvancePct, req.AdvanceAmount, req.ProgressPaymentNote,
		req.RetentionPct, req.AdvanceDeductPct, req.OtherDeductionNote,
		req.StartDate, req.DurationDays, req.EndDate, req.PenaltyPctPerDay, req.WarrantyYears,
		req.RefNo, req.OtherTerms, req.CostCode,
		claims.UserID, req.SectionHeadID, req.AuthorizedBy, req.Remarks,
	).Scan(&woID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "insert error: "+err.Error())
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO work_order_status_log (wo_id, from_status, to_status, changed_by)
		VALUES ($1,NULL,'DRAFT',$2)`, woID, claims.UserID,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"id": woID, "wo_no": woNo},
	})
}

const workOrderSelectCols = `
	wo.id, wo.wo_no, TO_CHAR(wo.wo_date,'YYYY-MM-DD'), wo.employer_name, wo.project_code, p.project_name,
	wo.project_scope_text, wo.supplier_code, wo.supplier_name, wo.contact_person,
	wo.supplier_address, wo.supplier_phone, wo.contract_type, wo.work_system,
	wo.contract_description, wo.contract_amount, wo.vat_rate, wo.wht_rate,
	wo.advance_pct, wo.advance_amount, wo.progress_payment_note,
	wo.retention_pct, wo.advance_deduct_pct, wo.other_deduction_note,
	TO_CHAR(wo.start_date,'YYYY-MM-DD'), wo.duration_days, TO_CHAR(wo.end_date,'YYYY-MM-DD'),
	wo.penalty_pct_per_day, wo.warranty_years, wo.ref_no, wo.other_terms, wo.cost_code,
	wo.status, wo.entered_by, eu.full_name, wo.section_head_id, shu.full_name,
	wo.authorized_by, au.full_name, wo.subcontractor_signed_name, wo.remarks,
	wo.created_at, wo.updated_at`

const workOrderJoins = `
	FROM work_order wo
	LEFT JOIN project p ON p.project_code = wo.project_code
	LEFT JOIN users eu ON eu.id = wo.entered_by
	LEFT JOIN users shu ON shu.id = wo.section_head_id
	LEFT JOIN users au ON au.id = wo.authorized_by`

func scanWorkOrder(row interface{ Scan(dest ...any) error }) (*models.WorkOrder, error) {
	var w models.WorkOrder
	err := row.Scan(
		&w.ID, &w.WoNo, &w.WoDate, &w.EmployerName, &w.ProjectCode, &w.ProjectName,
		&w.ProjectScopeText, &w.SupplierCode, &w.SupplierName, &w.ContactPerson,
		&w.SupplierAddress, &w.SupplierPhone, &w.ContractType, &w.WorkSystem,
		&w.ContractDescription, &w.ContractAmount, &w.VatRate, &w.WhtRate,
		&w.AdvancePct, &w.AdvanceAmount, &w.ProgressPaymentNote,
		&w.RetentionPct, &w.AdvanceDeductPct, &w.OtherDeductionNote,
		&w.StartDate, &w.DurationDays, &w.EndDate,
		&w.PenaltyPctPerDay, &w.WarrantyYears, &w.RefNo, &w.OtherTerms, &w.CostCode,
		&w.Status, &w.EnteredBy, &w.EnteredByName, &w.SectionHeadID, &w.SectionHeadName,
		&w.AuthorizedBy, &w.AuthorizedByName, &w.SubcontractorSignedName, &w.Remarks,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// List godoc
// @Summary      รายการหนังสือสั่งจ้าง
// @Tags         WorkOrder
// @Security     BearerAuth
// @Produce      json
// @Param        status        query  string  false  "Status"
// @Param        project_code  query  string  false  "Project code"
// @Param        search        query  string  false  "Search by wo_no/supplier_name/employer_name"
// @Param        date_from     query  string  false  "Date from (YYYY-MM-DD)"
// @Param        date_to       query  string  false  "Date to (YYYY-MM-DD)"
// @Param        page          query  int     false  "Page"
// @Param        page_size     query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /work-order [get]
func (h *WorkOrderHandler) List(c *fiber.Ctx) error {
	var f models.WorkOrderFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}

	ctx := context.Background()

	where := []string{"1=1"}
	args := []any{}
	i := 1
	if f.Status != "" {
		where = append(where, fmt.Sprintf("wo.status = $%d", i))
		args = append(args, f.Status)
		i++
	}
	if f.ProjectCode != "" {
		where = append(where, fmt.Sprintf("wo.project_code = $%d", i))
		args = append(args, f.ProjectCode)
		i++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf(
			"(wo.wo_no ILIKE $%d OR wo.supplier_name ILIKE $%d OR wo.employer_name ILIKE $%d)", i, i, i))
		args = append(args, "%"+f.Search+"%")
		i++
	}
	if f.DateFrom != "" {
		where = append(where, fmt.Sprintf("wo.wo_date >= $%d", i))
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where = append(where, fmt.Sprintf("wo.wo_date <= $%d", i))
		args = append(args, f.DateTo)
		i++
	}
	whereClause := strings.Join(where, " AND ")

	var total int64
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, workOrderJoins, whereClause), args...).Scan(&total); err != nil {
		return err
	}

	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT %s
		%s
		WHERE %s
		ORDER BY wo.id DESC
		LIMIT $%d OFFSET $%d`, workOrderSelectCols, workOrderJoins, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []models.WorkOrder
	for rows.Next() {
		w, err := scanWorkOrder(rows)
		if err != nil {
			return err
		}
		items = append(items, *w)
	}
	if items == nil {
		items = []models.WorkOrder{}
	}

	totalPages := int(total) / f.PageSize
	if int(total)%f.PageSize != 0 {
		totalPages++
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: f.Page, PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// Get godoc
// @Summary      รายละเอียดหนังสือสั่งจ้าง
// @Tags         WorkOrder
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Work Order ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /work-order/{id} [get]
func (h *WorkOrderHandler) Get(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	w, err := scanWorkOrder(h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s
		%s
		WHERE wo.id=$1`, workOrderSelectCols, workOrderJoins), id))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "work order not found")
	}

	return c.JSON(fiber.Map{"success": true, "data": w})
}

// Submit godoc
// @Summary      ยื่นขออนุมัติหนังสือสั่งจ้าง (DRAFT/REJECTED → PENDING_APPROVAL)
// @Description  Opens the step-1 approval_request if doc_type='WO' step 1 is configured in
// @Description  approval_config; otherwise the status change happens with no approval request
// @Description  (mirrors MemoHandler.Submit's hasConfig guard). Actual approve/reject happens via
// @Description  PUT /approval/WO/{id}/approve|reject (GenericApprovalHandler), not here.
// @Tags         WorkOrder
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Work Order ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /work-order/{id}/submit [post]
func (h *WorkOrderHandler) Submit(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var currentStatus, woNo string
	if err := h.db.QueryRow(ctx, `SELECT status, wo_no FROM work_order WHERE id=$1`, id).
		Scan(&currentStatus, &woNo); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "work order not found")
	}
	if currentStatus != "DRAFT" && currentStatus != "REJECTED" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot submit work order with status '%s' — must be DRAFT or REJECTED", currentStatus))
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE work_order SET status='PENDING_APPROVAL', updated_at=NOW(), updated_by=$1
		WHERE id=$2`, claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO work_order_status_log (wo_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'PENDING_APPROVAL',$3,'Submitted for approval')`,
		id, currentStatus, claims.UserID,
	); err != nil {
		return err
	}

	var hasConfig bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM approval_config WHERE doc_type='WO' AND step_no=1 AND is_active=true)`,
	).Scan(&hasConfig); err != nil {
		return err
	}

	if hasConfig {
		if _, err := tx.Exec(ctx, `
			INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status, amount)
			SELECT 'WO',$1,$2,1,$3,NULL,'PENDING',contract_amount FROM work_order WHERE id=$1`,
			id, woNo, claims.UserID,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "approval request error: "+err.Error())
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "work order submitted for approval"})
}

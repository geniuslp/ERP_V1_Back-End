package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
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

// validateWorkOrderRequest holds the validation rules shared by Create and Update — both
// accept the same models.CreateWorkOrderRequest shape (Update reuses it rather than a
// dedicated UpdateWorkOrderRequest, mirroring POHandler.Update reusing CreatePORequest).
func validateWorkOrderRequest(req *models.CreateWorkOrderRequest) error {
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
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least 1 line is required")
	}
	for i, l := range req.Lines {
		if strings.TrimSpace(l.CostCode) == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: cost_code is required", i))
		}
	}
	return nil
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
	if err := validateWorkOrderRequest(&req); err != nil {
		return err
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

	useDiscount := req.UseDiscount != nil && *req.UseDiscount
	useVAT := req.UseVAT != nil && *req.UseVAT
	useWHT := req.UseWHT != nil && *req.UseWHT
	discountType := "pct"
	if req.DiscountType != nil && *req.DiscountType != "" {
		discountType = *req.DiscountType
	}

	totalAmount, discountAmount, vatAmount, whtAmount, netAmount :=
		calcWorkOrderLines(req.Lines, useDiscount, useVAT, useWHT)

	var woID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO work_order (
			wo_no, wo_date, employer_name, project_code, project_scope_text,
			supplier_code, supplier_name, contact_person, supplier_address, supplier_phone,
			contract_type, work_system, contract_description, contract_amount,
			vat_rate, wht_rate, advance_pct, advance_amount, progress_payment_note,
			retention_pct, advance_deduct_pct, other_deduction_note,
			start_date, duration_days, end_date, penalty_pct_per_day, warranty_years,
			ref_no, other_terms, status,
			entered_by, entered_at, section_head_id, authorized_by, remarks,
			use_discount, discount_type, use_vat, use_wht,
			total_amount, discount_amount, vat_amount, wht_amount, net_amount,
			created_by, updated_by
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,$10,
			$11,$12,$13,$14,
			COALESCE($15,7.00), COALESCE($16,3.00), COALESCE($17,0), COALESCE($18,0), $19,
			COALESCE($20,5.00), COALESCE($21,0), $22,
			$23,$24,$25, COALESCE($26,0), COALESCE($27,1),
			$28,$29,'DRAFT',
			$30,NOW(),$31,$32,$33,
			$34,$35,$36,$37,
			$38,$39,$40,$41,$42,
			$30,$30
		) RETURNING id`,
		woNo, woDate, req.EmployerName, req.ProjectCode, req.ProjectScopeText,
		req.SupplierCode, req.SupplierName, req.ContactPerson, req.SupplierAddress, req.SupplierPhone,
		req.ContractType, req.WorkSystem, req.ContractDescription, req.ContractAmount,
		req.VatRate, req.WhtRate, req.AdvancePct, req.AdvanceAmount, req.ProgressPaymentNote,
		req.RetentionPct, req.AdvanceDeductPct, req.OtherDeductionNote,
		req.StartDate, req.DurationDays, req.EndDate, req.PenaltyPctPerDay, req.WarrantyYears,
		req.RefNo, req.OtherTerms,
		claims.UserID, req.SectionHeadID, req.AuthorizedBy, req.Remarks,
		useDiscount, discountType, useVAT, useWHT,
		totalAmount, discountAmount, vatAmount, whtAmount, netAmount,
	).Scan(&woID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "insert error: "+err.Error())
	}

	if err := insertWorkOrderLines(ctx, tx, woID, req.Lines, claims.UserID); err != nil {
		return err
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
	wo.penalty_pct_per_day, wo.warranty_years, wo.ref_no, wo.other_terms,
	wo.status, wo.entered_by, eu.full_name, wo.section_head_id, shu.full_name,
	wo.authorized_by, au.full_name, wo.subcontractor_signed_name, wo.remarks,
	wo.created_at, wo.updated_at,
	wo.use_discount, wo.discount_type, wo.use_vat, wo.use_wht,
	wo.total_amount, wo.discount_amount, wo.vat_amount, wo.wht_amount, wo.net_amount`

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
		&w.PenaltyPctPerDay, &w.WarrantyYears, &w.RefNo, &w.OtherTerms,
		&w.Status, &w.EnteredBy, &w.EnteredByName, &w.SectionHeadID, &w.SectionHeadName,
		&w.AuthorizedBy, &w.AuthorizedByName, &w.SubcontractorSignedName, &w.Remarks,
		&w.CreatedAt, &w.UpdatedAt,
		&w.UseDiscount, &w.DiscountType, &w.UseVAT, &w.UseWHT,
		&w.TotalAmount, &w.DiscountAmount, &w.VatAmount, &w.WhtAmount, &w.NetAmount,
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

	lines, err := fetchWorkOrderLines(ctx, h.db, id)
	if err != nil {
		return err
	}
	w.Lines = lines

	return c.JSON(fiber.Map{"success": true, "data": w})
}

func fetchWorkOrderLines(ctx context.Context, db *pgxpool.Pool, woID int64) ([]models.WorkOrderLine, error) {
	rows, err := db.Query(ctx, `
		SELECT id, wo_id, sort_order, cost_code, description, qty, unit_price, amount,
		       disc, disc_type, vat_rate, wht_rate, created_at, created_by
		FROM work_order_line
		WHERE wo_id=$1
		ORDER BY sort_order`, woID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []models.WorkOrderLine{}
	for rows.Next() {
		var l models.WorkOrderLine
		if err := rows.Scan(
			&l.ID, &l.WoID, &l.SortOrder, &l.CostCode, &l.Description, &l.Qty, &l.UnitPrice, &l.Amount,
			&l.Disc, &l.DiscType, &l.VatRate, &l.WhtRate, &l.CreatedAt, &l.CreatedBy,
		); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, nil
}

// woLineCalc holds the server-computed discount/VAT/WHT amounts for one line, derived
// the same way PO computes purchase_order_line's line_discount/line_vat/line_wht (see
// po.go Create: base -> discount -> VAT -> WHT -> net), except WO stores only the raw
// disc/vat_rate/wht_rate inputs per line (per the locked frontend field set) and keeps
// calcWorkOrderLines mirrors PO's Create calculation order exactly: per-line
// base -> discount -> VAT -> WHT, then summed into header totals ->
// total_amount, discount_amount, vat_amount, wht_amount, net_amount. Per-line
// discount/VAT/WHT amounts aren't persisted (only the raw disc/vat_rate/wht_rate
// inputs are, per the locked frontend field set), so only the aggregates are returned.
func calcWorkOrderLines(lines []models.WorkOrderLineInput, useDiscount, useVAT, useWHT bool) (
	totalAmount, discountAmount, vatAmount, whtAmount, netAmount float64,
) {
	for _, l := range lines {
		base := l.Qty * l.UnitPrice
		var discAmt float64
		if useDiscount && l.Disc != nil && *l.Disc != 0 {
			discType := "pct"
			if l.DiscType != nil && *l.DiscType != "" {
				discType = *l.DiscType
			}
			if discType == "amt" {
				discAmt = *l.Disc
			} else {
				discAmt = base * (*l.Disc) / 100
			}
		}
		afterDisc := base - discAmt
		var vatAmt, whtAmt float64
		if useVAT && l.VatRate != nil {
			vatAmt = afterDisc * (*l.VatRate) / 100
		}
		if useWHT && l.WhtRate != nil {
			whtAmt = afterDisc * (*l.WhtRate) / 100
		}

		totalAmount += base
		discountAmount += discAmt
		vatAmount += vatAmt
		whtAmount += whtAmt
	}
	netAmount = totalAmount - discountAmount + vatAmount - whtAmount
	return
}

// insertWorkOrderLines inserts req.Lines into work_order_line, in order. amount is a
// generated column (qty*unit_price), so it's never in the INSERT column list; the
// computed discount/VAT/WHT amounts (woLineCalc, from calcWorkOrderLines) only feed the
// header aggregates and aren't stored per line, per the locked frontend field set.
func insertWorkOrderLines(ctx context.Context, tx pgx.Tx, woID int64, lines []models.WorkOrderLineInput, userID int64) error {
	for i, l := range lines {
		discType := "pct"
		if l.DiscType != nil && *l.DiscType != "" {
			discType = *l.DiscType
		}
		var disc float64
		if l.Disc != nil {
			disc = *l.Disc
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO work_order_line
			  (wo_id, sort_order, cost_code, description, qty, unit_price, disc, disc_type, vat_rate, wht_rate, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			woID, i+1, l.CostCode, l.Description, l.Qty, l.UnitPrice, disc, discType, l.VatRate, l.WhtRate, userID,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("lines[%d]: insert error: %s", i, err.Error()))
		}
	}
	return nil
}

// UpdateLines godoc
// @Summary      แทนที่รายการ line item (cost code) ทั้งหมดของหนังสือสั่งจ้าง
// @Tags         WorkOrder
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                              true  "Work Order ID"
// @Param        body  body  models.UpdateWorkOrderLinesRequest true  "Line items"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /work-order/{id}/lines [put]
func (h *WorkOrderHandler) UpdateLines(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.UpdateWorkOrderLinesRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least 1 line is required")
	}
	for i, l := range req.Lines {
		if strings.TrimSpace(l.CostCode) == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: cost_code is required", i))
		}
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM work_order WHERE id=$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "work order not found")
	}

	useDiscount := req.UseDiscount != nil && *req.UseDiscount
	useVAT := req.UseVAT != nil && *req.UseVAT
	useWHT := req.UseWHT != nil && *req.UseWHT
	discountType := "pct"
	if req.DiscountType != nil && *req.DiscountType != "" {
		discountType = *req.DiscountType
	}

	totalAmount, discountAmount, vatAmount, whtAmount, netAmount :=
		calcWorkOrderLines(req.Lines, useDiscount, useVAT, useWHT)

	if _, err := tx.Exec(ctx, `
		UPDATE work_order
		SET use_discount=$1, discount_type=$2, use_vat=$3, use_wht=$4,
		    total_amount=$5, discount_amount=$6, vat_amount=$7, wht_amount=$8, net_amount=$9,
		    updated_at=NOW(), updated_by=$10
		WHERE id=$11`,
		useDiscount, discountType, useVAT, useWHT,
		totalAmount, discountAmount, vatAmount, whtAmount, netAmount,
		claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM work_order_line WHERE wo_id=$1`, id); err != nil {
		return err
	}
	if err := insertWorkOrderLines(ctx, tx, id, req.Lines, claims.UserID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "lines updated"})
}

// Update godoc
// @Summary      แก้ไขหนังสือสั่งจ้าง (header + line items ทั้งใบ, เฉพาะสถานะ DRAFT)
// @Description  Accepts the same body shape as Create (models.CreateWorkOrderRequest) — lines are
// @Description  always replaced together with the header in this single call, mirroring
// @Description  POHandler.Update's all-in-one PUT rather than requiring a separate lines call.
// @Description  The dedicated PUT /work-order/{id}/lines endpoint remains available for a
// @Description  lines-only edit that doesn't touch header fields. Only allowed while
// @Description  status = DRAFT — mirrors PO's Update rule exactly (PO also rejects everything
// @Description  else, including REJECTED, and points APPROVED callers at its edit-approved
// @Description  endpoint instead; WO has no such endpoint yet, so APPROVED/other statuses just 400).
// @Description  The payload's own `status` field ("บันทึกร่าง" sends DRAFT, "ส่งอนุมัติ" sends
// @Description  PENDING_APPROVAL) is written to the row and, when it actually changes, logged to
// @Description  work_order_status_log — a transition to PENDING_APPROVAL also opens the step-1
// @Description  approval_request exactly like POST /:id/submit does, since this is an alternate
// @Description  entry point into the same submit flow. Any other status value is rejected —
// @Description  approval decisions only ever happen through the dedicated Approve/Reject flow.
// @Tags         WorkOrder
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                            true  "Work Order ID"
// @Param        body  body  models.CreateWorkOrderRequest  true  "Work Order data"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /work-order/{id} [put]
func (h *WorkOrderHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.CreateWorkOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := validateWorkOrderRequest(&req); err != nil {
		return err
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	if err := h.db.QueryRow(ctx, `SELECT status FROM work_order WHERE id=$1`, id).Scan(&currentStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "work order not found")
	}
	if currentStatus != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("work order must be DRAFT to edit (current status: %s)", currentStatus))
	}

	// Since currentStatus is guaranteed DRAFT above, the only legal transitions through this
	// endpoint are DRAFT->DRAFT (no-op save, "บันทึกร่าง") and DRAFT->PENDING_APPROVAL (inline
	// submit, "ส่งอนุมัติ"). Anything else — jumping straight to APPROVED/REJECTED/CANCELLED —
	// is rejected; those only ever happen through the dedicated Approve/Reject flow.
	newStatus := "DRAFT"
	if req.Status != nil && *req.Status != "" {
		newStatus = *req.Status
	}
	if newStatus != "DRAFT" && newStatus != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "status must be DRAFT or PENDING_APPROVAL")
	}

	woDate := ""
	if req.WoDate != nil && *req.WoDate != "" {
		woDate = *req.WoDate
	}

	useDiscount := req.UseDiscount != nil && *req.UseDiscount
	useVAT := req.UseVAT != nil && *req.UseVAT
	useWHT := req.UseWHT != nil && *req.UseWHT
	discountType := "pct"
	if req.DiscountType != nil && *req.DiscountType != "" {
		discountType = *req.DiscountType
	}

	totalAmount, discountAmount, vatAmount, whtAmount, netAmount :=
		calcWorkOrderLines(req.Lines, useDiscount, useVAT, useWHT)

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE work_order SET
			wo_date              = COALESCE(NULLIF($1,'')::date, wo_date),
			employer_name        = $2,
			project_code         = $3,
			project_scope_text   = $4,
			supplier_code        = $5,
			supplier_name        = $6,
			contact_person       = $7,
			supplier_address     = $8,
			supplier_phone       = $9,
			contract_type        = $10,
			work_system          = $11,
			contract_description = $12,
			contract_amount      = $13,
			vat_rate             = COALESCE($14,7.00),
			wht_rate             = COALESCE($15,3.00),
			advance_pct          = COALESCE($16,0),
			advance_amount       = COALESCE($17,0),
			progress_payment_note = $18,
			retention_pct        = COALESCE($19,5.00),
			advance_deduct_pct   = COALESCE($20,0),
			other_deduction_note = $21,
			start_date           = $22,
			duration_days        = COALESCE($23,0),
			end_date             = $24,
			penalty_pct_per_day  = COALESCE($25,1),
			warranty_years       = COALESCE($26,1),
			ref_no               = $27,
			other_terms          = $28,
			section_head_id      = $29,
			authorized_by        = $30,
			remarks              = $31,
			use_discount         = $32,
			discount_type        = $33,
			use_vat              = $34,
			use_wht              = $35,
			total_amount         = $36,
			discount_amount      = $37,
			vat_amount           = $38,
			wht_amount           = $39,
			net_amount           = $40,
			status               = $41,
			updated_at           = NOW(),
			updated_by           = $42
		WHERE id = $43`,
		woDate, req.EmployerName, req.ProjectCode, req.ProjectScopeText,
		req.SupplierCode, req.SupplierName, req.ContactPerson, req.SupplierAddress, req.SupplierPhone,
		req.ContractType, req.WorkSystem, req.ContractDescription, req.ContractAmount,
		req.VatRate, req.WhtRate, req.AdvancePct, req.AdvanceAmount, req.ProgressPaymentNote,
		req.RetentionPct, req.AdvanceDeductPct, req.OtherDeductionNote,
		req.StartDate, req.DurationDays, req.EndDate, req.PenaltyPctPerDay, req.WarrantyYears,
		req.RefNo, req.OtherTerms, req.SectionHeadID, req.AuthorizedBy, req.Remarks,
		useDiscount, discountType, useVAT, useWHT,
		totalAmount, discountAmount, vatAmount, whtAmount, netAmount,
		newStatus, claims.UserID, id,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "update error: "+err.Error())
	}

	if _, err := tx.Exec(ctx, `DELETE FROM work_order_line WHERE wo_id=$1`, id); err != nil {
		return err
	}
	if err := insertWorkOrderLines(ctx, tx, id, req.Lines, claims.UserID); err != nil {
		return err
	}

	if newStatus != currentStatus {
		var logRemarks *string
		if newStatus == "PENDING_APPROVAL" {
			remarks := "Submitted for approval"
			logRemarks = &remarks
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO work_order_status_log (wo_id, from_status, to_status, changed_by, remarks)
			VALUES ($1,$2,$3,$4,$5)`,
			id, currentStatus, newStatus, claims.UserID, logRemarks,
		); err != nil {
			return err
		}

		if newStatus == "PENDING_APPROVAL" {
			var woNo string
			if err := tx.QueryRow(ctx, `SELECT wo_no FROM work_order WHERE id=$1`, id).Scan(&woNo); err != nil {
				return err
			}
			if err := openWorkOrderApproval(ctx, tx, id, woNo, claims.UserID); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	w, err := scanWorkOrder(h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s
		%s
		WHERE wo.id=$1`, workOrderSelectCols, workOrderJoins), id))
	if err != nil {
		return err
	}
	lines, err := fetchWorkOrderLines(ctx, h.db, id)
	if err != nil {
		return err
	}
	w.Lines = lines

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

	if err := openWorkOrderApproval(ctx, tx, id, woNo, claims.UserID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "work order submitted for approval"})
}

// openWorkOrderApproval opens the step-1 approval_request for a WO transitioning into
// PENDING_APPROVAL, if doc_type='WO' step 1 is configured — shared by Submit and Update (the
// "ส่งอนุมัติ" button submits inline via PUT /work-order/:id with status=PENDING_APPROVAL rather
// than a separate call to POST /:id/submit, so both entry points must open the same request or
// the WO would sit in PENDING_APPROVAL with no approval_request row for anyone to act on).
func openWorkOrderApproval(ctx context.Context, tx pgx.Tx, woID int64, woNo string, requestedBy int64) error {
	var hasConfig bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM approval_config WHERE doc_type='WO' AND step_no=1 AND is_active=true)`,
	).Scan(&hasConfig); err != nil {
		return err
	}
	if !hasConfig {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status, amount)
		SELECT 'WO',$1,$2,1,$3,NULL,'PENDING',contract_amount FROM work_order WHERE id=$1`,
		woID, woNo, requestedBy,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "approval request error: "+err.Error())
	}
	return nil
}

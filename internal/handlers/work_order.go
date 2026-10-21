package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

// woContractTypeInfo is the single source of truth for the allowed contract_type values and
// the DEFAULT withholding-tax rate for each. Defaults only: saved lines keep their own
// wht_rate and stored amounts are never recomputed from this table.
type woContractTypeInfo struct {
	Code           string
	Label          string
	DefaultWhtRate float64
}

var woContractTypes = []woContractTypeInfo{
	{"LABOR_MATERIAL", "รับเหมาทั้งค่าของค่าแรง", 3},
	{"LABOR_ONLY", "รับเหมาค่าแรงอย่างเดียว", 3},
	{"RENTAL", "ค่าเช่า", 5},
	{"SERVICE", "ค่าบริการ", 3},
}

// woAllowedWhtRates are the per-line wht_rate values a Work Order line may use (1 is never a default).
var woAllowedWhtRates = []float64{1, 3, 5}

const woContractDescriptionMaxRunes = 2000

func isValidWOContractType(code string) bool {
	for _, ct := range woContractTypes {
		if ct.Code == code {
			return true
		}
	}
	return false
}

// validateWorkOrderRequest holds the validation rules shared by Create and Update. It also
// normalises the request (trims project fields, dedupes work_system) and checks that
// project_code exists, hence the db argument.
func (h *WorkOrderHandler) validateWorkOrderRequest(ctx context.Context, req *models.CreateWorkOrderRequest) error {
	if strings.TrimSpace(req.EmployerName) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณากรอกชื่อผู้ว่าจ้าง")
	}
	if strings.TrimSpace(req.SupplierName) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณากรอกชื่อผู้รับจ้าง")
	}
	if !isValidWOContractType(req.ContractType) {
		return fiber.NewError(fiber.StatusBadRequest, "ประเภทสัญญาไม่ถูกต้อง")
	}

	// Project: exactly one of project_code (existing project) or project_name_text (free text).
	if req.ProjectCode != nil {
		v := strings.TrimSpace(*req.ProjectCode)
		if v == "" {
			req.ProjectCode = nil
		} else {
			req.ProjectCode = &v
		}
	}
	if req.ProjectNameText != nil {
		v := strings.TrimSpace(*req.ProjectNameText)
		if v == "" {
			req.ProjectNameText = nil
		} else {
			req.ProjectNameText = &v
		}
	}
	switch {
	case req.ProjectCode == nil && req.ProjectNameText == nil:
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกโครงการหรือกรอกชื่อโครงการ")
	case req.ProjectCode != nil && req.ProjectNameText != nil:
		return fiber.NewError(fiber.StatusBadRequest, "ระบุได้อย่างใดอย่างหนึ่งเท่านั้น: เลือกโครงการหรือกรอกชื่อโครงการ")
	case req.ProjectNameText != nil && utf8.RuneCountInString(*req.ProjectNameText) > 200:
		return fiber.NewError(fiber.StatusBadRequest, "ชื่อโครงการต้องไม่เกิน 200 ตัวอักษร")
	case req.ProjectCode != nil:
		var exists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code=$1)`, *req.ProjectCode).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, "ไม่พบโครงการที่เลือก")
		}
	}

	if req.ContractDescription != nil && utf8.RuneCountInString(*req.ContractDescription) > woContractDescriptionMaxRunes {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("รายละเอียดสัญญาต้องไม่เกิน %d ตัวอักษร", woContractDescriptionMaxRunes))
	}

	// work_system is a text[] (multiple allowed, at least 1). New/edited documents accept only
	// "M*" job codes that exist in JobTypes (the project job type list); old P/E/S values are
	// only ever read back, never accepted here.
	if len(req.WorkSystem) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกงานระบบอย่างน้อย 1 ระบบ")
	}
	seen := make(map[string]bool, len(req.WorkSystem))
	uniq := make([]string, 0, len(req.WorkSystem))
	for _, ws := range req.WorkSystem {
		ws = strings.TrimSpace(ws)
		if !strings.HasPrefix(ws, "M") || !validJobCodes[ws] {
			return fiber.NewError(fiber.StatusBadRequest, "งานระบบต้องเป็นประเภทงานที่ขึ้นต้นด้วย M ตามรายการประเภทงานของโครงการ")
		}
		if !seen[ws] {
			seen[ws] = true
			uniq = append(uniq, ws)
		}
	}
	req.WorkSystem = uniq
	if req.SubcontractorSignedName != nil && strings.TrimSpace(*req.SubcontractorSignedName) == "" {
		req.SubcontractorSignedName = nil
	}
	if req.ContractAmount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "contract_amount must be positive")
	}
	// Lines are optional here: a DRAFT may be created/saved without any (header totals are then 0).
	// "At least 1 line" is enforced where it matters - on submit (see requireWOLinesForSubmit).
	for i, l := range req.Lines {
		if strings.TrimSpace(l.CostCode) == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: cost_code is required", i))
		}
	}
	return nil
}

// woBodyHasLines reports whether the JSON body carries a non-null "lines" key, so Update can tell
// "lines omitted" (keep existing lines + totals) from "lines: []" (replace with nothing).
func woBodyHasLines(body []byte) bool {
	var probe struct {
		Lines json.RawMessage `json:"lines"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	return len(probe.Lines) > 0 && string(probe.Lines) != "null"
}

// requireWOLinesForSubmit blocks submit-for-approval of a work order with no lines. lineCount is the
// number of lines the request carries, or -1 when the request omitted them (then the DB is counted).
// Plain Thai messages only - never a raw DB error.
func requireWOLinesForSubmit(ctx context.Context, db interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, woID int64, lineCount int) error {
	if lineCount < 0 {
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM work_order_line WHERE wo_id=$1`, woID).Scan(&lineCount); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบรายการของหนังสือสั่งจ้างได้ กรุณาลองใหม่อีกครั้ง")
		}
	}
	if lineCount == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเพิ่มรายการอย่างน้อย 1 รายการก่อนส่งขออนุมัติ")
	}
	return nil
}

// Create godoc
// @Summary      สร้างหนังสือสั่งจ้าง (DRAFT)
// @Description  `lines` may be omitted or empty: the work order is created as DRAFT with all header totals 0.
// @Description  Each line that IS sent must have a non-empty cost_code. contract_amount must be > 0.
// @Description  At least one line is required later, at submit time.
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
	if err := h.validateWorkOrderRequest(context.Background(), &req); err != nil {
		return err
	}

	// advance_deduct_amount: sent => as is; omitted => ROUND(contract_amount * advance_deduct_pct / 100, 2).
	advanceDeductAmount := computeAdvanceDeductAmount(req.AdvanceDeductAmount, req.ContractAmount, req.AdvanceDeductPct)
	if err := checkAdvanceDeductAmount(advanceDeductAmount, req.ContractAmount); err != nil {
		return err
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)
	// approver_id is optional at create (required at submit); when sent it must be in the eligible pool.
	if req.ApproverID != nil {
		if err := requireWOApprover(ctx, h.db, req.ApproverID); err != nil {
			return err
		}
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// wo_no is generated internally from wo_seq inside this transaction — the frontend's
	// create-WO page does not call reserve-number, unlike PR/PO. wo_seq (a real Postgres
	// sequence) replaces the old unlocked MAX(id)+1 read, so this stays race-safe.
	var seq int64
	if err := tx.QueryRow(ctx, `SELECT nextval('wo_seq')`).Scan(&seq); err != nil {
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
			supplier_id, supplier_name, contact_person, supplier_address, supplier_phone,
			contract_type, work_system, contract_description, contract_amount,
			vat_rate, wht_rate, advance_pct, advance_amount, progress_payment_note,
			retention_pct, advance_deduct_pct, other_deduction_note,
			start_date, duration_days, end_date, penalty_pct_per_day, warranty_years,
			ref_no, other_terms, status,
			entered_by, entered_at, section_head_id, authorized_by, remarks,
			use_discount, discount_type, use_vat, use_wht,
			total_amount, discount_amount, vat_amount, wht_amount, net_amount,
			project_name_text, subcontractor_signed_name, advance_deduct_amount, approver_id,
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
			$43,$44,COALESCE($45,0),$46,
			$30,$30
		) RETURNING id`,
		woNo, woDate, req.EmployerName, req.ProjectCode, req.ProjectScopeText,
		req.SupplierID, req.SupplierName, req.ContactPerson, req.SupplierAddress, req.SupplierPhone,
		req.ContractType, req.WorkSystem, req.ContractDescription, req.ContractAmount,
		req.VatRate, req.WhtRate, req.AdvancePct, req.AdvanceAmount, req.ProgressPaymentNote,
		req.RetentionPct, req.AdvanceDeductPct, req.OtherDeductionNote,
		req.StartDate, req.DurationDays, req.EndDate, req.PenaltyPctPerDay, req.WarrantyYears,
		req.RefNo, req.OtherTerms,
		claims.UserID, req.SectionHeadID, req.AuthorizedBy, req.Remarks,
		useDiscount, discountType, useVAT, useWHT,
		totalAmount, discountAmount, vatAmount, whtAmount, netAmount,
		req.ProjectNameText, req.SubcontractorSignedName, advanceDeductAmount, req.ApproverID,
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

// ReserveWONumber godoc
// @Summary      Reserve the next Work Order number (consumes wo_seq)
// @Description  Calls nextval('wo_seq') and formats it immediately as the real wo_no (WO-<YYYY>-NNNNNN). Unlike the old MAX(id)+1 generation, this actually consumes the sequence right away — the number is reserved even if the create is never submitted (a gap is expected and fine). The frontend calls this once when the create-WO page opens, then submits the returned wo_no as part of POST /work-order.
// @Tags         WorkOrder
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /work-order/reserve-number [get]
func (h *WorkOrderHandler) ReserveWONumber(c *fiber.Ctx) error {
	var seq int64
	if err := h.db.QueryRow(context.Background(), `SELECT nextval('wo_seq')`).Scan(&seq); err != nil {
		return err
	}
	woNo := fmt.Sprintf("WO-%s-%06d", time.Now().Format("2006"), seq)
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"wo_no": woNo}})
}

// WhtDefaults godoc
// @Summary      Contract types and their default withholding-tax rate
// @Description  Single source of truth for the allowed contract_type values and the DEFAULT wht_rate per type (LABOR_MATERIAL 3, LABOR_ONLY 3, RENTAL 5, SERVICE 3). Defaults only: saved lines keep their own wht_rate and stored amounts are never recomputed. allowed_rates are the per-line wht_rate values accepted (1, 3, 5).
// @Tags         WorkOrder
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /work-order/wht-defaults [get]
func (h *WorkOrderHandler) WhtDefaults(c *fiber.Ctx) error {
	items := make([]fiber.Map, 0, len(woContractTypes))
	for _, ct := range woContractTypes {
		items = append(items, fiber.Map{
			"contract_type":    ct.Code,
			"label":            ct.Label,
			"default_wht_rate": ct.DefaultWhtRate,
		})
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"contract_types": items,
		"allowed_rates":  woAllowedWhtRates,
	}})
}

const (
	msgWOApproverRequired    = "กรุณาเลือกผู้อนุมัติก่อนส่งขออนุมัติ"
	msgWOApproverNotEligible = "ผู้อนุมัติที่เลือกไม่มีสิทธิ์อนุมัติหนังสือสั่งจ้าง กรุณาเลือกใหม่"
)

const msgWOContractAmountLowerOnlyEditApproved = "การลดมูลค่าสัญญาทำได้เฉพาะการแก้ไขหลังอนุมัติ"

// woEditableStatuses: statuses in which the header / lines may still be edited (APPROVED, CANCELLED are locked).
var woEditableStatuses = map[string]bool{"DRAFT": true, "REJECTED": true, "PENDING_APPROVAL": true}

func woNotEditableErr(status string) error {
	if status == "APPROVED" || status == "PENDING_REAPPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("หนังสือสั่งจ้างที่มีสถานะ %s แก้ไขได้ผ่านการแก้ไขพร้อมระบุเหตุผล (PUT /work-order/{id}/edit-approved) เท่านั้น", status))
	}
	return fiber.NewError(fiber.StatusBadRequest,
		fmt.Sprintf("ไม่สามารถแก้ไขหนังสือสั่งจ้างที่มีสถานะ %s ได้ (แก้ไขได้เฉพาะ DRAFT, REJECTED หรือ PENDING_APPROVAL)", status))
}

// requireWOApprover checks the approver exists in the eligible pool for doc_type WO — the same pool
// GET /master/eligible-approvers returns (approval_config roles + approval_delegation).
func requireWOApprover(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, approverID *int64) error {
	if approverID == nil {
		return fiber.NewError(fiber.StatusBadRequest, msgWOApproverRequired)
	}
	ok, err := isEligibleApproverPick(ctx, db, "WO", *approverID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบผู้อนุมัติได้ กรุณาลองใหม่อีกครั้ง")
	}
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, msgWOApproverNotEligible)
	}
	return nil
}

const msgAdvanceDeductOverContract = "เงินล่วงหน้าที่จ่ายแล้วมากกว่ามูลค่าสัญญา กรุณาตรวจสอบ"

// computeAdvanceDeductAmount returns the sent amount, or ROUND(contract*pct/100, 2) when it was omitted.
func computeAdvanceDeductAmount(sent *float64, contractAmount float64, pct *float64) float64 {
	if sent != nil {
		return *sent
	}
	if pct == nil {
		return 0
	}
	return math.Round(contractAmount*(*pct)/100*100) / 100
}

// checkAdvanceDeductAmount rejects a negative amount or one above contract_amount (plain Thai 400, before any SQL).
func checkAdvanceDeductAmount(amount, contractAmount float64) error {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "เงินล่วงหน้าที่จ่ายแล้วต้องไม่ติดลบ")
	}
	if amount > contractAmount {
		return fiber.NewError(fiber.StatusBadRequest, msgAdvanceDeductOverContract)
	}
	return nil
}

const workOrderSelectCols = `
	wo.id, wo.wo_no, TO_CHAR(wo.wo_date,'YYYY-MM-DD'), wo.employer_name, wo.project_code, p.project_name,
	wo.project_scope_text, NULL::text AS supplier_code, wo.supplier_name, wo.contact_person,
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
	wo.total_amount, wo.discount_amount, wo.vat_amount, wo.wht_amount, wo.net_amount,
	wo.supplier_id, wo.project_name_text, wo.advance_deduct_amount, wo.approver_id,
	(SELECT COUNT(*) FROM work_order_edit_log wel WHERE wel.wo_id = wo.id) AS revision_round`

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
		&w.SupplierID, &w.ProjectNameText, &w.AdvanceDeductAmount, &w.ApproverID, &w.RevisionRound,
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
		log.Printf("work order list query: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการหนังสือสั่งจ้างได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()

	var items []models.WorkOrder
	for rows.Next() {
		w, err := scanWorkOrder(rows)
		if err != nil {
			log.Printf("work order list scan: %v", err)
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดรายการหนังสือสั่งจ้างได้ กรุณาลองใหม่อีกครั้ง")
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
		// Only a genuinely missing row is a 404; any other failure (query/scan/schema) is a 500 whose real
		// cause goes to the log, never to the client.
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "work order not found")
		}
		log.Printf("[WO Get] id=%d step=main-query/scan err=%v", id, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดข้อมูลหนังสือสั่งจ้างได้ กรุณาลองใหม่อีกครั้ง")
	}
	// Print components (WorkOrderPrintPOStyle etc.) read data.revisionRound; this endpoint is their data source.
	rr := w.RevisionRound
	w.RevisionRoundPrint = &rr

	lines, err := fetchWorkOrderLines(ctx, h.db, id)
	if err != nil {
		log.Printf("[WO Get] id=%d step=lines err=%v", id, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดข้อมูลหนังสือสั่งจ้างได้ กรุณาลองใหม่อีกครั้ง")
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
	if err := rows.Err(); err != nil {
		return nil, err
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
// @Summary      แทนที่รายการ line item (cost code) ทั้งหมดของหนังสือสั่งจ้าง (เฉพาะสถานะ DRAFT)
// @Description  Replaces ALL lines of the work order (DELETE + INSERT) and recomputes the header totals
// @Description  (total/discount/vat/wht/net) and flags (use_discount, discount_type, use_vat, use_wht) from the
// @Description  new lines in the same transaction. Only allowed while status = DRAFT (400 otherwise, same message
// @Description  as PUT /work-order/{id}); 404 if the work order does not exist. At least 1 line is required
// @Description  here (an empty or omitted `lines` is a 400) — to clear all lines use PUT /work-order/{id}
// @Description  with `lines: []`.
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

	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM work_order WHERE id=$1 FOR UPDATE`, id).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "work order not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านข้อมูลหนังสือสั่งจ้างได้ กรุณาลองใหม่อีกครั้ง")
	}
	if !woEditableStatuses[currentStatus] {
		return woNotEditableErr(currentStatus)
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
// @Description  Accepts the same body shape as Create (models.CreateWorkOrderRequest), mirroring
// @Description  POHandler.Update's all-in-one PUT. The `lines` key decides what happens to lines:
// @Description  OMITTED (or null) = existing lines are kept untouched and the header totals
// @Description  (total/discount/vat/wht/net) and their flags (use_discount, discount_type, use_vat,
// @Description  use_wht) are NOT recomputed — header fields are still updated; PRESENT, even as an
// @Description  empty array = existing lines are deleted and replaced by the sent ones and the totals
// @Description  are recomputed from them (an empty array leaves 0 lines and all-zero totals).
// @Description  Header fields that are omitted from the body are overwritten with blank/default
// @Description  values (not kept) — always send the full header. A transition to PENDING_APPROVAL
// @Description  requires at least one line (sent in this request, or already stored when `lines` is
// @Description  omitted) — otherwise 400 with a Thai message. The dedicated
// @Description  PUT /work-order/{id}/lines endpoint remains available for a lines-only edit that
// @Description  doesn't touch header fields. Only allowed while
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
func (h *WorkOrderHandler) Update(c *fiber.Ctx) error { return h.update(c, false) }

// EditApproved godoc
// @Summary      แก้ไขหนังสือสั่งจ้างที่อนุมัติแล้ว (ต้องระบุเหตุผล, ส่งกลับอนุมัติใหม่)
// @Description  Mirrors PO's PUT /po/{id}/edit-approved. Only allowed while status = APPROVED or
// @Description  PENDING_REAPPROVAL (400 otherwise). Body = the same shape as PUT /work-order/{id} plus a
// @Description  mandatory `reason` (plain Thai 400 when empty). In ONE transaction: header and lines are
// @Description  updated exactly like the normal edit (`lines` omitted = lines and totals kept; present = replaced
// @Description  and recomputed), status becomes PENDING_REAPPROVAL, a row is added to work_order_edit_log
// @Description  (this is what revision_round counts) and to work_order_status_log, and the document's existing
// @Description  approval_request is reopened (status PENDING) — same request, same assigned_to, so it goes
// @Description  back to the SAME approver. A sent approver_id and status are ignored. At least one line is
// @Description  required. wo_no never changes; the response/detail/list carry revision_round instead.
// @Tags         WorkOrder
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                            true  "Work Order ID"
// @Param        body  body  models.CreateWorkOrderRequest  true  "Work Order data + reason"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /work-order/{id}/edit-approved [put]
func (h *WorkOrderHandler) EditApproved(c *fiber.Ctx) error { return h.update(c, true) }

// update is the shared body of PUT /:id (editApproved=false) and PUT /:id/edit-approved (true).
func (h *WorkOrderHandler) update(c *fiber.Ctx, editApproved bool) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.CreateWorkOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	var reason string
	if editApproved {
		var rb struct {
			Reason string `json:"reason"`
		}
		_ = c.BodyParser(&rb)
		reason = strings.TrimSpace(rb.Reason)
		if reason == "" {
			return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุเหตุผลในการแก้ไข")
		}
		// Re-approval goes back to the same approver / same approval_request: nothing sent here may re-route it.
		req.ApproverID = nil
		req.Status = nil
	}
	if err := h.validateWorkOrderRequest(context.Background(), &req); err != nil {
		return err
	}
	// lines omitted => keep existing lines AND existing totals; lines present (even []) => replace + recompute.
	linesPresent := woBodyHasLines(c.Body())

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	var currentAdvanceDeduct, currentContractAmount float64
	var currentApprover *int64
	if err := h.db.QueryRow(ctx, `SELECT status, advance_deduct_amount, contract_amount, approver_id FROM work_order WHERE id=$1`, id).
		Scan(&currentStatus, &currentAdvanceDeduct, &currentContractAmount, &currentApprover); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "work order not found")
	}
	if editApproved {
		if currentStatus != "APPROVED" && currentStatus != "PENDING_REAPPROVAL" {
			return fiber.NewError(fiber.StatusBadRequest,
				fmt.Sprintf("แก้ไขพร้อมเหตุผลได้เฉพาะหนังสือสั่งจ้างที่มีสถานะ APPROVED หรือ PENDING_REAPPROVAL (สถานะปัจจุบัน: %s)", currentStatus))
		}
	} else if !woEditableStatuses[currentStatus] {
		return woNotEditableErr(currentStatus)
	}
	// Lowering contract_amount is only possible through edit-approved (increase / same value is fine).
	// 0.005 tolerance so numeric(…,2) round-trips never read as a decrease.
	if !editApproved && req.ContractAmount < currentContractAmount-0.005 {
		return fiber.NewError(fiber.StatusBadRequest, msgWOContractAmountLowerOnlyEditApproved)
	}
	// advance_deduct_amount: sent => stored as is; omitted => the stored amount is kept (never recomputed
	// from the pct). Either way the amount that will be stored must not exceed the (new) contract_amount.
	effectiveAdvanceDeduct := currentAdvanceDeduct
	if req.AdvanceDeductAmount != nil {
		effectiveAdvanceDeduct = *req.AdvanceDeductAmount
	}
	if err := checkAdvanceDeductAmount(effectiveAdvanceDeduct, req.ContractAmount); err != nil {
		return err
	}

	// Status: an edit never changes the status by itself. The only transition through this endpoint is
	// DRAFT/REJECTED -> PENDING_APPROVAL (inline submit, "ส่งอนุมัติ"); a PENDING_APPROVAL document stays
	// PENDING_APPROVAL whatever is sent. Anything other than DRAFT / PENDING_APPROVAL is rejected —
	// approval decisions only ever happen through the dedicated Approve/Reject flow.
	if req.Status != nil && *req.Status != "" && *req.Status != "DRAFT" && *req.Status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "status must be DRAFT or PENDING_APPROVAL")
	}
	newStatus := currentStatus
	submitting := false
	if req.Status != nil && *req.Status == "PENDING_APPROVAL" && currentStatus != "PENDING_APPROVAL" {
		newStatus = "PENDING_APPROVAL"
		submitting = true
	}

	if submitting {
		lineCount := -1
		if linesPresent {
			lineCount = len(req.Lines)
		}
		if err := requireWOLinesForSubmit(ctx, h.db, id, lineCount); err != nil {
			return err
		}
	}

	if editApproved {
		// Edit-approved always re-submits: it needs at least one line (sent now, or already stored).
		lineCount := -1
		if linesPresent {
			lineCount = len(req.Lines)
		}
		if err := requireWOLinesForSubmit(ctx, h.db, id, lineCount); err != nil {
			return err
		}
		newStatus = "PENDING_REAPPROVAL"
	}

	// approver_id: omitted => the stored value is kept. A sent value that differs from the stored one is
	// validated against the eligible pool; a submit always needs one (sent now or already stored).
	effectiveApprover := currentApprover
	approverChanged := false
	if req.ApproverID != nil {
		effectiveApprover = req.ApproverID
		approverChanged = currentApprover == nil || *currentApprover != *req.ApproverID
	}
	if submitting && effectiveApprover == nil {
		return fiber.NewError(fiber.StatusBadRequest, msgWOApproverRequired)
	}
	if approverChanged || submitting {
		if err := requireWOApprover(ctx, h.db, effectiveApprover); err != nil {
			return err
		}
	}

	woDate := ""
	if req.WoDate != nil && *req.WoDate != "" {
		woDate = *req.WoDate
	}

	// Totals and the flags that drive them (use_discount/discount_type/use_vat/use_wht) are written
	// only when lines are sent; nil => COALESCE keeps the stored value so totals never desync.
	var useDiscount, useVAT, useWHT *bool
	var discountType *string
	var totalAmount, discountAmount, vatAmount, whtAmount, netAmount *float64
	if linesPresent {
		ud := req.UseDiscount != nil && *req.UseDiscount
		uv := req.UseVAT != nil && *req.UseVAT
		uw := req.UseWHT != nil && *req.UseWHT
		dt := "pct"
		if req.DiscountType != nil && *req.DiscountType != "" {
			dt = *req.DiscountType
		}
		ta, da, va, wa, na := calcWorkOrderLines(req.Lines, ud, uv, uw)
		useDiscount, useVAT, useWHT, discountType = &ud, &uv, &uw, &dt
		totalAmount, discountAmount, vatAmount, whtAmount, netAmount = &ta, &da, &va, &wa, &na
	}

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
			supplier_id          = $5,
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
			use_discount         = COALESCE($32, use_discount),
			discount_type        = COALESCE($33, discount_type),
			use_vat              = COALESCE($34, use_vat),
			use_wht              = COALESCE($35, use_wht),
			total_amount         = COALESCE($36, total_amount),
			discount_amount      = COALESCE($37, discount_amount),
			vat_amount           = COALESCE($38, vat_amount),
			wht_amount           = COALESCE($39, wht_amount),
			net_amount           = COALESCE($40, net_amount),
			status               = $41,
			updated_at           = NOW(),
			updated_by           = $42,
			project_name_text    = $44,
			subcontractor_signed_name = $45,
			advance_deduct_amount = COALESCE($46, advance_deduct_amount),
			approver_id = COALESCE($47, approver_id)
		WHERE id = $43`,
		woDate, req.EmployerName, req.ProjectCode, req.ProjectScopeText,
		req.SupplierID, req.SupplierName, req.ContactPerson, req.SupplierAddress, req.SupplierPhone,
		req.ContractType, req.WorkSystem, req.ContractDescription, req.ContractAmount,
		req.VatRate, req.WhtRate, req.AdvancePct, req.AdvanceAmount, req.ProgressPaymentNote,
		req.RetentionPct, req.AdvanceDeductPct, req.OtherDeductionNote,
		req.StartDate, req.DurationDays, req.EndDate, req.PenaltyPctPerDay, req.WarrantyYears,
		req.RefNo, req.OtherTerms, req.SectionHeadID, req.AuthorizedBy, req.Remarks,
		useDiscount, discountType, useVAT, useWHT,
		totalAmount, discountAmount, vatAmount, whtAmount, netAmount,
		newStatus, claims.UserID, id,
		req.ProjectNameText, req.SubcontractorSignedName, req.AdvanceDeductAmount, req.ApproverID,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "update error: "+err.Error())
	}

	if linesPresent {
		if _, err := tx.Exec(ctx, `DELETE FROM work_order_line WHERE wo_id=$1`, id); err != nil {
			return err
		}
		if err := insertWorkOrderLines(ctx, tx, id, req.Lines, claims.UserID); err != nil {
			return err
		}
	}

	if editApproved {
		if _, err := tx.Exec(ctx, `
			INSERT INTO work_order_edit_log (wo_id, edited_by, reason, edited_at)
			VALUES ($1,$2,$3,NOW())`, id, claims.UserID, reason); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "edit log error: "+err.Error())
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO work_order_status_log (wo_id, from_status, to_status, changed_by, remarks)
			VALUES ($1,$2,'PENDING_REAPPROVAL',$3,$4)`, id, currentStatus, claims.UserID, reason); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "status log error: "+err.Error())
		}
		// Reopen the document's existing approval_request (same id / assigned_to — routing is not recomputed).
		tag, err := tx.Exec(ctx, `
			UPDATE approval_request SET status='PENDING', updated_at=NOW()
			WHERE id = (
				SELECT id FROM approval_request
				WHERE doc_type='WO' AND doc_id=$1
				ORDER BY created_at DESC, id DESC LIMIT 1
			)`, id)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "approval_request update error: "+err.Error())
		}
		if tag.RowsAffected() == 0 {
			return fiber.NewError(fiber.StatusConflict, "ไม่พบคำขออนุมัติเดิมของหนังสือสั่งจ้างนี้ จึงไม่สามารถส่งกลับเพื่ออนุมัติใหม่ได้")
		}
	} else if newStatus != currentStatus {
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
			if err := openWorkOrderApproval(ctx, tx, id, woNo, claims.UserID, *effectiveApprover); err != nil {
				return err
			}
		}
	} else if approverChanged && currentStatus == "PENDING_APPROVAL" {
		// Explicitly re-assigned while pending: move the open request to the new approver (same as Memo).
		if _, err := tx.Exec(ctx, `
			UPDATE approval_request SET assigned_to=$1, updated_at=NOW()
			WHERE doc_type='WO' AND doc_id=$2 AND status='PENDING'`, *effectiveApprover, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO work_order_status_log (wo_id, from_status, to_status, changed_by, remarks)
			VALUES ($1,'PENDING_APPROVAL','PENDING_APPROVAL',$2,$3)`,
			id, claims.UserID, fmt.Sprintf("approver changed to user %d", *effectiveApprover)); err != nil {
			return err
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
// @Description  Requires at least one line (400 with a Thai message otherwise). Opens the step-1 approval_request
// @Description  if doc_type='WO' step 1 is configured in
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
	var approverID *int64
	if err := h.db.QueryRow(ctx, `SELECT status, wo_no, approver_id FROM work_order WHERE id=$1`, id).
		Scan(&currentStatus, &woNo, &approverID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "work order not found")
	}
	if currentStatus != "DRAFT" && currentStatus != "REJECTED" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot submit work order with status '%s' — must be DRAFT or REJECTED", currentStatus))
	}

	if err := requireWOLinesForSubmit(ctx, h.db, id, -1); err != nil {
		return err
	}
	if err := requireWOApprover(ctx, h.db, approverID); err != nil {
		return err
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

	if err := openWorkOrderApproval(ctx, tx, id, woNo, claims.UserID, *approverID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "work order submitted for approval"})
}

// ─── Payment Conditions (installments / retention / penalty) ──────────────────

func fetchWorkOrderPaymentInstallments(ctx context.Context, db *pgxpool.Pool, woID int64) ([]models.WorkOrderPaymentInstallment, error) {
	rows, err := db.Query(ctx, `
		SELECT id, installment_no, description, percent_of_contract, amount,
		       TO_CHAR(due_date,'YYYY-MM-DD'), payment_status, TO_CHAR(paid_date,'YYYY-MM-DD'), remarks
		FROM work_order_payment_installment
		WHERE wo_id=$1
		ORDER BY installment_no ASC`, woID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.WorkOrderPaymentInstallment{}
	for rows.Next() {
		var it models.WorkOrderPaymentInstallment
		if err := rows.Scan(&it.ID, &it.InstallmentNo, &it.Description, &it.PercentOfContract, &it.Amount,
			&it.DueDate, &it.PaymentStatus, &it.PaidDate, &it.Remarks); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

func fetchWorkOrderRetentions(ctx context.Context, db *pgxpool.Pool, woID int64) ([]models.WorkOrderRetention, error) {
	rows, err := db.Query(ctx, `
		SELECT id, description, percent_of_contract, amount, remarks
		FROM work_order_retention
		WHERE wo_id=$1
		ORDER BY id ASC`, woID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.WorkOrderRetention{}
	for rows.Next() {
		var it models.WorkOrderRetention
		if err := rows.Scan(&it.ID, &it.Description, &it.PercentOfContract, &it.Amount, &it.Remarks); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

func fetchWorkOrderPenalties(ctx context.Context, db *pgxpool.Pool, woID int64) ([]models.WorkOrderPenalty, error) {
	rows, err := db.Query(ctx, `
		SELECT id, description, percent_per_day, TO_CHAR(contract_start_date,'YYYY-MM-DD'),
		       TO_CHAR(contract_end_date,'YYYY-MM-DD'), remarks
		FROM work_order_penalty
		WHERE wo_id=$1
		ORDER BY id ASC`, woID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.WorkOrderPenalty{}
	for rows.Next() {
		var it models.WorkOrderPenalty
		if err := rows.Scan(&it.ID, &it.Description, &it.PercentPerDay, &it.ContractStartDate,
			&it.ContractEndDate, &it.Remarks); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

// GetPaymentConditions godoc
// @Summary      ดึงเงื่อนไขการชำระเงินของหนังสือสั่งจ้าง (งวดชำระ/เงินประกันผลงาน/ค่าปรับ)
// @Description  Fetches all 3 payment-condition lists for this WO in one call: installments (work_order_payment_installment), retentions (work_order_retention), penalties (work_order_penalty).
// @Tags         WorkOrder
// @Security     BearerAuth
// @Produce      json
// @Param        woId  path  int  true  "Work Order ID"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /work-order/{woId}/payment-conditions [get]
func (h *WorkOrderHandler) GetPaymentConditions(c *fiber.Ctx) error {
	woID, err := strconv.ParseInt(c.Params("woId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid woId")
	}
	ctx := context.Background()

	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM work_order WHERE id=$1)`, woID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "work order not found")
	}

	installments, err := fetchWorkOrderPaymentInstallments(ctx, h.db, woID)
	if err != nil {
		return err
	}
	retentions, err := fetchWorkOrderRetentions(ctx, h.db, woID)
	if err != nil {
		return err
	}
	penalties, err := fetchWorkOrderPenalties(ctx, h.db, woID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"installments": installments,
			"retentions":   retentions,
			"penalties":    penalties,
		},
	})
}

// UpdatePaymentConditions godoc
// @Summary      บันทึกเงื่อนไขการชำระเงินของหนังสือสั่งจ้าง (แทนที่ทั้งหมดทั้ง 3 รายการ)
// @Description  Full replace-sync for all 3 tables in one transaction: rows with an id are updated, rows without one are inserted, and any existing row for this wo_id not present in the submitted array is deleted. No cross-row validation (e.g. percentages summing to 100) is applied — only what's listed here. installments.payment_status must be UNPAID or PAID; switching to PAID without paid_date auto-fills it to today, switching back to UNPAID clears paid_date.
// @Tags         WorkOrder
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        woId  path  int  true  "Work Order ID"
// @Param        body  body  models.UpdateWorkOrderPaymentConditionsRequest  true  "Full desired state of installments/retentions/penalties"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /work-order/{woId}/payment-conditions [post]
func (h *WorkOrderHandler) UpdatePaymentConditions(c *fiber.Ctx) error {
	woID, err := strconv.ParseInt(c.Params("woId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid woId")
	}
	var req models.UpdateWorkOrderPaymentConditionsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	for i, ins := range req.Installments {
		if ins.PaymentStatus != "UNPAID" && ins.PaymentStatus != "PAID" {
			return fiber.NewError(fiber.StatusBadRequest,
				fmt.Sprintf("installments[%d]: payment_status must be UNPAID or PAID", i))
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
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM work_order WHERE id=$1)`, woID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "work order not found")
	}

	today := time.Now().Format("2006-01-02")

	// ── installments ──
	keepInstallmentIDs := []int64{}
	for _, ins := range req.Installments {
		if ins.ID != nil {
			keepInstallmentIDs = append(keepInstallmentIDs, *ins.ID)
		}
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM work_order_payment_installment
		WHERE wo_id=$1 AND NOT (id = ANY($2))`, woID, keepInstallmentIDs,
	); err != nil {
		return err
	}
	for i, ins := range req.Installments {
		paidDate := ins.PaidDate
		if ins.PaymentStatus == "PAID" && (paidDate == nil || *paidDate == "") {
			paidDate = &today
		} else if ins.PaymentStatus == "UNPAID" {
			paidDate = nil
		}
		if ins.ID != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE work_order_payment_installment SET
					installment_no=$1, description=$2, percent_of_contract=$3, amount=$4,
					due_date=NULLIF($5,'')::date, payment_status=$6, paid_date=$7, remarks=$8,
					updated_at=NOW(), updated_by=$9
				WHERE id=$10 AND wo_id=$11`,
				ins.InstallmentNo, ins.Description, ins.PercentOfContract, ins.Amount,
				ins.DueDate, ins.PaymentStatus, paidDate, ins.Remarks,
				claims.UserID, *ins.ID, woID,
			); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("installments[%d]: update error: %s", i, err.Error()))
			}
		} else {
			if _, err := tx.Exec(ctx, `
				INSERT INTO work_order_payment_installment
				  (wo_id, installment_no, description, percent_of_contract, amount, due_date, payment_status, paid_date, remarks, created_by, updated_by)
				VALUES ($1,$2,$3,$4,$5, NULLIF($6,'')::date, $7,$8,$9,$10,$10)`,
				woID, ins.InstallmentNo, ins.Description, ins.PercentOfContract, ins.Amount,
				ins.DueDate, ins.PaymentStatus, paidDate, ins.Remarks, claims.UserID,
			); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("installments[%d]: insert error: %s", i, err.Error()))
			}
		}
	}

	// ── retentions ──
	keepRetentionIDs := []int64{}
	for _, r := range req.Retentions {
		if r.ID != nil {
			keepRetentionIDs = append(keepRetentionIDs, *r.ID)
		}
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM work_order_retention
		WHERE wo_id=$1 AND NOT (id = ANY($2))`, woID, keepRetentionIDs,
	); err != nil {
		return err
	}
	for i, r := range req.Retentions {
		if r.ID != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE work_order_retention SET
					description=$1, percent_of_contract=$2, amount=$3, remarks=$4,
					updated_at=NOW(), updated_by=$5
				WHERE id=$6 AND wo_id=$7`,
				r.Description, r.PercentOfContract, r.Amount, r.Remarks,
				claims.UserID, *r.ID, woID,
			); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("retentions[%d]: update error: %s", i, err.Error()))
			}
		} else {
			if _, err := tx.Exec(ctx, `
				INSERT INTO work_order_retention
				  (wo_id, description, percent_of_contract, amount, remarks, created_by, updated_by)
				VALUES ($1,$2,$3,$4,$5,$6,$6)`,
				woID, r.Description, r.PercentOfContract, r.Amount, r.Remarks, claims.UserID,
			); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("retentions[%d]: insert error: %s", i, err.Error()))
			}
		}
	}

	// ── penalties ──
	keepPenaltyIDs := []int64{}
	for _, p := range req.Penalties {
		if p.ID != nil {
			keepPenaltyIDs = append(keepPenaltyIDs, *p.ID)
		}
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM work_order_penalty
		WHERE wo_id=$1 AND NOT (id = ANY($2))`, woID, keepPenaltyIDs,
	); err != nil {
		return err
	}
	for i, p := range req.Penalties {
		if p.ID != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE work_order_penalty SET
					description=$1, percent_per_day=$2,
					contract_start_date=NULLIF($3,'')::date, contract_end_date=NULLIF($4,'')::date, remarks=$5,
					updated_at=NOW(), updated_by=$6
				WHERE id=$7 AND wo_id=$8`,
				p.Description, p.PercentPerDay, p.ContractStartDate, p.ContractEndDate, p.Remarks,
				claims.UserID, *p.ID, woID,
			); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("penalties[%d]: update error: %s", i, err.Error()))
			}
		} else {
			if _, err := tx.Exec(ctx, `
				INSERT INTO work_order_penalty
				  (wo_id, description, percent_per_day, contract_start_date, contract_end_date, remarks, created_by, updated_by)
				VALUES ($1,$2,$3, NULLIF($4,'')::date, NULLIF($5,'')::date, $6,$7,$7)`,
				woID, p.Description, p.PercentPerDay, p.ContractStartDate, p.ContractEndDate, p.Remarks, claims.UserID,
			); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("penalties[%d]: insert error: %s", i, err.Error()))
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	installments, err := fetchWorkOrderPaymentInstallments(ctx, h.db, woID)
	if err != nil {
		return err
	}
	retentions, err := fetchWorkOrderRetentions(ctx, h.db, woID)
	if err != nil {
		return err
	}
	penalties, err := fetchWorkOrderPenalties(ctx, h.db, woID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"installments": installments,
			"retentions":   retentions,
			"penalties":    penalties,
		},
	})
}

// openWorkOrderApproval opens the step-1 approval_request for a WO transitioning into
// PENDING_APPROVAL, if doc_type='WO' step 1 is configured — shared by Submit and Update (the
// "ส่งอนุมัติ" button submits inline via PUT /work-order/:id with status=PENDING_APPROVAL rather
// than a separate call to POST /:id/submit, so both entry points must open the same request or
// the WO would sit in PENDING_APPROVAL with no approval_request row for anyone to act on).
func openWorkOrderApproval(ctx context.Context, tx pgx.Tx, woID int64, woNo string, requestedBy, approverID int64) error {
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
		SELECT 'WO',$1,$2,1,$3,$4,'PENDING',contract_amount FROM work_order WHERE id=$1`,
		woID, woNo, requestedBy, approverID,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "approval request error: "+err.Error())
	}
	return nil
}

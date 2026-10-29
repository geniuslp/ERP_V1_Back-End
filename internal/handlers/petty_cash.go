package handlers

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

// PettyCashHandler drives the petty cash requisition (ใบเบิกเงินสดย่อย) document — a request
// for cash reimbursement/advance to buy materials. It is NOT a stock-issue document: mat_code
// on each line is reference-only for the material picker / project_stock display, and nothing
// here writes to stock_item, stock_inventory, project_stock, or any inventory_transaction table
// (see CLAUDE.md — inventory/inventory_transaction are not used in production at all).
//
// One document can span multiple projects: project_code lives on petty_cash_requisition_line
// (nullable since 2026-10: a department-only requisition has NULL here), not the header. Every
// stock/project join below is done per-LINE, never against a single header project.
//
// Discount/VAT/WHT math mirrors POHandler.Create/Update exactly (internal/handlers/po.go):
// per line -> base = qty*unit_price, discount (pct of base or flat amt), VAT = 7% of
// (base-discount) when use_vat, WHT = wht_rate% of (base-discount) when use_wht and a rate is
// given, line_net = (base-discount)+vat-wht. Header totals sum the per-line components, with
// header vat_amount recomputed as 7% of (total-discount) rather than summed from lines (same
// as PO) to avoid rounding drift.
//
// The material picker reuses GET /master/allMaterial (master.go's GetAllMaterial) — the same
// hierarchical group->subgroup->mat_name->spec->brand picker already used elsewhere — extended
// with an optional project_code param that adds a read-only stock_on_hand hint per material
// (LEFT JOIN LATERAL project_stock). There is no petty-cash-specific material search endpoint.
//
// Approval follows the PO/MEMO per-document approver_id model (see isEligibleApprover's doc
// comment in approval_status.go): a specific approver is chosen client-side from the
// role-eligible pool (GET /master/eligible-approvers?doc_type=PETTY_CASH) and stored as
// petty_cash_requisition.approver_id. approval_doc_types already has a PETTY_CASH row wired to
// petty_cash_requisition/petty_cash_status_log, so GenericApprovalHandler's
// PUT /approval/PETTY_CASH/:id/approve|reject also works once isEligibleApprover knows about
// PETTY_CASH — Approve/Reject below are the doc-specific equivalents (mirrors MemoHandler.Approve).
type PettyCashHandler struct {
	db *pgxpool.Pool
}

func NewPettyCashHandler(db *pgxpool.Pool) *PettyCashHandler {
	return &PettyCashHandler{db: db}
}

// pcHeaderLimits are the varchar limits of the Tab A header columns (verified against
// information_schema); lengths are counted in runes, not bytes, because most of this text is Thai.
var pcHeaderLimits = []struct {
	label string
	max   int
	get   func(r *models.CreatePettyCashRequest) string
}{
	{"doc_type", 30, func(r *models.CreatePettyCashRequest) string { return r.DocType }},
	{"branch_code", 20, func(r *models.CreatePettyCashRequest) string { return r.BranchCode }},
	{"payee_emp_code", 30, func(r *models.CreatePettyCashRequest) string { return r.PayeeEmpCode }},
	{"payee_name", 200, func(r *models.CreatePettyCashRequest) string { return r.PayeeName }},
	{"project_code", 20, func(r *models.CreatePettyCashRequest) string { return r.ProjectCode }},
	{"project_name", 200, func(r *models.CreatePettyCashRequest) string { return r.ProjectName }},
	{"dept_code", 20, func(r *models.CreatePettyCashRequest) string { return r.DeptCode }},
	{"dept_name", 200, func(r *models.CreatePettyCashRequest) string { return r.DeptName }},
	{"ref_doc_no", 50, func(r *models.CreatePettyCashRequest) string { return r.RefDocNo }},
	{"currency", 10, func(r *models.CreatePettyCashRequest) string { return r.Currency }},
}

func pcNilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// prepareHeader trims and validates the Tab A header fields before any SQL runs, applies the
// doc_type default, and pushes the header project_code down into lines that left theirs empty.
func (h *PettyCashHandler) prepareHeader(ctx context.Context, req *models.CreatePettyCashRequest) error {
	req.DocType = strings.TrimSpace(req.DocType)
	req.BranchCode = strings.TrimSpace(req.BranchCode)
	req.PayeeEmpCode = strings.TrimSpace(req.PayeeEmpCode)
	req.PayeeName = strings.TrimSpace(req.PayeeName)
	req.ProjectCode = strings.TrimSpace(req.ProjectCode)
	req.ProjectName = strings.TrimSpace(req.ProjectName)
	req.OrderType = strings.TrimSpace(req.OrderType)
	req.DeptCode = strings.TrimSpace(req.DeptCode)
	req.DeptName = strings.TrimSpace(req.DeptName)
	req.RefDocNo = strings.TrimSpace(req.RefDocNo)
	req.Currency = strings.TrimSpace(req.Currency)
	if req.DocType == "" {
		req.DocType = "PETTY_CASH"
	}
	// A requisition may be department-only, but it needs a department or a project somewhere:
	// header project, header department, or at least one line with a project. Checked before any SQL.
	if req.ProjectCode == "" && req.DeptCode == "" {
		hasLineProject := false
		for _, l := range req.Lines {
			if strings.TrimSpace(l.ProjectCode) != "" {
				hasLineProject = true
				break
			}
		}
		if !hasLineProject {
			return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกแผนกหรือโครงการอย่างน้อยหนึ่งอย่าง")
		}
	}
	for _, f := range pcHeaderLimits {
		if n := utf8.RuneCountInString(f.get(req)); n > f.max {
			return fiber.NewError(fiber.StatusBadRequest,
				fmt.Sprintf("%s ยาวเกินกำหนด (สูงสุด %d ตัวอักษร, ส่งมา %d)", f.label, f.max, n))
		}
	}
	if req.OrderType != "" {
		if err := validateOrderType(req.OrderType); err != nil {
			return err
		}
	}
	if req.ProjectCode != "" {
		var exists bool
		if err := h.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM project WHERE project_code=$1)`, req.ProjectCode,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, "ไม่พบโครงการที่ระบุ (project_code ไม่ถูกต้อง)")
		}
	}
	for i := range req.Lines {
		req.Lines[i].ProjectCode = strings.TrimSpace(req.Lines[i].ProjectCode)
		if req.Lines[i].ProjectCode == "" {
			req.Lines[i].ProjectCode = req.ProjectCode
		}
	}
	return nil
}

const pcRemarksRequiredMsg = "กรุณากรอกหมายเหตุผู้เบิก"
const pcLinesRequiredMsg = "กรุณาเพิ่มรายการอย่างน้อย 1 รายการ"

// pcVatRate is the fixed VAT rate. po.go has no shared constant — it repeats the literal 0.07 inline
// (lines ~1166/1180 etc.) — so this is the one definition petty cash uses; PO is left untouched.
// pcVatRatePercent is the same value as a whole percent for the line-options response; both are
// untyped constants, so 0.07*100 is exactly 7.
const (
	pcVatRate        = 0.07
	pcVatRatePercent = pcVatRate * 100
)

// pcTaxForms is the single list of allowed tax_form_code values: used by validation and by
// GET /petty-cash/line-options (the DB CHECK chk_pcl_tax_form allows the same two codes).
var pcTaxForms = []struct {
	Code    string `json:"code"`
	LabelTH string `json:"label_th"`
}{
	{"PND3", "ภ.ง.ด.3 (บุคคลธรรมดา)"},
	{"PND53", "ภ.ง.ด.53 (นิติบุคคล)"},
}

func isPCTaxForm(code string) bool {
	for _, f := range pcTaxForms {
		if f.Code == code {
			return true
		}
	}
	return false
}

// pcLine is one fully validated, calculated line, ready to INSERT.
type pcLine struct {
	projectCode                             *string // nil = department-only line (project_code is nullable)
	matCode                                 *string
	unit                                    *string
	costSubgroupID                          *int64
	description, remarks                    *string
	amount, vat, wht, net                   float64
	qty                                     float64 // จำนวน (units); amount stays the entered money total
	supplierID                              *int64
	supplierName, supplierAddr, supplierTax *string
	hasTaxInvoice                           bool
	taxInvoiceNo                            *string
	taxInvoiceDate                          *time.Time
	hasWht                                  bool
	whtBase, whtRate                        *float64
	taxForm                                 *string
	whtIncomeTypeID                         *int64
	whtPayDate                              *time.Time
	invoiceNo                               *string    // plain (non-tax) invoice, independent of has_tax_invoice
	invoiceDate                             *time.Time // plain (non-tax) invoice date
}

type pcTotals struct {
	total, vat, wht, net float64
	useVat, useWht       bool
}

// pcRound2 rounds a money value to 2 decimals (half away from zero) — the server-side figure for
// line_vat / line_wht / line_net and the header totals, so they match the line modal's display.
func pcRound2(v float64) float64 {
	return math.Round(v*100) / 100
}

func pcParseDate(s string) (*time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, false
	}
	return &t, true
}

// resolveLines validates every line (Thai errors, with the line number, before any write) and
// calculates it. line_vat / line_wht / line_net and the header totals are rounded to 2 decimals
// server-side (pcRound2): VAT = round(amount*7%), WHT = round(wht_base*rate/100), only when the
// matching has_tax_invoice / has_wht flag is on, else 0.
// prepareHeader must run first so empty line project_codes already fell back to the header's.
func (h *PettyCashHandler) resolveLines(ctx context.Context, req *models.CreatePettyCashRequest) ([]pcLine, pcTotals, error) {
	var tot pcTotals
	out := make([]pcLine, 0, len(req.Lines))

	// per-request lookup caches so a 50-line doc doesn't run 50 identical existence checks
	projOK := map[string]bool{}
	matOK := map[string]bool{}
	costOK := map[int64]bool{}

	for i, l := range req.Lines {
		n := i + 1
		bad := func(msg string) error {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("รายการที่ %d: %s", n, msg))
		}
		var ln pcLine

		// ── project ──
		// project is optional (department-only requisitions): empty is stored as NULL.
		if pcode := strings.TrimSpace(l.ProjectCode); pcode != "" {
			if _, ok := projOK[pcode]; !ok {
				var ex bool
				if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code=$1)`, pcode).Scan(&ex); err != nil {
					return nil, tot, err
				}
				projOK[pcode] = ex
			}
			if !projOK[pcode] {
				return nil, tot, bad("ไม่พบโครงการที่ระบุ")
			}
			ln.projectCode = &pcode
		}

		// ── name / material / cost code ──
		mat := strings.TrimSpace(l.MatCode)
		desc := strings.TrimSpace(l.Description)
		if mat == "" && desc == "" {
			return nil, tot, bad("กรุณาระบุรหัสวัสดุหรือรายละเอียดรายการ")
		}
		if utf8.RuneCountInString(mat) > 20 {
			return nil, tot, bad("รหัสวัสดุยาวเกินกำหนด (สูงสุด 20 ตัวอักษร)")
		}
		if mat != "" {
			if _, ok := matOK[mat]; !ok {
				var ex bool
				if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM material_code WHERE mat_code=$1)`, mat).Scan(&ex); err != nil {
					return nil, tot, err
				}
				matOK[mat] = ex
			}
			if !matOK[mat] {
				return nil, tot, bad("ไม่พบรหัสวัสดุที่ระบุ")
			}
		}
		ln.matCode = pcNilIfEmpty(mat)
		ln.description = pcNilIfEmpty(desc)
		unit := strings.TrimSpace(l.Unit)
		if unit == "" {
			return nil, tot, bad("กรุณาระบุหน่วย")
		}
		if utf8.RuneCountInString(unit) > 50 {
			return nil, tot, bad("หน่วยยาวเกินกำหนด (สูงสุด 50 ตัวอักษร)")
		}
		ln.unit = pcNilIfEmpty(unit)
		ln.remarks = l.Remarks
		if l.CostSubgroupID != nil {
			id := *l.CostSubgroupID
			if _, ok := costOK[id]; !ok {
				var ex bool
				if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cost_subgroup WHERE id=$1)`, id).Scan(&ex); err != nil {
					return nil, tot, err
				}
				costOK[id] = ex
			}
			if !costOK[id] {
				return nil, tot, bad("ไม่พบรหัสต้นทุน (Cost Code) ที่ระบุ")
			}
			ln.costSubgroupID = l.CostSubgroupID
		}

		// ── amount (before VAT) ──
		var amount float64
		switch {
		case l.Amount != nil:
			amount = *l.Amount
		case l.Qty > 0 && l.UnitPrice > 0: // legacy callers: amount = qty*unit_price
			amount = l.Qty * l.UnitPrice
		}
		if amount <= 0 {
			return nil, tot, bad("จำนวนเงินต้องมากกว่า 0")
		}
		ln.amount = amount
		// qty is the quantity of the item (info only, amount is not derived from it); default 1
		ln.qty = 1
		if l.Qty > 0 {
			ln.qty = l.Qty
		}

		// ── vendor ──
		if l.SupplierID != nil && *l.SupplierID > 0 {
			var name string
			var taxID, addr *string
			err := h.db.QueryRow(ctx, `SELECT supplier_name, tax_id, address FROM supplier WHERE id=$1`, *l.SupplierID).
				Scan(&name, &taxID, &addr)
			if err == pgx.ErrNoRows {
				return nil, tot, bad("ไม่พบผู้ขายที่ระบุ")
			}
			if err != nil {
				return nil, tot, err
			}
			// the server snapshots the supplier master; client-sent name/address/tax id are ignored
			ln.supplierID = l.SupplierID
			ln.supplierName, ln.supplierTax, ln.supplierAddr = pcNilIfEmpty(strings.TrimSpace(name)), taxID, addr
			if ln.supplierTax != nil && utf8.RuneCountInString(*ln.supplierTax) > 50 {
				ln.supplierTax = nil
			}
		} else {
			name := strings.TrimSpace(l.SupplierName)
			tax := strings.TrimSpace(l.SupplierTaxID)
			if utf8.RuneCountInString(name) > 200 {
				return nil, tot, bad("ชื่อผู้ขายยาวเกินกำหนด (สูงสุด 200 ตัวอักษร)")
			}
			if utf8.RuneCountInString(tax) > 50 {
				return nil, tot, bad("เลขประจำตัวผู้เสียภาษีของผู้ขายยาวเกินกำหนด (สูงสุด 50 ตัวอักษร)")
			}
			ln.supplierName = pcNilIfEmpty(name)
			ln.supplierAddr = pcNilIfEmpty(strings.TrimSpace(l.SupplierAddress))
			ln.supplierTax = pcNilIfEmpty(tax)
		}

		// ── plain invoice (optional, not tied to has_tax_invoice) ──
		if inv := strings.TrimSpace(l.InvoiceNo); inv != "" {
			if utf8.RuneCountInString(inv) > 50 {
				return nil, tot, bad("เลขที่ใบแจ้งหนี้ยาวเกินกำหนด (สูงสุด 50 ตัวอักษร)")
			}
			ln.invoiceNo = &inv
		}
		if s := strings.TrimSpace(l.InvoiceDate); s != "" {
			d, ok := pcParseDate(s)
			if !ok {
				return nil, tot, bad("วันที่ใบแจ้งหนี้ไม่ถูกต้อง (รูปแบบ YYYY-MM-DD)")
			}
			ln.invoiceDate = d
		}

		// ── tax invoice / VAT ──
		if l.HasTaxInvoice {
			no := strings.TrimSpace(l.TaxInvoiceNo)
			if no == "" {
				return nil, tot, bad("กรุณากรอกเลขที่ใบกำกับภาษี")
			}
			if utf8.RuneCountInString(no) > 50 {
				return nil, tot, bad("เลขที่ใบกำกับภาษียาวเกินกำหนด (สูงสุด 50 ตัวอักษร)")
			}
			if strings.TrimSpace(l.TaxInvoiceDate) == "" {
				return nil, tot, bad("กรุณากรอกวันที่ใบกำกับภาษี")
			}
			d, ok := pcParseDate(strings.TrimSpace(l.TaxInvoiceDate))
			if !ok {
				return nil, tot, bad("วันที่ใบกำกับภาษีไม่ถูกต้อง (รูปแบบ YYYY-MM-DD)")
			}
			ln.hasTaxInvoice, ln.taxInvoiceNo, ln.taxInvoiceDate = true, &no, d
			ln.vat = pcRound2(amount * pcVatRate)
		}

		// ── withholding tax ──
		if l.HasWht {
			// ภ.ง.ด. is no longer asked for; keep it only if a client still sends a valid code.
			var taxForm *string
			if form := strings.TrimSpace(l.TaxFormCode); isPCTaxForm(form) {
				taxForm = &form
			}
			if l.WhtIncomeTypeID == nil || *l.WhtIncomeTypeID <= 0 {
				return nil, tot, bad("กรุณาเลือกประเภทเงินได้ที่หัก ณ ที่จ่าย")
			}
			var rate float64
			var active bool
			err := h.db.QueryRow(ctx, `SELECT rate, is_active FROM wht_income_type WHERE id=$1`, *l.WhtIncomeTypeID).Scan(&rate, &active)
			if err == pgx.ErrNoRows || (err == nil && !active) {
				return nil, tot, bad("ประเภทเงินได้ที่หัก ณ ที่จ่ายไม่ถูกต้องหรือถูกปิดใช้งาน")
			}
			if err != nil {
				return nil, tot, err
			}
			base := amount
			if l.WhtBaseAmount != nil {
				base = *l.WhtBaseAmount
			}
			if base <= 0 {
				return nil, tot, bad("ฐานภาษีหัก ณ ที่จ่ายต้องมากกว่า 0")
			}
			if base > amount {
				return nil, tot, bad("ฐานภาษีหัก ณ ที่จ่ายต้องไม่เกินจำนวนเงิน")
			}
			if s := strings.TrimSpace(l.WhtPayDate); s != "" {
				d, ok := pcParseDate(s)
				if !ok {
					return nil, tot, bad("วันที่จ่ายเงินไม่ถูกต้อง (รูปแบบ YYYY-MM-DD)")
				}
				ln.whtPayDate = d
			}
			ln.hasWht, ln.taxForm, ln.whtIncomeTypeID = true, taxForm, l.WhtIncomeTypeID
			ln.whtBase, ln.whtRate = &base, &rate // rate is the server-side snapshot, never the client's
			ln.wht = pcRound2(base * rate / 100)
		}

		ln.net = pcRound2(amount + ln.vat - ln.wht)
		// header totals are sums of the already-rounded line figures, so they always agree with the lines
		tot.total = pcRound2(tot.total + ln.amount)
		tot.vat = pcRound2(tot.vat + ln.vat)
		tot.wht = pcRound2(tot.wht + ln.wht)
		tot.net = pcRound2(tot.net + ln.net)
		tot.useVat = tot.useVat || ln.hasTaxInvoice
		tot.useWht = tot.useWht || ln.hasWht
		out = append(out, ln)
	}
	return out, tot, nil
}

// insertLines writes the resolved lines. New-style lines store the entered qty (default 1), unit_price = amount / qty,
// no discount (the line modal has no price).
func insertPCLines(ctx context.Context, tx pgx.Tx, pcID int64, lines []pcLine) error {
	for i, ln := range lines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO petty_cash_requisition_line
			  (pc_id, line_no, project_code, mat_code, cost_subgroup_id, description, qty, unit_price, amount,
			   discount, disc_type, line_discount, line_vat, line_wht, line_net, wht_rate, remarks,
			   supplier_id, supplier_name, supplier_address, supplier_tax_id,
			   has_tax_invoice, tax_invoice_no, tax_invoice_date,
			   has_wht, wht_base_amount, tax_form_code, wht_income_type_id, wht_pay_date,
				   invoice_no, invoice_date, unit)
			VALUES ($1,$2,$3,$4,$5,$6,$28,$7::numeric / $28::numeric,$7,0,'pct',0,$8,$9,$10,$11,$12,
			        $13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)`,
			pcID, i+1, ln.projectCode, ln.matCode, ln.costSubgroupID, ln.description, ln.amount,
			ln.vat, ln.wht, ln.net, ln.whtRate, ln.remarks,
			ln.supplierID, ln.supplierName, ln.supplierAddr, ln.supplierTax,
			ln.hasTaxInvoice, ln.taxInvoiceNo, ln.taxInvoiceDate,
			ln.hasWht, ln.whtBase, ln.taxForm, ln.whtIncomeTypeID, ln.whtPayDate,
			ln.invoiceNo, ln.invoiceDate, ln.unit, ln.qty,
		); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && (pgErr.Code == "23503" || pgErr.Code == "23514") {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("รายการที่ %d: ข้อมูลไม่ถูกต้อง กรุณาตรวจสอบอีกครั้ง", i+1))
			}
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("รายการที่ %d: บันทึกไม่สำเร็จ", i+1))
		}
	}
	return nil
}

func (h *PettyCashHandler) getByID(ctx context.Context, id int64) (*models.PettyCashRequisition, error) {
	var pc models.PettyCashRequisition
	err := h.db.QueryRow(ctx, `
		SELECT pc.id, pc.pc_no, pc.pc_date, pc.requested_by, pc.purpose,
		       pc.currency, pc.total_amount, pc.use_discount, pc.discount_type, pc.discount_amount,
		       pc.use_vat, pc.vat_amount, pc.use_wht, pc.wht_amount, pc.net_amount,
		       pc.status, pc.approver_id, pc.remarks,
		       pc.created_at, pc.updated_at, pc.created_by, pc.updated_by,
		       pc.doc_type, pc.branch_code, pc.payee_emp_code, pc.payee_name, pc.project_code, pc.project_name,
		       pc.order_type, pc.dept_code, pc.dept_name, pc.ref_doc_no, pc.approver_remarks,
		       u.full_name AS requested_by_name, u.username AS requested_by_username, au.full_name AS approver_name
		FROM petty_cash_requisition pc
		LEFT JOIN users u ON u.id = pc.requested_by
		LEFT JOIN users au ON au.id = pc.approver_id
		WHERE pc.id = $1`, id,
	).Scan(
		&pc.ID, &pc.PCNo, &pc.PCDate, &pc.RequestedBy, &pc.Purpose,
		&pc.Currency, &pc.TotalAmount, &pc.UseDiscount, &pc.DiscountType, &pc.DiscountAmount,
		&pc.UseVat, &pc.VatAmount, &pc.UseWht, &pc.WhtAmount, &pc.NetAmount,
		&pc.Status, &pc.ApproverID, &pc.Remarks,
		&pc.CreatedAt, &pc.UpdatedAt, &pc.CreatedBy, &pc.UpdatedBy,
		&pc.DocType, &pc.BranchCode, &pc.PayeeEmpCode, &pc.PayeeName, &pc.ProjectCode, &pc.ProjectName,
		&pc.OrderType, &pc.DeptCode, &pc.DeptName, &pc.RefDocNo, &pc.ApproverRemarks,
		&pc.RequestedByName, &pc.RequestedByUsername, &pc.ApproverName,
	)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.Query(ctx, `
		SELECT pcl.id, pcl.pc_id, pcl.line_no, pcl.project_code, pcl.mat_code, pcl.cost_subgroup_id, pcl.description,
		       pcl.qty, pcl.unit_price, COALESCE(pcl.amount, pcl.qty * pcl.unit_price), pcl.discount, pcl.disc_type,
		       pcl.line_discount, pcl.line_vat, pcl.line_wht, pcl.line_net, pcl.wht_rate, pcl.remarks,
		       pcl.supplier_id, pcl.supplier_name, pcl.supplier_address, pcl.supplier_tax_id,
		       pcl.has_tax_invoice, pcl.tax_invoice_no, to_char(pcl.tax_invoice_date, 'YYYY-MM-DD'),
		       pcl.has_wht, pcl.wht_base_amount, pcl.tax_form_code, pcl.wht_income_type_id,
		       to_char(pcl.wht_pay_date, 'YYYY-MM-DD'),
		       pcl.invoice_no, to_char(pcl.invoice_date, 'YYYY-MM-DD'),
		       mn.mat_name AS mat_name_th,
		       CASE WHEN pcl.cost_subgroup_id IS NULL THEN NULL
		            ELSE ` + costCodeSQL("csub","cj","cg","csg") + ` END AS cost_code,
		       wit.label_th,
		       p.project_name, mn.mat_name, COALESCE(NULLIF(pcl.unit, ''), u.unit_name), COALESCE(ps.qty_on_hand, 0)
		FROM petty_cash_requisition_line pcl
		LEFT JOIN project p ON p.project_code = pcl.project_code
		LEFT JOIN material_code mc ON mc.mat_code = pcl.mat_code
		LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
		LEFT JOIN unit u ON u.id = mc.unit_id
		LEFT JOIN cost_subgroup csg ON csg.id = pcl.cost_subgroup_id
		LEFT JOIN cost_group cg ON cg.id = csg.group_id
		LEFT JOIN cost_job cj ON cj.id = cg.job_id
		LEFT JOIN cost_subject csub ON csub.id = cj.subject_id
		LEFT JOIN wht_income_type wit ON wit.id = pcl.wht_income_type_id
		LEFT JOIN LATERAL (
			SELECT qty_on_hand FROM project_stock
			WHERE project_code = pcl.project_code AND mat_code = pcl.mat_code
			ORDER BY updated_at DESC LIMIT 1
		) ps ON TRUE
		WHERE pcl.pc_id = $1
		ORDER BY pcl.line_no`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projectSet := map[string]bool{}
	for rows.Next() {
		var l models.PettyCashLine
		if err := rows.Scan(
			&l.ID, &l.PCID, &l.LineNo, &l.ProjectCode, &l.MatCode, &l.CostSubgroupID, &l.Description,
			&l.Qty, &l.UnitPrice, &l.Amount, &l.Discount, &l.DiscType,
			&l.LineDiscount, &l.LineVat, &l.LineWht, &l.LineNet, &l.WhtRate, &l.Remarks,
			&l.SupplierID, &l.SupplierName, &l.SupplierAddress, &l.SupplierTaxID,
			&l.HasTaxInvoice, &l.TaxInvoiceNo, &l.TaxInvoiceDate,
			&l.HasWht, &l.WhtBaseAmount, &l.TaxFormCode, &l.WhtIncomeTypeID, &l.WhtPayDate,
			&l.InvoiceNo, &l.InvoiceDate,
			&l.MatNameTH, &l.CostCode, &l.WhtIncomeLabel,
			&l.ProjectName, &l.ItemName, &l.Unit, &l.StockOnHand,
		); err != nil {
			return nil, err
		}
		pc.Lines = append(pc.Lines, l)
		if l.ProjectCode != nil && *l.ProjectCode != "" && !projectSet[*l.ProjectCode] {
			projectSet[*l.ProjectCode] = true
			pc.ProjectCodes = append(pc.ProjectCodes, *l.ProjectCode)
		}
	}

	atts, err := h.listAttachments(ctx, id)
	if err != nil {
		return nil, err
	}
	pc.Attachments = atts

	return &pc, nil
}

// List godoc
// @Summary      List petty cash requisitions
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        status        query  string  false  "filter by status"
// @Param        project_code  query  string  false  "filter: document has at least one line with this project"
// @Param        date_from     query  string  false  "YYYY-MM-DD"
// @Param        date_to       query  string  false  "YYYY-MM-DD"
// @Param        page          query  int     false  "default 1"
// @Param        page_size     query  int     false  "default 20"
// @Success      200  {object}  models.PaginatedResponse
// @Router       /petty-cash [get]
func (h *PettyCashHandler) List(c *fiber.Ctx) error {
	var f models.PettyCashListFilter
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
	if f.Status != "" {
		where += fmt.Sprintf(" AND pc.status = $%d", i)
		args = append(args, f.Status)
		i++
	}
	if f.ProjectCode != "" {
		where += fmt.Sprintf(` AND (pc.project_code = $%d OR EXISTS (
			SELECT 1 FROM petty_cash_requisition_line pcl2
			WHERE pcl2.pc_id = pc.id AND pcl2.project_code = $%d
		))`, i, i)
		args = append(args, f.ProjectCode)
		i++
	}
	if f.DeptCode != "" {
		where += fmt.Sprintf(" AND pc.dept_code = $%d", i)
		args = append(args, f.DeptCode)
		i++
	}
	if f.OrderType != "" {
		if err := validateOrderType(f.OrderType); err != nil {
			return err
		}
		where += fmt.Sprintf(" AND pc.order_type = $%d", i)
		args = append(args, f.OrderType)
		i++
	}
	if f.DateFrom != "" {
		where += fmt.Sprintf(" AND pc.pc_date >= $%d", i)
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where += fmt.Sprintf(" AND pc.pc_date <= $%d", i)
		args = append(args, f.DateTo)
		i++
	}

	ctx := context.Background()

	var total int64
	countSQL := "SELECT COUNT(*) FROM petty_cash_requisition pc " + where
	if err := h.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return err
	}

	args = append(args, f.PageSize, offset)
	// project_codes is aggregated (distinct, comma-joined) from every line on the doc, since
	// the header itself has no project_code — this doc can span multiple projects.
	listSQL := fmt.Sprintf(`
		SELECT pc.id, pc.pc_no, pc.pc_date, pc.requested_by, pc.purpose,
		       pc.currency, pc.total_amount, pc.status, pc.approver_id, pc.net_amount,
		       pc.created_at,
		       pc.doc_type, pc.branch_code, pc.project_code, pc.project_name, pc.payee_name,
		       pc.dept_code, pc.dept_name, pc.ref_doc_no, pc.order_type,
		       u.full_name AS requested_by_name, au.full_name AS approver_name,
		       pcodes.codes,
		       COALESCE(pc.project_code, agg.first_project) AS project_display_code,
		       CASE WHEN pc.project_code IS NOT NULL THEN COALESCE(pc.project_name, hp.project_name)
		            ELSE fp.project_name END AS project_display_name,
		       COALESCE(array_length(agg.project_codes, 1), 0)
		         + CASE WHEN pc.project_code IS NOT NULL AND NOT (pc.project_code = ANY(COALESCE(agg.project_codes, '{}'))) THEN 1 ELSE 0 END AS project_count,
		       agg.supplier_names, COALESCE(agg.supplier_count, 0),
		       COALESCE(agg.wht, 0), COALESCE(agg.amount, 0),
		       COALESCE(agg.amount, 0) + COALESCE(agg.vat, 0) - COALESCE(agg.wht, 0) AS net_value
		FROM petty_cash_requisition pc
		LEFT JOIN users u ON u.id = pc.requested_by
		LEFT JOIN users au ON au.id = pc.approver_id
		LEFT JOIN (
			SELECT l.pc_id,
			       SUM(COALESCE(l.amount, l.qty * l.unit_price) - COALESCE(l.line_discount, 0)) AS amount,
			       SUM(COALESCE(l.line_vat, 0)) AS vat,
			       SUM(COALESCE(l.line_wht, 0)) AS wht,
			       string_agg(DISTINCT COALESCE(s.supplier_name, NULLIF(BTRIM(l.supplier_name), '')), ', ')
			         FILTER (WHERE COALESCE(s.supplier_name, NULLIF(BTRIM(l.supplier_name), '')) IS NOT NULL) AS supplier_names,
			       COUNT(DISTINCT COALESCE(s.supplier_name, NULLIF(BTRIM(l.supplier_name), ''))) AS supplier_count,
			       (array_agg(l.project_code ORDER BY l.line_no, l.id) FILTER (WHERE l.project_code IS NOT NULL))[1] AS first_project,
			       array_agg(DISTINCT l.project_code) FILTER (WHERE l.project_code IS NOT NULL) AS project_codes
			FROM petty_cash_requisition_line l
			LEFT JOIN supplier s ON s.id = l.supplier_id
			GROUP BY l.pc_id
		) agg ON agg.pc_id = pc.id
		LEFT JOIN project hp ON hp.project_code = pc.project_code
		LEFT JOIN project fp ON fp.project_code = agg.first_project
		LEFT JOIN LATERAL (
			SELECT array_agg(DISTINCT pcl.project_code ORDER BY pcl.project_code) FILTER (WHERE pcl.project_code IS NOT NULL) AS codes
			FROM petty_cash_requisition_line pcl WHERE pcl.pc_id = pc.id
		) pcodes ON TRUE
		%s
		ORDER BY pc.id DESC
		LIMIT $%d OFFSET $%d`, where, i, i+1)

	rows, err := h.db.Query(ctx, listSQL, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []models.PettyCashRequisition{}
	for rows.Next() {
		var pc models.PettyCashRequisition
		if err := rows.Scan(
			&pc.ID, &pc.PCNo, &pc.PCDate, &pc.RequestedBy, &pc.Purpose,
			&pc.Currency, &pc.TotalAmount, &pc.Status, &pc.ApproverID, &pc.NetAmount,
			&pc.CreatedAt,
			&pc.DocType, &pc.BranchCode, &pc.ProjectCode, &pc.ProjectName, &pc.PayeeName,
			&pc.DeptCode, &pc.DeptName, &pc.RefDocNo, &pc.OrderType,
			&pc.RequestedByName, &pc.ApproverName,
			&pc.ProjectCodes,
			&pc.ProjectDisplayCode, &pc.ProjectDisplayName, &pc.ProjectCount,
			&pc.SupplierNames, &pc.SupplierCount, &pc.WhtTotal, &pc.Amount, &pc.NetValue,
		); err != nil {
			return err
		}
		items = append(items, pc)
	}

	totalPages := (int(total) + f.PageSize - 1) / f.PageSize
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: f.Page, PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// Get godoc
// @Summary      Get a petty cash requisition with lines
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Petty cash requisition ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /petty-cash/{id} [get]
func (h *PettyCashHandler) Get(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	pc, err := h.getByID(context.Background(), int64(id))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	chain, err := h.loadApprovalChain(context.Background(), int64(id))
	if err != nil {
		return err
	}
	pc.ApprovalChain = chain
	return c.JSON(fiber.Map{"success": true, "data": pc})
}

// LineOptions godoc
// @Summary      Dropdown options for the petty cash line modal
// @Description  tax_forms (ภ.ง.ด.3 / ภ.ง.ด.53), active wht_income_types ordered by sort_order, and the fixed vat_rate (percent) used by the line calculation. Static route — registered before /:id.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/line-options [get]
func (h *PettyCashHandler) LineOptions(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT id, code, label_th, rate FROM wht_income_type WHERE is_active = true ORDER BY sort_order, id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type incomeType struct {
		ID      int     `json:"id"`
		Code    string  `json:"code"`
		LabelTH string  `json:"label_th"`
		Rate    float64 `json:"rate"`
	}
	types := []incomeType{}
	for rows.Next() {
		var t incomeType
		if err := rows.Scan(&t.ID, &t.Code, &t.LabelTH, &t.Rate); err != nil {
			return err
		}
		types = append(types, t)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"tax_forms":        pcTaxForms,
		"wht_income_types": types,
		"vat_rate":         float64(pcVatRatePercent),
	}})
}

// pcSubmitLine is the part of a line that the submit rules look at. lineNo is the number shown to the user.
type pcSubmitLine struct {
	lineNo         int
	costSubgroupID *int64
}

// requireSubmittable enforces the rules for entering PENDING_APPROVAL: ≥1 line, a requester remark
// (หมายเหตุผู้เบิก), and on every line a cost code that is a selectable ITEM (not Head, not Major).
// Unlike requireNonHeadCostCodes it does not grandfather a code already stored on the document: a
// DRAFT may keep a missing/Head/Major code, but it cannot go to approval with one. Drafts are not
// checked at all. Call before any write.
func requireSubmittable(ctx context.Context, q pgQuerier, remarks string, lines []pcSubmitLine) error {
	if len(lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, pcLinesRequiredMsg)
	}
	if strings.TrimSpace(remarks) == "" {
		return fiber.NewError(fiber.StatusBadRequest, pcRemarksRequiredMsg)
	}
	var ids []int64
	for _, l := range lines {
		if l.costSubgroupID != nil && *l.costSubgroupID > 0 {
			ids = append(ids, *l.costSubgroupID)
		}
	}
	rollup := map[int64]bool{} // Head or Major subgroups among the ids
	if len(ids) > 0 {
		rows, err := q.Query(ctx, `SELECT id FROM cost_subgroup WHERE id = ANY($1) AND (is_head OR is_major)`, ids)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			rollup[id] = true
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}
	for _, l := range lines {
		if l.costSubgroupID == nil || *l.costSubgroupID <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("บรรทัดที่ %d ยังไม่ได้ระบุรหัสต้นทุน", l.lineNo))
		}
		if rollup[*l.costSubgroupID] {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("บรรทัดที่ %d ใช้รหัสต้นทุนที่เลือกไม่ได้ (Head/Major)", l.lineNo))
		}
	}
	return nil
}

// Create godoc
// @Summary      Create a petty cash requisition
// @Description  status starts as DRAFT unless status=PENDING_APPROVAL is passed (requires approver_id, ≥1 line and remarks). A DRAFT may have zero lines. pc_no is generated server-side (PC-YYYYMM-NNNN); any client pc_no is ignored. A line with an empty project_code falls back to the header project_code; if both are empty the line is stored with project_code NULL (department-only). At least one of header project_code, header dept_code or a line project_code is required (400 otherwise). With status=PENDING_APPROVAL every line must carry a cost_subgroup_id that is a selectable ITEM (not Head/Major), else 400 "บรรทัดที่ N ยังไม่ได้ระบุรหัสต้นทุน" / "บรรทัดที่ N ใช้รหัสต้นทุนที่เลือกไม่ได้ (Head/Major)"; a DRAFT may be saved without a cost code.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreatePettyCashRequest  true  "Petty cash requisition data"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /petty-cash [post]
func (h *PettyCashHandler) Create(c *fiber.Ctx) error {
	var req models.CreatePettyCashRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	ctx := context.Background()
	if err := h.prepareHeader(ctx, &req); err != nil {
		return err
	}
	lines, tot, err := h.resolveLines(ctx, &req)
	if err != nil {
		return err
	}
	// A pending create goes through the strict requireSubmittable below (line-numbered messages);
	// drafts only get the lenient check.
	if req.Status != "PENDING_APPROVAL" {
		ids := make([]*int64, len(lines))
		for i, l := range lines {
			ids[i] = l.costSubgroupID
		}
		if err := requireNonHeadCostCodes(ctx, h.db, ids, nil); err != nil {
			return err
		}
	}

	status := req.Status
	if status == "" {
		status = "DRAFT"
	}
	if status != "DRAFT" && status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "status must be DRAFT or PENDING_APPROVAL")
	}
	if status == "PENDING_APPROVAL" {
		if req.ApproverID == nil {
			return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกผู้อนุมัติ")
		}
		if err := requireApprovalReady(ctx, h.db, "PETTY_CASH", req.ApproverID); err != nil {
			return err
		}
		submitLines := make([]pcSubmitLine, len(lines))
		for i, l := range lines {
			submitLines[i] = pcSubmitLine{lineNo: i + 1, costSubgroupID: l.costSubgroupID}
		}
		if err := requireSubmittable(ctx, h.db, req.Remarks, submitLines); err != nil {
			return err
		}
	}

	claims := middleware.GetClaims(c)

	if req.ApproverID != nil {
		var approverExists bool
		if err := h.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *req.ApproverID,
		).Scan(&approverExists); err != nil {
			return err
		}
		if !approverExists {
			return fiber.NewError(fiber.StatusBadRequest, "ไม่พบผู้อนุมัติที่เลือก")
		}
	}
	if status == "PENDING_APPROVAL" {
		var hasConfig bool
		if err := h.db.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM approval_config WHERE doc_type='PETTY_CASH' AND step_no=1 AND is_active=true)`,
		).Scan(&hasConfig); err != nil {
			return err
		}
		if !hasConfig {
			return fiber.NewError(fiber.StatusBadRequest, "ยังไม่ได้ตั้งค่าขั้นตอนการอนุมัติสำหรับใบเบิกเงินสดย่อย กรุณาติดต่อผู้ดูแลระบบ")
		}
	}

	// Header totals are sums of the per-line figures (VAT/WHT are per line now, not a header rate).
	// Header flags are derived from the lines; any client-sent header flags are ignored.
	useDiscount, useVat, useWht := false, tot.useVat, tot.useWht
	discountType, discountAmount := "pct", 0.0
	totalAmount, vatAmount, whtAmount, netAmount := tot.total, tot.vat, tot.wht, tot.net
	currency := req.Currency
	if currency == "" {
		currency = "THB"
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Allocated on the create tx, right before the header INSERT: a failed save gives the number back.
	now := bangkokNow()
	seq, err := nextMonthlyNumber(ctx, tx, "pc_number_counter", now)
	if err != nil {
		return err
	}
	pcNo := formatPCNo(now, seq)

	var pcID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO petty_cash_requisition
		  (pc_no, pc_date, requested_by, purpose, currency, total_amount,
		   use_discount, discount_type, discount_amount, use_vat, vat_amount, use_wht, wht_amount,
		   net_amount, status, approver_id, remarks, created_by, updated_by,
		   doc_type, branch_code, payee_emp_code, payee_name, project_code, project_name,
		   order_type, dept_code, dept_name, ref_doc_no)
		VALUES ($1,CURRENT_DATE,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17,
		        $18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
		RETURNING id`,
		pcNo, claims.UserID, pcNilIfEmpty(req.Purpose), currency, totalAmount,
		useDiscount, discountType, discountAmount, useVat, vatAmount, useWht, whtAmount,
		netAmount, status, req.ApproverID, pcNilIfEmpty(req.Remarks), claims.UserID,
		req.DocType, pcNilIfEmpty(req.BranchCode), pcNilIfEmpty(req.PayeeEmpCode), pcNilIfEmpty(req.PayeeName),
		pcNilIfEmpty(req.ProjectCode), pcNilIfEmpty(req.ProjectName),
		pcNilIfEmpty(req.OrderType), pcNilIfEmpty(req.DeptCode), pcNilIfEmpty(req.DeptName), pcNilIfEmpty(req.RefDocNo),
	).Scan(&pcID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "บันทึกใบเบิกเงินสดย่อยไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
	}

	if err := insertPCLines(ctx, tx, pcID, lines); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO petty_cash_status_log (pc_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,NULL,$2,$3,'Petty cash requisition created')`, pcID, status, claims.UserID,
	); err != nil {
		return err
	}

	if status == "PENDING_APPROVAL" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status, amount)
			VALUES ('PETTY_CASH',$1,$2,1,$3,$4,'PENDING',$5)`,
			pcID, pcNo, claims.UserID, *req.ApproverID, totalAmount,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	pc, err := h.getByID(context.Background(), pcID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": pc})
}

// Update godoc
// @Summary      Update a petty cash requisition (only while DRAFT or REJECTED)
// @Description  Head cost codes (cost_subgroup.is_head) are rejected with 400 "Cost Code นี้เป็น Head ไม่สามารถเลือกใช้ได้ …" when the value is NEW or CHANGED vs what the document already stores.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                            true  "Petty cash requisition ID"
// @Param        body  body  models.UpdatePettyCashRequest  true  "Petty cash requisition data"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /petty-cash/{id} [put]
func (h *PettyCashHandler) Update(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.UpdatePettyCashRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	ctx := context.Background()
	if err := h.prepareHeader(ctx, &req); err != nil {
		return err
	}
	lines, tot, err := h.resolveLines(ctx, &req)
	if err != nil {
		return err
	}
	{
		stored, err := storedCostSubgroupIDs(ctx, h.db, "petty_cash_requisition_line", int64(id))
		if err != nil {
			return err
		}
		ids := make([]*int64, len(lines))
		for i, l := range lines {
			ids[i] = l.costSubgroupID
		}
		if err := requireNonHeadCostCodes(ctx, h.db, ids, stored); err != nil {
			return err
		}
	}

	claims := middleware.GetClaims(c)

	var currentStatus string
	if err := h.db.QueryRow(ctx,
		`SELECT status FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&currentStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	if currentStatus != "DRAFT" && currentStatus != "REJECTED" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot update petty cash requisition with status '%s' — must be DRAFT or REJECTED", currentStatus))
	}

	if req.ApproverID != nil {
		var approverExists bool
		if err := h.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *req.ApproverID,
		).Scan(&approverExists); err != nil {
			return err
		}
		if !approverExists {
			return fiber.NewError(fiber.StatusBadRequest, "approver not found")
		}
	}

	// Header totals/flags are derived from the per-line figures (see Create).
	useDiscount, useVat, useWht := false, tot.useVat, tot.useWht
	discountType, discountAmount := "pct", 0.0
	totalAmount, vatAmount, whtAmount, netAmount := tot.total, tot.vat, tot.wht, tot.net

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// purpose and currency keep their stored value when absent from the request; the Tab A header
	// fields are overwritten as sent (an empty value clears them). pc_no, pc_date, requested_by and
	// status are never touched here.
	if _, err := tx.Exec(ctx, `
		UPDATE petty_cash_requisition
		SET purpose=COALESCE($1, purpose), currency=COALESCE($2, currency), total_amount=$3,
		    use_discount=$4, discount_type=$5, discount_amount=$6, use_vat=$7, vat_amount=$8,
		    use_wht=$9, wht_amount=$10, net_amount=$11, approver_id=$12, remarks=$13,
		    doc_type=$14, branch_code=$15, payee_emp_code=$16, payee_name=$17,
		    project_code=$18, project_name=$19, order_type=$20, dept_code=$21, dept_name=$22, ref_doc_no=$23,
		    updated_at=NOW(), updated_by=$24
		WHERE id=$25`,
		pcNilIfEmpty(req.Purpose), pcNilIfEmpty(req.Currency), totalAmount,
		useDiscount, discountType, discountAmount, useVat, vatAmount,
		useWht, whtAmount, netAmount, req.ApproverID, pcNilIfEmpty(req.Remarks),
		req.DocType, pcNilIfEmpty(req.BranchCode), pcNilIfEmpty(req.PayeeEmpCode), pcNilIfEmpty(req.PayeeName),
		pcNilIfEmpty(req.ProjectCode), pcNilIfEmpty(req.ProjectName),
		pcNilIfEmpty(req.OrderType), pcNilIfEmpty(req.DeptCode), pcNilIfEmpty(req.DeptName), pcNilIfEmpty(req.RefDocNo),
		claims.UserID, id,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "แก้ไขใบเบิกเงินสดย่อยไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
	}

	if _, err := tx.Exec(ctx, `DELETE FROM petty_cash_requisition_line WHERE pc_id=$1`, id); err != nil {
		return err
	}
	if err := insertPCLines(ctx, tx, int64(id), lines); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	pc, err := h.getByID(context.Background(), int64(id))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": pc})
}

// Delete godoc
// @Summary      Delete a petty cash requisition (only while DRAFT)
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Petty cash requisition ID"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id} [delete]
func (h *PettyCashHandler) Delete(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var currentStatus string
	if err := h.db.QueryRow(ctx,
		`SELECT status FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&currentStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	if currentStatus != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot delete petty cash requisition with status '%s' — must be DRAFT", currentStatus))
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM petty_cash_requisition_line WHERE pc_id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM petty_cash_status_log WHERE pc_id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM petty_cash_requisition WHERE id=$1`, id); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "petty cash requisition deleted"})
}

// Submit godoc
// @Summary      Submit for approval (DRAFT/REJECTED → PENDING_APPROVAL)
// @Description  Requires ≥1 line, requester remarks, and on every line a cost_subgroup_id that is a selectable ITEM (not Head/Major): 400 "บรรทัดที่ N ยังไม่ได้ระบุรหัสต้นทุน" / "บรรทัดที่ N ใช้รหัสต้นทุนที่เลือกไม่ได้ (Head/Major)". Unlike save-time validation, a Head/Major code already stored on the draft is NOT tolerated here.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int  true  "Petty cash requisition ID"
// @Param        body  body  object{approver_id=int64}  false  "approver_id — required if not already set"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id}/submit [post]
func (h *PettyCashHandler) Submit(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var body struct {
		ApproverID *int64 `json:"approver_id"`
	}
	_ = c.BodyParser(&body)

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	var pcNo string
	var approverID *int64
	var remarks *string
	if err := h.db.QueryRow(ctx, `
		SELECT pc.status, pc.pc_no, pc.approver_id, pc.remarks
		FROM petty_cash_requisition pc WHERE pc.id=$1`, id,
	).Scan(&currentStatus, &pcNo, &approverID, &remarks); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	if currentStatus != "DRAFT" && currentStatus != "REJECTED" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot submit petty cash requisition with status '%s' — must be DRAFT or REJECTED", currentStatus))
	}
	remarkText := ""
	if remarks != nil {
		remarkText = *remarks
	}
	lineRows, err := h.db.Query(ctx,
		`SELECT line_no, cost_subgroup_id FROM petty_cash_requisition_line WHERE pc_id=$1 ORDER BY line_no, id`, id)
	if err != nil {
		return err
	}
	var submitLines []pcSubmitLine
	for lineRows.Next() {
		var l pcSubmitLine
		if err := lineRows.Scan(&l.lineNo, &l.costSubgroupID); err != nil {
			lineRows.Close()
			return err
		}
		submitLines = append(submitLines, l)
	}
	lineRows.Close()
	if err := lineRows.Err(); err != nil {
		return err
	}
	if err := requireSubmittable(ctx, h.db, remarkText, submitLines); err != nil {
		return err
	}

	if body.ApproverID != nil {
		approverID = body.ApproverID
	}
	if approverID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกผู้อนุมัติ")
	}
	var approverExists bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *approverID,
	).Scan(&approverExists); err != nil {
		return err
	}
	if !approverExists {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่พบผู้อนุมัติที่เลือก")
	}

	var hasConfig bool
	if err := h.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM approval_config WHERE doc_type='PETTY_CASH' AND step_no=1 AND is_active=true)`,
	).Scan(&hasConfig); err != nil {
		return err
	}
	if !hasConfig {
		return fiber.NewError(fiber.StatusBadRequest, "ยังไม่ได้ตั้งค่าขั้นตอนการอนุมัติสำหรับใบเบิกเงินสดย่อย กรุณาติดต่อผู้ดูแลระบบ")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE petty_cash_requisition
		SET status='PENDING_APPROVAL', approver_id=$1, updated_at=NOW(), updated_by=$2
		WHERE id=$3`, *approverID, claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO petty_cash_status_log (pc_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'PENDING_APPROVAL',$3,'Submitted for approval')`,
		id, currentStatus, claims.UserID,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status, amount)
		SELECT 'PETTY_CASH', $1, $2, 1, $3, $4, 'PENDING', total_amount
		FROM petty_cash_requisition WHERE id=$1`,
		id, pcNo, claims.UserID, *approverID,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	var approverName *string
	_ = h.db.QueryRow(ctx, `SELECT full_name FROM users WHERE id=$1`, *approverID).Scan(&approverName)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Petty cash requisition submitted for approval",
		"data": fiber.Map{
			"id":            id,
			"doc_no":        pcNo,
			"status":        "PENDING_APPROVAL",
			"approver_id":   *approverID,
			"approver_name": approverName,
		},
	})
}

// ApprovalChain godoc
// @Summary      Approval chain of a petty cash requisition
// @Description  One row per approval step: approver user id, employee_code, step_no, full_name, full_name_th, level (highest roles.level the user holds), roles [{role_code, role_name}] and role_names (comma-joined, for display) and approval status (null = not yet requested | PENDING | APPROVED | REJECTED).
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Petty cash requisition ID"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id}/approval-chain [get]
func (h *PettyCashHandler) ApprovalChain(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var pcNo string
	if err := h.db.QueryRow(ctx,
		`SELECT pc_no FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&pcNo); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	rows, err := h.loadApprovalChain(ctx, int64(id))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"doc_no": pcNo, "approvers": rows}})
}

// pcChainRow is one approver in a petty cash approval chain.
type pcChainRow struct {
	UserID       *int64          `json:"user_id"`
	EmployeeCode *string         `json:"employee_code"`
	StepNo       int             `json:"step_no"`
	FullName     *string         `json:"full_name"`
	FullNameTH   *string         `json:"full_name_th"`
	Level        *int            `json:"level"`
	Roles        []userRoleBrief `json:"roles"`
	RoleNames    string          `json:"role_names"`
	Status       *string         `json:"status"`
}

// loadApprovalChain returns one row per approval step (latest approval_request per step), or the
// chosen-but-not-yet-submitted approver as step 1 with a null status.
func (h *PettyCashHandler) loadApprovalChain(ctx context.Context, id int64) ([]pcChainRow, error) {
	var approverID *int64
	if err := h.db.QueryRow(ctx,
		`SELECT approver_id FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&approverID); err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}

	rows := []pcChainRow{}

	// Latest approval_request per step (a resubmit after REJECT adds a new row for the same step).
	r, err := h.db.Query(ctx, `
		SELECT x.assigned_to, u.employee_code, x.step_no, u.full_name, u.full_name_th, lvl.level, x.status
		FROM (
			SELECT DISTINCT ON (step_no) step_no, assigned_to, status
			FROM approval_request
			WHERE doc_type='PETTY_CASH' AND doc_id=$1
			ORDER BY step_no, id DESC
		) x
		LEFT JOIN users u ON u.id = x.assigned_to
		LEFT JOIN LATERAL (
			SELECT MAX(ro.level) AS level
			FROM user_roles ur JOIN roles ro ON ro.id = ur.role_id AND ro.is_active = true
			WHERE ur.user_id = u.id
		) lvl ON true
		ORDER BY x.step_no`, id)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดสายการอนุมัติได้")
	}
	defer r.Close()
	for r.Next() {
		var row pcChainRow
		if err := r.Scan(&row.UserID, &row.EmployeeCode, &row.StepNo, &row.FullName, &row.FullNameTH, &row.Level, &row.Status); err != nil {
			return nil, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านสายการอนุมัติได้")
		}
		rows = append(rows, row)
	}
	if err := r.Err(); err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดสายการอนุมัติได้")
	}
	r.Close()

	// Not submitted yet (or no approval_config) - show the chosen approver with null status.
	if len(rows) == 0 && approverID != nil {
		var row pcChainRow
		row.StepNo = 1
		if err := h.db.QueryRow(ctx, `
			SELECT u.id, u.employee_code, u.full_name, u.full_name_th,
			       (SELECT MAX(ro.level) FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id AND ro.is_active=true WHERE ur.user_id=u.id)
			FROM users u WHERE u.id=$1`, *approverID,
		).Scan(&row.UserID, &row.EmployeeCode, &row.FullName, &row.FullNameTH, &row.Level); err == nil {
			rows = append(rows, row)
		}
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.UserID != nil {
			ids = append(ids, *row.UserID)
		}
	}
	rolesByUser, namesByUser, err := loadActiveRolesByUser(ctx, h.db, ids)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดบทบาทของผู้อนุมัติได้")
	}
	for i := range rows {
		rows[i].Roles = []userRoleBrief{}
		if rows[i].UserID != nil {
			if rs := rolesByUser[*rows[i].UserID]; rs != nil {
				rows[i].Roles = rs
			}
			rows[i].RoleNames = namesByUser[*rows[i].UserID]
		}
	}
	return rows, nil
}

// Approve godoc
// @Summary      Approve a petty cash requisition (PENDING_APPROVAL → APPROVED)
// @Description  One transaction: approval_request, document status, petty_cash_status_log, approval_log (APPROVE, with comments + ip). Caller must be the assigned approver (403 otherwise); an already decided document returns 409. Also reachable as PUT /approval/PETTY_CASH/{docId}/approve.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                            true  "Petty cash requisition ID"
// @Param        body  body  models.ApprovalActionRequest  false  "Optional comments"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id}/approve [post]
// @Router       /approval/PETTY_CASH/{id}/approve [put]
func (h *PettyCashHandler) Approve(c *fiber.Ctx) error {
	return h.approvalDecide(c, "APPROVE")
}

// Reject godoc
// @Summary      Reject a petty cash requisition (PENDING_APPROVAL → REJECTED)
// @Description  Same as approve, but comments are required (400). Also reachable as PUT /approval/PETTY_CASH/{docId}/reject.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                            true  "Petty cash requisition ID"
// @Param        body  body  models.ApprovalActionRequest  true  "comments required"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id}/reject [post]
// @Router       /approval/PETTY_CASH/{id}/reject [put]
func (h *PettyCashHandler) Reject(c *fiber.Ctx) error {
	return h.approvalDecide(c, "REJECT")
}

// Cancel godoc
// @Summary      Cancel a petty cash requisition (DRAFT → CANCELLED)
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Petty cash requisition ID"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id}/cancel [post]
func (h *PettyCashHandler) Cancel(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	if err := h.db.QueryRow(ctx,
		`SELECT status FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&currentStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	if currentStatus != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot cancel petty cash requisition with status '%s' — must be DRAFT", currentStatus))
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE petty_cash_requisition SET status='CANCELLED', updated_at=NOW(), updated_by=$1 WHERE id=$2`,
		claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO petty_cash_status_log (pc_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'CANCELLED',$3,'Cancelled')`, id, currentStatus, claims.UserID,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "petty cash requisition cancelled"})
}

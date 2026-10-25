package handlers

import (
	"context"
	"errors"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"erp-api/internal/middleware"
)

// บันทึกงานของผู้รับเหมาตามใบสั่งจ้าง — tab 2 ประเภทรับวางบิล. Bills hang off a wo_contractor_log row
// (tab 1 must be saved first). Handler type: WOContractorLogHandler (wo_contractor_log.go).

type woBillLine struct {
	LineCode string   `json:"line_code"` // ADV | PROGRESS | ADV_DEDUCT | VAT | RETENTION | WHT | OTHER
	Percent  *float64 `json:"percent"`
	Amount   float64  `json:"amount"`
}

type woBillInvoice struct {
	InvoiceNo    *string `json:"invoice_no"`
	InvoiceDate  *string `json:"invoice_date"`
	DeliveryDate *string `json:"delivery_date"`
	BillDate     *string `json:"bill_date"`
	PayDate      *string `json:"pay_date"`
}

type woBillRequest struct {
	BillType              string          `json:"bill_type"` // ADV | PROGRESS | RETENTION
	InstallmentNo         *int            `json:"installment_no"`
	ApVoucherNo           *string         `json:"ap_voucher_no"`
	PayTo                 *string         `json:"pay_to"`
	CreditTermDays        *int            `json:"credit_term_days"`
	Description           *string         `json:"description"`
	ProgressInstallmentNo *int            `json:"progress_installment_no"`
	WhtType               *string         `json:"wht_type"`
	Status                string          `json:"status"` // DRAFT (default) | SAVED
	Lines                 []woBillLine    `json:"lines"`
	Invoices              []woBillInvoice `json:"invoices"`
}

// calcWOBillTotals: subtotal = ADV + PROGRESS - ADV_DEDUCT + VAT ("รวมเป็นเงิน (1)"),
// net = subtotal - RETENTION - WHT - OTHER ("จ่ายสุทธิ"). The client sends line amounts; these two are always recomputed.
func calcWOBillTotals(lines []woBillLine) (subtotal, net float64) {
	amt := map[string]float64{}
	for _, l := range lines {
		amt[l.LineCode] = l.Amount
	}
	subtotal = round2(amt["ADV"] + amt["PROGRESS"] - amt["ADV_DEDUCT"] + amt["VAT"])
	net = round2(subtotal - amt["RETENTION"] - amt["WHT"] - amt["OTHER"])
	return
}

func validateWOBillRequest(req *woBillRequest) error {
	switch req.BillType {
	case "ADV", "PROGRESS", "RETENTION":
	default:
		return fiber.NewError(fiber.StatusBadRequest, "bill_type must be ADV, PROGRESS or RETENTION")
	}
	if req.Status == "" {
		req.Status = "DRAFT"
	}
	if req.Status != "DRAFT" && req.Status != "SAVED" {
		return fiber.NewError(fiber.StatusBadRequest, "status must be DRAFT or SAVED")
	}
	seen := map[string]bool{}
	for _, l := range req.Lines {
		switch l.LineCode {
		case "ADV", "PROGRESS", "ADV_DEDUCT", "VAT", "RETENTION", "WHT", "OTHER":
		default:
			return fiber.NewError(fiber.StatusBadRequest, "invalid line_code: "+l.LineCode)
		}
		if seen[l.LineCode] {
			return fiber.NewError(fiber.StatusBadRequest, "duplicate line_code: "+l.LineCode)
		}
		seen[l.LineCode] = true
		if l.Amount < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "amount must not be negative ("+l.LineCode+")")
		}
	}
	return nil
}

func insertWOBillChildren(ctx context.Context, tx pgx.Tx, billID int64, req *woBillRequest) error {
	for _, l := range req.Lines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO wo_contractor_log_bill_line (bill_id, line_code, percent, amount)
			VALUES ($1,$2,$3,$4)`, billID, l.LineCode, l.Percent, round2(l.Amount),
		); err != nil {
			return err
		}
	}
	for i, inv := range req.Invoices {
		if _, err := tx.Exec(ctx, `
			INSERT INTO wo_contractor_log_bill_invoice
			  (bill_id, line_no, invoice_no, invoice_date, delivery_date, bill_date, pay_date)
			VALUES ($1,$2,$3, NULLIF($4,'')::date, NULLIF($5,'')::date, NULLIF($6,'')::date, NULLIF($7,'')::date)`,
			billID, i+1, inv.InvoiceNo,
			derefStr(inv.InvoiceDate), derefStr(inv.DeliveryDate), derefStr(inv.BillDate), derefStr(inv.PayDate),
		); err != nil {
			return err
		}
	}
	return nil
}

// ListBills godoc
// @Summary      รายการรับวางบิลของบันทึกงานผู้รับเหมา
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Log ID"
// @Success      200  {object}  fiber.Map
// @Router       /wo-contractor-log/{id}/bills [get]
func (h *WOContractorLogHandler) ListBills(c *fiber.Ctx) error {
	logID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	rows, err := h.db.Query(context.Background(), `
		SELECT id, bill_type, installment_no, ap_voucher_no, subtotal, net_amount, status
		FROM wo_contractor_log_bill WHERE log_id = $1 ORDER BY id`, logID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []fiber.Map{}
	for rows.Next() {
		var (
			id                  int64
			billType, status    string
			installmentNo       *int
			apVoucherNo         *string
			subtotal, netAmount float64
		)
		if err := rows.Scan(&id, &billType, &installmentNo, &apVoucherNo, &subtotal, &netAmount, &status); err != nil {
			return err
		}
		items = append(items, fiber.Map{
			"id": id, "bill_type": billType, "installment_no": installmentNo, "ap_voucher_no": apVoucherNo,
			"subtotal": subtotal, "net_amount": netAmount, "status": status,
		})
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// CreateBill godoc
// @Summary      สร้างรายการรับวางบิล (tab ประเภทรับวางบิล)
// @Description  lines: up to 7 summary lines (ADV, PROGRESS, ADV_DEDUCT, VAT, RETENTION, WHT, OTHER). subtotal and net_amount are recomputed server-side.
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int            true  "Log ID"
// @Param        body  body  woBillRequest  true  "bill"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /wo-contractor-log/{id}/bills [post]
func (h *WOContractorLogHandler) CreateBill(c *fiber.Ctx) error {
	logID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req woBillRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := validateWOBillRequest(&req); err != nil {
		return err
	}
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wo_contractor_log WHERE id=$1)`, logID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "record not found")
	}

	subtotal, net := calcWOBillTotals(req.Lines)
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var billID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO wo_contractor_log_bill
		  (log_id, bill_type, installment_no, ap_voucher_no, pay_to, credit_term_days, description,
		   progress_installment_no, wht_type, subtotal, net_amount, status, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13) RETURNING id`,
		logID, req.BillType, req.InstallmentNo, req.ApVoucherNo, req.PayTo, req.CreditTermDays, req.Description,
		req.ProgressInstallmentNo, req.WhtType, subtotal, net, req.Status, claims.UserID,
	).Scan(&billID); err != nil {
		log.Printf("wo bill insert: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลได้ กรุณาลองใหม่อีกครั้ง")
	}
	if err := insertWOBillChildren(ctx, tx, billID, &req); err != nil {
		log.Printf("wo bill children insert: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลได้ กรุณาลองใหม่อีกครั้ง")
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true, "data": fiber.Map{"id": billID, "subtotal": subtotal, "net_amount": net},
	})
}

// UpdateBill godoc
// @Summary      แก้ไขรายการรับวางบิล (แทนที่ lines และ invoices ทั้งหมด)
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        billId  path  int            true  "Bill ID"
// @Param        body    body  woBillRequest  true  "bill"
// @Success      200     {object}  fiber.Map
// @Failure      404     {object}  fiber.Map
// @Router       /wo-contractor-log/bills/{billId} [put]
func (h *WOContractorLogHandler) UpdateBill(c *fiber.Ctx) error {
	billID, err := strconv.ParseInt(c.Params("billId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid billId")
	}
	var req woBillRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := validateWOBillRequest(&req); err != nil {
		return err
	}
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	subtotal, net := calcWOBillTotals(req.Lines)
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE wo_contractor_log_bill SET
		  bill_type=$1, installment_no=$2, ap_voucher_no=$3, pay_to=$4, credit_term_days=$5, description=$6,
		  progress_installment_no=$7, wht_type=$8, subtotal=$9, net_amount=$10, status=$11,
		  updated_at=NOW(), updated_by=$12
		WHERE id=$13`,
		req.BillType, req.InstallmentNo, req.ApVoucherNo, req.PayTo, req.CreditTermDays, req.Description,
		req.ProgressInstallmentNo, req.WhtType, subtotal, net, req.Status, claims.UserID, billID)
	if err != nil {
		log.Printf("wo bill update: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลได้ กรุณาลองใหม่อีกครั้ง")
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "record not found")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM wo_contractor_log_bill_line WHERE bill_id=$1`, billID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM wo_contractor_log_bill_invoice WHERE bill_id=$1`, billID); err != nil {
		return err
	}
	if err := insertWOBillChildren(ctx, tx, billID, &req); err != nil {
		log.Printf("wo bill children insert: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลได้ กรุณาลองใหม่อีกครั้ง")
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": billID, "subtotal": subtotal, "net_amount": net}})
}

// GetBill godoc
// @Summary      รายละเอียดรายการรับวางบิล
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Produce      json
// @Param        billId  path  int  true  "Bill ID"
// @Success      200     {object}  fiber.Map
// @Failure      404     {object}  fiber.Map
// @Router       /wo-contractor-log/bills/{billId} [get]
func (h *WOContractorLogHandler) GetBill(c *fiber.Ctx) error {
	billID, err := strconv.ParseInt(c.Params("billId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid billId")
	}
	ctx := context.Background()

	var (
		logID                                                int64
		billType, status                                     string
		installmentNo, creditTermDays, progressInstallmentNo *int
		apVoucherNo, payTo, description, whtType             *string
		subtotal, netAmount                                  float64
	)
	err = h.db.QueryRow(ctx, `
		SELECT log_id, bill_type, installment_no, ap_voucher_no, pay_to, credit_term_days, description,
		       progress_installment_no, wht_type, subtotal, net_amount, status
		FROM wo_contractor_log_bill WHERE id=$1`, billID,
	).Scan(&logID, &billType, &installmentNo, &apVoucherNo, &payTo, &creditTermDays, &description,
		&progressInstallmentNo, &whtType, &subtotal, &netAmount, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "record not found")
	}
	if err != nil {
		return err
	}

	lineRows, err := h.db.Query(ctx, `SELECT line_code, percent, amount FROM wo_contractor_log_bill_line WHERE bill_id=$1`, billID)
	if err != nil {
		return err
	}
	defer lineRows.Close()
	lines := []woBillLine{}
	for lineRows.Next() {
		var l woBillLine
		if err := lineRows.Scan(&l.LineCode, &l.Percent, &l.Amount); err != nil {
			return err
		}
		lines = append(lines, l)
	}

	invRows, err := h.db.Query(ctx, `
		SELECT invoice_no, TO_CHAR(invoice_date,'YYYY-MM-DD'), TO_CHAR(delivery_date,'YYYY-MM-DD'),
		       TO_CHAR(bill_date,'YYYY-MM-DD'), TO_CHAR(pay_date,'YYYY-MM-DD')
		FROM wo_contractor_log_bill_invoice WHERE bill_id=$1 ORDER BY line_no`, billID)
	if err != nil {
		return err
	}
	defer invRows.Close()
	invoices := []woBillInvoice{}
	for invRows.Next() {
		var inv woBillInvoice
		if err := invRows.Scan(&inv.InvoiceNo, &inv.InvoiceDate, &inv.DeliveryDate, &inv.BillDate, &inv.PayDate); err != nil {
			return err
		}
		invoices = append(invoices, inv)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"id": billID, "log_id": logID, "bill_type": billType, "installment_no": installmentNo,
			"ap_voucher_no": apVoucherNo, "pay_to": payTo, "credit_term_days": creditTermDays,
			"description": description, "progress_installment_no": progressInstallmentNo, "wht_type": whtType,
			"subtotal": subtotal, "net_amount": netAmount, "status": status,
			"lines": lines, "invoices": invoices,
		},
	})
}

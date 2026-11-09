package handlers

import (
	"context"
	"errors"
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

type woLogSummaryRow struct {
	Percent    float64 `json:"percent"`
	Amount     float64 `json:"amount"`
	Accumulate float64 `json:"accumulate"`
	Balance    float64 `json:"balance"`
}

func woRound2(v float64) float64 { return math.Round(v*100) / 100 }

// SummaryByWO godoc
// @Summary      Contract / Advance / Retention summary of a work order's contractor bills
// @Description  All values are calculated, nothing is typed in. Only SAVED bills are counted.
// @Description  CONTRACT: amount = Work Progress paid so far (payment_base = contract value minus the advance, contract_total = the full value); accumulate = sum of every bill's "เงินงวดนี้" (PROGRESS) amount;
// @Description  percent = sum of those bills' PROGRESS percent (e.g. installment 1 = 20% and installment 2 = 30% -> 50%); balance = amount - accumulate.
// @Description  ADVANCE: percent / amount = the WO's advance (7.1, advance_deduct_pct / advance_deduct_amount); accumulate = sum of the bills'
// @Description  ADV_DEDUCT lines; balance = amount - accumulate.
// @Description  RETENTION: only what was entered in the bills (5. หักเงินประกันผลงาน), the WO's own retention terms are ignored.
// @Description  amount = accumulate = sum of the bills' RETENTION lines; percent = amount / contract value; balance = 0.
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Produce      json
// @Param        woId  path  int  true  "work_order.id"
// @Success      200  {object}  fiber.Map
// @Router       /wo-contractor-log/summary/{woId} [get]
func (h *WOContractorLogHandler) SummaryByWO(c *fiber.Ctx) error {
	woID, err := strconv.ParseInt(c.Params("woId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid woId")
	}
	ctx := context.Background()

	var contract, advPct, advAmount float64
	err = h.db.QueryRow(ctx, `
		SELECT COALESCE(contract_amount, 0)::float8, COALESCE(advance_deduct_pct, 0)::float8, COALESCE(advance_deduct_amount, 0)::float8
		FROM work_order WHERE id = $1`, woID).Scan(&contract, &advPct, &advAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "record not found")
	}
	if err != nil {
		return err
	}

	// Totals over every SAVED bill of this work order's contractor log.
	var progPct, progAmt, deductAmt, retAmt, advPaid float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE WHEN l.line_code = 'PROGRESS'   THEN COALESCE(l.percent, 0) END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'PROGRESS'   THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'ADV_DEDUCT' THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'RETENTION'  THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'ADV'        THEN l.amount END), 0)::float8
		FROM wo_contractor_log lg
		JOIN wo_contractor_log_bill b ON b.log_id = lg.id AND b.status = 'SAVED'
		JOIN wo_contractor_log_bill_line l ON l.bill_id = b.id
		WHERE lg.wo_id = $1`, woID).Scan(&progPct, &progAmt, &deductAmt, &retAmt, &advPaid); err != nil {
		return err
	}

	// Contract money already paid through the bills, before VAT / withholding tax:
	// advance paid + this-installment amounts - advance deducted again, over every SAVED bill.
	var billsPaid float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE l.line_code WHEN 'ADV' THEN l.amount WHEN 'PROGRESS' THEN l.amount WHEN 'ADV_DEDUCT' THEN -l.amount ELSE 0 END), 0)::float8
		FROM wo_contractor_log lg
		JOIN wo_contractor_log_bill b ON b.log_id = lg.id AND b.status = 'SAVED'
		JOIN wo_contractor_log_bill_line l ON l.bill_id = b.id
		WHERE lg.wo_id = $1`, woID).Scan(&billsPaid); err != nil {
		return err
	}

	// Bills of any status (DRAFT or SAVED): the client keeps the table empty until the first one exists.
	var billCount int
	if err := h.db.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM wo_contractor_log lg
		JOIN wo_contractor_log_bill b ON b.log_id = lg.id
		WHERE lg.wo_id = $1`, woID).Scan(&billCount); err != nil {
		return err
	}

	// Credit term of the bill form: live-read from the WO's supplier.payment_terms (free text like "30 วัน") — first integer in it.
	var creditTermDays *int
	if err := h.db.QueryRow(ctx, `
		SELECT NULLIF(substring(s.payment_terms from '[0-9]+'), '')::int
		FROM work_order wo LEFT JOIN supplier s ON s.id = wo.supplier_id
		WHERE wo.id = $1`, woID).Scan(&creditTermDays); err != nil {
		return err
	}

	// Retention comes ONLY from what was typed in the bills (line RETENTION, 5. หักเงินประกันผลงาน) — the WO's own
	// retention terms (7.3) are not used. Its % is that total against the contract value.
	retAmount := woRound2(retAmt)
	retPct := 0.0
	if contract > 0 {
		retPct = retAmount / contract * 100
	}
	// The installments (เงินงวดนี้) are a % of the contract value AFTER the advance is taken out
	// (3,000,000 - 10% advance = 2,700,000), so the Contract row works on that base.
	if advAmount == 0 {
		advAmount = woRound2(contract * advPct / 100)
	}
	// Once an advance bill is SAVED, what was actually paid is the advance of this contract — the bill's % may differ
	// from the work order's own 7.1 terms (terms say 10%, the bill paid 20% -> the Advance row shows 20% / 200,000).
	// Without a saved advance bill (or with the WO advance left empty) the row falls back to the WO's 7.1 terms.
	if advPaid > 0 {
		advAmount = woRound2(advPaid)
		if contract > 0 {
			advPct = woRound2(advPaid / contract * 100)
		}
	}
	payBase := contract - advAmount
	// The Contract balance (what is still to be billed as Work Progress) only starts counting once a Work Progress
	// installment has been saved; with none saved yet the whole row stays at 0.
	// Installments are a % of the FULL contract value (30% of 1,000,000 = 300,000) and every progress bill takes the
	// advance back (ADV_DEDUCT), so the balance is measured against the full contract and reaches 0 when the
	// installments add up to 100% (advance paid + installments - advance deducted = the contract value).
	contractBalance := 0.0
	if progAmt > 0 {
		contractBalance = woRound2(contract - progAmt)
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"bills_paid_total": woRound2(billsPaid), // ex VAT / WHT, see above
			"contract_total":   woRound2(contract),  // full WO contract value (before the advance is taken out)
			"payment_base":     woRound2(payBase),   // contract value minus the advance: the base of every installment %
			"credit_term_days": creditTermDays,
			"bill_count":       billCount, // number of bills (any status) on this WO's contractor log
			"contract": woLogSummaryRow{
				// Amount = what has been paid as Work Progress so far (0 until the first installment bill is saved);
				// payment_base below is the value those installments are a % of.
				Percent: woRound2(progPct), Amount: woRound2(progAmt), Accumulate: woRound2(progAmt), Balance: contractBalance,
			},
			"advance": woLogSummaryRow{
				Percent: woRound2(advPct), Amount: woRound2(advAmount), Accumulate: woRound2(deductAmt), Balance: woRound2(advAmount - deductAmt),
			},
			"retention": woLogSummaryRow{
				Percent: woRound2(retPct), Amount: retAmount, Accumulate: woRound2(retAmt), Balance: woRound2(retAmount - retAmt),
			},
		},
	})
}

// SummaryByPO is SummaryByWO for a purchase order. Same response shape; the PO has no advance or retention terms of
// its own, so the contract is the PO value before VAT after discount and the base of every installment is that value.
// @Router       /wo-contractor-log/summary-po/{poId} [get]
func (h *WOContractorLogHandler) SummaryByPO(c *fiber.Ctx) error {
	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}
	ctx := context.Background()

	var contract float64
	var creditTermDays *int
	err = h.db.QueryRow(ctx, `
		SELECT (COALESCE(po.total_amount,0) - COALESCE(po.discount_amount,0))::float8,
		       NULLIF(substring(s.payment_terms from '[0-9]+'), '')::int
		FROM purchase_order po LEFT JOIN supplier s ON s.id = po.supplier_id
		WHERE po.id = $1`, poID).Scan(&contract, &creditTermDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "record not found")
	}
	if err != nil {
		return err
	}

	var progPct, progAmt, deductAmt, retAmt, billsPaid float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE WHEN l.line_code = 'PROGRESS'   THEN COALESCE(l.percent, 0) END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'PROGRESS'   THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'ADV_DEDUCT' THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'RETENTION'  THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE l.line_code WHEN 'ADV' THEN l.amount WHEN 'PROGRESS' THEN l.amount WHEN 'ADV_DEDUCT' THEN -l.amount ELSE 0 END), 0)::float8
		FROM wo_contractor_log lg
		JOIN wo_contractor_log_bill b ON b.log_id = lg.id AND b.status = 'SAVED'
		JOIN wo_contractor_log_bill_line l ON l.bill_id = b.id
		WHERE lg.po_id = $1`, poID).Scan(&progPct, &progAmt, &deductAmt, &retAmt, &billsPaid); err != nil {
		return err
	}
	var billCount int
	if err := h.db.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM wo_contractor_log lg JOIN wo_contractor_log_bill b ON b.log_id = lg.id
		WHERE lg.po_id = $1`, poID).Scan(&billCount); err != nil {
		return err
	}

	retPct := 0.0
	if contract > 0 {
		retPct = woRound2(retAmt) / contract * 100
	}
	contractBalance := 0.0
	if progAmt > 0 {
		contractBalance = woRound2(contract - progAmt)
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"bills_paid_total": woRound2(billsPaid),
			"contract_total":   woRound2(contract),
			"payment_base":     woRound2(contract),
			"credit_term_days": creditTermDays,
			"bill_count":       billCount,
			"contract":         woLogSummaryRow{Percent: woRound2(progPct), Amount: woRound2(progAmt), Accumulate: woRound2(progAmt), Balance: contractBalance},
			"advance":          woLogSummaryRow{Accumulate: woRound2(deductAmt), Balance: woRound2(-deductAmt)},
			"retention":        woLogSummaryRow{Percent: woRound2(retPct), Amount: woRound2(retAmt), Accumulate: woRound2(retAmt)},
		},
	})
}

// SummaryByPC is SummaryByPO for a petty cash requisition. Same response shape; no advance, no VAT and no credit term.
// The contract is the value before VAT after discount and the base of every installment is that value.
// @Router       /wo-contractor-log/summary-pc/{pcId} [get]
func (h *WOContractorLogHandler) SummaryByPC(c *fiber.Ctx) error {
	pcID, err := strconv.ParseInt(c.Params("pcId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid pcId")
	}
	ctx := context.Background()

	var contract float64
	err = h.db.QueryRow(ctx, `
		SELECT (COALESCE(total_amount,0) - COALESCE(discount_amount,0))::float8
		FROM petty_cash_requisition WHERE id = $1`, pcID).Scan(&contract)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "record not found")
	}
	if err != nil {
		return err
	}

	var progPct, progAmt, retAmt, billsPaid float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE WHEN l.line_code = 'PROGRESS'  THEN COALESCE(l.percent, 0) END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'PROGRESS'  THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'RETENTION' THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE l.line_code WHEN 'PROGRESS' THEN l.amount ELSE 0 END), 0)::float8
		FROM wo_contractor_log lg
		JOIN wo_contractor_log_bill b ON b.log_id = lg.id AND b.status = 'SAVED'
		JOIN wo_contractor_log_bill_line l ON l.bill_id = b.id
		WHERE lg.pc_id = $1`, pcID).Scan(&progPct, &progAmt, &retAmt, &billsPaid); err != nil {
		return err
	}
	var billCount int
	if err := h.db.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM wo_contractor_log lg JOIN wo_contractor_log_bill b ON b.log_id = lg.id
		WHERE lg.pc_id = $1`, pcID).Scan(&billCount); err != nil {
		return err
	}

	retPct := 0.0
	if contract > 0 {
		retPct = woRound2(retAmt) / contract * 100
	}
	contractBalance := 0.0
	if progAmt > 0 {
		contractBalance = woRound2(contract - progAmt)
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"bills_paid_total": woRound2(billsPaid),
			"contract_total":   woRound2(contract),
			"payment_base":     woRound2(contract),
			"credit_term_days": nil,
			"bill_count":       billCount,
			"contract":         woLogSummaryRow{Percent: woRound2(progPct), Amount: woRound2(progAmt), Accumulate: woRound2(progAmt), Balance: contractBalance},
			"advance":          woLogSummaryRow{},
			"retention":        woLogSummaryRow{Percent: woRound2(retPct), Amount: woRound2(retAmt), Accumulate: woRound2(retAmt)},
		},
	})
}

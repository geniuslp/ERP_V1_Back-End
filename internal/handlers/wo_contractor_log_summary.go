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
// @Description  CONTRACT: amount = the WO contract value (ex VAT); accumulate = sum of every bill's "เงินงวดนี้" (PROGRESS) amount;
// @Description  percent = sum of those bills' PROGRESS percent (e.g. installment 1 = 20% and installment 2 = 30% -> 50%); balance = amount - accumulate.
// @Description  ADVANCE: percent / amount = the WO's advance (7.1, advance_deduct_pct / advance_deduct_amount); accumulate = sum of the bills'
// @Description  ADV_DEDUCT lines; balance = amount - accumulate.
// @Description  RETENTION: percent = the WO's retention % (7.3); amount = contract value * percent / 100; accumulate = sum of the bills' RETENTION
// @Description  lines; balance = amount - accumulate.
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

	var retPct float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE((SELECT percent_of_contract FROM work_order_retention WHERE wo_id = $1 ORDER BY id LIMIT 1), 0)::float8`,
		woID).Scan(&retPct); err != nil {
		return err
	}

	// Totals over every SAVED bill of this work order's contractor log.
	var progPct, progAmt, deductAmt, retAmt float64
	if err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE WHEN l.line_code = 'PROGRESS'   THEN COALESCE(l.percent, 0) END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'PROGRESS'   THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'ADV_DEDUCT' THEN l.amount END), 0)::float8,
		       COALESCE(SUM(CASE WHEN l.line_code = 'RETENTION'  THEN l.amount END), 0)::float8
		FROM wo_contractor_log lg
		JOIN wo_contractor_log_bill b ON b.log_id = lg.id AND b.status = 'SAVED'
		JOIN wo_contractor_log_bill_line l ON l.bill_id = b.id
		WHERE lg.wo_id = $1`, woID).Scan(&progPct, &progAmt, &deductAmt, &retAmt); err != nil {
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

	retAmount := woRound2(contract * retPct / 100)
	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"bills_paid_total": woRound2(billsPaid), // ex VAT / WHT, see above
			"contract": woLogSummaryRow{
				Percent: woRound2(progPct), Amount: woRound2(contract), Accumulate: woRound2(progAmt), Balance: woRound2(contract - progAmt),
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

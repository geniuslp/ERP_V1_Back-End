package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type woLogInvoiceHistoryRow struct {
	BillID        int64   `json:"bill_id"`
	BillNo        int     `json:"bill_no"` // 1, 2, ... in the order the bills were created (ครั้งที่จ่าย)
	BillType      string  `json:"bill_type"`
	InstallmentNo *int    `json:"installment_no"`
	Status        string  `json:"status"`
	InvoiceNo     *string `json:"invoice_no"`
	InvoiceDate   *string `json:"invoice_date"`
	DeliveryDate  *string `json:"delivery_date"`
	BillDate      *string `json:"bill_date"`
	PayDate       *string `json:"pay_date"`
}

// ListInvoices godoc
// @Summary      Invoices of every bill of a contractor log
// @Description  One row per invoice of every bill (DRAFT and SAVED), oldest bill first. bill_no is the bill's order number (payment no. 1, 2, ...).
// @Description  A bill with no invoices still appears once (invoice fields null) so each payment is visible.
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Log ID"
// @Success      200  {object}  fiber.Map
// @Router       /wo-contractor-log/{id}/invoices [get]
func (h *WOContractorLogHandler) ListInvoices(c *fiber.Ctx) error {
	logID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	rows, err := h.db.Query(context.Background(), `
		SELECT b.id, ROW_NUMBER() OVER (ORDER BY b.id)::int, b.bill_type, b.installment_no, b.status,
		       i.invoice_no, TO_CHAR(i.invoice_date,'YYYY-MM-DD'), TO_CHAR(i.delivery_date,'YYYY-MM-DD'),
		       TO_CHAR(i.bill_date,'YYYY-MM-DD'), TO_CHAR(i.pay_date,'YYYY-MM-DD')
		FROM wo_contractor_log_bill b
		LEFT JOIN wo_contractor_log_bill_invoice i ON i.bill_id = b.id
		WHERE b.log_id = $1
		ORDER BY b.id, i.line_no`, logID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []woLogInvoiceHistoryRow{}
	for rows.Next() {
		var r woLogInvoiceHistoryRow
		if err := rows.Scan(&r.BillID, &r.BillNo, &r.BillType, &r.InstallmentNo, &r.Status,
			&r.InvoiceNo, &r.InvoiceDate, &r.DeliveryDate, &r.BillDate, &r.PayDate); err != nil {
			return err
		}
		items = append(items, r)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

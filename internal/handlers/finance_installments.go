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

	"erp-api/internal/middleware"
)

// installmentDescMax is a conservative cap for payment_installment.description (checked in runes).
const installmentDescMax = 255

type paymentInstallmentRow struct {
	ID            int64      `json:"id"`
	InstallmentNo int        `json:"installment_no"`
	Percent       float64    `json:"percent"`
	Amount        float64    `json:"amount"`
	Description   *string    `json:"description"`
	IsPaid        bool       `json:"is_paid"`
	PaidAt        *time.Time `json:"paid_at"`
	PaidByName    *string    `json:"paid_by_name"`
	PaymentLogID  *int64     `json:"payment_log_id"`
	PaymentDate   *time.Time `json:"payment_date"` // payment_log.paid_date of the linked payment
}

type installmentInput struct {
	ID          *int64   `json:"id"`
	Percent     *float64 `json:"percent"`
	Amount      *float64 `json:"amount"`
	Description *string  `json:"description"`
	IsPaid      bool     `json:"is_paid"` // ignored: paid state is driven by POST /finance/payments (installment_ids)
}

type saveInstallmentsRequest struct {
	Installments []installmentInput `json:"installments"`
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// loadInstallments reads rows in installment_no order (LEFT JOIN users for the payer name).
func loadInstallments(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, docType string, docID int) ([]paymentInstallmentRow, error) {
	rows, err := q.Query(ctx, `
		SELECT i.id, i.installment_no, i.percent, i.amount, i.description, i.is_paid, i.paid_at, u.full_name,
		       i.payment_log_id, pl.paid_date
		FROM payment_installment i
		LEFT JOIN users u ON u.id = i.paid_by
		LEFT JOIN payment_log pl ON pl.id = i.payment_log_id
		WHERE i.doc_type = $1 AND i.doc_id = $2
		ORDER BY i.installment_no`, docType, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []paymentInstallmentRow{}
	for rows.Next() {
		var r paymentInstallmentRow
		if err := rows.Scan(&r.ID, &r.InstallmentNo, &r.Percent, &r.Amount, &r.Description, &r.IsPaid, &r.PaidAt, &r.PaidByName, &r.PaymentLogID, &r.PaymentDate); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetInstallments godoc
// @Summary      List payment installments of a PO, WO or PC
// @Description  Rows ordered by installment_no, plus the document net_amount and the sum of installment
// @Description  amounts. The "installment mode on" state on the screen is simply: rows exist.
// @Tags         Finance
// @Security     BearerAuth
// @Produce      json
// @Param        doc_type  path  string  true  "PO, WO or PC"
// @Param        doc_id    path  int     true  "document id"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /finance/payments/{doc_type}/{doc_id}/installments [get]
func (h *FinanceHandler) GetInstallments(c *fiber.Ctx) error {
	docType := c.Params("doc_type")
	table, _, ok := docTable(docType)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type ต้องเป็น PO, WO หรือ PC")
	}
	docID, err := c.ParamsInt("doc_id")
	if err != nil || docID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "doc_id ไม่ถูกต้อง")
	}
	const failMsg = "ไม่สามารถโหลดงวดการจ่ายเงินได้ กรุณาลองใหม่อีกครั้ง"
	ctx := context.Background()

	var netAmount float64
	err = h.db.QueryRow(ctx, fmt.Sprintf(`SELECT net_amount FROM %s WHERE id = $1`, table), docID).Scan(&netAmount)
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("ไม่พบเอกสาร %s id %d", docType, docID))
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	items, err := loadInstallments(ctx, h.db, docType, docID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	var sum float64
	for _, it := range items {
		sum += it.Amount
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"doc_type":         docType,
		"doc_id":           docID,
		"net_amount":       netAmount,
		"installments_sum": round2(sum),
		"installments":     items,
	}})
}

// SaveInstallments godoc
// @Summary      Replace all payment installments of a PO, WO or PC
// @Description  Full replace in one transaction. Rows are renumbered 1..n in request order. An empty list
// @Description  deletes everything (installment mode off) unless some existing row is already paid.
// @Description  is_paid in the body is ignored (rows are marked paid via POST /finance/payments with installment_ids).
// @Description  Rows already is_paid cannot be deleted, and their percent/amount/description cannot change.
// @Description  If only percent is sent amount = round(net*percent/100,2); if only amount is sent
// @Description  percent = round(amount/net*100,4). Total amount must not exceed the document net_amount.
// @Tags         Finance
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        doc_type  path  string  true  "PO, WO or PC"
// @Param        doc_id    path  int     true  "document id"
// @Param        body      body  saveInstallmentsRequest  true  "Installments"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /finance/payments/{doc_type}/{doc_id}/installments [put]
func (h *FinanceHandler) SaveInstallments(c *fiber.Ctx) error {
	docType := c.Params("doc_type")
	table, noCol, ok := docTable(docType)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type ต้องเป็น PO, WO หรือ PC")
	}
	docID, err := c.ParamsInt("doc_id")
	if err != nil || docID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "doc_id ไม่ถูกต้อง")
	}
	var req saveInstallmentsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
	}

	// Request-only validation (no DB needed).
	seen := map[int64]bool{}
	for i, in := range req.Installments {
		n := i + 1
		if in.Percent == nil && in.Amount == nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("งวดที่ %d: กรุณาระบุเปอร์เซ็นต์หรือจำนวนเงิน", n))
		}
		if in.Percent != nil && (*in.Percent < 0 || *in.Percent > 100 || math.IsNaN(*in.Percent)) {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("งวดที่ %d: เปอร์เซ็นต์ต้องอยู่ระหว่าง 0 ถึง 100", n))
		}
		if in.Amount != nil && (*in.Amount < 0 || math.IsNaN(*in.Amount) || math.IsInf(*in.Amount, 0)) {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("งวดที่ %d: จำนวนเงินต้องไม่ติดลบ", n))
		}
		if in.Description != nil && utf8.RuneCountInString(strings.TrimSpace(*in.Description)) > installmentDescMax {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("งวดที่ %d: รายละเอียดยาวเกิน %d ตัวอักษร", n, installmentDescMax))
		}
		if in.ID != nil {
			if seen[*in.ID] {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("งวดที่ %d: รหัสงวดซ้ำกัน", n))
			}
			seen[*in.ID] = true
		}
	}

	const failMsg = "ไม่สามารถบันทึกงวดการจ่ายเงินได้ กรุณาลองใหม่อีกครั้ง"
	ctx := context.Background()
	claims := middleware.GetClaims(c)

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	defer tx.Rollback(ctx)

	// Lock the document (serializes concurrent saves) and check it is payable.
	var status string
	var netAmount float64
	err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT status, net_amount FROM %s WHERE id = $1 FOR UPDATE`, table), docID).
		Scan(&status, &netAmount)
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("ไม่พบเอกสาร %s id %d", docType, docID))
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if status != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "เอกสารนี้ยังไม่ได้รับการอนุมัติ ไม่สามารถกำหนดงวดการจ่ายเงินได้")
	}
	if docType == "PO" {
		var payable bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM purchase_order po
			               WHERE po.id = $1 AND po.deleted_at IS NULL
			                 AND EXISTS (SELECT 1 FROM purchase_order_line pol
			                             WHERE pol.po_id = po.id AND pol.status <> 'CANCELLED' AND pol.qty_received > 0))`,
			docID).Scan(&payable); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
		if !payable {
			return fiber.NewError(fiber.StatusBadRequest, "ใบสั่งซื้อนี้ยังไม่มีการรับของ จึงยังไม่สามารถกำหนดงวดการจ่ายเงินได้")
		}
	}
	_ = noCol

	// Existing rows, locked.
	type existing struct {
		ID          int64
		Percent     float64
		Amount      float64
		Description *string
		IsPaid      bool
		PaidAt      *time.Time
		PaidBy      *int64
	}
	exRows, err := tx.Query(ctx, `
		SELECT id, percent, amount, description, is_paid, paid_at, paid_by
		FROM payment_installment
		WHERE doc_type = $1 AND doc_id = $2
		ORDER BY installment_no
		FOR UPDATE`, docType, docID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	exMap := map[int64]existing{}
	for exRows.Next() {
		var e existing
		if err := exRows.Scan(&e.ID, &e.Percent, &e.Amount, &e.Description, &e.IsPaid, &e.PaidAt, &e.PaidBy); err != nil {
			exRows.Close()
			return fiber.NewError(fiber.StatusInternalServerError, failMsg)
		}
		exMap[e.ID] = e
	}
	exRows.Close()
	if err := exRows.Err(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}

	if len(req.Installments) == 0 {
		for _, e := range exMap {
			if e.IsPaid {
				return fiber.NewError(fiber.StatusBadRequest, "มีงวดที่จ่ายเงินแล้ว ไม่สามารถปิดการแบ่งงวดได้")
			}
		}
	}
	// Every paid row must survive the replace.
	for id, e := range exMap {
		if e.IsPaid && !seen[id] {
			return fiber.NewError(fiber.StatusBadRequest, "ไม่สามารถลบงวดที่จ่ายเงินแล้วได้")
		}
	}

	// Resolve final values per row and validate against existing rows.
	type finalRow struct {
		ID          *int64
		Percent     float64
		Amount      float64
		Description *string
		IsPaid      bool
		PaidAt      *time.Time
		PaidBy      *int64
	}
	finals := make([]finalRow, 0, len(req.Installments))
	var sumSatang int64
	for i, in := range req.Installments {
		n := i + 1
		var desc *string
		if in.Description != nil {
			t := strings.TrimSpace(*in.Description)
			if t != "" {
				desc = &t
			}
		}
		f := finalRow{ID: in.ID, Description: desc}
		var ex existing
		if in.ID != nil {
			var found bool
			ex, found = exMap[*in.ID]
			if !found {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("งวดที่ %d: ไม่พบงวดนี้ในเอกสาร กรุณาโหลดข้อมูลใหม่", n))
			}
		}

		switch {
		case in.Percent != nil && in.Amount != nil:
			f.Percent, f.Amount = *in.Percent, round2(*in.Amount)
		case in.Percent != nil:
			f.Percent = *in.Percent
			f.Amount = round2(netAmount * f.Percent / 100)
		default:
			f.Amount = round2(*in.Amount)
			if netAmount > 0 {
				f.Percent = math.Round(f.Amount/netAmount*100*10000) / 10000
			}
		}
		if f.Percent > 100 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("งวดที่ %d: เปอร์เซ็นต์ต้องอยู่ระหว่าง 0 ถึง 100", n))
		}

		if in.ID != nil && ex.IsPaid {
			// Paid rows keep their stored percent/amount/description; any difference is an edit attempt.
			sameDesc := (ex.Description == nil && f.Description == nil) ||
				(ex.Description != nil && f.Description != nil && *ex.Description == *f.Description)
			if math.Abs(ex.Amount-f.Amount) > 0.001 || math.Abs(ex.Percent-f.Percent) > 0.00005 || !sameDesc {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("งวดที่ %d จ่ายเงินแล้ว ไม่สามารถแก้ไขจำนวนเงิน เปอร์เซ็นต์ หรือรายละเอียดได้", n))
			}
		}

		// Paid state is never changed here; existing rows keep theirs (payment_log_id is not touched).
		if in.ID != nil {
			f.IsPaid, f.PaidAt, f.PaidBy = ex.IsPaid, ex.PaidAt, ex.PaidBy
		}
		sumSatang += int64(math.Round(f.Amount * 100))
		finals = append(finals, f)
	}
	if sumSatang > int64(math.Round(netAmount*100))+1 {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("ยอดรวมของทุกงวด (%.2f บาท) เกินยอดเอกสาร (%.2f บาท)", float64(sumSatang)/100, netAmount))
	}

	// Delete rows not kept.
	keep := make([]int64, 0, len(seen))
	for id := range seen {
		keep = append(keep, id)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM payment_installment
		WHERE doc_type = $1 AND doc_id = $2 AND NOT (id = ANY($3))`, docType, docID, keep); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	// Pass 1: park kept rows on unique out-of-range numbers so renumbering can't hit UNIQUE(doc_type, doc_id, installment_no).
	if _, err := tx.Exec(ctx, `
		UPDATE payment_installment SET installment_no = 1000000 + id
		WHERE doc_type = $1 AND doc_id = $2`, docType, docID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	// Pass 2: final numbers 1..n in request order.
	for i, f := range finals {
		no := i + 1
		if f.ID != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE payment_installment SET
				    installment_no = $1, percent = $2, amount = $3, description = $4,
				    is_paid = $5, paid_at = $6, paid_by = $7,
				    updated_at = NOW(), updated_by = $8
				WHERE id = $9 AND doc_type = $10 AND doc_id = $11`,
				no, f.Percent, f.Amount, f.Description, f.IsPaid, f.PaidAt, f.PaidBy,
				claims.UserID, *f.ID, docType, docID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, failMsg)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				INSERT INTO payment_installment
				    (doc_type, doc_id, installment_no, percent, amount, description,
				     is_paid, paid_at, paid_by, created_by, updated_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)`,
				docType, docID, no, f.Percent, f.Amount, f.Description,
				f.IsPaid, f.PaidAt, f.PaidBy, claims.UserID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, failMsg)
			}
		}
	}

	items, err := loadInstallments(ctx, tx, docType, docID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if err := tx.Commit(ctx); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"doc_type":         docType,
		"doc_id":           docID,
		"net_amount":       netAmount,
		"installments_sum": round2(float64(sumSatang) / 100),
		"installments":     items,
	}})
}

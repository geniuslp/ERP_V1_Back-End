package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"erp-api/internal/middleware"
)

// poPRNosSQL aggregates distinct pr_no(s) for the outer purchase_order alias po, covering BOTH the
// header link (po.pr_id) and the line link (purchase_order_line.pr_line_id). Same expression as List.
const poPRNosSQL = `(SELECT ARRAY_AGG(DISTINCT x.pr_no ORDER BY x.pr_no) FROM (
		SELECT pr.pr_no
		FROM purchase_order_line pol
		JOIN purchase_request_line prl ON prl.id = pol.pr_line_id
		JOIN purchase_request pr ON pr.id = prl.pr_id AND pr.deleted_at IS NULL
		WHERE pol.po_id = po.id
		UNION
		SELECT pr2.pr_no FROM purchase_request pr2 WHERE pr2.id = po.pr_id AND pr2.deleted_at IS NULL
	) x)`

// poSummaryNewFilters builds the dept_code + search conditions shared by ListLineItems and Export.
// Each condition uses `$n::type IS NULL OR ...`-style optionality by only being appended when the
// param is present; placeholders are numbered from the current length of args.
func poSummaryNewFilters(c *fiber.Ctx, conditions []string, args []any) ([]string, []any) {
	if v := strings.TrimSpace(c.Query("dept_code")); v != "" {
		args = append(args, v)
		// requester's department; fall back to the linked PR's dept_code when requested_by is NULL.
		conditions = append(conditions, fmt.Sprintf(`(CASE
			WHEN po.requested_by IS NOT NULL THEN (SELECT ru.dept_code FROM users ru WHERE ru.id = po.requested_by)
			ELSE (SELECT rp.dept_code FROM purchase_request rp WHERE rp.id = po.pr_id)
		END)::text = $%d::text`, len(args)))
	}
	if v := strings.TrimSpace(c.Query("search")); v != "" {
		args = append(args, "%"+v+"%")
		n := len(args)
		conditions = append(conditions, fmt.Sprintf(`(po.po_no ILIKE $%[1]d::text OR EXISTS (
			SELECT 1 FROM unnest(`+poPRNosSQL+`) AS pn(pr_no)
			WHERE pn.pr_no ILIKE $%[1]d::text))`, n))
	}
	return conditions, args
}

// poCredit returns credit days parsed from supplier.payment_terms, or the raw text when unparseable.
func poCredit(paymentTerms *string) any {
	if d := parsePaymentTermsDays(paymentTerms); d != nil {
		return *d
	}
	if paymentTerms != nil && strings.TrimSpace(*paymentTerms) != "" {
		return *paymentTerms
	}
	return nil
}

// ExportSummary godoc
// @Summary      Export approved POs (one row per PO, no pagination)
// @Description  Same filters as GET /po/line-items (date_from, date_to, po_no, requested_by, project_code,
// @Description  receiving_status, dept_code, search) but returns every matching PO, without pagination and
// @Description  without nested lines. total_amount is pre-VAT after discount (purchase_order.total_amount -
// @Description  discount_amount). credit is credit days parsed from supplier.payment_terms, or the raw text
// @Description  when it cannot be parsed. dept_code = department of the requester (falls back to the linked PR).
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        date_from    query  string  false  "po_date >= (YYYY-MM-DD)"
// @Param        date_to      query  string  false  "po_date <= (YYYY-MM-DD)"
// @Param        po_no        query  string  false  "po_no ILIKE filter"
// @Param        requested_by query  int     false  "requested_by user id filter"
// @Param        project_code query  string  false  "po.project_code filter"
// @Param        receiving_status query string false "NOT_RECEIVED | PARTIAL | COMPLETE"
// @Param        dept_code    query  string  false  "department of the requester (fallback: linked PR dept_code)"
// @Param        search       query  string  false  "matches po_no or any linked pr_no (ILIKE)"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /po/export [get]
func (h *POHandler) ExportSummary(c *fiber.Ctx) error {
	conditions := []string{"po.status = 'APPROVED'", "po.deleted_at IS NULL"}
	var args []any

	if v := strings.TrimSpace(c.Query("date_from")); v != "" {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("po.po_date >= $%d::date", len(args)))
	}
	if v := strings.TrimSpace(c.Query("date_to")); v != "" {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("po.po_date <= $%d::date", len(args)))
	}
	if v := strings.TrimSpace(c.Query("po_no")); v != "" {
		args = append(args, "%"+v+"%")
		conditions = append(conditions, fmt.Sprintf("po.po_no ILIKE $%d", len(args)))
	}
	if v := c.QueryInt("requested_by", 0); v > 0 {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("po.requested_by = $%d", len(args)))
	}
	if v := strings.TrimSpace(c.Query("project_code")); v != "" {
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("po.project_code = $%d", len(args)))
	}
	if v := strings.ToUpper(strings.TrimSpace(c.Query("receiving_status"))); v != "" {
		if v != "NOT_RECEIVED" && v != "PARTIAL" && v != "COMPLETE" {
			return fiber.NewError(fiber.StatusBadRequest, "receiving_status ต้องเป็น NOT_RECEIVED, PARTIAL หรือ COMPLETE")
		}
		args = append(args, v)
		conditions = append(conditions, fmt.Sprintf("(%s) = $%d", poHeaderReceivingSQL, len(args)))
	}
	conditions, args = poSummaryNewFilters(c, conditions, args)

	rows, err := h.db.Query(context.Background(), `
		SELECT po.id, po.po_no, `+poPRNosSQL+`, s.supplier_name,
		       po.total_amount - COALESCE(po.discount_amount, 0), po.net_amount,
		       s.payment_terms, po.project_code, pj.project_name,
		       COALESCE(po.deposit_paid, false), po.remarks, po.status, po.status_receive
		FROM purchase_order po
		LEFT JOIN supplier s  ON s.id = po.supplier_id
		LEFT JOIN project  pj ON pj.project_code = po.project_code
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY po.po_date DESC, po.po_no DESC`, args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถดึงข้อมูลใบสั่งซื้อได้")
	}
	defer rows.Close()

	type exportRow struct {
		ID            int64    `json:"id"`
		PONo          string   `json:"po_no"`
		PRNos         []string `json:"pr_nos"`
		SupplierName  *string  `json:"supplier_name"`
		TotalAmount   float64  `json:"total_amount"`
		NetAmount     float64  `json:"net_amount"`
		Credit        any      `json:"credit"`
		ProjectCode   *string  `json:"project_code"`
		ProjectName   *string  `json:"project_name"`
		DepositPaid   bool     `json:"deposit_paid"`
		Remarks       *string  `json:"remarks"`
		Status        string   `json:"status"`
		StatusReceive string   `json:"status_receive"`
	}
	items := []exportRow{}
	for rows.Next() {
		var r exportRow
		var terms *string
		if err := rows.Scan(&r.ID, &r.PONo, &r.PRNos, &r.SupplierName, &r.TotalAmount, &r.NetAmount,
			&terms, &r.ProjectCode, &r.ProjectName, &r.DepositPaid, &r.Remarks, &r.Status, &r.StatusReceive); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านข้อมูลใบสั่งซื้อได้")
		}
		if r.PRNos == nil {
			r.PRNos = []string{}
		}
		r.Credit = poCredit(terms)
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านข้อมูลใบสั่งซื้อได้")
	}
	return c.JSON(fiber.Map{"success": true, "data": items, "total": len(items)})
}

// ListExport godoc
// @Summary      Export purchase orders (no pagination, no implicit status filter)
// @Description  Same filters and same row fields as GET /po (status, order_type, my, date_from, date_to, dept_code,
// @Description  search), but returns every matching PO in one response. Intended for client-side .xlsx export.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        status    query  string  false  "status filter — comma-separated for multiple"
// @Param        my        query  bool    false  "when true, only POs created by the current user"
// @Param        order_type query string  false  "stock | cost | asset_machine | asset_office_equipment | asset_tools"
// @Param        date_from query  string  false  "po_date >= (YYYY-MM-DD)"
// @Param        date_to   query  string  false  "po_date <= (YYYY-MM-DD)"
// @Param        dept_code query  string  false  "requester's department (fallback: linked PR dept_code)"
// @Param        search    query  string  false  "matches po_no or any linked pr_no (ILIKE)"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /po/list-export [get]
func (h *POHandler) ListExport(c *fiber.Ctx) error {
	where, args, err := buildPOListFilters(c)
	if err != nil {
		return err
	}
	rows, err := h.db.Query(context.Background(), poListSelectSQL+where+` ORDER BY po.created_at DESC`, args...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถดึงข้อมูลใบสั่งซื้อได้")
	}
	defer rows.Close()
	items, err := scanPOListRows(rows)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านข้อมูลใบสั่งซื้อได้")
	}
	return c.JSON(fiber.Map{"success": true, "data": items, "total": len(items)})
}

type setDepositPaidRequest struct {
	DepositPaid *bool `json:"deposit_paid"`
}

// SetDepositPaid godoc
// @Summary      Mark a PO's deposit as paid / unpaid
// @Description  Sets deposit_paid. When true, stamps deposit_paid_at (Asia/Bangkok) and deposit_paid_by (current
// @Description  user); when false, both are reset to NULL. Does not touch status or status_receive.
// @Tags         Purchase Order
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                     true  "PO ID"
// @Param        body  body  setDepositPaidRequest   true  "deposit_paid flag"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /po/{id}/deposit-paid [put]
func (h *POHandler) SetDepositPaid(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "กรุณาเข้าสู่ระบบอีกครั้ง")
	}
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสใบสั่งซื้อไม่ถูกต้อง")
	}
	var req setDepositPaidRequest
	if err := c.BodyParser(&req); err != nil || req.DepositPaid == nil {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุ deposit_paid เป็น true หรือ false")
	}

	ctx := context.Background()
	var exists bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM purchase_order WHERE id = $1 AND deleted_at IS NULL)`, id,
	).Scan(&exists); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบใบสั่งซื้อได้")
	}
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบใบสั่งซื้อ")
	}

	var paidAt *time.Time
	var paidBy *int64
	if *req.DepositPaid {
		now := bangkokNow()
		paidAt = &now
		paidBy = &claims.UserID
	}
	if _, err := h.db.Exec(ctx, `
		UPDATE purchase_order
		SET deposit_paid = $1, deposit_paid_at = $2, deposit_paid_by = $3
		WHERE id = $4`, *req.DepositPaid, paidAt, paidBy, id,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกสถานะมัดจำได้")
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"id": id, "deposit_paid": *req.DepositPaid, "deposit_paid_at": paidAt, "deposit_paid_by": paidBy,
	}})
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

// FinanceHandler covers Payment Tracking — checking whether PO/WO payments have
// been made and rolling paid amounts up per project. Access control for these
// endpoints is intentionally not wired here (handled separately later).
type FinanceHandler struct{ db *pgxpool.Pool }

func NewFinanceHandler(db *pgxpool.Pool) *FinanceHandler {
	return &FinanceHandler{db: db}
}

// docTable resolves doc_type -> the real table/doc-no-column names. Validated
// against this fixed 3-value allowlist before any use in a query string, so
// building SQL with a dynamic table name here never touches unvalidated input.
func docTable(docType string) (table, noCol string, ok bool) {
	switch docType {
	case "PO":
		return "purchase_order", "po_no", true
	case "WO":
		return "work_order", "wo_no", true
	case "PC":
		// petty cash: payment_log.doc_type is 'PC' (not 'PETTY_CASH', which is used by
		// petty_cash_requisition.doc_type and approval_*). Status stays APPROVED; "paid" is derived.
		return "petty_cash_requisition", "pc_no", true
	default:
		return "", "", false
	}
}

// pcProjectSpentSQL / pcProjectPaidSQL are the petty cash terms added to a project's spent_amount /
// paid_amount (project_handler.go, project_overview.go). They expect the outer project alias `p`.
// Spent = SUM(line_net) (fallback amount) of APPROVED petty cash lines of that project. Paid = the
// document's SUM(payment_log.amount_paid, doc_type 'PC', reversal rows included) split by the
// project's share of the document's line_net total; documents whose line total is 0 are skipped.
const pcProjectSpentSQL = `COALESCE((SELECT SUM(COALESCE(l.line_net, l.amount))
		FROM petty_cash_requisition_line l
		JOIN petty_cash_requisition h ON h.id = l.pc_id
		WHERE h.status = 'APPROVED' AND l.project_code = p.project_code), 0)`

const pcProjectPaidSQL = `COALESCE((SELECT SUM(pp.paid * COALESCE(l.line_net, l.amount) / pt.total)
		FROM petty_cash_requisition_line l
		JOIN (SELECT doc_id, SUM(amount_paid) AS paid FROM payment_log
		      WHERE doc_type = 'PC' GROUP BY doc_id) pp ON pp.doc_id = l.pc_id
		JOIN (SELECT pc_id, SUM(COALESCE(line_net, amount)) AS total
		      FROM petty_cash_requisition_line GROUP BY pc_id
		      HAVING SUM(COALESCE(line_net, amount)) <> 0) pt ON pt.pc_id = l.pc_id
		WHERE l.project_code = p.project_code), 0)`

// payableProjectSQL returns, for a document alias d: the SQL for the project_code column, an extra
// join condition source for the project name, and the JSON list of distinct projects.
// PO/WO have one header project. A petty cash document spans projects on its LINES; its
// project_code is the header's when set, else the first line's (line_no order).
func payableProjectSQL(docType string) (codeExpr, projectsExpr string) {
	if docType != "PC" {
		return "d.project_code", `NULL::text`
	}
	codeExpr = `COALESCE(d.project_code, (SELECT l.project_code FROM petty_cash_requisition_line l
			WHERE l.pc_id = d.id ORDER BY l.line_no, l.id LIMIT 1))`
	projectsExpr = `(SELECT COALESCE(json_agg(json_build_object('project_code', x.project_code,
				'project_name', x.project_name) ORDER BY x.first_ln), '[]'::json)::text
			FROM (SELECT l.project_code, MIN(pj2.project_name) AS project_name, MIN(l.line_no) AS first_ln
			      FROM petty_cash_requisition_line l
			      LEFT JOIN project pj2 ON pj2.project_code = l.project_code
			      WHERE l.pc_id = d.id GROUP BY l.project_code) x)`
	return codeExpr, projectsExpr
}

// fillPayableProjects sets Projects/ProjectCount. For PC it decodes the JSON list; for PO/WO it is
// the single header project (empty when the document has none).
func fillPayableProjects(p *models.PayableDoc, projectsJSON *string) {
	p.Projects = []models.PayableDocProject{}
	if projectsJSON != nil && *projectsJSON != "" {
		_ = json.Unmarshal([]byte(*projectsJSON), &p.Projects)
	} else if p.ProjectCode != nil && *p.ProjectCode != "" {
		p.Projects = append(p.Projects, models.PayableDocProject{ProjectCode: *p.ProjectCode, ProjectName: p.ProjectName})
	}
	p.ProjectCount = len(p.Projects)
}

// ListPayableDocs godoc
// @Summary      List PO, WO or PC (petty cash) headers with paid/remaining amounts
// @Tags         Finance
// @Security     BearerAuth
// @Produce      json
// @Description  Only APPROVED documents are returned — a doc that hasn't been approved
// @Description  isn't payable yet, so status is fixed and not caller-settable.
// @Description  Each row carries project_code and project_name (LEFT JOIN project; null when the
// @Description  document has no project or the code matches none).
// @Description  For doc_type=PO, a non-deleted document additionally requires at least one
// @Description  non-CANCELLED line with qty_received > 0 — partial receipt is sufficient. This
// @Description  does not apply to WO. Each PO row carries receiving_status: FULLY_RECEIVED,
// @Description  PARTIALLY_RECEIVED or NOT_RECEIVED (null for WO), computed live from
// @Description  purchase_order_line.qty_received vs qty_ordered, not from grn or status_receive.
// @Param        doc_type      query  string  true   "PO, WO or PC"
// @Param        project_code  query  string  false  "filter project_code"
// @Param        search        query  string  false  "search doc_no"
// @Param        page          query  int     false  "page"
// @Param        page_size     query  int     false  "page size"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /finance/payments [get]
func (h *FinanceHandler) ListPayableDocs(c *fiber.Ctx) error {
	var f models.PayableDocFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	table, noCol, ok := docTable(f.DocType)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type ต้องเป็น PO, WO หรือ PC")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}

	ctx := context.Background()

	// buildFilters renders the optional project_code/status/search conditions with
	// placeholders starting at startIdx. Called separately for the count query (which
	// doesn't reference doc_type at all — table is already scoped to PO or WO) and the
	// list query (which reserves $1 for doc_type), so each query's placeholder numbering
	// is self-consistent instead of sharing one counter across two differently-shaped queries.
	buildFilters := func(startIdx int) (clause string, args []any) {
		// Only APPROVED docs are payable — fixed, not a caller-controlled filter.
		where := []string{"d.status = 'APPROVED'"}
		// PO additionally requires that something has actually been received — partial
		// receipt is sufficient. Judged from purchase_order_line.qty_received, NOT from the
		// grn table: receiving now goes through IC (ic.go SubmitReceiveLines) and
		// POST /grn/receive, neither of which writes grn rows (the live grn table is empty),
		// so a "grn exists" check silently hides every PO. Do not revert this to a GRN check.
		// WO has no receiving concept, so this is scoped to doc_type == 'PO' only.
		if f.DocType == "PO" {
			where = append(where, "d.deleted_at IS NULL")
			where = append(where, `EXISTS (SELECT 1 FROM purchase_order_line pol
				WHERE pol.po_id = d.id AND pol.status <> 'CANCELLED' AND pol.qty_received > 0)`)
		}
		i := startIdx
		if f.ProjectCode != "" {
			if f.DocType == "PC" {
				where = append(where, fmt.Sprintf(`(d.project_code = $%d OR EXISTS (SELECT 1 FROM petty_cash_requisition_line pl2
					WHERE pl2.pc_id = d.id AND pl2.project_code = $%d))`, i, i))
			} else {
				where = append(where, fmt.Sprintf("d.project_code = $%d", i))
			}
			args = append(args, f.ProjectCode)
			i++
		}
		if f.Search != "" {
			where = append(where, fmt.Sprintf("d.%s ILIKE $%d", noCol, i))
			args = append(args, "%"+f.Search+"%")
			i++
		}
		return strings.Join(where, " AND "), args
	}

	countClause, countArgs := buildFilters(1)
	var total int64
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM %s d WHERE %s`, table, countClause)
	if err := h.db.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return err
	}

	listClause, listFilterArgs := buildFilters(2) // $1 reserved for doc_type
	listArgs := append([]any{f.DocType}, listFilterArgs...)
	nextIdx := 2 + len(listFilterArgs)

	offset := (f.Page - 1) * f.PageSize
	listArgs = append(listArgs, f.PageSize, offset)

	// receiving_status is only meaningful for PO (WO has no receiving concept) — computed live
	// from purchase_order_line.qty_received vs qty_ordered (CANCELLED lines ignored). It does
	// NOT use grn/grn_line: receiving goes through IC and POST /grn/receive, which never write
	// grn rows, and it avoids relying on purchase_order.status_receive, which can go stale via
	// the legacy POST /grn/:id/confirm. Output strings are the existing contract.
	receivingStatusSelect := "NULL::text"
	if f.DocType == "PO" {
		receivingStatusSelect = `(
			SELECT CASE
			    WHEN COUNT(*) > 0 AND BOOL_AND(pol.qty_received >= pol.qty_ordered) THEN 'FULLY_RECEIVED'
			    WHEN COALESCE(BOOL_OR(pol.qty_received > 0), false) THEN 'PARTIALLY_RECEIVED'
			    ELSE 'NOT_RECEIVED'
			END
			FROM purchase_order_line pol
			WHERE pol.po_id = d.id AND pol.status <> 'CANCELLED'
		)`
	}

	projCode, projectsExpr := payableProjectSQL(f.DocType)
	listSQL := fmt.Sprintf(`
		SELECT d.id, d.%s, $1 AS doc_type, %s AS project_code, pj.project_name, d.net_amount, d.status,
		       COALESCE((SELECT SUM(pl.amount_paid) FROM payment_log pl
		                 WHERE pl.doc_type = $1 AND pl.doc_id = d.id), 0) AS paid_amount,
		       %s AS receiving_status,
		       %s AS projects_json
		FROM %s d
		LEFT JOIN project pj ON pj.project_code = %s
		WHERE %s
		ORDER BY d.id DESC
		LIMIT $%d OFFSET $%d`, noCol, projCode, receivingStatusSelect, projectsExpr, table, projCode,
		listClause, nextIdx, nextIdx+1)

	rows, err := h.db.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []models.PayableDoc{}
	for rows.Next() {
		var p models.PayableDoc
		var projectsJSON *string
		if err := rows.Scan(&p.Id, &p.DocNo, &p.DocType, &p.ProjectCode, &p.ProjectName, &p.NetAmount, &p.Status, &p.PaidAmount, &p.ReceivingStatus, &projectsJSON); err != nil {
			return err
		}
		fillPayableProjects(&p, projectsJSON)
		p.RemainingToPay = p.NetAmount - p.PaidAmount
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	totalPages := int(total)/f.PageSize + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: f.Page,
			PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// GetPaymentLog godoc
// @Summary      Get payment_log history for one PO, WO or PC
// @Tags         Finance
// @Security     BearerAuth
// @Produce      json
// @Param        doc_type  path  string  true  "PO, WO or PC"
// @Param        doc_id    path  int     true  "document id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /finance/payments/{doc_type}/{doc_id}/log [get]
func (h *FinanceHandler) GetPaymentLog(c *fiber.Ctx) error {
	docType := c.Params("doc_type")
	if _, _, ok := docTable(docType); !ok {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type ต้องเป็น PO, WO หรือ PC")
	}
	docID, err := c.ParamsInt("doc_id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid doc_id")
	}

	ctx := context.Background()
	rows, err := h.db.Query(ctx, `
		SELECT pl.id, pl.doc_type, pl.doc_id, pl.doc_no, pl.amount_paid, pl.paid_date,
		       pl.paid_by, u.full_name AS paid_by_name, pl.remark, pl.reverses_id,
		       pl.created_at, pl.created_by
		FROM payment_log pl
		LEFT JOIN users u ON u.id = pl.paid_by
		WHERE pl.doc_type = $1 AND pl.doc_id = $2
		ORDER BY pl.id ASC`, docType, docID)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []models.PaymentLog{}
	for rows.Next() {
		var p models.PaymentLog
		if err := rows.Scan(&p.Id, &p.DocType, &p.DocID, &p.DocNo, &p.AmountPaid, &p.PaidDate,
			&p.PaidBy, &p.PaidByName, &p.Remark, &p.ReversesID, &p.CreatedAt, &p.CreatedBy); err != nil {
			return err
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// RecordPayment godoc
// @Summary      Record a new payment against a PO, WO or PC
// @Description  New payments only (amount_paid > 0). Locks the document row, requires status APPROVED,
// @Description  rejects a fully-paid document and any amount above the remaining balance. Reversals
// @Description  are no longer accepted here (reverses_id -> 400); use POST /finance/payments/{id}/cancel.
// @Tags         Finance
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.RecordPaymentRequest  true  "Payment data"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /finance/payments [post]
func (h *FinanceHandler) RecordPayment(c *fiber.Ctx) error {
	var req models.RecordPaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
	}
	if req.ReversesID != nil {
		return fiber.NewError(fiber.StatusBadRequest, "การยกเลิกการจ่ายต้องใช้เมนูยกเลิกการจ่าย")
	}
	table, noCol, ok := docTable(req.DocType)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type ต้องเป็น PO, WO หรือ PC")
	}
	if req.AmountPaid <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "จำนวนเงินที่จ่ายต้องมากกว่า 0")
	}
	if req.PaidBy == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุผู้จ่ายเงิน")
	}

	paidDate := bangkokNow().Format("2006-01-02")
	if req.PaidDate != nil && *req.PaidDate != "" {
		if _, err := time.Parse("2006-01-02", *req.PaidDate); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "รูปแบบวันที่จ่ายไม่ถูกต้อง")
		}
		paidDate = *req.PaidDate
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)
	const failMsg = "ไม่สามารถบันทึกการจ่ายเงินได้ กรุณาลองใหม่อีกครั้ง"

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	defer tx.Rollback(ctx)

	// Lock the document first so concurrent payments on the same document serialize.
	var docNo, status string
	var netAmount float64
	err = tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT %s, net_amount, status FROM %s WHERE id = $1 FOR UPDATE`, noCol, table), req.DocID,
	).Scan(&docNo, &netAmount, &status)
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("ไม่พบเอกสาร %s id %d", req.DocType, req.DocID))
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if status != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "เอกสารนี้ยังไม่ได้รับการอนุมัติ ไม่สามารถบันทึกการจ่ายเงินได้")
	}

	var paid float64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_paid), 0) FROM payment_log WHERE doc_type = $1 AND doc_id = $2`,
		req.DocType, req.DocID).Scan(&paid); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	// Compare in satang to avoid float noise on 2-decimal money values.
	remainingSatang := int64(math.Round(netAmount*100)) - int64(math.Round(paid*100))
	if remainingSatang <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "เอกสารนี้จ่ายเงินครบแล้ว ไม่สามารถบันทึกการจ่ายเพิ่มได้")
	}
	if int64(math.Round(req.AmountPaid*100)) > remainingSatang {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("ยอดจ่ายเกินยอดคงเหลือ (คงเหลือ %.2f บาท)", float64(remainingSatang)/100))
	}

	var newID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO payment_log
		    (doc_type, doc_id, doc_no, amount_paid, paid_date, paid_by, remark, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		req.DocType, req.DocID, docNo, req.AmountPaid, paidDate, req.PaidBy, req.Remark, claims.UserID,
	).Scan(&newID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if err := tx.Commit(ctx); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"id": newID, "doc_no": docNo},
	})
}

// CancelPayment godoc
// @Summary      Cancel one payment by inserting a linked negative reversal row
// @Description  The reversal amount is -original.amount_paid taken from the stored row, never from the client.
// @Description  Rejects an empty reason, a payment that is itself a reversal, and a payment already reversed.
// @Tags         Finance
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                          true  "payment_log id"
// @Param        body  body  models.CancelPaymentRequest  true  "Reason"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /finance/payments/{id}/cancel [post]
func (h *FinanceHandler) CancelPayment(c *fiber.Ctx) error {
	payID, err := c.ParamsInt("id")
	if err != nil || payID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "รหัสรายการจ่ายเงินไม่ถูกต้อง")
	}
	var req models.CancelPaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return fiber.NewError(fiber.StatusBadRequest, "กรุณาระบุเหตุผลการยกเลิก")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)
	const failMsg = "ไม่สามารถยกเลิกการจ่ายเงินได้ กรุณาลองใหม่อีกครั้ง"

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	defer tx.Rollback(ctx)

	var docType, docNo string
	var docID, paidBy int64
	var amount float64
	var reversesID *int64
	err = tx.QueryRow(ctx, `
		SELECT doc_type, doc_id, doc_no, amount_paid, paid_by, reverses_id
		FROM payment_log WHERE id = $1 FOR UPDATE`, payID,
	).Scan(&docType, &docID, &docNo, &amount, &paidBy, &reversesID)
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบรายการจ่ายเงินนี้")
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}

	table, _, ok := docTable(docType)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "ประเภทเอกสารของรายการจ่ายเงินไม่ถูกต้อง")
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`SELECT 1 FROM %s WHERE id = $1 FOR UPDATE`, table), docID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}

	if reversesID != nil {
		return fiber.NewError(fiber.StatusBadRequest, "ไม่สามารถยกเลิกรายการที่เป็นการยกเลิกอยู่แล้ว")
	}
	var already bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_log WHERE reverses_id = $1)`, payID).Scan(&already); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if already {
		return fiber.NewError(fiber.StatusBadRequest, "รายการนี้ถูกยกเลิกไปแล้ว")
	}

	var newID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO payment_log
		    (doc_type, doc_id, doc_no, amount_paid, paid_date, paid_by, remark, reverses_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		docType, docID, docNo, -amount, bangkokNow().Format("2006-01-02"), paidBy, reason, payID, claims.UserID,
	).Scan(&newID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fiber.NewError(fiber.StatusBadRequest, "รายการนี้ถูกยกเลิกไปแล้ว")
		}
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}
	if err := tx.Commit(ctx); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, failMsg)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"id": newID, "doc_no": docNo, "reversed_id": payID},
	})
}

// GetPayableDoc godoc
// @Summary      Get one PO, WO or PC in the same shape as a ListPayableDocs row
// @Description  No status filter (a non-APPROVED document is returned so the screen can say why it is not payable).
// @Tags         Finance
// @Security     BearerAuth
// @Produce      json
// @Param        doc_type  path  string  true  "PO, WO or PC"
// @Param        doc_id    path  int     true  "document id"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /finance/payments/{doc_type}/{doc_id} [get]
func (h *FinanceHandler) GetPayableDoc(c *fiber.Ctx) error {
	docType := c.Params("doc_type")
	table, noCol, ok := docTable(docType)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type ต้องเป็น PO, WO หรือ PC")
	}
	docID, err := c.ParamsInt("doc_id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "doc_id ไม่ถูกต้อง")
	}
	receiving := "NULL::text"
	if docType == "PO" {
		receiving = poReceivingStatusSelect
	}
	var p models.PayableDoc
	var projectsJSON *string
	projCode, projectsExpr := payableProjectSQL(docType)
	err = h.db.QueryRow(context.Background(), fmt.Sprintf(`
		SELECT d.id, d.%s, $1::text AS doc_type, %s AS project_code, pj.project_name, d.net_amount, d.status,
		       COALESCE((SELECT SUM(pl.amount_paid) FROM payment_log pl
		                 WHERE pl.doc_type = $1 AND pl.doc_id = d.id), 0) AS paid_amount,
		       %s AS receiving_status,
		       %s AS projects_json
		FROM %s d
		LEFT JOIN project pj ON pj.project_code = %s
		WHERE d.id = $2`, noCol, projCode, receiving, projectsExpr, table, projCode), docType, docID,
	).Scan(&p.Id, &p.DocNo, &p.DocType, &p.ProjectCode, &p.ProjectName, &p.NetAmount, &p.Status, &p.PaidAmount, &p.ReceivingStatus, &projectsJSON)
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("ไม่พบเอกสาร %s id %d", docType, docID))
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดข้อมูลเอกสารได้ กรุณาลองใหม่อีกครั้ง")
	}
	fillPayableProjects(&p, projectsJSON)
	p.RemainingToPay = p.NetAmount - p.PaidAmount
	return c.JSON(fiber.Map{"success": true, "data": p})
}

// poReceivingStatusSelect is the live receiving_status subquery shared by the list and the single-document GET.
const poReceivingStatusSelect = `(
	SELECT CASE
	    WHEN COUNT(*) > 0 AND BOOL_AND(pol.qty_received >= pol.qty_ordered) THEN 'FULLY_RECEIVED'
	    WHEN COALESCE(BOOL_OR(pol.qty_received > 0), false) THEN 'PARTIALLY_RECEIVED'
	    ELSE 'NOT_RECEIVED'
	END
	FROM purchase_order_line pol
	WHERE pol.po_id = d.id AND pol.status <> 'CANCELLED'
)`

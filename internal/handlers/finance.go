package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
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
// against this fixed 2-value allowlist before any use in a query string, so
// building SQL with a dynamic table name here never touches unvalidated input.
func docTable(docType string) (table, noCol string, ok bool) {
	switch docType {
	case "PO":
		return "purchase_order", "po_no", true
	case "WO":
		return "work_order", "wo_no", true
	default:
		return "", "", false
	}
}

// ListPayableDocs godoc
// @Summary      List PO or WO headers with paid/remaining amounts
// @Tags         Finance
// @Security     BearerAuth
// @Produce      json
// @Description  Only APPROVED documents are returned — a doc that hasn't been approved
// @Description  isn't payable yet, so status is fixed and not caller-settable.
// @Param        doc_type      query  string  true   "PO or WO"
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
		return fiber.NewError(fiber.StatusBadRequest, "doc_type must be PO or WO")
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
		i := startIdx
		if f.ProjectCode != "" {
			where = append(where, fmt.Sprintf("d.project_code = $%d", i))
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

	listSQL := fmt.Sprintf(`
		SELECT d.id, d.%s, $1 AS doc_type, d.project_code, d.net_amount, d.status,
		       COALESCE((SELECT SUM(pl.amount_paid) FROM payment_log pl
		                 WHERE pl.doc_type = $1 AND pl.doc_id = d.id), 0) AS paid_amount
		FROM %s d
		WHERE %s
		ORDER BY d.id DESC
		LIMIT $%d OFFSET $%d`, noCol, table, listClause, nextIdx, nextIdx+1)

	rows, err := h.db.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []models.PayableDoc{}
	for rows.Next() {
		var p models.PayableDoc
		if err := rows.Scan(&p.Id, &p.DocNo, &p.DocType, &p.ProjectCode, &p.NetAmount, &p.Status, &p.PaidAmount); err != nil {
			return err
		}
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
// @Summary      Get payment_log history for one PO or WO
// @Tags         Finance
// @Security     BearerAuth
// @Produce      json
// @Param        doc_type  path  string  true  "PO or WO"
// @Param        doc_id    path  int     true  "document id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /finance/payments/{doc_type}/{doc_id}/log [get]
func (h *FinanceHandler) GetPaymentLog(c *fiber.Ctx) error {
	docType := c.Params("doc_type")
	if _, _, ok := docTable(docType); !ok {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type must be PO or WO")
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
// @Summary      Record a payment (or reversal) against a PO or WO
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
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	table, noCol, ok := docTable(req.DocType)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "doc_type must be PO or WO")
	}
	if req.AmountPaid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "amount_paid must not be zero")
	}
	if req.PaidBy == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "paid_by is required")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var docNo string
	docNoSQL := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, noCol, table)
	err := h.db.QueryRow(ctx, docNoSQL, req.DocID).Scan(&docNo)
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("%s id %d not found", req.DocType, req.DocID))
	}
	if err != nil {
		return err
	}

	if req.ReversesID != nil {
		var count int
		err := h.db.QueryRow(ctx, `
			SELECT COUNT(*) FROM payment_log
			WHERE id = $1 AND doc_type = $2 AND doc_id = $3`,
			*req.ReversesID, req.DocType, req.DocID).Scan(&count)
		if err != nil {
			return err
		}
		if count == 0 {
			return fiber.NewError(fiber.StatusBadRequest,
				"reverses_id does not reference an existing payment_log row for this doc_type/doc_id")
		}
	}

	paidDate := time.Now().Format("2006-01-02")
	if req.PaidDate != nil && *req.PaidDate != "" {
		paidDate = *req.PaidDate
	}

	var newID int64
	err = h.db.QueryRow(ctx, `
		INSERT INTO payment_log
		    (doc_type, doc_id, doc_no, amount_paid, paid_date, paid_by, remark, reverses_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		req.DocType, req.DocID, docNo, req.AmountPaid, paidDate, req.PaidBy, req.Remark, req.ReversesID, claims.UserID,
	).Scan(&newID)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"id": newID, "doc_no": docNo},
	})
}

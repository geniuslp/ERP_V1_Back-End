package handlers

import (
	"context"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// GetPOHeader godoc
// @Summary      PO / PR / project header for the IC receive and return modals
// @Description  pr_nos is the DISTINCT, ordered list of PR numbers linked to the PO through BOTH the header link (purchase_order.pr_id) and the line link (purchase_order_line.pr_line_id -> purchase_request_line -> purchase_request) — the same query GET /po uses for its pr_nos, so the two always agree. Never null (empty array when there is none). project_code and project_name are nullable. 404 when the PO does not exist.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId  path  int  true  "purchase_order.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /ic/pos/{poId}/header [get]
func (h *ICHandler) GetPOHeader(c *fiber.Ctx) error {
	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "poId ไม่ถูกต้อง")
	}

	var (
		poNo, orderType          string
		projectCode, projectName *string
		prNos                    []string
	)
	err = h.db.QueryRow(context.Background(), `
		SELECT po.po_no, po.order_type, po.project_code, pj.project_name,
		       (SELECT ARRAY_AGG(DISTINCT x.pr_no ORDER BY x.pr_no) FROM (
		           SELECT pr.pr_no
		           FROM purchase_order_line pol
		           JOIN purchase_request_line prl ON prl.id = pol.pr_line_id
		           JOIN purchase_request pr ON pr.id = prl.pr_id AND pr.deleted_at IS NULL
		           WHERE pol.po_id = po.id
		           UNION
		           SELECT pr2.pr_no FROM purchase_request pr2 WHERE pr2.id = po.pr_id AND pr2.deleted_at IS NULL
		       ) x) AS pr_nos
		FROM purchase_order po
		LEFT JOIN project pj ON pj.project_code = po.project_code
		WHERE po.id = $1 AND po.deleted_at IS NULL`, poID,
	).Scan(&poNo, &orderType, &projectCode, &projectName, &prNos)
	if err == pgx.ErrNoRows {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบใบสั่งซื้อ")
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถดึงข้อมูลใบสั่งซื้อได้ กรุณาลองใหม่อีกครั้ง")
	}
	if prNos == nil {
		prNos = []string{}
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"po_id": poID, "po_no": poNo, "pr_nos": prNos,
		"project_code": projectCode, "project_name": projectName, "order_type": orderType,
	}})
}

// ICPODocument is one row of GET /ic/pos/{poId}/documents. Keys are identical for both doc types;
// fields that do not apply to a type are null.
type ICPODocument struct {
	DocType          string   `json:"doc_type"`
	ID               int64    `json:"id"`
	DocNo            *string  `json:"doc_no"`
	CreatedAt        string   `json:"created_at"`
	CreatedByName    *string  `json:"created_by_name"`
	LineCount        int64    `json:"line_count"`
	TotalQty         *float64 `json:"total_qty"`
	TaxInvoiceNo     *string  `json:"tax_invoice_no"`
	TaxInvoiceDate   *string  `json:"tax_invoice_date"`
	TempDeliveryNo   *string  `json:"temp_delivery_no"`
	TempDeliveryDate *string  `json:"temp_delivery_date"`
	RefKind          *string  `json:"ref_kind"`
	RefNo            *string  `json:"ref_no"`
	RefDate          *string  `json:"ref_date"`
	DueDate          *string  `json:"due_date"`
	ReturnDate       *string  `json:"return_date"`
}

// ListPODocuments godoc
// @Summary      List ALL documents of a PO — receives and returns merged
// @Description  Newest first (created_at DESC, then id DESC). doc_type RECEIVE|RETURN; id is ic_po_receive_document.id or ic_po_return_document.id; doc_no is receive_no (null for a receive document with no number yet, same as GET /ic/pos/{poId}/receive-documents) or return_no. line_count uses the same logic as the existing receive list (ledger rows linked by receive_document_id across ic_project_cost_item_transaction, ic_project_receipt_pool_transaction and stock_transaction) and return list (ic_po_return_line rows). total_qty: RECEIVE = SUM(qty) of those same linked ledger rows (stock and cost branches); RETURN = SUM(ic_po_return_line.return_qty). total_qty is null for a RECEIVE document that has no linked ledger rows (an empty draft, or legacy rows written before receive_document_id existed) — it cannot be derived reliably. RECEIVE only: tax_invoice_no, tax_invoice_date, temp_delivery_no, temp_delivery_date, due_date and the server-computed single reference ref_kind / ref_no / ref_date: ref_kind = TAX_INVOICE when tax_invoice_no is non-empty (after trim; primary when both exist), else TEMP_DELIVERY when temp_delivery_no is non-empty, else null; ref_no = the number of that kind; ref_date = the date of that kind, falling back to the other date when it is null (never invented; null when both are null or when ref_kind is null). RETURN only: return_date (ref_kind/ref_no/ref_date are null for returns). Fields that do not apply are null; every row has the same keys. 404 when the PO does not exist.
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId  path  int  true  "purchase_order.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /ic/pos/{poId}/documents [get]
func (h *ICHandler) ListPODocuments(c *fiber.Ctx) error {
	ctx := context.Background()

	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "poId ไม่ถูกต้อง")
	}
	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM purchase_order WHERE id = $1)`, poID).Scan(&exists); err != nil || !exists {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบใบสั่งซื้อ")
	}

	rows, err := h.db.Query(ctx, `
		SELECT doc_type, id, doc_no, created_at, created_by_name, line_count, total_qty,
		       tax_invoice_no, tax_invoice_date, due_date, return_date,
		       temp_delivery_no, temp_delivery_date, ref_kind, ref_no, ref_date
		FROM (
			SELECT 'RECEIVE'::text AS doc_type, rd.id, rd.receive_no AS doc_no, rd.created_at,
			       u.full_name AS created_by_name,
			       COALESCE(t.line_count, 0)::bigint AS line_count, t.total_qty::float8 AS total_qty,
			       rd.tax_invoice_no, rd.tax_invoice_date::text AS tax_invoice_date, rd.due_date::text AS due_date,
			       NULL::text AS return_date,
			       rd.temp_delivery_no, rd.temp_delivery_date::text AS temp_delivery_date,
			       CASE WHEN NULLIF(BTRIM(rd.tax_invoice_no), '') IS NOT NULL THEN 'TAX_INVOICE'
			            WHEN NULLIF(BTRIM(rd.temp_delivery_no), '') IS NOT NULL THEN 'TEMP_DELIVERY' END AS ref_kind,
			       CASE WHEN NULLIF(BTRIM(rd.tax_invoice_no), '') IS NOT NULL THEN BTRIM(rd.tax_invoice_no)
			            WHEN NULLIF(BTRIM(rd.temp_delivery_no), '') IS NOT NULL THEN BTRIM(rd.temp_delivery_no) END AS ref_no,
			       CASE WHEN NULLIF(BTRIM(rd.tax_invoice_no), '') IS NOT NULL THEN COALESCE(rd.tax_invoice_date, rd.temp_delivery_date)::text
			            WHEN NULLIF(BTRIM(rd.temp_delivery_no), '') IS NOT NULL THEN COALESCE(rd.temp_delivery_date, rd.tax_invoice_date)::text END AS ref_date
			FROM ic_po_receive_document rd
			LEFT JOIN users u ON u.id = rd.created_by
			LEFT JOIN (
				SELECT receive_document_id, COUNT(*) AS line_count, SUM(qty) AS total_qty FROM (
					SELECT receive_document_id, qty FROM ic_project_cost_item_transaction WHERE receive_document_id IS NOT NULL
					UNION ALL
					SELECT receive_document_id, qty FROM ic_project_receipt_pool_transaction WHERE receive_document_id IS NOT NULL
					UNION ALL
					SELECT receive_document_id, qty FROM stock_transaction WHERE receive_document_id IS NOT NULL
				) combined
				GROUP BY receive_document_id
			) t ON t.receive_document_id = rd.id
			WHERE rd.po_id = $1
			UNION ALL
			SELECT 'RETURN'::text, d.id, d.return_no, d.created_at, u2.full_name,
			       COUNT(rl.id)::bigint, SUM(rl.return_qty)::float8,
			       NULL::text, NULL::text, NULL::text, d.return_date::text,
			       NULL::text, NULL::text, NULL::text, NULL::text, NULL::text
			FROM ic_po_return_document d
			LEFT JOIN users u2 ON u2.id = d.created_by
			LEFT JOIN ic_po_return_line rl ON rl.return_id = d.id
			WHERE d.po_id = $1
			GROUP BY d.id, d.return_no, d.created_at, u2.full_name, d.return_date
		) docs
		ORDER BY created_at DESC, id DESC`, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถดึงรายการเอกสารของใบสั่งซื้อได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()

	items := []ICPODocument{}
	for rows.Next() {
		var d ICPODocument
		var createdAt time.Time
		if err := rows.Scan(&d.DocType, &d.ID, &d.DocNo, &createdAt, &d.CreatedByName, &d.LineCount, &d.TotalQty,
			&d.TaxInvoiceNo, &d.TaxInvoiceDate, &d.DueDate, &d.ReturnDate,
			&d.TempDeliveryNo, &d.TempDeliveryDate, &d.RefKind, &d.RefNo, &d.RefDate); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านรายการเอกสารของใบสั่งซื้อได้")
		}
		d.CreatedAt = createdAt.Format("2006-01-02 15:04:05.999999")
		items = append(items, d)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

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

type StockTransactionHandler struct{ db *pgxpool.Pool }

func NewStockTransactionHandler(db *pgxpool.Pool) *StockTransactionHandler {
	return &StockTransactionHandler{db: db}
}

func timeYYMM() string {
	return time.Now().Format("0601")
}

func generateTxnNo(ctx context.Context, tx pgx.Tx) (string, error) {
	var seq int64
	if err := tx.QueryRow(ctx, "SELECT nextval('stock_txn_seq')").Scan(&seq); err != nil {
		return "", err
	}
	return fmt.Sprintf("TXN-%s-%04d", timeYYMM(), seq), nil
}

// List godoc
// @Summary      รายการ Stock Transaction
// @Tags         Stock
// @Security     BearerAuth
// @Produce      json
// @Param        txn_type  query  string  false  "Transaction type"
// @Param        date_from query  string  false  "Date from (YYYY-MM-DD)"
// @Param        date_to   query  string  false  "Date to (YYYY-MM-DD)"
// @Param        search    query  string  false  "Search"
// @Param        page      query  int     false  "Page"
// @Param        page_size query  int     false  "Page size"
// @Success      200  {object}  fiber.Map
// @Router       /stock/transactions [get]
func (h *StockTransactionHandler) List(c *fiber.Ctx) error {
	var f models.StockTransactionFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}

	ctx := context.Background()

	where := []string{"1=1"}
	args := []interface{}{}
	i := 1

	txnType := f.TxnType
	if txnType == "" {
		txnType = f.Type
	}
	if txnType != "" {
		// Accepts either a single value or a comma-separated list (e.g. the
		// frontend's "Adjust" filter sends "ADJUST_PLUS,ADJUST_MINUS" since
		// plain "ADJUST" isn't a real txn_type value) — ANY() handles both
		// a one-element and multi-element slice identically.
		types := strings.Split(txnType, ",")
		for idx, t := range types {
			types[idx] = strings.TrimSpace(t)
		}
		where = append(where, fmt.Sprintf("st.txn_type = ANY($%d)", i))
		args = append(args, types)
		i++
	}
	if f.DateFrom != "" {
		where = append(where, fmt.Sprintf("st.txn_date >= $%d", i))
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where = append(where, fmt.Sprintf("st.txn_date <= $%d", i))
		args = append(args, f.DateTo)
		i++
	}
	if f.MatCode != "" {
		where = append(where, fmt.Sprintf("si.mat_code = $%d", i))
		args = append(args, f.MatCode)
		i++
	}
	if f.PONo != "" {
		where = append(where, fmt.Sprintf("po.po_no = $%d", i))
		args = append(args, f.PONo)
		i++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf(
			"(si.mat_code ILIKE $%d OR si.item_name ILIKE $%d OR st.txn_no ILIKE $%d OR po.po_no ILIKE $%d OR pr_doc.pr_no ILIKE $%d)",
			i, i+1, i+2, i+3, i+4))
		like := "%" + f.Search + "%"
		args = append(args, like, like, like, like, like)
		i += 5
	}

	whereClause := strings.Join(where, " AND ")

	// LEFT JOIN grn (via ref_doc_id when ref_doc_type='GRN') and purchase_order through grn.po_id,
	// so GRN-sourced rows can carry po_id/po_no/scores without a second request; non-GRN rows
	// (existing txn_types like ISSUE/TRANSFER/ADJUST) simply get NULLs here and are unaffected.
	//
	// ref_doc_no resolution: one LEFT JOIN per ref_doc_type actually written by any INSERT INTO
	// stock_transaction in this codebase (GRN, PR, REQUISITION, BORROW — confirmed by grepping
	// every insert site), each gated on st.ref_doc_type so exactly zero or one of them matches
	// per row; COALESCE below picks whichever matched. 'PO' is never a literal ref_doc_type here
	// (a GRN's PO is reached via grn.po_id, already joined) so it's not listed separately.
	// For GRN-sourced rows specifically, ref_doc_no resolves to the PO number (po.po_no), not the
	// GRN number — users recognize PO numbers, not GRN numbers. The GRN number itself is still
	// exposed separately as grn_no (already joined) so traceability to the receiving document
	// isn't lost, it's just no longer the primary ref_doc_no.
	// STOCK_TRANSFER is a real ref_doc_type value (see stock_transfer.go) but the `stock_transfer`
	// table does not exist in this database (confirmed via information_schema) — that's a
	// pre-existing schema gap, not something to paper over here; those rows simply resolve
	// ref_doc_no to NULL until that table exists.
	joinClause := `
		FROM stock_transaction st
		JOIN stock_item si ON si.id = st.item_id
		LEFT JOIN users u ON u.id = st.created_by
		LEFT JOIN grn ON st.ref_doc_type = 'GRN' AND grn.id = st.ref_doc_id
		LEFT JOIN purchase_order po ON po.id = grn.po_id
		LEFT JOIN purchase_request pr_doc ON st.ref_doc_type = 'PR' AND pr_doc.id = st.ref_doc_id
		LEFT JOIN requisition req_doc ON st.ref_doc_type = 'REQUISITION' AND req_doc.id = st.ref_doc_id
		LEFT JOIN borrow borrow_doc ON st.ref_doc_type = 'BORROW' AND borrow_doc.id = st.ref_doc_id`

	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*)
		%s
		WHERE %s`, joinClause, whereClause), countArgs...).Scan(&total); err != nil {
		return err
	}

	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT st.id, st.txn_no, st.txn_type,
		       st.item_id, si.mat_code, si.item_name,
		       st.from_location, st.to_location, st.qty,
		       st.qty_before, st.qty_after,
		       st.ref_doc_type, st.ref_doc_id, st.remarks,
		       COALESCE(po.po_no, pr_doc.pr_no, req_doc.req_no, borrow_doc.borrow_no) AS ref_doc_no,
		       TO_CHAR(st.txn_date, 'YYYY-MM-DD') AS txn_date,
		       COALESCE(u.full_name, '') AS created_by_name,
		       st.created_at,
		       grn.po_id, po.po_no, grn.grn_no,
		       grn.score_quality, grn.score_quantity, grn.score_ontime, grn.score_notes
		%s
		WHERE %s
		ORDER BY st.created_at DESC
		LIMIT $%d OFFSET $%d`, joinClause, whereClause, i, i+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var txns []models.StockTransaction
	for rows.Next() {
		var t models.StockTransaction
		if err := rows.Scan(
			&t.ID, &t.TxnNo, &t.TxnType,
			&t.ItemID, &t.MatCode, &t.ItemName,
			&t.FromLocation, &t.ToLocation, &t.Qty,
			&t.QtyBefore, &t.QtyAfter,
			&t.RefDocType, &t.RefDocID, &t.Remarks, &t.RefDocNo,
			&t.TxnDate, &t.CreatedByName, &t.CreatedAt,
			&t.GRNPOID, &t.GRNPONo, &t.GRNNo,
			&t.GRNScoreQuality, &t.GRNScoreQuantity, &t.GRNScoreOntime, &t.GRNScoreNotes,
		); err != nil {
			return err
		}
		txns = append(txns, t)
	}
	if txns == nil {
		txns = []models.StockTransaction{}
	}

	totalPages := int(total) / f.PageSize
	if int(total)%f.PageSize != 0 {
		totalPages++
	}
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: txns, Total: total, Page: f.Page, PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// allowedTxnTypes mirrors the live stock_txn_type_check DB constraint exactly
// (IN, OUT, TRANSFER, ADJUST_PLUS, ADJUST_MINUS, BORROW_OUT, BORROW_RETURN).
// TxnTypeReceive/TxnTypeIssue/TxnTypeReturn ("RECEIVE"/"ISSUE"/"RETURN") are NOT
// in this list on purpose — those constants don't match the constraint (see
// CLAUDE.md "Session learnings (2026-08-16) #4" and the pr.go fix on 2026-08-27).
var allowedTxnTypes = map[string]bool{
	"IN": true, "OUT": true,
	TxnTypeAdjustPlus: true, TxnTypeAdjustMinus: true,
	TxnTypeTransfer: true, TxnTypeBorrowOut: true, TxnTypeBorrowReturn: true,
}

// Create godoc
// @Summary      สร้าง Stock Transaction
// @Tags         Stock
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateStockTransactionRequest  true  "Request body"
// @Success      201  {object}  fiber.Map
// @Router       /stock/transactions [post]
func (h *StockTransactionHandler) Create(c *fiber.Ctx) error {
	var req models.CreateStockTransactionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if !allowedTxnTypes[req.TxnType] {
		return fiber.NewError(fiber.StatusBadRequest, "invalid txn_type")
	}
	if req.Qty <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "qty must be positive")
	}

	ctx := context.Background()
	claims := middleware.GetClaims(c)

	var itemExists bool
	h.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM stock_item WHERE id=$1)", req.ItemID).Scan(&itemExists)
	if !itemExists {
		return fiber.NewError(fiber.StatusBadRequest, "item not found")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	txnNo, err := generateTxnNo(ctx, tx)
	if err != nil {
		return err
	}

	locationCode := req.ToLocation
	if locationCode == nil {
		locationCode = req.FromLocation
	}

	if locationCode != nil {
		delta := req.Qty
		if req.TxnType == "OUT" || req.TxnType == TxnTypeAdjustMinus || req.TxnType == TxnTypeBorrowOut {
			delta = -req.Qty
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO stock_inventory (item_id, location_code, qty_on_hand, qty_reserved)
			VALUES ($1, $2, $3, 0)
			ON CONFLICT (item_id, location_code) DO UPDATE
			SET qty_on_hand = stock_inventory.qty_on_hand + $3, updated_at=NOW()`,
			req.ItemID, *locationCode, delta)
		if err != nil {
			return err
		}
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO stock_transaction (txn_no, txn_type, item_id, from_location, to_location, qty, remarks, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		txnNo, req.TxnType, req.ItemID, req.FromLocation, req.ToLocation, req.Qty, req.Remarks, claims.UserID,
	).Scan(&id)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id, "txn_no": txnNo}})
}

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PRHandler struct {
	db *pgxpool.Pool
}

func NewPRHandler(db *pgxpool.Pool) *PRHandler {
	return &PRHandler{db: db}
}

// validateMatCodesExist checks that every mat_code in matCodes has a matching row in
// material_code, returning a clear 400 error naming the first missing one instead of letting
// the INSERT fail on the mat_code FK constraint with a raw Postgres error.
func validateMatCodesExist(ctx context.Context, db *pgxpool.Pool, matCodes []string) error {
	seen := make(map[string]bool)
	var codes []string
	for _, m := range matCodes {
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		codes = append(codes, m)
	}
	if len(codes) == 0 {
		return nil
	}

	rows, err := db.Query(ctx, `SELECT mat_code FROM material_code WHERE mat_code = ANY($1)`, codes)
	if err != nil {
		return err
	}
	defer rows.Close()

	found := make(map[string]bool)
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return err
		}
		found[m] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range codes {
		if !found[m] {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("Material Code '%s' นี้ไม่มีในระบบ ไม่สามารถบันทึกได้", m))
		}
	}
	return nil
}

// ListPR godoc
// NOTE: this handler is currently unreachable — RegisterPRApprovalRoutes (routes/pr.go)
// registers PRApprovalHandler.List on the same GET /pr path, after this one, and wins.
// Left as-is / not fixed here since it's out of scope for this task; see pr_approval.go
// for the endpoint that actually backs GET /pr today.
// @Summary      List purchase requests
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        status    query  string  false  "status filter"
// @Param        priority  query  string  false  "priority filter"
// @Param        page      query  int     false  "page"  default(1)
// @Param        page_size query  int     false  "page_size"  default(20)
// @Success      200  {object}  models.PaginatedResponse
// @Router       /pr [get]
func (h *PRHandler) List(c *fiber.Ctx) error {
	log.Println("✅ List handler called")

	page := max(c.QueryInt("page", 1), 1)
	size := min(c.QueryInt("page_size", 20), 100)
	offset := (page - 1) * size

	var total int64
	err := h.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM v_pr_full`).Scan(&total)
	if err != nil {
		log.Printf("❌ count error: %v", err)
		return c.Status(500).JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT pr_id, pr_no, pr_date, required_date, pr_status, priority,
		       requested_by_name, location_name, location_type, warehouse_name, line_count, remarks, created_at
		FROM v_pr_full ORDER BY created_at DESC LIMIT $1 OFFSET $2`, size, offset)
	if err != nil {
		log.Printf("❌ query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	defer rows.Close()

	type PRRow struct {
		PRID          int64      `json:"pr_id"`
		PRNo          string     `json:"pr_no"`
		PRDate        time.Time  `json:"pr_date"`
		RequiredDate  *time.Time `json:"required_date,omitempty"`
		Status        string     `json:"status"`
		Priority      string     `json:"priority"`
		RequestedBy   string     `json:"requested_by"`
		LocationName  string     `json:"location_name"`
		LocationType  string     `json:"location_type"`
		WarehouseName *string    `json:"warehouse_name,omitempty"`
		LineCount     int        `json:"line_count"`
		Remarks       *string    `json:"remarks,omitempty"`
		CreatedAt     time.Time  `json:"created_at"`
	}

	var items []PRRow
	for rows.Next() {
		var r PRRow
		if err := rows.Scan(
			&r.PRID, &r.PRNo, &r.PRDate, &r.RequiredDate, &r.Status, &r.Priority,
			&r.RequestedBy, &r.LocationName, &r.LocationType, &r.WarehouseName,
			&r.LineCount, &r.Remarks, &r.CreatedAt,
		); err != nil {
			log.Printf("❌ scan error: %v", err)
			return c.Status(500).JSON(fiber.Map{"success": false, "message": err.Error()})
		}
		items = append(items, r)
	}

	if items == nil {
		items = []PRRow{}
	}

	totalPages := int(total)/size + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data:       items,
			Total:      total,
			Page:       page,
			PageSize:   size,
			TotalPages: totalPages,
		},
	})
}

// CreatePR godoc
// @Summary      Create purchase request
// @Description  job_code is required — one of the 12 fixed job type codes (MP, ME, MS, MF, MG, MH, FS, FP, FB, DE, RE, G).
// @Description  lines[].deduct_stock (optional, default true): whether Submit should reserve this line against stock_item.qty. false skips deduction entirely — the whole qty routes to qty_to_order for PO.
// @Tags         Purchase Request
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreatePRRequest  true  "PR data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /pr [post]
func (h *PRHandler) Create(c *fiber.Ctx) error {
	var req models.CreatePRRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.PRNo == "" || req.LocationText == "" {
		return fiber.NewError(fiber.StatusBadRequest, "pr_no and location_text are required")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line required")
	}
	matCodes := make([]string, len(req.Lines))
	for i, line := range req.Lines {
		matCodes[i] = line.MatCode
	}
	if err := validateMatCodesExist(context.Background(), h.db, matCodes); err != nil {
		return err
	}

	for _, line := range req.Lines {
		if line.CostSubgroupID == nil {
			continue
		}
		var exists bool
		if err := h.db.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM cost_subgroup WHERE id = $1)`, *line.CostSubgroupID,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("cost_subgroup_id %d not found for mat_code %s", *line.CostSubgroupID, line.MatCode))
		}
	}

	if req.OrderType == "" {
		req.OrderType = "stock"
	}
	if req.OrderType != "stock" && req.OrderType != "cost" {
		return fiber.NewError(fiber.StatusBadRequest, "order_type must be 'stock' or 'cost'")
	}

	if req.PRType == "" {
		req.PRType = "PO_WO"
	}
	if req.PRType != "PO_WO" && req.PRType != "PO_ONLY" && req.PRType != "WO_ONLY" {
		return fiber.NewError(fiber.StatusBadRequest, "pr_type must be 'PO_WO', 'PO_ONLY', or 'WO_ONLY'")
	}

	if strings.TrimSpace(req.JobCode) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "job_code is required")
	}
	if err := ValidateJobCode(req.JobCode); err != nil {
		return err
	}

	if req.MemoID != nil {
		var existingPRNo string
		err := h.db.QueryRow(context.Background(),
			`SELECT pr_no FROM purchase_request WHERE memo_id = $1`, *req.MemoID,
		).Scan(&existingPRNo)
		if err == nil {
			return fiber.NewError(fiber.StatusConflict, "memo already used by PR "+existingPRNo)
		} else if err != pgx.ErrNoRows {
			return err
		}

		// Snapshot the memo's delivery_location into location_text at selection time —
		// only when the memo actually has one set, otherwise keep the client-supplied value.
		var memoDeliveryLocation *string
		if err := h.db.QueryRow(context.Background(),
			`SELECT delivery_location FROM public.memo WHERE id = $1`, *req.MemoID,
		).Scan(&memoDeliveryLocation); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "memo not found")
		}
		if memoDeliveryLocation != nil && strings.TrimSpace(*memoDeliveryLocation) != "" {
			req.LocationText = *memoDeliveryLocation
		}
	}

	tx, err := h.db.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	// 1. Insert PR header
	var prID int64
	err = tx.QueryRow(context.Background(), `
		INSERT INTO purchase_request
		    (pr_no, pr_date, requested_by, location_text, warehouse_code, required_date,
		     project_code, dept_code, status, order_type, pr_type, job_code, remarks, memo_id, created_at, updated_at, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,now(),now(),$15,$15)
		RETURNING id`,
		req.PRNo, req.PRDate, req.RequestedBy, req.LocationText, req.WarehouseCode,
		req.RequiredDate, req.ProjectCode, req.DeptCode, req.Status, req.OrderType, req.PRType, req.JobCode, req.Remarks, req.MemoID, req.CreatedBy,
	).Scan(&prID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create PR: "+err.Error())
	}

	// 2. Insert lines
	for _, line := range req.Lines {
		deductStock := true
		if line.DeductStock != nil {
			deductStock = *line.DeductStock
		}
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO purchase_request_line (pr_id, line_no, mat_code, qty_requested, status, cost_subgroup_id, deduct_stock)
			VALUES ($1,$2,$3,$4,'OPEN',$5,$6)`,
			prID, line.LineNo, line.MatCode, line.QtyRequested, line.CostSubgroupID, deductStock,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to insert line: "+err.Error())
		}
	}

	// 3. Insert attachments (if any) — file_path came from a prior /upload/pr call; verify the
	// file is actually on disk before creating a row that references it (see fileurl.go).
	for _, att := range req.Attachments {
		if _, err := os.Stat(toRelativeDiskPath(att.FilePath)); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("attachment %q was not found on disk — please re-upload", att.FileName))
		}
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO pr_attachment (pr_id, file_name, file_path, file_size, file_type, uploaded_by, uploaded_at)
			VALUES ($1,$2,$3,$4,$5,$6,now())`,
			prID, att.FileName, att.FilePath, att.FileSize, att.FileType, req.CreatedBy,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to insert attachment: "+err.Error())
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"id": prID, "pr_no": req.PRNo},
	})
}

// SubmitPR godoc
// @Summary      Submit PR (no approval required — goes straight to COMPLETED)
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PR ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /pr/{id}/submit [post]
func (h *PRHandler) Submit(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PR id")
	}
	ctx := context.Background()

	var currentStatus string
	var prNo string
	var orderType string
	var projectCode *string
	if err := h.db.QueryRow(ctx,
		`SELECT status, pr_no, order_type, project_code FROM purchase_request WHERE id=$1`, id,
	).Scan(&currentStatus, &prNo, &orderType, &projectCode); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}
	if currentStatus != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("cannot submit PR in status: %s", currentStatus))
	}
	if orderType == "cost" && projectCode == nil {
		return fiber.NewError(fiber.StatusBadRequest, "ต้องระบุโครงการก่อนส่งใบขอซื้อประเภทซื้อเข้าโครงการ")
	}

	// Reopen's reversal query finds prior deductions by ref_doc_id + mat_code only (no
	// pr_line_id column on stock_transaction), so it can't distinguish two lines with the
	// same mat_code but different deduct_stock settings. Reject that combination up front.
	{
		deductByMatCode := make(map[string]bool)
		mixed := make(map[string]bool)
		rows, err := h.db.Query(ctx, `SELECT mat_code, deduct_stock FROM purchase_request_line WHERE pr_id=$1`, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var matCode string
			var deductStock bool
			if err := rows.Scan(&matCode, &deductStock); err != nil {
				rows.Close()
				return err
			}
			if prev, seen := deductByMatCode[matCode]; seen {
				if prev != deductStock {
					mixed[matCode] = true
				}
			} else {
				deductByMatCode[matCode] = deductStock
			}
		}
		rows.Close()
		if len(mixed) > 0 {
			matCodes := make([]string, 0, len(mixed))
			for mc := range mixed {
				matCodes = append(matCodes, mc)
			}
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf(
				"mixed deduct_stock settings aren't supported for duplicate mat_code within one PR (mat_code: %s) — "+
					"Reopen's stock reversal is keyed by mat_code, not by line, so it cannot tell which line's deduction to reverse",
				strings.Join(matCodes, ", ")))
		}
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := h.deductStockOnSubmit(ctx, tx, id, orderType, projectCode, claims.UserID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE purchase_request SET status='COMPLETED', updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO pr_status_log (pr_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'COMPLETED',$3,'submitted')`, id, currentStatus, claims.UserID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO erp_audit_log (table_name, record_id, action, changed_by, new_data)
		VALUES ('purchase_request',$1,'UPDATE',$2,$3)`,
		id, claims.UserID, fmt.Sprintf(`{"pr_no":"%s","status":"COMPLETED"}`, prNo),
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "PR status changed to COMPLETED"})
}

// deductStockOnSubmit implements the "auto-split from available stock" rule for PR
// submission: for every line, whatever is currently on hand in stock_item is reserved
// immediately (first-come-first-served, no blocking), and the remainder is routed to
// qty_to_order for later procurement via PO. qty_reserved and qty_to_order are persisted
// on purchase_request_line so downstream PO-line creation knows how much is still
// orderable. For order_type='cost' PRs, the reserved qty is also mirrored into
// project_stock via addToProjectStock, matching how Requisition/StockTransfer track
// project-level usage. All writes happen inside the caller's tx so a failure anywhere
// rolls back PR completion too.
// Lines with deduct_stock=false skip stock_item/stock_transaction entirely — qty_reserved
// is forced to 0 and qty_to_order to qty_requested, routing the whole line to PO.
func (h *PRHandler) deductStockOnSubmit(ctx context.Context, tx pgx.Tx, prID int64, orderType string, projectCode *string, userID int64) error {
	rows, err := tx.Query(ctx, `
		SELECT id, mat_code, qty_requested, deduct_stock
		FROM purchase_request_line
		WHERE pr_id = $1`, prID)
	if err != nil {
		return err
	}
	type line struct {
		ID           int64
		MatCode      string
		QtyRequested float64
		DeductStock  bool
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.ID, &l.MatCode, &l.QtyRequested, &l.DeductStock); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	rows.Close()

	for _, l := range lines {
		if !l.DeductStock {
			if _, err := tx.Exec(ctx, `
				UPDATE purchase_request_line
				SET qty_reserved = 0, qty_to_order = qty_requested
				WHERE id = $1`,
				l.ID,
			); err != nil {
				return err
			}
			continue
		}

		var itemID int64
		var available float64
		var unit *string
		err := tx.QueryRow(ctx,
			`SELECT id, qty, unit FROM stock_item WHERE mat_code = $1 FOR UPDATE`, l.MatCode,
		).Scan(&itemID, &available, &unit)
		hasStockItem := err == nil
		if !hasStockItem {
			available = 0
		}

		qtyReserved := l.QtyRequested
		if qtyReserved > available {
			qtyReserved = available
		}
		if qtyReserved < 0 {
			qtyReserved = 0
		}
		qtyToOrder := l.QtyRequested - qtyReserved

		if _, err := tx.Exec(ctx, `
			UPDATE purchase_request_line
			SET qty_reserved = $1, qty_to_order = $2
			WHERE id = $3`,
			qtyReserved, qtyToOrder, l.ID,
		); err != nil {
			return err
		}

		if qtyReserved <= 0 {
			continue
		}

		if _, err := tx.Exec(ctx,
			`UPDATE stock_item SET qty = qty - $1, updated_at = NOW() WHERE id = $2`,
			qtyReserved, itemID,
		); err != nil {
			return err
		}

		txnNo, err := generateTxnNo(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_transaction
			    (txn_no, txn_type, item_id, qty, ref_doc_type, ref_doc_id, remarks, txn_date, created_by)
			VALUES ($1,$2,$3,$4,'PR',$5,'ตัด stock อัตโนมัติจากการ submit PR',CURRENT_DATE,$6)`,
			// 'OUT', not TxnTypeIssue ("ISSUE") — stock_txn_type_check only allows
			// IN/OUT/TRANSFER/ADJUST_PLUS/ADJUST_MINUS/BORROW_OUT/BORROW_RETURN; the
			// TxnType* constants in stock_constants.go don't match the live constraint.
			// See CLAUDE.md "Session learnings (2026-08-16) #4" and the fix on 2026-08-27.
			txnNo, "OUT", itemID, qtyReserved, prID, userID,
		); err != nil {
			return err
		}

		if orderType == "cost" && projectCode != nil {
			if err := addToProjectStock(ctx, tx, *projectCode, l.MatCode, unit, qtyReserved); err != nil {
				return err
			}
		}
	}

	return nil
}

// Reopen godoc
// @Summary      Reopen a COMPLETED PR back to DRAFT for editing
// @Description  Only allowed when status='COMPLETED'. Blocks with 400 if any PO derived from this
// @Description  PR's lines (via purchase_order_line.pr_line_id) is not yet "closed" — closed means
// @Description  status='CANCELLED' OR status_receive='RECEIVED' (a fully-received PO has already
// @Description  served its purpose and no longer needs to block PR edits). The system does not
// @Description  auto-cancel/auto-close anything; the user must get the referencing PO(s) to one of
// @Description  those states manually first. Reverses any stock
// @Description  deducted by deductStockOnSubmit (stock_transaction rows with ref_doc_type='PR',
// @Description  ref_doc_id=id, txn_type='OUT') by restoring stock_item.qty and recording an
// @Description  offsetting 'IN' transaction, then sets status back to DRAFT. Relies on the
// @Description  confirmed rule that a PR never has duplicate mat_code across its own lines, so
// @Description  reversing by ref_doc_id=pr_id alone (without a pr_line_id column on
// @Description  stock_transaction) is unambiguous.
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PR ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /pr/{id}/reopen [put]
func (h *PRHandler) Reopen(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PR id")
	}
	var userID int64
	if claims != nil {
		userID = claims.UserID
	}
	ctx := context.Background()

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var currentStatus, prNo, orderType string
	var projectCode *string
	if err := tx.QueryRow(ctx,
		`SELECT status, pr_no, order_type, project_code FROM purchase_request WHERE id=$1 FOR UPDATE`, id,
	).Scan(&currentStatus, &prNo, &orderType, &projectCode); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}
	if currentStatus != "COMPLETED" {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("cannot reopen PR in status: %s", currentStatus))
	}

	blockRows, err := tx.Query(ctx, `
		SELECT DISTINCT po.po_no
		FROM purchase_order_line pol
		JOIN purchase_order po ON po.id = pol.po_id
		JOIN purchase_request_line prl ON prl.id = pol.pr_line_id
		WHERE prl.pr_id = $1 AND po.status != 'CANCELLED' AND po.status_receive != 'RECEIVED'
		ORDER BY po.po_no`, id)
	if err != nil {
		return err
	}
	var blockingPONos []string
	for blockRows.Next() {
		var poNo string
		if err := blockRows.Scan(&poNo); err != nil {
			blockRows.Close()
			return err
		}
		blockingPONos = append(blockingPONos, poNo)
	}
	blockRows.Close()
	if len(blockingPONos) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf(
			"กรุณายกเลิก PO ต่อไปนี้ก่อนแก้ไข PR: %s", strings.Join(blockingPONos, ", ")))
	}

	// FOR UPDATE here is belt-and-suspenders: the purchase_request row lock acquired above
	// already serializes concurrent Reopen calls for this PR (a second Reopen blocks until
	// the first commits, then re-reads status='DRAFT' and is rejected), so this select can't
	// actually race in practice. Locking these rows too costs nothing and guards against any
	// future code path that reverses stock_transaction rows without first locking the PR.
	issueRows, err := tx.Query(ctx, `
		SELECT st.id, st.item_id, st.qty, si.mat_code
		FROM stock_transaction st
		JOIN stock_item si ON si.id = st.item_id
		WHERE st.ref_doc_type = 'PR' AND st.ref_doc_id = $1 AND st.txn_type = $2 AND st.reversed_at IS NULL
		FOR UPDATE OF st`, id, "OUT")
	if err != nil {
		return err
	}
	type issuedQty struct {
		TxnID   int64
		ItemID  int64
		Qty     float64
		MatCode string
	}
	var issued []issuedQty
	for issueRows.Next() {
		var q issuedQty
		if err := issueRows.Scan(&q.TxnID, &q.ItemID, &q.Qty, &q.MatCode); err != nil {
			issueRows.Close()
			return err
		}
		issued = append(issued, q)
	}
	issueRows.Close()

	for _, q := range issued {
		if _, err := tx.Exec(ctx,
			`UPDATE stock_item SET qty = qty + $1, updated_at = NOW() WHERE id = $2`,
			q.Qty, q.ItemID,
		); err != nil {
			return err
		}

		if orderType == "cost" && projectCode != nil {
			if err := deductProjectStock(ctx, tx, *projectCode, q.MatCode, q.Qty); err != nil {
				if fe, ok := err.(*fiber.Error); ok {
					return fiber.NewError(fiber.StatusConflict,
						"ไม่สามารถแก้ไขได้ เนื่องจากโครงการใช้วัสดุนี้ไปแล้ว กรุณาติดต่อผู้ดูแลระบบ ("+fe.Message+")")
				}
				return err
			}
		}

		txnNo, err := generateTxnNo(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_transaction
			    (txn_no, txn_type, item_id, qty, ref_doc_type, ref_doc_id, remarks, txn_date, created_by)
			VALUES ($1,$2,$3,$4,'PR',$5,'คืน stock จากการ reopen PR',CURRENT_DATE,$6)`,
			// 'IN', not TxnTypeReturn ("RETURN") — same constraint mismatch as the
			// submit-side 'OUT' fix above; mirrors it as the reversal direction.
			txnNo, "IN", q.ItemID, q.Qty, id, userID,
		); err != nil {
			return err
		}

		// Mark the OUT row consumed so a future Reopen (after another submit→reopen cycle)
		// doesn't re-reverse it — this was the root cause of the double/triple-reversal bug.
		if _, err := tx.Exec(ctx,
			`UPDATE stock_transaction SET reversed_at = NOW() WHERE id = $1`,
			q.TxnID,
		); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE purchase_request_line SET qty_reserved = 0, qty_to_order = qty_requested
		 WHERE pr_id = $1`, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE purchase_request SET status='DRAFT', updated_at=NOW() WHERE id=$1`, id,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO pr_status_log (pr_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'DRAFT',$3,'reopened for editing')`, id, currentStatus, userID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO erp_audit_log (table_name, record_id, action, changed_by, new_data)
		VALUES ('purchase_request',$1,'UPDATE',$2,$3)`,
		id, userID, fmt.Sprintf(`{"pr_no":"%s","status":"DRAFT","action":"reopen"}`, prNo),
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "PR reopened for editing"})
}

// Update godoc
// @Summary      Edit a DRAFT PR's header and lines
// @Description  Only allowed while status='DRAFT' (including PRs freshly reopened via
// @Description  PUT /pr/{id}/reopen, which itself guarantees no active PO references any of
// @Description  this PR's lines — see Reopen). Because that guard already holds by the time a
// @Description  PR can reach DRAFT, editing here is fully free: lines are deleted and
// @Description  reinserted from the payload with no FK-safety restrictions. Logs the edit to
// @Description  erp_audit_log. Status stays DRAFT afterward — the user must explicitly call
// @Description  POST /pr/{id}/submit to re-run deductStockOnSubmit against the new
// @Description  qty_requested values and go back to COMPLETED.
// @Description  job_code: omit to keep the PR's current value; job_code must be one of the 12 fixed job type codes if sent.
// @Description  lines[].deduct_stock (optional, default true): whether Submit should reserve this line against stock_item.qty.
// @Tags         Purchase Request
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                    true  "PR ID"
// @Param        body  body  models.UpdatePRRequest true  "Updated PR data"
// @Success      200   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /pr/{id} [put]
func (h *PRHandler) Update(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	prID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PR id")
	}

	var req models.UpdatePRRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line required")
	}
	for i, l := range req.Lines {
		if l.MatCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: mat_code is required", i))
		}
		if l.QtyRequested <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: qty_requested must be > 0", i))
		}
	}
	{
		matCodes := make([]string, len(req.Lines))
		for i, l := range req.Lines {
			matCodes[i] = l.MatCode
		}
		if err := validateMatCodesExist(context.Background(), h.db, matCodes); err != nil {
			return err
		}
	}
	for _, l := range req.Lines {
		if l.CostSubgroupID == nil {
			continue
		}
		var exists bool
		if err := h.db.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM cost_subgroup WHERE id = $1)`, *l.CostSubgroupID,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("cost_subgroup_id %d not found for mat_code %s", *l.CostSubgroupID, l.MatCode))
		}
	}

	ctx := context.Background()

	if req.ProjectCode != nil && strings.TrimSpace(*req.ProjectCode) != "" {
		var exists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code=$1)`, *req.ProjectCode).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, "invalid project_code")
		}
	}

	var currentStatus, prNo, currentOrderType, currentPRType, currentJobCode string
	if err := h.db.QueryRow(ctx, `SELECT status, pr_no, order_type, pr_type, job_code FROM purchase_request WHERE id=$1`, prID).Scan(&currentStatus, &prNo, &currentOrderType, &currentPRType, &currentJobCode); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}
	if currentStatus != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("PR must be DRAFT to edit (current status: %s)", currentStatus))
	}

	orderType := req.OrderType
	if orderType == "" {
		orderType = currentOrderType
	}
	if orderType != "stock" && orderType != "cost" {
		return fiber.NewError(fiber.StatusBadRequest, "order_type must be 'stock' or 'cost'")
	}

	prType := req.PRType
	if prType == "" {
		prType = currentPRType
	}
	if prType != "PO_WO" && prType != "PO_ONLY" && prType != "WO_ONLY" {
		return fiber.NewError(fiber.StatusBadRequest, "pr_type must be 'PO_WO', 'PO_ONLY', or 'WO_ONLY'")
	}

	jobCode := req.JobCode
	if jobCode == "" {
		jobCode = currentJobCode
	}
	if err := ValidateJobCode(jobCode); err != nil {
		return err
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE purchase_request SET
		    pr_date=$1, requested_by=$2, location_text=$3, warehouse_code=$4,
		    required_date=$5, project_code=$6, dept_code=$7, order_type=$8, pr_type=$9, job_code=$10, remarks=$11, updated_at=NOW(), updated_by=$12
		WHERE id=$13`,
		req.PRDate, req.RequestedBy, req.LocationText, req.WarehouseCode,
		req.RequiredDate, req.ProjectCode, req.DeptCode, orderType, prType, jobCode, req.Remarks, claims.UserID, prID,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update PR: "+err.Error())
	}

	// No active PO can reference any of this PR's lines at this point (guaranteed by the
	// Reopen guard), so lines can simply be wiped and reinserted from the payload.
	if _, err := tx.Exec(ctx, `DELETE FROM purchase_request_line WHERE pr_id=$1`, prID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to clear lines: "+err.Error())
	}

	for i, line := range req.Lines {
		lineNo := line.LineNo
		if lineNo == 0 {
			lineNo = i + 1
		}
		deductStock := true
		if line.DeductStock != nil {
			deductStock = *line.DeductStock
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_request_line (pr_id, line_no, mat_code, qty_requested, status, cost_subgroup_id, deduct_stock)
			VALUES ($1,$2,$3,$4,'OPEN',$5,$6)`,
			prID, lineNo, line.MatCode, line.QtyRequested, line.CostSubgroupID, deductStock,
		); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: invalid mat_code", i))
			}
			return fiber.NewError(fiber.StatusInternalServerError, "line insert error: "+err.Error())
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO erp_audit_log (table_name, record_id, action, changed_by, new_data)
		VALUES ('purchase_request',$1,'UPDATE',$2,$3)`,
		prID, claims.UserID, fmt.Sprintf(`{"pr_no":"%s","action":"edit","status":"DRAFT"}`, prNo),
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "audit log error: "+err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "PR updated",
		"data":    fiber.Map{"pr_id": prID, "status": "DRAFT"},
	})
}

// GetPRLogs godoc
// @Summary      Get PR status history
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PR ID"
// @Success      200  {array}  fiber.Map
// @Router       /pr/{id}/logs [get]
func (h *PRHandler) GetLogs(c *fiber.Ctx) error {
	id := c.Params("id")
	rows, err := h.db.Query(context.Background(), `
		SELECT l.id, l.from_status, l.to_status, u.full_name, l.changed_at, l.remarks
		FROM pr_status_log l JOIN users u ON u.id = l.changed_by
		WHERE l.pr_id=$1 ORDER BY l.changed_at`, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	type LogRow struct {
		LogID      int64     `json:"log_id"`
		FromStatus *string   `json:"from_status"`
		ToStatus   string    `json:"to_status"`
		ChangedBy  string    `json:"changed_by"`
		ChangedAt  time.Time `json:"changed_at"`
		Remarks    *string   `json:"remarks"`
	}
	var logs []LogRow
	for rows.Next() {
		var l LogRow
		rows.Scan(&l.LogID, &l.FromStatus, &l.ToStatus, &l.ChangedBy, &l.ChangedAt, &l.Remarks)
		logs = append(logs, l)
	}
	return c.JSON(fiber.Map{"success": true, "data": logs})
}

// NextNumber godoc
// @Summary      Get next PR number
// @Description  Returns the next available PR number for the current month: PR<YYYYMM>-<4-digit sequence>, e.g. PR202608-0001. Sequence resets to 0001 each new month.
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /pr/next-number [get]
func (h *PRHandler) NextNumber(c *fiber.Ctx) error {
	now := time.Now()
	prefix := fmt.Sprintf("PR%04d%02d", now.Year(), int(now.Month()))
	pattern := prefix + "-%"

	var lastNo string
	err := h.db.QueryRow(context.Background(), `
		SELECT pr_no FROM purchase_request
		WHERE pr_no LIKE $1
		  AND status NOT IN ('CANCELLED')
		ORDER BY pr_no DESC
		LIMIT 1`, pattern).Scan(&lastNo)

	var next string
	if err != nil {
		// No existing row this month → start at 0001
		next = fmt.Sprintf("%s-0001", prefix)
	} else {
		parts := strings.Split(lastNo, "-")
		seq, _ := strconv.Atoi(parts[len(parts)-1])
		next = fmt.Sprintf("%s-%04d", prefix, seq+1)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"next_number": next},
	})
}

// LinesWithPOStatus godoc
// @Summary      Get PR lines enriched with PO claim status
// @Description  Returns purchase_request_line rows for the PR with the quantity already claimed by purchase orders, which PO numbers claimed each line, and qty_remaining = qty_requested - qty_ordered. Pass exclude_po_id when editing an existing PO so its own claim is left out of referenced_pos. Each line includes spec_name (nullable) — the material spec, same field name/join as GET /pr/{id}.
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        id             path   int  true   "PR ID"
// @Param        exclude_po_id  query  int  false  "Exclude this PO's lines from referenced_pos (edit-PO flow)"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /pr/{id}/lines-with-po-status [get]
func (h *PRHandler) LinesWithPOStatus(c *fiber.Ctx) error {
	prID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid PR id")
	}

	var excludePOID *int64
	if v := c.Query("exclude_po_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid exclude_po_id")
		}
		excludePOID = &id
	}

	ctx := context.Background()

	var prNo, prStatus string
	if err := h.db.QueryRow(ctx, `SELECT pr_no, status FROM purchase_request WHERE id=$1`, prID).Scan(&prNo, &prStatus); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("PR %d not found", prID))
	}
	if prStatus == "CANCELLED" {
		return fiber.NewError(fiber.StatusBadRequest, "PR is CANCELLED, cannot view line/PO status")
	}

	rows, err := h.db.Query(ctx, `
    SELECT
        base.id, base.line_no, base.mat_code, base.mat_name, base.unit_name, base.spec_name,
        base.qty_requested, base.qty_reserved, base.qty_ordered, base.qty_remaining,
        base.status, base.referenced_pos,
        ph.last_price,
        ph.last_price_date,
        COALESCE(ph.price_history, '[]'::json) AS price_history,
        base.cost_subgroup_id, base.cost_code, base.cost_subgroup_name,
        base.job_code, base.job_name
    FROM (
        SELECT
            prl.id, prl.line_no, prl.mat_code, mn.mat_name, u.unit_name,
            ss.spec_description AS spec_name,
            prl.qty_requested, prl.qty_reserved,
            -- Both qty_ordered and qty_remaining are derived from the live referenced_pos
            -- sum below, not the cached prl.qty_ordered column, so neither can disagree
            -- with referenced_pos or with any other endpoint (e.g. the PO-creation PR list
            -- filter) that reads the same live data.
            COALESCE(SUM(pol.qty_ordered) FILTER (WHERE pol.id IS NOT NULL), 0) AS qty_ordered,
            (prl.qty_requested - COALESCE(SUM(pol.qty_ordered) FILTER (WHERE pol.id IS NOT NULL), 0)) AS qty_remaining,
            prl.status,
            COALESCE(
                JSON_AGG(
                    JSON_BUILD_OBJECT('po_id', po.id, 'po_no', po.po_no, 'qty', pol.qty_ordered)
                ) FILTER (WHERE pol.id IS NOT NULL),
                '[]'
            ) AS referenced_pos,
            prl.cost_subgroup_id,
            csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code AS cost_code,
            csg.subgroup_name AS cost_subgroup_name,
            cj.job_code AS job_code,
            cj.job_name AS job_name
        FROM purchase_request_line prl
        JOIN material_code mc ON mc.mat_code = prl.mat_code
        JOIN mat_name mn       ON mn.id = mc.mat_name_id
        JOIN unit u            ON u.id = mc.unit_id
        LEFT JOIN spec_size ss ON ss.id = mc.spec_id
        LEFT JOIN purchase_order_line pol
               ON pol.pr_line_id = prl.id
              AND pol.status != 'CANCELLED'
              AND ($2::bigint IS NULL OR pol.po_id != $2)
        LEFT JOIN purchase_order po ON po.id = pol.po_id
        LEFT JOIN cost_subgroup csg  ON csg.id = prl.cost_subgroup_id
        LEFT JOIN cost_group    cg   ON cg.id = csg.group_id
        LEFT JOIN cost_job      cj   ON cj.id = cg.job_id
        LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id
        WHERE prl.pr_id = $1
        GROUP BY prl.id, prl.line_no, prl.mat_code, mn.mat_name, u.unit_name, ss.spec_description,
                 prl.qty_requested, prl.qty_reserved, prl.qty_ordered, prl.status,
                 prl.cost_subgroup_id, csub.subject_code, cj.job_code, cj.job_name, cg.group_code,
                 csg.subgroup_code, csg.subgroup_name
    ) base
    LEFT JOIN LATERAL (
        SELECT
		
            (ARRAY_AGG(hist.unit_price ORDER BY hist.po_date DESC))[1] AS last_price,
            (ARRAY_AGG(hist.po_date    ORDER BY hist.po_date DESC))[1] AS last_price_date,
            JSON_AGG(
                JSON_BUILD_OBJECT(
                    'price',         hist.unit_price,
                    'date',          hist.po_date,
                    'qty',           hist.qty_ordered,
                    'supplier_name', hist.supplier_name,
                    'po_no',         hist.po_no
                ) ORDER BY hist.po_date DESC
            ) AS price_history
        FROM (
            SELECT
                pol.unit_price,
                po.po_date,
                pol.qty_ordered,
                s.supplier_name,
                po.po_no
            FROM purchase_order_line pol
            JOIN purchase_order po ON po.id = pol.po_id
            LEFT JOIN supplier s   ON s.id = po.supplier_id
            WHERE pol.mat_code = base.mat_code
              AND pol.status != 'CANCELLED'
              AND po.status   IN ('APPROVED','SENT','PARTIALLY_RECEIVED','RECEIVED')
              AND pol.unit_price > 0
            ORDER BY po.po_date DESC
            LIMIT 10
        ) hist
    ) ph ON TRUE
    ORDER BY base.line_no`, prID, excludePOID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "query error: "+err.Error())
	}
	defer rows.Close()

	lines := make([]models.PRLineWithPOStatus, 0)
	for rows.Next() {
		var l models.PRLineWithPOStatus
		var refJSON []byte
		var lastPrice *float64
		var lastPriceDate *time.Time
		var priceHistJSON []byte
		if err := rows.Scan(&l.PRLineID, &l.LineNo, &l.MatCode, &l.MatName, &l.Unit, &l.SpecName,
			&l.QtyRequested, &l.QtyReserved, &l.QtyOrdered, &l.QtyRemaining, &l.LineStatus, &refJSON,
			&lastPrice, &lastPriceDate, &priceHistJSON,
			&l.CostSubgroupID, &l.CostCode, &l.CostSubgroupName,
			&l.JobCode, &l.JobName); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "scan error: "+err.Error())
		}
		l.ReferencedPOs = []models.ReferencedPO{}
		if err := json.Unmarshal(refJSON, &l.ReferencedPOs); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "decode error: "+err.Error())
		}
		l.LastPrice = lastPrice
		if lastPriceDate != nil {
			s := lastPriceDate.Format("2006-01-02")
			l.LastPriceDate = &s
		}
		l.PriceHistory = []models.PriceHistoryItem{}
		_ = json.Unmarshal(priceHistJSON, &l.PriceHistory)
		lines = append(lines, l)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PRLinesWithPOStatusResponse{
			PRNo:     prNo,
			PRStatus: prStatus,
			Lines:    lines,
		},
	})
}

func (h *PRHandler) changeStatus(c *fiber.Ctx, id, from, to, remarks string, userID int64, prNo string) error {
	tx, _ := h.db.Begin(context.Background())
	defer tx.Rollback(context.Background())

	tx.Exec(context.Background(), `UPDATE purchase_request SET status=$1, updated_at=NOW() WHERE id=$2`, to, id)
	tx.Exec(context.Background(), `
		INSERT INTO pr_status_log (pr_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,$3,$4,$5)`, id, from, to, userID, remarks)

	// Create approval_log entry
	tx.Exec(context.Background(), `
		INSERT INTO erp_audit_log (table_name, record_id, action, changed_by, new_data)
		VALUES ('purchase_request',$1,'UPDATE',$2,$3)`,
		id, userID, fmt.Sprintf(`{"pr_no":"%s","status":"%s"}`, prNo, to))

	tx.Commit(context.Background())
	return c.JSON(fiber.Map{"success": true, "message": fmt.Sprintf("PR status changed to %s", to)})
}

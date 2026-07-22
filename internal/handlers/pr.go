package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PRHandler struct {
	db *pgxpool.Pool
}

func NewPRHandler(db *pgxpool.Pool) *PRHandler {
	return &PRHandler{db: db}
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

// GetPR godoc
// @Summary      Get purchase request by ID
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PR ID"
// @Success      200  {object}  models.PurchaseRequest
// @Failure      404  {object}  fiber.Map
// @Router       /pr/{id} [get]
func (h *PRHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	row := h.db.QueryRow(context.Background(), `
		SELECT id, pr_no, pr_date, requested_by, location_text, warehouse_code,
		       required_date::text, status, priority, remarks, created_at, updated_at
		FROM purchase_request WHERE id=$1`, id)

	var pr models.PurchaseRequest
	if err := row.Scan(&pr.PRID, &pr.PRNo, &pr.PRDate, &pr.RequestedBy, &pr.LocationText,
		&pr.WarehouseCode, &pr.RequiredDate, &pr.Status, &pr.Priority, &pr.Remarks, &pr.CreatedAt, &pr.UpdatedAt); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}

	rows, _ := h.db.Query(context.Background(), `
		SELECT prl.id, prl.pr_id, prl.line_no, prl.mat_code, prl.qty_requested, prl.qty_reserved, prl.qty_to_order, prl.remarks, prl.status,
		       prl.cost_subgroup_id,
		       csub.subject_code || cj.job_code || cg.group_code || csg.subgroup_code AS cost_code,
		       csg.subgroup_name
		FROM purchase_request_line prl
		LEFT JOIN cost_subgroup csg ON csg.id = prl.cost_subgroup_id
		LEFT JOIN cost_group cg ON cg.id = csg.group_id
		LEFT JOIN cost_job cj ON cj.id = cg.job_id
		LEFT JOIN cost_subject csub ON csub.id = cj.subject_id
		WHERE prl.pr_id=$1 ORDER BY prl.line_no`, id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var l models.PRLine
			rows.Scan(&l.LineID, &l.PRID, &l.LineNo, &l.MatCode,
				&l.QtyRequested, &l.QtyReserved, &l.QtyToOrder, &l.Remarks, &l.Status,
				&l.CostSubgroupID, &l.CostCode, &l.CostSubgroupName)
			pr.Lines = append(pr.Lines, l)
		}
	}

	return c.JSON(fiber.Map{"success": true, "data": pr})
}

// CreatePR godoc
// @Summary      Create purchase request
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
		     project_code, status, remarks, created_at, updated_at, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now(),now(),$10,$10)
		RETURNING id`,
		req.PRNo, req.PRDate, req.RequestedBy, req.LocationText, req.WarehouseCode,
		req.RequiredDate, req.ProjectCode, req.Status, req.Remarks, req.CreatedBy,
	).Scan(&prID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create PR: "+err.Error())
	}

	// 2. Insert lines
	for _, line := range req.Lines {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO purchase_request_line (pr_id, line_no, mat_code, qty_requested, status, cost_subgroup_id)
			VALUES ($1,$2,$3,$4,'OPEN',$5)`,
			prID, line.LineNo, line.MatCode, line.QtyRequested, line.CostSubgroupID,
		); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to insert line: "+err.Error())
		}
	}

	// 3. Insert attachments (if any)
	for _, att := range req.Attachments {
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
	if err := h.db.QueryRow(ctx,
		`SELECT status, pr_no FROM purchase_request WHERE id=$1`, id,
	).Scan(&currentStatus, &prNo); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}
	if currentStatus != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("cannot submit PR in status: %s", currentStatus))
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	shortages, err := h.deductStockOnSubmit(ctx, tx, id, claims.UserID)
	if err != nil {
		return err
	}
	if len(shortages) > 0 {
		var msgs []string
		for _, s := range shortages {
			msgs = append(msgs, fmt.Sprintf(
				"สต็อกไม่พอสำหรับ %s ต้องการ %g มีในสต็อก %g กรุณาแก้ไขจำนวนสั่งซื้อ",
				s.MatCode, s.Requested, s.Available))
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   strings.Join(msgs, "; "),
			"data":    fiber.Map{"shortages": shortages},
		})
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

// stockShortage is one PR line whose stock_item.qty on-hand is insufficient to cover
// the shortfall (qty_requested - qty_to_order) that submission would otherwise deduct.
type stockShortage struct {
	MatCode   string  `json:"mat_code"`
	Requested float64 `json:"requested"` // the shortfall qty that would have been deducted
	Available float64 `json:"available"` // stock_item.qty currently on hand
}

// deductStockOnSubmit implements the "cover the shortfall from existing stock" rule for
// PR submission: for every line where qty_requested > qty_to_order, the difference
// (shortfall) is assumed to come out of stock_item.qty rather than being purchased.
//
// If ANY line's shortfall exceeds stock_item.qty, the whole submission is blocked —
// every affected line is returned so the caller can report them all at once, not just
// the first. Nothing is written in that case. Otherwise every shortfall line's
// stock_item.qty is decremented and a stock_transaction (TxnTypeIssue) is recorded,
// all inside the caller's tx so a failure anywhere rolls back PR completion too.
func (h *PRHandler) deductStockOnSubmit(ctx context.Context, tx pgx.Tx, prID int64, userID int64) ([]stockShortage, error) {
	rows, err := tx.Query(ctx, `
		SELECT mat_code, (qty_requested - qty_to_order) AS shortfall
		FROM purchase_request_line
		WHERE pr_id = $1 AND (qty_requested - qty_to_order) > 0`, prID)
	if err != nil {
		return nil, err
	}
	type line struct {
		MatCode   string
		Shortfall float64
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.MatCode, &l.Shortfall); err != nil {
			rows.Close()
			return nil, err
		}
		lines = append(lines, l)
	}
	rows.Close()

	if len(lines) == 0 {
		return nil, nil
	}

	var shortages []stockShortage
	itemIDs := make(map[string]int64, len(lines))
	for _, l := range lines {
		var itemID int64
		var available float64
		if err := tx.QueryRow(ctx,
			`SELECT id, qty FROM stock_item WHERE mat_code = $1`, l.MatCode,
		).Scan(&itemID, &available); err != nil {
			shortages = append(shortages, stockShortage{MatCode: l.MatCode, Requested: l.Shortfall, Available: 0})
			continue
		}
		if available < l.Shortfall {
			shortages = append(shortages, stockShortage{MatCode: l.MatCode, Requested: l.Shortfall, Available: available})
			continue
		}
		itemIDs[l.MatCode] = itemID
	}

	// Block the entire submission — list every affected line, write nothing.
	if len(shortages) > 0 {
		return shortages, nil
	}

	for _, l := range lines {
		itemID := itemIDs[l.MatCode]

		if _, err := tx.Exec(ctx,
			`UPDATE stock_item SET qty = qty - $1, updated_at = NOW() WHERE id = $2`,
			l.Shortfall, itemID,
		); err != nil {
			return nil, err
		}

		txnNo, err := generateTxnNo(ctx, tx)
		if err != nil {
			return nil, err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_transaction
			    (txn_no, txn_type, item_id, qty, ref_doc_type, ref_doc_id, remarks, txn_date, created_by)
			VALUES ($1,$2,$3,$4,'PR',$5,'ตัด stock อัตโนมัติจากการ submit PR',CURRENT_DATE,$6)`,
			txnNo, TxnTypeIssue, itemID, l.Shortfall, prID, userID,
		); err != nil {
			return nil, err
		}
	}

	return nil, nil
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
// @Description  Returns the next available PR number for the current month (Buddhist year format)
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /pr/next-number [get]
func (h *PRHandler) NextNumber(c *fiber.Ctx) error {
	now := time.Now()
	gregorianYear := now.Year() % 100
	prefix := fmt.Sprintf("PR%02d%02d", gregorianYear, int(now.Month()))
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
		// No existing row → start at 001
		next = fmt.Sprintf("%s-001", prefix)
	} else {
		parts := strings.Split(lastNo, "-")
		seq, _ := strconv.Atoi(parts[len(parts)-1])
		next = fmt.Sprintf("%s-%03d", prefix, seq+1)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"next_number": next},
	})
}

// LinesWithPOStatus godoc
// @Summary      Get PR lines enriched with PO claim status
// @Description  Returns purchase_request_line rows for the PR with the quantity already claimed by purchase orders, which PO numbers claimed each line, and qty_remaining = qty_requested - qty_ordered. Pass exclude_po_id when editing an existing PO so its own claim is left out of referenced_pos.
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
	if prStatus != "COMPLETED" && prStatus != "PARTIALLY_FILLED" {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("PR status must be COMPLETED or PARTIALLY_FILLED, got %s", prStatus))
	}

	rows, err := h.db.Query(ctx, `
    SELECT
        base.id, base.line_no, base.mat_code, base.mat_name, base.unit_name,
        base.qty_requested, base.qty_reserved, base.qty_ordered, base.qty_remaining,
        base.status, base.referenced_pos,
        ph.last_price,
        ph.last_price_date,
        COALESCE(ph.price_history, '[]'::json) AS price_history
    FROM (
        SELECT
            prl.id, prl.line_no, prl.mat_code, mn.mat_name, u.unit_name,
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
            ) AS referenced_pos
        FROM purchase_request_line prl
        JOIN material_code mc ON mc.mat_code = prl.mat_code
        JOIN mat_name mn       ON mn.id = mc.mat_name_id
        JOIN unit u            ON u.id = mc.unit_id
        LEFT JOIN purchase_order_line pol
               ON pol.pr_line_id = prl.id
              AND pol.status != 'CANCELLED'
              AND ($2::bigint IS NULL OR pol.po_id != $2)
        LEFT JOIN purchase_order po ON po.id = pol.po_id
        WHERE prl.pr_id = $1
        GROUP BY prl.id, prl.line_no, prl.mat_code, mn.mat_name, u.unit_name,
                 prl.qty_requested, prl.qty_reserved, prl.qty_ordered, prl.status
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
            JOIN supplier s        ON s.supplier_code = po.supplier_code
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
		if err := rows.Scan(&l.PRLineID, &l.LineNo, &l.MatCode, &l.MatName, &l.Unit,
			&l.QtyRequested, &l.QtyReserved, &l.QtyOrdered, &l.QtyRemaining, &l.LineStatus, &refJSON,
			&lastPrice, &lastPriceDate, &priceHistJSON); err != nil {
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

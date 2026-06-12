package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"erp-api/internal/middleware"
	"erp-api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PRHandler struct {
	db *pgxpool.Pool
}

func NewPRHandler(db *pgxpool.Pool) *PRHandler {
	return &PRHandler{db: db}
}

// ListPR godoc
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
		SELECT pr_id, pr_no, pr_date, requested_by, location_code, warehouse_code,
		       required_date::text, status, priority, remarks, created_at, updated_at
		FROM purchase_request WHERE pr_id=$1`, id)

	var pr models.PurchaseRequest
	if err := row.Scan(&pr.PRID, &pr.PRNo, &pr.PRDate, &pr.RequestedBy, &pr.LocationCode,
		&pr.WarehouseCode, &pr.RequiredDate, &pr.Status, &pr.Priority, &pr.Remarks, &pr.CreatedAt, &pr.UpdatedAt); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}

	rows, _ := h.db.Query(context.Background(), `
		SELECT line_id, pr_id, line_no, mat_code, qty_requested, qty_reserved, qty_to_order, remarks, status
		FROM purchase_request_line WHERE pr_id=$1 ORDER BY line_no`, id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var l models.PRLine
			rows.Scan(&l.LineID, &l.PRID, &l.LineNo, &l.MatCode,
				&l.QtyRequested, &l.QtyReserved, &l.QtyToOrder, &l.Remarks, &l.Status)
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
	if req.PRNo == "" || req.LocationCode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "pr_no and location_code are required")
	}
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one line required")
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
		    (pr_no, pr_date, requested_by, location_code, required_date,
		     project_code, status, remarks, created_at, updated_at, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now(),now(),$9,$9)
		RETURNING id`,
		req.PRNo, req.PRDate, req.RequestedBy, req.LocationCode,
		req.RequiredDate, req.ProjectCode, req.Status, req.Remarks, req.CreatedBy,
	).Scan(&prID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create PR: "+err.Error())
	}

	// 2. Insert lines
	for _, line := range req.Lines {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO purchase_request_line (pr_id, line_no, mat_code, qty_requested, status)
			VALUES ($1,$2,$3,$4,'OPEN')`,
			prID, line.LineNo, line.MatCode, line.QtyRequested,
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
// @Summary      Submit PR for approval
// @Tags         Purchase Request
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PR ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /pr/{id}/submit [post]
func (h *PRHandler) Submit(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	id := c.Params("id")

	var currentStatus string
	var prNo string
	err := h.db.QueryRow(context.Background(), `SELECT status, pr_no FROM purchase_request WHERE pr_id=$1`, id).Scan(&currentStatus, &prNo)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "PR not found")
	}
	if currentStatus != "DRAFT" && currentStatus != "REJECTED" {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("cannot submit PR in status: %s", currentStatus))
	}

	return h.changeStatus(c, id, currentStatus, "PENDING_APPROVAL", "submitted for approval", claims.UserID, prNo)
}

// ApprovePR godoc
// @Summary      Approve or reject PR (Senior Team Project)
// @Tags         Purchase Request
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                          true  "PR ID"
// @Param        body  body  models.ApprovalActionRequest true  "action"
// @Success      200  {object}  fiber.Map
// @Router       /pr/{id}/approve [post]
func (h *PRHandler) Approve(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	id := c.Params("id")

	var req models.ApprovalActionRequest
	c.BodyParser(&req)

	var currentStatus, prNo string
	h.db.QueryRow(context.Background(), `SELECT status, pr_no FROM purchase_request WHERE pr_id=$1`, id).Scan(&currentStatus, &prNo)
	if currentStatus != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "PR is not pending approval")
	}

	newStatus := "APPROVED"
	if req.Action == "REJECT" {
		newStatus = "REJECTED"
	} else if req.Action == "RETURN" {
		newStatus = "DRAFT"
	}

	return h.changeStatus(c, id, currentStatus, newStatus, req.Action, claims.UserID, prNo)
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
		SELECT l.log_id, l.from_status, l.to_status, u.full_name, l.changed_at, l.remarks
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
	buddhistYear := (now.Year() + 543) % 100
	prefix := fmt.Sprintf("%02d%02d", buddhistYear, int(now.Month()))
	pattern := prefix + "-%"

	var lastNo string
	err := h.db.QueryRow(context.Background(), `
		SELECT pr_no FROM purchase_request
		WHERE pr_no LIKE $1
		  AND status NOT IN ('CANCELLED','REJECTED')
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

func (h *PRHandler) changeStatus(c *fiber.Ctx, id, from, to, remarks string, userID int64, prNo string) error {
	tx, _ := h.db.Begin(context.Background())
	defer tx.Rollback(context.Background())

	tx.Exec(context.Background(), `UPDATE purchase_request SET status=$1, updated_at=NOW() WHERE pr_id=$2`, to, id)
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

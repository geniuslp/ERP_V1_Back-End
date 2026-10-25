package handlers

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
)

// WOContractorLogHandler handles บันทึกงานของผู้รับเหมาตามใบสั่งจ้าง. Tab 1 (ข้อมูลเอกสาร) only:
// a pointer to a work order, the "PJ" project pick, and the typed Contract/Advance/Retention rows.
type WOContractorLogHandler struct{ db *pgxpool.Pool }

func NewWOContractorLogHandler(db *pgxpool.Pool) *WOContractorLogHandler {
	return &WOContractorLogHandler{db: db}
}

type woContractorLogTerm struct {
	ItemType   string   `json:"item_type"` // CONTRACT | ADVANCE | RETENTION
	Percent    *float64 `json:"percent"`
	Amount     *float64 `json:"amount"`
	Accumulate *float64 `json:"accumulate"`
	Balance    *float64 `json:"balance"`
}

type woContractorLogRequest struct {
	WoID        int64                 `json:"wo_id"`
	ProjectCode *string               `json:"project_code"`
	Terms       []woContractorLogTerm `json:"terms"`
}

func validWOContractorLogItemType(t string) bool {
	return t == "CONTRACT" || t == "ADVANCE" || t == "RETENTION"
}

func (h *WOContractorLogHandler) validate(ctx context.Context, req *woContractorLogRequest) error {
	if req.WoID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "wo_id is required")
	}
	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM work_order WHERE id=$1)`, req.WoID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusBadRequest, "work order not found")
	}
	if req.ProjectCode != nil {
		pc := strings.TrimSpace(*req.ProjectCode)
		if pc == "" {
			req.ProjectCode = nil
		} else {
			req.ProjectCode = &pc
			if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project WHERE project_code=$1)`, pc).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return fiber.NewError(fiber.StatusBadRequest, "project not found")
			}
		}
	}
	seen := map[string]bool{}
	for _, t := range req.Terms {
		if !validWOContractorLogItemType(t.ItemType) {
			return fiber.NewError(fiber.StatusBadRequest, "item_type must be CONTRACT, ADVANCE or RETENTION")
		}
		if seen[t.ItemType] {
			return fiber.NewError(fiber.StatusBadRequest, "duplicate item_type: "+t.ItemType)
		}
		seen[t.ItemType] = true
	}
	return nil
}

func insertWOContractorLogTerms(ctx context.Context, tx pgx.Tx, logID int64, terms []woContractorLogTerm) error {
	for _, t := range terms {
		if _, err := tx.Exec(ctx, `
			INSERT INTO wo_contractor_log_term (log_id, item_type, percent, amount, accumulate, balance)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			logID, t.ItemType, t.Percent, t.Amount, t.Accumulate, t.Balance,
		); err != nil {
			return err
		}
	}
	return nil
}

// Create godoc
// @Summary      สร้างบันทึกงานของผู้รับเหมาตามใบสั่งจ้าง (tab ข้อมูลเอกสาร)
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  woContractorLogRequest  true  "wo_id, project_code (PJ), terms"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Router       /wo-contractor-log [post]
func (h *WOContractorLogHandler) Create(c *fiber.Ctx) error {
	var req woContractorLogRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()
	if err := h.validate(ctx, &req); err != nil {
		return err
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO wo_contractor_log (wo_id, project_code, created_by, updated_by)
		VALUES ($1,$2,$3,$3) RETURNING id`, req.WoID, req.ProjectCode, claims.UserID,
	).Scan(&id); err != nil {
		log.Printf("wo contractor log insert: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลได้ กรุณาลองใหม่อีกครั้ง")
	}
	if err := insertWOContractorLogTerms(ctx, tx, id, req.Terms); err != nil {
		log.Printf("wo contractor log terms insert: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลได้ กรุณาลองใหม่อีกครั้ง")
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id}})
}

// Update godoc
// @Summary      แก้ไขบันทึกงานของผู้รับเหมาตามใบสั่งจ้าง (แทนที่ terms ทั้งหมด)
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                     true  "Log ID"
// @Param        body  body  woContractorLogRequest  true  "wo_id, project_code (PJ), terms"
// @Success      200   {object}  fiber.Map
// @Failure      404   {object}  fiber.Map
// @Router       /wo-contractor-log/{id} [put]
func (h *WOContractorLogHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req woContractorLogRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()
	if err := h.validate(ctx, &req); err != nil {
		return err
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE wo_contractor_log SET wo_id=$1, project_code=$2, updated_at=NOW(), updated_by=$3
		WHERE id=$4`, req.WoID, req.ProjectCode, claims.UserID, id)
	if err != nil {
		log.Printf("wo contractor log update: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลได้ กรุณาลองใหม่อีกครั้ง")
	}
	if tag.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusNotFound, "record not found")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM wo_contractor_log_term WHERE log_id=$1`, id); err != nil {
		return err
	}
	if err := insertWOContractorLogTerms(ctx, tx, id, req.Terms); err != nil {
		log.Printf("wo contractor log terms insert: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลได้ กรุณาลองใหม่อีกครั้ง")
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id}})
}

// Get godoc
// @Summary      รายละเอียดบันทึกงานของผู้รับเหมาตามใบสั่งจ้าง
// @Description  Supplier / WO project / contract description are live-read from work_order (never stored here).
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Log ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /wo-contractor-log/{id} [get]
func (h *WOContractorLogHandler) Get(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var (
		woID                                       int64
		projectCode                                *string
		woNo                                       string
		supplierName, woProjectCode, woProjectName *string
		woProjectNameText, contractDescription     *string
		contractAmount                             float64
		creditTermDays                             *int
	)
	// credit term is live-read from supplier.payment_terms (free text like "30  วัน") — first integer in it.
	err = h.db.QueryRow(ctx, `
		SELECT l.wo_id, l.project_code, wo.wo_no, wo.supplier_name, wo.project_code, p.project_name,
		       wo.project_name_text, wo.contract_description, wo.contract_amount,
		       NULLIF(substring(s.payment_terms from '[0-9]+'), '')::int
		FROM wo_contractor_log l
		JOIN work_order wo ON wo.id = l.wo_id
		LEFT JOIN project p ON p.project_code = wo.project_code
		LEFT JOIN supplier s ON s.id = wo.supplier_id
		WHERE l.id = $1`, id,
	).Scan(&woID, &projectCode, &woNo, &supplierName, &woProjectCode, &woProjectName,
		&woProjectNameText, &contractDescription, &contractAmount, &creditTermDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "record not found")
	}
	if err != nil {
		return err
	}

	rows, err := h.db.Query(ctx, `
		SELECT item_type, percent, amount, accumulate, balance
		FROM wo_contractor_log_term WHERE log_id = $1
		ORDER BY CASE item_type WHEN 'CONTRACT' THEN 1 WHEN 'ADVANCE' THEN 2 ELSE 3 END`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	terms := []woContractorLogTerm{}
	for rows.Next() {
		var t woContractorLogTerm
		if err := rows.Scan(&t.ItemType, &t.Percent, &t.Amount, &t.Accumulate, &t.Balance); err != nil {
			return err
		}
		terms = append(terms, t)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"id":                   id,
			"wo_id":                woID,
			"project_code":         projectCode,
			"wo_no":                woNo,
			"supplier_name":        supplierName,
			"wo_project_code":      woProjectCode,
			"wo_project_name":      woProjectName,
			"wo_project_name_text": woProjectNameText,
			"contract_description": contractDescription,
			"contract_amount":      contractAmount,
			"credit_term_days":     creditTermDays,
			"terms":                terms,
		},
	})
}

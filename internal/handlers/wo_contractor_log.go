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
	PoID        int64                 `json:"po_id"` // exactly one of wo_id / po_id / pc_id (the finance PO page uses po_id)
	PcID        int64                 `json:"pc_id"` // petty cash requisition (the finance petty cash page uses pc_id)
	ProjectCode *string               `json:"project_code"`
	Terms       []woContractorLogTerm `json:"terms"`
}

func validWOContractorLogItemType(t string) bool {
	return t == "CONTRACT" || t == "ADVANCE" || t == "RETENTION"
}

func (h *WOContractorLogHandler) validate(ctx context.Context, req *woContractorLogRequest) error {
	n := 0
	for _, v := range []int64{req.WoID, req.PoID, req.PcID} {
		if v > 0 {
			n++
		}
	}
	if n != 1 {
		return fiber.NewError(fiber.StatusBadRequest, "exactly one of wo_id, po_id or pc_id is required")
	}
	var exists bool
	if req.PcID > 0 {
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM petty_cash_requisition WHERE id=$1)`, req.PcID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, "petty cash requisition not found")
		}
	} else if req.PoID > 0 {
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM purchase_order WHERE id=$1)`, req.PoID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, "purchase order not found")
		}
	} else {
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM work_order WHERE id=$1)`, req.WoID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fiber.NewError(fiber.StatusBadRequest, "work order not found")
		}
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

// nullID maps 0 to SQL NULL for the wo_id / po_id / pc_id columns.
func nullID(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
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

	// One record per work order: the finance WO detail page opens it by WO, so a second one would be unreachable.
	var dup bool
	if err := h.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM wo_contractor_log WHERE wo_id IS NOT DISTINCT FROM $1 AND po_id IS NOT DISTINCT FROM $2 AND pc_id IS NOT DISTINCT FROM $3)`,
		nullID(req.WoID), nullID(req.PoID), nullID(req.PcID)).Scan(&dup); err != nil {
		return err
	}
	if dup {
		return fiber.NewError(fiber.StatusConflict, "เอกสารนี้มีบันทึกงานของผู้รับเหมาอยู่แล้ว")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO wo_contractor_log (wo_id, po_id, pc_id, project_code, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$5) RETURNING id`, nullID(req.WoID), nullID(req.PoID), nullID(req.PcID), req.ProjectCode, claims.UserID,
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
		UPDATE wo_contractor_log SET wo_id=$1, po_id=$2, pc_id=$3, project_code=$4, updated_at=NOW(), updated_by=$5
		WHERE id=$6`, nullID(req.WoID), nullID(req.PoID), nullID(req.PcID), req.ProjectCode, claims.UserID, id)
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

	var isPO, isPC bool
	if err := h.db.QueryRow(ctx, `SELECT po_id IS NOT NULL, pc_id IS NOT NULL FROM wo_contractor_log WHERE id = $1`, id).Scan(&isPO, &isPC); errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "record not found")
	} else if err != nil {
		return err
	}
	if isPO {
		return h.getForPO(c, id)
	}
	if isPC {
		return h.getForPC(c, id)
	}

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

// FindByWO godoc
// @Summary      หา id ของบันทึกงานผู้รับเหมาจากหนังสือสั่งจ้าง
// @Description  Returns data = {id} of the (latest) record for this work order, or data = null when none exists yet. The record is then read with GET /wo-contractor-log/{id}.
// @Tags         WOContractorLog
// @Security     BearerAuth
// @Produce      json
// @Param        woId  path  int  true  "Work Order ID"
// @Success      200   {object}  fiber.Map
// @Router       /wo-contractor-log/by-wo/{woId} [get]
func (h *WOContractorLogHandler) FindByWO(c *fiber.Ctx) error {
	woID, err := strconv.ParseInt(c.Params("woId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid woId")
	}
	var id int64
	err = h.db.QueryRow(context.Background(),
		`SELECT id FROM wo_contractor_log WHERE wo_id = $1 ORDER BY id DESC LIMIT 1`, woID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"success": true, "data": nil})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id}})
}

// getForPO is Get for a log that belongs to a purchase order. Same response shape as the WO one so the panel can
// share its code: wo_no carries the PO number, contract_amount is the PO value before VAT after discount
// (total_amount - discount_amount), and there is no advance / retention (those terms stay empty).
func (h *WOContractorLogHandler) getForPO(c *fiber.Ctx, id int64) error {
	ctx := context.Background()
	var (
		poID                                   int64
		projectCode                            *string
		poNo                                   string
		supplierName, poProjectCode, poProject *string
		contractAmount                         float64
		creditTermDays                         *int
	)
	err := h.db.QueryRow(ctx, `
		SELECT l.po_id, l.project_code, po.po_no, s.supplier_name, po.project_code, p.project_name,
		       (COALESCE(po.total_amount,0) - COALESCE(po.discount_amount,0))::float8,
		       NULLIF(substring(s.payment_terms from '[0-9]+'), '')::int
		FROM wo_contractor_log l
		JOIN purchase_order po ON po.id = l.po_id
		LEFT JOIN project p ON p.project_code = po.project_code
		LEFT JOIN supplier s ON s.id = po.supplier_id
		WHERE l.id = $1`, id,
	).Scan(&poID, &projectCode, &poNo, &supplierName, &poProjectCode, &poProject, &contractAmount, &creditTermDays)
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
			"id":               id,
			"po_id":            poID,
			"project_code":     projectCode,
			"wo_no":            poNo,
			"po_no":            poNo,
			"supplier_name":    supplierName,
			"wo_project_code":  poProjectCode,
			"wo_project_name":  poProject,
			"contract_amount":  contractAmount,
			"credit_term_days": creditTermDays,
			"terms":            terms,
		},
	})
}

// FindByPO is FindByWO for a purchase order: data = {id} of its log, or null when none exists yet.
// @Router       /wo-contractor-log/by-po/{poId} [get]
func (h *WOContractorLogHandler) FindByPO(c *fiber.Ctx) error {
	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid poId")
	}
	var id int64
	err = h.db.QueryRow(context.Background(),
		`SELECT id FROM wo_contractor_log WHERE po_id = $1 ORDER BY id DESC LIMIT 1`, poID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"success": true, "data": nil})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id}})
}

// getForPC is Get for a log that belongs to a petty cash requisition. Same response shape as the PO one so the panel
// shares its code: wo_no carries the PC number, supplier_name is the payee (requester), contract_amount is the value
// before VAT after discount (total_amount - discount_amount), no credit term (petty cash has no supplier).
func (h *WOContractorLogHandler) getForPC(c *fiber.Ctx, id int64) error {
	ctx := context.Background()
	var (
		pcID                                int64
		projectCode                         *string
		pcNo                                string
		payeeName, pcProjectCode, pcProject *string
		contractAmount                      float64
	)
	// Header project is optional on petty cash (it lives on the lines): fall back to the first line's project.
	err := h.db.QueryRow(ctx, `
		SELECT l.pc_id, l.project_code, pc.pc_no,
		       COALESCE(NULLIF(pc.payee_name, ''), u.full_name),
		       COALESCE(pc.project_code, fl.project_code), COALESCE(p.project_name, pc.project_name),
		       (COALESCE(pc.total_amount,0) - COALESCE(pc.discount_amount,0))::float8
		FROM wo_contractor_log l
		JOIN petty_cash_requisition pc ON pc.id = l.pc_id
		LEFT JOIN users u ON u.id = pc.requested_by
		LEFT JOIN LATERAL (SELECT x.project_code FROM petty_cash_requisition_line x
		                   WHERE x.pc_id = pc.id ORDER BY x.line_no, x.id LIMIT 1) fl ON TRUE
		LEFT JOIN project p ON p.project_code = COALESCE(pc.project_code, fl.project_code)
		WHERE l.id = $1`, id,
	).Scan(&pcID, &projectCode, &pcNo, &payeeName, &pcProjectCode, &pcProject, &contractAmount)
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
			"id":               id,
			"pc_id":            pcID,
			"project_code":     projectCode,
			"wo_no":            pcNo,
			"pc_no":            pcNo,
			"supplier_name":    payeeName,
			"wo_project_code":  pcProjectCode,
			"wo_project_name":  pcProject,
			"contract_amount":  contractAmount,
			"credit_term_days": nil,
			"terms":            terms,
		},
	})
}

// FindByPC is FindByWO for a petty cash requisition: data = {id} of its log, or null when none exists yet.
// @Router       /wo-contractor-log/by-pc/{pcId} [get]
func (h *WOContractorLogHandler) FindByPC(c *fiber.Ctx) error {
	pcID, err := strconv.ParseInt(c.Params("pcId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid pcId")
	}
	var id int64
	err = h.db.QueryRow(context.Background(),
		`SELECT id FROM wo_contractor_log WHERE pc_id = $1 ORDER BY id DESC LIMIT 1`, pcID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"success": true, "data": nil})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id}})
}

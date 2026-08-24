package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"erp-api/internal/middleware"
	"erp-api/internal/models"
)

// PettyCashHandler drives the petty cash requisition (ใบเบิกเงินสดย่อย) document — a request
// for cash reimbursement/advance to buy materials. It is NOT a stock-issue document: mat_code
// on each line is reference-only for the material picker / project_stock display, and nothing
// here writes to stock_item, stock_inventory, project_stock, or any inventory_transaction table
// (see CLAUDE.md — inventory/inventory_transaction are not used in production at all).
//
// One document can span multiple projects: project_code lives on petty_cash_requisition_line
// (NOT NULL), not the header — petty_cash_requisition has no project_code column at all. Every
// stock/project join below is done per-LINE, never against a single header project.
//
// Discount/VAT/WHT math mirrors POHandler.Create/Update exactly (internal/handlers/po.go):
// per line -> base = qty*unit_price, discount (pct of base or flat amt), VAT = 7% of
// (base-discount) when use_vat, WHT = wht_rate% of (base-discount) when use_wht and a rate is
// given, line_net = (base-discount)+vat-wht. Header totals sum the per-line components, with
// header vat_amount recomputed as 7% of (total-discount) rather than summed from lines (same
// as PO) to avoid rounding drift.
//
// The material picker reuses GET /master/allMaterial (master.go's GetAllMaterial) — the same
// hierarchical group->subgroup->mat_name->spec->brand picker already used elsewhere — extended
// with an optional project_code param that adds a read-only stock_on_hand hint per material
// (LEFT JOIN LATERAL project_stock). There is no petty-cash-specific material search endpoint.
//
// Approval follows the PO/MEMO per-document approver_id model (see isEligibleApprover's doc
// comment in approval_status.go): a specific approver is chosen client-side from the
// role-eligible pool (GET /master/eligible-approvers?doc_type=PETTY_CASH) and stored as
// petty_cash_requisition.approver_id. approval_doc_types already has a PETTY_CASH row wired to
// petty_cash_requisition/petty_cash_status_log, so GenericApprovalHandler's
// PUT /approval/PETTY_CASH/:id/approve|reject also works once isEligibleApprover knows about
// PETTY_CASH — Approve/Reject below are the doc-specific equivalents (mirrors MemoHandler.Approve).
type PettyCashHandler struct {
	db *pgxpool.Pool
}

func NewPettyCashHandler(db *pgxpool.Pool) *PettyCashHandler {
	return &PettyCashHandler{db: db}
}

func (h *PettyCashHandler) generatePCNo(ctx context.Context) (string, error) {
	var seq int64
	if err := h.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(id), 0)+1 FROM petty_cash_requisition`,
	).Scan(&seq); err != nil {
		return "", err
	}
	return fmt.Sprintf("PC-%s-%06d", time.Now().Format("2006"), seq), nil
}

type pcLineCalc struct {
	base, discAmt, afterDisc, vatAmt, whtAmt, net float64
	discType                                      string
}

// calcPettyCashLines mirrors POHandler.Create's discount/VAT/WHT calculation exactly (po.go ~line 605).
func calcPettyCashLines(lines []models.CreatePettyCashLineItem, useDiscount, useVat, useWht bool) ([]pcLineCalc, float64, float64, float64) {
	calcs := make([]pcLineCalc, len(lines))
	var totalAmount, discountAmount, whtAmount float64
	for i, l := range lines {
		lc := pcLineCalc{base: l.Qty * l.UnitPrice}
		lc.discType = l.DiscType
		if lc.discType == "" {
			lc.discType = "pct"
		}
		if useDiscount && l.Discount != 0 {
			if lc.discType == "amt" {
				lc.discAmt = l.Discount
			} else {
				lc.discAmt = lc.base * l.Discount / 100
			}
		}
		lc.afterDisc = lc.base - lc.discAmt
		if useVat {
			lc.vatAmt = lc.afterDisc * 0.07
		}
		if useWht && l.WhtRate != nil {
			lc.whtAmt = lc.afterDisc * (*l.WhtRate) / 100
		}
		lc.net = lc.afterDisc + lc.vatAmt - lc.whtAmt
		calcs[i] = lc

		totalAmount += lc.base
		discountAmount += lc.discAmt
		whtAmount += lc.whtAmt
	}
	return calcs, totalAmount, discountAmount, whtAmount
}

func (h *PettyCashHandler) getByID(ctx context.Context, id int64) (*models.PettyCashRequisition, error) {
	var pc models.PettyCashRequisition
	err := h.db.QueryRow(ctx, `
		SELECT pc.id, pc.pc_no, pc.pc_date, pc.requested_by, pc.purpose,
		       pc.currency, pc.total_amount, pc.use_discount, pc.discount_type, pc.discount_amount,
		       pc.use_vat, pc.vat_amount, pc.use_wht, pc.wht_amount, pc.net_amount,
		       pc.status, pc.approver_id, pc.remarks,
		       pc.created_at, pc.updated_at, pc.created_by, pc.updated_by,
		       u.full_name AS requested_by_name, au.full_name AS approver_name
		FROM petty_cash_requisition pc
		LEFT JOIN users u ON u.id = pc.requested_by
		LEFT JOIN users au ON au.id = pc.approver_id
		WHERE pc.id = $1`, id,
	).Scan(
		&pc.ID, &pc.PCNo, &pc.PCDate, &pc.RequestedBy, &pc.Purpose,
		&pc.Currency, &pc.TotalAmount, &pc.UseDiscount, &pc.DiscountType, &pc.DiscountAmount,
		&pc.UseVat, &pc.VatAmount, &pc.UseWht, &pc.WhtAmount, &pc.NetAmount,
		&pc.Status, &pc.ApproverID, &pc.Remarks,
		&pc.CreatedAt, &pc.UpdatedAt, &pc.CreatedBy, &pc.UpdatedBy,
		&pc.RequestedByName, &pc.ApproverName,
	)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.Query(ctx, `
		SELECT pcl.id, pcl.pc_id, pcl.line_no, pcl.project_code, pcl.mat_code, pcl.cost_subgroup_id, pcl.description,
		       pcl.qty, pcl.unit_price, pcl.amount, pcl.discount, pcl.disc_type,
		       pcl.line_discount, pcl.line_vat, pcl.line_wht, pcl.line_net, pcl.wht_rate, pcl.remarks,
		       p.project_name, mn.mat_name, u.unit_name, COALESCE(ps.qty_on_hand, 0)
		FROM petty_cash_requisition_line pcl
		LEFT JOIN project p ON p.project_code = pcl.project_code
		LEFT JOIN material_code mc ON mc.mat_code = pcl.mat_code
		LEFT JOIN mat_name mn ON mn.id = mc.mat_name_id
		LEFT JOIN unit u ON u.id = mc.unit_id
		LEFT JOIN LATERAL (
			SELECT qty_on_hand FROM project_stock
			WHERE project_code = pcl.project_code AND mat_code = pcl.mat_code
			ORDER BY updated_at DESC LIMIT 1
		) ps ON TRUE
		WHERE pcl.pc_id = $1
		ORDER BY pcl.line_no`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projectSet := map[string]bool{}
	for rows.Next() {
		var l models.PettyCashLine
		if err := rows.Scan(
			&l.ID, &l.PCID, &l.LineNo, &l.ProjectCode, &l.MatCode, &l.CostSubgroupID, &l.Description,
			&l.Qty, &l.UnitPrice, &l.Amount, &l.Discount, &l.DiscType,
			&l.LineDiscount, &l.LineVat, &l.LineWht, &l.LineNet, &l.WhtRate, &l.Remarks,
			&l.ProjectName, &l.ItemName, &l.Unit, &l.StockOnHand,
		); err != nil {
			return nil, err
		}
		pc.Lines = append(pc.Lines, l)
		if !projectSet[l.ProjectCode] {
			projectSet[l.ProjectCode] = true
			pc.ProjectCodes = append(pc.ProjectCodes, l.ProjectCode)
		}
	}

	return &pc, nil
}

// List godoc
// @Summary      List petty cash requisitions
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        status        query  string  false  "filter by status"
// @Param        project_code  query  string  false  "filter: document has at least one line with this project"
// @Param        date_from     query  string  false  "YYYY-MM-DD"
// @Param        date_to       query  string  false  "YYYY-MM-DD"
// @Param        page          query  int     false  "default 1"
// @Param        page_size     query  int     false  "default 20"
// @Success      200  {object}  models.PaginatedResponse
// @Router       /petty-cash [get]
func (h *PettyCashHandler) List(c *fiber.Ctx) error {
	var f models.PettyCashListFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid query params")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	offset := (f.Page - 1) * f.PageSize

	args := []any{}
	where := "WHERE 1=1"
	i := 1
	if f.Status != "" {
		where += fmt.Sprintf(" AND pc.status = $%d", i)
		args = append(args, f.Status)
		i++
	}
	if f.ProjectCode != "" {
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM petty_cash_requisition_line pcl2
			WHERE pcl2.pc_id = pc.id AND pcl2.project_code = $%d
		)`, i)
		args = append(args, f.ProjectCode)
		i++
	}
	if f.DateFrom != "" {
		where += fmt.Sprintf(" AND pc.pc_date >= $%d", i)
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where += fmt.Sprintf(" AND pc.pc_date <= $%d", i)
		args = append(args, f.DateTo)
		i++
	}

	ctx := context.Background()

	var total int64
	countSQL := "SELECT COUNT(*) FROM petty_cash_requisition pc " + where
	if err := h.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return err
	}

	args = append(args, f.PageSize, offset)
	// project_codes is aggregated (distinct, comma-joined) from every line on the doc, since
	// the header itself has no project_code — this doc can span multiple projects.
	listSQL := fmt.Sprintf(`
		SELECT pc.id, pc.pc_no, pc.pc_date, pc.requested_by, pc.purpose,
		       pc.currency, pc.total_amount, pc.status, pc.approver_id, pc.net_amount,
		       pc.created_at,
		       u.full_name AS requested_by_name, au.full_name AS approver_name,
		       pcodes.codes
		FROM petty_cash_requisition pc
		LEFT JOIN users u ON u.id = pc.requested_by
		LEFT JOIN users au ON au.id = pc.approver_id
		LEFT JOIN LATERAL (
			SELECT array_agg(DISTINCT pcl.project_code ORDER BY pcl.project_code) AS codes
			FROM petty_cash_requisition_line pcl WHERE pcl.pc_id = pc.id
		) pcodes ON TRUE
		%s
		ORDER BY pc.id DESC
		LIMIT $%d OFFSET $%d`, where, i, i+1)

	rows, err := h.db.Query(ctx, listSQL, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	items := []models.PettyCashRequisition{}
	for rows.Next() {
		var pc models.PettyCashRequisition
		if err := rows.Scan(
			&pc.ID, &pc.PCNo, &pc.PCDate, &pc.RequestedBy, &pc.Purpose,
			&pc.Currency, &pc.TotalAmount, &pc.Status, &pc.ApproverID, &pc.NetAmount,
			&pc.CreatedAt,
			&pc.RequestedByName, &pc.ApproverName,
			&pc.ProjectCodes,
		); err != nil {
			return err
		}
		items = append(items, pc)
	}

	totalPages := int(total)/f.PageSize + 1
	return c.JSON(fiber.Map{
		"success": true,
		"data": models.PaginatedResponse{
			Data: items, Total: total, Page: f.Page, PageSize: f.PageSize, TotalPages: totalPages,
		},
	})
}

// Get godoc
// @Summary      Get a petty cash requisition with lines
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Petty cash requisition ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /petty-cash/{id} [get]
func (h *PettyCashHandler) Get(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	pc, err := h.getByID(context.Background(), int64(id))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	return c.JSON(fiber.Map{"success": true, "data": pc})
}

func validatePettyCashRequest(req *models.CreatePettyCashRequest) error {
	if len(req.Lines) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least 1 line is required")
	}
	for i, l := range req.Lines {
		if l.ProjectCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: project_code is required", i))
		}
		if l.MatCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: mat_code is required", i))
		}
		if l.Qty <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: qty must be > 0", i))
		}
		if l.UnitPrice < 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: unit_price must be >= 0", i))
		}
	}
	return nil
}

// Create godoc
// @Summary      Create a petty cash requisition
// @Description  status starts as DRAFT unless status=PENDING_APPROVAL is passed (requires approver_id). Each line picks its own project_code — the document itself has no single project.
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreatePettyCashRequest  true  "Petty cash requisition data"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /petty-cash [post]
func (h *PettyCashHandler) Create(c *fiber.Ctx) error {
	var req models.CreatePettyCashRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := validatePettyCashRequest(&req); err != nil {
		return err
	}

	status := req.Status
	if status == "" {
		status = "DRAFT"
	}
	if status != "DRAFT" && status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "status must be DRAFT or PENDING_APPROVAL")
	}
	if status == "PENDING_APPROVAL" && req.ApproverID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "approver_id is required to submit directly")
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	if req.ApproverID != nil {
		var approverExists bool
		if err := h.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *req.ApproverID,
		).Scan(&approverExists); err != nil {
			return err
		}
		if !approverExists {
			return fiber.NewError(fiber.StatusBadRequest, "approver not found")
		}
	}

	useDiscount := req.UseDiscount
	useVat := req.UseVat
	useWht := req.UseWht
	discountType := req.DiscountType
	if discountType == "" {
		discountType = "pct"
	}
	currency := req.Currency
	if currency == "" {
		currency = "THB"
	}

	calcs, totalAmount, discountAmount, whtAmount := calcPettyCashLines(req.Lines, useDiscount, useVat, useWht)
	vatAmount := totalAmount - discountAmount
	if useVat {
		vatAmount *= 0.07
	} else {
		vatAmount = 0
	}
	netAmount := totalAmount - discountAmount + vatAmount - whtAmount

	pcNo, err := h.generatePCNo(ctx)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var remarks *string
	if req.Remarks != "" {
		remarks = &req.Remarks
	}
	var purpose *string
	if req.Purpose != "" {
		purpose = &req.Purpose
	}

	var pcID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO petty_cash_requisition
		  (pc_no, pc_date, requested_by, purpose, currency, total_amount,
		   use_discount, discount_type, discount_amount, use_vat, vat_amount, use_wht, wht_amount,
		   net_amount, status, approver_id, remarks, created_by, updated_by)
		VALUES ($1,CURRENT_DATE,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)
		RETURNING id`,
		pcNo, claims.UserID, purpose, currency, totalAmount,
		useDiscount, discountType, discountAmount, useVat, vatAmount, useWht, whtAmount,
		netAmount, status, req.ApproverID, remarks, claims.UserID,
	).Scan(&pcID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create petty cash requisition: "+err.Error())
	}

	for i, line := range req.Lines {
		lc := calcs[i]
		var desc *string
		if line.Description != "" {
			desc = &line.Description
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO petty_cash_requisition_line
			  (pc_id, line_no, project_code, mat_code, cost_subgroup_id, description, qty, unit_price, amount,
			   discount, disc_type, line_discount, line_vat, line_wht, line_net, wht_rate, remarks)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			pcID, i+1, line.ProjectCode, line.MatCode, line.CostSubgroupID, desc, line.Qty, line.UnitPrice, lc.base,
			line.Discount, lc.discType, lc.discAmt, lc.vatAmt, lc.whtAmt, lc.net, line.WhtRate, line.Remarks,
		); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: invalid project_code or mat_code", i))
			}
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("line %d insert error: %s", i+1, err.Error()))
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO petty_cash_status_log (pc_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,NULL,$2,$3,'Petty cash requisition created')`, pcID, status, claims.UserID,
	); err != nil {
		return err
	}

	if status == "PENDING_APPROVAL" {
		var hasConfig bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM approval_config WHERE doc_type='PETTY_CASH' AND step_no=1 AND is_active=true)`,
		).Scan(&hasConfig); err != nil {
			return err
		}
		if hasConfig {
			if _, err := tx.Exec(ctx, `
				INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status, amount)
				VALUES ('PETTY_CASH',$1,$2,1,$3,$4,'PENDING',$5)`,
				pcID, pcNo, claims.UserID, *req.ApproverID, totalAmount,
			); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	pc, err := h.getByID(context.Background(), pcID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": pc})
}

// Update godoc
// @Summary      Update a petty cash requisition (only while DRAFT or REJECTED)
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                            true  "Petty cash requisition ID"
// @Param        body  body  models.UpdatePettyCashRequest  true  "Petty cash requisition data"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Router       /petty-cash/{id} [put]
func (h *PettyCashHandler) Update(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	var req models.UpdatePettyCashRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := validatePettyCashRequest(&req); err != nil {
		return err
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	if err := h.db.QueryRow(ctx,
		`SELECT status FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&currentStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	if currentStatus != "DRAFT" && currentStatus != "REJECTED" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot update petty cash requisition with status '%s' — must be DRAFT or REJECTED", currentStatus))
	}

	if req.ApproverID != nil {
		var approverExists bool
		if err := h.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *req.ApproverID,
		).Scan(&approverExists); err != nil {
			return err
		}
		if !approverExists {
			return fiber.NewError(fiber.StatusBadRequest, "approver not found")
		}
	}

	useDiscount := req.UseDiscount
	useVat := req.UseVat
	useWht := req.UseWht
	discountType := req.DiscountType
	if discountType == "" {
		discountType = "pct"
	}
	currency := req.Currency
	if currency == "" {
		currency = "THB"
	}

	calcs, totalAmount, discountAmount, whtAmount := calcPettyCashLines(req.Lines, useDiscount, useVat, useWht)
	vatAmount := totalAmount - discountAmount
	if useVat {
		vatAmount *= 0.07
	} else {
		vatAmount = 0
	}
	netAmount := totalAmount - discountAmount + vatAmount - whtAmount

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var remarks *string
	if req.Remarks != "" {
		remarks = &req.Remarks
	}
	var purpose *string
	if req.Purpose != "" {
		purpose = &req.Purpose
	}

	if _, err := tx.Exec(ctx, `
		UPDATE petty_cash_requisition
		SET purpose=$1, currency=$2, total_amount=$3,
		    use_discount=$4, discount_type=$5, discount_amount=$6, use_vat=$7, vat_amount=$8,
		    use_wht=$9, wht_amount=$10, net_amount=$11, approver_id=$12, remarks=$13,
		    updated_at=NOW(), updated_by=$14
		WHERE id=$15`,
		purpose, currency, totalAmount,
		useDiscount, discountType, discountAmount, useVat, vatAmount,
		useWht, whtAmount, netAmount, req.ApproverID, remarks,
		claims.UserID, id,
	); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update petty cash requisition: "+err.Error())
	}

	if _, err := tx.Exec(ctx, `DELETE FROM petty_cash_requisition_line WHERE pc_id=$1`, id); err != nil {
		return err
	}
	for i, line := range req.Lines {
		lc := calcs[i]
		var desc *string
		if line.Description != "" {
			desc = &line.Description
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO petty_cash_requisition_line
			  (pc_id, line_no, project_code, mat_code, cost_subgroup_id, description, qty, unit_price, amount,
			   discount, disc_type, line_discount, line_vat, line_wht, line_net, wht_rate, remarks)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			id, i+1, line.ProjectCode, line.MatCode, line.CostSubgroupID, desc, line.Qty, line.UnitPrice, lc.base,
			line.Discount, lc.discType, lc.discAmt, lc.vatAmt, lc.whtAmt, lc.net, line.WhtRate, line.Remarks,
		); err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("lines[%d]: invalid project_code or mat_code", i))
			}
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("line %d insert error: %s", i+1, err.Error()))
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	pc, err := h.getByID(context.Background(), int64(id))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": pc})
}

// Delete godoc
// @Summary      Delete a petty cash requisition (only while DRAFT)
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Petty cash requisition ID"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id} [delete]
func (h *PettyCashHandler) Delete(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	ctx := context.Background()

	var currentStatus string
	if err := h.db.QueryRow(ctx,
		`SELECT status FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&currentStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	if currentStatus != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot delete petty cash requisition with status '%s' — must be DRAFT", currentStatus))
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM petty_cash_requisition_line WHERE pc_id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM petty_cash_status_log WHERE pc_id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM petty_cash_requisition WHERE id=$1`, id); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "petty cash requisition deleted"})
}

// Submit godoc
// @Summary      Submit for approval (DRAFT/REJECTED → PENDING_APPROVAL)
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int  true  "Petty cash requisition ID"
// @Param        body  body  object{approver_id=int64}  false  "approver_id — required if not already set"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id}/submit [post]
func (h *PettyCashHandler) Submit(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var body struct {
		ApproverID *int64 `json:"approver_id"`
	}
	_ = c.BodyParser(&body)

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	var pcNo string
	var approverID *int64
	if err := h.db.QueryRow(ctx,
		`SELECT status, pc_no, approver_id FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&currentStatus, &pcNo, &approverID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	if currentStatus != "DRAFT" && currentStatus != "REJECTED" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot submit petty cash requisition with status '%s' — must be DRAFT or REJECTED", currentStatus))
	}

	if body.ApproverID != nil {
		approverID = body.ApproverID
	}
	if approverID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "approver_id is required")
	}
	var approverExists bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *approverID,
	).Scan(&approverExists); err != nil {
		return err
	}
	if !approverExists {
		return fiber.NewError(fiber.StatusBadRequest, "approver not found")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE petty_cash_requisition
		SET status='PENDING_APPROVAL', approver_id=$1, updated_at=NOW(), updated_by=$2
		WHERE id=$3`, *approverID, claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO petty_cash_status_log (pc_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'PENDING_APPROVAL',$3,'Submitted for approval')`,
		id, currentStatus, claims.UserID,
	); err != nil {
		return err
	}

	var hasConfig bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM approval_config WHERE doc_type='PETTY_CASH' AND step_no=1 AND is_active=true)`,
	).Scan(&hasConfig); err != nil {
		return err
	}
	if hasConfig {
		if _, err := tx.Exec(ctx, `
			INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status, amount)
			SELECT 'PETTY_CASH', $1, $2, 1, $3, $4, 'PENDING', total_amount
			FROM petty_cash_requisition WHERE id=$1`,
			id, pcNo, claims.UserID, *approverID,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "Petty cash requisition submitted for approval"})
}

// approvalDecide handles both Approve and Reject — mirrors MemoHandler.Approve.
func (h *PettyCashHandler) approvalDecide(c *fiber.Ctx, action string) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var req models.ApprovalActionRequest
	_ = c.BodyParser(&req)
	if action == "REJECT" && (req.Comments == nil || strings.TrimSpace(*req.Comments) == "") {
		return fiber.NewError(fiber.StatusBadRequest, "comments are required when rejecting")
	}

	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	if err := h.db.QueryRow(ctx,
		`SELECT status FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&currentStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	if currentStatus != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot %s petty cash requisition with status '%s' — must be PENDING_APPROVAL", strings.ToLower(action), currentStatus))
	}

	// Menu permission (checked at the route layer) controls whether the approval screen/action
	// exists for this user at all; this check controls whether THIS document is theirs to act
	// on. Neither replaces the other — see isEligibleApprover's doc comment.
	isAssigned, err := isEligibleApprover(ctx, h.db, "PETTY_CASH", 1, int64(id), claims.UserID)
	if err != nil {
		return err
	}
	if !isAssigned {
		return fiber.NewError(fiber.StatusForbidden, "you are not the assigned approver for this petty cash requisition")
	}

	newStatus := "APPROVED"
	if action == "REJECT" {
		newStatus = "REJECTED"
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE petty_cash_requisition SET status=$1, updated_at=NOW(), updated_by=$2 WHERE id=$3`,
		newStatus, claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO petty_cash_status_log (pc_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,$3,$4,$5)`,
		id, currentStatus, newStatus, claims.UserID, req.Comments,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE approval_request SET status=$1, updated_at=NOW()
		WHERE doc_type='PETTY_CASH' AND doc_id=$2 AND status='PENDING'`,
		newStatus, id,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": fmt.Sprintf("Petty cash requisition %s", newStatus)})
}

// Approve godoc
// @Summary      Approve a petty cash requisition (PENDING_APPROVAL → APPROVED)
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                            true  "Petty cash requisition ID"
// @Param        body  body  models.ApprovalActionRequest  false  "Optional comments"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id}/approve [post]
func (h *PettyCashHandler) Approve(c *fiber.Ctx) error {
	return h.approvalDecide(c, "APPROVE")
}

// Reject godoc
// @Summary      Reject a petty cash requisition (PENDING_APPROVAL → REJECTED)
// @Tags         Petty Cash
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int                            true  "Petty cash requisition ID"
// @Param        body  body  models.ApprovalActionRequest  true  "comments required"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id}/reject [post]
func (h *PettyCashHandler) Reject(c *fiber.Ctx) error {
	return h.approvalDecide(c, "REJECT")
}

// Cancel godoc
// @Summary      Cancel a petty cash requisition (DRAFT → CANCELLED)
// @Tags         Petty Cash
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "Petty cash requisition ID"
// @Success      200  {object}  fiber.Map
// @Router       /petty-cash/{id}/cancel [post]
func (h *PettyCashHandler) Cancel(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	claims := middleware.GetClaims(c)
	ctx := context.Background()

	var currentStatus string
	if err := h.db.QueryRow(ctx,
		`SELECT status FROM petty_cash_requisition WHERE id=$1`, id,
	).Scan(&currentStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "petty cash requisition not found")
	}
	if currentStatus != "DRAFT" {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("cannot cancel petty cash requisition with status '%s' — must be DRAFT", currentStatus))
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE petty_cash_requisition SET status='CANCELLED', updated_at=NOW(), updated_by=$1 WHERE id=$2`,
		claims.UserID, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO petty_cash_status_log (pc_id, from_status, to_status, changed_by, remarks)
		VALUES ($1,$2,'CANCELLED',$3,'Cancelled')`, id, currentStatus, claims.UserID,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "petty cash requisition cancelled"})
}

package handlers

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type POApprovalHandler struct {
	db *pgxpool.Pool
}

func NewPOApprovalHandler(db *pgxpool.Pool) *POApprovalHandler {
	return &POApprovalHandler{db: db}
}

// ListPOApproval godoc
// @Summary      List purchase orders with filter and pagination
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        status           query  string  false  "status filter"
// @Param        my               query  bool    false  "when true, only POs created by the current user"
// @Param        po_no            query  string  false  "partial match on PO number"
// @Param        supplier         query  string  false  "partial match on supplier name, or exact/partial supplier code"
// @Param        created_by_name  query  string  false  "partial match on creator's full name"
// @Param        page             query  int     false  "page"   default(1)
// @Param        limit            query  int     false  "limit"  default(20)
// @Success      200  {object}  fiber.Map
// @Router       /po [get]
func (h *POApprovalHandler) List(c *fiber.Ctx) error {
	status := c.Query("status")
	page := max(c.QueryInt("page", 1), 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var statusFilter *string
	if status != "" {
		statusFilter = &status
	}

	var createdByFilter *int64
	if c.QueryBool("my", false) {
		claims := middleware.GetClaims(c)
		if claims == nil {
			return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
		}
		createdByFilter = &claims.UserID
	}

	var poNoFilter, supplierFilter, createdByNameFilter *string
	if v := strings.TrimSpace(c.Query("po_no")); v != "" {
		p := "%" + v + "%"
		poNoFilter = &p
	}
	if v := strings.TrimSpace(c.Query("supplier")); v != "" {
		p := "%" + v + "%"
		supplierFilter = &p
	}
	if v := strings.TrimSpace(c.Query("created_by_name")); v != "" {
		p := "%" + v + "%"
		createdByNameFilter = &p
	}

	var total int64
	h.db.QueryRow(context.Background(),
		`SELECT COUNT(*)
		FROM purchase_order po
		LEFT JOIN supplier s ON s.supplier_code = po.supplier_code
		LEFT JOIN users u ON u.id = po.created_by
		WHERE ($1::text IS NULL OR po.status = $1)
		AND ($2::bigint IS NULL OR po.created_by = $2)
		AND ($3::text IS NULL OR po.po_no ILIKE $3)
		AND ($4::text IS NULL OR s.supplier_name ILIKE $4 OR po.supplier_code ILIKE $4)
		AND ($5::text IS NULL OR u.full_name ILIKE $5)`,
		statusFilter, createdByFilter, poNoFilter, supplierFilter, createdByNameFilter,
	).Scan(&total)

	type POListItem struct {
		ID             int64   `json:"id"`
		PONo           string  `json:"po_no"`
		PODate         string  `json:"po_date"`
		Status         string  `json:"status"`
		TotalAmount    float64 `json:"total_amount"`
		NetAmount      float64 `json:"net_amount"`
		Currency       string  `json:"currency"`
		SupplierName   string  `json:"supplier_name"`
		CreatedByName  string  `json:"created_by_name"`
		ExpectedDate   *string `json:"expected_date"`
		UseDiscount    bool    `json:"use_discount"`
		UseVAT         bool    `json:"use_vat"`
		UseWHT         bool    `json:"use_wht"`
		DiscountAmount float64 `json:"discount_amount"`
		VATAmount      float64 `json:"vat_amount"`
		WHTAmount      float64 `json:"wht_amount"`
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT
		    po.id, po.po_no, po.po_date::text, po.status,
		    po.total_amount, po.net_amount, po.currency,
		    COALESCE(s.supplier_name, po.supplier_code) AS supplier_name,
		    COALESCE(u.full_name, '') AS created_by_name,
		    po.expected_date::text,
		    po.use_discount, po.use_vat, po.use_wht,
		    po.discount_amount, po.vat_amount, po.wht_amount
		FROM purchase_order po
		LEFT JOIN supplier s ON s.supplier_code = po.supplier_code
		LEFT JOIN users u ON u.id = po.created_by
		WHERE ($1::text IS NULL OR po.status = $1)
		AND ($2::bigint IS NULL OR po.created_by = $2)
		AND ($3::text IS NULL OR po.po_no ILIKE $3)
		AND ($4::text IS NULL OR s.supplier_name ILIKE $4 OR po.supplier_code ILIKE $4)
		AND ($5::text IS NULL OR u.full_name ILIKE $5)
		ORDER BY po.created_at DESC
		LIMIT $6 OFFSET $7`,
		statusFilter, createdByFilter, poNoFilter, supplierFilter, createdByNameFilter, limit, offset,
	)
	if err != nil {
		log.Printf("❌ list PO query error: %v", err)
		return err
	}
	defer rows.Close()

	items := make([]POListItem, 0)
	for rows.Next() {
		var item POListItem
		if err := rows.Scan(
			&item.ID, &item.PONo, &item.PODate, &item.Status,
			&item.TotalAmount, &item.NetAmount, &item.Currency,
			&item.SupplierName, &item.CreatedByName, &item.ExpectedDate,
			&item.UseDiscount, &item.UseVAT, &item.UseWHT,
			&item.DiscountAmount, &item.VATAmount, &item.WHTAmount,
		); err != nil {
			log.Printf("❌ list PO scan error: %v", err)
		}
		items = append(items, item)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"items": items,
			"total": total,
			"page":  page,
			"limit": limit,
		},
	})
}

// GetPODetail godoc
// @Summary      Get full PO detail including lines with material info
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/{id} [get]
func (h *POApprovalHandler) GetDetail(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "invalid id"})
	}

	type POAttachItem struct {
		ID         int64  `json:"id"`
		FileName   string `json:"file_name"`
		FilePath   string `json:"file_path"`
		FileSize   int64  `json:"file_size"`
		FileType   string `json:"file_type"`
		UploadedAt string `json:"uploaded_at"`
		UploadedBy int64  `json:"uploaded_by"`
	}

	type POLineItem struct {
		ID              int64    `json:"id"`
		LineNo          int      `json:"line_no"`
		MatCode         string   `json:"mat_code"`
		QtyOrdered      float64  `json:"qty_ordered"`
		QtyReceived     float64  `json:"qty_received"`
		UnitPrice       float64  `json:"unit_price"`
		Amount          float64  `json:"amount"`
		Status          string   `json:"status"`
		Description     *string  `json:"description,omitempty"`
		Remarks         *string  `json:"remarks,omitempty"`
		MatName         *string  `json:"mat_name,omitempty"`
		UnitName        *string  `json:"unit_name,omitempty"`
		BrandName       *string  `json:"brand_name,omitempty"`
		SpecDescription *string  `json:"spec_description,omitempty"`
		GroupName       *string  `json:"group_name,omitempty"`
		SubgroupName    *string  `json:"subgroup_name,omitempty"`
		CurrentStockQty float64  `json:"current_stock_qty"`
		Discount        float64  `json:"discount"`
		WHTRate         *float64 `json:"wht_rate"`
		LineDiscount    float64  `json:"line_discount"`
		LineVAT         float64  `json:"line_vat"`
		LineWHT         float64  `json:"line_wht"`
		LineNet         float64  `json:"line_net"`
	}

	type PODetail struct {
		ID                   int64          `json:"id"`
		PONo                 string         `json:"po_no"`
		PODate               string         `json:"po_date"`
		Status               string         `json:"status"`
		SupplierCode         string         `json:"supplier_code"`
		SupplierName         string         `json:"supplier_name"`
		SupplierShortName    *string        `json:"supplier_short_name,omitempty"`
		SupplierTaxID        *string        `json:"supplier_tax_id,omitempty"`
		SupplierAddress      *string        `json:"supplier_address,omitempty"`
		SupplierContactName  *string        `json:"supplier_contact_name,omitempty"`
		SupplierContactPhone *string        `json:"supplier_contact_phone,omitempty"`
		SupplierContactEmail *string        `json:"supplier_contact_email,omitempty"`
		SupplierOfficePhone  *string        `json:"supplier_office_phone,omitempty"`
		SupplierFax          *string        `json:"supplier_fax,omitempty"`
		SupplierSalesPerson  *string        `json:"supplier_sales_person,omitempty"`
		SupplierPaymentTerms *string        `json:"supplier_payment_terms,omitempty"`
		PaymentTerms         *string        `json:"payment_terms"`
		DeliveryAddress      *string        `json:"delivery_address"`
		LocationText         *string        `json:"location_text"`
		WarehouseCode        *string        `json:"warehouse_code"`
		ProjectCode          *string        `json:"project_code,omitempty"`
		PRID                 *int64         `json:"pr_id,omitempty"`
		PRNo                 *string        `json:"pr_no,omitempty"`
		PRDate               *string        `json:"pr_date,omitempty"`
		RequestedBy          *int64         `json:"requested_by,omitempty"`
		RequestedByName      string         `json:"requested_by_name,omitempty"`
		ApproverID           *int64         `json:"approver_id,omitempty"`
		ApproverName         *string        `json:"approver_name,omitempty"`
		Ref                  *string        `json:"ref,omitempty"`
		Currency             string         `json:"currency"`
		TotalAmount          float64        `json:"total_amount"`
		VATAmount            float64        `json:"vat_amount"`
		NetAmount            float64        `json:"net_amount"`
		UseDiscount          bool           `json:"use_discount"`
		DiscountType         string         `json:"discount_type"`
		UseVAT               bool           `json:"use_vat"`
		UseWHT               bool           `json:"use_wht"`
		DiscountAmount       float64        `json:"discount_amount"`
		WHTAmount            float64        `json:"wht_amount"`
		ExpectedDate         *string        `json:"expected_date"`
		Remarks              *string        `json:"remarks"`
		CreatedByName        string         `json:"created_by_name"`
		CanEditApproved      bool           `json:"can_edit_approved"`
		Lines                []POLineItem   `json:"lines"`
		Attachments          []POAttachItem `json:"attachments"`
	}

	row := h.db.QueryRow(context.Background(), `
		SELECT
		    po.id, po.po_no, po.po_date::text, po.status,
		    po.supplier_code, COALESCE(s.supplier_name, po.supplier_code) AS supplier_name,
		    s.supplier_short_name, s.tax_id, s.address, s.contact_name, s.contact_phone, s.contact_email,
		    s.office_phone, s.fax, s.sales_person, s.payment_terms AS supplier_payment_terms,
		    po.payment_terms, po.delivery_address, po.location_text, po.warehouse_code, po.project_code, po.pr_id,
		    pr.pr_no, pr.pr_date::text,
		    po.requested_by, COALESCE(ru.full_name, '') AS requested_by_name,
		    po.approver_id, au.full_name AS approver_name, po.ref,
		    po.currency, po.total_amount, po.vat_amount, po.net_amount,
		    po.use_discount, po.discount_type, po.use_vat, po.use_wht,
		    po.discount_amount, po.wht_amount,
		    po.expected_date::text, po.remarks,
		    COALESCE(u.full_name, '') AS created_by_name,
		    po.created_at
		FROM purchase_order po
		LEFT JOIN supplier s ON s.supplier_code = po.supplier_code
		LEFT JOIN users u ON u.id = po.created_by
		LEFT JOIN users ru ON ru.id = po.requested_by
		LEFT JOIN users au ON au.id = po.approver_id
		LEFT JOIN purchase_request pr ON pr.id = po.pr_id
		WHERE po.id = $1`, id)

	var po PODetail
	po.Lines = make([]POLineItem, 0)
	po.Attachments = make([]POAttachItem, 0)
	var createdAt time.Time

	if err := row.Scan(
		&po.ID, &po.PONo, &po.PODate, &po.Status,
		&po.SupplierCode, &po.SupplierName,
		&po.SupplierShortName, &po.SupplierTaxID, &po.SupplierAddress, &po.SupplierContactName,
		&po.SupplierContactPhone, &po.SupplierContactEmail, &po.SupplierOfficePhone, &po.SupplierFax,
		&po.SupplierSalesPerson, &po.SupplierPaymentTerms,
		&po.PaymentTerms, &po.DeliveryAddress, &po.LocationText, &po.WarehouseCode, &po.ProjectCode, &po.PRID,
		&po.PRNo, &po.PRDate,
		&po.RequestedBy, &po.RequestedByName,
		&po.ApproverID, &po.ApproverName, &po.Ref,
		&po.Currency, &po.TotalAmount, &po.VATAmount, &po.NetAmount,
		&po.UseDiscount, &po.DiscountType, &po.UseVAT, &po.UseWHT,
		&po.DiscountAmount, &po.WHTAmount,
		&po.ExpectedDate, &po.Remarks, &po.CreatedByName, &createdAt,
	); err != nil {
		log.Printf("❌ PO detail header scan error: %v", err)
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	po.CanEditApproved = po.Status == "APPROVED" && time.Since(createdAt) < 365*24*time.Hour

	lineRows, err := h.db.Query(context.Background(), `
		SELECT
		    pol.id, pol.line_no, pol.mat_code, pol.qty_ordered,
		    pol.qty_received, pol.unit_price, pol.amount, pol.status, pol.description, pol.remarks,
		    mn.mat_name, u.unit_name, b.brand_name, ss.spec_description,
		    mg.group_name, sg.subgroup_name,
		    COALESCE(si.qty, 0) AS current_stock_qty,
		    pol.discount, pol.wht_rate, pol.line_discount, pol.line_vat, pol.line_wht, pol.line_net
		FROM purchase_order_line pol
		LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
		LEFT JOIN mat_group     mg ON mg.id = mc.group_id
		LEFT JOIN subgroup      sg ON sg.id = mc.subgroup_id
		LEFT JOIN mat_name      mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size     ss ON ss.id = mc.spec_id
		LEFT JOIN brand         b  ON b.id  = mc.brand_id
		LEFT JOIN unit          u  ON u.id  = mc.unit_id
		LEFT JOIN stock_item    si ON si.mat_code = pol.mat_code
		WHERE pol.po_id = $1
		ORDER BY pol.line_no`, id)
	if err != nil {
		log.Printf("❌ PO lines query error: %v", err)
	} else {
		defer lineRows.Close()
		for lineRows.Next() {
			var l POLineItem
			if err := lineRows.Scan(
				&l.ID, &l.LineNo, &l.MatCode, &l.QtyOrdered,
				&l.QtyReceived, &l.UnitPrice, &l.Amount, &l.Status, &l.Description, &l.Remarks,
				&l.MatName, &l.UnitName, &l.BrandName, &l.SpecDescription,
				&l.GroupName, &l.SubgroupName, &l.CurrentStockQty,
				&l.Discount, &l.WHTRate, &l.LineDiscount, &l.LineVAT, &l.LineWHT, &l.LineNet,
			); err != nil {
				log.Printf("❌ PO lines scan error: %v", err)
			}
			po.Lines = append(po.Lines, l)
		}
	}

	attRows, err := h.db.Query(context.Background(), `
		SELECT id, file_name, file_path, file_size, file_type,
		       to_char(uploaded_at, 'YYYY-MM-DD HH24:MI'), COALESCE(uploaded_by, 0)
		FROM po_attachment WHERE po_id = $1 ORDER BY uploaded_at`, id)
	if err != nil {
		log.Printf("❌ PO attachments query error: %v", err)
	} else {
		defer attRows.Close()
		for attRows.Next() {
			var a POAttachItem
			if err := attRows.Scan(
				&a.ID, &a.FileName, &a.FilePath, &a.FileSize,
				&a.FileType, &a.UploadedAt, &a.UploadedBy,
			); err != nil {
				log.Printf("❌ PO attachments scan error: %v", err)
			}
			po.Attachments = append(po.Attachments, a)
		}
	}

	return c.JSON(fiber.Map{"success": true, "data": po})
}

// ApprovePO godoc
// SUPERSEDED: PO approve now goes through the generic, config-driven
// PUT /approval/PO/{id}/approve (generic_approval.go), which also enforces eligibility
// (this handler never did — see the commented-out TODO below). Left in place, still
// routed at PUT /po/{id}/approve, only for rollback safety — do not add new callers.
// @Summary      Approve a purchase order
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/{id}/approve [put]
func (h *POApprovalHandler) Approve(c *fiber.Ctx) error {
	// TODO: enable when permission system is ready
	// claims := middleware.GetClaims(c)
	// if !hasRole(claims, "MANAGER", "DIRECTOR", "MD") {
	//     return c.Status(403).JSON(fiber.Map{"success": false, "message": "permission denied"})
	// }

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "invalid id"})
	}

	var userID int64
	if claims := middleware.GetClaims(c); claims != nil {
		userID = claims.UserID
	}

	var status string
	if err := h.db.QueryRow(context.Background(),
		`SELECT status FROM purchase_order WHERE id = $1`, id,
	).Scan(&status); err != nil {
		log.Printf("❌ PO approve fetch error: %v", err)
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if status != "PENDING_APPROVAL" && status != "PENDING_REAPPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not pending approval")
	}

	if _, err := h.db.Exec(context.Background(),
		`UPDATE purchase_order SET status = 'APPROVED', updated_at = NOW(), updated_by = $1 WHERE id = $2`,
		userID, id,
	); err != nil {
		log.Printf("❌ PO approve update error: %v", err)
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "PO approved successfully"})
}

// RejectPO godoc
// SUPERSEDED: see the Approve method's note above — use PUT /approval/PO/{id}/reject instead.
// @Summary      Reject a purchase order
// @Tags         Purchase Order
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int     true  "PO ID"
// @Param        body  body  object  true  "Rejection reason"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/{id}/reject [put]
func (h *POApprovalHandler) Reject(c *fiber.Ctx) error {
	// TODO: enable when permission system is ready
	// claims := middleware.GetClaims(c)
	// if !hasRole(claims, "MANAGER", "DIRECTOR", "MD") {
	//     return c.Status(403).JSON(fiber.Map{"success": false, "message": "permission denied"})
	// }

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "invalid id"})
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Reason == "" {
		return fiber.NewError(fiber.StatusBadRequest, "reason is required")
	}

	var userID int64
	if claims := middleware.GetClaims(c); claims != nil {
		userID = claims.UserID
	}

	var status string
	if err := h.db.QueryRow(context.Background(),
		`SELECT status FROM purchase_order WHERE id = $1`, id,
	).Scan(&status); err != nil {
		log.Printf("❌ PO reject fetch error: %v", err)
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if status != "PENDING_APPROVAL" && status != "PENDING_REAPPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not pending approval")
	}

	if _, err := h.db.Exec(context.Background(),
		`UPDATE purchase_order SET status = 'REJECTED', remarks = $1, updated_at = NOW(), updated_by = $2 WHERE id = $3`,
		req.Reason, userID, id,
	); err != nil {
		log.Printf("❌ PO reject update error: %v", err)
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "PO rejected successfully"})
}

// CancelPO godoc
// @Summary      Cancel an approved purchase order
// @Tags         Purchase Order
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "PO ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /po/{id}/cancel [put]
func (h *POApprovalHandler) Cancel(c *fiber.Ctx) error {
	// TODO: enable when permission system is ready
	// claims := middleware.GetClaims(c)
	// if !hasRole(claims, "MANAGER", "DIRECTOR", "MD") {
	//     return c.Status(403).JSON(fiber.Map{"success": false, "message": "permission denied"})
	// }

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "invalid id"})
	}

	var userID int64
	if claims := middleware.GetClaims(c); claims != nil {
		userID = claims.UserID
	}

	var status string
	if err := h.db.QueryRow(context.Background(),
		`SELECT status FROM purchase_order WHERE id = $1`, id,
	).Scan(&status); err != nil {
		log.Printf("❌ PO cancel fetch error: %v", err)
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if status != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "only approved POs can be cancelled")
	}

	if _, err := h.db.Exec(context.Background(),
		`UPDATE purchase_order SET status = 'CANCELLED', updated_at = NOW(), updated_by = $1 WHERE id = $2`,
		userID, id,
	); err != nil {
		log.Printf("❌ PO cancel update error: %v", err)
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "PO cancelled successfully"})
}

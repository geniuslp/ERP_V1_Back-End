package handlers

import (
	"context"
	"log"
	"strconv"

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
// @Param        status  query  string  false  "status filter"
// @Param        page    query  int     false  "page"   default(1)
// @Param        limit   query  int     false  "limit"  default(20)
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

	var total int64
	h.db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM purchase_order WHERE ($1::text IS NULL OR status = $1)`,
		statusFilter,
	).Scan(&total)

	type POListItem struct {
		ID            int64   `json:"id"`
		PONo          string  `json:"po_no"`
		PODate        string  `json:"po_date"`
		Status        string  `json:"status"`
		TotalAmount   float64 `json:"total_amount"`
		NetAmount     float64 `json:"net_amount"`
		Currency      string  `json:"currency"`
		SupplierName  string  `json:"supplier_name"`
		CreatedByName string  `json:"created_by_name"`
		ExpectedDate  *string `json:"expected_date"`
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT
		    po.id, po.po_no, po.po_date::text, po.status,
		    po.total_amount, po.net_amount, po.currency,
		    COALESCE(s.supplier_name, po.supplier_code) AS supplier_name,
		    COALESCE(u.full_name, '') AS created_by_name,
		    po.expected_date::text
		FROM purchase_order po
		LEFT JOIN supplier s ON s.supplier_code = po.supplier_code
		LEFT JOIN users u ON u.id = po.created_by
		WHERE ($1::text IS NULL OR po.status = $1)
		ORDER BY po.created_at DESC
		LIMIT $2 OFFSET $3`,
		statusFilter, limit, offset,
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

	type POLineItem struct {
		ID              int64   `json:"id"`
		LineNo          int     `json:"line_no"`
		MatCode         string  `json:"mat_code"`
		QtyOrdered      float64 `json:"qty_ordered"`
		QtyReceived     float64 `json:"qty_received"`
		UnitPrice       float64 `json:"unit_price"`
		Amount          float64 `json:"amount"`
		Status          string  `json:"status"`
		Description     *string `json:"description,omitempty"`
		Remarks         *string `json:"remarks,omitempty"`
		MatName         *string `json:"mat_name,omitempty"`
		UnitName        *string `json:"unit_name,omitempty"`
		BrandName       *string `json:"brand_name,omitempty"`
		SpecDescription *string `json:"spec_description,omitempty"`
		GroupName       *string `json:"group_name,omitempty"`
		SubgroupName    *string `json:"subgroup_name,omitempty"`
	}

	type PODetail struct {
		ID              int64        `json:"id"`
		PONo            string       `json:"po_no"`
		PODate          string       `json:"po_date"`
		Status          string       `json:"status"`
		SupplierCode    string       `json:"supplier_code"`
		SupplierName    string       `json:"supplier_name"`
		PaymentTerms    *string      `json:"payment_terms"`
		DeliveryAddress *string      `json:"delivery_address"`
		WarehouseCode   *string      `json:"warehouse_code"`
		Currency        string       `json:"currency"`
		TotalAmount     float64      `json:"total_amount"`
		VATAmount       float64      `json:"vat_amount"`
		NetAmount       float64      `json:"net_amount"`
		ExpectedDate    *string      `json:"expected_date"`
		Remarks         *string      `json:"remarks"`
		CreatedByName   string       `json:"created_by_name"`
		Lines           []POLineItem `json:"lines"`
	}

	row := h.db.QueryRow(context.Background(), `
		SELECT
		    po.id, po.po_no, po.po_date::text, po.status,
		    po.supplier_code, COALESCE(s.supplier_name, po.supplier_code) AS supplier_name,
		    po.payment_terms, po.delivery_address, po.warehouse_code,
		    po.currency, po.total_amount, po.vat_amount, po.net_amount,
		    po.expected_date::text, po.remarks,
		    COALESCE(u.full_name, '') AS created_by_name
		FROM purchase_order po
		LEFT JOIN supplier s ON s.supplier_code = po.supplier_code
		LEFT JOIN users u ON u.id = po.created_by
		WHERE po.id = $1`, id)

	var po PODetail
	po.Lines = make([]POLineItem, 0)

	if err := row.Scan(
		&po.ID, &po.PONo, &po.PODate, &po.Status,
		&po.SupplierCode, &po.SupplierName,
		&po.PaymentTerms, &po.DeliveryAddress, &po.WarehouseCode,
		&po.Currency, &po.TotalAmount, &po.VATAmount, &po.NetAmount,
		&po.ExpectedDate, &po.Remarks, &po.CreatedByName,
	); err != nil {
		log.Printf("❌ PO detail header scan error: %v", err)
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}

	lineRows, err := h.db.Query(context.Background(), `
		SELECT
		    pol.id, pol.line_no, pol.mat_code, pol.qty_ordered,
		    pol.qty_received, pol.unit_price, pol.amount, pol.status, pol.description, pol.remarks,
		    mn.mat_name, u.unit_name, b.brand_name, ss.spec_description,
		    mg.group_name, sg.subgroup_name
		FROM purchase_order_line pol
		LEFT JOIN material_code mc ON mc.mat_code = pol.mat_code
		LEFT JOIN mat_group     mg ON mg.id = mc.group_id
		LEFT JOIN subgroup      sg ON sg.id = mc.subgroup_id
		LEFT JOIN mat_name      mn ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size     ss ON ss.id = mc.spec_id
		LEFT JOIN brand         b  ON b.id  = mc.brand_id
		LEFT JOIN unit          u  ON u.id  = mc.unit_id
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
				&l.GroupName, &l.SubgroupName,
			); err != nil {
				log.Printf("❌ PO lines scan error: %v", err)
			}
			po.Lines = append(po.Lines, l)
		}
	}

	return c.JSON(fiber.Map{"success": true, "data": po})
}

// ApprovePO godoc
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
		`SELECT status FROM purchase_order WHERE po_id = $1`, id,
	).Scan(&status); err != nil {
		log.Printf("❌ PO approve fetch error: %v", err)
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not pending approval")
	}

	if _, err := h.db.Exec(context.Background(),
		`UPDATE purchase_order SET status = 'APPROVED', updated_at = NOW(), updated_by = $1 WHERE po_id = $2`,
		userID, id,
	); err != nil {
		log.Printf("❌ PO approve update error: %v", err)
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "PO approved successfully"})
}

// RejectPO godoc
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
		`SELECT status FROM purchase_order WHERE po_id = $1`, id,
	).Scan(&status); err != nil {
		log.Printf("❌ PO reject fetch error: %v", err)
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if status != "PENDING_APPROVAL" {
		return fiber.NewError(fiber.StatusBadRequest, "PO is not pending approval")
	}

	if _, err := h.db.Exec(context.Background(),
		`UPDATE purchase_order SET status = 'REJECTED', remarks = $1, updated_at = NOW(), updated_by = $2 WHERE po_id = $3`,
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
		`SELECT status FROM purchase_order WHERE po_id = $1`, id,
	).Scan(&status); err != nil {
		log.Printf("❌ PO cancel fetch error: %v", err)
		return fiber.NewError(fiber.StatusNotFound, "PO not found")
	}
	if status != "APPROVED" {
		return fiber.NewError(fiber.StatusBadRequest, "only approved POs can be cancelled")
	}

	if _, err := h.db.Exec(context.Background(),
		`UPDATE purchase_order SET status = 'CANCELLED', updated_at = NOW(), updated_by = $1 WHERE po_id = $2`,
		userID, id,
	); err != nil {
		log.Printf("❌ PO cancel update error: %v", err)
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": "PO cancelled successfully"})
}

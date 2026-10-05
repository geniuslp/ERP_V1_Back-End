package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// ICReturnDocumentSummary is one row of GET /ic/pos/{poId}/returns. Naming mirrors
// ICReceiveDocumentSummary (id, *_no, created_at, line_count) plus return_date / created_by_name /
// total_qty.
type ICReturnDocumentSummary struct {
	ID            int64   `json:"id"`
	DocType       string  `json:"doc_type"`
	ReturnNo      string  `json:"return_no"`
	ReturnDate    string  `json:"return_date"`
	CreatedAt     string  `json:"created_at"`
	CreatedByName *string `json:"created_by_name"`
	LineCount     int64   `json:"line_count"`
	TotalQty      float64 `json:"total_qty"`
}

// ListReturnDocuments godoc
// @Summary      List the return documents (RT-) of a PO
// @Description  Newest first. A PO can have many return documents (one per submit of POST /ic/pos/{poId}/return-lines). line_count / total_qty come from ic_po_return_line. Returns made before return documents existed have no row here (not backfilled). Same naming as GET /ic/pos/{poId}/receive-documents; doc_type is always "RETURN".
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        poId  path  int  true  "purchase_order.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /ic/pos/{poId}/returns [get]
func (h *ICHandler) ListReturnDocuments(c *fiber.Ctx) error {
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
		SELECT rd.id, rd.return_no, rd.return_date::text, rd.created_at::text, u.full_name,
		       COUNT(rl.id), COALESCE(SUM(rl.return_qty), 0)
		FROM ic_po_return_document rd
		LEFT JOIN users u ON u.id = rd.created_by
		LEFT JOIN ic_po_return_line rl ON rl.return_id = rd.id
		WHERE rd.po_id = $1
		GROUP BY rd.id, rd.return_no, rd.return_date, rd.created_at, u.full_name
		ORDER BY rd.created_at DESC, rd.id DESC`, poID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถดึงรายการเอกสารคืนสินค้าได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()

	items := []ICReturnDocumentSummary{}
	for rows.Next() {
		var s ICReturnDocumentSummary
		if err := rows.Scan(&s.ID, &s.ReturnNo, &s.ReturnDate, &s.CreatedAt, &s.CreatedByName, &s.LineCount, &s.TotalQty); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านรายการเอกสารคืนสินค้าได้")
		}
		s.DocType = "RETURN"
		items = append(items, s)
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"documents": items}})
}

type ICReturnDocumentLine struct {
	LineNo    int     `json:"line_no"`
	CostCode  *string `json:"cost_code"`
	MatCode   string  `json:"mat_code"`
	ItemName  *string `json:"item_name"`
	SpecName  *string `json:"spec_name"`
	Unit      *string `json:"unit"`
	ReturnQty float64 `json:"return_qty"`
	Remarks   *string `json:"remarks"`
}

// GetReturnDocument godoc
// @Summary      Get one return document in full (print feed)
// @Description  Header (return_no, return_date, remarks, created_at, created_by_name), the PO (po_id, po_no, order_type), project (project_code, project_name), supplier_name, and lines with the 4-level cost code (subject+job+group+subgroup), mat_code, item_name, spec_name, unit, return_qty, remarks. doc_type is always "RETURN".
// @Tags         IC
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  int  true  "ic_po_return_document.id"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Router       /ic/returns/{id} [get]
func (h *ICHandler) GetReturnDocument(c *fiber.Ctx) error {
	ctx := context.Background()

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "id ไม่ถูกต้อง")
	}

	var (
		poID                                   int64
		returnNo, returnDate, createdAt, ordTy string
		remarks, createdByName                 *string
		poNo                                   string
		projectCode, projectName, supplierName *string
	)
	if err := h.db.QueryRow(ctx, `
		SELECT rd.po_id, rd.return_no, rd.return_date::text, rd.created_at::text, rd.remarks, u.full_name,
		       po.po_no, po.order_type, po.project_code, pj.project_name, s.supplier_name
		FROM ic_po_return_document rd
		JOIN purchase_order po ON po.id = rd.po_id
		LEFT JOIN users u ON u.id = rd.created_by
		LEFT JOIN project pj ON pj.project_code = po.project_code
		LEFT JOIN supplier s ON s.id = po.supplier_id
		WHERE rd.id = $1`, id,
	).Scan(&poID, &returnNo, &returnDate, &createdAt, &remarks, &createdByName,
		&poNo, &ordTy, &projectCode, &projectName, &supplierName); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "ไม่พบเอกสารคืนสินค้า")
	}

	rows, err := h.db.Query(ctx, `
		SELECT rl.line_no,
		       csub.subject_code || cj.job_code || cg.group_code || cs.subgroup_code,
		       rl.mat_code, mn.mat_name, sp.spec_description, un.unit_name, rl.return_qty, rl.remarks
		FROM ic_po_return_line rl
		LEFT JOIN cost_subgroup cs ON cs.id = rl.cost_subgroup_id
		LEFT JOIN cost_group    cg ON cg.id = cs.group_id
		LEFT JOIN cost_job      cj ON cj.id = cg.job_id
		LEFT JOIN cost_subject  csub ON csub.id = cj.subject_id
		LEFT JOIN material_code mc ON mc.mat_code = rl.mat_code
		LEFT JOIN mat_name mn      ON mn.id = mc.mat_name_id
		LEFT JOIN spec_size sp     ON sp.id = mc.spec_id
		LEFT JOIN unit un          ON un.id = mc.unit_id
		WHERE rl.return_id = $1
		ORDER BY rl.line_no`, id)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถดึงรายการคืนสินค้าได้ กรุณาลองใหม่อีกครั้ง")
	}
	defer rows.Close()

	lines := []ICReturnDocumentLine{}
	for rows.Next() {
		var l ICReturnDocumentLine
		if err := rows.Scan(&l.LineNo, &l.CostCode, &l.MatCode, &l.ItemName, &l.SpecName, &l.Unit, &l.ReturnQty, &l.Remarks); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถอ่านรายการคืนสินค้าได้")
		}
		lines = append(lines, l)
	}

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"id":              id,
		"doc_type":        "RETURN",
		"return_no":       returnNo,
		"return_date":     returnDate,
		"remarks":         remarks,
		"created_at":      createdAt,
		"created_by_name": createdByName,
		"po_id":           poID,
		"po_no":           poNo,
		"order_type":      ordTy,
		"project_code":    projectCode,
		"project_name":    projectName,
		"supplier_name":   supplierName,
		"lines":           lines,
	}})
}

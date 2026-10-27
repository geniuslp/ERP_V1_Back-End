package handlers

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"erp-api/internal/middleware"
)

// Warehouse stock upload (Master > คลังสินค้า, Excel). Same template as the Stock Items import
// (parseStockItemExcelRows: 3-row header, DESCRIPTION / QTY / UNIT / MAT CODE / ... columns), applied to ONE warehouse
// chosen on the page, in one of two modes:
//
//	update — the file's QTY overwrites the quantity on hand (unit, unit_cost and DESCRIPTION are updated too)
//	add    — the file's QTY is added on top of the quantity on hand (e.g. goods returned from site back into stock)
//
// Both write stock_transaction rows (ref_doc_type BULK_UPLOAD) so the change shows in the movement history.
// stock_item is keyed by mat_code alone, so an item already stocked in ANOTHER warehouse is rejected rather than
// silently moved. The upload is all-or-nothing: any problem row rejects the whole file, so a corrected file can be
// uploaded again without double-adding the rows that were fine.
const (
	whUploadModeUpdate = "update"
	whUploadModeAdd    = "add"
)

type whUploadRow struct {
	RowNo       int      `json:"row_no"`
	MatCode     string   `json:"mat_code"`
	MatName     string   `json:"mat_name"`
	Unit        string   `json:"unit"`
	UnitCost    float64  `json:"unit_cost"`
	FileQty     float64  `json:"file_qty"`
	CurrentQty  *float64 `json:"current_qty"` // nil = not in stock yet (a new stock item)
	NewQty      float64  `json:"new_qty"`     // quantity on hand after the upload
	CurrentWH   string   `json:"current_warehouse,omitempty"`
	IsNew       bool     `json:"is_new"`
	Status      string   `json:"status"` // ok | code_not_found | other_warehouse | invalid_qty | duplicate
	Message     string   `json:"message,omitempty"`
	descStore   string
	itemName    string
	description string
}

type whUploadPlan struct {
	Mode      string
	Warehouse string
	Rows      []whUploadRow
	Problems  []string // file-level parse errors plus one line per rejected row
}

func (p whUploadPlan) ok() bool {
	if len(p.Problems) > 0 {
		return false
	}
	for _, r := range p.Rows {
		if r.Status != "ok" {
			return false
		}
	}
	return len(p.Rows) > 0
}

// buildWarehouseUploadPlan parses the file and works out, per row, what the upload would do. Read-only.
func (h *StockItemHandler) buildWarehouseUploadPlan(c *fiber.Ctx) (*whUploadPlan, error) {
	mode := strings.ToLower(strings.TrimSpace(c.FormValue("mode")))
	if mode != whUploadModeUpdate && mode != whUploadModeAdd {
		return nil, fiber.NewError(fiber.StatusBadRequest, "mode ต้องเป็น update หรือ add")
	}
	wh := strings.TrimSpace(c.FormValue("warehouse_code"))
	if wh == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "กรุณาเลือกคลัง")
	}
	ctx := context.Background()
	var whOK bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM warehouse WHERE warehouse_code=$1 AND is_active=true)`, wh).Scan(&whOK); err != nil {
		return nil, err
	}
	if !whOK {
		return nil, fiber.NewError(fiber.StatusBadRequest, "ไม่พบคลังที่เลือก")
	}

	rows, err := readStockItemExcelUpload(c)
	if err != nil {
		return nil, err
	}
	parsed, parseErrs := parseStockItemExcelRows(rows)
	plan := &whUploadPlan{Mode: mode, Warehouse: wh, Problems: parseErrs}

	matCodes := uniqueMatCodes(parsed)
	master, err := fetchStockItemMaterialMaster(ctx, h.db, matCodes)
	if err != nil {
		return nil, err
	}

	type cur struct {
		qty float64
		wh  string
	}
	existing := map[string]cur{}
	if len(matCodes) > 0 {
		dbRows, err := h.db.Query(ctx, `SELECT mat_code, qty, warehouse_code FROM stock_item WHERE mat_code = ANY($1)`, matCodes)
		if err != nil {
			return nil, err
		}
		for dbRows.Next() {
			var code, w string
			var q float64
			if err := dbRows.Scan(&code, &q, &w); err != nil {
				dbRows.Close()
				return nil, err
			}
			existing[code] = cur{qty: q, wh: w}
		}
		dbRows.Close()
	}

	seen := map[string]int{}
	for _, p := range parsed {
		r := whUploadRow{
			RowNo: p.RowNo, MatCode: p.MatCode, Unit: p.Unit, UnitCost: p.UnitCost, FileQty: p.Qty,
			descStore: p.DescriptionStore, Status: "ok",
		}
		bad := func(status, msg string) {
			r.Status, r.Message = status, msg
			plan.Problems = append(plan.Problems, fmt.Sprintf("row %d: %s", p.RowNo, msg))
		}

		mi, found := master[p.MatCode]
		if found {
			r.MatName = strings.TrimSpace(deref(mi.MatName) + " " + deref(mi.SpecDescription))
			r.itemName = deref(mi.MatName)
			r.description = mi.composedDescription()
			if r.Unit == "" {
				r.Unit = deref(mi.UnitName)
			}
		}
		e, inStock := existing[p.MatCode]
		if inStock {
			q := e.qty
			r.CurrentQty = &q
			r.CurrentWH = e.wh
		}
		r.IsNew = !inStock

		firstRow, dup := seen[p.MatCode]
		switch {
		case dup:
			bad("duplicate", fmt.Sprintf("รหัสวัสดุ %s ซ้ำในไฟล์ (ซ้ำกับแถว %d)", p.MatCode, firstRow))
		case !found:
			bad("code_not_found", fmt.Sprintf("ไม่พบรหัสวัสดุ %s ในข้อมูลหลัก", p.MatCode))
		case inStock && e.wh != wh:
			bad("other_warehouse", fmt.Sprintf("%s อยู่ในคลัง %s ไม่ใช่คลัง %s ที่เลือก", p.MatCode, e.wh, wh))
		case mode == whUploadModeAdd && p.Qty <= 0:
			bad("invalid_qty", fmt.Sprintf("%s: จำนวนที่เพิ่มต้องมากกว่า 0", p.MatCode))
		case p.Qty < 0 || math.IsNaN(p.Qty):
			bad("invalid_qty", fmt.Sprintf("%s: จำนวนต้องไม่ติดลบ", p.MatCode))
		}
		if !dup {
			seen[p.MatCode] = p.RowNo
		}

		base := 0.0
		if r.CurrentQty != nil {
			base = *r.CurrentQty
		}
		if mode == whUploadModeAdd {
			r.NewQty = base + p.Qty
		} else {
			r.NewQty = p.Qty
		}
		plan.Rows = append(plan.Rows, r)
	}
	if len(parsed) == 0 && len(parseErrs) == 0 {
		plan.Problems = append(plan.Problems, "ไม่พบข้อมูลในไฟล์ (ข้อมูลเริ่มที่แถวที่ 4)")
	}
	return plan, nil
}

// PreviewWarehouseUpload godoc
// @Summary      Warehouse stock upload — preview (writes nothing)
// @Description  multipart: file (stock items template), mode = update|add, warehouse_code. Returns every row with the current and resulting quantity and a status (ok, code_not_found, other_warehouse, invalid_qty, duplicate), plus a summary. can_apply is true only when every row is ok.
// @Tags         Stock
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Router       /stock/warehouse-upload/preview [post]
func (h *StockItemHandler) PreviewWarehouseUpload(c *fiber.Ctx) error {
	plan, err := h.buildWarehouseUploadPlan(c)
	if err != nil {
		return err
	}
	newItems := 0
	for _, r := range plan.Rows {
		if r.IsNew && r.Status == "ok" {
			newItems++
		}
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"mode": plan.Mode, "warehouse_code": plan.Warehouse, "rows": plan.Rows,
		"summary":   fiber.Map{"total": len(plan.Rows), "new_items": newItems, "problems": len(plan.Problems)},
		"problems":  plan.Problems,
		"can_apply": plan.ok(),
	}})
}

// ApplyWarehouseUpload godoc
// @Summary      Warehouse stock upload — apply (Update overwrites, Add adds on top)
// @Description  Same request as the preview. All-or-nothing: if any row has a problem nothing is written and the problems are returned with 400. Writes stock_item and stock_transaction (ref_doc_type BULK_UPLOAD) in one transaction.
// @Tags         Stock
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Router       /stock/warehouse-upload/apply [post]
func (h *StockItemHandler) ApplyWarehouseUpload(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	plan, err := h.buildWarehouseUploadPlan(c)
	if err != nil {
		return err
	}
	if !plan.ok() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false, "error": "ไฟล์มีรายการที่ไม่ถูกต้อง ยังไม่ได้บันทึกอะไร", "problems": plan.Problems,
		})
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	remark := "Bulk upload (" + strings.ToUpper(plan.Mode) + ")"
	created, updated := 0, 0
	for _, r := range plan.Rows {
		var itemID int64
		var loc string
		var before float64
		err := tx.QueryRow(ctx, `SELECT id, qty, location_code FROM stock_item WHERE mat_code=$1 FOR UPDATE`, r.MatCode).Scan(&itemID, &before, &loc)
		isNew := err == pgx.ErrNoRows
		if err != nil && !isNew {
			return err
		}

		after := r.FileQty
		if plan.Mode == whUploadModeAdd && !isNew {
			after = before + r.FileQty // recomputed from the locked row, not from the preview
		}
		if isNew {
			before = 0
			if err := tx.QueryRow(ctx, `
				INSERT INTO stock_item (mat_code, item_name, description, description_store, item_type, unit, qty, unit_cost, warehouse_code)
				VALUES ($1,$2,$3,$4,'CONSUMABLE',$5,$6,$7,$8)
				RETURNING id, location_code`,
				r.MatCode, r.itemName, r.description, nullableCell(r.descStore), r.Unit, after, r.UnitCost, plan.Warehouse,
			).Scan(&itemID, &loc); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("row %d: บันทึกไม่สำเร็จ", r.RowNo))
			}
			created++
		} else {
			if plan.Mode == whUploadModeAdd {
				_, err = tx.Exec(ctx, `UPDATE stock_item SET qty=$1, updated_at=NOW() WHERE id=$2`, after, itemID)
			} else {
				_, err = tx.Exec(ctx, `
					UPDATE stock_item SET qty=$1, unit=$2, unit_cost=$3, description_store=$4, updated_at=NOW() WHERE id=$5`,
					after, r.Unit, r.UnitCost, nullableCell(r.descStore), itemID)
			}
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("row %d: บันทึกไม่สำเร็จ", r.RowNo))
			}
			updated++
		}

		// movement history: only when the quantity actually changed
		delta := after - before
		if math.Abs(delta) < 1e-9 {
			continue
		}
		txnType, qty := "IN", delta
		switch {
		case plan.Mode == whUploadModeUpdate && !isNew && delta > 0:
			txnType = "ADJUST_PLUS"
		case plan.Mode == whUploadModeUpdate && !isNew && delta < 0:
			txnType, qty = "ADJUST_MINUS", -delta
		}
		var seq int64
		if err := tx.QueryRow(ctx, `SELECT nextval('stock_txn_seq')`).Scan(&seq); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_transaction
			    (txn_no, txn_type, item_id, to_location, from_location, qty, ref_doc_type, remarks, qty_before, qty_after, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,'BULK_UPLOAD',$7,$8,$9,$10)`,
			fmt.Sprintf("TXN-%s-%04d", timeYYMM(), seq), txnType, itemID,
			nullIf(txnType != "ADJUST_MINUS", loc), nullIf(txnType == "ADJUST_MINUS", loc),
			qty, remark, before, after, claims.UserID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("row %d: บันทึกประวัติไม่สำเร็จ", r.RowNo))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
		"mode": plan.Mode, "warehouse_code": plan.Warehouse, "rows": len(plan.Rows), "created": created, "updated": updated,
	}})
}

// nullIf returns &v when cond is true, else nil (for nullable text columns).
func nullIf(cond bool, v string) *string {
	if !cond {
		return nil
	}
	return &v
}

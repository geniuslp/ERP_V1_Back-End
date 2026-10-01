package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// order_type values for purchase_request / purchase_order (varchar(30), DB CHECK allows
// exactly these five). 'stock' and 'cost' go through the IC receiving flow; the three
// asset/office types have NO goods-receiving process and must never appear in IC.
const (
	OrderTypeStock           = "stock"
	OrderTypeCost            = "cost"
	OrderTypeAssetEquipment  = "asset_equipment"
	OrderTypeOfficeEquipment = "office_equipment"
	OrderTypeAssetTool       = "asset_tool"
)

// AllowedOrderTypes is the single source of truth for valid order_type values.
var AllowedOrderTypes = []string{
	OrderTypeStock, OrderTypeCost, OrderTypeAssetEquipment, OrderTypeOfficeEquipment, OrderTypeAssetTool,
}

// dbJobCodes mirrors the live CHECK on purchase_request.job_code / purchase_order.job_code
// exactly: MP, ME, MS, MF, MG, MH, FS, FP, FB, DE, RE, G. Deliberately separate from JobTypes
// (job_type.go), whose list has "OH" where the DB has "G" — left untouched so stock/cost
// behavior doesn't change.
var dbJobCodes = map[string]bool{
	"MP": true, "ME": true, "MS": true, "MF": true, "MG": true, "MH": true,
	"FS": true, "FP": true, "FB": true, "DE": true, "RE": true, "G": true,
}

// DefaultAssetJobCode is the job_code default for the 3 asset/office types: 'G' (General), the
// value the DB CHECK allows. 'OH' is only the cost_subject code used by requireOHCostSubgroup —
// never a valid job_code.
const DefaultAssetJobCode = "G"

// ValidateAssetJobCode rejects any job_code outside the 12 DB-allowed values with a Thai 400, so
// the client never sees a raw *_job_code_check violation. Asset/office branches only.
func ValidateAssetJobCode(code string) error {
	if !dbJobCodes[code] {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("job_code %q ไม่ถูกต้อง ต้องเป็นหนึ่งใน: MP, ME, MS, MF, MG, MH, FS, FP, FB, DE, RE, G", code))
	}
	return nil
}

const orderTypeErrMsg = "order_type ต้องเป็นหนึ่งใน: stock, cost, asset_equipment, office_equipment, asset_tool"

// IsValidOrderType reports whether v is one of the 5 allowed order_type values.
func IsValidOrderType(v string) bool {
	for _, a := range AllowedOrderTypes {
		if v == a {
			return true
		}
	}
	return false
}

// IsAssetOrOfficeType is true only for the 3 non-IC types (asset_equipment, office_equipment,
// asset_tool).
func IsAssetOrOfficeType(v string) bool {
	return v == OrderTypeAssetEquipment || v == OrderTypeOfficeEquipment || v == OrderTypeAssetTool
}

// validateOrderType returns a 400 with a Thai message when v is not one of the 5 values.
func validateOrderType(v string) error {
	if !IsValidOrderType(v) {
		return fiber.NewError(fiber.StatusBadRequest, orderTypeErrMsg)
	}
	return nil
}

// RequireICPO is route middleware for every /ic/pos/:poId/* endpoint: POs whose order_type is not
// 'stock'/'cost' (i.e. the asset/office types, which have no goods receiving) are rejected with a
// 400 before the real handler — and therefore any DB write — runs. An unknown PO id falls through
// so the handler keeps returning its own existing 404/400.
func (h *ICHandler) RequireICPO(c *fiber.Ctx) error {
	poID, err := strconv.ParseInt(c.Params("poId"), 10, 64)
	if err != nil {
		return c.Next()
	}
	var orderType string
	err = h.db.QueryRow(context.Background(), `SELECT order_type FROM purchase_order WHERE id = $1`, poID).Scan(&orderType)
	if err == pgx.ErrNoRows {
		return c.Next()
	}
	if err != nil {
		return err
	}
	if orderType != OrderTypeStock && orderType != OrderTypeCost {
		return fiber.NewError(fiber.StatusBadRequest, "PO ประเภทนี้ไม่มีขั้นตอนรับของ จึงไม่อยู่ในระบบ Inventory Control (IC)")
	}
	return c.Next()
}

type pgRowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// requireOHCostSubgroup enforces that a PR/PO line of an asset/office order_type carries a
// cost_subgroup_id under cost_subject.subject_code = 'OH'. The check goes all the way up to
// cost_subject — cost_job.job_code = 'G' also exists under subjects L/M/S, so job_code alone
// does not identify OH. label is e.g. "lines[2]" for the error message.
func requireOHCostSubgroup(ctx context.Context, q pgRowQuerier, id *int64, label string) error {
	if id == nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: ต้องระบุรหัสต้นทุน (cost code) ของกลุ่ม OH/General สำหรับ order_type นี้", label))
	}
	var ok bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS(
		  SELECT 1 FROM cost_subgroup sg
		  JOIN cost_group g ON g.id = sg.group_id
		  JOIN cost_job j ON j.id = g.job_id
		  JOIN cost_subject sub ON sub.id = j.subject_id
		  WHERE sg.id = $1 AND sub.subject_code = 'OH')`, *id,
	).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: รหัสต้นทุนต้องเป็นกลุ่ม OH/General เท่านั้นสำหรับ order_type นี้", label))
	}
	return nil
}

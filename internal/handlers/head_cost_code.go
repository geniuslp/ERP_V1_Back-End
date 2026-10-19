package handlers

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// Head (cost_subgroup.is_head) and Major (is_major) cost codes are roll-up nodes: never selectable on a document.
// requireNonHeadCostCodes is the single check used by every input path (PR, PO, petty cash, IC
// movement lines, ...). Only NEW or CHANGED values are rejected: a code already stored on the same
// document stays valid even if it later became Head, so old documents remain editable unchanged.

// storedCostColumns whitelists (table, document fk column, cost column) for storedCostSubgroupIDs.
var storedCostColumns = map[string][2]string{
	"purchase_request_line":       {"pr_id", "cost_subgroup_id"},
	"purchase_order_line":         {"po_id", "cost_subgroup_id"},
	"petty_cash_requisition_line": {"pc_id", "cost_subgroup_id"},
	"ic_project_movement_line:to": {"movement_id", "to_cost_subgroup_id"},
}

// storedCostSubgroupIDs returns the cost_subgroup ids currently stored on the document's lines.
// table is a key of storedCostColumns (a ":to" suffix only disambiguates the movement destination).
func storedCostSubgroupIDs(ctx context.Context, q pgQuerier, table string, docID int64) ([]int64, error) {
	cols, ok := storedCostColumns[table]
	if !ok {
		return nil, fmt.Errorf("storedCostSubgroupIDs: table %q not whitelisted", table)
	}
	realTable := table
	if i := indexByte(table, ':'); i >= 0 {
		realTable = table[:i]
	}
	rows, err := q.Query(ctx, fmt.Sprintf(`SELECT DISTINCT %s FROM %s WHERE %s = $1 AND %s IS NOT NULL`, cols[1], realTable, cols[0], cols[1]), docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// requireNonHeadCostCodes returns a 400 (plain Thai) if any id in newIDs (nil / <=0 ignored) that is
// not already in stored refers to a Head subgroup. Call before any write.
func requireNonHeadCostCodes(ctx context.Context, q pgQuerier, newIDs []*int64, stored []int64) error {
	skip := make(map[int64]bool, len(stored))
	for _, id := range stored {
		skip[id] = true
	}
	seen := map[int64]bool{}
	var cand []int64
	for _, p := range newIDs {
		if p == nil || *p <= 0 || skip[*p] || seen[*p] {
			continue
		}
		seen[*p] = true
		cand = append(cand, *p)
	}
	if len(cand) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, `
		SELECT `+costCodeSQL("sub", "j", "g", "sg")+`, sg.is_major
		FROM cost_subgroup sg
		JOIN cost_group g ON g.id = sg.group_id
		JOIN cost_job j ON j.id = g.job_id
		JOIN cost_subject sub ON sub.id = j.subject_id
		WHERE sg.id = ANY($1) AND (sg.is_head OR sg.is_major)
		ORDER BY sg.id LIMIT 1`, cand)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		var code string
		var major bool
		if err := rows.Scan(&code, &major); err != nil {
			return err
		}
		if major {
			return fiber.NewError(fiber.StatusBadRequest,
				fmt.Sprintf("Cost Code นี้เป็น Major ไม่สามารถเลือกใช้ได้ กรุณาเลือกรายการย่อย (%s)", code))
		}
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("Cost Code นี้เป็น Head ไม่สามารถเลือกใช้ได้ กรุณาเลือกรายการย่อย (%s)", code))
	}
	return rows.Err()
}

// ptrID is a tiny helper for callers holding plain int64 ids.
func ptrID(v int64) *int64 { return &v }

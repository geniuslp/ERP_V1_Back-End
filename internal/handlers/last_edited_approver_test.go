package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"erp-api/internal/auth"
	"erp-api/internal/middleware"
)

// Integration test (dev DB, skipped when unreachable): approving never rewrites updated_by, and the PO / WO lists
// show the last editor (A) and the approver (B). "Edited by A" is set up directly (updated_by = A is what PO Update /
// WO Update write); the PO approval goes through the real config-driven route.
func TestLastEditedByAndApproverNames(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx, `SELECT id, full_name FROM users WHERE is_active ORDER BY id LIMIT 2`)
	if err != nil {
		t.Skipf("users: %v", err)
	}
	var ids []int64
	var names []string
	for rows.Next() {
		var id int64
		var n string
		_ = rows.Scan(&id, &n)
		ids, names = append(ids, id), append(names, n)
	}
	rows.Close()
	if len(ids) < 2 || names[0] == names[1] {
		t.Skip("need two distinct users")
	}
	a, b, aName, bName := ids[0], ids[1], names[0], names[1]

	clean := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM approval_request WHERE doc_no LIKE 'ZZ-%'`)
		_, _ = pool.Exec(ctx, `DELETE FROM po_status_log WHERE po_id IN (SELECT id FROM purchase_order WHERE po_no LIKE 'ZZ-%')`)
		_, _ = pool.Exec(ctx, `DELETE FROM purchase_order WHERE po_no LIKE 'ZZ-%'`)
		_, _ = pool.Exec(ctx, `DELETE FROM work_order_status_log WHERE wo_id IN (SELECT id FROM work_order WHERE wo_no LIKE 'ZZ-%')`)
		_, _ = pool.Exec(ctx, `DELETE FROM work_order WHERE wo_no LIKE 'ZZ-%'`)
	}
	clean()
	t.Cleanup(clean)

	ins := func(q string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, q, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	// Created and last edited by A, approver chosen = B, waiting for approval.
	poID := ins(`INSERT INTO purchase_order (po_no, job_code, created_by, updated_by, approver_id, status)
		VALUES ('ZZ-PO-1','MS',$1,$1,$2,'PENDING_APPROVAL') RETURNING id`, a, b)
	poNull := ins(`INSERT INTO purchase_order (po_no, job_code, created_by, updated_by, approver_id, status)
		VALUES ('ZZ-PO-2','MS',$1,$1,NULL,'PENDING_APPROVAL') RETURNING id`, a)
	woID := ins(`INSERT INTO work_order (wo_no, work_system, employer_name, supplier_name, created_by, updated_by, approver_id, status)
		VALUES ('ZZ-WO-1', ARRAY['MS'],'ZZ','ZZ',$1,$1,$2,'PENDING_APPROVAL') RETURNING id`, a, b)
	if _, err := pool.Exec(ctx, `INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status)
		VALUES ('PO',$1,'ZZ-PO-1',1,$2,$3,'PENDING')`, poID, a, b); err != nil {
		t.Fatal(err)
	}

	as := a
	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}
		return c.Status(code).JSON(fiber.Map{"success": false, "error": err.Error()})
	}})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.ClaimsKey, &auth.Claims{UserID: as, Username: "zz"})
		return c.Next()
	})
	app.Get("/po", NewPOHandler(pool).List)
	app.Get("/wo", NewWorkOrderHandler(pool).List)
	app.Put("/approval/:docType/:docId/approve", NewGenericApprovalHandler(pool).Approve)

	do := func(method, path string, body any) (int, []byte) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			j, _ := json.Marshal(body)
			rd = bytes.NewReader(j)
		}
		req := httptest.NewRequest(method, path, rd)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, out
	}
	findRow := func(path, key string, id int64) map[string]any {
		t.Helper()
		_, body := do("GET", path+"?page_size=100", nil)
		var r struct {
			Data struct {
				Data []map[string]any `json:"data"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &r)
		for _, row := range r.Data.Data {
			if v, ok := row[key].(float64); ok && int64(v) == id {
				return row
			}
		}
		t.Fatalf("%s: row %d not in list", path, id)
		return nil
	}
	str := func(m map[string]any, k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	updatedBy := func(tbl string, id int64) int64 {
		var u int64
		_ = pool.QueryRow(ctx, fmt.Sprintf(`SELECT updated_by FROM %s WHERE id=$1`, tbl), id).Scan(&u)
		return u
	}

	// before approval: the approver column already shows the assigned approver
	row := findRow("/po", "po_id", poID)
	t.Logf("PO list before approve: updated_by_name=%v approver_name=%v approved_by_name=%v", row["updated_by_name"], row["approver_name"], row["approved_by_name"])
	if str(row, "updated_by_name") != aName || str(row, "approver_name") != bName || row["approved_by_name"] != nil {
		t.Errorf("PO before approve: %v", row)
	}
	wrow := findRow("/wo", "id", woID)
	if str(wrow, "updated_by_name") != aName || str(wrow, "approver_name") != bName {
		t.Errorf("WO before approve: %v", wrow)
	}

	// B approves through the real config-driven route
	as = b
	if code, body := do("PUT", fmt.Sprintf("/approval/PO/%d/approve", poID), map[string]any{}); code != 200 {
		t.Fatalf("approve PO: %d %s", code, body)
	}
	if u := updatedBy("purchase_order", poID); u != a {
		t.Errorf("PO updated_by changed by approve: %d (want %d)", u, a)
	}
	as = a
	row = findRow("/po", "po_id", poID)
	t.Logf("PO list after approve: updated_by_name=%v approver_name=%v approved_by_name=%v approved_at=%v", row["updated_by_name"], row["approver_name"], row["approved_by_name"], row["approved_at"])
	if str(row, "updated_by_name") != aName || str(row, "approver_name") != bName || str(row, "approved_by_name") != bName || row["approved_at"] == nil {
		t.Errorf("PO after approve: %v", row)
	}

	// legacy NULL approver: falls back to the user who actually approved (status log)
	if _, err := pool.Exec(ctx, `UPDATE purchase_order SET status='APPROVED' WHERE id=$1`, poNull); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO po_status_log (po_id, from_status, to_status, changed_by) VALUES ($1,'PENDING_APPROVAL','APPROVED',$2)`, poNull, b); err != nil {
		t.Fatal(err)
	}
	row = findRow("/po", "po_id", poNull)
	if str(row, "approver_name") != bName || str(row, "updated_by_name") != aName {
		t.Errorf("PO NULL approver fallback: %v", row)
	}

	// WO: the config-driven route has no WO doc type on dev, so record the approval the way it would
	// (status + status-log row by B); updated_by is not touched.
	if _, err := pool.Exec(ctx, `UPDATE work_order SET status='APPROVED' WHERE id=$1`, woID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO work_order_status_log (wo_id, from_status, to_status, changed_by) VALUES ($1,'PENDING_APPROVAL','APPROVED',$2)`, woID, b); err != nil {
		t.Fatal(err)
	}
	wrow = findRow("/wo", "id", woID)
	t.Logf("WO list after approve: updated_by_name=%v approver_name=%v approved_by_name=%v", wrow["updated_by_name"], wrow["approver_name"], wrow["approved_by_name"])
	if str(wrow, "updated_by_name") != aName || str(wrow, "approver_name") != bName || str(wrow, "approved_by_name") != bName || updatedBy("work_order", woID) != a {
		t.Errorf("WO after approve: %v", wrow)
	}
}

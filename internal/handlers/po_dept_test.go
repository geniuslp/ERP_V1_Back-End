package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"

	"erp-api/internal/auth"
	"erp-api/internal/middleware"
)

// Integration test (dev DB, skipped when unreachable): purchase_order.dept_code is saved by POST/PUT /po, returned by
// GET /po/:id and GET /po, taken from the linked PR when it has one, and used by the dept_code list filter.
func TestPODeptCode(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()

	var userID int64
	var supplierID int64
	var matCode, whProject string
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE is_active ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
		t.Skipf("users: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM supplier ORDER BY id LIMIT 1`).Scan(&supplierID); err != nil {
		t.Skipf("supplier: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT mat_code FROM material_code ORDER BY mat_code LIMIT 1`).Scan(&matCode); err != nil {
		t.Skipf("material_code: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT project_code FROM project WHERE COALESCE(warehouse_code,'') <> '' ORDER BY project_code LIMIT 1`).Scan(&whProject); err != nil {
		t.Skipf("warehouse project: %v", err)
	}
	var deptA, deptB string
	rows, err := pool.Query(ctx, `SELECT dept_code FROM departments WHERE is_active ORDER BY sort_order, dept_code LIMIT 2`)
	if err != nil {
		t.Skipf("departments: %v", err)
	}
	var depts []string
	for rows.Next() {
		var d string
		_ = rows.Scan(&d)
		depts = append(depts, d)
	}
	rows.Close()
	if len(depts) < 2 {
		t.Skip("need two departments")
	}
	deptA, deptB = depts[0], depts[1]

	var createdPOs []int64
	var prID int64
	clean := func() {
		for _, id := range createdPOs {
			_, _ = pool.Exec(ctx, `DELETE FROM approval_request WHERE doc_type='PO' AND doc_id=$1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM po_status_log WHERE po_id=$1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM purchase_order_line WHERE po_id=$1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM po_edit_log WHERE po_id=$1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM purchase_order WHERE id=$1`, id)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM purchase_request WHERE pr_no LIKE 'ZZ-%'`)
	}
	t.Cleanup(clean)

	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}
		return c.Status(code).JSON(fiber.Map{"success": false, "error": err.Error()})
	}})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.ClaimsKey, &auth.Claims{UserID: userID, Username: "zz"})
		return c.Next()
	})
	h := NewPOHandler(pool)
	app.Post("/po", h.Create)
	app.Put("/po/:id", h.Update)
	app.Get("/po/:id", h.Get)
	app.Get("/po", h.List)

	do := func(method, path string, body any) (int, map[string]any) {
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
		var m map[string]any
		_ = json.Unmarshal(out, &m)
		return resp.StatusCode, m
	}
	body := func(extra map[string]any) map[string]any {
		b := map[string]any{
			"supplier_id": supplierID, "location_text": "ZZ test", "approver_id": userID, "status": "DRAFT",
			"order_type": "stock", "project_code": whProject, "job_code": "MS",
			"lines": []map[string]any{{"mat_code": matCode, "qty_ordered": 1, "unit_price": 10}},
		}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	poIDOf := func(m map[string]any) int64 {
		d, _ := m["data"].(map[string]any)
		if v, ok := d["po_id"].(float64); ok {
			return int64(v)
		}
		return 0
	}
	getDept := func(id int64) (string, string) {
		_, m := do("GET", "/po/"+strconv.FormatInt(id, 10), nil)
		d, _ := m["data"].(map[string]any)
		c, _ := d["dept_code"].(string)
		n, _ := d["dept_name"].(string)
		return c, n
	}

	// 1) create without a PR, with a department the user typed
	code, m := do("POST", "/po", body(map[string]any{"dept_code": deptA}))
	if code != 201 && code != 200 {
		t.Fatalf("create: %d %v", code, m)
	}
	po1 := poIDOf(m)
	createdPOs = append(createdPOs, po1)
	if c, n := getDept(po1); c != deptA || n == "" {
		t.Errorf("after create GET /po/:id dept = %q (%q), want %q with a name", c, n, deptA)
	}
	var stored *string
	_ = pool.QueryRow(ctx, `SELECT dept_code FROM purchase_order WHERE id=$1`, po1).Scan(&stored)
	if stored == nil || *stored != deptA {
		t.Errorf("stored purchase_order.dept_code = %v, want %s", stored, deptA)
	}

	// 2) the list returns it and the dept filter finds it
	_, lm := do("GET", "/po?page_size=100&dept_code="+deptA, nil)
	found := false
	if d, ok := lm["data"].(map[string]any); ok {
		if arr, ok := d["data"].([]any); ok {
			for _, it := range arr {
				row, _ := it.(map[string]any)
				if int64(row["po_id"].(float64)) == po1 {
					found = true
					if row["dept_code"] != deptA {
						t.Errorf("list row dept_code = %v", row["dept_code"])
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("GET /po?dept_code=%s did not return the PO", deptA)
	}

	// 3) update to another department, then clear it
	if code, m := do("PUT", "/po/"+strconv.FormatInt(po1, 10), body(map[string]any{"dept_code": deptB})); code != 200 {
		t.Fatalf("update: %d %v", code, m)
	}
	if c, _ := getDept(po1); c != deptB {
		t.Errorf("after update dept = %q, want %q", c, deptB)
	}
	if code, m := do("PUT", "/po/"+strconv.FormatInt(po1, 10), body(nil)); code != 200 {
		t.Fatalf("update (clear): %d %v", code, m)
	}
	if c, _ := getDept(po1); c != "" {
		t.Errorf("after clearing dept = %q, want empty", c)
	}

	// 4) an unknown department code is rejected
	if code, _ := do("POST", "/po", body(map[string]any{"dept_code": "NO-SUCH-DEPT"})); code != 400 {
		t.Errorf("unknown dept_code: status %d, want 400", code)
	}

	// 5) PO linked to a COMPLETED PR that has a department: the PR's department wins over what the client sends
	if err := pool.QueryRow(ctx, `INSERT INTO purchase_request (pr_no, status, requested_by, job_code, order_type, project_code, warehouse_code, dept_code, created_by)
		VALUES ('ZZ-PR-1','COMPLETED',$1,'MS','stock',$2::varchar,(SELECT warehouse_code FROM project WHERE project_code=$2::varchar),$3,$1) RETURNING id`, userID, whProject, deptA).Scan(&prID); err != nil {
		t.Fatalf("insert PR: %v", err)
	}
	code, m = do("POST", "/po", body(map[string]any{"pr_id": prID, "dept_code": deptB}))
	if code != 201 && code != 200 {
		t.Fatalf("create from PR: %d %v", code, m)
	}
	po2 := poIDOf(m)
	createdPOs = append(createdPOs, po2)
	if c, _ := getDept(po2); c != deptA {
		t.Errorf("PO from PR: dept = %q, want the PR's %q", c, deptA)
	}

	// 6) PO linked to a PR WITHOUT a department: the client's choice is kept
	_, _ = pool.Exec(ctx, `UPDATE purchase_request SET dept_code = NULL WHERE id=$1`, prID)
	code, m = do("POST", "/po", body(map[string]any{"pr_id": prID, "dept_code": deptB}))
	if code != 201 && code != 200 {
		t.Fatalf("create from PR without dept: %d %v", code, m)
	}
	po3 := poIDOf(m)
	createdPOs = append(createdPOs, po3)
	if c, _ := getDept(po3); c != deptB {
		t.Errorf("PO from PR without dept: dept = %q, want the client's %q", c, deptB)
	}
}

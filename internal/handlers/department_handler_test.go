package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Integration test (dev DB, skipped when unreachable) for the department master CRUD. Synthetic codes start with ZZ_
// and are removed afterwards. The menu-permission middleware is not part of this test (it needs the menu row).
func TestDepartmentCRUD(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()
	clean := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE username LIKE 'zz_dept_%'`)
		_, _ = pool.Exec(ctx, `DELETE FROM departments WHERE dept_code LIKE 'ZZ\_%'`)
	}
	clean()
	t.Cleanup(clean)

	h := NewDepartmentHandler(pool)
	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}
		return c.Status(code).JSON(fiber.Map{"success": false, "error": err.Error()})
	}})
	app.Get("/departments", h.List)
	app.Post("/departments", h.Create)
	app.Put("/departments/:id", h.Update)
	app.Delete("/departments/:id", h.Delete)

	do := func(method, path string, body any) (int, string) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, rd)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	idOf := func(body string) int64 {
		var r struct {
			Data struct {
				ID int64 `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal([]byte(body), &r)
		return r.Data.ID
	}
	row := func(code string) string {
		var id, so int64
		var name string
		var active bool
		if err := pool.QueryRow(ctx, `SELECT id, dept_name, sort_order, is_active FROM departments WHERE dept_code=$1`, code).Scan(&id, &name, &so, &active); err != nil {
			return "(no row)"
		}
		b, _ := json.Marshal(map[string]any{"id": id, "dept_code": code, "dept_name": name, "sort_order": so, "is_active": active})
		return string(b)
	}
	want := func(label string, code int, body string, wantCode int, contains string) {
		t.Helper()
		if code != wantCode || (contains != "" && !strings.Contains(body, contains)) {
			t.Errorf("%s: got %d %s (want %d %q)", label, code, body, wantCode, contains)
		}
	}

	// create
	code, body := do("POST", "/departments", map[string]any{"dept_code": " zz_test1 ", "dept_name": " ZZ Test ", "sort_order": 50})
	want("create ok", code, body, 201, "ZZ_TEST1")
	id := idOf(body)
	t.Logf("SELECT after create: %s", row("ZZ_TEST1"))
	code, body = do("POST", "/departments", map[string]any{"dept_code": "zz_test1", "dept_name": "Dup"})
	want("duplicate", code, body, 409, "รหัสแผนกนี้มีอยู่แล้ว")
	for _, bad := range []string{"ZZ-1", "ZZ 1", "รหัส", strings.Repeat("A", 21), ""} {
		code, body = do("POST", "/departments", map[string]any{"dept_code": bad, "dept_name": "x"})
		want("bad code "+bad, code, body, 400, "")
	}
	code, body = do("POST", "/departments", map[string]any{"dept_code": "ZZ_NONAME", "dept_name": "   "})
	want("empty name", code, body, 400, "กรุณาระบุชื่อแผนก")
	code, body = do("POST", "/departments", map[string]any{"dept_code": "ZZ_NEG", "dept_name": "x", "sort_order": -1})
	want("negative sort", code, body, 400, "ลำดับการแสดง")
	code, body = do("POST", "/departments", map[string]any{"dept_code": "ZZ_NAME", "dept_name": strings.Repeat("ก", 201)})
	want("name too long", code, body, 400, "ชื่อแผนกต้องไม่เกิน")
	if r := row("ZZ_NEG"); r != "(no row)" {
		t.Errorf("rejected create left a row: %s", r)
	}

	// update
	path := "/departments/" + itoa(id)
	code, body = do("PUT", path, map[string]any{"dept_name": "ZZ Renamed", "sort_order": 7, "is_active": false})
	want("update", code, body, 200, "ZZ Renamed")
	t.Logf("SELECT after update: %s", row("ZZ_TEST1"))
	code, body = do("PUT", path, map[string]any{"dept_name": "ZZ Renamed 2"})
	want("update keeps sort/active", code, body, 200, `"is_active":false`)
	code, body = do("PUT", path, map[string]any{"dept_code": "ZZ_OTHER", "dept_name": "x"})
	want("code change rejected", code, body, 400, "ไม่สามารถเปลี่ยนรหัสแผนกได้")
	code, body = do("PUT", path, map[string]any{"dept_code": "zz_test1", "dept_name": "ZZ Same Code"})
	want("same code accepted", code, body, 200, "ZZ Same Code")
	code, body = do("PUT", path, map[string]any{"dept_name": ""})
	want("update empty name", code, body, 400, "")
	code, body = do("PUT", "/departments/99999999", map[string]any{"dept_name": "x"})
	want("update missing id", code, body, 404, "ไม่พบแผนกนี้")
	_, listAll := do("GET", "/departments", nil)
	_, listActive := do("GET", "/departments?active_only=true", nil)
	if !strings.Contains(listAll, "ZZ_TEST1") || strings.Contains(listActive, "ZZ_TEST1") {
		t.Errorf("list: inactive department must show in the full list only")
	}

	// delete used (a user row) -> 409 listing the table
	code, body = do("POST", "/departments", map[string]any{"dept_code": "ZZ_USED", "dept_name": "ZZ Used"})
	want("create used", code, body, 201, "")
	usedID := idOf(body)
	if _, err := pool.Exec(ctx, `INSERT INTO users (username, full_name, dept_code) VALUES ('zz_dept_u1','ZZ U1','ZZ_USED'),('zz_dept_u2','ZZ U2','ZZ_USED')`); err != nil {
		t.Fatal(err)
	}
	code, body = do("DELETE", "/departments/"+itoa(usedID), nil)
	want("delete used", code, body, 409, "ผู้ใช้ 2")
	want("delete used message", code, body, 409, "กรุณาปิดการใช้งานแทน")
	t.Logf("delete used -> %d %s", code, body)
	if r := row("ZZ_USED"); r == "(no row)" {
		t.Errorf("a used department was deleted")
	}

	// delete unused
	code, body = do("DELETE", path, nil)
	want("delete unused", code, body, 200, "")
	t.Logf("SELECT after delete: %s", row("ZZ_TEST1"))
	code, body = do("DELETE", path, nil)
	want("delete again (missing)", code, body, 404, "ไม่พบแผนกนี้")
	code, body = do("DELETE", "/departments/99999999", nil)
	want("delete missing id", code, body, 404, "ไม่พบแผนกนี้")
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

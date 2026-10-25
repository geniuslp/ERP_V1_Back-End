package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"erp-api/internal/auth"
	"erp-api/internal/middleware"
)

// Integration test (dev DB via DATABASE_URL / ../../.env, skipped when unreachable) for the cost-code rules
// that apply when a petty cash document enters PENDING_APPROVAL. Synthetic rows are prefixed ZZ- and removed.

func TestPettyCashSubmitCostCodeRules(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()
	cbTestCleanup(pool)
	t.Cleanup(func() { cbTestCleanup(pool) })

	var head, major, item int64
	for q, dst := range map[string]*int64{
		`SELECT id FROM cost_subgroup WHERE is_head ORDER BY id LIMIT 1`:                      &head,
		`SELECT id FROM cost_subgroup WHERE is_major ORDER BY id LIMIT 1`:                     &major,
		`SELECT id FROM cost_subgroup WHERE NOT is_head AND NOT is_major ORDER BY id LIMIT 1`: &item,
	} {
		if err := pool.QueryRow(ctx, q).Scan(dst); err != nil {
			t.Skipf("no cost_subgroup fixture: %v", err)
		}
	}
	p := func(v int64) *int64 { return &v }

	// ── requireSubmittable directly ──
	cases := []struct {
		name  string
		lines []pcSubmitLine
		want  string // "" = ok, else a substring of the 400 message
	}{
		{"valid item code", []pcSubmitLine{{1, p(item)}, {2, p(item)}}, ""},
		{"missing code", []pcSubmitLine{{1, p(item)}, {2, nil}}, "บรรทัดที่ 2 ยังไม่ได้ระบุรหัสต้นทุน"},
		{"zero id counts as missing", []pcSubmitLine{{1, p(0)}}, "บรรทัดที่ 1 ยังไม่ได้ระบุรหัสต้นทุน"},
		{"head code", []pcSubmitLine{{1, p(head)}}, "บรรทัดที่ 1 ใช้รหัสต้นทุนที่เลือกไม่ได้ (Head/Major)"},
		{"major code", []pcSubmitLine{{1, p(item)}, {2, p(major)}}, "บรรทัดที่ 2 ใช้รหัสต้นทุนที่เลือกไม่ได้ (Head/Major)"},
		{"no lines", nil, pcLinesRequiredMsg},
	}
	for _, c := range cases {
		err := requireSubmittable(ctx, pool, "remark", c.lines)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
	if err := requireSubmittable(ctx, pool, "  ", []pcSubmitLine{{1, p(item)}}); err == nil || !strings.Contains(err.Error(), pcRemarksRequiredMsg) {
		t.Errorf("blank remarks: got %v", err)
	}

	// ── handlers ──
	h := NewPettyCashHandler(pool)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.ClaimsKey, &auth.Claims{UserID: 1, Username: "zz"})
		return c.Next()
	})
	app.Post("/petty-cash", h.Create)
	app.Post("/petty-cash/:id/submit", h.Submit)

	post := func(path string, body any) (int, string) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req := httptest.NewRequest("POST", path, rd)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	// Submit: DRAFT documents inserted directly (no approver, so a valid code stops at the approver check,
	// proving the cost-code gate passed without moving anything to PENDING_APPROVAL).
	draft := func(pcNo string, costs ...*int64) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO petty_cash_requisition (pc_no, pc_date, requested_by, created_by, status, doc_type, remarks, dept_code)
			VALUES ($1, CURRENT_DATE, 1, 1, 'DRAFT', 'PETTY_CASH', 'ZZ remark', 'ZZ') RETURNING id`, pcNo).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for i, c := range costs {
			if _, err := pool.Exec(ctx, `INSERT INTO petty_cash_requisition_line (pc_id, line_no, cost_subgroup_id, description, qty, unit_price, amount, line_net)
				VALUES ($1,$2,$3,'ZZ line',1,100,100,100)`, id, i+1, c); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	submit := func(id int64) (int, string) { return post(fmt.Sprintf("/petty-cash/%d/submit", id), nil) }

	if code, body := submit(draft("ZZ-PC-S1", p(item), nil)); code != 400 || !strings.Contains(body, "บรรทัดที่ 2 ยังไม่ได้ระบุรหัสต้นทุน") {
		t.Errorf("submit missing code: %d %s", code, body)
	}
	if code, body := submit(draft("ZZ-PC-S2", p(head))); code != 400 || !strings.Contains(body, "บรรทัดที่ 1 ใช้รหัสต้นทุนที่เลือกไม่ได้ (Head/Major)") {
		t.Errorf("submit head code: %d %s", code, body)
	}
	if code, body := submit(draft("ZZ-PC-S3", p(major))); code != 400 || !strings.Contains(body, "บรรทัดที่ 1 ใช้รหัสต้นทุนที่เลือกไม่ได้ (Head/Major)") {
		t.Errorf("submit major code: %d %s", code, body)
	}
	if code, body := submit(draft("ZZ-PC-S4", p(item))); code != 400 || !strings.Contains(body, "กรุณาเลือกผู้อนุมัติ") {
		t.Errorf("submit valid code should pass the cost-code gate and stop at the approver check: %d %s", code, body)
	}
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM petty_cash_requisition WHERE pc_no='ZZ-PC-S1'`).Scan(&status)
	if status != "DRAFT" {
		t.Errorf("rejected submit must leave the document DRAFT, got %s", status)
	}

	// Create with status=PENDING_APPROVAL: rejected before any write (no number is consumed).
	line := func(cost *int64) map[string]any {
		return map[string]any{"description": "ZZ line", "amount": 100, "cost_subgroup_id": cost}
	}
	pending := func(lines ...map[string]any) (int, string) {
		return post("/petty-cash", map[string]any{"status": "PENDING_APPROVAL", "approver_id": 1, "remarks": "ZZ", "dept_code": "ZZ", "lines": lines})
	}
	if code, body := pending(line(p(item)), line(nil)); code != 400 || !strings.Contains(body, "บรรทัดที่ 2 ยังไม่ได้ระบุรหัสต้นทุน") {
		t.Errorf("create pending missing code: %d %s", code, body)
	}
	if code, body := pending(line(p(head))); code != 400 || !strings.Contains(body, "บรรทัดที่ 1 ใช้รหัสต้นทุนที่เลือกไม่ได้ (Head/Major)") {
		t.Errorf("create pending head code: %d %s", code, body)
	}
	if code, body := pending(line(p(major))); code != 400 || !strings.Contains(body, "บรรทัดที่ 1 ใช้รหัสต้นทุนที่เลือกไม่ได้ (Head/Major)") {
		t.Errorf("create pending major code: %d %s", code, body)
	}

	// Create as DRAFT without a cost code stays allowed. It allocates a PC number; the counter is put
	// back afterwards (only if nothing else moved it) so dev keeps no gap.
	var ym string
	var before int
	_ = pool.QueryRow(ctx, `SELECT year_month, last_seq FROM pc_number_counter ORDER BY year_month DESC LIMIT 1`).Scan(&ym, &before)
	code, body := post("/petty-cash", map[string]any{"status": "DRAFT", "remarks": "ZZ", "dept_code": "ZZ", "lines": []map[string]any{line(nil)}})
	var created struct {
		Data struct {
			ID   int64  `json:"id"`
			PCNo string `json:"pc_no"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &created)
	if created.Data.ID != 0 {
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM petty_cash_requisition WHERE id=$1`, created.Data.ID)
			_, _ = pool.Exec(context.Background(), `UPDATE pc_number_counter SET last_seq=$2 WHERE year_month=$1 AND last_seq=$2+1`, ym, before)
		})
	}
	if code != 201 || created.Data.ID == 0 {
		t.Errorf("create DRAFT without cost code: %d %s", code, body)
	}
}

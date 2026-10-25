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

// Integration test (dev DB, skipped when unreachable): a PR created from a memo inherits memo.job_code
// when the request has none. Synthetic memo ZZ-MEMO-*; created PRs are deleted and the PR counter restored.
func TestPRCreateFromMemoJobCode(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()

	const project = "2026-XXX-004" // job_codes {MP,ME,MS,MF,MH,OH} on dev
	var mat string
	if err := pool.QueryRow(ctx, `SELECT mat_code FROM material_code LIMIT 1`).Scan(&mat); err != nil {
		t.Skipf("no material fixture: %v", err)
	}
	var ym string
	var before int
	_ = pool.QueryRow(ctx, `SELECT year_month, last_seq FROM pr_number_counter ORDER BY year_month DESC LIMIT 1`).Scan(&ym, &before)

	var prIDs []int64
	clean := func() {
		for _, id := range prIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM pr_status_log WHERE pr_id=$1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM purchase_request_line WHERE pr_id=$1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM purchase_request WHERE id=$1`, id)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM memo WHERE memo_no LIKE 'ZZ-MEMO-%'`)
		_, _ = pool.Exec(ctx, `UPDATE pr_number_counter SET last_seq=$2::int WHERE year_month=$1 AND last_seq=$2::int+$3::int`, ym, before, len(prIDs))
	}
	clean()
	t.Cleanup(clean)

	mkMemo := func(no string, job *string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO memo (memo_no, project_code, job_code, requested_by, status)
			VALUES ($1,$2,$3,1,'APPROVED') RETURNING id`, no, project, job).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	me, bad := "ME", "ZZ"
	memoME, memoBad, memoNone := mkMemo("ZZ-MEMO-1", &me), mkMemo("ZZ-MEMO-2", &bad), mkMemo("ZZ-MEMO-3", nil)

	h := NewPRHandler(pool)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.ClaimsKey, &auth.Claims{UserID: 1, Username: "zz"})
		return c.Next()
	})
	app.Post("/pr", h.Create)

	create := func(memoID int64, jobCode string) (int, string) {
		t.Helper()
		body := map[string]any{
			"pr_date": "2026-10-22", "requested_by": 1, "created_by": 1, "location_text": "ZZ",
			"project_code": project, "memo_id": memoID, "order_type": "cost", "job_code": jobCode,
			"lines": []map[string]any{{"line_no": 1, "mat_code": mat, "qty_requested": 1}},
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/pr", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		var r struct {
			Data struct {
				ID int64 `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(out, &r)
		if r.Data.ID != 0 {
			prIDs = append(prIDs, r.Data.ID)
		}
		return resp.StatusCode, string(out)
	}
	stored := func(body string) string {
		var r struct {
			Data struct {
				ID int64 `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal([]byte(body), &r)
		var jc, no string
		_ = pool.QueryRow(ctx, `SELECT job_code, pr_no FROM purchase_request WHERE id=$1`, r.Data.ID).Scan(&jc, &no)
		t.Logf("SELECT pr_no, job_code FROM purchase_request WHERE id=%d -> %s, %s", r.Data.ID, no, jc)
		return jc
	}

	// memo has ME, request sends none -> stored ME
	if code, body := create(memoME, ""); code != 201 || stored(body) != "ME" {
		t.Errorf("default from memo: %d %s", code, body)
	}
	// explicit job_code wins over the memo's
	if code, body := create(memoME, "MP"); code != 201 || stored(body) != "MP" {
		t.Errorf("explicit job_code: %d %s", code, body)
	}
	// memo job not allowed -> plain 400, nothing stored
	if code, body := create(memoBad, ""); code != 400 || strings.Contains(strings.ToLower(body), "sqlstate") {
		t.Errorf("memo with disallowed job: %d %s", code, body)
	} else {
		t.Log("disallowed:", body)
	}
	// memo without job and request without job -> 400 "job_code is required"
	if code, body := create(memoNone, ""); code != 400 {
		t.Errorf("no job anywhere: %d %s", code, body)
	}
	// job not in the project's job_codes -> 400
	if code, body := create(memoME, "FS"); code != 400 {
		t.Errorf("job outside project: %d %s", code, fmt.Sprint(body))
	}
}

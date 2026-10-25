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

// Integration test (dev DB, skipped when unreachable). Synthetic rows are named ZZ-* and removed afterwards.
// Covers: a NULL-approver pending document is actionable by nobody (list condition, badge count, approve/reject
// 403 in plain Thai), and the submit paths refuse a missing approver / unconfigured flow before changing status.

func TestApproverScopeAndSubmitGates(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()
	const me int64 = 1 // JWT user in the test

	clean := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM approval_request WHERE doc_no LIKE 'ZZ-%'`)
		_, _ = pool.Exec(ctx, `DELETE FROM memo_status_log WHERE memo_id IN (SELECT id FROM memo WHERE memo_no LIKE 'ZZ-%')`)
		_, _ = pool.Exec(ctx, `DELETE FROM memo WHERE memo_no LIKE 'ZZ-%'`)
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
	mkReq := func(docType string, id int64, no string) {
		t.Helper()
		// assigned_to deliberately points at the caller: only documents.approver_id may decide access.
		if _, err := pool.Exec(ctx, `INSERT INTO approval_request (doc_type, doc_id, doc_no, step_no, requested_by, assigned_to, status)
			VALUES ($1,$2,$3,1,$4,$4,'PENDING')`, docType, id, no, me); err != nil {
			t.Fatal(err)
		}
	}
	mkMemo := func(no string, approver *int64, status string) int64 {
		return ins(`INSERT INTO memo (memo_no, requested_by, approver_id, status) VALUES ($1,$2,$3,$4) RETURNING id`, no, me, approver, status)
	}
	mkPO := func(no string, approver *int64, status string) int64 {
		return ins(`INSERT INTO purchase_order (po_no, job_code, created_by, approver_id, status) VALUES ($1,'MP',$2,$3,$4) RETURNING id`, no, me, approver, status)
	}
	mkWO := func(no string, approver *int64, status string) int64 {
		return ins(`INSERT INTO work_order (wo_no, work_system, employer_name, supplier_name, created_by, approver_id, status)
			VALUES ($1, ARRAY['MP'], 'ZZ', 'ZZ', $2, $3, $4) RETURNING id`, no, me, approver, status)
	}
	meP := me

	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}
		return c.Status(code).JSON(fiber.Map{"success": false, "error": err.Error()})
	}})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.ClaimsKey, &auth.Claims{UserID: me, Username: "zz"})
		return c.Next()
	})
	gen := NewGenericApprovalHandler(pool)
	app.Put("/approval/:docType/:docId/approve", gen.Approve)
	app.Put("/approval/:docType/:docId/reject", gen.Reject)
	memoH := NewMemoHandler(pool)
	app.Post("/memo/:id/submit", memoH.Submit)
	app.Post("/memo/:id/approve", memoH.Approve)
	app.Get("/counts", NewApprovalHandler(pool).MyPendingCounts)
	woH := NewWorkOrderHandler(pool)
	app.Post("/work-order/:id/submit", woH.Submit)

	call := func(method, path string, body any) (int, string) {
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
	counts := func() (memo, po, wo int) {
		t.Helper()
		_, body := call("GET", "/counts", nil)
		var r struct {
			Data struct{ Memo, Po, Wo int } `json:"data"`
		}
		_ = json.Unmarshal([]byte(strings.NewReplacer(`"memo"`, `"Memo"`, `"po"`, `"Po"`, `"wo"`, `"Wo"`).Replace(body)), &r)
		return r.Data.Memo, r.Data.Po, r.Data.Wo
	}

	// ── 10. NULL-approver documents: actionable by nobody ──
	m0, p0, w0 := counts()
	memoNull, poNull, woNull := mkMemo("ZZ-M-NULL", nil, "PENDING_APPROVAL"), mkPO("ZZ-P-NULL", nil, "PENDING_APPROVAL"), mkWO("ZZ-W-NULL", nil, "PENDING_APPROVAL")
	mkReq("MEMO", memoNull, "ZZ-M-NULL")
	mkReq("PO", poNull, "ZZ-P-NULL")
	mkReq("WO", woNull, "ZZ-W-NULL")
	if m, p, w := counts(); m != m0 || p != p0 || w != w0 {
		t.Errorf("NULL-approver docs must not change the badge counts: before %d/%d/%d after %d/%d/%d", m0, p0, w0, m, p, w)
	}
	memoOK, poOK, woOK := mkMemo("ZZ-M-OK", &meP, "PENDING_APPROVAL"), mkPO("ZZ-P-OK", &meP, "PENDING_APPROVAL"), mkWO("ZZ-W-OK", &meP, "PENDING_APPROVAL")
	mkReq("MEMO", memoOK, "ZZ-M-OK")
	mkReq("PO", poOK, "ZZ-P-OK")
	mkReq("WO", woOK, "ZZ-W-OK")
	if m, p, w := counts(); m != m0+1 || p != p0+1 || w != w0+1 {
		t.Errorf("assigned docs must be counted: before %d/%d/%d after %d/%d/%d", m0, p0, w0, m, p, w)
	}

	for _, c := range []struct {
		doc          string
		null, assign int64
	}{{"MEMO", memoNull, memoOK}, {"PO", poNull, poOK}, {"WO", woNull, woOK}} {
		// approverMayAct (approve/reject eligibility) and the list/visibility condition
		claims := &auth.Claims{UserID: me}
		if ok, err := approverMayAct(ctx, pool, claims, c.doc, c.null); err != nil || ok {
			t.Errorf("%s NULL-approver doc: approverMayAct = %v, %v (want false)", c.doc, ok, err)
		}
		if ok, err := approverMayAct(ctx, pool, claims, c.doc, c.assign); err != nil || !ok {
			t.Errorf("%s assigned doc: approverMayAct = %v, %v (want true)", c.doc, ok, err)
		}
		cond, args := approvalListCond(claims, c.doc, "d", nil)
		var inList int
		q := fmt.Sprintf(`SELECT COUNT(*) FROM %s d WHERE d.id IN ($%d,$%d) AND %s`, approvalScopeDocs[c.doc].table, len(args)+1, len(args)+2, cond)
		if err := pool.QueryRow(ctx, q, append(args, c.null, c.assign)...).Scan(&inList); err != nil || inList != 1 {
			t.Errorf("%s list condition returned %d of {null, assigned} (want only the assigned one): %v", c.doc, inList, err)
		}
	}
	for _, doc := range []struct {
		t  string
		id int64
	}{{"MEMO", memoNull}, {"PO", poNull}} { // WO has no approval_doc_types row on dev, so the generic route cannot run for it
		for _, act := range []string{"approve", "reject"} {
			code, body := call("PUT", fmt.Sprintf("/approval/%s/%d/%s", doc.t, doc.id, act), map[string]any{"comments": "x"})
			if code != 403 || !strings.Contains(body, msgNotAssignedApprover) {
				t.Errorf("%s %s on NULL-approver doc: %d %s", doc.t, act, code, body)
			}
		}
	}
	if code, body := call("POST", fmt.Sprintf("/memo/%d/approve", memoNull), map[string]any{"action": "APPROVE"}); code != 403 || !strings.Contains(body, msgNotAssignedApprover) {
		t.Errorf("legacy memo approve on NULL-approver doc: %d %s", code, body)
	}
	var st string
	_ = pool.QueryRow(ctx, `SELECT status FROM memo WHERE id=$1`, memoNull).Scan(&st)
	if st != "PENDING_APPROVAL" {
		t.Errorf("NULL-approver memo status changed to %s", st)
	}

	// ── 9. submit needs an approver, before anything changes ──
	memoDraftNoApprover := mkMemo("ZZ-M-DRAFT0", nil, "DRAFT")
	if code, body := call("POST", fmt.Sprintf("/memo/%d/submit", memoDraftNoApprover), nil); code != 400 || !strings.Contains(body, msgApproverRequiredToSubmit) {
		t.Errorf("memo submit without approver: %d %s", code, body)
	}
	woDraftNoApprover := mkWO("ZZ-W-DRAFT0", nil, "DRAFT")
	if code, body := call("POST", fmt.Sprintf("/work-order/%d/submit", woDraftNoApprover), nil); code != 400 {
		t.Errorf("wo submit without approver: %d %s", code, body)
	}
	for _, c := range []struct {
		tbl string
		id  int64
	}{{"memo", memoDraftNoApprover}, {"work_order", woDraftNoApprover}} {
		var s string
		_ = pool.QueryRow(ctx, fmt.Sprintf(`SELECT status FROM %s WHERE id=$1`, c.tbl), c.id).Scan(&s)
		if s != "DRAFT" {
			t.Errorf("%s moved to %s despite the rejected submit", c.tbl, s)
		}
	}

	// memo with an approver: status change and approval_request row land together
	memoDraft := mkMemo("ZZ-M-DRAFT1", &meP, "DRAFT")
	if code, body := call("POST", fmt.Sprintf("/memo/%d/submit", memoDraft), nil); code != 200 {
		t.Errorf("memo submit with approver: %d %s", code, body)
	}
	var reqCount int
	var assigned int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*), COALESCE(MAX(assigned_to),0) FROM approval_request WHERE doc_type='MEMO' AND doc_id=$1 AND status='PENDING'`, memoDraft).Scan(&reqCount, &assigned)
	_ = pool.QueryRow(ctx, `SELECT status FROM memo WHERE id=$1`, memoDraft).Scan(&st)
	if st != "PENDING_APPROVAL" || reqCount != 1 || assigned != me {
		t.Errorf("memo submit: status=%s pending requests=%d assigned_to=%d", st, reqCount, assigned)
	}
	t.Logf("memo submit -> status=%s, approval_request rows=%d, assigned_to=%d", st, reqCount, assigned)

	// requireApprovalReady itself: no approver / unconfigured doc type / configured doc type
	if err := requireApprovalReady(ctx, pool, "PO", nil); err == nil || !strings.Contains(err.Error(), msgApproverRequiredToSubmit) {
		t.Errorf("PO without approver: %v", err)
	}
	if err := requireApprovalReady(ctx, pool, "PO", &meP); err != nil {
		t.Errorf("PO with approver and configured flow: %v", err)
	}
	if err := requireApprovalReady(ctx, pool, "ZZ_NO_SUCH_TYPE", &meP); err == nil || !strings.Contains(err.Error(), msgApprovalFlowNotConfigured) {
		t.Errorf("unconfigured flow: %v", err)
	}
}

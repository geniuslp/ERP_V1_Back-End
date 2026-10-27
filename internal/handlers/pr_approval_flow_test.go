package handlers

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Integration test (dev DB, skipped when unreachable) for PR approval: everything runs inside ONE transaction that is
// always rolled back, on the first live PR, so no data is left behind.
// Covers: a submit without / with an ineligible approver is refused before any change; a valid submit makes the PR
// PENDING_APPROVAL with an open approval_request assigned to the chosen approver (who is inside the approver scope);
// PostApprove (the generic approve hook) runs the stock reservation without error; REJECTED is reopenable and reopening
// withdraws a pending request.
func TestPRApprovalFlow(t *testing.T) {
	pool := cbTestPool(t)
	ctx := context.Background()
	h := NewPRHandler(pool)

	var prID int64
	var prNo, status string
	if err := pool.QueryRow(ctx,
		`SELECT id, pr_no, status FROM purchase_request WHERE deleted_at IS NULL ORDER BY id LIMIT 1`,
	).Scan(&prID, &prNo, &status); err != nil {
		t.Skipf("no PR to test with: %v", err)
	}
	var approver int64
	if err := pool.QueryRow(ctx, `SELECT u.id FROM users u WHERE `+eligibleApproverCond+` ORDER BY u.id LIMIT 1`, "PR").Scan(&approver); err != nil {
		t.Skipf("no eligible PR approver in this database: %v", err)
	}
	var requester int64
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE is_active = true AND id <> $1 ORDER BY id LIMIT 1`, approver).Scan(&requester); err != nil {
		requester = approver
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	// bring the PR back to DRAFT first (a no-op reversal when nothing was reserved)
	if status != "DRAFT" {
		if err := reopenPRTx(ctx, tx, prID, prNo, status, requester, "test"); err != nil {
			t.Fatalf("reopen: %v", err)
		}
	}

	getStatus := func() string {
		var s string
		if err := tx.QueryRow(ctx, `SELECT status FROM purchase_request WHERE id=$1`, prID).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	// 1. no approver -> 400, status untouched
	if err := h.requestApprovalTx(ctx, tx, prID, prNo, nil, "DRAFT", requester, "t"); err == nil {
		t.Fatal("submit without an approver must fail")
	} else if fe, ok := err.(*fiber.Error); !ok || fe.Code != fiber.StatusBadRequest {
		t.Fatalf("want a 400, got %v", err)
	}
	// 2. an approver who is not in the eligible pool -> 400
	var stranger int64
	if err := tx.QueryRow(ctx, `SELECT u.id FROM users u WHERE u.is_active = true AND NOT (`+eligibleApproverCond+`) ORDER BY u.id LIMIT 1`, "PR").Scan(&stranger); err == nil {
		if err := h.requestApprovalTx(ctx, tx, prID, prNo, &stranger, "DRAFT", requester, "t"); err == nil {
			t.Fatal("submit with an ineligible approver must fail")
		}
	}
	if got := getStatus(); got != "DRAFT" {
		t.Fatalf("failed submits must not change the status, got %s", got)
	}

	// 3. valid submit
	if err := h.requestApprovalTx(ctx, tx, prID, prNo, &approver, "DRAFT", requester, "submitted"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if got := getStatus(); got != "PENDING_APPROVAL" {
		t.Fatalf("want PENDING_APPROVAL, got %s", got)
	}
	var assigned int64
	var reqStatus string
	if err := tx.QueryRow(ctx,
		`SELECT assigned_to, status FROM approval_request WHERE doc_type='PR' AND doc_id=$1 ORDER BY id DESC LIMIT 1`, prID,
	).Scan(&assigned, &reqStatus); err != nil || assigned != approver || reqStatus != "PENDING" {
		t.Fatalf("approval_request: assigned=%d status=%s err=%v", assigned, reqStatus, err)
	}
	if _, allowed, err := docApprovalAccess(ctx, tx, "PR", prID, approver, true); err != nil || !allowed {
		t.Fatalf("the chosen approver must be inside the approver scope (allowed=%v err=%v)", allowed, err)
	}
	if _, allowed, _ := docApprovalAccess(ctx, tx, "PR", prID, requester, true); allowed && requester != approver {
		t.Fatal("someone who is not the assigned approver must not be allowed to act")
	}

	// 4. approve: the generic route flips the status to COMPLETED, then runs this hook inside the same tx
	if _, err := tx.Exec(ctx, `UPDATE purchase_request SET status='COMPLETED' WHERE id=$1`, prID); err != nil {
		t.Fatal(err)
	}
	if err := h.PostApprove(ctx, tx, prID, approver); err != nil {
		t.Fatalf("PostApprove (stock reservation): %v", err)
	}

	// 5. a REJECTED PR can be reopened, and reopening withdraws a still-pending request
	if err := reopenPRTx(ctx, tx, prID, prNo, "COMPLETED", requester, "t"); err != nil {
		t.Fatalf("reopen after approve: %v", err)
	}
	if err := h.requestApprovalTx(ctx, tx, prID, prNo, &approver, "DRAFT", requester, "resubmitted"); err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE purchase_request SET status='REJECTED' WHERE id=$1`, prID); err != nil {
		t.Fatal(err)
	}
	if !isPREditableStatus("REJECTED") || !isPREditableStatus("PENDING_APPROVAL") {
		t.Fatal("REJECTED and PENDING_APPROVAL PRs must be editable/reopenable")
	}
	if err := h.requestApprovalTx(ctx, tx, prID, prNo, &approver, "REJECTED", requester, "again"); err != nil {
		t.Fatalf("resubmit after reject: %v", err)
	}
	var pending int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM approval_request WHERE doc_type='PR' AND doc_id=$1 AND status='PENDING'`, prID).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("exactly one pending request expected after a resubmission, got %d (err %v)", pending, err)
	}
	if err := reopenPRTx(ctx, tx, prID, prNo, "PENDING_APPROVAL", requester, "withdraw"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM approval_request WHERE doc_type='PR' AND doc_id=$1 AND status='PENDING'`, prID).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("reopen must withdraw the pending request, still %d (err %v)", pending, err)
	}
}

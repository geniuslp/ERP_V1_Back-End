package handlers

import (
	"context"
	"fmt"
	"log"

	"erp-api/internal/auth"
	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// Approver scoping for Memo / PO / WO / PR: an approver sees and may act on ONLY the documents assigned to them
// (<table>.approver_id = their user id). A document with approver_id IS NULL is actionable by nobody and is in no
// queue or badge count (it has to be given an approver first). approval_delegation (extra approvers) does NOT widen
// the scope: an extra approver is chosen as a document's approver_id through the eligible-approver picker like
// anyone else.

const msgNotAssignedApprover = "เอกสารนี้ไม่ได้ถูกกำหนดให้คุณเป็นผู้อนุมัติ"

// canViewAllApprovals is the single exception hook: return true for users who may see/act on every approval
// document regardless of assignment (e.g. an admin role). It deliberately returns false for everyone today.
func canViewAllApprovals(_ *auth.Claims) bool {
	return false
}

type approvalScopeDoc struct {
	table   string // document table (constant, never request input)
	logTbl  string // status-log table
	logFK   string // status-log column pointing at the document id
	pending string // SQL list of pending statuses
}

var approvalScopeDocs = map[string]approvalScopeDoc{
	"MEMO": {"memo", "memo_status_log", "memo_id", "'PENDING_APPROVAL'"},
	"PO":   {"purchase_order", "po_status_log", "po_id", "'PENDING_APPROVAL','PENDING_REAPPROVAL'"},
	"WO":   {"work_order", "work_order_status_log", "wo_id", "'PENDING_APPROVAL','PENDING_REAPPROVAL'"},
	"PR":   {"purchase_request", "pr_status_log", "pr_id", "'PENDING_APPROVAL'"},
}

// approvalAssignedSQL: the document is assigned to $user. alias is the document table alias; userIdx is the 1-based
// placeholder number of the user id. approver_id IS NULL never matches.
func approvalAssignedSQL(alias string, userIdx int) string {
	return fmt.Sprintf(`%[1]s.approver_id = $%[2]d`, alias, userIdx)
}

// approvalVisibleSQL = assigned, OR (for documents that are no longer pending) this user recorded the
// APPROVED/REJECTED decision — the "already decided" history. Pending documents are never exposed through the
// decided clause, so a previous decider cannot see a document now waiting on someone else.
func approvalVisibleSQL(docType, alias string, userIdx int) string {
	d := approvalScopeDocs[docType]
	return fmt.Sprintf(`(%s
		OR (%s.status NOT IN (%s) AND EXISTS (
			SELECT 1 FROM %s l WHERE l.%s = %s.id AND l.changed_by = $%d AND l.to_status IN ('APPROVED','REJECTED'))))`,
		approvalAssignedSQL(alias, userIdx), alias, d.pending, d.logTbl, d.logFK, alias, userIdx)
}

type approvalQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// docApprovalAccess reports whether the document exists and whether userID is inside the approver scope.
// forAction=true is the stricter "assigned" rule used by approve/reject; false adds the decided-history clause.
func docApprovalAccess(ctx context.Context, q approvalQuerier, docType string, docID, userID int64, forAction bool) (exists, allowed bool, err error) {
	d, ok := approvalScopeDocs[docType]
	if !ok {
		return false, false, fmt.Errorf("approval scope: unsupported doc type %q", docType)
	}
	cond := approvalAssignedSQL("d", 2)
	if !forAction {
		cond = approvalVisibleSQL(docType, "d", 2)
	}
	err = q.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS(SELECT 1 FROM %[1]s WHERE id = $1),
		       EXISTS(SELECT 1 FROM %[1]s d WHERE d.id = $1 AND %[2]s)`, d.table, cond),
		docID, userID).Scan(&exists, &allowed)
	return exists, allowed, err
}

// approverMayAct is the approve/reject/eligibility check. A missing document is "not allowed" here; callers
// have already resolved existence.
func approverMayAct(ctx context.Context, q approvalQuerier, user *auth.Claims, docType string, docID int64) (bool, error) {
	if user == nil {
		return false, nil
	}
	if canViewAllApprovals(user) {
		return true, nil
	}
	_, allowed, err := docApprovalAccess(ctx, q, docType, docID, user.UserID, true)
	return allowed, err
}

// isApprovalScopedDocType: doc types that use the approver_id scope above.
func isApprovalScopedDocType(docType string) bool {
	_, ok := approvalScopeDocs[docType]
	return ok
}

// requireApprovalDetailAccess guards the detail endpoints when the approval page asks for them
// (?for_approval=true). A missing document passes through so the handler returns its normal 404.
func requireApprovalDetailAccess(c *fiber.Ctx, q approvalQuerier, docType string, docID int64) error {
	if !c.QueryBool("for_approval", false) {
		return nil
	}
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	if canViewAllApprovals(claims) {
		return nil
	}
	exists, allowed, err := docApprovalAccess(context.Background(), q, docType, docID, claims.UserID, false)
	if err != nil {
		log.Printf("approval detail scope %s id=%d: %v", docType, docID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบสิทธิ์การอนุมัติได้ กรุณาลองใหม่อีกครั้ง")
	}
	if exists && !allowed {
		return fiber.NewError(fiber.StatusForbidden, msgNotAssignedApprover)
	}
	return nil
}

// approvalListCond returns a WHERE fragment (and appends its two args) that restricts a list query to the
// documents visible to the caller, or "" when canViewAllApprovals lets them see everything.
func approvalListCond(claims *auth.Claims, docType, alias string, args []any) (string, []any) {
	if canViewAllApprovals(claims) {
		return "", args
	}
	args = append(args, claims.UserID)
	return approvalVisibleSQL(docType, alias, len(args)), args
}

// MyPendingCounts godoc
// @Summary      Pending-approval counts for the current user (sidebar badges)
// @Description  Number of Memo / PO / WO / PR documents waiting for the caller: status PENDING_APPROVAL (Memo, PR) or PENDING_APPROVAL/PENDING_REAPPROVAL (PO, WO), assigned to the caller (approver_id; a NULL approver_id counts for nobody). Same filter as the approval lists.
// @Tags         Approvals
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Router       /approvals/my-pending-counts [get]
func (h *ApprovalHandler) MyPendingCounts(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()
	out := fiber.Map{}
	total := 0
	for _, docType := range []string{"MEMO", "PO", "WO", "PR"} {
		d := approvalScopeDocs[docType]
		var n int
		var err error
		if canViewAllApprovals(claims) {
			err = h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s d WHERE d.status IN (%s)%s`,
				d.table, d.pending, approvalNotDeleted(docType))).Scan(&n)
		} else {
			err = h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s d WHERE d.status IN (%s)%s AND %s`,
				d.table, d.pending, approvalNotDeleted(docType), approvalAssignedSQL("d", 1)),
				claims.UserID).Scan(&n)
		}
		if err != nil {
			log.Printf("my-pending-counts %s: %v", docType, err)
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดจำนวนเอกสารรออนุมัติได้ กรุณาลองใหม่อีกครั้ง")
		}
		out[map[string]string{"MEMO": "memo", "PO": "po", "WO": "wo", "PR": "pr"}[docType]] = n
		total += n
	}
	out["total"] = total
	return c.JSON(fiber.Map{"success": true, "data": out})
}

// approvalNotDeleted: purchase_order and purchase_request are soft-deleted; memo and work_order have no deleted_at column.
func approvalNotDeleted(docType string) string {
	if docType == "PO" || docType == "PR" {
		return " AND d.deleted_at IS NULL"
	}
	return ""
}

const (
	msgApproverRequiredToSubmit  = "กรุณาเลือกผู้อนุมัติก่อนส่งขออนุมัติ"
	msgApprovalFlowNotConfigured = "ยังไม่ได้ตั้งค่าขั้นตอนการอนุมัติของเอกสารประเภทนี้ กรุณาติดต่อผู้ดูแลระบบ"
)

// requireApprovalReady is the gate every path that moves a document to PENDING_APPROVAL runs BEFORE changing the
// status: an approver must be chosen, and the doc type must have an active step-1 approval_config so the
// approval_request row can be written in the same transaction (a document must never sit in PENDING_APPROVAL
// without a request someone can act on). Plain Thai 400s, never a DB error.
func requireApprovalReady(ctx context.Context, q approvalQuerier, docType string, approverID *int64) error {
	if approverID == nil || *approverID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, msgApproverRequiredToSubmit)
	}
	var ok bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM approval_config WHERE doc_type=$1 AND step_no=1 AND is_active=true)`, docType,
	).Scan(&ok); err != nil {
		log.Printf("approval ready %s: %v", docType, err)
		return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถตรวจสอบขั้นตอนการอนุมัติได้ กรุณาลองใหม่อีกครั้ง")
	}
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, msgApprovalFlowNotConfigured)
	}
	return nil
}

// approvedEventLateralSQL is a "LEFT JOIN LATERAL ... AS ev" giving the latest approval of a document as
// (ev_at, ev_by): from approval_log (action APPROVE) or, because the config-driven approve path
// (PUT /approval/<type>/:id/approve) writes only the document's status-log table, from that table's
// to_status='APPROVED' row. All arguments are code constants, never request input.
func approvedEventLateralSQL(docType, alias, logTbl, logFK string) string {
	return fmt.Sprintf(`LEFT JOIN LATERAL (
		SELECT x.ev_at, x.ev_by FROM (
			SELECT action_at AS ev_at, action_by AS ev_by FROM approval_log
			WHERE doc_type = '%[1]s' AND doc_id = %[2]s.id AND action = 'APPROVE'
			UNION ALL
			SELECT changed_at, changed_by FROM %[3]s WHERE %[4]s = %[2]s.id AND to_status = 'APPROVED'
		) x ORDER BY x.ev_at DESC LIMIT 1
	) ev ON true`, docType, alias, logTbl, logFK)
}

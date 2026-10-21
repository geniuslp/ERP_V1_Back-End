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

// Approver scoping for Memo / PO / WO: an approver sees and may act on ONLY the documents assigned to them
// (<table>.approver_id = their user id). Legacy documents with approver_id IS NULL were routed by role before the
// per-document approver existed, so they stay visible only to users holding a role in an active approval_config
// row of that doc type. approval_delegation (extra approvers) does NOT widen the scope: an extra approver is
// chosen as a document's approver_id through the eligible-approver picker like anyone else.

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
}

// approvalAssignedSQL: the document is assigned to $user, or is a legacy NULL-approver document and $user holds an
// approving role of $docType in approval_config. alias is the document table alias; userIdx/typeIdx are 1-based
// placeholder numbers (the doc type is passed as a parameter, never inlined).
func approvalAssignedSQL(alias string, userIdx, typeIdx int) string {
	return fmt.Sprintf(`(%[1]s.approver_id = $%[2]d
		OR (%[1]s.approver_id IS NULL AND EXISTS (
			SELECT 1 FROM approval_config ac
			JOIN user_roles ur ON ur.role_id = ac.approver_role_id
			JOIN roles r ON r.id = ac.approver_role_id AND r.is_active = true
			WHERE ac.doc_type = $%[3]d AND ac.is_active = true AND ur.user_id = $%[2]d)))`,
		alias, userIdx, typeIdx)
}

// approvalVisibleSQL = assigned, OR (for documents that are no longer pending) this user recorded the
// APPROVED/REJECTED decision — the "already decided" history. Pending documents are never exposed through the
// decided clause, so a previous decider cannot see a document now waiting on someone else.
func approvalVisibleSQL(docType, alias string, userIdx, typeIdx int) string {
	d := approvalScopeDocs[docType]
	return fmt.Sprintf(`(%s
		OR (%s.status NOT IN (%s) AND EXISTS (
			SELECT 1 FROM %s l WHERE l.%s = %s.id AND l.changed_by = $%d AND l.to_status IN ('APPROVED','REJECTED'))))`,
		approvalAssignedSQL(alias, userIdx, typeIdx), alias, d.pending, d.logTbl, d.logFK, alias, userIdx)
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
	cond := approvalAssignedSQL("d", 2, 3)
	if !forAction {
		cond = approvalVisibleSQL(docType, "d", 2, 3)
	}
	err = q.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS(SELECT 1 FROM %[1]s WHERE id = $1),
		       EXISTS(SELECT 1 FROM %[1]s d WHERE d.id = $1 AND %[2]s)`, d.table, cond),
		docID, userID, docType).Scan(&exists, &allowed)
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
	args = append(args, claims.UserID, docType)
	return approvalVisibleSQL(docType, alias, len(args)-1, len(args)), args
}

// MyPendingCounts godoc
// @Summary      Pending-approval counts for the current user (sidebar badges)
// @Description  Number of Memo / PO / WO documents waiting for the caller: status PENDING_APPROVAL (Memo) or PENDING_APPROVAL/PENDING_REAPPROVAL (PO, WO), assigned to the caller (approver_id) or legacy NULL-approver documents the caller can approve by role. Same filter as the approval lists.
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
	for _, docType := range []string{"MEMO", "PO", "WO"} {
		d := approvalScopeDocs[docType]
		var n int
		var err error
		if canViewAllApprovals(claims) {
			err = h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s d WHERE d.status IN (%s)%s`,
				d.table, d.pending, approvalNotDeleted(docType))).Scan(&n)
		} else {
			err = h.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s d WHERE d.status IN (%s)%s AND %s`,
				d.table, d.pending, approvalNotDeleted(docType), approvalAssignedSQL("d", 1, 2)),
				claims.UserID, docType).Scan(&n)
		}
		if err != nil {
			log.Printf("my-pending-counts %s: %v", docType, err)
			return fiber.NewError(fiber.StatusInternalServerError, "ไม่สามารถโหลดจำนวนเอกสารรออนุมัติได้ กรุณาลองใหม่อีกครั้ง")
		}
		out[map[string]string{"MEMO": "memo", "PO": "po", "WO": "wo"}[docType]] = n
		total += n
	}
	out["total"] = total
	return c.JSON(fiber.Map{"success": true, "data": out})
}

// approvalNotDeleted: purchase_order is soft-deleted; memo and work_order have no deleted_at column.
func approvalNotDeleted(docType string) string {
	if docType == "PO" {
		return " AND d.deleted_at IS NULL"
	}
	return ""
}

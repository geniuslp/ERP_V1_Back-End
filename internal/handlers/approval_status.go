package handlers

import (
	"context"
	"fmt"

	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// isEligibleApprover decides whether userID may act on the current PENDING step of the
// given document, and is shared by MyApprovalStatus (button gating) and
// GenericApprovalHandler.decide (server-side enforcement) so the two never diverge.
//
// MEMO is a special case: each memo names a specific approver_id at create/edit time
// (chosen from the eligible pool returned by GET /master/eligible-approvers, which IS
// role-based via approval_config). Once that specific person is chosen, eligibility to
// act on THAT memo is just "are you that person" (or an extra approver from
// approval_delegation) — not "does your role still match approval_config" again. A role
// change or approval_config edit after the memo was created must not retroactively grant
// or revoke approval rights on an already-assigned memo.
//
// PO now follows the exact same per-document-approver_id model as MEMO: a specific person
// is chosen from the role-eligible pool (GET /master/eligible-approvers) at PO creation time,
// stored on purchase_order.approver_id, and only that person (or an active PO extra approver)
// may act on it — not everyone who currently holds the approving role.
//
// All other doc types (BORROW, ...) keep the original role-based + extra-approver +
// assigned_to union, since they have no per-document approver_id field.
func isEligibleApprover(ctx context.Context, db *pgxpool.Pool, docType string, stepNo int, docID int64, userID int64) (bool, error) {
	if docType == "MEMO" {
		var eligible bool
		err := db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM memo WHERE id = $2 AND approver_id = $1

				UNION

				SELECT 1
				FROM approval_delegation ad
				WHERE (ad.doc_type = 'MEMO' OR ad.doc_type IS NULL) AND ad.user_id = $1
			)`,
			userID, docID,
		).Scan(&eligible)
		return eligible, err
	}

	if docType == "PO" {
		var eligible bool
		err := db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM purchase_order WHERE id = $2 AND approver_id = $1

				UNION

				SELECT 1
				FROM approval_delegation ad
				WHERE (ad.doc_type = 'PO' OR ad.doc_type IS NULL) AND ad.user_id = $1
			)`,
			userID, docID,
		).Scan(&eligible)
		return eligible, err
	}

	var eligible bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM approval_config ac
			JOIN user_roles ur ON ur.role_id = ac.approver_role_id
			JOIN roles r ON r.id = ac.approver_role_id AND r.is_active = true
			WHERE ac.doc_type = $1 AND ac.step_no = $2 AND ac.is_active = true AND ur.user_id = $3

			UNION

			SELECT 1
			FROM approval_delegation ad
			WHERE (ad.doc_type = $1 OR ad.doc_type IS NULL) AND ad.user_id = $3

			UNION

			SELECT 1
			FROM approval_request ar2
			WHERE ar2.doc_type = $1 AND ar2.doc_id = $4 AND ar2.step_no = $2 AND ar2.assigned_to = $3
		)`,
		docType, stepNo, userID, docID,
	).Scan(&eligible)
	return eligible, err
}

// docTypeTable maps a doc_type to the table that owns its lifecycle status column.
// approval_doc_types does not currently store this mapping (it only has doc_type/doc_label),
// so this stays hardcoded and only needs entries for doc types actually wired into the
// approval_request/approval_config flow today (MEMO, BORROW, PO). PR is NOT here —
// PR approval was removed; it 400s via the approval_doc_types/docTypeTable check below.
// If more doc types are added to this generic flow, add them here too (or better, add
// a table_name column to approval_doc_types).
var docTypeTable = map[string]string{
	"MEMO":   "memo",
	"BORROW": "borrow",
	"PO":     "purchase_order",
	"WO":     "work_order",
}

// docTypePK maps a doc_type to the primary key column of its docTypeTable entry.
// Defaults to "id" when absent. purchase_order's PK is actually "id" (verified directly
// against the live schema — migrations/001_master_ddl.sql:208 claims "po_id" but is stale),
// so no override is needed for PO; this map is currently empty and kept for future doc types
// whose table really does deviate from "id".
var docTypePK = map[string]string{}

func pkColumn(docType string) string {
	if col, ok := docTypePK[docType]; ok {
		return col
	}
	return "id"
}

// MyApprovalStatus godoc
// @Summary      Check whether the current user is an eligible approver for a document's current step
// @Description  Backs Approve/Reject button gating on the frontend. Eligibility = role-based approver
// @Description  (approval_config + user_roles) OR extra approver (approval_delegation), for the doc's
// @Description  current PENDING approval_request row.
// @Tags         Approvals
// @Security     BearerAuth
// @Produce      json
// @Param        docType  path  string  true  "Document type, e.g. MEMO, BORROW, or PO"
// @Param        docId    path  int     true  "Document ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-request/{docType}/{docId}/my-status [get]
func (h *ApprovalHandler) MyApprovalStatus(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	fmt.Println("[DEBUG] docType param:", c.Params("docType"))
	fmt.Println("[DEBUG] docID param:", c.Params("docId"))
	fmt.Println("[DEBUG] claims.UserID:", claims.UserID)

	docType := c.Params("docType")
	docID, err := c.ParamsInt("docId")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid docId"})
	}

	ctx := context.Background()

	var docTypeExists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM approval_doc_types WHERE doc_type = $1)`, docType).Scan(&docTypeExists); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !docTypeExists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unrecognized doc_type: " + docType})
	}

	table, ok := docTypeTable[docType]
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "doc_type '" + docType + "' is not wired into the step-based approval flow yet"})
	}

	pk := pkColumn(docType)

	var docExists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE `+pk+` = $1)`, docID).Scan(&docExists); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !docExists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "document not found"})
	}

	// Current pending step = the approval_request row still awaiting a decision.
	// At most one PENDING row exists per doc at a time under the current single-active-step model.
	var stepNo int
	var stepStatus string
	var stepName *string
	err = h.db.QueryRow(ctx, `
		SELECT ar.step_no, ar.status, ac.step_name
		FROM approval_request ar
		LEFT JOIN approval_config ac ON ac.doc_type = ar.doc_type AND ac.step_no = ar.step_no
		WHERE ar.doc_type = $1 AND ar.doc_id = $2 AND ar.status = 'PENDING'
		ORDER BY ar.step_no DESC
		LIMIT 1`,
		docType, docID,
	).Scan(&stepNo, &stepStatus, &stepName)

	if err != nil {
		// No pending step — either never submitted, or already resolved. Report the
		// most recent approval_request status if one exists; otherwise fall back to
		// the document's own status column.
		var lastStatus *string
		h.db.QueryRow(ctx, `
			SELECT status FROM approval_request
			WHERE doc_type = $1 AND doc_id = $2
			ORDER BY step_no DESC, id DESC
			LIMIT 1`,
			docType, docID,
		).Scan(&lastStatus)

		docStatus := "unknown"
		if lastStatus != nil {
			docStatus = *lastStatus
		} else {
			h.db.QueryRow(ctx, `SELECT status FROM `+table+` WHERE `+pk+` = $1`, docID).Scan(&docStatus)
		}

		return c.JSON(fiber.Map{"data": fiber.Map{
			"is_assigned_approver": false,
			"current_step_no":      nil,
			"current_step_name":    nil,
			"doc_status":           docStatus,
		}})
	}

	isAssigned, err := isEligibleApprover(ctx, h.db, docType, stepNo, int64(docID), claims.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": fiber.Map{
		"is_assigned_approver": isAssigned,
		"current_step_no":      stepNo,
		"current_step_name":    stepName,
		"doc_status":           stepStatus,
	}})
}

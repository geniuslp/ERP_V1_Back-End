package handlers

import (
	"context"
	"fmt"
	"strings"

	"erp-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GenericApprovalHandler drives Approve/Reject for any doc_type configured in
// approval_doc_types, instead of a hand-written handler per doc type. A new doc type gets
// approve/reject support by adding rows to approval_doc_types + approval_config — zero new
// Go code — provided its status-log table (if any) follows the same shape as po_status_log /
// memo_status_log: (id, <fk>, from_status, to_status, changed_by, changed_at, remarks).
type GenericApprovalHandler struct {
	db *pgxpool.Pool
}

func NewGenericApprovalHandler(db *pgxpool.Pool) *GenericApprovalHandler {
	return &GenericApprovalHandler{db: db}
}

type docTypeConfig struct {
	IsActive          bool
	TableName         *string
	IDColumn          string
	StatusColumn      string
	ApprovedValue     string
	RejectedValue     string
	StatusLogTable    *string
	StatusLogFKColumn *string
}

// Approve godoc
// @Summary      Approve a document (generic, config-driven)
// @Tags         Approvals
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        docType  path  string  true  "Document type, e.g. PO or MEMO"
// @Param        docId    path  int     true  "Document ID"
// @Param        body     body  object  false "Optional comments"
// @Success      200  {object}  fiber.Map
// @Router       /approval/{docType}/{docId}/approve [put]
func (h *GenericApprovalHandler) Approve(c *fiber.Ctx) error {
	return h.decide(c, "APPROVE")
}

// Reject godoc
// @Summary      Reject a document (generic, config-driven)
// @Tags         Approvals
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        docType  path  string  true  "Document type, e.g. PO or MEMO"
// @Param        docId    path  int     true  "Document ID"
// @Param        body     body  object  false "Optional comments"
// @Success      200  {object}  fiber.Map
// @Router       /approval/{docType}/{docId}/reject [put]
func (h *GenericApprovalHandler) Reject(c *fiber.Ctx) error {
	return h.decide(c, "REJECT")
}

func (h *GenericApprovalHandler) decide(c *fiber.Ctx, action string) error {
	docType := c.Params("docType")
	docID, err := c.ParamsInt("docId")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid docId")
	}

	var req struct {
		Comments *string `json:"comments"`
	}
	_ = c.BodyParser(&req)

	ctx := context.Background()
	claims := middleware.GetClaims(c)
	if claims == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	cfg, err := h.loadDocTypeConfig(ctx, docType)
	if err != nil {
		return err
	}

	// Identifiers below (cfg.TableName / IDColumn / StatusColumn / StatusLogTable /
	// StatusLogFKColumn) all come from approval_doc_types, an admin-managed table — never
	// from request input — but are still whitelist-verified against information_schema
	// just before use, as defense in depth against a misconfigured or tampered row.
	if err := h.assertRealColumn(ctx, *cfg.TableName, cfg.IDColumn); err != nil {
		return err
	}
	if err := h.assertRealColumn(ctx, *cfg.TableName, cfg.StatusColumn); err != nil {
		return err
	}
	if cfg.StatusLogTable != nil {
		if err := h.assertRealColumn(ctx, *cfg.StatusLogTable, *cfg.StatusLogFKColumn); err != nil {
			return err
		}
	}

	var currentDocStatus string
	docExistsSQL := fmt.Sprintf(`SELECT %s FROM %s WHERE %s=$1`, cfg.StatusColumn, *cfg.TableName, cfg.IDColumn)
	if err := h.db.QueryRow(ctx, docExistsSQL, docID).Scan(&currentDocStatus); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "document not found")
	}

	// The presence of a PENDING approval_request row for this doc is the generic precondition
	// for "is this document currently approvable" — deliberately not a hardcoded status list,
	// since valid pre-approval statuses differ per doc type (e.g. PO's PENDING_REAPPROVAL).
	var approvalReqID int64
	var stepNo int
	if err := h.db.QueryRow(ctx, `
		SELECT id, step_no FROM approval_request
		WHERE doc_type=$1 AND doc_id=$2 AND status='PENDING'
		ORDER BY step_no DESC LIMIT 1`,
		docType, docID,
	).Scan(&approvalReqID, &stepNo); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "no pending approval for this document")
	}

	// Eligibility mirrors approval_status.go's MyApprovalStatus exactly (via the shared
	// isEligibleApprover helper), so the Approve/Reject button gating and the actual
	// server-side enforcement never diverge. MEMO uses its own approver_id + extra-approver
	// rule instead of role-based matching — see isEligibleApprover's doc comment.
	eligible, err := isEligibleApprover(ctx, h.db, docType, stepNo, int64(docID), claims.UserID)
	if err != nil {
		return err
	}
	if !eligible {
		return fiber.NewError(fiber.StatusForbidden, "you are not an eligible approver for this document")
	}

	newValue := cfg.ApprovedValue
	if action == "REJECT" {
		newValue = cfg.RejectedValue
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	updateSQL := fmt.Sprintf(`UPDATE %s SET %s=$1 WHERE %s=$2`, *cfg.TableName, cfg.StatusColumn, cfg.IDColumn)
	if _, err := tx.Exec(ctx, updateSQL, newValue, docID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update document status: "+err.Error())
	}

	if _, err := tx.Exec(ctx, `UPDATE approval_request SET status=$1, updated_at=NOW() WHERE id=$2`, newValue, approvalReqID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update approval_request: "+err.Error())
	}

	if cfg.StatusLogTable != nil {
		logSQL := fmt.Sprintf(
			`INSERT INTO %s (%s, from_status, to_status, changed_by, remarks) VALUES ($1,$2,$3,$4,$5)`,
			*cfg.StatusLogTable, *cfg.StatusLogFKColumn,
		)
		if _, err := tx.Exec(ctx, logSQL, docID, currentDocStatus, newValue, claims.UserID, req.Comments); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to write status log: "+err.Error())
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"success": true, "message": fmt.Sprintf("%s %s", docType, strings.ToLower(newValue))})
}

func (h *GenericApprovalHandler) loadDocTypeConfig(ctx context.Context, docType string) (*docTypeConfig, error) {
	var cfg docTypeConfig
	err := h.db.QueryRow(ctx, `
		SELECT is_active, table_name, id_column, status_column, approved_value, rejected_value,
		       status_log_table, status_log_fk_column
		FROM approval_doc_types WHERE doc_type=$1`, docType,
	).Scan(&cfg.IsActive, &cfg.TableName, &cfg.IDColumn, &cfg.StatusColumn, &cfg.ApprovedValue, &cfg.RejectedValue,
		&cfg.StatusLogTable, &cfg.StatusLogFKColumn)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "unrecognized doc_type: "+docType)
	}
	if !cfg.IsActive {
		return nil, fiber.NewError(fiber.StatusNotFound, "unrecognized doc_type: "+docType)
	}
	if cfg.TableName == nil || *cfg.TableName == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "doc_type '"+docType+"' is not configured for generic approval (no table_name)")
	}
	return &cfg, nil
}

// assertRealColumn is the defense-in-depth whitelist check: table/column names are only ever
// sourced from approval_doc_types (admin-managed config, never request input), but are still
// verified against information_schema right before being string-formatted into SQL.
func (h *GenericApprovalHandler) assertRealColumn(ctx context.Context, table, column string) error {
	var exists bool
	if err := h.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name=$1 AND column_name=$2
		)`, table, column).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("approval config error: %s.%s is not a real column", table, column))
	}
	return nil
}

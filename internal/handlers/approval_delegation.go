package handlers

import (
	"context"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApprovalDelegationHandler struct{ db *pgxpool.Pool }

func NewApprovalDelegationHandler(db *pgxpool.Pool) *ApprovalDelegationHandler {
	return &ApprovalDelegationHandler{db: db}
}

type approvalDelegationItem struct {
	ID        int64     `json:"id"`
	DocType   *string   `json:"doc_type"`
	UserID    int64     `json:"user_id"`
	UserName  *string   `json:"user_name"`
	Reason    *string   `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy *int64    `json:"created_by"`
}

// List godoc
// @Summary      List extra approvers
// @Tags         Approval Delegation
// @Produce      json
// @Param        doc_type  query  string  false  "Filter by doc type"
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-delegation [get]
func (h *ApprovalDelegationHandler) List(c *fiber.Ctx) error {
	query := `
		SELECT ad.id, ad.doc_type, ad.user_id, u.full_name, ad.reason, ad.created_at, ad.created_by
		FROM approval_delegation ad
		LEFT JOIN users u ON u.id = ad.user_id
		WHERE 1=1`
	args := []any{}

	if docType := c.Query("doc_type"); docType != "" {
		args = append(args, docType)
		query += " AND ad.doc_type = $" + strconv.Itoa(len(args))
	}
	query += " ORDER BY ad.id DESC"

	rows, err := h.db.Query(context.Background(), query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var out []approvalDelegationItem
	for rows.Next() {
		var it approvalDelegationItem
		if err := rows.Scan(&it.ID, &it.DocType, &it.UserID, &it.UserName, &it.Reason, &it.CreatedAt, &it.CreatedBy); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out = append(out, it)
	}
	if out == nil {
		out = []approvalDelegationItem{}
	}
	return c.JSON(fiber.Map{"data": out})
}

type approvalDelegationRequest struct {
	DocType *string `json:"doc_type"`
	UserID  *int64  `json:"user_id"`
	Reason  *string `json:"reason"`
}

// Create godoc
// @Summary      Add an extra approver (additive, on top of role-based approvers)
// @Tags         Approval Delegation
// @Accept       json
// @Produce      json
// @Param        body  body  approvalDelegationRequest  true  "Extra approver payload"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-delegation [post]
func (h *ApprovalDelegationHandler) Create(c *fiber.Ctx) error {
	var body approvalDelegationRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.UserID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id is required"})
	}

	ctx := context.Background()

	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, *body.UserID).Scan(&exists); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !exists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id does not exist"})
	}

	// empty string / omitted doc_type means "applies to all doc types" -> store as NULL
	var docType *string
	if body.DocType != nil && *body.DocType != "" {
		var docExists bool
		if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM approval_doc_types WHERE doc_type = $1)`, *body.DocType).Scan(&docExists); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if !docExists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "doc_type does not exist"})
		}
		docType = body.DocType
	}

	var id int64
	var createdAt time.Time
	err := h.db.QueryRow(ctx,
		`INSERT INTO approval_delegation (doc_type, user_id, reason) VALUES ($1, $2, $3) RETURNING id, created_at`,
		docType, *body.UserID, body.Reason,
	).Scan(&id, &createdAt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": fiber.Map{
		"id": id, "doc_type": docType, "user_id": *body.UserID, "reason": body.Reason, "created_at": createdAt,
	}})
}

// Delete godoc
// @Summary      Remove an extra approver
// @Tags         Approval Delegation
// @Produce      json
// @Param        id  path  int  true  "Extra approver row ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-delegation/{id} [delete]
func (h *ApprovalDelegationHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	tag, err := h.db.Exec(context.Background(), `DELETE FROM approval_delegation WHERE id = $1`, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"id": id, "deleted": true}})
}

// ResolveEffectiveApprovers returns the union of role-based approvers (from approval_config +
// user_roles, for the given doc_type/step) and extra approvers (from approval_delegation).
// Extra approvers are additive, not a substitution: a row in approval_delegation means that
// user_id can ALSO approve this doc_type, on top of whoever the role-based config already
// allows. Extra approvers have no step concept, so they apply regardless of stepNo.
func (h *ApprovalDelegationHandler) ResolveEffectiveApprovers(ctx context.Context, docType string, stepNo int) ([]int64, error) {
	rows, err := h.db.Query(ctx, `
		SELECT DISTINCT ur.user_id
		FROM approval_config ac
		JOIN user_roles ur ON ur.role_id = ac.approver_role_id
		WHERE ac.doc_type = $1 AND ac.step_no = $2 AND ac.is_active = true

		UNION

		SELECT ad.user_id
		FROM approval_delegation ad
		WHERE ad.doc_type = $1 OR ad.doc_type IS NULL`,
		docType, stepNo,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		out = append(out, userID)
	}
	return out, nil
}

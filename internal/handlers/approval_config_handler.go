package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApprovalConfigHandler struct{ db *pgxpool.Pool }

func NewApprovalConfigHandler(db *pgxpool.Pool) *ApprovalConfigHandler {
	return &ApprovalConfigHandler{db: db}
}

// List godoc
// @Summary      List approval configs
// @Tags         Approval Config
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-config [get]
func (h *ApprovalConfigHandler) List(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(), `
		SELECT ac.id, ac.doc_type, ac.step_no, ac.step_name, ac.approver_role_id, ac.is_active
		FROM approval_config ac
		ORDER BY ac.doc_type, ac.step_no`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type item struct {
		ID             int64  `json:"id"`
		DocType        string `json:"doc_type"`
		StepNo         int    `json:"step_no"`
		StepName       string `json:"step_name"`
		ApproverRoleID *int64  `json:"approver_role_id"`
		IsActive       bool   `json:"is_active"`
	}
	var out []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.DocType, &it.StepNo, &it.StepName, &it.ApproverRoleID, &it.IsActive); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out = append(out, it)
	}
	if out == nil {
		out = []item{}
	}
	return c.JSON(fiber.Map{"data": out})
}

// Toggle godoc
// @Summary      Toggle approval config active status
// @Tags         Approval Config
// @Accept       json
// @Produce      json
// @Param        id    path  int     true  "ID"
// @Param        body  body  object  true  "is_active"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-config/{id} [patch]
func (h *ApprovalConfigHandler) Toggle(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	tag, err := h.db.Exec(context.Background(),
		`UPDATE approval_config SET is_active = $1, updated_at = NOW() WHERE id = $2`, body.IsActive, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"id": id, "is_active": body.IsActive}})
}

// Delete godoc
// @Summary      Delete approval config
// @Tags         Approval Config
// @Produce      json
// @Param        id  path  int  true  "ID"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-config/{id} [delete]
func (h *ApprovalConfigHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	tag, err := h.db.Exec(context.Background(), `DELETE FROM approval_config WHERE id = $1`, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"id": id, "deleted": true}})
}

// BatchUpdate godoc
// @Summary      Batch create/delete approval config steps
// @Tags         Approval Config
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "create array and delete id array"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-config/batch [post]
func (h *ApprovalConfigHandler) BatchUpdate(c *fiber.Ctx) error {
	var body struct {
		Create []struct {
			DocType        string `json:"doc_type"`
			ApproverRoleID *int64  `json:"approver_role_id"`
		} `json:"create"`
		Delete []int64 `json:"delete"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	ctx := context.Background()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer tx.Rollback(ctx)

	// 1. delete
	if len(body.Delete) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM approval_config WHERE id = ANY($1)`, body.Delete); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	// 2. create
	for _, cr := range body.Create {
		if cr.DocType == "" || cr.ApproverRoleID == nil || *cr.ApproverRoleID == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "doc_type and approver_role_id are required"})
		}

		var stepNo int
		tx.QueryRow(ctx,
			`SELECT COALESCE(MAX(step_no), 0) + 1 FROM approval_config WHERE doc_type = $1`, cr.DocType,
		).Scan(&stepNo)

		var roleName string
		if err := tx.QueryRow(ctx, `SELECT role_name FROM roles WHERE id = $1`, cr.ApproverRoleID).Scan(&roleName); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "approver_role_id not found"})
		}
		stepName := roleName + " approve"

		if _, err := tx.Exec(ctx,
			`INSERT INTO approval_config (doc_type, step_no, step_name, approver_role_id, is_active) VALUES ($1, $2, $3, $4, true)`,
			cr.DocType, stepNo, stepName, cr.ApproverRoleID,
		); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// return updated list
	rows, err := h.db.Query(ctx,
		`SELECT id, doc_type, step_no, step_name, approver_role_id, is_active FROM approval_config ORDER BY doc_type, step_no`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type item struct {
		ID             int64  `json:"id"`
		DocType        string `json:"doc_type"`
		StepNo         int    `json:"step_no"`
		StepName       string `json:"step_name"`
		ApproverRoleID *int64  `json:"approver_role_id"`
		IsActive       bool   `json:"is_active"`
	}
	var out []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.DocType, &it.StepNo, &it.StepName, &it.ApproverRoleID, &it.IsActive); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out = append(out, it)
	}
	if out == nil {
		out = []item{}
	}
	return c.JSON(fiber.Map{"data": out})
}

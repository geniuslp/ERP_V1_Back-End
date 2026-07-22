package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApprovalDocTypeHandler struct{ db *pgxpool.Pool }

func NewApprovalDocTypeHandler(db *pgxpool.Pool) *ApprovalDocTypeHandler {
	return &ApprovalDocTypeHandler{db: db}
}

// List godoc
// @Summary      List approval document types
// @Tags         Approval Doc Types
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-doc-types [get]
func (h *ApprovalDocTypeHandler) List(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT id, doc_type, doc_label, is_active FROM approval_doc_types ORDER BY id`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type item struct {
		ID       int64  `json:"id"`
		DocType  string `json:"doc_type"`
		DocLabel string `json:"doc_label"`
		IsActive bool   `json:"is_active"`
	}
	var out []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.DocType, &it.DocLabel, &it.IsActive); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out = append(out, it)
	}
	if out == nil {
		out = []item{}
	}
	return c.JSON(fiber.Map{"data": out})
}

// Create godoc
// @Summary      Create approval document type
// @Tags         Approval Doc Types
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "doc_type, doc_label, is_active"
// @Success      201  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-doc-types [post]
func (h *ApprovalDocTypeHandler) Create(c *fiber.Ctx) error {
	var body struct {
		DocType  string `json:"doc_type"`
		DocLabel string `json:"doc_label"`
		IsActive bool   `json:"is_active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.DocType == "" || body.DocLabel == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "doc_type and doc_label are required"})
	}

	var id int64
	err := h.db.QueryRow(context.Background(),
		`INSERT INTO approval_doc_types (doc_type, doc_label, is_active) VALUES ($1, $2, $3) RETURNING id`,
		body.DocType, body.DocLabel, body.IsActive,
	).Scan(&id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": fiber.Map{
		"id": id, "doc_type": body.DocType, "doc_label": body.DocLabel, "is_active": body.IsActive,
	}})
}

// Toggle godoc
// @Summary      Toggle approval document type active status
// @Tags         Approval Doc Types
// @Accept       json
// @Produce      json
// @Param        id    path  int     true  "ID"
// @Param        body  body  object  true  "is_active"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      404  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /approval-doc-types/{id} [patch]
func (h *ApprovalDocTypeHandler) Toggle(c *fiber.Ctx) error {
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
		`UPDATE approval_doc_types SET is_active = $1, updated_at = NOW() WHERE id = $2`,
		body.IsActive, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"id": id, "is_active": body.IsActive}})
}

package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DepartmentHandler struct{ db *pgxpool.Pool }

func NewDepartmentHandler(db *pgxpool.Pool) *DepartmentHandler {
	return &DepartmentHandler{db: db}
}

// List godoc
// @Summary      List departments
// @Tags         Departments
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /departments [get]
func (h *DepartmentHandler) List(c *fiber.Ctx) error {
	rows, err := h.db.Query(context.Background(),
		`SELECT id, dept_code, dept_name, sort_order, is_active
		 FROM departments
		 WHERE is_active = true
		 ORDER BY sort_order`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type item struct {
		ID        int64  `json:"id"`
		DeptCode  string `json:"dept_code"`
		DeptName  string `json:"dept_name"`
		SortOrder int    `json:"sort_order"`
		IsActive  bool   `json:"is_active"`
	}
	var out []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.DeptCode, &it.DeptName, &it.SortOrder, &it.IsActive); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out = append(out, it)
	}
	if out == nil {
		out = []item{}
	}
	return c.JSON(fiber.Map{"data": out})
}

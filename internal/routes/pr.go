package routes

import (
	"erp-api/internal/handlers"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterPRApprovalRoutes attaches the department-gated PR approval endpoints
// to the provided router group (expected to be /api/v1/pr).
func RegisterPRApprovalRoutes(pr fiber.Router, db *pgxpool.Pool) {
	h := handlers.NewPRApprovalHandler(db)
	pr.Get("/", h.List)
	pr.Get("/:id", h.GetDetail)
	pr.Put("/:id/approve", h.Approve)
	pr.Put("/:id/reject", h.Reject)
}

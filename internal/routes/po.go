package routes

import (
	"erp-api/internal/handlers"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterPOApprovalRoutes attaches PO approval endpoints to the provided router group
// (expected to be /api/v1/po).
func RegisterPOApprovalRoutes(po fiber.Router, db *pgxpool.Pool) {
	h := handlers.NewPOApprovalHandler(db)
	po.Get("/", h.List)
	po.Get("/:id", h.GetDetail)
	po.Put("/:id/approve", h.Approve)
	po.Put("/:id/reject", h.Reject)
	po.Put("/:id/cancel", h.Cancel)
}

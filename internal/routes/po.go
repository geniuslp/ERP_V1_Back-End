package routes

import (
	"erp-api/internal/handlers"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterPOApprovalRoutes attaches PO approval endpoints to the provided router group
// (expected to be /api/v1/po).
// RegisterPOApprovalRoutes previously also registered GET "/" and GET "/:id" here
// (POApprovalHandler.List/GetDetail), but those were dead code — permanently shadowed by
// POHandler.List/Get registered earlier on the same group (routes.go) since Fiber matches
// the first-registered route for a given method+path. Removed rather than left as
// unreachable duplicates; POHandler.List (po.go) is the single source of truth for GET /po
// going forward — see its added limit/status[] support.
func RegisterPOApprovalRoutes(po fiber.Router, db *pgxpool.Pool) {
	h := handlers.NewPOApprovalHandler(db)
	po.Put("/:id/approve", h.Approve)
	po.Put("/:id/reject", h.Reject)
	po.Put("/:id/cancel", h.Cancel)
}

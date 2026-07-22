package handlers

import (
	"context"

	"erp-api/internal/repository"
	"erp-api/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleMenuPermissionHandler struct {
	svc *service.RoleMenuPermissionService
}

func NewRoleMenuPermissionHandler(db *pgxpool.Pool) *RoleMenuPermissionHandler {
	repo := repository.NewRoleMenuPermissionRepo(db)
	svc := service.NewRoleMenuPermissionService(repo)
	return &RoleMenuPermissionHandler{svc: svc}
}

// List godoc
// @Summary      List role menu permissions
// @Tags         Role-Menu Permissions
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /role-menu-permissions [get]
func (h *RoleMenuPermissionHandler) List(c *fiber.Ctx) error {
	rows, err := h.svc.ListAll(context.Background())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if rows == nil {
		rows = []repository.RoleMenuPermissionRow{}
	}
	return c.JSON(fiber.Map{"data": rows})
}

// BatchUpsert godoc
// @Summary      Batch upsert role menu permissions
// @Tags         Role-Menu Permissions
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "permissions array"
// @Success      200  {object}  fiber.Map
// @Failure      400  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /role-menu-permissions/batch [post]
func (h *RoleMenuPermissionHandler) BatchUpsert(c *fiber.Ctx) error {
	var body struct {
		Permissions []repository.UpsertPermissionInput `json:"permissions"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if len(body.Permissions) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "permissions must not be empty"})
	}
	if err := h.svc.BatchUpsert(context.Background(), body.Permissions); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"updated": len(body.Permissions)}})
}

// ListMenus godoc
// @Summary      List menus as parent/child tree
// @Tags         Role-Menu Permissions
// @Produce      json
// @Success      200  {object}  fiber.Map
// @Failure      500  {object}  fiber.Map
// @Router       /menus [get]
func (h *RoleMenuPermissionHandler) ListMenus(c *fiber.Ctx) error {
	menus, err := h.svc.ListMenus(context.Background())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// build parent/child tree
	byID := make(map[int64]*repository.MenuRow, len(menus))
	for i := range menus {
		byID[menus[i].ID] = &menus[i]
	}
	var roots []*repository.MenuRow
	for i := range menus {
		m := &menus[i]
		if m.ParentID == nil {
			roots = append(roots, m)
		} else if parent, ok := byID[*m.ParentID]; ok {
			parent.Children = append(parent.Children, m)
		}
	}
	if roots == nil {
		roots = []*repository.MenuRow{}
	}
	return c.JSON(fiber.Map{"data": roots})
}

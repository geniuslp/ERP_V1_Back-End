package routes

import (
	"erp-api/internal/config"
	"erp-api/internal/handlers"
	"erp-api/internal/middleware"
	"erp-api/internal/repository"
	"erp-api/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Register(app *fiber.App, db *pgxpool.Pool, cfg *config.Config) {
	// Serve uploaded files as static assets
	app.Static("/uploads", "./uploads")

	// Handler instances
	authH := handlers.NewAuthHandler(db, cfg)
	masterH := handlers.NewMasterHandler(db)
	locationH := handlers.NewLocationHandler(db)
	projectH := handlers.NewProjectHandler(db)
	inventoryH := handlers.NewInventoryHandler(db)
	prH := handlers.NewPRHandler(db)
	poH := handlers.NewPOHandler(db)
	grnH := handlers.NewGRNHandler(db)
	approvalH := handlers.NewApprovalHandler(db)
	usersH := handlers.NewUsersHandler(db, cfg)
	groupH := handlers.NewGroupHandler(db)
	attachH := handlers.NewAttachmentHandler(db)
	matRepo := repository.NewMaterialRepo(db)
	matSvc := service.NewMaterialService(matRepo)
	matH := handlers.NewMaterialCodeHandler(matSvc)
	memoH := handlers.NewMemoHandler(db)

	api := app.Group("/api/v1")

	// ── Public ──────────────────────────────────────────────────────────────
	auth := api.Group("/auth")
	auth.Post("/login", authH.Login)
	auth.Post("/refresh", authH.RefreshToken)

	// ── Protected ───────────────────────────────────────────────────────────
	// jwt := middleware.JWTProtected(cfg) // TODO: re-enable for production
	jwt := middleware.DevBypass()

	// Auth
	auth.Get("/me", jwt, authH.Me)
	auth.Post("/change-password", jwt, authH.ChangePassword)

	// Master data
	master := api.Group("/master")
	master.Get("/groups", masterH.ListGroups)
	master.Get("/units", masterH.ListUnits)
	master.Get("/materials", masterH.ListMaterials)
	master.Post("/materials", masterH.CreateMaterial)
	master.Post("/materials/bulk", masterH.BulkCreateMaterial)
	master.Get("/materials/export", jwt, masterH.ExportMaterials)
	master.Put("/materials/:code", masterH.UpdateMaterial)
	master.Get("/materials/:code", masterH.GetMaterial)
	master.Get("/allMaterial", masterH.GetAllMaterial)

	// Material type-ahead search (Create PO combobox)
	api.Get("/materials/search", masterH.SearchMaterials)
	master.Get("/subgroups", masterH.ListSubgroups)
	master.Get("/mat-names", masterH.ListMatNames)
	master.Get("/locations", locationH.ListLocations)
	master.Get("/locations/:id", locationH.GetLocation)
	master.Post("/locations", locationH.CreateLocation)
	master.Put("/locations/:id", locationH.UpdateLocation)
	master.Delete("/locations/:id", locationH.DeleteLocation)

	master.Get("/projects", projectH.ListProjects)
	master.Post("/projects", projectH.CreateProject)
	master.Put("/projects/:id", projectH.UpdateProject)
	master.Delete("/projects/:id", projectH.DeleteProject)
	master.Get("/warehouses", masterH.ListWarehouses)
	master.Post("/warehouses", masterH.CreateWarehouse)
	master.Get("/warehouses/:code/zones", masterH.ListZones)
	supplierH := handlers.NewSupplierHandler(db)
	master.Get("/suppliers", masterH.ListSuppliers)
	master.Post("/suppliers", masterH.CreateSupplier)
	master.Post("/suppliers/bulk", supplierH.BulkCreateSupplier)

	// Inventory
	inv := api.Group("/inventory")
	inv.Get("/", inventoryH.ListInventory)
	inv.Post("/transactions", inventoryH.CreateTransaction)
	inv.Get("/transactions", inventoryH.ListTransactions)

	// File upload
	upload := api.Group("/upload")
	upload.Post("/pr", attachH.UploadPRFile)

	// Purchase Request
	pr := api.Group("/pr")
	pr.Get("/list-test", func(c *fiber.Ctx) error {
		return c.SendString("list works")
	})
	pr.Get("/next-number", prH.NextNumber)
	pr.Post("/", prH.Create)
	pr.Post("/:id/submit", prH.Submit)
	pr.Post("/:id/approve", middleware.RequireRole("SENIOR_TEAM", "MANAGER", "DIRECTOR", "MD", "ADMIN"), prH.Approve)
	pr.Get("/:id/logs", prH.GetLogs)
	pr.Get("/:id/lines-with-po-status", prH.LinesWithPOStatus)
	pr.Post("/:id/attachments", attachH.Add)
	pr.Get("/:id/attachments", attachH.List)
	pr.Delete("/:id/attachments/:attach_id", attachH.Delete)

	RegisterPRApprovalRoutes(pr, db)

	// Purchase Order
	po := api.Group("/po")
	po.Use(jwt)
	po.Post("/", poH.Create)
	po.Post("/:id/approve", middleware.RequireRole("MANAGER", "DIRECTOR", "MD", "ADMIN"), poH.Approve)
	po.Post("/:id/send", poH.Send)
	po.Post("/:id/lines", poH.AddLines)
	po.Put("/:id/lines/:line_id", poH.UpdateLine)
	po.Get("/:id/logs", poH.GetLogs)
	RegisterPOApprovalRoutes(po, db)

	// Memo
	memo := api.Group("/memo", jwt)
	memo.Get("/", memoH.List)
	memo.Post("/", memoH.Create)
	memo.Get("/:id", memoH.GetByID)
	memo.Put("/:id", memoH.Update)
	memo.Delete("/:id", memoH.Delete)
	memo.Post("/:id/submit", memoH.Submit)
	memo.Post("/:id/approve", middleware.RequireRole("SENIOR_TEAM", "MANAGER", "ADMIN"), memoH.Approve)

	// GRN
	grn := api.Group("/grn")
	grn.Get("/", grnH.List)
	grn.Post("/", grnH.Create)
	grn.Post("/:id/confirm", grnH.Confirm)

	// Approvals & Audit
	approvals := api.Group("/approvals")
	approvals.Get("/pending", approvalH.Pending)
	approvals.Get("/logs", approvalH.Logs)
	approvals.Get("/audit", approvalH.AuditLogs)

	// Users
	users := api.Group("/users")
	users.Get("/allUser", usersH.List)
	users.Get("/:id", usersH.Get)
	users.Post("/", middleware.RequireRole("ADMIN"), usersH.Create)
	users.Put("/:id", middleware.RequireRole("ADMIN"), usersH.Update)
	users.Delete("/:id", middleware.RequireRole("ADMIN"), usersH.Delete)

	// Material Code Management (unit → group → subgroup → mat_name → spec → brand → material_code)
	mc := api.Group("/matcode")
	mc.Get("/units", matH.ListUnits)
	mc.Get("/units/:code", matH.GetUnit)
	mc.Post("/units", matH.UpsertUnit)
	mc.Delete("/units/:code", matH.DeleteUnit)

	mc.Get("/subgroups", matH.ListSubgroups)
	mc.Get("/subgroups/:code", matH.GetSubgroup)
	mc.Post("/subgroups", matH.UpsertSubgroup)
	mc.Delete("/subgroups/:code", matH.DeleteSubgroup)

	mc.Get("/matnames", matH.ListMatNames)
	mc.Get("/matnames/:subgroup/:code", matH.GetMatName)
	mc.Post("/matnames", matH.UpsertMatName)
	mc.Delete("/matnames/:subgroup/:code", matH.DeleteMatName)

	mc.Get("/specs", matH.ListSpecSizes)
	mc.Get("/specs/:matname/:code", matH.GetSpecSize)
	mc.Post("/specs", matH.UpsertSpecSize)
	mc.Delete("/specs/:matname/:code", matH.DeleteSpecSize)

	mc.Get("/brands", matH.ListBrands)
	mc.Get("/brands/:code", matH.GetBrand)
	mc.Post("/brands", matH.UpsertBrand)
	mc.Delete("/brands/:code", matH.DeleteBrand)

	mc.Get("/codes", matH.ListMaterialCodes)
	mc.Get("/codes/:code", matH.GetMaterialCode)
	mc.Post("/codes", matH.CreateMaterialCode)
	mc.Put("/codes/:code", matH.UpdateMaterialCode)
	mc.Delete("/codes/:code", matH.DeleteMaterialCode)

	// Groups
	groups := api.Group("/groups", jwt)
	groups.Get("/", groupH.ListGroups)
	groups.Get("/:code", groupH.GetGroup)
	groups.Post("/", groupH.CreateGroup)
	groups.Put("/:code", groupH.UpdateGroup)
	groups.Delete("/:code", groupH.DeleteGroup)
	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "erp-api"})
	})
}

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
	goodsReceiptH := handlers.NewGoodsReceiptHandler(db)
	approvalH := handlers.NewApprovalHandler(db)
	usersH := handlers.NewUsersHandler(db, cfg)
	groupH := handlers.NewGroupHandler(db)
	attachH := handlers.NewAttachmentHandler(db)
	matRepo := repository.NewMaterialRepo(db)
	matSvc := service.NewMaterialService(matRepo)
	matH := handlers.NewMaterialCodeHandler(matSvc)
	memoH := handlers.NewMemoHandler(db)
	costCodeH := handlers.NewCostCodeHandler(db)

	api := app.Group("/api/v1")

	// ── Public ──────────────────────────────────────────────────────────────
	auth := api.Group("/auth")
	auth.Post("/login", authH.Login)
	auth.Post("/refresh", authH.RefreshToken)

	// ── Protected ───────────────────────────────────────────────────────────
	jwt := middleware.JWTProtected(cfg)

	// Auth
	auth.Get("/me", jwt, authH.Me)
	auth.Post("/change-password", jwt, authH.ChangePassword)

	// Master data
	master := api.Group("/master")
	master.Use(jwt)
	master.Get("/groups", masterH.ListGroups)
	master.Put("/groups/:id", masterH.UpdateMatGroup)
	master.Get("/units", masterH.ListUnits)
	master.Put("/units/:id", masterH.UpdateUnit)
	master.Get("/roles", masterH.ListRoles)
	master.Get("/eligible-approvers", masterH.ListEligibleApprovers)
	master.Get("/materials", masterH.ListMaterials)
	master.Post("/materials", masterH.CreateMaterial)
	master.Post("/materials/bulk", masterH.BulkCreateMaterial)
	master.Get("/materials/export", jwt, masterH.ExportMaterials)
	master.Get("/materials/stats", masterH.GetMaterialStats)
	master.Put("/materials/:code", masterH.UpdateMaterial)
	master.Get("/materials/:code", masterH.GetMaterial)
	master.Get("/allMaterial", masterH.GetAllMaterial)

	// Cost Code hierarchy (subject → job → group → subgroup)
	// Cost Code is selected per PR line item (purchase_request_line.cost_subgroup_id),
	// not assigned to a material — there is no material↔cost-code linking endpoint.
	master.Get("/cost-code/subjects", costCodeH.ListSubjects)
	master.Get("/cost-code/jobs", costCodeH.ListJobs)
	master.Get("/cost-code/groups", costCodeH.ListGroupsByJob)
	master.Get("/cost-code/subgroups", costCodeH.ListSubgroupsByGroup)
	master.Get("/cost-code/full", costCodeH.ListFull)
	master.Get("/cost-code/options", costCodeH.GetCostCodeOptions)
	master.Get("/cost-code/export", costCodeH.ExportCostCode)
	master.Post("/cost-code/import", costCodeH.ImportCostCode)
	master.Get("/cost-code/list", costCodeH.ListCostCode)
	master.Get("/cost-code/stats", costCodeH.GetCostCodeStats)

	// Material type-ahead search (Create PO combobox)
	api.Get("/materials/search", masterH.SearchMaterials)
	master.Get("/subgroups", masterH.ListSubgroups)
	master.Put("/subgroups/:id", masterH.UpdateSubgroup)
	master.Get("/mat-names", masterH.ListMatNames)
	master.Put("/mat-names/:id", masterH.UpdateMatName)
	master.Get("/specs", masterH.ListSpecs)
	master.Put("/specs/:id", masterH.UpdateSpecSize)
	master.Get("/brands", masterH.ListBrands)
	master.Put("/brands/:id", masterH.UpdateBrand)
	master.Get("/locations", locationH.ListLocations)
	master.Get("/locations/:id", locationH.GetLocation)
	master.Post("/locations", locationH.CreateLocation)
	master.Put("/locations/:id", locationH.UpdateLocation)
	master.Delete("/locations/:id", locationH.DeleteLocation)

	master.Get("/projects", projectH.List)
	master.Get("/projects/:id", projectH.GetByID)
	master.Post("/projects", projectH.Create)
	master.Put("/projects/:id", projectH.Update)
	master.Delete("/projects/:id", projectH.SoftDelete)
	master.Get("/warehouses", masterH.ListWarehouses)
	master.Post("/warehouses", masterH.CreateWarehouse)
	master.Get("/warehouses/:code/zones", masterH.ListZones)
	supplierH := handlers.NewSupplierHandler(db)
	master.Get("/suppliers", supplierH.ListSuppliers)
	master.Get("/suppliers/:code", supplierH.GetSupplier)
	master.Post("/suppliers", supplierH.CreateSupplier)
	master.Put("/suppliers/:code", supplierH.UpdateSupplier)
	master.Delete("/suppliers/:code", supplierH.DeleteSupplier)
	master.Post("/suppliers/bulk", supplierH.BulkCreateSupplier)

	// Inventory
	inv := api.Group("/inventory")
	inv.Get("/", inventoryH.ListInventory)
	inv.Post("/transactions", inventoryH.CreateTransaction)
	inv.Get("/transactions", inventoryH.ListTransactions)

	// File upload
	upload := api.Group("/upload")
	upload.Post("/pr", attachH.UploadPRFile)
	upload.Post("/memo", attachH.UploadMemoFile)
	upload.Post("/po", attachH.UploadPOFile)

	// Purchase Request
	pr := api.Group("/pr")
	pr.Use(jwt)
	pr.Get("/list-test", func(c *fiber.Ctx) error {
		return c.SendString("list works")
	})
	pr.Get("/next-number", prH.NextNumber)
	pr.Post("/", prH.Create)
	pr.Put("/:id", prH.Update)
	pr.Post("/:id/submit", prH.Submit)
	pr.Put("/:id/reopen", prH.Reopen)
	pr.Get("/:id/logs", prH.GetLogs)
	pr.Get("/:id/lines-with-po-status", prH.LinesWithPOStatus)
	pr.Post("/:id/attachments", attachH.Add)
	pr.Get("/:id/attachments", attachH.List)
	pr.Delete("/:id/attachments/:attach_id", attachH.Delete)

	RegisterPRApprovalRoutes(pr, db)

	// Purchase Order
	po := api.Group("/po")
	po.Use(jwt)
	po.Get("/next-number", poH.NextPONumber)
	po.Get("/search", goodsReceiptH.SearchApprovedPO)
	po.Get("/available-prs", poH.GetAvailablePRs)
	po.Get("/receivable", poH.GetReceivablePOs)
	po.Get("/line-items", poH.ListLineItems)
	po.Get("/", poH.List)
	po.Get("/:id", poH.Get)
	po.Get("/:id/receivable-lines", poH.GetReceivableLines)
	po.Post("/", poH.Create)
	po.Put("/:id", poH.Update)
	po.Post("/:id/approve", middleware.RequireRole("MANAGER", "DIRECTOR", "MD", "ADMIN_CENTER"), poH.Approve)
	po.Put("/:id/edit-approved", poH.EditApprovedPO)
	po.Post("/:id/send", poH.Send)
	po.Post("/:id/lines", poH.AddLines)
	po.Put("/:id/lines/:line_id", poH.UpdateLine)
	po.Get("/:id/logs", poH.GetLogs)
	po.Get("/:id/print-data", poH.PrintData)
	po.Post("/:id/attachments", attachH.AddPO)
	po.Get("/:id/attachments", attachH.ListPO)
	po.Delete("/:id/attachments/:attach_id", attachH.DeletePO)
	// TODO: RegisterPOApprovalRoutes registers GET "/" and GET "/:id" again on the same
	// group — poH.List/poH.Get above (lines 149-150) win since Fiber matches the first
	// registered route, so POApprovalHandler.List/GetDetail are currently unreachable dead
	// code. Needs a separate task to resolve (same bug class as the PR module's shadowed
	// route, see CLAUDE.md).
	RegisterPOApprovalRoutes(po, db)

	// Memo
	memo := api.Group("/memo", jwt)
	memo.Get("/", memoH.List)
	memo.Post("/", memoH.Create)
	memo.Get("/:id", memoH.GetByID)
	memo.Put("/:id", memoH.Update)
	memo.Delete("/:id", memoH.Delete)
	memo.Post("/:id/submit", memoH.Submit)
	memo.Post("/:id/approve", memoH.Approve)
	memo.Patch("/:id/cancel", memoH.Cancel)

	// GRN
	grn := api.Group("/grn")
	grn.Get("/", grnH.List)
	grn.Post("/", grnH.Create)
	grn.Post("/:id/confirm", grnH.Confirm)
	grn.Get("/history", jwt, goodsReceiptH.History)
	grn.Post("/receive", jwt, goodsReceiptH.Receive)
	grn.Post("/:id/score", jwt, goodsReceiptH.Score)

	// Approvals & Audit
	approvals := api.Group("/approvals")
	approvals.Get("/pending", approvalH.Pending)
	approvals.Get("/logs", approvalH.Logs)
	approvals.Get("/audit", approvalH.AuditLogs)

	// Approval Request — my-status (backs Approve/Reject button gating on the frontend)
	approvalReq := api.Group("/approval-request", jwt)
	approvalReq.Get("/:docType/:docId/my-status", approvalH.MyApprovalStatus)

	// Generic, config-driven Approve/Reject (approval_doc_types-based). New doc type =
	// approval_doc_types + approval_config rows, zero new Go code. Currently configured for
	// PO and MEMO; supersedes po_approval.go/memo.go's Approve for those two doc types, but
	// the old handlers are left registered and unused-by-frontend for now (rollback safety).
	genericApprovalH := handlers.NewGenericApprovalHandler(db)
	genericApproval := api.Group("/approval", jwt)
	genericApproval.Put("/:docType/:docId/approve", genericApprovalH.Approve)
	genericApproval.Put("/:docType/:docId/reject", genericApprovalH.Reject)

	// Users
	users := api.Group("/users")
	users.Use(jwt)
	users.Get("/allUser", usersH.List)
	users.Get("/:id", usersH.Get)
	users.Post("/", middleware.RequireRole("ADMIN_CENTER"), usersH.Create)
	users.Put("/:id", middleware.RequireRole("ADMIN_CENTER"), usersH.Update)
	users.Put("/:id/roles", middleware.RequireRole("ADMIN_CENTER"), usersH.UpdateRoles)
	users.Put("/:id/password", middleware.RequireRole("ADMIN_CENTER"), usersH.ResetPassword)
	users.Delete("/:id", middleware.RequireRole("ADMIN_CENTER"), usersH.Delete)

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

	// Stock Management
	stockItemH := handlers.NewStockItemHandler(db)
	stockItemImgH := handlers.NewStockItemImageHandler(db)
	stockInvH := handlers.NewStockInventoryHandler(db)
	stockTxnH := handlers.NewStockTransactionHandler(db)
	stockBorrowH := handlers.NewStockBorrowHandler(db)
	stockResH := handlers.NewStockReservationHandler(db)

	stock := api.Group("/stock", jwt)

	// Item Master
	stock.Get("/categories", stockItemH.ListCategories)
	stock.Get("/items", stockItemH.List)
	stock.Get("/lookup", stockItemH.Lookup)
	stock.Post("/items", stockItemH.Create)
	stock.Post("/items/import", stockItemH.ImportExcel)
	stock.Post("/items/bulk/preview", stockItemH.PreviewImportExcel)
	stock.Get("/items/:id", stockItemH.GetByID)
	stock.Put("/items/:id", stockItemH.Update)
	stock.Delete("/items/:id", stockItemH.SoftDelete)

	stock.Post("/items/:id/images", stockItemImgH.UploadImages)
	stock.Get("/items/:id/images", stockItemImgH.ListImages)
	stock.Put("/items/:id/images/:image_id/primary", stockItemImgH.SetPrimary)
	stock.Delete("/items/:id/images/:image_id", stockItemImgH.DeleteImage)

	// Inventory
	stock.Get("/inventory", stockInvH.List)
	stock.Post("/inventory/adjust", stockInvH.AdjustStock)
	stock.Post("/inventory/batch-lookup", stockInvH.BatchLookup)
	stock.Post("/inventory/transfer", stockInvH.Transfer)

	// Transactions
	stock.Get("/transactions", stockTxnH.List)
	stock.Post("/transactions", stockTxnH.Create)

	// Borrow & Return
	stock.Get("/borrow", stockBorrowH.List)
	stock.Post("/borrow", stockBorrowH.Create)
	stock.Get("/borrow/scan/:item_code", stockBorrowH.ScanQR)
	stock.Get("/borrow/:id", stockBorrowH.GetByID)
	stock.Post("/borrow/:id/submit", stockBorrowH.Submit)
	stock.Post("/borrow/:id/approve", middleware.RequireRole("MANAGER", "ADMIN_CENTER"), stockBorrowH.Approve)
	stock.Post("/borrow/:id/receive", stockBorrowH.Receive)
	stock.Post("/borrow/:id/return", stockBorrowH.Return)

	// Reservation
	stock.Get("/reservations", stockResH.List)
	stock.Post("/reservations", stockResH.Create)
	stock.Post("/reservations/:id/cancel", stockResH.Cancel)

	// Requisition (ใบเบิกของ)
	requisitionH := handlers.NewRequisitionHandler(db)
	requisition := api.Group("/requisition", jwt)
	requisition.Get("/", requisitionH.List)
	requisition.Post("/", requisitionH.Create)
	requisition.Get("/history", requisitionH.History)
	requisition.Get("/:id", requisitionH.Get)
	requisition.Post("/:id/confirm", middleware.RequireRole("STOCK", "ADMIN"), requisitionH.Confirm)
	requisition.Post("/:id/cancel", requisitionH.Cancel)

	// Work Order (หนังสือสั่งจ้าง) — approve/reject goes through the generic engine
	// (PUT /approval/WO/:id/approve|reject), not a route here.
	workOrderH := handlers.NewWorkOrderHandler(db)
	workOrder := api.Group("/work-order", jwt)
	workOrder.Get("/", workOrderH.List)
	workOrder.Post("/", workOrderH.Create)
	workOrder.Get("/:id", workOrderH.Get)
	workOrder.Post("/:id/submit", workOrderH.Submit)

	// Stock Transfer (ย้ายคลัง)
	stockTransferH := handlers.NewStockTransferHandler(db)
	stockTransfer := api.Group("/stock-transfer", jwt)
	stockTransfer.Get("/history", stockTransferH.History) // static route before :id
	stockTransfer.Get("/", stockTransferH.List)
	stockTransfer.Post("/", stockTransferH.Create)
	stockTransfer.Get("/:id", stockTransferH.Get)
	stockTransfer.Post("/:id/confirm", middleware.RequireRole("STOCK", "ADMIN"), stockTransferH.Confirm)
	stockTransfer.Post("/:id/cancel", stockTransferH.Cancel)

	// Role-Menu Permissions
	roleMenuH := handlers.NewRoleMenuPermissionHandler(db)
	roleMenu := api.Group("/role-menu-permissions")
	roleMenu.Get("/", roleMenuH.List)
	roleMenu.Post("/batch", roleMenuH.BatchUpsert)

	menusRoute := api.Group("/menus")
	menusRoute.Get("/", roleMenuH.ListMenus)

	// Approval Doc Types
	docTypeH := handlers.NewApprovalDocTypeHandler(db)
	docTypes := api.Group("/approval-doc-types")
	docTypes.Get("/", docTypeH.List)
	docTypes.Post("/", docTypeH.Create)
	docTypes.Patch("/:id", docTypeH.Toggle)

	// Approval Roles
	approvalRoleH := handlers.NewApprovalRoleHandler(db)
	approvalRoles := api.Group("/roles")
	approvalRoles.Get("/", approvalRoleH.List)
	approvalRoles.Post("/", approvalRoleH.Create)
	approvalRoles.Patch("/:id", approvalRoleH.Toggle)
	approvalRoles.Delete("/:id", approvalRoleH.Delete)

	// Approval Config
	approvalCfgH := handlers.NewApprovalConfigHandler(db)
	approvalCfg := api.Group("/approval-config")
	approvalCfg.Get("/", approvalCfgH.List)
	approvalCfg.Patch("/:id", approvalCfgH.Toggle)
	approvalCfg.Delete("/:id", approvalCfgH.Delete)
	approvalCfg.Post("/batch", approvalCfgH.BatchUpdate)

	// Approval Delegation (extra approvers, additive on top of role-based approval_config)
	approvalDelegationH := handlers.NewApprovalDelegationHandler(db)
	approvalDelegation := api.Group("/approval-delegation")
	approvalDelegation.Get("/", approvalDelegationH.List)
	approvalDelegation.Post("/", approvalDelegationH.Create)
	approvalDelegation.Delete("/:id", approvalDelegationH.Delete)

	// Departments
	deptH := handlers.NewDepartmentHandler(db)
	api.Get("/departments", deptH.List)

	// Dept-Menu Permissions
	deptPermH := handlers.NewDeptMenuPermissionHandler(db)
	deptPerm := api.Group("/dept-menu-permissions")
	deptPerm.Get("/", deptPermH.List)
	deptPerm.Post("/batch", deptPermH.BatchUpsert)

	// User-Menu Permissions
	userPermH := handlers.NewUserMenuPermissionHandler(db)
	userPerm := api.Group("/user-menu-permissions")
	userPerm.Post("/batch", userPermH.BatchUpsert)
	userPerm.Delete("/", userPermH.DeleteByUser)

	// Effective Permissions
	effectiveH := handlers.NewEffectivePermissionHandler(db)
	api.Get("/permissions/effective", effectiveH.GetEffective)

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "erp-api"})
	})
}

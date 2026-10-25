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
	app.Static("/uploads", handlers.UploadRoot())

	// Absolutize relative file_path values returned in attachment/image responses
	handlers.SetPublicBaseURL(cfg.PublicBaseURL)

	// Handler instances
	authH := handlers.NewAuthHandler(db, cfg)
	masterH := handlers.NewMasterHandler(db)
	locationH := handlers.NewLocationHandler(db)
	projectH := handlers.NewProjectHandler(db)
	projectOverviewH := handlers.NewProjectOverviewHandler(db)
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
	pettyCashH := handlers.NewPettyCashHandler(db)

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
	master.Get("/units/used", masterH.ListUsedUnits)
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
	master.Get("/materials/:code/price-history", masterH.GetMaterialPriceHistory)
	master.Get("/allMaterial", masterH.GetAllMaterial)

	// Cost Code hierarchy (subject → job → group → subgroup)
	// Cost Code is selected per PR line item (purchase_request_line.cost_subgroup_id),
	// not assigned to a material — there is no material↔cost-code linking endpoint.
	master.Get("/cost-code/subjects", costCodeH.ListSubjects)
	master.Get("/cost-code/jobs", costCodeH.ListJobs)
	master.Get("/cost-code/groups", costCodeH.ListGroupsByJob)
	master.Post("/cost-code/groups", costCodeH.CreateGroup)
	master.Patch("/cost-code/groups/:id", costCodeH.UpdateGroup)
	master.Patch("/cost-code/groups/:id/deactivate", costCodeH.DeactivateGroup)
	master.Get("/cost-code/subgroups", costCodeH.ListSubgroupsByGroup)
	master.Post("/cost-code/subgroups", costCodeH.CreateSubgroup)
	master.Patch("/cost-code/subgroups/:id/head", costCodeH.SetSubgroupHead)
	master.Patch("/cost-code/subgroups/:id/major", costCodeH.SetSubgroupMajor)
	master.Patch("/cost-code/subgroups/:id", costCodeH.UpdateSubgroup)
	master.Patch("/cost-code/subgroups/:id/deactivate", costCodeH.DeactivateSubgroup)
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

	master.Get("/job-types", projectH.ListJobTypes)
	master.Get("/projects", projectH.List)
	master.Get("/projects/next-code", projectH.NextCode) // static, before /projects/:id
	master.Get("/projects/options", projectH.Options)   // static, before /projects/:id — unpaginated dropdown list
	master.Get("/projects/:id", projectH.GetByID)
	master.Post("/projects", projectH.Create)
	master.Post("/projects/import", projectH.Import)
	master.Get("/projects/import/template", projectH.ImportTemplate)
	master.Put("/projects/:id", projectH.Update)
	master.Delete("/projects/:id", projectH.SoftDelete)
	master.Get("/warehouses", masterH.ListWarehouses)
	master.Post("/warehouses", masterH.CreateWarehouse)
	master.Get("/warehouses/:code/zones", masterH.ListZones)
	supplierH := handlers.NewSupplierHandler(db)
	master.Get("/suppliers", supplierH.ListSuppliers)
	master.Get("/suppliers/:id", supplierH.GetSupplier)
	master.Post("/suppliers", supplierH.CreateSupplier)
	master.Put("/suppliers/:id", supplierH.UpdateSupplier)
	master.Delete("/suppliers/:id", supplierH.DeleteSupplier)
	master.Post("/suppliers/bulk", supplierH.BulkCreateSupplier)

	// Supplier (standalone group — bulk insert)
	supplier := api.Group("/supplier")
	supplier.Use(jwt)
	supplier.Post("/bulk", supplierH.BulkInsertSupplier)

	// Project Overview (cross-entity search: projects + approved PO line items)
	projectOverview := api.Group("/project-overview")
	projectOverview.Use(jwt)
	projectOverview.Get("/", projectOverviewH.List) // money columns: Contract Budget / Pu / Ac / Status
	projectOverview.Get("/search", projectOverviewH.Search)

	// Inventory
	inv := api.Group("/inventory")
	inv.Get("/", inventoryH.ListInventory)
	inv.Post("/transactions", inventoryH.CreateTransaction)
	inv.Get("/transactions", inventoryH.ListTransactions)

	// File upload
	// NOTE: /upload/pr and /upload/memo remain — both PR and Memo create/update accept a
	// batch of already-uploaded attachment refs in the same JSON body as the rest of the
	// document, before the PR/Memo id exists, so a pre-upload step is still required there.
	// /upload/po was removed: PO attachments are only ever added to an existing PO, so
	// POST /po/{id}/attachments now accepts the file directly (see AddPO in attachment.go).
	upload := api.Group("/upload")
	upload.Post("/pr", attachH.UploadPRFile)
	upload.Post("/memo", attachH.UploadMemoFile)

	// Purchase Request
	pr := api.Group("/pr")
	pr.Use(jwt)
	pr.Get("/list-test", func(c *fiber.Ctx) error {
		return c.SendString("list works")
	})
	pr.Post("/", prH.Create)
	pr.Put("/:id", prH.Update)
	pr.Delete("/:id", prH.Delete)
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
	po.Get("/search", goodsReceiptH.SearchApprovedPO)
	po.Get("/available-prs", poH.GetAvailablePRs)
	po.Get("/pr-lines/:pr_id", poH.GetPRLinesForPO)
	po.Get("/receivable", poH.GetReceivablePOs)
	po.Get("/line-items", poH.ListLineItems)
	po.Get("/export", poH.ExportSummary)
	po.Get("/list-export", poH.ListExport)
	po.Get("/cost-budget", poH.CostBudget)
	po.Get("/cost-budget/:projectCode/cost-codes/:costSubgroupId/po-lines", poH.CostBudgetPOLines)
	po.Get("/:projectCode/cost-budget-detail", poH.CostBudgetDetail)
	po.Get("/:projectCode/cost-budget-detail/:subgroupId/purchases", poH.CostBudgetDetailPurchases)
	po.Get("/", poH.List)
	po.Get("/:id", poH.Get)
	po.Get("/:id/receivable-lines", poH.GetReceivableLines)
	po.Post("/", poH.Create)
	po.Put("/:id", poH.Update)
	po.Delete("/:id", poH.Delete)
	po.Post("/:id/approve", middleware.RequireRole("MANAGER", "DIRECTOR", "MD", "ADMIN_CENTER"), poH.Approve)
	po.Put("/:id/edit-approved", poH.EditApprovedPO)
	po.Post("/:id/send", poH.Send)
	po.Put("/:id/deposit-paid", poH.SetDepositPaid)
	po.Post("/:id/lines", poH.AddLines)
	po.Put("/:id/lines/:line_id", poH.UpdateLine)
	po.Get("/:id/logs", poH.GetLogs)
	po.Get("/:id/print-data", poH.PrintData)
	po.Post("/:id/attachments", attachH.AddPO)
	po.Get("/:id/attachments", attachH.ListPO)
	po.Delete("/:id/attachments/:attach_id", attachH.DeletePO)
	// RegisterPOApprovalRoutes only adds PUT approve/reject/cancel here now — its GET "/" and
	// GET "/:id" (POApprovalHandler.List/GetDetail) were removed as unreachable dead code,
	// permanently shadowed by poH.List/poH.Get above. See internal/routes/po.go.
	RegisterPOApprovalRoutes(po, db)

	// Memo
	memo := api.Group("/memo", jwt)
	memo.Get("/", memoH.List)
	memo.Post("/", memoH.Create)
	memo.Get("/:id/available-lines", memoH.AvailableLines)
	memo.Get("/:id", memoH.GetByID)
	memo.Put("/:id", memoH.Update)
	memo.Delete("/:id", memoH.Delete)
	memo.Post("/:id/submit", memoH.Submit)
	memo.Post("/:id/approve", memoH.Approve)
	memo.Patch("/:id/cancel", memoH.Cancel)

	// Petty Cash Requisition (ใบเบิกเงินสดย่อย)
	// Material picker reuses GET /master/allMaterial (extended with an optional
	// project_code param for stock_on_hand) — no petty-cash-specific search endpoint.
	pc := api.Group("/petty-cash", jwt)
	pc.Get("/line-options", pettyCashH.LineOptions) // static — must stay before /:id
	pc.Get("/pending-approval", pettyCashH.PendingApproval) // static - must stay before /:id
	pc.Get("/", pettyCashH.List)
	pc.Post("/", pettyCashH.Create)
	pc.Get("/:id/attachments", pettyCashH.ListAttachments)
	pc.Post("/:id/attachments", pettyCashH.AddAttachment)
	pc.Delete("/:id/attachments/:attId", pettyCashH.DeleteAttachment)
	pc.Get("/:id/approval-chain", pettyCashH.ApprovalChain)
	pc.Get("/:id", pettyCashH.Get)
	pc.Put("/:id", pettyCashH.Update)
	pc.Delete("/:id", pettyCashH.Delete)
	pc.Post("/:id/submit", pettyCashH.Submit)
	pc.Post("/:id/approve", pettyCashH.Approve)
	pc.Post("/:id/reject", pettyCashH.Reject)
	pc.Post("/:id/cancel", pettyCashH.Cancel)

	// GRN
	grn := api.Group("/grn")
	grn.Get("/", grnH.List)
	grn.Get("/reserve-number", jwt, goodsReceiptH.ReserveGRNNumber)
	grn.Post("/", grnH.Create)
	grn.Post("/:id/confirm", grnH.Confirm)
	grn.Get("/history", jwt, goodsReceiptH.History)
	grn.Post("/receive", jwt, goodsReceiptH.Receive)
	grn.Post("/:id/score", jwt, goodsReceiptH.Score)

	// Approvals & Audit
	approvals := api.Group("/approvals")
	approvals.Get("/pending", approvalH.Pending)
	approvals.Get("/my-pending-counts", approvalH.MyPendingCounts) // Memo/PO/WO approver-scoped badge counts
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
	// PETTY_CASH is handled by its own Approve/Reject (writes approval_log + approver remark, Thai errors);
	// registered BEFORE the generic /:docType routes so Fiber matches these first.
	genericApproval.Put("/PETTY_CASH/:docId/approve", pettyCashH.Approve)
	genericApproval.Put("/PETTY_CASH/:docId/reject", pettyCashH.Reject)
	genericApproval.Put("/:docType/:docId/approve", genericApprovalH.Approve)
	genericApproval.Put("/:docType/:docId/reject", genericApprovalH.Reject)

	// Users
	users := api.Group("/users")
	users.Use(jwt)
	users.Get("/allUser", usersH.List)
	users.Get("/options", usersH.Options) // static, before /users/:id
	users.Get("/:id", usersH.Get)
	users.Get("/:id/signature", usersH.GetSignature)
	users.Post("/", middleware.RequireRole("ADMIN_CENTER"), usersH.Create)
	users.Post("/:id/signature", middleware.RequireRole("ADMIN_CENTER"), usersH.UploadSignature)
	users.Put("/:id", middleware.RequireRole("ADMIN_CENTER"), usersH.Update)
	users.Put("/:id/roles", middleware.RequireRole("ADMIN_CENTER"), usersH.UpdateRoles)
	users.Put("/:id/password", middleware.RequireRole("ADMIN_CENTER"), usersH.ResetPassword)
	users.Delete("/:id", middleware.RequireRole("ADMIN_CENTER"), usersH.Delete)
	users.Delete("/:id/signature", middleware.RequireRole("ADMIN_CENTER"), usersH.DeleteSignature)

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
	// Borrow & Return retired (2026-09-25): /stock/borrow routes intentionally unregistered — see stock_borrow.go
	// (kept, reversible). Its rollupStockItemQty would zero stock_item.qty while stock_inventory is empty.
	stockResH := handlers.NewStockReservationHandler(db)

	stock := api.Group("/stock", jwt)

	// Project-first Stock page (/master/stock): project list -> balance -> history. Registered before
	// the other /stock routes; the two stock systems are returned typed by `source`, never UNIONed.
	stockByProjectH := handlers.NewStockByProjectHandler(db)
	stock.Get("/by-project", stockByProjectH.ListProjects)
	stock.Get("/by-project/:projectCode/balance", stockByProjectH.GetBalance)
	stock.Get("/by-project/:projectCode/transactions", stockByProjectH.ListTransactions)

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

	// Reservation
	stock.Get("/reservations", stockResH.List)
	stock.Post("/reservations", stockResH.Create)
	stock.Post("/reservations/:id/cancel", stockResH.Cancel)

	// Requisition (ใบเบิกของ)
	requisitionH := handlers.NewRequisitionHandler(db)
	requisition := api.Group("/requisition", jwt)
	requisition.Get("/", requisitionH.List)
	requisition.Get("/reserve-number", requisitionH.ReserveReqNumber)
	requisition.Post("/", requisitionH.Create)
	requisition.Get("/history", requisitionH.History)
	requisition.Get("/:id", requisitionH.Get)
	requisition.Post("/:id/confirm", middleware.RequireRole("STOCK", "ADMIN"), requisitionH.Confirm)
	requisition.Post("/:id/cancel", requisitionH.Cancel)

	// Customer Master
	customerH := handlers.NewCustomerHandler(db)
	customer := api.Group("/customer", jwt)
	customer.Get("/", customerH.List)
	customer.Post("/", customerH.Create)
	customer.Post("/import", customerH.Import)
	customer.Get("/import/template", customerH.ImportTemplate)
	customer.Get("/:id", customerH.Get)
	customer.Put("/:id", customerH.Update)
	customer.Delete("/:id", middleware.RequireMenuPermission(db, "MENU_MASTER_CUSTOMER", "delete"), customerH.Delete)

	// Customer master under /master (same handlers; JWT via the master group, like Supplier).
	// Permission = menu MENU_MASTER_CUSTOMER (user > role > dept precedence). /import is a static
	// route and is registered before every /:id route.
	const customerMenu = "MENU_MASTER_CUSTOMER"
	master.Get("/customer", customerH.List)
	master.Post("/customer", middleware.RequireMenuPermission(db, customerMenu, "write"), customerH.Create)
	master.Post("/customer/import", middleware.RequireMenuPermission(db, customerMenu, "write"), customerH.ImportRows)
	master.Get("/customer/:id", customerH.Get)
	master.Put("/customer/:id", middleware.RequireMenuPermission(db, customerMenu, "update"), customerH.Update)
	master.Delete("/customer/:id", middleware.RequireMenuPermission(db, customerMenu, "delete"), customerH.Delete)

	// Work Order (หนังสือสั่งจ้าง) — approve/reject goes through the generic engine
	// (PUT /approval/WO/:id/approve|reject), not a route here.
	workOrderH := handlers.NewWorkOrderHandler(db)
	workOrder := api.Group("/work-order", jwt)
	workOrder.Get("/", workOrderH.List)
	workOrder.Get("/reserve-number", workOrderH.ReserveWONumber)
	workOrder.Get("/wht-defaults", workOrderH.WhtDefaults)
	workOrder.Post("/", workOrderH.Create)
	workOrder.Get("/:id", workOrderH.Get)
	workOrder.Put("/:id", workOrderH.Update)
	workOrder.Put("/:id/edit-approved", workOrderH.EditApproved)
	workOrder.Post("/:id/submit", workOrderH.Submit)
	workOrder.Put("/:id/lines", workOrderH.UpdateLines)
	workOrder.Get("/:woId/payment-conditions", workOrderH.GetPaymentConditions)
	workOrder.Post("/:woId/payment-conditions", workOrderH.UpdatePaymentConditions)

	woContractorLogH := handlers.NewWOContractorLogHandler(db)
	woContractorLog := api.Group("/wo-contractor-log", jwt)
	woContractorLog.Post("/", woContractorLogH.Create)
	woContractorLog.Get("/bills/:billId", woContractorLogH.GetBill)
	woContractorLog.Put("/bills/:billId", woContractorLogH.UpdateBill)
	woContractorLog.Get("/:id/bills", woContractorLogH.ListBills)
	woContractorLog.Post("/:id/bills", woContractorLogH.CreateBill)
	woContractorLog.Get("/:id", woContractorLogH.Get)
	woContractorLog.Put("/:id", woContractorLogH.Update)

	financeH := handlers.NewFinanceHandler(db)
	finance := api.Group("/finance", jwt)
	finance.Get("/payments", financeH.ListPayableDocs)
	finance.Post("/payments", financeH.RecordPayment)
	finance.Post("/payments/:id/cancel", financeH.CancelPayment)
	finance.Get("/payments/:doc_type/:doc_id/log", financeH.GetPaymentLog)
	finance.Get("/payments/:doc_type/:doc_id/installments", financeH.GetInstallments)
	finance.Put("/payments/:doc_type/:doc_id/installments", financeH.SaveInstallments)
	finance.Get("/payments/:doc_type/:doc_id", financeH.GetPayableDoc)

	// Stock Transfer (ย้ายคลัง)
	stockTransferH := handlers.NewStockTransferHandler(db)
	stockTransfer := api.Group("/stock-transfer", jwt)
	stockTransfer.Get("/history", stockTransferH.History)                      // static route before :id
	stockTransfer.Get("/reserve-number", stockTransferH.ReserveTransferNumber) // static route before :id
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
	// Department master. Reading needs only a login (dropdowns everywhere use it); changes are gated by the menu
	// permission MENU_ADMIN_DEPARTMENTS (user > role > dept precedence). No static sub-routes exist, and all
	// /:id routes come after the collection routes.
	deptH := handlers.NewDepartmentHandler(db)
	const deptMenu = "MENU_ADMIN_DEPARTMENTS"
	departments := api.Group("/departments", jwt)
	departments.Get("/", deptH.List)
	departments.Post("/", middleware.RequireMenuPermission(db, deptMenu, "write"), deptH.Create)
	departments.Put("/:id", middleware.RequireMenuPermission(db, deptMenu, "update"), deptH.Update)
	departments.Delete("/:id", middleware.RequireMenuPermission(db, deptMenu, "delete"), deptH.Delete)

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

	// Inventory Control (IC)
	icH := handlers.NewICHandler(db)
	ic := api.Group("/ic", jwt)
	ic.Get("/preparers", icH.ListPreparers)
	ic.Get("/projects", icH.ListProjects)
	icMoveH := handlers.NewIcProjectMovementHandler(db)
	ic.Get("/projects/by-code/:projectCode", icMoveH.GetProjectByCode)
	ic.Get("/projects/:projectId", icH.GetProject)
	ic.Get("/projects/:projectId/pos", icH.ListProjectPOs)
	ic.Get("/projects/:projectId/po-search-options", icH.POSearchOptions)
	ic.Get("/pos/:poId/header", icH.RequireICPO, icH.GetPOHeader)
	ic.Get("/pos/:poId/documents", icH.RequireICPO, icH.ListPODocuments)
	ic.Get("/pos/:poId/receive-document", icH.RequireICPO, icH.GetReceiveDocument)
	ic.Post("/pos/:poId/receive-document", icH.RequireICPO, icH.SubmitReceiveDocument)
	ic.Get("/pos/:poId/receive-documents", icH.RequireICPO, icH.ListReceiveDocuments)
	ic.Get("/pos/:poId/receive-documents/:docId", icH.RequireICPO, icH.GetReceiveDocumentByID)
	ic.Delete("/pos/:poId/receive-documents/:docId", icH.RequireICPO, icH.DeleteReceiveDocument)
	ic.Put("/pos/:poId/receive-documents/:docId/rating", icH.RequireICPO, icH.RateReceiveDocument)
	ic.Get("/pos/:poId/receive-documents/:docId/lines", icH.RequireICPO, icH.GetReceiveDocumentLines)
	ic.Get("/pos/:poId/receive-lines", icH.RequireICPO, icH.ListReceiveLines)
	ic.Post("/pos/:poId/receive-lines/submit", icH.RequireICPO, icH.SubmitReceiveLines)
	ic.Get("/returns/:id", icH.GetReturnDocument)
	ic.Get("/pos/:poId/returns", icH.RequireICPO, icH.ListReturnDocuments)
	ic.Post("/pos/:poId/return-lines", icH.RequireICPO, icH.SubmitReturnLines)
	ic.Get("/projects/:projectId/return-pos", icH.ListReturnPOs)
	ic.Get("/pos/:poId/return-lines", icH.RequireICPO, icH.ListReturnLines)
	ic.Post("/pos/:poId/return-lines/submit", icH.RequireICPO, icH.SubmitReturnLines)

	// IC Project Movement (Issue/Transfer)
	ic.Get("/projects/:projectCode/job-codes", icMoveH.ListProjectJobCodes)
	ic.Get("/projects/:projectCode/movements", icMoveH.ListMovements)
	ic.Post("/projects/:projectCode/movements", icMoveH.CreateMovement)
	// Single-save flow (no movement id yet) — static paths MUST precede the /:id routes below.
	ic.Get("/projects/:projectCode/movements/available-materials", icMoveH.ListAvailableMaterialsNoMovement)
	ic.Post("/projects/:projectCode/movements/submit", icMoveH.SubmitMovementNew)
	ic.Get("/projects/:projectCode/movements/:id", icMoveH.GetMovement)
	ic.Get("/projects/:projectCode/movements/:id/available-materials", icMoveH.ListAvailableMaterials)
	ic.Get("/projects/:projectCode/cost-codes", icMoveH.ListProjectCostCodes)
	ic.Post("/projects/:projectCode/movements/:id/lines", icMoveH.AddMovementLine)
	ic.Get("/projects/:projectCode/movements/:id/lines", icMoveH.ListMovementLines)
	ic.Post("/projects/:projectCode/movements/:id/submit", icMoveH.SubmitMovement)
	ic.Get("/projects/:projectCode/cost-transactions", icMoveH.ListCostTransactions)
	ic.Get("/projects/:projectCode/stock", icMoveH.ListProjectStock)

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "erp-api"})
	})
}

# ERP API — Skill Guide for Claude Code

## Adding a new feature — checklist
เวลาจะเพิ่ม feature ใหม่ให้ทำตามลำดับนี้เสมอ

1. **Model** → เพิ่ม struct ใน `internal/models/models.go`
2. **Migration** → เพิ่ม table/column ใน `migrations/` (ใช้ `IF NOT EXISTS`)
3. **Handler** → สร้างไฟล์ใน `internal/handlers/` พร้อม Swagger annotation
4. **Route** → ลงทะเบียนใน `internal/routes/routes.go`
5. **Swagger** → รัน `swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal`

---

## Models (internal/models/models.go)

### Pattern: Request / Response แยกกันเสมอ
```go
// DB struct — map จาก query result
type Material struct {
    MatCode  string `json:"mat_code" db:"mat_code"`
    UnitCode string `json:"unit_code" db:"unit_code"`
}

// Request struct — รับจาก client
type CreateMaterialRequest struct {
    MatCode string `json:"mat_code" validate:"required,max=20"`
    // validate tag ใช้สำหรับ document เท่านั้น — validate เองใน handler
}
```

### Nullable fields ใช้ pointer
```go
type Foo struct {
    Name     string  `json:"name"`           // NOT NULL
    Remarks  *string `json:"remarks,omitempty"` // nullable
    ParentID *int64  `json:"parent_id,omitempty"`
}
```

---

## Handlers (internal/handlers/)

### File naming
- 1 domain = 1 file เช่น `rfq.go`, `borrow.go`, `stock_count.go`
- ถ้า domain เล็กมาก รวมได้ เช่น `grn_approval.go`

### Handler struct template
```go
package handlers

import (
    "context"
    "erp-api/internal/middleware"
    "erp-api/internal/models"
    "github.com/gofiber/fiber/v2"
    "github.com/jackc/pgx/v5/pgxpool"
)

type RFQHandler struct {
    db *pgxpool.Pool
}

func NewRFQHandler(db *pgxpool.Pool) *RFQHandler {
    return &RFQHandler{db: db}
}
```

### Swagger annotation — ต้องครบทุก endpoint
```go
// Create godoc
// @Summary      Create RFQ
// @Description  Send request for quotation to supplier
// @Tags         RFQ
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateRFQRequest  true  "RFQ data"
// @Success      201   {object}  fiber.Map
// @Failure      400   {object}  fiber.Map
// @Failure      401   {object}  fiber.Map
// @Router       /rfq [post]
func (h *RFQHandler) Create(c *fiber.Ctx) error {
```

### BodyParser pattern
```go
var req models.CreateXxxRequest
if err := c.BodyParser(&req); err != nil {
    return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
}
// validate manually
if req.SupplierCode == "" {
    return fiber.NewError(fiber.StatusBadRequest, "supplier_code is required")
}
```

### Get claims (current user)
```go
claims := middleware.GetClaims(c)
// claims.UserID  int64
// claims.Username string
// claims.Roles   []string
```

### Query single row
```go
var item models.Foo
err := h.db.QueryRow(context.Background(),
    `SELECT id, name FROM foo WHERE id=$1`, id,
).Scan(&item.ID, &item.Name)
if err != nil {
    return fiber.NewError(fiber.StatusNotFound, "not found")
}
```

### Query multiple rows
```go
rows, err := h.db.Query(context.Background(),
    `SELECT id, name FROM foo ORDER BY id LIMIT $1 OFFSET $2`, size, offset)
if err != nil {
    return err
}
defer rows.Close()

var items []models.Foo
for rows.Next() {
    var f models.Foo
    rows.Scan(&f.ID, &f.Name)
    items = append(items, f)
}
```

### Transaction pattern
```go
tx, err := h.db.Begin(context.Background())
if err != nil {
    return err
}
defer tx.Rollback(context.Background())

// ... tx.Exec / tx.QueryRow ...

if err := tx.Commit(context.Background()); err != nil {
    return err
}
```

### Document number generation
```go
var seq int64
h.db.QueryRow(context.Background(),
    `SELECT COALESCE(MAX(rfq_id), 0)+1 FROM rfq`).Scan(&seq)
rfqNo := fmt.Sprintf("RFQ-%s-%06d", time.Now().Format("2006"), seq)
```

### Pagination helper (copy จาก master.go)
```go
page := max(c.QueryInt("page", 1), 1)
size := min(c.QueryInt("page_size", 20), 100)
offset := (page - 1) * size

var total int64
h.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM foo`).Scan(&total)

totalPages := int(total)/size + 1
return c.JSON(fiber.Map{
    "success": true,
    "data": models.PaginatedResponse{
        Data: items, Total: total, Page: page,
        PageSize: size, TotalPages: totalPages,
    },
})
```

### Success responses
```go
// List / Get
return c.JSON(fiber.Map{"success": true, "data": item})

// Create
return c.Status(fiber.StatusCreated).JSON(fiber.Map{
    "success": true,
    "data": fiber.Map{"id": newID, "no": docNo},
})

// Action (approve/submit/send)
return c.JSON(fiber.Map{"success": true, "message": "action completed"})
```

### Error responses — ใช้ fiber.NewError เสมอ (GlobalErrorHandler จัดการ format)
```go
return fiber.NewError(fiber.StatusBadRequest, "message")      // 400
return fiber.NewError(fiber.StatusUnauthorized, "message")    // 401
return fiber.NewError(fiber.StatusForbidden, "message")       // 403
return fiber.NewError(fiber.StatusNotFound, "message")        // 404
return fiber.NewError(fiber.StatusConflict, "message")        // 409
return fiber.NewError(fiber.StatusInternalServerError, "msg") // 500
```

---

## Routes (internal/routes/routes.go)

### Adding new handler
```go
// 1. สร้าง instance
rfqH := handlers.NewRFQHandler(db)

// 2. สร้าง group
rfq := api.Group("/rfq", jwt)

// 3. ลงทะเบียน routes
rfq.Get("/", rfqH.List)
rfq.Post("/", rfqH.Create)
rfq.Get("/:id", rfqH.Get)

// 4. Route ที่ต้องการ role เฉพาะ
rfq.Post("/:id/approve",
    middleware.RequireRole("PURCHASING", "ADMIN"),
    rfqH.Approve)
```

### Available roles สำหรับ RequireRole
```
ADMIN, SENIOR_TEAM, MANAGER, DIRECTOR, MD, PURCHASING, STOCK, ENGINEERING
```

---

## Database changes
This project does NOT use migration files. When a schema change is needed:
- Give the user the raw SQL to run (they execute it directly in pgAdmin Query Tool)
- Always wrap in BEGIN; ... COMMIT; for safety
- Always use IF NOT EXISTS for new tables/columns/indexes so it's safe to re-run
- Never assume the SQL has been applied — the user must confirm they ran it before you rely
  on the new column/table existing
- Do not create files under migrations/ for future schema changes

---

## Inventory transaction types
เวลาสร้าง inventory_transaction ต้องใช้ txn_type ที่กำหนดไว้เท่านั้น

| txn_type | ความหมาย | from_warehouse | to_warehouse |
|---|---|---|---|
| `ISSUE` | เบิกใช้งาน | ✅ warehouse ที่เบิก | ❌ |
| `RETURN` | คืนของเข้าคลัง | ❌ | ✅ warehouse ปลายทาง |
| `TRANSFER_OUT` | โอนออก | ✅ | ✅ |
| `TRANSFER_IN` | โอนเข้า | ❌ | ✅ |
| `ADJUST_PLUS` | ปรับเพิ่ม | ❌ | ✅ |
| `ADJUST_MINUS` | ปรับลด | ✅ | ❌ |
| `GRN_IN` | รับสินค้าจาก PO | ❌ | ✅ |
| `BORROW_OUT` | ยืมออก | ✅ | ❌ |
| `BORROW_RETURN` | คืนจากยืม | ❌ | ✅ |

หลัง insert inventory_transaction ต้อง update `inventory.qty_on_hand` ด้วยเสมอ:
```sql
INSERT INTO inventory (mat_code, warehouse_code, qty_on_hand)
VALUES ($1, $2, $3)
ON CONFLICT (mat_code, warehouse_code)
DO UPDATE SET qty_on_hand = inventory.qty_on_hand + $3, updated_at = NOW()
```

---

## Approval log pattern
เมื่อ approve/reject document ต้อง log ทุกครั้ง

```go
// 1. Update document status
tx.Exec(ctx, `UPDATE purchase_order SET status=$1, updated_at=NOW() WHERE po_id=$2`, newStatus, id)

// 2. Log ใน status_log table
tx.Exec(ctx, `
    INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, remarks)
    VALUES ($1,$2,$3,$4,$5)`, id, oldStatus, newStatus, claims.UserID, comments)

// 3. Log ใน approval_log (ถ้ามี approval_request)
tx.Exec(ctx, `
    INSERT INTO approval_log
      (approval_id, doc_type, doc_id, doc_no, step_no, action, action_by, comments, old_status, new_status)
    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
    approvalID, "PO", id, poNo, 1, action, claims.UserID, comments, oldStatus, newStatus)
```

---

## handlers ที่ยังไม่ได้สร้าง (TODO)
สามารถสร้างตาม pattern ข้างบนได้เลย

| Handler | File | Tables |
|---|---|---|
| RFQ | `internal/handlers/rfq.go` | `rfq`, `rfq_line` |
| Borrow/Return | `internal/handlers/borrow.go` | `borrow`, `borrow_line` |
| Stock Count | `internal/handlers/stock_count.go` | `stock_count`, `stock_count_line` |
| User management | `internal/handlers/user.go` | `users`, `roles`, `user_roles` |

---

## Swagger re-generate (ทำทุกครั้งที่แก้ annotation)
```powershell
swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal
```

ถ้า swag not found:
```powershell
$env:PATH += ";$env:GOPATH\bin"
```

---

## Windows-specific notes
- ใช้ `copy` แทน `cp`
- ใช้ `$env:PATH +=` แทน `export PATH=`
- path separator ใช้ `\` แต่ใน Go code ใช้ `/` ได้ปกติ
- PowerShell รัน `.exe` ต้องใส่ `&` นำหน้า เช่น `& "C:\Program Files\PostgreSQL\18\bin\psql.exe"`

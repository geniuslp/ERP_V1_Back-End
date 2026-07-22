# ERP API — Claude Code Context (BACKEND: erp-api / Go Fiber v2)

> ⚠️ ปรับปรุงจาก DB dump ล่าสุด (ERP_V12, 2026-07-09) — เจอ table/behavior ที่ต่างจากเอกสารเดิมหลายจุด
> ดูหัวข้อ **"🔴 ความต่างจาก DB dump ล่าสุด"** ด้านล่างก่อนแก้โค้ด

## Project overview
Go Fiber v2 REST API สำหรับระบบ ERP ครอบคลุม PR / PO / RFQ / GRN / Stock (2 ระบบคู่ขนาน) / Borrow-Return / Memo / Approval Workflow / Menu-Permission (RBAC ละเอียดระดับ user+role+dept+menu)
เชื่อมต่อ PostgreSQL ผ่าน pgx/v5 pool, Auth ด้วย JWT + bcrypt, Docs ด้วย Swagger

---

## Tech stack
| Layer | Library | Version |
|---|---|---|
| Framework | github.com/gofiber/fiber/v2 | v2.52.5 |
| Database driver | github.com/jackc/pgx/v5 | v5.6.0 |
| Auth | github.com/golang-jwt/jwt/v5 | v5.2.1 |
| Password hash | golang.org/x/crypto (bcrypt) | v0.24.0 |
| Swagger gen | github.com/swaggo/swag | v1.16.3 |
| Swagger UI | github.com/gofiber/swagger | v1.1.0 |
| Env loader | github.com/joho/godotenv | v1.5.1 |
| UUID | github.com/google/uuid | v1.6.0 |

---

## 🔴 ความต่างจาก DB dump ล่าสุด (ERP_V12) — สำคัญมาก

1. **ไม่มี VIEW ใดๆ ใน dump เลย** — เอกสารเดิมอ้างถึง `v_material_full`, `v_inventory_full`,
   `v_pending_approvals`, `v_pr_full`, `v_po_full` แต่ dump ปัจจุบัน**ไม่มี CREATE VIEW แม้แต่ตัวเดียว**
   → ถ้า handler ไหน query จาก view เหล่านี้ จะ error แน่นอน ต้องเช็คว่ายัง apply migration ที่สร้าง view อยู่หรือเปล่า
   หรือเปลี่ยนไป join ตรงใน query แทน

2. **`purchase_request.status` ไม่มี PENDING_APPROVAL/APPROVED/REJECTED**
   ค่าจริงใน DB คือ: `DRAFT, COMPLETED, STOCK_CHECK, PARTIALLY_FILLED, FULFILLED, CANCELLED`
   → การอนุมัติ PR ไม่ได้เก็บอยู่ที่ field `status` ของ PR โดยตรง แต่ต้องดูผ่าน `approval_request` / `approval_log`
   (field `status` ของ PR สะท้อน "สถานะการเติมของ/สต๊อก" มากกว่า "สถานะอนุมัติ")
   PR ยังมี `priority` (LOW/NORMAL/HIGH/URGENT), `project_code`, `memo_id` ที่เอกสารเดิมไม่เคยพูดถึง

3. **`purchase_order.status`** มีเพิ่ม `PENDING_REAPPROVAL` นอกเหนือจาก flow เดิม:
   `DRAFT → PENDING_APPROVAL → APPROVED/REJECTED/PENDING_REAPPROVAL → SENT → PARTIALLY_RECEIVED/RECEIVED → CANCELLED`
   PO ยังมี field คำนวณเงินเพิ่มที่เอกสารเดิมไม่มี: `use_discount`, `discount_type(pct/amt)`, `discount_amount`,
   `use_vat`, `vat_amount`, `use_wht`, `wht_amount`, `net_amount`, `currency`

4. **RFQ มีตารางจริงแล้ว** (`rfq`, `rfq_line`) — ไม่ใช่ "ยังไม่มี handler" อย่างเดียวแล้ว table พร้อมใช้
   `rfq.status`: `SENT, RECEIVED, SELECTED, REJECTED`

5. **Borrow/Return มีตารางจริงแล้ว** (`borrow`, `borrow_line`, `borrow_status_log`)
   `borrow.status`: `DRAFT, PENDING_APPROVAL, APPROVED, REJECTED, BORROWED, RETURNED, PARTIALLY_RETURNED, CANCELLED`

6. **Stock Count มีตารางจริงแล้ว** (`stock_count`, `stock_count_line`)
   `stock_count.status`: `DRAFT, IN_PROGRESS, COMPLETED`

7. **มี 2 ระบบ stock คู่ขนานกัน** — ต้องระวังอย่าสับสน:
   - **Inventory module (เดิม)**: `inventory`, `inventory_transaction` — ใช้กับ GRN_IN, ISSUE, RETURN, TRANSFER, ADJUST, BORROW ตาม txn_type เดิมใน SKILL.md
   - **Stock module (ใหม่ ไม่เคยมีในเอกสารเดิม)**: `stock_item`, `stock_category`, `stock_inventory`, `stock_transaction`, `stock_reservation`
   → ยังไม่ชัดเจนว่า 2 ระบบนี้แยกโดเมนกันจริง (เช่น consumable vs asset) หรือเป็นของเก่า/ใหม่ซ้อนกัน — **ต้องถามทีมก่อนต่อ handler ใหม่ที่แตะ stock**

8. **Memo module ใหม่** (`memo`, `memo_line`, `memo_status_log`) — เชื่อมกับ PR ผ่าน `purchase_request.memo_id`
   `memo.status`: `DRAFT, PENDING_APPROVAL, APPROVED, REJECTED, CANCELLED`

9. **ระบบ Menu/Permission ละเอียดกว่าที่บอกไว้มาก** ไม่ใช่แค่ role-based ธรรมดา:
   - `modules`, `menus`, `permissions`
   - `role_menus`, `role_menu_permissions`, `role_permissions`
   - `user_menu_permissions`, `user_permissions`
   - `dept_menu_permissions`, `departments`
   - `permission_audit_logs`
   → มี trigger `enforce_read_before_write()`: ถ้า `can_read = false` จะ auto force `can_write/can_update/can_delete = false` ด้วย
   → RBAC จริงคือ **ผสมกันได้ 4 ชั้น**: role permission, user permission (override เฉพาะคน), dept permission, menu permission — ไม่ใช่แค่ `RequireRole()` middleware เดิม

10. **Master data ละเอียดขึ้น**: มี `mat_name`, `material_code`, `brand`, `spec_size`, `project`
    (เดิมเอกสารพูดถึงแค่ `mat_group`, `subgroup`, unit ระดับบนๆ)

11. **Audit/Log เพิ่มเติม**: `po_edit_log`, `pr_attachment`, `permission_audit_logs`

---

## Directory structure
```
erp-api/
├── cmd/server/main.go              # Entry point, Fiber app init, Swagger @annotations
├── internal/
│   ├── auth/service.go             # JWT generate/validate, bcrypt hash/check
│   ├── config/config.go            # Load .env → Config struct
│   ├── database/database.go        # pgxpool.New, ping, pool settings
│   ├── middleware/jwt.go           # JWTProtected(), RequireRole(), GetClaims(), ErrorHandler()
│   ├── models/models.go            # All domain structs, request/response types
│   ├── handlers/
│   │   ├── auth.go                 # Login, RefreshToken, Me, ChangePassword
│   │   ├── master.go               # Material, Location, Warehouse, Supplier, Brand, SpecSize, Project
│   │   ├── inventory.go            # Inventory balance, Transaction ledger (legacy inventory module)
│   │   ├── stock.go                # ⚠️ ถ้ายังไม่มี ต้องสร้าง — stock_item/stock_inventory/stock_transaction
│   │   ├── pr.go                   # Purchase Request CRUD + submit + approve + logs
│   │   ├── po.go                   # Purchase Order CRUD + approve + send + logs
│   │   ├── rfq.go                  # ⚠️ table พร้อมแล้ว handler ยังต้องสร้างตาม SKILL.md
│   │   ├── borrow.go               # ⚠️ table พร้อมแล้ว handler ยังต้องสร้าง
│   │   ├── stock_count.go          # ⚠️ table พร้อมแล้ว handler ยังต้องสร้าง
│   │   ├── memo.go                 # ⚠️ table พร้อมแล้ว handler ยังต้องสร้าง
│   │   ├── menu_permission.go      # ⚠️ ระบบ menu/permission ละเอียด — handler ยังต้องสร้าง
│   │   └── grn_approval.go         # GRN create/confirm + Approval/Audit log queries
│   └── routes/routes.go            # Register all routes + attach middleware
├── migrations/
│   └── 001_master_ddl.sql          # Full DDL — ⚠️ ล้าหลังกว่า ERP_V12 dump มาก ควร sync ใหม่
├── docs/                           # Generated by: swag init -g cmd/server/main.go -o docs
├── Dockerfile                      # Multi-stage build (golang:1.22-alpine → alpine:3.19)
├── docker-compose.yml              # Services: postgres + api + pgadmin (profile: tools)
├── .env.example                    # Template — copy เป็น .env แล้วแก้ค่า
└── go.mod
```

---

## Environment variables (.env)
```
PORT=8080
DATABASE_URL=postgres://postgres:PASSWORD@localhost:5432/postgres?sslmode=disable
JWT_SECRET=change-me-in-production-use-32chars!!
JWT_EXPIRY_HOURS=8
JWT_REFRESH_HOURS=168
CORS_ORIGINS=*
APP_ENV=development
BCRYPT_COST=12
```

---

## How to run (Windows, no Docker for API)
```powershell
go install github.com/swaggo/swag/cmd/swag@latest
$env:PATH += ";$env:GOPATH\bin"
swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal
go run ./cmd/server/main.go
```
- API: http://localhost:8080
- Swagger UI: http://localhost:8080/swagger/index.html

## How to run (Docker — Postgres only)
```powershell
docker compose up postgres -d
go run ./cmd/server/main.go
```

## How to run (Full Docker)
```powershell
docker compose up --build -d
# pgAdmin: docker compose --profile tools up -d → http://localhost:5050
```

---

## Database conventions
- **Schema**: `public`
- **Primary keys**: ⚠️ ผสมกัน — table เก่าใช้ `BIGSERIAL`, table ใหม่ใน dump (เช่น `approval_config`,
  `approval_request`, `menus` ฯลฯ) ใช้ `GENERATED ALWAYS AS IDENTITY` แทน — พฤติกรรมใช้งานเหมือนกัน
  แต่เวลาเขียน migration ใหม่ให้ตาม pattern `GENERATED ALWAYS AS IDENTITY` ของไฟล์ล่าสุด
- **Timestamps**: `created_at`, `updated_at` เป็น `TIMESTAMP NOT NULL DEFAULT NOW()`
- **Soft delete**: ใช้ `is_active BOOLEAN` ไม่มี hard delete
- **Document numbers**: generate ใน handler เช่น `PR-2026-000001`, `PO-2026-000001`
- **All queries**: ใช้ `pgxpool` โดยตรง ไม่มี ORM — raw SQL ทั้งหมด
- **Transactions**: ใช้ `db.Begin()` / `tx.Commit()` / `defer tx.Rollback()`
- **ไม่มี VIEW ใน DB จริง** (ดูข้อ 1 ด้านบน) — join ตรงใน query แทนถ้า view หาย

## Key views (PostgreSQL) — ⚠️ ตามเอกสารเดิม แต่ไม่พบใน dump ล่าสุด ต้องตรวจสอบก่อนใช้
| View | Description |
|---|---|
| `v_material_full` | Material พร้อม group/subgroup/spec/brand/unit |
| `v_inventory_full` | Inventory + material info + stock_status (OK/LOW/CRITICAL) |
| `v_pending_approvals` | Approval requests ที่ status = PENDING |
| `v_pr_full` | PR พร้อม requested_by name, location, warehouse |
| `v_po_full` | PO พร้อม supplier name, warehouse, PR reference |

---

## Auth flow
```
POST /api/v1/auth/login → { access_token, refresh_token, expires_at }
Header: Authorization: Bearer <access_token>
POST /api/v1/auth/refresh → new token pair
```

## JWT Claims structure
```go
type Claims struct {
    UserID   int64
    Username string
    FullName string
    Roles    []string  // role_code จาก roles table
    jwt.RegisteredClaims
}
```

## RBAC — 4 ชั้น (อัปเดตจาก dump จริง)
เดิมเอกสารบอกว่าใช้แค่ role ธรรมดา แต่ dump มีระบบ permission ละเอียดกว่านั้น:

| ชั้น | Table | ความหมาย |
|---|---|---|
| Role | `roles`, `role_permissions`, `role_menus`, `role_menu_permissions` | สิทธิ์ตาม role (เดิม) |
| User override | `user_permissions`, `user_menu_permissions` | override เฉพาะคน (ใหม่) |
| Department | `dept_menu_permissions`, `departments` | สิทธิ์ตามแผนก (ใหม่) |
| Menu/Module | `menus`, `modules` | โครงสร้างเมนู/สิทธิ์การเข้าถึงหน้า (ใหม่) |

Trigger `enforce_read_before_write()`: ถ้า `can_read=false` → auto set `can_write/can_update/can_delete=false`
ทุก permission table ที่มี 4 field นี้ต้องคำนึงถึง trigger นี้ตอน insert/update

| Role code | สิทธิ์ |
|---|---|
| `ADMIN` | Full access |
| `SENIOR_TEAM` | Approve PR step 1 |
| `MANAGER` | Approve PR step 2 + PO |
| `DIRECTOR` | Approve PO |
| `MD` | Approve PO |
| `PURCHASING` | Create PO, RFQ, GRN |
| `STOCK` | Inventory transactions |
| `ENGINEERING` | Create PR |

---

## PR → PO → GRN flow (แก้ตาม dump จริง)
```
PR (DRAFT)
  → approval ผ่าน approval_request/approval_log (ไม่ใช่ pr.status โดยตรง — ดูข้อ 2)
  → pr.status: DRAFT → STOCK_CHECK → PARTIALLY_FILLED/FULFILLED/COMPLETED (หรือ CANCELLED)
  → RFQ (SENT → RECEIVED → SELECTED/REJECTED) → select supplier
  → PO (DRAFT) → submit → PENDING_APPROVAL
    → MANAGER/DIRECTOR/MD approve → APPROVED (หรือ PENDING_REAPPROVAL ถ้าถูกตีกลับ) → SENT
    → GRN (DRAFT) → confirm → CONFIRMED → POSTED
      → auto inventory_transaction (GRN_IN)
      → auto update inventory.qty_on_hand
```

## Status transitions (ค่าจริงจาก DB CHECK constraint)
- **PR**: `DRAFT, COMPLETED, STOCK_CHECK, PARTIALLY_FILLED, FULFILLED, CANCELLED` (ไม่มี approval status ใน field นี้)
- **PO**: `DRAFT, PENDING_APPROVAL, APPROVED, REJECTED, PENDING_REAPPROVAL, SENT, PARTIALLY_RECEIVED, RECEIVED, CANCELLED`
- **GRN**: `DRAFT, CONFIRMED, POSTED` (+ `quality_status`: `PENDING, PASSED, FAILED, PARTIAL`)
- **RFQ**: `SENT, RECEIVED, SELECTED, REJECTED`
- **Borrow**: `DRAFT, PENDING_APPROVAL, APPROVED, REJECTED, BORROWED, RETURNED, PARTIALLY_RETURNED, CANCELLED`
- **Stock Count**: `DRAFT, IN_PROGRESS, COMPLETED`
- **Memo**: `DRAFT, PENDING_APPROVAL, APPROVED, REJECTED, CANCELLED`
- **Approval action** (`approval_log.action`): `SUBMIT, APPROVE, REJECT, RETURN, CANCEL, ESCALATE, COMMENT`
- **Approval request status** (`approval_request.status`): `PENDING, APPROVED, REJECTED, CANCELLED, ESCALATED`

---

## Audit & logging tables
| Table | Purpose |
|---|---|
| `approval_log` | Immutable log ทุก approve/reject action พร้อม ip_address |
| `erp_audit_log` | Generic audit ทุก table — old_data/new_data เป็น JSONB |
| `pr_status_log` | PR status history |
| `po_status_log` | PO status history |
| `po_edit_log` | **ใหม่** — log การแก้ไข PO (แยกจาก status_log) |
| `borrow_status_log` | **ใหม่** — Borrow status history |
| `memo_status_log` | **ใหม่** — Memo status history |
| `permission_audit_logs` | **ใหม่** — log การเปลี่ยนแปลง permission |
| `pr_attachment` | **ใหม่** — ไฟล์แนบของ PR |

---

## Common patterns

### Handler structure
```go
type XxxHandler struct { db *pgxpool.Pool }
func NewXxxHandler(db *pgxpool.Pool) *XxxHandler { return &XxxHandler{db: db} }
func (h *XxxHandler) MethodName(c *fiber.Ctx) error { ... }
```

### Get current user in handler
```go
claims := middleware.GetClaims(c)
// claims.UserID, claims.Username, claims.Roles
```

### Return paginated response
```go
return c.JSON(fiber.Map{
    "success": true,
    "data": models.PaginatedResponse{
        Data: items, Total: total, Page: page, PageSize: size, TotalPages: totalPages,
    },
})
```

### Error response
```go
return fiber.NewError(fiber.StatusBadRequest, "message")
// GlobalErrorHandler จะ format เป็น {"success": false, "error": "message"}
```

### Database transaction
```go
tx, err := h.db.Begin(context.Background())
if err != nil { return err }
defer tx.Rollback(context.Background())
// ... queries ...
return tx.Commit(context.Background())
```

---

## Swagger annotation pattern (ทุก handler ต้องมี)
```go
// MethodName godoc
// @Summary      Short description
// @Description  Longer description
// @Tags         TagName
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  int               true  "ID"
// @Param        body  body  models.XxxRequest true  "Request body"
// @Success      200   {object}  models.XxxModel
// @Failure      400   {object}  fiber.Map
// @Router       /path/{id} [method]
```

Re-generate หลังแก้ annotation:
```powershell
swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal
```

---

## 🧭 Session learnings (2026-07-22) — read before touching PR/PO/Memo approval or writing raw SQL

### 1. Document approval flow — who actually requires approval (easy to get wrong)
- **Memo**: REQUIRES approval. Role-based via `approval_config`, PLUS extra eligible approvers
  from `approval_delegation` (see #6). A specific approver is chosen from the eligible pool and
  stored as `memo.approver_id` — it is NOT purely role-based routing.
- **PR**: does **NOT** require approval at all. Flow is `DRAFT → COMPLETED` directly.
  There are **no PR approve/reject endpoints** — they were removed entirely. **Never re-add
  PR approval UI or routes** — this was a deliberate design decision, not an oversight.
- **PO**: REQUIRES approval, same "select a specific approver from the eligible pool" model as
  Memo (`purchase_order.approver_id`), NOT purely role-based. Flow:
  `DRAFT → PENDING_APPROVAL → APPROVED/REJECTED`. Approved POs can still be edited within 1 year
  via `PUT /po/:id/edit-approved` (reason is mandatory) — this transitions the PO to
  `PENDING_REAPPROVAL` and routes back to the **same** approver, not a fresh pool selection.

### 2. Schema-drift bug class — ~20 instances found this session, check proactively
- **Symptom**: Go SQL references a column name that doesn't match the real DB schema — e.g.
  using `po_id`/`pr_id`/`approval_id`/`grn_id`/`line_id`/`log_id` when the actual PK/FK column
  is almost always just `id`.
- **Root cause**: SQL copy-pasted between similar handlers without checking the live schema.
- **Rule**: before writing SQL for a table you haven't touched recently, verify real column
  names against `information_schema.columns` — never assume based on the table's "logical" name.
- **Silent field drop at the SQL layer**: a field being correctly present in the request struct
  AND correctly present in the handler's in-memory payload does **not** guarantee it's in the
  actual `INSERT`/`UPDATE` column list. This session found `approver_id`, `warehouse_code`,
  `ref`, and supplier contact fields all silently missing from the SQL despite being correct
  everywhere else in the call chain. When a field "isn't saving," check the SQL column list
  directly first — don't assume the struct/payload layer is at fault.

### 3. Nullable FK joins — use LEFT JOIN, never INNER JOIN
Any join on an optional/nullable foreign key must be `LEFT JOIN`, not `INNER JOIN`:
`requested_by → users`, `location_code → location`, `warehouse_code → warehouse`,
`project_code → project`, `supplier_code → supplier` (for contact fields). An `INNER JOIN`
silently excludes the entire row when the FK is `NULL` — this caused a confusing "PO not found"
bug for older records created before newer nullable columns existed.

### 4. Route registration order matters
Static routes (e.g. `/search`) MUST be registered **before** dynamic param routes (e.g. `/:id`)
in the same Fiber route group, or the router matches the dynamic route first and the static
route becomes unreachable — a request to `/po/search` gets captured as `/po/:id` with
`id="search"`. Always check registration order first when a static sub-route 404s unexpectedly
— but also verify it's actually a routing issue and not the handler's own query logic returning
a 404 (see below: an exact-match `WHERE po_no = $1` returning no rows looks identical to a
route being unreachable from the client's point of view).

### 5. Supplier contact data — live-joined, not snapshotted
`purchase_order` does **NOT** store its own copy of supplier contact fields (`office_phone`,
`fax`, `sales_person`, `contact_email`, `contact_phone`). These are always live-joined from the
`supplier` table via `supplier_code` at read time (`GET /po/:id` uses `LEFT JOIN supplier`).
Do not add these as `purchase_order` columns — this was explicitly decided against, reversing
an earlier snapshot-based design.

### 6. `approval_delegation` table
Columns: `id`, `doc_type` (nullable — `NULL` means it applies to **all** doc types), `user_id`,
`reason`, `created_at`, `created_by`. This is the "extra approver" mechanism, **additive** to
role-based `approval_config`, not a replacement for it. No from/to fields, no date range —
inserting a row makes it active immediately, deleting a row removes it immediately.

### 7. Debugging protocol for "field silently doesn't persist" reports
When a PR/PO/Memo form field looks correct in the frontend but doesn't save, the protocol that
actually worked this session:
1. Capture the real network payload via browser DevTools and confirm it's correct.
2. Confirm the field is in the handler's parsed request struct.
3. **Demand a pasted, actually-executed `SELECT` result after a real save** — never accept
   "should be fixed now" as a status. This session had multiple false "fixed" claims that
   turned out untested when actually queried. The bug is almost always in the SQL column list
   (see #2), not the struct or the frontend.

---

## Known issues / TODO (อัปเดตตาม dump จริง)
- [ ] **ตรวจสอบว่า view (`v_material_full` ฯลฯ) ยังจำเป็นหรือหายไปจริง** — ถ้าหายจริงต้อง refactor query ที่พึ่งพา view เหล่านี้
- [ ] **สอบถามทีมเรื่อง 2 ระบบ stock ซ้อนกัน** (`inventory` vs `stock_item/stock_inventory`) ก่อนพัฒนา handler ใหม่
- [ ] `go.sum` ต้อง generate ก่อนด้วย `go mod tidy` (ต้องมี internet)
- [ ] Swagger docs ต้อง `swag init` ก่อน run API ครั้งแรก
- [ ] `docs/docs.go` ปัจจุบันเป็น stub — จะถูก overwrite เมื่อ swag init
- [ ] ยังไม่มี unit tests — TODO เพิ่ม `_test.go` ในแต่ละ package
- [ ] **RFQ handler** — table พร้อมแล้ว (`rfq`, `rfq_line`) แต่ handler ยังต้องสร้างตาม pattern ใน SKILL.md
- [ ] **Borrow/Return handler** — table พร้อมแล้ว (`borrow`, `borrow_line`, `borrow_status_log`)
- [ ] **Stock Count handler** — table พร้อมแล้ว (`stock_count`, `stock_count_line`)
- [ ] **Memo handler** — table พร้อมแล้ว (`memo`, `memo_line`, `memo_status_log`)
- [ ] **Menu/Permission handler** — ระบบ 4 ชั้น (role/user/dept/menu) ยังไม่มี handler ใน CLAUDE.md เดิม
- [ ] `migrations/001_master_ddl.sql` ล้าหลังกว่า schema จริงมาก — ควร dump migration ใหม่จาก ERP_V12 แล้วแยกเป็นไฟล์ 00X ตาม module
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

3. **`purchase_order.status`** — 🔴 2026-07-27: แยกออกจาก receiving flow แล้ว ดูหัวข้อ
   "Session learnings (2026-07-27)" ท้ายไฟล์ ตอนนี้ `status` เก็บแค่ approval flow:
   `DRAFT → PENDING_APPROVAL → APPROVED/REJECTED/PENDING_REAPPROVAL → CANCELLED`
   ส่วนการรับของอยู่ที่ field ใหม่ `status_receive` (`NOT_SENT|SENT|PARTIALLY_RECEIVED|RECEIVED`)
   PO ยังมี field คำนวณเงินเพิ่มที่เอกสารเดิมไม่มี: `use_discount`, `discount_type(pct/amt)`, `discount_amount`,
   `use_vat`, `vat_amount`, `use_wht`, `wht_amount`, `net_amount`, `currency`

4. **RFQ มีตารางจริงแล้ว** (`rfq`, `rfq_line`) — ไม่ใช่ "ยังไม่มี handler" อย่างเดียวแล้ว table พร้อมใช้
   `rfq.status`: `SENT, RECEIVED, SELECTED, REJECTED`

5. **Borrow/Return มีตารางจริงแล้ว** (`borrow`, `borrow_line`, `borrow_status_log`)
   `borrow.status`: `DRAFT, PENDING_APPROVAL, APPROVED, REJECTED, BORROWED, RETURNED, PARTIALLY_RETURNED, CANCELLED`

6. **Stock Count มีตารางจริงแล้ว** (`stock_count`, `stock_count_line`)
   `stock_count.status`: `DRAFT, IN_PROGRESS, COMPLETED`

7. **มี 2 ระบบ stock คู่ขนานกัน — ตัดสินใจแล้ว (2026-07-27), ไม่ใช่ TODO อีกต่อไป**:
   - **`inventory` + `inventory_transaction`** (ผูกกับ `mat_code`, คู่กับ PR→PO→GRN, txn_type
     GRN_IN/ISSUE/RETURN/TRANSFER/ADJUST/BORROW ตามที่เคยเขียนไว้ใน SKILL.md เดิม)
     **❌ ไม่ได้ใช้งานจริง — ห้ามเพิ่ม logic update `inventory`/`inventory_transaction` ในทุก
     handler (GRN confirm, Borrow, Stock Count) จนกว่าจะมีคนสั่งเปลี่ยนนโยบายนี้อย่างชัดเจน**
   - **`stock_item` + `stock_category` + `stock_inventory` + `stock_transaction` + `stock_reservation`**
     (ผูกกับ `item_id` — ไม่มี FK กับ `mat_code` แม้ค่าจะซ้ำกันได้)
     **✅ ระบบนี้คือของจริงที่ใช้งาน** — คู่กับ Borrow/Return module, `stock_item.qty` ตัดตรงนี้
     ตอน borrow/requisition ได้รับอนุมัติ
     🔴 **2026-08-04 — ผูกเข้ากับ GRN receiving แล้ว** (ดูหัวข้อ "Session learnings (2026-08-04)"
     ท้ายไฟล์): `POST /grn/receive` ตอนนี้ upsert `stock_inventory.qty_on_hand` ต่อ
     `item_id + location_code` จริง แล้ว roll up `stock_item.qty` เป็นผลรวมของทุก location ของ
     item นั้น — ไม่ได้ update `stock_item.qty` ตรง ๆ อีกต่อไป (เดิมทำแบบนั้นชั่วคราวก่อนมี
     `stock_inventory` breakdown). `CreateMaterial` auto-create `stock_item` แถวเปล่า (`qty=0`)
     ให้ทุก material ใหม่ เพื่อให้ GRN receive หา stock_item เจอเสมอ
   - **กฎ**: เจอคำว่า "stock"/"inventory" ในงานใหม่ ต้องเช็คก่อนเสมอว่ากำลังหมายถึงระบบไหน อย่าเดา
     เพราะชื่อคล้ายกันมากและเป็นต้นเหตุ schema-drift ที่เจอไปแล้วรอบนึง (ดู #2 ด้านล่าง)

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
- **Requisition** (ใบเบิกของ, `internal/handlers/requisition.go`): `DRAFT, SUBMITTED, ISSUED, CANCELLED`
- **Stock Transfer**: `DRAFT / CONFIRMED / CANCELLED`
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

## 🧭 Session learnings (2026-07-27) — GRN receiving / "รับเข้า" logic

### 1. PO ↔ GRN receiving relationship — สอง level ต้องคู่กันเสมอ
- `purchase_order.status` = ภาพรวมทั้งใบ ใช้ตอน filter list/badge หน้า list ทั่วไป
- `purchase_order_line.status` (`OPEN|PARTIAL|RECEIVED|CANCELLED`) + `qty_received` vs
  `qty_ordered` = ความจริงระดับบรรทัด ใช้คำนวณว่า "เหลืออะไรให้รับอีก"
- หัว PO เป็นแค่ **ผลสรุป (derived)** จากทุกบรรทัด ไม่ใช่ source of truth เดี่ยว ๆ — ห้าม stamp
  แค่ `purchase_order.status` อย่างเดียวโดยไม่อัปเดต `purchase_order_line` ด้วย เพราะ 1 PO รับของ
  เป็นงวด ๆ ได้ (หลาย GRN ต่อ PO ใบเดียว, แต่ละบรรทัดรับไม่พร้อมกัน)
- `purchase_order.status` เป็น `NOT NULL DEFAULT 'DRAFT'` ระดับ DB — สร้าง PO ใหม่จะไม่มีทาง
  เป็น null

### 2. Endpoint search PO สำหรับหน้า "รับเข้า" (GRN) — query ที่ควรใช้
> 🔴 อัปเดตหลังแยก `status`/`status_receive` — ดู #4 ด้านล่าง
```sql
SELECT po.id, po.po_no, po.po_date, po.supplier_code, po.status, po.status_receive
FROM purchase_order po
WHERE po.status = 'APPROVED'
  AND po.status_receive IN ('NOT_SENT', 'SENT', 'PARTIALLY_RECEIVED')
  AND EXISTS (
    SELECT 1 FROM purchase_order_line pol
    WHERE pol.po_id = po.id AND pol.status IN ('OPEN', 'PARTIAL')
  )
```
โหลด PO detail สำหรับกรอกฟอร์ม GRN ก็ select เฉพาะ line ที่ `status IN ('OPEN','PARTIAL')`
พร้อมส่ง `qty_ordered - qty_received` (qty คงเหลือที่รับได้) ไปด้วย กัน over-receive ฝั่ง UI

### 3. GRN confirm flow (`POST /grn/:id/confirm`) — 5 ขั้นตอนใน transaction เดียว, ไม่มี inventory
1. `SELECT ... FOR UPDATE` ล็อก `grn` แถวเดียวกัน + update `grn.status → CONFIRMED` (กัน confirm ซ้ำ)
2. insert `grn_line` (`qty_accepted` ต่อบรรทัด)
3. update `purchase_order_line.qty_received += qty_accepted` แล้ว set `status`:
   `qty_received >= qty_ordered` → `RECEIVED`, ไม่งั้น → `PARTIAL`
4. recompute `purchase_order.status_receive` (🔴 ไม่ใช่ `status` แล้ว — `status` คือ approval
   flow ห้ามแตะตอน confirm GRN) จากทุกบรรทัดที่ไม่ใช่ `CANCELLED`:
   ทุกบรรทัด `RECEIVED` ครบ → `status_receive = RECEIVED`, ไม่ครบ → `PARTIALLY_RECEIVED`
5. insert `po_status_log` (from/to ของ `status_receive` — พิจารณาเพิ่มคอลัมน์ระบุว่า log นี้เป็น
   approval change หรือ receive change ถ้า `po_status_log` ยังไม่แยกประเภทไว้)
6. commit

**ห้ามเพิ่ม step insert `inventory_transaction` / upsert `inventory` กลับเข้ามาในนี้** — ตัดสินใจ
แล้วว่าไม่ใช้ระบบ `inventory` (ดู #7 ในหัวข้อ "ความต่างจาก DB dump ล่าสุด" ด้านบน) ถ้าจะเชื่อม stock
จริง ๆ ต้องเป็น `stock_item`/`stock_inventory` และต้องเป็น task แยกที่คุยกันก่อน ไม่ใช่ผูกอัตโนมัติกับ
GRN confirm

### 4. `purchase_order.status` แยกออกจาก `status_receive` แล้ว (migration `003_po_split_receive_status.sql`)
เหตุผล: field เดียวเดิมรวม approval flow + receiving flow ปนกัน ทำให้ `PENDING_REAPPROVAL`
ทับ `PARTIALLY_RECEIVED` ไม่ได้ (เก็บพร้อมกันไม่ได้ในค่าเดียว) ตอนนี้:
- `status` = approval เท่านั้น: `DRAFT|PENDING_APPROVAL|APPROVED|REJECTED|PENDING_REAPPROVAL|CANCELLED`
- `status_receive` = receiving เท่านั้น: `NOT_SENT|SENT|PARTIALLY_RECEIVED|RECEIVED`
- สอง field เป็นอิสระต่อกัน — PO ที่ `PENDING_REAPPROVAL` แต่รับของไปแล้วบางส่วนจะเป็น
  `status='PENDING_REAPPROVAL', status_receive='PARTIALLY_RECEIVED'` พร้อมกันได้ปกติ
- **จุดที่ต้องแก้ตาม**: endpoint `POST /po/:id/send` ต้อง update `status_receive='SENT'` แทนที่
  จะ update `status`; `PUT /po/:id/edit-approved` set `status=PENDING_REAPPROVAL` โดยไม่ต้องแตะ
  `status_receive` เลย; ทุก handler เก่าที่เคย query `status IN ('SENT','PARTIALLY_RECEIVED',...)`
  ต้องแก้เป็น query `status_receive` แทน
- migration รันแล้วตอนที่ระบบมีข้อมูลจริงแค่ 1 แถว (`status='APPROVED'`) จึงไม่ต้อง backfill

---

## 🧭 Session learnings (2026-08-04) — GRN receiving ↔ stock_item/stock_inventory wiring

### 1. Material create auto-creates a matching `stock_item`
`MasterHandler.CreateMaterial` (`internal/handlers/master.go`) now inserts a zero-qty
`stock_item` row (`mat_code`, `item_name`, `unit`, `qty=0`) for every new material inside the
same transaction, if one doesn't already exist for that `mat_code`. Reason: `POST /grn/receive`
(below) requires a `stock_item` row to exist for every `mat_code` it receives — without this,
materials created before a GRN would have no stock target and receiving would fail.

### 2. `POST /grn/receive` (`GoodsReceiptHandler.Receive`, `internal/handlers/goods_receipt.go`)
now writes real per-location stock, not just a flat `stock_item.qty` bump
- **`stock_inventory`** has a unique constraint on `(item_id, location_code)` only —
  `warehouse_code` is **not** part of the uniqueness key, it's just descriptive. Both
  `location_code` and `warehouse_code` are FKs (`location.location_code`, `warehouse.warehouse_code`)
  — inserting a `location_code` that isn't a real row in `location` will fail the FK, so never
  invent one.
- Per line: upsert `stock_inventory` via `ON CONFLICT (item_id, location_code) DO UPDATE SET
  qty_on_hand = stock_inventory.qty_on_hand + EXCLUDED.qty_on_hand` (additive, not overwrite),
  then roll up `stock_item.qty = SUM(qty_on_hand) FROM stock_inventory WHERE item_id = ...` —
  `stock_item.qty` is now a **derived total**, never written to directly outside this rollup.
- **`location_code` resolution order**: `purchase_order.location_code` (nullable — currently
  unused by any existing PO, so expect it to usually be null) wins if set, otherwise fall back
  to the receiving `stock_item`'s own `location_code` (defaults to `'SAL'` per
  `CreateMaterial`/DB default). Don't hardcode `'SAL'` — always read it from `stock_item` so a
  future default change doesn't silently break receiving.
- `stock_transaction.qty_before`/`qty_after` record the **item-level total** (sum across all
  locations), matching what `stock_item.qty` represents — not the single location's before/after.
  Mixing the two scopes was an early mistake this session; keep them at the same (item) level.
- This is a **different handler** from the legacy `GRNHandler.Confirm` (`POST /grn/:id/confirm`,
  see "Session learnings (2026-07-27)" #3 above) — that legacy flow explicitly does **not** touch
  stock. The prohibition there is unchanged; it does not apply to `GoodsReceiptHandler`, which is
  the intended, decided-on path for stock to move on receipt.

---

## 🧭 Session learnings (2026-08-16) — Stock Requisition + Stock Transfer

### 1. Two separate systems, not one — despite similar names
`internal/handlers/requisition.go` (ใบเบิกของ, คลัง→โครงการ) and
`internal/handlers/stock_transfer.go` (ย้ายคลัง, WH↔WH/project) are **deliberately separate**,
each with its own DB tables and status flow:
- `requisition` / `requisition_line` / `requisition_status_log` — pre-existing tables (already in
  the DB before this session), status `DRAFT → SUBMITTED → ISSUED / CANCELLED`.
- `stock_transfer` / `stock_transfer_line` / `stock_transfer_status_log` — status
  `DRAFT → CONFIRMED / CANCELLED`, `transfer_type` = `WH_TO_WH | WH_TO_PROJECT | PROJECT_TO_WH`.
They only share `stock_transaction` for movement history (`ref_doc_type = 'REQUISITION'` vs
`'STOCK_TRANSFER'`) via `GET /stock-transfer/history`, which reads both. Do **not** try to merge
these into one table later without a deliberate decision — they were kept apart because
`requisition` already existed with a different shape and re-doing it wasn't worth the churn.

### 2. `stock_item.mat_code` uniqueness was changed — check before assuming single-warehouse
Before this session, `stock_item` had `UNIQUE(mat_code)` — **one row per mat_code system-wide**,
which made cross-warehouse transfer impossible (you can't have the same mat_code in two
warehouses). Changed to `UNIQUE(mat_code, warehouse_code)`. Any old code doing
`SELECT ... FROM stock_item WHERE mat_code=$1` without also filtering `warehouse_code` will
silently break (or return the wrong row / error on multiple rows) once a second warehouse exists
with real data — currently only `WH01` is in use, so this hasn't surfaced yet, but it will the
moment `WH02`+ gets stock. Audit any handler that queries `stock_item` by `mat_code` alone before
relying on it for a multi-warehouse scenario.

### 3. Movement math works on `stock_item.qty` directly, not `stock_inventory`
Both Requisition Issue and Stock Transfer Confirm decrement/increment `stock_item.qty` directly
(`SELECT ... FOR UPDATE` + `UPDATE stock_item SET qty=...`) — they do **not** touch
`stock_inventory.qty_on_hand`. This matches the schema actually given for these two features
(`stock_transfer_line`/`requisition_line` have no `location_code` column, so there's no
sub-location breakdown to update) but is **inconsistent** with `stock_borrow.go` and
`stock_inventory.go`'s `Transfer`, which write `stock_inventory.qty_on_hand` per
`(item_id, location_code)` instead and never roll that up into `stock_item.qty`. This drift
between `stock_item.qty` and `stock_inventory.qty_on_hand` already existed before this session
(no trigger keeps them in sync) — it is not something this session introduced or fixed, just
another instance of it. Don't assume either column is authoritative without checking which flow
last touched the item.

### 4. `stock_transaction.txn_type` CHECK constraint is stricter than the constants suggest
The live CHECK constraint only allows `IN, OUT, TRANSFER, ADJUST_PLUS, ADJUST_MINUS, BORROW_OUT,
BORROW_RETURN` — **not** `ISSUE`/`RECEIVE`/`RETURN`, even though `stock_constants.go` defines
`TxnTypeIssue = "ISSUE"` etc. and `pr.go` inserts using `TxnTypeIssue`. This looks like existing
drift between the constants file and the DB constraint (not something fixed in this session,
flagging for whoever investigates why PR-driven stock issue inserts might be failing). Requisition
Issue and Stock Transfer Confirm use the constraint-legal literal values directly
(`'OUT'`, `'IN'`, `'TRANSFER'`) rather than the `TxnType*` constants — do the same for any new
insert into `stock_transaction` until the constants/constraint mismatch is resolved.

### 5. No granular per-warehouse permission check exists yet
The task asked for "caller must have rights on `to_warehouse_code`" style checks using the 4-layer
RBAC (`role_menu_permissions`/`user_menu_permissions`/`dept_menu_permissions`). No middleware or
handler in this codebase enforces that anywhere yet (confirmed — `internal/middleware` only
exports `RequireRole`; the permission tables have no handler built on them, matching the
"Menu/Permission handler ยังไม่มี" TODO below). `Requisition.Issue` and `StockTransfer.Confirm`
use `middleware.RequireRole("STOCK", "ADMIN")` instead, matching how `stock_borrow.go`'s `Approve`
gates its confirm-style action. Building real per-warehouse permission checks is a separate,
larger task — don't assume it's covered.

### 6. Menu rows already existed
`MENU_STOCK_REQUISITION` (id 46), `MENU_STOCK_TRANSFER` (id 47), `MENU_STOCK_HISTORY` (id 48)
already existed under the `MENU_STOCK` (id 13, "คลังสินค้า") parent menu before this session — no
SQL needed to create them. However `role_menus` has **zero** rows for these 3 menu IDs, meaning no
role currently has menu-level access to them via the role-based path; someone needs to wire that
up (outside this session's scope — it's a data/admin-UI task, not a code change).

---

## Known issues / TODO (อัปเดตตาม dump จริง)
- [ ] **ตรวจสอบว่า view (`v_material_full` ฯลฯ) ยังจำเป็นหรือหายไปจริง** — ถ้าหายจริงต้อง refactor query ที่พึ่งพา view เหล่านี้
- [x] ~~สอบถามทีมเรื่อง 2 ระบบ stock ซ้อนกัน~~ — ตัดสินใจแล้ว 2026-07-27: `inventory` ไม่ใช้,
  `stock_item`/`stock_inventory` ใช้จริงคู่กับ Borrow/Return (ดูหัวข้อ #7 ด้านบน)
- [ ] **สร้าง endpoint `GET /grn/po-search` (หรือเทียบเท่า)** สำหรับหน้ารับเข้า — ยังไม่มี handler
  ที่ filter PO ตาม logic ใน session note ด้านบน (query PO ที่ status ไม่ครบรับ + มี line เหลือ)
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
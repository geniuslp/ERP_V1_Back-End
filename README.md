# ERP API — Golang Fiber v2

REST API สำหรับระบบ ERP ครอบคลุม PR / PO / Store Management / Approval Workflow พร้อม Swagger UI

## Stack

| Layer | Technology |
|---|---|
| Framework | Go Fiber v2 |
| Database | PostgreSQL 16 |
| Auth | JWT (golang-jwt/jwt v5) + bcrypt |
| Docs | Swagger (swaggo/swag + gofiber/swagger) |
| Container | Docker + Docker Compose |

---

## Quick start

### 1. Prerequisites
- Go 1.22+
- Docker + Docker Compose
- `swag` CLI: `make install-swag`

### 2. Clone & setup
```bash
cp .env.example .env
# Edit .env if needed
```

### 3. Generate Swagger docs
```bash
make swagger
```

### 4. Run with Docker (recommended)
```bash
make docker-up
```

- API: http://localhost:8080
- Swagger UI: http://localhost:8080/swagger/index.html
- pgAdmin (optional): `make docker-tools` → http://localhost:5050

### 5. Run locally
```bash
# Start postgres separately, then:
make run
```

---

## Authentication

All protected endpoints require:
```
Authorization: Bearer <access_token>
```

### Login
```
POST /api/v1/auth/login
{"username": "admin", "password": "Admin@1234"}
```

Returns `access_token` and `refresh_token`.

### Refresh
```
POST /api/v1/auth/refresh
{"refresh_token": "..."}
```

---

## API Endpoints

### Auth
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/auth/login` | Login, get token pair |
| POST | `/api/v1/auth/refresh` | Refresh access token |
| GET | `/api/v1/auth/me` | Current user profile |
| POST | `/api/v1/auth/change-password` | Change password |

### Master Data
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/master/groups` | Material groups |
| GET | `/api/v1/master/units` | Units of measure |
| GET | `/api/v1/master/materials` | Materials (paginated, searchable) |
| GET | `/api/v1/master/materials/:code` | Material detail |
| GET/POST | `/api/v1/master/locations` | Locations (dept/project/site) |
| GET/POST | `/api/v1/master/warehouses` | Warehouses |
| GET | `/api/v1/master/warehouses/:code/zones` | Storage zones |
| GET/POST | `/api/v1/master/suppliers` | Suppliers |

### Inventory
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/inventory` | Stock balances (with stock_status) |
| GET/POST | `/api/v1/inventory/transactions` | Inventory movements ledger |

### Purchase Request (PR)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/pr` | List PRs (paginated) |
| POST | `/api/v1/pr` | Create PR (DRAFT) |
| GET | `/api/v1/pr/:id` | PR detail with lines |
| POST | `/api/v1/pr/:id/submit` | Submit for approval |
| POST | `/api/v1/pr/:id/approve` | Approve/Reject (role: SENIOR_TEAM+) |
| GET | `/api/v1/pr/:id/logs` | Status history |

### Purchase Order (PO)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/po` | List POs (paginated) |
| POST | `/api/v1/po` | Create PO (auto VAT 7%) |
| GET | `/api/v1/po/:id` | PO detail with lines |
| POST | `/api/v1/po/:id/approve` | Approve/Reject (role: MANAGER+) |
| POST | `/api/v1/po/:id/send` | Send to supplier |
| GET | `/api/v1/po/:id/logs` | Status history |

### GRN
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/grn` | List GRNs |
| POST | `/api/v1/grn` | Create GRN |
| POST | `/api/v1/grn/:id/confirm` | Confirm + auto inventory update |

### Approvals & Audit
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/approvals/pending` | My pending approvals |
| GET | `/api/v1/approvals/logs` | Approval history for a doc |
| GET | `/api/v1/approvals/audit` | ERP audit log |

---

## Role-based access

| Role code | Access |
|---|---|
| `ADMIN` | Full access |
| `SENIOR_TEAM` | PR approval step 1 |
| `MANAGER` | PR approval step 2 + PO approval |
| `DIRECTOR` | PO approval |
| `MD` | PO approval |
| `PURCHASING` | Create/manage PO, RFQ, GRN |
| `STOCK` | Inventory transactions |
| `ENGINEERING` | Create PR |

---

## PR → PO Flow

```
[Engineering]              [Senior Team]         [Stock]         [Purchasing]         [Manager/MD]
Create PR (DRAFT)
  → Submit
             → Approve PR
                           → Check stock
                             Reserve or create Indent
                                              → RFQ → Select Supplier
                                              → Create PO (DRAFT)
                                                                    → Approve PO
                                              → Send PO to Supplier
                                              → Track delivery
                                              → Create GRN
                           → Confirm GRN
                             (inventory auto-updated)
```

---

## Project structure

```
erp-api/
├── cmd/server/main.go          # Entry point + Swagger annotations
├── internal/
│   ├── auth/service.go         # JWT + bcrypt
│   ├── config/config.go        # Env config loader
│   ├── database/database.go    # pgxpool connection
│   ├── middleware/jwt.go       # JWT + RBAC middleware
│   ├── models/models.go        # All domain structs + request/response types
│   ├── handlers/
│   │   ├── auth.go             # Login, refresh, me, change-password
│   │   ├── master.go           # Material, location, warehouse, supplier
│   │   ├── inventory.go        # Inventory balances + transactions
│   │   ├── pr.go               # Purchase Request CRUD + approval
│   │   ├── po.go               # Purchase Order CRUD + approval
│   │   └── grn_approval.go     # GRN + approval/audit log queries
│   └── routes/routes.go        # Route registration
├── migrations/
│   └── 001_master_ddl.sql      # Full DDL (run after Erp_v1)
├── docs/                       # Generated by swag init
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── .env.example
```

# Database Reference — ERP V10

> Generated from DB dump. ใช้เป็น reference สำหรับเขียน prompt ให้ AI — ไม่ต้องแนบไฟล์ dump อีก
> อัปเดตไฟล์นี้ทุกครั้งที่มี migration หรือ ALTER TABLE

---

## Table Index

| Table | กลุ่ม | หมายเหตุ |
|-------|--------|----------|
| [approval_config](#approval_config) | Approval | config ขั้นตอนอนุมัติต่อ doc_type |
| [approval_delegation](#approval_delegation) | Approval | มอบหมายสิทธิ์อนุมัติ |
| [approval_doc_types](#approval_doc_types) | Approval | ทะเบียน doc_type ที่ใช้ approval engine |
| [approval_log](#approval_log) | Approval | log ทุก action ของ approval |
| [approval_request](#approval_request) | Approval | คำขออนุมัติ pending |
| [borrow](#borrow) | Stock | ใบขอยืม/เบิกวัสดุ |
| [borrow_line](#borrow_line) | Stock | รายการใน borrow |
| [borrow_status_log](#borrow_status_log) | Stock | log สถานะ borrow |
| [brand](#brand) | Master | แบรนด์สินค้า |
| [cost_group](#cost_group) | Cost | กลุ่มต้นทุน |
| [cost_job](#cost_job) | Cost | งานต้นทุน |
| [cost_subgroup](#cost_subgroup) | Cost | กลุ่มย่อยต้นทุน |
| [cost_subject](#cost_subject) | Cost | หัวข้อต้นทุน |
| [departments](#departments) | Org | แผนก |
| [dept_menu_permissions](#dept_menu_permissions) | Permission | สิทธิ์เมนูตามแผนก |
| [erp_audit_log](#erp_audit_log) | Audit | audit log ทั้งระบบ |
| [grn](#grn) | GRN | ใบรับเข้าสินค้า |
| [grn_line](#grn_line) | GRN | รายการใน GRN |
| [inventory](#inventory) | Stock | stock PR/PO (mat_code based) |
| [inventory_transaction](#inventory_transaction) | Stock | transaction inventory |
| [location](#location) | Master | สถานที่/ที่ตั้ง |
| [mat_group](#mat_group) | Material | กลุ่มวัสดุ |
| [mat_name](#mat_name) | Material | ชื่อวัสดุ |
| [material_code](#material_code) | Material | รหัสวัสดุ master |
| [memo](#memo) | Memo | บันทึกข้อความ |
| [memo_attachment](#memo_attachment) | Memo | ไฟล์แนบ memo |
| [memo_line](#memo_line) | Memo | รายการใน memo |
| [memo_status_log](#memo_status_log) | Memo | log สถานะ memo |
| [menus](#menus) | System | เมนูระบบ |
| [modules](#modules) | System | โมดูลระบบ |
| [permission_audit_logs](#permission_audit_logs) | Permission | audit log สิทธิ์ |
| [permissions](#permissions) | Permission | permission key |
| [po_attachment](#po_attachment) | PO | ไฟล์แนบ PO |
| [po_edit_log](#po_edit_log) | PO | log การแก้ไข PO |
| [po_status_log](#po_status_log) | PO | log สถานะ PO |
| [pr_attachment](#pr_attachment) | PR | ไฟล์แนบ PR |
| [pr_status_log](#pr_status_log) | PR | log สถานะ PR |
| [project](#project) | Master | โครงการ |
| [project_stock](#project_stock) | Stock | ยอดวัสดุคงเหลือที่โครงการ (จาก requisition/stock_transfer) |
| [purchase_order](#purchase_order) | PO | ใบสั่งซื้อ |
| [purchase_order_line](#purchase_order_line) | PO | รายการใน PO |
| [purchase_request](#purchase_request) | PR | ใบขอซื้อ |
| [purchase_request_line](#purchase_request_line) | PR | รายการใน PR |
| [rfq](#rfq) | RFQ | ใบขอเสนอราคา |
| [rfq_line](#rfq_line) | RFQ | รายการใน RFQ |
| [role_menu_permissions](#role_menu_permissions) | Permission | สิทธิ์เมนูตาม role |
| [role_menus](#role_menus) | Permission | เมนูที่ role เข้าถึงได้ |
| [role_permissions](#role_permissions) | Permission | permission ของ role |
| [roles](#roles) | Org | บทบาทผู้ใช้ |
| [spec_size](#spec_size) | Material | spec/ขนาดวัสดุ |
| [stock_category](#stock_category) | Stock | หมวดหมู่ stock |
| [stock_count](#stock_count) | Stock | ใบนับสต็อก |
| [stock_count_line](#stock_count_line) | Stock | รายการนับสต็อก |
| [stock_inventory](#stock_inventory) | Stock | inventory ต่อ location (stock_item based) |
| [stock_item](#stock_item) | Stock | รายการวัสดุในคลัง (qty จริง) |
| [stock_reservation](#stock_reservation) | Stock | การจองวัสดุ |
| [stock_transaction](#stock_transaction) | Stock | transaction stock_item |
| [stock_transfer](#stock_transfer) | Stock | ใบย้ายคลัง (WH_TO_WH / WH_TO_PROJECT / PROJECT_TO_WH) |
| [stock_transfer_line](#stock_transfer_line) | Stock | รายการใน stock_transfer |
| [stock_transfer_status_log](#stock_transfer_status_log) | Stock | log สถานะ stock_transfer |
| [requisition](#requisition) | Stock | ใบเบิกของ (คลัง → โครงการ) |
| [requisition_line](#requisition_line) | Stock | รายการใน requisition |
| [requisition_status_log](#requisition_status_log) | Stock | log สถานะ requisition |
| [storage_zone](#storage_zone) | Master | โซนคลัง |
| [subgroup](#subgroup) | Material | กลุ่มย่อยวัสดุ |
| [supplier](#supplier) | Master | ซัพพลายเออร์ |
| [unit](#unit) | Master | หน่วยนับ |
| [user_menu_permissions](#user_menu_permissions) | Permission | สิทธิ์เมนูรายบุคคล |
| [user_permissions](#user_permissions) | Permission | permission รายบุคคล |
| [user_roles](#user_roles) | Org | role ของแต่ละ user |
| [users](#users) | Org | ผู้ใช้งาน |
| [warehouse](#warehouse) | Master | คลังสินค้า |
| [work_order](#work_order) | WO | หนังสือสั่งจ้าง (subcontractor hiring) |
| [work_order_line](#work_order_line) | WO | รายการต่อบรรทัด (cost_code แทน item) |
| [work_order_status_log](#work_order_status_log) | WO | log สถานะ work_order |
| [work_order_attachment](#work_order_attachment) | WO | ไฟล์แนบ work_order |

---

## ⚠️ Important Notes

- `stock_item.qty` — qty จริงของวัสดุในคลัง ใช้ตัดเมื่อ borrow/requisition อนุมัติ
- `inventory.qty_on_hand` — ใช้กับ PR/PO flow (mat_code based) คนละตัวกับ stock_item
- `material_code.mat_code` ≠ `stock_item.mat_code` — ใช้ค่าเดียวกันแต่คนละตาราง ไม่มี FK ข้าม
- 🔴 **`stock_item.mat_code` ไม่ใช่ unique เดี่ยวๆ อีกต่อไป** (2026-08-16) — เปลี่ยนจาก
  `UNIQUE(mat_code)` เป็น `UNIQUE(mat_code, warehouse_code)` เพื่อให้ mat_code เดียวกันมีแถวคนละ
  warehouse ได้ (จำเป็นสำหรับ `stock_transfer` แบบ WH_TO_WH) — โค้ดที่เคย `SELECT ... WHERE
  mat_code=$1` แบบไม่ระบุ warehouse_code ต้องเช็คว่าอาจได้มากกว่า 1 แถวถ้ามีหลาย warehouse ในอนาคต
  (ปัจจุบันมี warehouse เดียว `WH01` ในข้อมูลจริง จึงยังไม่เจอปัญหานี้)
- `borrow_type` — `'BORROW'` = ขอยืม/เบิก, `'RETURN'` = คืน
- `borrow_line.mat_type` — `'RETURNABLE'` = ต้องคืน, `'CONSUMABLE'` = ไม่ต้องคืน
- `grn` มี `quality_status`, `confirmed_by`, `delivery_note` — ใช้ schema นี้ ไม่ใช่ inventory table
- `purchase_request.order_type` — `'stock'` = ซื้อเข้าคลัง (warehouse), `'cost'` = ซื้อเข้าโครงการ (project cost) — คนละความหมายกับ `pr_type`
- `work_order` (WO) เป็นเอกสารแยกจาก `purchase_order` (PO) — ใช้จ้างผู้รับเหมาช่วง
  ไม่มี FK เชื่อมกับ `purchase_request`/`purchase_order`
- 🔴 work_order line items mirror โครงสร้างของ `purchase_order_line` เกือบทั้งหมด (ยกเว้น
  item reference → เปลี่ยนเป็น `cost_code`) — ดู field list เต็มที่ [work_order_line](#work_order_line)
- 🔴 `discount_type`/`disc_type` เป็น varchar + CHECK ('PERCENT'|'AMOUNT') ไม่ใช่ Postgres enum —
  ยืนยันค่าจริงกับทีมก่อนใช้งาน เพราะไม่มีต้นแบบตรงจาก PO (PO เก็บ discount คนละแบบ)
- 🔴 `work_order_line.vat_rate` เป็นของใหม่ที่ WO เพิ่มเอง — PO เก็บ VAT ต่อบรรทัดแบบ hardcode
  7% ในโค้ด ไม่มีคอลัมน์ rate ดังนั้น WO ไม่ได้ mirror ตรงๆ จาก PO ในจุดนี้
- `work_order_line.wht_rate` มี CHECK (1,3,5) mirror มาจาก PO ตรงๆ
- `work_order.employer_name` **ไม่ใช่ free text อิสระอีกต่อไป** — ผูกกับ dropdown
  "สำนักงาน/สาขา" (`employer_branch`, frontend-only field ไม่มีคอลัมน์ DB) ที่ compose ข้อความ
  อัตโนมัติ เช่น เลือก HO → "บริษัท จีเนียส เอนจิเนียริง (สำนักงานใหญ่) จำกัด" — ตัวเลือกปัจจุบัน
  คือ HO/FAC-S/FAC-P/BO อาจมีเพิ่มทีหลัง
- `work_order.contract_description` (ลักษณะของสัญญา) เปลี่ยนจาก free text เป็น dropdown
  ตัวเลือกคงที่ (RENOVATION/NEW_CONSTRUCTION/REPAIR/MAINTENANCE/INSTALLATION/DEMOLITION) —
  ยังไม่ confirm ว่าครบกับธุรกิจจริงหรือไม่
- สิทธิ์เห็นเมนู/สร้าง/แก้ไข/อนุมัติ WO **ไม่ hardcode role ใดๆ ในโค้ดหรือ SQL เด็ดขาด** —
  ตั้งค่าทั้งหมดผ่านหน้า Permission Matrix / Approval Matrix ที่มีอยู่แล้วในระบบ
- ⚠️ `work_order_cost_code` (multi-select แบบแรก ก่อนเปลี่ยนเป็น line items) — **deprecated**
  ไม่ใช้แล้ว เก็บไว้เฉยๆ อย่าเพิ่ม routing ใหม่ในนี้

---

## Approval Engine

```
approval_doc_types  ← ลงทะเบียน doc_type (BORROW, PO, PR, ฯลฯ)
approval_config     ← กำหนด step + approver_role_id ต่อ doc_type
approval_request    ← คำขอ pending รอ action
approval_log        ← log ทุก action (SUBMIT/APPROVE/REJECT/CANCEL)
```

**Status flow (ทั่วไป):** `DRAFT → PENDING_APPROVAL → APPROVED / REJECTED`
- REJECTED → สามารถแก้ไขแล้ว re-submit ได้
- APPROVED → lock ห้ามแก้ไข

---

## Schema Detail

---

### approval_config
```
id               bigint        NOT NULL  PK
doc_type         varchar(30)   NOT NULL  — 'PO','PR','BORROW', etc.
step_no          integer       NOT NULL
step_name        varchar(200)  NOT NULL
approver_role_id bigint        nullable  — FK → roles.id
min_amount       numeric(18,4) nullable
max_amount       numeric(18,4) nullable
is_active        boolean       NOT NULL  DEFAULT true
created_at       timestamp     NOT NULL  DEFAULT now()
updated_at       timestamp     NOT NULL  DEFAULT now()
created_by       bigint        nullable
updated_by       bigint        nullable
```

---

### approval_delegation
```
id         bigint      NOT NULL  PK
doc_type   varchar(30) nullable
user_id    bigint      NOT NULL
reason     text        nullable
created_at timestamp   NOT NULL  DEFAULT now()
created_by bigint      nullable
```

---

### approval_doc_types
```
id                 bigint      NOT NULL  PK
doc_type           varchar(30) NOT NULL  UNIQUE
doc_label          varchar(200)NOT NULL
is_active          boolean     NOT NULL  DEFAULT true
table_name         varchar(50) nullable  — ชื่อตารางที่ update status
id_column          varchar(50) nullable  DEFAULT 'id'
status_column      varchar(50) nullable  DEFAULT 'status'
approved_value     varchar(30) nullable  DEFAULT 'APPROVED'
rejected_value     varchar(30) nullable  DEFAULT 'REJECTED'
status_log_table   varchar(50) nullable  — ตาราง log สถานะ
status_log_fk_column varchar(50) nullable
created_at         timestamp   NOT NULL  DEFAULT now()
updated_at         timestamp   NOT NULL  DEFAULT now()
created_by         bigint      nullable
updated_by         bigint      nullable
```

---

### approval_log
```
id          bigint      NOT NULL  PK
approval_id bigint      NOT NULL  — FK → approval_request.id
doc_type    varchar(30) NOT NULL
doc_id      bigint      NOT NULL
doc_no      varchar(30) NOT NULL
step_no     integer     NOT NULL
action      varchar(20) NOT NULL  — SUBMIT|APPROVE|REJECT|RETURN|CANCEL|ESCALATE|COMMENT
action_by   bigint      NOT NULL  — FK → users.id
action_at   timestamp   NOT NULL  DEFAULT now()
comments    text        nullable
old_status  varchar(20) nullable
new_status  varchar(20) nullable
ip_address  inet        nullable
user_agent  text        nullable
```

---

### approval_request
```
id           bigint      NOT NULL  PK
doc_type     varchar(30) NOT NULL
doc_id       bigint      NOT NULL
doc_no       varchar(30) NOT NULL
step_no      integer     NOT NULL
requested_by bigint      NOT NULL  — FK → users.id
assigned_to  bigint      nullable  — FK → users.id (ผู้อนุมัติ)
status       varchar(20) NOT NULL  DEFAULT 'PENDING'
             — PENDING|APPROVED|REJECTED|CANCELLED|ESCALATED
due_date     date        nullable
amount       numeric(18,4) nullable
created_at   timestamp   NOT NULL  DEFAULT now()
updated_at   timestamp   NOT NULL  DEFAULT now()
created_by   bigint      nullable
updated_by   bigint      nullable
```

---

### borrow
```
id             bigint      NOT NULL  PK
borrow_no      varchar(30) NOT NULL
borrow_type    varchar(20) NOT NULL  DEFAULT 'BORROW'  — BORROW|RETURN
warehouse_code varchar(20) NOT NULL
borrower_id    bigint      NOT NULL  — FK → users.id
location_code  varchar(20) nullable
borrow_date    date        NOT NULL  DEFAULT CURRENT_DATE
expected_return date       nullable
actual_return  date        nullable
status         varchar(20) NOT NULL  DEFAULT 'OPEN'
               — DRAFT|PENDING_APPROVAL|APPROVED|REJECTED|BORROWED|RETURNED|PARTIALLY_RETURNED|CANCELLED
purpose        text        nullable  — วัตถุประสงค์การยืม
approved_by    bigint      nullable  — FK → users.id
approved_at    timestamp   nullable
remarks        text        nullable
created_at     timestamp   NOT NULL  DEFAULT now()
updated_at     timestamp   NOT NULL  DEFAULT now()
created_by     bigint      NOT NULL
updated_by     bigint      nullable
```

---

### borrow_line
```
id            bigint        NOT NULL  PK
borrow_id     bigint        NOT NULL  — FK → borrow.id
line_no       integer       nullable
stock_item_id bigint        nullable  — FK → stock_item.id
mat_code      varchar(20)   NOT NULL
mat_type      varchar(20)   NOT NULL  DEFAULT 'RETURNABLE'  — RETURNABLE|CONSUMABLE
unit          varchar(50)   nullable
qty_requested numeric(18,4) nullable  — จำนวนที่ขอ
qty_approved  numeric(18,4) nullable  — จำนวนที่อนุมัติ (อาจน้อยกว่า)
qty_borrowed  numeric(18,4) NOT NULL  DEFAULT 0
qty_returned  numeric(18,4) NOT NULL  DEFAULT 0
condition_out varchar(50)   nullable  — GOOD|FAIR|DAMAGED
condition_in  varchar(50)   nullable  — GOOD|FAIR|DAMAGED
remarks       text          nullable
location_code varchar(20)   nullable
```

---

### borrow_status_log
```
id          bigint      NOT NULL  PK
borrow_id   bigint      NOT NULL  — FK → borrow.id
from_status varchar(30) nullable
to_status   varchar(30) NOT NULL
changed_by  bigint      NOT NULL  — FK → users.id
changed_at  timestamp   NOT NULL  DEFAULT now()
remarks     text        nullable
```

---

### brand
```
id         integer     NOT NULL  PK
brand_code varchar(20) NOT NULL
spec_id    integer     NOT NULL  — FK → spec_size.id
brand_name varchar(100)NOT NULL
is_active  boolean     NOT NULL  DEFAULT true
created_at timestamp   NOT NULL  DEFAULT now()
updated_at timestamp   NOT NULL  DEFAULT now()
created_by bigint      nullable
updated_by bigint      nullable
```

---

### cost_group / cost_job / cost_subgroup / cost_subject
```
-- cost_subject
id, subject_code, subject_name, is_active, created_at, updated_at, created_by, updated_by

-- cost_job (FK → cost_subject)
id, subject_id, job_code, job_name, is_active, created_at, updated_at, created_by, updated_by

-- cost_group (FK → cost_job)
id, job_id, group_code, group_name, is_active, created_at, updated_at, created_by, updated_by

-- cost_subgroup (FK → cost_group)
id, group_id, subgroup_code, subgroup_name, is_active, created_at, updated_at, created_by, updated_by
```

---

### departments
```
id        bigint      NOT NULL  PK
dept_code varchar(20) NOT NULL  UNIQUE
dept_name varchar(200)NOT NULL
is_active boolean     NOT NULL  DEFAULT true
sort_order integer    NOT NULL  DEFAULT 0
created_at timestamp  NOT NULL  DEFAULT now()
updated_at timestamp  NOT NULL  DEFAULT now()
```

---

### dept_menu_permissions
```
id         bigint   NOT NULL  PK
dept_code  varchar  NOT NULL
menu_id    bigint   NOT NULL  — FK → menus.id
can_read   boolean  NOT NULL  DEFAULT false
can_write  boolean  NOT NULL  DEFAULT false
can_update boolean  NOT NULL  DEFAULT false
can_delete boolean  NOT NULL  DEFAULT false
created_at timestamp NOT NULL DEFAULT now()
updated_at timestamp NOT NULL DEFAULT now()
created_by bigint   nullable
updated_by bigint   nullable
```

---

### erp_audit_log
```
id           bigint    NOT NULL  PK
table_name   varchar   NOT NULL
record_id    bigint    NOT NULL
action       varchar   NOT NULL  — INSERT|UPDATE|DELETE
changed_by   bigint    nullable  — FK → users.id
changed_at   timestamp NOT NULL  DEFAULT now()
old_data     jsonb     nullable
new_data     jsonb     nullable
session_info jsonb     nullable
```

---

### grn
```
id             bigint      NOT NULL  PK
grn_no         varchar(30) NOT NULL
grn_date       date        NOT NULL  DEFAULT CURRENT_DATE
po_id          bigint      NOT NULL  — FK → purchase_order.id
warehouse_code varchar(20) NOT NULL
supplier_code  varchar(20) NOT NULL
delivery_note  varchar(50) nullable  — เลขใบส่งของ
status         varchar(20) NOT NULL  DEFAULT 'DRAFT'  — DRAFT|CONFIRMED|POSTED
quality_status varchar(20) NOT NULL  DEFAULT 'PENDING' — PENDING|PASSED|FAILED|PARTIAL
received_by    bigint      NOT NULL  — FK → users.id
confirmed_by   bigint      nullable  — FK → users.id
confirmed_at   timestamp   nullable
remarks        text        nullable
score_quality  smallint    nullable  — 1-5
score_quantity smallint    nullable  — 1-5
score_ontime   smallint    nullable  — 1-5
score_notes    text        nullable
created_at     timestamp   NOT NULL  DEFAULT now()
updated_at     timestamp   NOT NULL  DEFAULT now()
created_by     bigint      nullable
updated_by     bigint      nullable
```

---

### grn_line
```
id              bigint        NOT NULL  PK
grn_id          bigint        NOT NULL  — FK → grn.id
line_no         integer       NOT NULL
po_line_id      bigint        NOT NULL  — FK → purchase_order_line.id
mat_code        varchar(20)   NOT NULL
zone_id         integer       nullable  — FK → storage_zone.id
qty_received    numeric(18,4) NOT NULL
qty_accepted    numeric(18,4) NOT NULL  DEFAULT 0
qty_rejected    numeric(18,4) NOT NULL  DEFAULT 0
quality_remarks text          nullable
```

---

### inventory
> 🔴 **ไม่ได้ใช้งานจริง (ตัดสินใจ 2026-07-27)** — table นี้กับ `inventory_transaction` มีอยู่ใน DB
> แต่ handler ไหน ๆ ก็ไม่ควรอ่าน/เขียนที่นี่ รวมถึง GRN confirm ด้วย ระบบ stock ที่ใช้จริงคือ
> `stock_item`/`stock_inventory` (ดูด้านล่าง) — อย่าสับสนสองระบบนี้เข้าด้วยกัน
```
id             bigint        NOT NULL  PK
mat_code       varchar(20)   NOT NULL  — ใช้กับ PR/PO flow
warehouse_code varchar(20)   NOT NULL
zone_id        integer       nullable
qty_on_hand    numeric(18,4) NOT NULL  DEFAULT 0
qty_reserved   numeric(18,4) NOT NULL  DEFAULT 0
qty_on_order   numeric(18,4) NOT NULL  DEFAULT 0
reorder_point  numeric(18,4) nullable
reorder_qty    numeric(18,4) nullable
min_stock      numeric(18,4) nullable
max_stock      numeric(18,4) nullable
last_counted_at timestamp    nullable
created_at     timestamp     NOT NULL  DEFAULT now()
updated_at     timestamp     NOT NULL  DEFAULT now()
created_by     bigint        nullable
updated_by     bigint        nullable
```

---

### inventory_transaction
> 🔴 **ไม่ได้ใช้งานจริง เช่นเดียวกับ `inventory`** — ดูหมายเหตุด้านบน
```
id             bigint        NOT NULL  PK
txn_no         varchar(30)   NOT NULL
txn_type       varchar(20)   NOT NULL
mat_code       varchar(20)   NOT NULL
from_warehouse varchar(20)   nullable
to_warehouse   varchar(20)   nullable
from_zone_id   integer       nullable
to_zone_id     integer       nullable
qty            numeric(18,4) NOT NULL
ref_doc_type   varchar(20)   nullable
ref_doc_no     varchar(30)   nullable
location_code  varchar(20)   nullable
reason         text          nullable
txn_date       date          NOT NULL  DEFAULT CURRENT_DATE
created_at     timestamp     NOT NULL  DEFAULT now()
updated_at     timestamp     NOT NULL  DEFAULT now()
created_by     bigint        NOT NULL
updated_by     bigint        nullable
```

---

### location
```
id            integer     NOT NULL  PK
location_code varchar(20) NOT NULL  UNIQUE
location_name varchar(100)NOT NULL
location_type varchar(20) NOT NULL
parent_id     integer     nullable  — self-ref
is_active     boolean     NOT NULL  DEFAULT true
created_at    timestamp   NOT NULL  DEFAULT now()
updated_at    timestamp   NOT NULL  DEFAULT now()
created_by    bigint      nullable
updated_by    bigint      nullable
```

---

### mat_group / subgroup / mat_name / spec_size / brand / material_code
```
-- mat_group
id, group_code, group_name, is_active, created_at, updated_at, created_by, updated_by

-- subgroup (FK → mat_group)
id, group_id, subgroup_code, subgroup_name, is_active, ...

-- mat_name (FK → subgroup)
id, mat_name_code, subgroup_id, mat_name, is_active, ...

-- spec_size (FK → mat_name)
id, spec_code, mat_name_id, spec_description, is_active, ...

-- brand (FK → spec_size)
id, brand_code, spec_id, brand_name, is_active, ...

-- material_code (FK → group, subgroup, mat_name, spec_size, brand, unit)
id, mat_code, group_id, subgroup_id, mat_name_id, spec_id, brand_id,
unit_id, cost_subgroup_id, is_active, created_at, updated_at, created_by, updated_by
```

---

### memo
```
id           bigint      NOT NULL  PK
memo_no      varchar(30) NOT NULL
title        varchar(200)NOT NULL
project_code varchar(20) nullable
requested_by bigint      NOT NULL  — FK → users.id
department   varchar(100)nullable
delivery_location varchar(255) nullable
note         text        nullable
status       varchar(20) NOT NULL  DEFAULT 'DRAFT'
approver_id  bigint      nullable
created_at   timestamp   NOT NULL  DEFAULT now()
updated_at   timestamp   NOT NULL  DEFAULT now()
created_by   bigint      nullable
updated_by   bigint      nullable
```

---

### menus
```
id          bigint      NOT NULL  PK
parent_id   bigint      nullable  — self-ref (FK → menus.id)
menu_code   varchar(50) NOT NULL  UNIQUE
menu_name   varchar(100)NOT NULL
menu_path   varchar(200)nullable
icon_name   varchar(50) nullable
sort_order  integer     DEFAULT 0
is_active   boolean     DEFAULT true
created_at  timestamp   NOT NULL  DEFAULT now()
updated_at  timestamp   NOT NULL  DEFAULT now()
created_by  bigint      nullable
updated_by  bigint      nullable
```

> ⚠️ menu_code ต้องใช้รูปแบบ MENU_XXX_YYY (UPPER_SNAKE) ให้ consistent
> ปัจจุบัน id=43 ('stock-receiving') และ id=44 ('stock-receiving-history') ยังเป็น kebab-case อยู่

> ⚠️ Memo menu ถูก rename + reorder แล้ว (ค่าปัจจุบันจริงจาก DB):
> - `MENU_MEMO` → menu_name = "ใบบันทึกขอซื้อ (Memo)", sort_order = 1
> - `MENU_MEMO_LIST` → menu_name = "รายการใบบันทึกขอซื้อ", sort_order = 1
> - `MENU_MEMO_CREATE` → menu_name = "สร้างใบบันทึกขอซื้อ", sort_order = 2
> - `MENU_MEMO_APPROVAL` → menu_name = "อนุมัติใบบันทึกขอซื้อ", sort_order = 3
> - `MENU_PR` → sort_order = 2
> - `MENU_PO` → sort_order = 3

---

### modules
```
id          bigint      NOT NULL  PK
module_code varchar(30) NOT NULL  UNIQUE
module_name varchar(100)NOT NULL
sort_order  integer     DEFAULT 0
is_active   boolean     DEFAULT true
created_at  timestamp   NOT NULL  DEFAULT now()
updated_at  timestamp   NOT NULL  DEFAULT now()
created_by  bigint      nullable
updated_by  bigint      nullable
```

---

### permissions / role_permissions / user_permissions
```
-- permissions
id, permission_key, permission_name, module_id, created_at, updated_at, created_by, updated_by

-- role_permissions
role_id, permission_id, created_at, created_by

-- user_permissions
user_id, permission_id, is_allow, created_at, updated_at, created_by, updated_by
```

---

### po_attachment / pr_attachment
```
id, [po_id|pr_id], file_name, file_path, file_size, file_type, uploaded_at, uploaded_by
```

---

### po_edit_log
```
id, po_id, edited_by, reason (NOT NULL), edited_at
```

---

### po_status_log / pr_status_log
```
id, [po_id|pr_id], from_status, to_status (NOT NULL), changed_by, changed_at, remarks
```

---

### project
```
id              integer     NOT NULL  PK
project_code    varchar(20) NOT NULL  UNIQUE
project_name    varchar(200)NOT NULL
location_code   varchar(20) nullable
start_date      date        nullable
end_date        date        nullable
status          varchar(20) NOT NULL  DEFAULT 'ACTIVE'
is_active       boolean     NOT NULL  DEFAULT true
owner_id        bigint      nullable  — FK → users.id
budget_amount   numeric(18,4) NOT NULL DEFAULT 0
consultant_name varchar(200)nullable
created_at      timestamp   NOT NULL  DEFAULT now()
updated_at      timestamp   NOT NULL  DEFAULT now()
created_by      bigint      nullable
updated_by      bigint      nullable
```

---

### purchase_order
> 🔴 **2026-07-27 — แยก status ออกเป็น 2 field แล้ว** (ก่อนหน้านี้ `status` เดียวรวมทั้ง
> approval flow และ receiving flow ปนกัน ทำให้ `PENDING_REAPPROVAL` กับ `PARTIALLY_RECEIVED`
> เก็บพร้อมกันไม่ได้) รัน migration `003_po_split_receive_status.sql` แล้วต้องใช้ตามนี้:
> - `status` = **สถานะอนุมัติเท่านั้น**: `DRAFT|PENDING_APPROVAL|APPROVED|REJECTED|PENDING_REAPPROVAL|CANCELLED`
> - `status_receive` = **สถานะรับของเท่านั้น**: `NOT_SENT|SENT|PARTIALLY_RECEIVED|RECEIVED`
> - ทั้งสอง field เป็นอิสระต่อกัน เช่น PO ที่กำลัง `PENDING_REAPPROVAL` แต่รับของไปแล้วบางส่วน
>   จะเป็น `status='PENDING_REAPPROVAL', status_receive='PARTIALLY_RECEIVED'` พร้อมกันได้ปกติ
```
id               bigint        NOT NULL  PK
po_no            varchar(30)   NOT NULL
po_date          date          NOT NULL  DEFAULT CURRENT_DATE
supplier_code    varchar(20)   NOT NULL
pr_id            bigint        nullable  — FK → purchase_request.id
rfq_id           bigint        nullable  — FK → rfq.id
currency         varchar(10)   NOT NULL  DEFAULT 'THB'
total_amount     numeric(18,4) NOT NULL  DEFAULT 0
vat_amount       numeric(18,4) NOT NULL  DEFAULT 0
net_amount       numeric(18,4) NOT NULL  DEFAULT 0
expected_date    date          nullable  — วันที่ส่งของที่คาดหวัง
status           varchar(30)   NOT NULL  DEFAULT 'DRAFT'
                 — สถานะอนุมัติเท่านั้น: DRAFT|PENDING_APPROVAL|APPROVED|REJECTED|PENDING_REAPPROVAL|CANCELLED
status_receive   varchar(20)   NOT NULL  DEFAULT 'NOT_SENT'
                 — สถานะรับของ แยกจาก status: NOT_SENT|SENT|PARTIALLY_RECEIVED|RECEIVED
payment_terms    varchar(100)  nullable
delivery_address text          nullable
remarks          text          nullable
use_discount     boolean       NOT NULL  DEFAULT false
discount_type    varchar(10)   NOT NULL  DEFAULT 'pct'
discount_amount  numeric(18,4) NOT NULL  DEFAULT 0
use_vat          boolean       NOT NULL  DEFAULT false
use_wht          boolean       NOT NULL  DEFAULT false
wht_amount       numeric(18,4) NOT NULL  DEFAULT 0
location_code    varchar(20)   nullable
location_text    varchar(200)  nullable
warehouse_code   varchar(20)   nullable
project_code     varchar(20)   nullable
requested_by     bigint        nullable  — FK → users.id
approver_id      bigint        nullable  — FK → users.id
ref              varchar(50)   nullable
created_at       timestamp     NOT NULL  DEFAULT now()
updated_at       timestamp     NOT NULL  DEFAULT now()
created_by       bigint        NOT NULL
updated_by       bigint        nullable
```

---

### purchase_order_line
> 💡 **หน้ารับเข้า (GRN) ใช้ field นี้ระบุ "PO ที่ยังรับไม่ครบ"**: filter
> `purchase_order.status IN ('APPROVED','SENT','PARTIALLY_RECEIVED')` ร่วมกับมี line ที่
> `status IN ('OPEN','PARTIAL')` — ดูรายละเอียด query ใน `CLAUDE.md` (backend) หัวข้อ
> "Session learnings (2026-07-27)"
```
id           bigint        NOT NULL  PK
po_id        bigint        NOT NULL  — FK → purchase_order.id
line_no      integer       NOT NULL
mat_code     varchar(20)   NOT NULL
pr_line_id   bigint        nullable  — FK → purchase_request_line.id
qty_ordered  numeric(18,4) NOT NULL
qty_received numeric(18,4) NOT NULL  DEFAULT 0
unit_price   numeric(18,4) NOT NULL
amount       numeric(18,4) nullable
discount     numeric(18,4) NOT NULL  DEFAULT 0
disc_type    varchar(10)   NOT NULL  DEFAULT 'pct'
line_discount numeric(18,4) NOT NULL DEFAULT 0
line_vat     numeric(18,4) NOT NULL  DEFAULT 0
line_wht     numeric(18,4) NOT NULL  DEFAULT 0
line_net     numeric(18,4) NOT NULL  DEFAULT 0
wht_rate     numeric(5,2)  nullable
status       varchar(20)   NOT NULL  DEFAULT 'OPEN'  — OPEN|PARTIAL|RECEIVED|CANCELLED
description  text          nullable
remarks      text          nullable
```

---

### purchase_request
```
id             bigint      NOT NULL  PK
pr_no          varchar(30) NOT NULL
pr_date        date        NOT NULL  DEFAULT CURRENT_DATE
requested_by   bigint      NOT NULL  — FK → users.id
warehouse_code varchar(20) nullable
required_date  date        nullable
status         varchar(20) NOT NULL  DEFAULT 'DRAFT'
priority       varchar(20) DEFAULT 'NORMAL'  — LOW|NORMAL|HIGH|URGENT
order_type     varchar(10) NOT NULL  DEFAULT 'stock'  — CHECK IN ('stock','cost') — 'stock' = ซื้อเข้าคลัง, 'cost' = ซื้อเข้าโครงการ (cost)
pr_type        varchar(10) NOT NULL  DEFAULT 'PO_WO'  — CHECK IN ('PO_WO','PO_ONLY','WO_ONLY') — เก็บไว้เฉยๆ ยังไม่มีโมดูล WO จริง ไม่มี logic ต่อ
remarks        text        nullable
project_code   varchar(20) nullable
memo_id        bigint      nullable  — FK → memo.id
location_text  varchar(200)nullable
created_at     timestamp   NOT NULL  DEFAULT now()
updated_at     timestamp   NOT NULL  DEFAULT now()
created_by     bigint      nullable
updated_by     bigint      nullable
```

---

### purchase_request_line
```
id              bigint        NOT NULL  PK
pr_id           bigint        NOT NULL  — FK → purchase_request.id
line_no         integer       NOT NULL
mat_code        varchar(20)   NOT NULL
qty_requested   numeric(18,4) NOT NULL
qty_reserved    numeric(18,4) NOT NULL  DEFAULT 0
qty_to_order    numeric(18,4) NOT NULL  DEFAULT 0
qty_ordered     numeric(18,4) NOT NULL  DEFAULT 0
status          varchar(20)   NOT NULL  DEFAULT 'OPEN'
cost_subgroup_id bigint       nullable
remarks         text          nullable
```

---

### rfq / rfq_line
```
-- rfq
id, rfq_no, rfq_date, supplier_code, pr_id, status (DEFAULT 'SENT'),
remarks, created_at, updated_at, created_by, updated_by

-- rfq_line
id, rfq_id, line_no, mat_code, qty, unit_price, currency (DEFAULT 'THB'),
lead_time_days, remarks
```

---

### roles
```
id          bigint      NOT NULL  PK
role_code   varchar(30) NOT NULL  UNIQUE
role_name   varchar(100)NOT NULL
description text        nullable
is_active   boolean     NOT NULL  DEFAULT true
level       integer     NOT NULL  DEFAULT 0
department  varchar(100)nullable
dept_code   varchar(20) nullable
created_at  timestamp   NOT NULL  DEFAULT now()
updated_at  timestamp   NOT NULL  DEFAULT now()
created_by  bigint      nullable
updated_by  bigint      nullable
```

---

### role_menu_permissions / role_menus
```
-- role_menu_permissions
id, role_id, menu_id, can_read, can_write, can_update, can_delete,
created_at, updated_at, created_by, updated_by

-- role_menus
role_id, menu_id, created_at, created_by
```

---

### stock_category
```
id          bigint      NOT NULL  PK
code        varchar(20) NOT NULL  UNIQUE
name        varchar(100)NOT NULL
description text        nullable
is_active   boolean     NOT NULL  DEFAULT true
created_at  timestamp   NOT NULL  DEFAULT now()
```

---

### stock_count / stock_count_line
```
-- stock_count
id, count_no, warehouse_code, count_date, status (DEFAULT 'DRAFT'),
remarks, approved_by, approved_at, created_at, updated_at, created_by, updated_by

-- stock_count_line
id, count_id, mat_code, zone_id, qty_system, qty_counted, qty_diff, remarks
```

---

### stock_inventory
```
id             bigint        NOT NULL  PK
item_id        bigint        NOT NULL  — FK → stock_item.id
location_code  varchar(20)   NOT NULL
warehouse_code varchar(20)   NOT NULL
qty_on_hand    numeric(18,4) NOT NULL  DEFAULT 0
qty_reserved   numeric(18,4) NOT NULL  DEFAULT 0
qty_available  numeric(18,4) nullable  — computed
updated_at     timestamp     NOT NULL  DEFAULT now()
```

---

### stock_item
> ✅ **ระบบ stock ที่ใช้งานจริง (2026-07-27)** — คู่กับ Borrow/Return module, `qty` ตัดตรงนี้ตอน
> borrow/requisition ได้รับอนุมัติ ผูกกับ `item_id` คนละตัวกับ `mat_code` (ไม่มี FK เชื่อมกัน แม้ค่า
> จะซ้ำได้) อย่าสับสนกับ `inventory`/`inventory_transaction` ด้านบนซึ่งไม่ได้ใช้งาน
```
id             bigint        NOT NULL  PK
mat_code       varchar(30)   NOT NULL  — ใช้ค่าเดียวกับ material_code แต่ไม่มี FK
item_name      varchar(255)  NOT NULL
description    text          nullable
category_id    bigint        nullable  — FK → stock_category.id
item_type      varchar(20)   NOT NULL  DEFAULT 'RETURNABLE'  — RETURNABLE|CONSUMABLE
tracking_type  varchar(10)   NOT NULL  DEFAULT 'sku'  — sku|serial
unit           varchar(50)   NOT NULL
qty            numeric(18,4) NOT NULL  DEFAULT 0  ← ตัดที่นี่เมื่อ borrow/requisition อนุมัติ
unit_cost      numeric(18,4) NOT NULL  DEFAULT 0
warehouse_code varchar(20)   NOT NULL  DEFAULT 'WH01'
location_code  varchar(20)   NOT NULL  DEFAULT 'SAL'
qr_code        text          nullable
is_active      boolean       NOT NULL  DEFAULT true
created_at     timestamp     NOT NULL  DEFAULT now()
updated_at     timestamp     NOT NULL  DEFAULT now()
created_by     bigint        nullable
updated_by     bigint        nullable
```

---

### stock_reservation
```
id              bigint        NOT NULL  PK
reservation_no  varchar(30)   NOT NULL
item_id         bigint        NOT NULL  — FK → stock_item.id
location_code   varchar(20)   NOT NULL
qty_reserved    numeric(18,4) NOT NULL
qty_fulfilled   numeric(18,4) NOT NULL  DEFAULT 0
status          varchar(20)   NOT NULL  DEFAULT 'PENDING'
requested_by    bigint        NOT NULL  — FK → users.id
needed_by       date          nullable
purpose         text          nullable
ref_doc_type    varchar(20)   nullable
ref_doc_id      bigint        nullable
created_at      timestamp     NOT NULL  DEFAULT now()
updated_at      timestamp     NOT NULL  DEFAULT now()
cancelled_at    timestamp     nullable
```

---

### stock_transaction
```
id            bigint        NOT NULL  PK
txn_no        varchar(30)   NOT NULL
txn_type      varchar(20)   NOT NULL
item_id       bigint        NOT NULL  — FK → stock_item.id
from_location varchar(20)   nullable
to_location   varchar(20)   nullable
qty           numeric(18,4) NOT NULL
qty_before    numeric(18,4) nullable
qty_after     numeric(18,4) nullable
ref_doc_type  varchar(20)   nullable
ref_doc_id    bigint        nullable
remarks       text          nullable
txn_date      date          NOT NULL  DEFAULT CURRENT_DATE
created_at    timestamp     NOT NULL  DEFAULT now()
created_by    bigint        NOT NULL
```

---

### stock_transfer
ใบย้ายคลัง (ย้ายคลัง). `transfer_type` กำหนดว่า field ไหนของ from_*/to_* ต้องมีค่า:
`WH_TO_WH` → from_warehouse_code + to_warehouse_code, `WH_TO_PROJECT` → from_warehouse_code +
to_project_code, `PROJECT_TO_WH` → from_project_code + to_warehouse_code.
```
id                   bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
transfer_no          varchar(30)   NOT NULL
transfer_type        varchar(20)   NOT NULL  — WH_TO_WH | WH_TO_PROJECT | PROJECT_TO_WH
transfer_date        date          NOT NULL  DEFAULT CURRENT_DATE
from_warehouse_code  varchar(20)   nullable
from_project_code    varchar(20)   nullable
to_warehouse_code    varchar(20)   nullable
to_project_code      varchar(20)   nullable
requested_by         bigint        NOT NULL
purpose              text          nullable
remarks              text          nullable
status               varchar(20)   NOT NULL  DEFAULT 'DRAFT' — DRAFT | CONFIRMED | CANCELLED
checked_by           bigint        nullable
checked_at           timestamp     nullable
created_at           timestamp     NOT NULL  DEFAULT now()
updated_at           timestamp     NOT NULL  DEFAULT now()
created_by           bigint        NOT NULL
updated_by           bigint        nullable
```

### stock_transfer_line
```
id             bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
transfer_id    bigint        NOT NULL  — FK → stock_transfer.id
line_no        integer       NOT NULL
item_id        bigint        NOT NULL  — FK → stock_item.id (source item for WH_TO_WH/WH_TO_PROJECT,
                                          destination item for PROJECT_TO_WH — see StockTransferHandler.Create)
mat_code       varchar(30)   NOT NULL
unit           varchar(50)   nullable
qty_requested  numeric(18,4) NOT NULL
qty_confirmed  numeric(18,4) nullable
remarks        text          nullable
```

### stock_transfer_status_log
```
id           bigint      NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
transfer_id  bigint      NOT NULL  — FK → stock_transfer.id
from_status  varchar(20) nullable
to_status    varchar(20) NOT NULL
changed_by   bigint      NOT NULL
changed_at   timestamp   NOT NULL  DEFAULT now()
remarks      text        nullable
```

---

### requisition
ใบเบิกของ (คลัง → โครงการ). ตารางนี้มีอยู่ใน DB ก่อนหน้านี้แล้ว (ไม่ใช่ table ใหม่จาก session นี้)
แยกออกจาก `stock_transfer` โดยเจตนา — คนละ status flow, header ผูก project_code+warehouse_code
ตรงๆ ไม่มี transfer_type.
```
id             bigint        NOT NULL  PK  (nextval requisition_id_seq)
req_no         varchar(30)   NOT NULL  UNIQUE
project_code   varchar(20)   NOT NULL  — FK → project.project_code
warehouse_code varchar(20)   NOT NULL
requester_id   bigint        NOT NULL  — FK → users.id
req_date       date          NOT NULL  DEFAULT CURRENT_DATE
purpose        text          nullable
status         varchar(20)   NOT NULL  DEFAULT 'DRAFT' — DRAFT | SUBMITTED | ISSUED | CANCELLED
checked_by     bigint        nullable  — FK → users.id
checked_at     timestamp     nullable
remarks        text          nullable
created_at     timestamp     NOT NULL  DEFAULT now()
updated_at     timestamp     NOT NULL  DEFAULT now()
created_by     bigint        NOT NULL
updated_by     bigint        nullable
```

### requisition_line
```
id             bigint        NOT NULL  PK  (nextval requisition_line_id_seq)
req_id         bigint        NOT NULL  — FK → requisition.id
line_no        integer       nullable
stock_item_id  bigint        nullable  — FK → stock_item.id
mat_code       varchar(20)   NOT NULL
unit           varchar(50)   nullable
qty_requested  numeric(18,4) NOT NULL
qty_issued     numeric(18,4) NOT NULL  DEFAULT 0
unit_cost      numeric(18,4) nullable
total_cost     numeric(18,4) nullable
remarks        text          nullable
```

### requisition_status_log
```
id           bigint      NOT NULL  PK  (nextval requisition_status_log_id_seq)
req_id       bigint      NOT NULL  — FK → requisition.id
from_status  varchar(30) nullable
to_status    varchar(30) NOT NULL
changed_by   bigint      NOT NULL  — FK → users.id
changed_at   timestamp   NOT NULL  DEFAULT now()
remarks      text        nullable
```

---

### project_stock
ยอดวัสดุคงเหลือที่โครงการ อัปเดตโดย `RequisitionHandler.Issue` และ
`StockTransferHandler.Confirm` (WH_TO_PROJECT เพิ่ม, PROJECT_TO_WH ลด) — ไม่มี handler CRUD ตรงๆ
ของตัวเอง เขียนผ่าน helper `addToProjectStock`/`deductProjectStock` เท่านั้น.
```
id           bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
project_code varchar(20)   NOT NULL
mat_code     varchar(30)   NOT NULL
unit         varchar(50)   nullable
qty_on_hand  numeric(18,4) NOT NULL  DEFAULT 0
updated_at   timestamp     NOT NULL  DEFAULT now()
-- UNIQUE (project_code, mat_code)
```

---

### storage_zone
```
id            integer     NOT NULL  PK
warehouse_id  integer     NOT NULL  — FK → warehouse.id
zone_code     varchar(20) NOT NULL
zone_name     varchar(100)NOT NULL
zone_type     varchar(20) nullable
created_at    timestamp   NOT NULL  DEFAULT now()
updated_at    timestamp   NOT NULL  DEFAULT now()
created_by    bigint      nullable
updated_by    bigint      nullable
```

---

### supplier
```
id                   integer     NOT NULL  PK
supplier_code        varchar(20) NOT NULL  UNIQUE
supplier_name        varchar(200)NOT NULL
supplier_short_name  varchar(50) nullable
tax_id               varchar(20) nullable
address              text        nullable
contact_name         varchar(100)nullable
contact_phone        varchar(20) nullable
contact_email        varchar(100)nullable
office_phone         varchar(20) nullable
fax                  varchar(20) nullable
payment_terms        varchar(50) nullable
currency             varchar(10) DEFAULT 'THB'
sales_person         varchar(100)nullable
is_active            boolean     NOT NULL  DEFAULT true
created_at           timestamp   NOT NULL  DEFAULT now()
updated_at           timestamp   NOT NULL  DEFAULT now()
created_by           bigint      nullable
updated_by           bigint      nullable
```

---

### unit
```
id        integer     NOT NULL  PK
unit_code varchar(20) NOT NULL  UNIQUE
unit_name varchar(50) NOT NULL
is_active boolean     NOT NULL  DEFAULT true
created_at timestamp  NOT NULL  DEFAULT now()
updated_at timestamp  NOT NULL  DEFAULT now()
created_by bigint     nullable
updated_by bigint     nullable
```

---

### user_menu_permissions
```
id         bigint   NOT NULL  PK
user_id    bigint   NOT NULL  — FK → users.id
menu_id    bigint   NOT NULL  — FK → menus.id
can_read   boolean  NOT NULL  DEFAULT false
can_write  boolean  NOT NULL  DEFAULT false
can_update boolean  NOT NULL  DEFAULT false
can_delete boolean  NOT NULL  DEFAULT false
created_at timestamp NOT NULL DEFAULT now()
updated_at timestamp NOT NULL DEFAULT now()
created_by bigint   nullable
updated_by bigint   nullable
```

---

### users
```
id            bigint      NOT NULL  PK
username      varchar(50) NOT NULL  UNIQUE
full_name     varchar(200)NOT NULL
email         varchar(100)nullable
password_hash text        nullable
is_active     boolean     NOT NULL  DEFAULT true
employee_code varchar(30) nullable
department    varchar(100)nullable
dept_code     varchar(20) nullable
location_code varchar(20) nullable
created_at    timestamp   NOT NULL  DEFAULT now()
updated_at    timestamp   NOT NULL  DEFAULT now()
created_by    bigint      nullable
updated_by    bigint      nullable
```

---

### user_roles
```
user_id    bigint    NOT NULL  — FK → users.id
role_id    bigint    NOT NULL  — FK → roles.id
created_at timestamp NOT NULL  DEFAULT now()
created_by bigint    nullable
```

---

### warehouse
```
id             integer     NOT NULL  PK
warehouse_code varchar(20) NOT NULL  UNIQUE
warehouse_name varchar(100)NOT NULL
address        text        nullable
is_active      boolean     NOT NULL  DEFAULT true
created_at     timestamp   NOT NULL  DEFAULT now()
updated_at     timestamp   NOT NULL  DEFAULT now()
created_by     bigint      nullable
updated_by     bigint      nullable
```

---

### work_order
> หนังสือสั่งจ้าง (subcontractor hiring document) — คนละเอกสารกับ purchase_order
```
id                       bigint        NOT NULL  PK
wo_no                    varchar(30)   NOT NULL  UNIQUE
wo_date                  date          NOT NULL  DEFAULT CURRENT_DATE
employer_name            varchar(200)  NOT NULL  — compose จาก branch dropdown ฝั่ง frontend
project_code             varchar(20)   nullable  — FK → project.project_code (ถ้าผูก)
project_scope_text       text          nullable
supplier_code            varchar(20)   nullable  — FK → supplier.supplier_code (ถ้าผูก)
supplier_name            varchar(200)  NOT NULL
contact_person           varchar(100)  nullable
supplier_address         text          nullable
supplier_phone           varchar(50)   nullable
contract_type            varchar(20)   NOT NULL  DEFAULT 'LABOR_MATERIAL'
                         — LABOR_ONLY | LABOR_MATERIAL
work_system              varchar(5)    NOT NULL  — P | E | S
contract_description     varchar(30)   nullable  — dropdown code (ดู Important Notes)
contract_amount          numeric(18,2) NOT NULL  DEFAULT 0
vat_rate                 numeric(5,2)  NOT NULL  DEFAULT 7.00   -- 🔴 legacy, ดูหมายเหตุ
wht_rate                 numeric(5,2)  NOT NULL  DEFAULT 3.00   -- 🔴 legacy, ดูหมายเหตุ
advance_pct              numeric(5,2)  NOT NULL  DEFAULT 0
advance_amount           numeric(18,2) NOT NULL  DEFAULT 0     -- label ใน UI: "เงินงวดสัญญา"
progress_payment_note    text          nullable
retention_pct            numeric(5,2)  NOT NULL  DEFAULT 5.00
advance_deduct_pct       numeric(5,2)  NOT NULL  DEFAULT 0
other_deduction_note     text          nullable
start_date               date          nullable
duration_days            integer       nullable
end_date                 date          nullable
penalty_pct_per_day      numeric(5,2)  NOT NULL  DEFAULT 0
warranty_years           integer       NOT NULL  DEFAULT 1
ref_no                   varchar(50)   nullable
other_terms              text          nullable
cost_code                varchar(50)   nullable   -- 🔴 DEPRECATED, ดูหมายเหตุ
status                   varchar(20)   NOT NULL  DEFAULT 'DRAFT'
                         — DRAFT|PENDING_APPROVAL|APPROVED|REJECTED|CANCELLED
use_discount             boolean       NOT NULL  DEFAULT false
discount_type            varchar(20)   nullable  — 'PERCENT'|'AMOUNT'
use_vat                  boolean       NOT NULL  DEFAULT true
use_wht                  boolean       NOT NULL  DEFAULT true
total_amount             numeric(18,2) NOT NULL  DEFAULT 0   -- subtotal ก่อนหักส่วนลด
discount_amount          numeric(18,2) NOT NULL  DEFAULT 0
vat_amount               numeric(18,2) NOT NULL  DEFAULT 0
wht_amount               numeric(18,2) NOT NULL  DEFAULT 0
net_amount               numeric(18,2) NOT NULL  DEFAULT 0
entered_by / entered_at / section_head_id / section_head_signed_at /
authorized_by / authorized_at / subcontractor_signed_name / subcontractor_signed_at
remarks, created_at, updated_at, created_by, updated_by  — เหมือนเดิม ไม่เปลี่ยน
```
> 🔴 `work_order.vat_rate`/`wht_rate` (คอลัมน์เดิมจาก schema แรกสุด) ไม่ได้ใช้จากโค้ดใหม่แล้ว —
> ถูกแทนที่ด้วย `work_order_line.vat_rate`/`wht_rate` ต่อบรรทัด บวก `use_vat`/`use_wht` ระดับ
> header ยังไม่ลบคอลัมน์เดิม รอ confirm กับทีมก่อน
>
> 🔴 `work_order.cost_code` (คอลัมน์เดิม) ก็ deprecated เช่นกัน แทนที่ด้วยตาราง
> `work_order_line` ทั้งหมด

---

### work_order_line
```
id          bigint        NOT NULL  PK
wo_id       bigint        NOT NULL  — FK → work_order.id
sort_order  integer       NOT NULL  DEFAULT 0
cost_code   varchar(50)   NOT NULL  — แทนที่ item/material reference ของ PO
description text          nullable
qty         numeric(18,4) NOT NULL  DEFAULT 0
unit_price  numeric(18,4) NOT NULL  DEFAULT 0
amount      numeric(18,2) GENERATED ALWAYS AS (qty * unit_price) STORED  -- mirror PO
disc        numeric(18,2) NOT NULL  DEFAULT 0
disc_type   varchar(20)   nullable  — 'PERCENT'|'AMOUNT'
vat_rate    numeric(5,2)  NOT NULL  DEFAULT 7.00   -- ของใหม่ ไม่ mirror จาก PO
wht_rate    numeric(5,2)  nullable  — CHECK (1,3,5) mirror จาก PO
created_at  timestamp     NOT NULL  DEFAULT now()
created_by  bigint        NOT NULL
```

---

### work_order_status_log / work_order_attachment
> mirror `po_status_log` / `po_attachment` — ไม่มีอะไรเปลี่ยนจากฉบับ schema แรกสุด

---

### work_order menus
```
MENU_WO           — หนังสือสั่งจ้าง (WO)     sort_order=4  (หลัง PO=3, ก่อน STOCK=5)
MENU_WO_LIST      — รายการหนังสือสั่งจ้าง      sort_order=1 (child)
MENU_WO_CREATE    — สร้างหนังสือสั่งจ้าง       sort_order=2 (child)
MENU_WO_APPROVAL  — อนุมัติหนังสือสั่งจ้าง      sort_order=3 (child)
```
ชื่อเมนู parent ปัจจุบัน: **"หนังสือสั่งจ้าง (WO)"** (เปลี่ยนจาก "(Work Order)" ตามที่ขอทีหลัง)

---

## Tables ที่อาจไม่ได้ใช้ (ตรวจสอบ row_count ก่อน DROP)

```sql
SELECT
    t.table_name,
    (xpath('/row/cnt/text()',
        query_to_xml('SELECT COUNT(*) AS cnt FROM public.' || t.table_name, false, false, ''))
    )[1]::text::int AS row_count
FROM information_schema.tables t
WHERE t.table_schema = 'public'
  AND t.table_type = 'BASE TABLE'
ORDER BY row_count ASC, t.table_name;
```

ผลที่ได้ row_count = 0 ให้พิจารณา DROP ทีละตัว ระวัง FK constraint ก่อน DROP ทุกครั้ง
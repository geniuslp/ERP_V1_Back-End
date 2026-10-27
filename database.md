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
| [customer](#customer) | Master | ลูกค้า |
| [departments](#departments) | Org | แผนก |
| [dept_menu_permissions](#dept_menu_permissions) | Permission | สิทธิ์เมนูตามแผนก |
| [erp_audit_log](#erp_audit_log) | Audit | audit log ทั้งระบบ |
| [grn](#grn) | GRN | ใบรับเข้าสินค้า |
| [grn_line](#grn_line) | GRN | รายการใน GRN |
| [ic_po_receive_document](#ic_po_receive_document) | IC | หัวเอกสารรับเข้า (Tab 1) ของ IC PO Receive, 1 แถวต่อ PO |
| [ic_project_cost_item](#ic_project_cost_item) | IC | ยอดคงเหลือของ order_type='cost' ต่อ project+mat_code+cost_subgroup |
| [ic_project_cost_item_transaction](#ic_project_cost_item_transaction) | IC | log การรับ/คืนของ ic_project_cost_item |
| [ic_project_movement](#ic_project_movement) | IC | หัวเอกสารตัดเบิก/โอน (ISSUE/TRANSFER) ของ project cost |
| [ic_project_movement_line](#ic_project_movement_line) | IC | รายการใน ic_project_movement |
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
| [payment_log](#payment_log) | Payment | ประวัติการจ่ายเงินของ PO/WO |
| [permission_audit_logs](#permission_audit_logs) | Permission | audit log สิทธิ์ |
| [permissions](#permissions) | Permission | permission key |
| [petty_cash_requisition](#petty_cash_requisition) | PettyCash | ใบเบิกเงินสดย่อย |
| [petty_cash_requisition_line](#petty_cash_requisition_line) | PettyCash | รายการใน petty_cash_requisition |
| [petty_cash_status_log](#petty_cash_status_log) | PettyCash | log สถานะ petty_cash_requisition |
| [po_attachment](#po_attachment) | PO | ไฟล์แนบ PO |
| [po_edit_log](#po_edit_log) | PO | log การแก้ไข PO |
| [po_number_counter](#po_number_counter) | PO | ตัว generate po_no แบบ monthly-reset (แทน po_seq) |
| [po_status_log](#po_status_log) | PO | log สถานะ PO |
| [por_number_counter](#por_number_counter) | IC | ตัว generate receive_no (IC-YYYYMM-NNNN) ของ IC |
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
| [stock_item_image](#stock_item_image) | Stock | รูปภาพของ stock_item |
| [stock_reservation](#stock_reservation) | Stock | การจองวัสดุ |
| [stock_transaction](#stock_transaction) | Stock | transaction stock_item |
| [stock_transfer](#stock_transfer) | Stock | 🔴 ใบย้ายคลัง — **ไม่พบใน live DB ที่ตรวจสอบล่าสุด (2026-09-09)**, ดูหมายเหตุ |
| [stock_transfer_line](#stock_transfer_line) | Stock | 🔴 ไม่พบใน live DB ที่ตรวจสอบล่าสุด (2026-09-09) |
| [stock_transfer_status_log](#stock_transfer_status_log) | Stock | 🔴 ไม่พบใน live DB ที่ตรวจสอบล่าสุด (2026-09-09) |
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
| [work_order_cost_code](#work_order_cost_code) | WO | 🔴 deprecated — multi-select cost_code แบบเก่า ก่อนเปลี่ยนเป็น work_order_line |
| [work_order_line](#work_order_line) | WO | รายการต่อบรรทัด (cost_code แทน item) |
| [work_order_status_log](#work_order_status_log) | WO | log สถานะ work_order |
| [work_order_attachment](#work_order_attachment) | WO | ไฟล์แนบ work_order |
| [work_order_payment_installment](#work_order_payment_installment) | WO | งวดชำระเงินของ WO (แทน advance_pct/advance_amount) |
| [work_order_retention](#work_order_retention) | WO | เงินประกันผลงานของ WO (แทน retention_pct) |
| [work_order_penalty](#work_order_penalty) | WO | ค่าปรับของ WO (แทน penalty_pct_per_day) |
| [wo_contractor_log](#wo_contractor_log) | WO | บันทึกงานของผู้รับเหมาตามใบสั่งจ้าง (tab ข้อมูลเอกสาร) |
| [wo_contractor_log_term](#wo_contractor_log_term) | WO | แถว Contract/Advance/Retention ของ wo_contractor_log |
| [wo_contractor_log_bill](#wo_contractor_log_bill) | WO | รายการรับวางบิล (tab ประเภทรับวางบิล) |
| [wo_contractor_log_bill_line](#wo_contractor_log_bill_line) | WO | 7 บรรทัดสรุปรายการเบิกของ bill |
| [wo_contractor_log_bill_invoice](#wo_contractor_log_bill_invoice) | WO | ใบกำกับภาษี/invoice ของ bill |

---

## ⚠️ Important Notes

- `stock_item.qty` — qty จริงของวัสดุในคลัง ใช้ตัดเมื่อ borrow/requisition อนุมัติ
- `inventory.qty_on_hand` — ใช้กับ PR/PO flow (mat_code based) คนละตัวกับ stock_item
- `material_code.mat_code` ≠ `stock_item.mat_code` — ใช้ค่าเดียวกันแต่คนละตาราง ไม่มี FK ข้าม
- 🔴 **`stock_item.mat_code` เป็น `UNIQUE(mat_code)` เดี่ยวๆ (ยืนยันจาก live DB, 2026-09-09)** —
  หมายเหตุก่อนหน้านี้ที่อ้างว่าเปลี่ยนเป็น `UNIQUE(mat_code, warehouse_code)` (2026-08-16) เป็นข้อมูล
  ที่ล้าสมัย/ไม่เคย apply จริงกับ instance ที่ตรวจสอบ — constraint จริงคือ `stock_item_code_uq
  UNIQUE (mat_code)` คอลัมน์เดียว ดังนั้น 1 mat_code = 1 แถวเท่านั้นทั่วทั้งระบบ (สอดคล้องกับ
  CLAUDE.md session note 2026-08-27 "corrected")
- `borrow_type` — `'BORROW'` = ขอยืม/เบิก, `'RETURN'` = คืน
- `borrow_line.mat_type` — `'RETURNABLE'` = ต้องคืน, `'CONSUMABLE'` = ไม่ต้องคืน
- `grn` มี `quality_status`, `confirmed_by`, `delivery_note` — ใช้ schema นี้ ไม่ใช่ inventory table
- `purchase_request.order_type` — varchar(30), CHECK `'stock'|'cost'|'asset_machine'|'asset_office_equipment'|'asset_tools'`. `'stock'` = ซื้อเข้าคลัง, `'cost'` = ซื้อเข้าโครงการ; อีก 3 ค่า (asset_machine/asset_office_equipment/asset_tools) = ซื้อทรัพย์สิน/อุปกรณ์สำนักงาน/เครื่องมือ — ไม่มีขั้นตอนรับของ ไม่เข้า IC, cost code ผูกกับ OH/General เท่านั้น — คนละความหมายกับ `pr_type`
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
- 🔴 **`po_seq` (sequence เดิมของ po_no) ถูกยกเลิกแล้ว — แทนที่ด้วย [po_number_counter](#po_number_counter)**
  (monthly-reset counter, pattern เดียวกับ pr_no/memo_no) ยืนยันจากโค้ด `internal/handlers/po.go`
  (`createPOTx` เขียนลง `po_number_counter` โดยตรง ไม่มีการเรียก `nextval('po_seq')`
  เหลืออยู่ในโค้ดปัจจุบันแล้ว) — **ยังไม่ได้ยืนยันด้วย query `information_schema` ตรงๆ ว่า sequence
  object `po_seq` เองถูก DROP ออกจาก DB จริงหรือแค่เลิกใช้งาน** เช็ค `information_schema.sequences`
  ก่อนอ้างว่า object นี้หายไปจริง
- 🔴 `purchase_order_line.cost_subgroup_id` (nullable, FK → cost_subgroup.id) มีอยู่แล้วในโค้ด
  (`po.go` select/insert) แต่ตกหล่นจาก field list เดิมของ [purchase_order_line](#purchase_order_line)
  ด้านล่าง — เพิ่มเข้าไปแล้ว ใช้เป็น Cost Code ต่อบรรทัด mirror จาก `purchase_request_line.cost_subgroup_id`
- 🔴 `purchase_order.job_code` และ `purchase_order.order_type` (`'stock'|'cost'`, ความหมายเดียวกับ
  `purchase_request.order_type`) มีอยู่แล้วในโค้ดเช่นกัน แต่ตกหล่นจาก field list ของ
  [purchase_order](#purchase_order) — เพิ่มเข้าไปแล้ว `job_code` เป็น required เสมอ (auto-fill จาก
  PR ถ้าไม่ส่งมาและมี pr_id), `order_type` บังคับ 'stock' หรือ 'cost' — 'cost' บังคับต้องมี pr_id ด้วย

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
id, group_id, subgroup_code, subgroup_name, is_active, created_at, updated_at, created_by, updated_by,
is_head, is_major
```
> 🆕 2026-10 — `is_major boolean NOT NULL DEFAULT false` + `CONSTRAINT chk_cost_subgroup_major_not_head CHECK (NOT (is_major AND is_head))`.
> **Applied on dev and UAT** (by hand in pgAdmin — no migration file, per project policy).
> `is_head` is read/written by the code (`cost_code_handler.go`, `head_cost_code.go`) but was missing from this listing; its
> column definition (type/default) is not verified here.
> Hierarchy: **Major** (one per job, `group_code '00'`, `subgroup_code '000'`: MP00000, ME00000, MS00000, MF00000, MG00000,
> MH00000 and the OH job one = 7 rows on dev) → **Head** (`is_head`, one per group) → normal rows. Only normal rows are
> selectable on documents. There is no parent column — the relation is derived from job/group. (The "7 Major rows" count is
> from the dev DB, not re-queried when this note was written.)
> Display: for `cost_subject.subject_code='OH'` the subject prefix is omitted (e.g. `G00000`) — implemented by `costCodeSQL`
> in `internal/handlers/cost_code_sql.go` and used across ~14 handler files; the DB value stays `OH`. Not every query was audited.

---

### customer
```
cus_id        integer      NOT NULL  PK  (nextval customer_cus_id_seq)
customer_code varchar(20)  NOT NULL  UNIQUE
customer_name varchar(255) NOT NULL
address       text         nullable
contact       text         nullable
credit_term   varchar(50)  nullable
remarks       text         nullable
is_active     boolean      NOT NULL  DEFAULT true
created_at    timestamptz  NOT NULL  DEFAULT now()
created_by    bigint       nullable
updated_at    timestamptz  NOT NULL  DEFAULT now()
```
⚠️ PK คือ `cus_id` (ไม่ใช่ `id` เหมือน table อื่นส่วนใหญ่) — ระวังเวลาเขียน SQL join/handler ใหม่

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
supplier_id    integer     nullable  — FK → supplier.id (clean-break migration off supplier_code,
                                       2026-08-24 — no backfill, pre-migration rows are NULL here)
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

### ic_po_receive_document
> 🆕 2026-09-21 — หัวเอกสาร Tab 1 ของหน้า IC PO Receive (Inventory Control module) 1 แถวต่อ 1 PO
> เท่านั้น (`UNIQUE(po_id)`, 409 ถ้า submit ซ้ำ) สร้างตอน submit Tab 1
> (`POST /ic/pos/{poId}/receive-document`) `receive_no` เป็น `nullable` ตอนสร้างแถวนี้ ยังไม่ generate
> — ถูกเติมทีเดียวตอน submit line-item รอบแรกที่สำเร็จ (`POST /ic/pos/{poId}/receive-lines`) ผ่าน
> [por_number_counter](#por_number_counter) แล้วไม่ regenerate อีกในรอบ partial-receive ถัดไป
> (idempotent) `due_date` คำนวณ = `tax_invoice_date + credit_days`, `credit_days` parse มาจาก
> supplier.payment_terms ของ PO นั้น (ผ่าน `parsePaymentTermsDays`)
```
id                bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
po_id             bigint        NOT NULL  UNIQUE  — FK → purchase_order.id
tax_invoice_no    varchar       nullable
tax_invoice_date  date          NOT NULL
temp_delivery_no  varchar       nullable
temp_delivery_date date         nullable
receive_no        varchar(30)   nullable  — เติมครั้งเดียวตอน submit line-item รอบแรกสำเร็จ, รูปแบบ IC-YYYYMM-NNNN
credit_days       integer       nullable  — parse จาก supplier.payment_terms
due_date          date          nullable  — = tax_invoice_date + credit_days
exchange_rate     numeric       nullable
remarks           text          nullable
created_at        timestamp     NOT NULL  DEFAULT now()
updated_at        timestamp     NOT NULL  DEFAULT now()
created_by        bigint        nullable
updated_by        bigint        nullable
```

---

### ic_project_cost_item
> 🆕 2026-09-21 — ยอดคงเหลือของ order_type='cost' (ซื้อเข้าโครงการแบบ cost ไม่ใช่ stock) คู่ขนานกับ
> `stock_item` (ที่ใช้กับ order_type='stock') ผูกกับ `project_code + mat_code + cost_subgroup_id`
> (`UNIQUE(project_code, mat_code, cost_subgroup_id)`) — auto-create แถวใหม่ (`qty_on_hand=0`,
> `last_unit_cost=0`) ตอน receive ถ้ายังไม่มีแถวสำหรับ combination นั้น โดยดึง `item_name`/`unit`
> จาก `material_code` (join mat_name/spec_size/unit) ตอน return **ไม่** auto-create — คืนของที่ไม่เคย
> รับมาก่อนจะ error
```
id               bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
project_code     varchar(20)   NOT NULL
mat_code         varchar(30)   NOT NULL
cost_subgroup_id bigint        NOT NULL  — FK → cost_subgroup.id
item_name        varchar       nullable
unit             varchar       nullable
qty_on_hand      numeric(18,4) NOT NULL  DEFAULT 0
last_unit_cost   numeric(18,4) NOT NULL  DEFAULT 0
updated_at       timestamp     NOT NULL  DEFAULT now()
-- UNIQUE (project_code, mat_code, cost_subgroup_id)
```

---

### ic_project_cost_item_transaction
> 🆕 2026-09-21 — log การรับ (`qty` บวก) / คืน (`qty` ลบ) เข้า `ic_project_cost_item` แต่ละครั้ง
> mirror บทบาทเดียวกับ `stock_transaction` ฝั่ง stock
> 🆕 2026-09-24 — po_id/po_line_id เปลี่ยนเป็น nullable, เพิ่ม `movement_id`, `movement_line_id`,
> `ref_type` เพื่อรองรับ log จาก [ic_project_movement](#ic_project_movement) (ตัดเบิก/โอน) ไม่ใช่แค่
> PO receive/return — CHECK constraint บังคับว่าต้องมีอย่างใดอย่างหนึ่งเท่านั้น (PO ref หรือ movement
> ref) ไม่ปนกัน: `ref_type='PO'` ต้องมี po_id+po_line_id (movement_id/movement_line_id เป็น NULL),
> `ref_type IN ('MOVEMENT_ISSUE','MOVEMENT_TRANSFER')` ต้องมี movement_id+movement_line_id
> (po_id/po_line_id เป็น NULL) — submit ลง 2 แถวต่อ line (ต้นทาง `qty` ลบ, ปลายทาง `qty` บวก)
```
id                   bigint        NOT NULL  PK  DEFAULT nextval('ic_project_cost_item_transaction_id_seq')
project_cost_item_id bigint        NOT NULL  — FK → ic_project_cost_item.id
po_id                bigint        nullable  — FK → purchase_order.id (เฉพาะ ref_type='PO')
po_line_id           bigint        nullable  — FK → purchase_order_line.id (เฉพาะ ref_type='PO')
qty                  numeric(18,4) NOT NULL  — บวก=รับ/เข้า, ลบ=คืน/ออก
qty_before           numeric(18,4) NOT NULL
qty_after            numeric(18,4) NOT NULL
unit_cost            numeric(18,4) NOT NULL
txn_date             date          NOT NULL  DEFAULT CURRENT_DATE
created_at           timestamp     NOT NULL  DEFAULT now()
created_by           bigint        nullable
remarks              text          nullable
receive_document_id  bigint        nullable  — FK → ic_po_receive_document.id
movement_id          bigint        nullable  — FK → ic_project_movement.id (เฉพาะ ref_type='MOVEMENT_*')
movement_line_id     bigint        nullable  — FK → ic_project_movement_line.id (เฉพาะ ref_type='MOVEMENT_*')
ref_type             varchar(20)   NOT NULL  DEFAULT 'PO'  — 'PO' | 'MOVEMENT_ISSUE' | 'MOVEMENT_TRANSFER'
-- CHECK chk_ic_project_cost_item_txn_ref_consistency:
--   ref_type='PO' → po_id+po_line_id NOT NULL, movement_* NULL
--   ref_type IN ('MOVEMENT_ISSUE','MOVEMENT_TRANSFER') → movement_id+movement_line_id NOT NULL, po_* NULL
```

---

### ic_project_movement
> 🆕 2026-09-24 — หัวเอกสารตัดเบิก (ISSUE) / โอน (TRANSFER) ของ project cost item ผูกกับ
> `project_code` + `job_code` (`job_code` ต้องเป็นหนึ่งใน `project.job_codes`) — status: `DRAFT` ตอน
> สร้าง, เปลี่ยนเป็น `POSTED` ตอน submit (ดู CLAUDE.md session เดียวกันนี้สำหรับ submit logic:
> lock+validate+หักต้นทาง/เครดิตปลายทาง แบบ all-or-nothing) submit แล้วห้ามเพิ่ม line หรือ submit
> ซ้ำอีก
```
id           bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
doc_no       varchar       NOT NULL
doc_type     varchar       NOT NULL  — 'ISSUE' | 'TRANSFER'
doc_date     date          nullable
project_code varchar(20)   NOT NULL  — FK → project.project_code
job_code     varchar       NOT NULL
requested_by bigint        NOT NULL  — FK → users.id
remarks      text          nullable
status       varchar       NOT NULL  DEFAULT 'DRAFT'  — 'DRAFT' | 'POSTED'
created_by   bigint        nullable
created_at   timestamp     NOT NULL  DEFAULT now()
updated_at   timestamp     NOT NULL  DEFAULT now()
```

---

### ic_project_movement_line
> 🆕 2026-09-24 — รายการใน ic_project_movement 1 แถว = 1 mat_code ที่จะตัด/โอน จาก
> (`movement.project_code`, `mat_code`, `cost_subgroup_id`) ไปยัง (`to_project_code`, `mat_code`,
> `to_cost_subgroup_id`) — ตอน add line จะ validate `qty <= qty_on_hand` ของต้นทางแล้ว (แต่ submit
> จะ re-validate อีกรอบ เผื่อยอดเปลี่ยนไปหลัง add) ยังไม่หัก `qty_on_hand` จริงตอน insert แถวนี้
> — หักจริงตอน submit เอกสารทั้งใบเท่านั้น (ดู [ic_project_movement](#ic_project_movement))
> แถวปลายทางใน `ic_project_cost_item` ต้องมีอยู่แล้ว (ไม่ auto-create) `line_no` running ต่อ `movement_id`
```
id                  bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
movement_id         bigint        NOT NULL  — FK → ic_project_movement.id
line_no             int           NOT NULL
mat_code            varchar(30)   NOT NULL
cost_subgroup_id    bigint        nullable  — FK → cost_subgroup.id
qty                 numeric(18,4) NOT NULL  — > 0
to_project_code     varchar(20)   NOT NULL
to_cost_subgroup_id bigint        nullable  — FK → cost_subgroup.id
remarks             text          nullable
created_at          timestamp     NOT NULL  DEFAULT now()
created_by          bigint        nullable
```
> 🔴 2026-09-26 — `cost_subgroup_id`/`to_cost_subgroup_id` changed from `NOT NULL` to nullable to
> support linked-warehouse projects (see [project](#project) and CLAUDE.md "Session learnings
> (2026-09-26)"). `NULL` means "not applicable" — only when the source or destination project
> (respectively) is warehouse-linked (`project.linked_warehouse_code IS NOT NULL`), since
> `stock_item` has no cost subgroup concept.

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
> 🆕 2026-09-26 — added `warehouse_code`, see below.
```
id             integer     NOT NULL  PK
location_code  varchar(20) NOT NULL  UNIQUE
location_name  varchar(100)NOT NULL
location_type  varchar(20) NOT NULL
parent_id      integer     nullable  — self-ref
warehouse_code varchar(20) nullable  — FK → warehouse.warehouse_code, 🆕 2026-09-26
is_active      boolean     NOT NULL  DEFAULT true
created_at     timestamp   NOT NULL  DEFAULT now()
updated_at     timestamp   NOT NULL  DEFAULT now()
created_by     bigint      nullable
updated_by     bigint      nullable
```
`warehouse_code` lets code resolve "which warehouse does this location belong to" without
hardcoding. Mapped rows: `SAL→FAC-S`, `PRJ→FAC-P`, `HO→HO`, `OFF→HO`, `BO→BO`. `NULL` for
non-warehouse locations (`DEP, DEPB, DEPU, DEPT, HUT, SOL, UEN, SELF`). See CLAUDE.md "Session
learnings (2026-09-26)" for the linked-warehouse-projects feature this supports.

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
title        varchar(200)nullable  — legacy: ยกเลิกการใช้งาน (2026-09-30) UI ไม่เก็บแล้ว, memo ใหม่ = NULL, แถวเก่ายังมีค่าเดิม
project_code varchar(20) nullable
job_code     varchar(20) nullable  — 🆕 2026-10, no FK, no CHECK. **Applied on dev only (NOT on UAT).** Rule (code: `validateMemoJobCode`, memo.go):
                                    memo has a project → job_code must be one of that project's `job_codes`
                                    (required when the project has job_codes); no project → job_code must be empty. Read + written by memo.go; not verified with real requests.
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
>
> 🆕 2026-10 — `GET /menus` (`RoleMenuPermissionHandler.ListMenus`, tree) does **not** return `is_active`
> (`repository.MenuRow` has no such field); the effective-permission query filters `m.is_active = true`.
> ⚠️ **UNCONFIRMED menu rename**: a SQL that renamed `MENU_PR` to "Office Service System (OF)", moved the three petty cash
> children under it and deactivated `MENU_PETTY_CASH` was run at one point and then reverted; its final state was never
> confirmed. Do NOT treat it as done — verify with `SELECT id, parent_id, menu_code, menu_name, is_active FROM menus ...` first.

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

### payment_log
```
id           bigint        NOT NULL  PK
doc_type     varchar(30)   NOT NULL  — CHECK IN ('PO','WO')
doc_id       bigint        NOT NULL
doc_no       varchar(30)   NOT NULL
amount_paid  numeric(18,2) NOT NULL  — CHECK <> 0 (ติดลบได้ = reverse)
paid_date    date          NOT NULL  DEFAULT CURRENT_DATE
paid_by      bigint        NOT NULL  — FK → users.id
remark       text          nullable
reverses_id  bigint        nullable  — FK → payment_log.id (self-ref, ใช้ mark ว่า row นี้ reverse row ไหน)
created_at   timestamp     NOT NULL  DEFAULT now()
created_by   bigint        NOT NULL  — FK → users.id
```

---

### petty_cash_requisition
```
id              bigint        NOT NULL  PK
pc_no           varchar(30)   NOT NULL  UNIQUE
pc_date         date          NOT NULL  DEFAULT CURRENT_DATE
requested_by    bigint        NOT NULL  — FK → users.id
purpose         text          nullable
currency        varchar(10)   NOT NULL  DEFAULT 'THB'
total_amount    numeric(18,4) NOT NULL  DEFAULT 0
use_discount    boolean       NOT NULL  DEFAULT false
discount_type   varchar(10)   NOT NULL  DEFAULT 'pct'  — CHECK ('pct'|'amt')
discount_amount numeric(18,4) NOT NULL  DEFAULT 0
use_vat         boolean       NOT NULL  DEFAULT false
vat_amount      numeric(18,4) NOT NULL  DEFAULT 0
use_wht         boolean       NOT NULL  DEFAULT false
wht_amount      numeric(18,4) NOT NULL  DEFAULT 0
net_amount      numeric(18,4) NOT NULL  DEFAULT 0
status          varchar(20)   NOT NULL  DEFAULT 'DRAFT'
                — CHECK: DRAFT|PENDING_APPROVAL|APPROVED|REJECTED|CANCELLED
approver_id     bigint        nullable  — FK → users.id
remarks         text          nullable
created_at      timestamp     NOT NULL  DEFAULT now()
updated_at      timestamp     NOT NULL  DEFAULT now()
created_by      bigint        NOT NULL
updated_by      bigint        nullable
```

### petty_cash_requisition_line
```
id               bigint        NOT NULL  PK
pc_id            bigint        NOT NULL  — FK → petty_cash_requisition.id
line_no          integer       NOT NULL
mat_code         varchar(20)   NOT NULL  — FK → material_code.mat_code
cost_subgroup_id bigint        nullable  — FK → cost_subgroup.id
description      text          nullable
qty              numeric(18,4) NOT NULL
unit_price       numeric(18,4) NOT NULL
amount           numeric(18,4) nullable
discount         numeric(18,4) NOT NULL  DEFAULT 0
disc_type        varchar(10)   NOT NULL  DEFAULT 'pct'  — CHECK ('pct'|'amt')
line_discount    numeric(18,4) NOT NULL  DEFAULT 0
line_vat         numeric(18,4) NOT NULL  DEFAULT 0
line_wht         numeric(18,4) NOT NULL  DEFAULT 0
line_net         numeric(18,4) NOT NULL  DEFAULT 0
wht_rate         numeric(5,2)  nullable
remarks          text          nullable
project_code     varchar(20)   NOT NULL  — FK → project.project_code
unit             varchar(50)   nullable  — 🆕 2026-10-26 (migration 027): หน่วย พิมพ์เอง/เติมจาก master ตอนเลือกวัสดุ; แถวเก่า NULL → อ่านกลับเป็นหน่วยของวัสดุ (COALESCE)
```

### petty_cash_status_log
```
id          bigint      NOT NULL  PK
pc_id       bigint      NOT NULL  — FK → petty_cash_requisition.id
from_status varchar(30) nullable
to_status   varchar(30) NOT NULL
changed_by  bigint      NOT NULL  — FK → users.id
changed_at  timestamp   NOT NULL  DEFAULT now()
remarks     text        nullable
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

### po_number_counter
> 🔄 2026-10-01 — `po_no` (และ `pr_no` → `pr_number_counter`, `memo_no` → `memo_number_counter`, โครงสร้างเหมือนกัน) generate
> **ตอนบันทึกเท่านั้น** ภายใน transaction เดียวกับ INSERT header ผ่าน `nextMonthlyNumber` (`internal/handlers/doc_number.go`,
> UPSERT ... RETURNING `last_seq`, เวลา Asia/Bangkok) บันทึกไม่สำเร็จ = rollback counter ไม่เกิดช่องเลขใหม่ ไม่มี reserve ฝั่ง client แล้ว
> (endpoint `reserve-number` / `next-number` ของ PR, PO และ memo ถูกลบแล้วทั้งหมด) รูปแบบ: `PRYYYYMM-NNNN`, `PO-YYYYMM-NNNN`, `MEM-YYMM-NNNN`.
```
year_month varchar(6) NOT NULL  PK  — 'YYYYMM'
last_seq   integer    NOT NULL  DEFAULT 0
```

---

### por_number_counter
> 🆕 2026-09-21 — pattern เดียวกับ `po_number_counter` generate `receive_no` ของ IC module รูปแบบ
> `IC-YYYYMM-NNNN` consume ตอน submit line-item รอบแรกที่สำเร็จของ IC PO Receive แล้วเก็บผลไว้ที่
> `ic_po_receive_document.receive_no` ดู [ic_po_receive_document](#ic_po_receive_document)
```
year_month varchar(6) NOT NULL  PK  — 'YYYYMM'
last_seq   integer    NOT NULL  DEFAULT 0
```

---

### po_status_log / pr_status_log
```
id, [po_id|pr_id], from_status, to_status (NOT NULL), changed_by, changed_at, remarks
```

---

### project
> 🆕 2026-09-26 — added `linked_warehouse_code`, see below. An earlier, now-abandoned attempt
> added `project_type` (`WAREHOUSE`/`PROJECT` enum) instead — that column was **dropped** and all
> code referencing it removed. Do not resurrect it; `linked_warehouse_code` is the only field that
> matters for this behavior.
```
id                    integer     NOT NULL  PK
project_code          varchar(20) NOT NULL  UNIQUE
project_name          varchar(200)NOT NULL
location_code         varchar(500)nullable  — 🔴 corrected 2026-10: was documented as varchar(20); live type is varchar(500) (free-text address, no FK)
start_date            date        nullable
end_date              date        nullable
status                varchar(20) NOT NULL  DEFAULT 'ACTIVE'
is_active             boolean     NOT NULL  DEFAULT true
owner_id              bigint      nullable  — FK → users.id, legacy "เจ้าของโครงการ" dropdown, ยัง accept ได้แต่ไม่ required/ไม่ขับเคลื่อนอะไรแล้ว
customer_id           bigint      nullable  — FK → customer.cus_id, dropdown "เจ้าของโครงการ" ตัวใหม่, ใช้คู่กับ owner_id ได้อิสระต่อกัน
budget_amount         numeric(18,4) NOT NULL DEFAULT 0
consultant_name       varchar(200)nullable
linked_warehouse_code varchar(20) nullable  — FK → warehouse.warehouse_code, 🆕 2026-09-26
project_kind          varchar     NOT NULL? — 'NEW' | 'VO' (CHECK). Read/written by project_handler.go; nullability/default not verified here.
job_codes             varchar[]   nullable  — array of job codes (validated against JobTypes by validateJobCodes)
vo_amount             numeric     — "มูลค่า VO" (separate from budget_amount); NEW needs budget_amount (vo_amount saved 0), VO needs vo_amount (budget saved 0). Exact type/default not verified here.
contract_amount, cost_amount — reported as existing project columns but NOT referenced by any Go code in this repo; columns NOT verified.
(other owner/customer-text columns used by the code and not listed above: project_owner_name, responsible_person_name,
 consultant_phone, dept_code — types not verified here)
created_at            timestamp   NOT NULL  DEFAULT now()
updated_at            timestamp   NOT NULL  DEFAULT now()
created_by            bigint      nullable
updated_by            bigint      nullable
```
> 🆕 2026-10 — `project_code` format `{year}-{customer_code}-{3-digit running no}` (e.g. `2026-LMT-001`). The handler
> (`project_handler.go` Create) allocates the running number from **`project_code_year_counter`** (one number per Asia/Bangkok
> year, shared by all customers). The `project_code_counter (code_year, cus_id, last_no)` table is NOT referenced by any Go code
> in this repo (apparently superseded) — do not use it.
`linked_warehouse_code`: `NULL` for a normal cost-tracked project. Set to a real `warehouse_code`
(e.g. `'FAC-S'`) for a project that is actually a warehouse proxy (e.g. `2026-WH-002`) — lets
`ic_project_movement` TRANSFER move stock between `stock_item` and this project's
`ic_project_cost_item`. See CLAUDE.md "Session learnings (2026-09-26)" for the full branch logic.

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
supplier_id      integer       nullable  — FK → supplier.id (clean-break migration off
                                            supplier_code, 2026-08-24 — no backfill,
                                            pre-migration rows are NULL here)
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
order_type       varchar(30)   NOT NULL  DEFAULT 'stock'  — CHECK ('stock'|'cost'|'asset_machine'|'asset_office_equipment'|'asset_tools'), เหมือน purchase_request.order_type
job_code         varchar(20)   NOT NULL  — required เสมอ, auto-fill จาก pr.job_code ถ้าไม่ส่งมาและมี pr_id
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
pr_line_id   bigint        nullable  — FK → purchase_request_line.id (purchase_order_line_pr_line_id_fkey)
                 — 🔴 this FK is what blocks clearing/deleting purchase_request_line rows once a
                   non-CANCELLED PO line references them; confirmed live 2026-09-28 with
                   PR202609-0041 ↔ PO-202609-0015 and PR202609-0029 ↔ PO-202609-0003/0004 — see
                   CLAUDE.md's "Session learnings (2026-09-28)" #2/#7
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
cost_subgroup_id bigint    nullable  — FK → cost_subgroup.id, Cost Code ต่อบรรทัด
                 — explicit value ชนะเสมอ, ถ้าไม่ส่งและมี pr_line_id จะ auto-fill จาก
                   purchase_request_line.cost_subgroup_id
description  text          nullable
remarks      text          nullable
```

---

### purchase_request
```
id             bigint      NOT NULL  PK
pr_no          varchar(30) NOT NULL
pr_date        date        NOT NULL  DEFAULT CURRENT_DATE
requested_by   bigint      NOT NULL  — FK → users.id (purchase_request_requested_by_fkey)
warehouse_code varchar(20) nullable
required_date  date        nullable
status         varchar(20) NOT NULL  DEFAULT 'DRAFT'
priority       varchar(20) DEFAULT 'NORMAL'  — LOW|NORMAL|HIGH|URGENT
order_type     varchar(30) NOT NULL  DEFAULT 'stock'  — CHECK IN ('stock','cost','asset_machine','asset_office_equipment','asset_tools') — 'stock' = ซื้อเข้าคลัง, 'cost' = ซื้อเข้าโครงการ (cost)
pr_type        varchar(10) NOT NULL  DEFAULT 'PO_WO'  — CHECK IN ('PO_WO','PO_ONLY','WO_ONLY') — เก็บไว้เฉยๆ ยังไม่มีโมดูล WO จริง ไม่มี logic ต่อ
job_code       varchar    nullable  — 🔴 added 2026-09-28, was missing from this doc entirely though it's a real, actively-queried column (pr.go/pr_approval.go); one of the 12 fixed job type codes (MP/ME/MS/MF/MG/MH/FS/FP/FB/DE/RE/G)
dept_code      varchar(20) nullable  — 🔴 added 2026-09-28, was missing from this doc entirely; NO FK enforced live (confirmed: empty dept_code round-trips with zero DB error)
remarks        text        nullable
project_code   varchar(20) nullable
memo_id        bigint      nullable  — FK → memo.id
location_text  varchar(200)nullable
created_at     timestamp   NOT NULL  DEFAULT now()
updated_at     timestamp   NOT NULL  DEFAULT now()
created_by     bigint      nullable
updated_by     bigint      nullable
```
⚠️ This entry was reconstructed handler-side (grep on `pr.go`/`pr_approval.go`), not re-verified
column-by-column against `information_schema` — treat `job_code`'s exact varchar length as
unconfirmed until checked live.

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
id, rfq_no, rfq_date, supplier_id (FK → supplier.id — clean-break migration off
supplier_code, 2026-08-24, nullable, no backfill), pr_id, status (DEFAULT 'SENT'),
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
> 🔴 2026-09-26 — confirmed **empty system-wide (0 rows)** as of this session. Not used by the
> linked-warehouse-projects feature (see [project](#project),
> CLAUDE.md "Session learnings (2026-09-26)") or any other live code path except the disabled
> Borrow/Return flow (`/stock/borrow/*` routes are unregistered — see the same session note). Do
> not build new features assuming this table is populated without checking `SELECT COUNT(*)` first.
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

### stock_item_image
```
id         bigint      NOT NULL  PK  (nextval stock_item_image_id_seq)
item_id    bigint      NOT NULL  — FK → stock_item.id
file_path  text        NOT NULL
file_name  varchar(255)nullable
is_primary boolean     NOT NULL  DEFAULT false
sort_order integer     NOT NULL  DEFAULT 0
created_at timestamp   NOT NULL  DEFAULT now()
created_by bigint      nullable
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
> 🔴 **ตรวจสอบ live DB (2026-09-09): ไม่พบ table `stock_transfer` / `stock_transfer_line` /
> `stock_transfer_status_log` ใน `information_schema.tables` ของ instance ที่เชื่อมต่ออยู่** แม้ว่า
> CLAUDE.md session note (2026-08-16) จะบรรยายฟีเจอร์นี้ว่า "มีตารางจริงแล้ว" และ handler
> `internal/handlers/stock_transfer.go` ในโค้ดก็ยังอ้างถึงตารางเหล่านี้อยู่ — **ยังไม่ยืนยันสาเหตุ**
> (อาจเป็น dev DB คนละตัวกับที่ session นั้น apply migration, หรือ table ถูก DROP ภายหลัง) ก่อนแก้/รัน
> โค้ดที่พึ่งพา 3 table นี้ ต้องเช็ค `information_schema.tables` ของ instance ที่จะ deploy จริงก่อน
> เสมอ — schema ด้านล่างคงไว้ตามที่เคยบันทึกไว้เป็น reference เผื่อยังใช้ได้กับ instance อื่น
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
> 🔴 **2026-08-24 — `supplier_code` dropped entirely (clean-break migration, no backward
> compat).** `supplier.id` (the existing integer PK) is now the only identifier. Every
> table that used to reference suppliers via `supplier_code` (`purchase_order`, `grn`,
> `rfq`) was switched to a nullable `supplier_id` FK → `supplier.id` in the same migration
> — no backfill, so pre-migration rows have `supplier_id = NULL`. `work_order` is the one
> exception: it never had a real FK to `supplier` (its `supplier_code`/`supplier_name`/
> `supplier_address`/`supplier_phone` are free text, `supplier_name` still `NOT NULL`) and
> keeps that shape unchanged — it only gained an additional, decoupled, nullable
> `supplier_id` column for optional reporting linkage, not a hard requirement.
```
id                   integer     NOT NULL  PK
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
supplier_code            varchar(20)   nullable  — free text, NO FK to supplier (never had one,
                                                    confirmed live 2026-08-24). Untouched by the
                                                    supplier_code→supplier_id migration below.
supplier_id              integer       nullable  — 🆕 2026-08-24, FK → supplier.id, optional/
                                                    decoupled — purely additive for reporting,
                                                    not authoritative; supplier_code/name/address/
                                                    phone stay the source of truth for this doc.
supplier_name            varchar(200)  NOT NULL
contact_person           varchar(100)  nullable
supplier_address         text          nullable
supplier_phone           varchar(50)   nullable
contract_type            varchar(20)   NOT NULL  DEFAULT 'LABOR_MATERIAL'
                         — 🆕 2026-10 (dev only): chk_wo_contract_type now allows LABOR_MATERIAL | LABOR_ONLY | RENTAL | SERVICE
work_system              varchar(5)    NOT NULL  — P | E | S   — 🔴 DIFFERS BY ENVIRONMENT:
                         **dev**: text[] ARRAY, chk_wo_work_system allows an array within P,E,S,MP,ME,MS,MF,MG,MH (applied on dev only).
                         **UAT**: still a single varchar with CHECK (P, E or S) — UAT needs a SEPARATE migration design before
                         the new code can run there. The line above is the old/UAT shape.
contract_description     text          nullable  — 🔴 corrected 2026-10: documented as varchar(30) but is text (handler caps at 2000 chars). "dropdown code" note is old.
project_name_text        varchar(200)  nullable  — 🆕 2026-10 (dev only). Exactly one of project_code / project_name_text is set (handler rule).
                         project_code is varchar(20) nullable with an FK → project.project_code.
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
has_penalty              boolean       NOT NULL  DEFAULT false  -- 🆕 2026-10-25: 8.1 เบี้ยปรับ มี/ไม่มี (อัตราคงที่ 1.00% ของมูลค่างาน/วัน แสดงเป็นข้อความ ไม่ได้เก็บ)
warranty_note            text          nullable                 -- 🆕 2026-10-25: 8.2 ประกันผลงาน 1 ปี (free text); แทน warranty_years ในฟอร์ม (คอลัมน์เดิมยังอยู่)
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
>
> 🔴 **2026-09-21 — `advance_pct`, `advance_amount`, `retention_pct`, `penalty_pct_per_day`,
> `progress_payment_note` deprecated สำหรับข้อมูลใหม่แล้ว** หน้า create/edit WO ไม่อ่าน/เขียน 5
> คอลัมน์นี้อีกต่อไป แทนที่ด้วย 3 ตารางใหม่ [work_order_payment_installment](#work_order_payment_installment),
> [work_order_retention](#work_order_retention), [work_order_penalty](#work_order_penalty) — ยังไม่ลบ
> คอลัมน์เดิมออกจาก DB เพราะ `WorkOrderPrintStandard.tsx`/`WorkOrderDetailPage.tsx` (frontend) ยังอ่าน
> ค่าเดิมอยู่ — WO เก่าจะยังเห็นค่าเดิมได้ปกติ แต่ WO ที่ถูกแก้ไขหลังจุดนี้ 5 คอลัมน์นี้จะเริ่ม stale
> (ไม่มีใครเขียนอัปเดตอีก) ยังไม่ได้แก้ไข รอ task แยก

---

### work_order_cost_code
> 🔴 **deprecated** (ดู Important Notes ด้านบน) — multi-select cost_code แบบแรกก่อนเปลี่ยนเป็น
> `work_order_line` เต็มรูปแบบ ยังอยู่ใน DB จริง แต่โค้ดปัจจุบันไม่เขียน/อ่านตารางนี้แล้ว
```
id         bigint      NOT NULL  PK
wo_id      bigint      NOT NULL  — FK → work_order.id
cost_code  varchar(50) NOT NULL
remarks    varchar(255)nullable
created_at timestamp   NOT NULL  DEFAULT now()
created_by bigint      NOT NULL
-- UNIQUE (wo_id, cost_code)
```

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

### work_order_payment_installment
> 🆕 2026-09-21 — งวดชำระเงินของ WO แทนที่ `advance_pct`/`advance_amount` (ดูหมายเหตุ deprecated
> ใน [work_order](#work_order)) หลายแถวต่อ 1 WO, save แบบ full-replace (`DELETE ... WHERE NOT (id =
> ANY(keepIDs))` ตามด้วย upsert ทีละแถว) ทุกครั้งที่บันทึกเงื่อนไขการชำระ
```
id                  bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
wo_id               bigint        NOT NULL  — FK → work_order.id
installment_no      integer       nullable
description         text          nullable
percent_of_contract numeric(5,2)  nullable
amount              numeric(18,2) nullable
due_date            date          nullable
payment_status      varchar(20)   NOT NULL  DEFAULT 'UNPAID'  — UNPAID|PAID
paid_date           date          nullable  — auto-set = today ถ้า payment_status='PAID' และไม่ส่งมา
remarks             text          nullable
created_at          timestamp     NOT NULL  DEFAULT now()
updated_at          timestamp     NOT NULL  DEFAULT now()
created_by          bigint        nullable
updated_by          bigint        nullable
```

---

### work_order_retention
> 🆕 2026-09-21 — เงินประกันผลงานของ WO แทนที่ `retention_pct` (ดูหมายเหตุ deprecated ใน
> [work_order](#work_order)) save แบบ full-replace เหมือน work_order_payment_installment
```
id                  bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
wo_id               bigint        NOT NULL  — FK → work_order.id
description         text          nullable
percent_of_contract numeric(5,2)  nullable
amount              numeric(18,2) nullable
remarks             text          nullable
created_at          timestamp     NOT NULL  DEFAULT now()
updated_at          timestamp     NOT NULL  DEFAULT now()
created_by          bigint        nullable
updated_by          bigint        nullable
```

---

### work_order_penalty
> 🆕 2026-09-21 — ค่าปรับของ WO แทนที่ `penalty_pct_per_day` (ดูหมายเหตุ deprecated ใน
> [work_order](#work_order)) save แบบ full-replace เหมือน 2 ตารางข้างบน
```
id                   bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
wo_id                bigint        NOT NULL  — FK → work_order.id
description          text          nullable
percent_per_day      numeric(5,2)  nullable
contract_start_date  date          nullable
contract_end_date    date          nullable
remarks              text          nullable
created_at           timestamp     NOT NULL  DEFAULT now()
updated_at           timestamp     NOT NULL  DEFAULT now()
created_by           bigint        nullable
updated_by           bigint        nullable
```

---

### wo_contractor_log
> 🆕 2026-10-25 (migration `020_wo_contractor_log.sql`) — หน้า "บันทึกงานของผู้รับเหมาตามใบสั่งจ้าง" tab ข้อมูลเอกสาร.
> ผู้รับจ้าง / โครงการของ WO / ลักษณะสัญญา **ไม่เก็บซ้ำ** อ่านสดจาก `work_order` (+ `supplier`) ทุกครั้ง
```
id            bigint       NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
wo_id         bigint       NOT NULL  — FK → work_order.id
project_code  varchar(20)  nullable  — FK → project.project_code  (ช่อง "PJ" แยกจากโครงการของ WO)
created_at / updated_at timestamp NOT NULL DEFAULT now(); created_by / updated_by bigint nullable
```

### wo_contractor_log_term
```
id          bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
log_id      bigint        NOT NULL  — FK → wo_contractor_log.id  ON DELETE CASCADE
item_type   varchar(20)   NOT NULL  — CHECK IN ('CONTRACT','ADVANCE','RETENTION');  UNIQUE (log_id, item_type)
percent     numeric(7,2)  nullable
amount      numeric(18,2) nullable
accumulate  numeric(18,2) nullable   — กรอกเอง (ยังไม่ได้คำนวณจากบิล)
balance     numeric(18,2) nullable   — กรอกเอง
created_at / updated_at timestamp NOT NULL DEFAULT now()
```

### wo_contractor_log_bill
> 🆕 2026-10-25 (migration `021_wo_contractor_log_bill.sql`) — tab ประเภทรับวางบิล. `subtotal` (รวมเป็นเงิน (1)) และ
> `net_amount` (จ่ายสุทธิ) **คำนวณใหม่ฝั่ง backend ทุกครั้งที่บันทึก** จากตาราง `_line`:
> subtotal = ADV + PROGRESS − ADV_DEDUCT + VAT ; net = subtotal − RETENTION − WHT − OTHER
```
id                      bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
log_id                  bigint        NOT NULL  — FK → wo_contractor_log.id  ON DELETE CASCADE
bill_type               varchar(20)   NOT NULL  — CHECK IN ('ADV','PROGRESS','RETENTION')
installment_no          integer       nullable  — "งวดที่"
ap_voucher_no           varchar(50)   nullable  — ฝ่ายบัญชีกรอก (ยังไม่ได้จำกัดสิทธิ์)
pay_to                  varchar(200)  nullable  — default = ชื่อผู้รับจ้างของ WO, แก้ได้
credit_term_days        integer       nullable  — default จาก supplier.payment_terms (เลขตัวแรกในข้อความ เช่น "30  วัน")
description             text          nullable
progress_installment_no integer       nullable  — งวดงานของ WO (work_order_payment_installment.installment_no) ที่เลือกใน "เงินงวดนี้"
wht_type                varchar(50)   nullable  — ค่าจ้างรับเหมา / ค่าบริการ / ค่าเช่า / ค่าขนส่ง (ข้อความอิสระ)
subtotal                numeric(18,2) NOT NULL  DEFAULT 0
net_amount              numeric(18,2) NOT NULL  DEFAULT 0
status                  varchar(20)   NOT NULL  DEFAULT 'DRAFT'  — CHECK IN ('DRAFT','SAVED')
created_at / updated_at timestamp NOT NULL DEFAULT now(); created_by / updated_by bigint nullable
```

### wo_contractor_log_bill_line
```
id         bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
bill_id    bigint        NOT NULL  — FK → wo_contractor_log_bill.id  ON DELETE CASCADE
line_code  varchar(20)   NOT NULL  — CHECK IN ('ADV','PROGRESS','ADV_DEDUCT','VAT','RETENTION','WHT','OTHER');  UNIQUE (bill_id, line_code)
percent    numeric(7,2)  nullable
amount     numeric(18,2) NOT NULL  DEFAULT 0
```
บรรทัดตามสเก็ตช์: 1 ADV=เงินเบิกล่วงหน้า, 2 PROGRESS=เงินงวดนี้, 3 ADV_DEDUCT=หักเงินจ่ายล่วงหน้า, 4 VAT, 5 RETENTION=หักเงินประกันผลงาน,
6 WHT=หัก ณ ที่จ่าย, 7 OTHER=หักอื่นๆ

### wo_contractor_log_bill_invoice
```
id             bigint       NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
bill_id        bigint       NOT NULL  — FK → wo_contractor_log_bill.id  ON DELETE CASCADE
line_no        integer      NOT NULL
invoice_no     varchar(50)  nullable
invoice_date   date         nullable
delivery_date  date         nullable   -- ป้าย "วันที่ส่งมอบงาน" (ลายมือในสเก็ตช์อ่านยาก — ยืนยันชื่อคอลัมน์อีกครั้ง)
bill_date      date         nullable   -- วันที่วางบิล
pay_date       date         nullable   -- วันที่จ่าย
```

---

### work_order menus
```
MENU_WO           — หนังสือสั่งจ้าง (WO)     sort_order=4  (หลัง PO=3, ก่อน STOCK=5)
MENU_WO_LIST      — รายการหนังสือสั่งจ้าง      sort_order=1 (child)
MENU_WO_CREATE    — สร้างหนังสือสั่งจ้าง       sort_order=2 (child)
MENU_WO_APPROVAL  — อนุมัติหนังสือสั่งจ้าง      sort_order=3 (child)
```
ชื่อเมนู parent ปัจจุบัน: **"หนังสือสั่งจ้าง (WO)"** (เปลี่ยนจาก "(Work Order)" ตามที่ขอทีหลัง)

> 🆕 2026-10-26 — `MENU_WO_CONTRACTOR_LOG` (เพิ่มโดย 018) **ถูกลบแล้วด้วย `026_remove_menu_contractor_log.sql`**: บันทึกงานผู้รับเหมาเปิดจากหน้าการเงิน > ติดตามการจ่ายเงิน > WO แทนการมีเมนูของตัวเอง
> (ข้อความ 2026-10-25 ด้านบนที่ว่าเพิ่มเมนูนี้ถือว่าล้าสมัย). `MENU_PO_COST_BUDGET` กลับเป็น sort_order 9.

> 🆕 2026-10-25 — สถานะจริงใน DB (dev): `MENU_WO` (id 51) **is_active=false** และไม่มีลูกเหลือ; เมนูย่อย WO (`MENU_WO_CREATE`/`MENU_WO_LIST`/`MENU_WO_APPROVAL`)
> อยู่ใต้ `MENU_PO` ("PO/WO Procurement ระบบจัดซื้อจัดจ้าง", id 5) แล้ว. เพิ่ม `MENU_WO_CONTRACTOR_LOG` (id 71,
> "บันทึกงานของผู้รับเหมาตามใบสั่งจ้าง", path `/work-order/contractor-log`) ใต้ `MENU_PO`, sort_order=9 (ก่อน `MENU_PO_COST_BUDGET`=10)
> — migrations `018_menu_contractor_work_log.sql` + `019_menu_contractor_log_reorder.sql`. สิทธิ์ของเมนูใหม่ต้องให้ผ่าน permission matrix เอง (ไม่มี grant ใน SQL).

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
---

### 🆕 2026-10-01 — ic_project_receipt_pool (cost receive target) and the cost return rule
- `ic_project_receipt_pool` (`project_code`, `mat_code`, `cost_subgroup_id`, `qty_received`, `qty_issued`, `last_unit_cost`, …):
  where an **order_type='cost' PO receive** posts. Available = `qty_received - qty_issued`; CHECK `qty_issued <= qty_received`.
  Not `ic_project_cost_item` — that is credited later by movement ISSUE/TRANSFER (or PR submit).
- `ic_project_receipt_pool_transaction.txn_type` CHECK: `'RECEIVE' | 'ISSUE_OUT' | 'RETURN'` (RETURN = supplier return, negative qty,
  `ref_type='PO'`, `po_id`/`po_line_id` set). No qty_before/qty_after columns.
- Cost return deducts from the pool's available balance first (lowers `qty_received`), then the remainder from
  `ic_project_cost_item.qty_on_hand` (logged in `ic_project_cost_item_transaction` with `ref_type='PO'`).

---

### 🆕 2026-10 — tables missing from the index above (columns verified only where stated)
> All schema changes in this project are applied by hand in pgAdmin — no new files under `migrations/` (standing policy).
> Which environment has each table is not verified unless stated.

- **project_job_share** — **applied on dev only (NOT on UAT)**. Columns: `project_id` (FK → project(id) ON DELETE CASCADE),
  `job_code varchar(10)`, `share_percent numeric(5,2)` CHECK `>0 AND <=100`, `created_at`, `created_by`; PK `(project_id, job_code)`.
  Deferred constraint trigger `trg_project_job_share_total` rejects a total above 100 per project. Used by
  `internal/handlers/project_job_share.go` (replace-on-save: DELETE then INSERT in the project tx). `share_amount` is derived, never stored.
- **project_responsible_person** — PK `(project_id, user_id)`, FK ON DELETE CASCADE; code also writes `created_by`
  (`project_handler.go`). Other columns not verified.
- **project_code_year_counter** — `(code_year, last_no)`; used for project_code (see `project` above).
- **project_code_counter** — `(code_year, cus_id, last_no)`; not referenced by Go code in this repo. Columns not verified.
- **doc_number_counter** — `(prefix, year_month, last_seq)`, PK/unique on `(prefix, year_month)`; used by `doc_number.go` for IS-/TF- movement numbers.
- **pc_number_counter**, **ret_number_counter** — monthly counters `(year_month, last_seq)` via `nextMonthlyNumber`
  (petty cash number; PO return `RT-YYYYMM-NNNN`). Other columns not verified.
- **ic_po_return_document** — `po_id, return_no, return_date, remarks, created_by` (+ `id`; others not verified) — written by `ic.go` return-lines.
- **ic_po_return_line** — `return_id, line_no, po_line_id, mat_code, cost_subgroup_id, return_qty, remarks` (+ `id`; others not verified).
- **payment_installment** — generic installments: `doc_type, doc_id, installment_no, percent, amount, description, is_paid, paid_at, paid_by, created_by, updated_by`
  (+ `id`; others not verified) — `finance_installments.go`, `finance.go`.
- **petty_cash_attachment** — `pc_id, file_name, file_path, file_size, file_type, uploaded_by` (+ `id`; others not verified).
- **wht_income_type** — `id, code, label_th, rate, is_active, sort_order` as read by `petty_cash.go`; other columns not verified.

---

### 🆕 2026-10-19 — project_cost_budget (per-project, per-cost-code budget)
> **Applied on dev only (NOT on UAT)** — by hand in pgAdmin, no migration file (project policy). Columns confirmed via
> `information_schema.columns` on the dev DB (`current_database()` = `postgres`) on 2026-10-19; table had 0 rows when verified.
```
id                bigint        NOT NULL  PK  GENERATED ALWAYS AS IDENTITY
project_id        integer       NOT NULL  FK → project(id) ON DELETE CASCADE
cost_subgroup_id  integer       NOT NULL  FK → cost_subgroup(id)   -- integer, same as cost_subgroup.id
budget_amount     numeric(18,2) NOT NULL  DEFAULT 0   CHECK (budget_amount >= 0)
created_at        timestamp     NOT NULL  DEFAULT now()
updated_at        timestamp     NOT NULL  DEFAULT now()   -- not auto-maintained; the handler must set it
created_by        bigint        nullable
updated_by        bigint        nullable
UNIQUE (project_id, cost_subgroup_id)
index on project_id
```
- **Trigger `trg_project_cost_budget_item_only`** (BEFORE INSERT OR UPDATE OF cost_subgroup_id, function
  `trg_project_cost_budget_item_only()`): rejects a `cost_subgroup_id` whose `is_head` OR `is_major` is true. Only ITEM subgroups
  (`NOT is_head AND NOT is_major`) may hold budget; Head / Major budgets are never stored, they are rolled up in the backend.
- The trigger fires only on writes to this table. The Head / Major toggles guard the other direction in code (`rejectIfHasBudget`,
  `cost_code_handler.go`) — nothing at DB level stops flipping a subgroup that already has a budget row.
- Read by `CostBudgetDetail` (`po_cost_budget_detail.go`) via LEFT JOIN. **No endpoint writes it yet.**
- Constraint names, trigger function body and index name were taken from the DDL proposed in the session, not re-read from `pg_constraint`/`pg_trigger`.

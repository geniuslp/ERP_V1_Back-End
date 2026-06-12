-- =============================================================================
-- ERP v2 — Full Master DDL Migration
-- Run this after the base Erp_v1 schema has been applied
-- PostgreSQL 14+
-- =============================================================================

BEGIN;

-- ─── Users extension ──────────────────────────────────────────────────────────
ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS location_code VARCHAR(20),
    ADD COLUMN IF NOT EXISTS employee_code VARCHAR(50),
    ADD COLUMN IF NOT EXISTS department    VARCHAR(200);

-- ─── Location ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.location (
    location_code   VARCHAR(20)  PRIMARY KEY,
    location_name   VARCHAR(200) NOT NULL,
    location_type   VARCHAR(50)  NOT NULL CHECK (location_type IN ('department','project','site')),
    parent_code     VARCHAR(20)  REFERENCES public.location(location_code),
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- ─── Warehouse ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.warehouse (
    warehouse_code  VARCHAR(20)  PRIMARY KEY,
    warehouse_name  VARCHAR(200) NOT NULL,
    address         TEXT,
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.storage_zone (
    zone_id         BIGSERIAL    PRIMARY KEY,
    warehouse_code  VARCHAR(20)  NOT NULL REFERENCES public.warehouse(warehouse_code),
    zone_code       VARCHAR(30)  NOT NULL,
    zone_name       VARCHAR(100) NOT NULL,
    zone_type       VARCHAR(50)  CHECK (zone_type IN ('rack','shelf','bin','area')),
    UNIQUE (warehouse_code, zone_code)
);

-- ─── Supplier ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.supplier (
    supplier_code   VARCHAR(20)  PRIMARY KEY,
    supplier_name   VARCHAR(200) NOT NULL,
    tax_id          VARCHAR(50),
    address         TEXT,
    contact_name    VARCHAR(200),
    contact_phone   VARCHAR(50),
    contact_email   VARCHAR(200),
    payment_terms   VARCHAR(100),
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- ─── Inventory ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.inventory (
    inventory_id    BIGSERIAL    PRIMARY KEY,
    mat_code        VARCHAR(20)  NOT NULL REFERENCES public.material_code(mat_code),
    warehouse_code  VARCHAR(20)  NOT NULL REFERENCES public.warehouse(warehouse_code),
    zone_id         BIGINT       REFERENCES public.storage_zone(zone_id),
    qty_on_hand     NUMERIC(18,4) NOT NULL DEFAULT 0,
    qty_reserved    NUMERIC(18,4) NOT NULL DEFAULT 0,
    qty_on_order    NUMERIC(18,4) NOT NULL DEFAULT 0,
    reorder_point   NUMERIC(18,4),
    reorder_qty     NUMERIC(18,4),
    min_stock       NUMERIC(18,4),
    max_stock       NUMERIC(18,4),
    last_counted_at TIMESTAMP,
    updated_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    UNIQUE (mat_code, warehouse_code)
);

CREATE TABLE IF NOT EXISTS public.inventory_transaction (
    txn_id          BIGSERIAL    PRIMARY KEY,
    txn_no          VARCHAR(30)  NOT NULL UNIQUE,
    txn_type        VARCHAR(30)  NOT NULL CHECK (txn_type IN (
                        'ISSUE','RETURN','TRANSFER_OUT','TRANSFER_IN',
                        'ADJUST_PLUS','ADJUST_MINUS','GRN_IN','BORROW_OUT','BORROW_RETURN')),
    mat_code        VARCHAR(20)  NOT NULL REFERENCES public.material_code(mat_code),
    from_warehouse  VARCHAR(20)  REFERENCES public.warehouse(warehouse_code),
    to_warehouse    VARCHAR(20)  REFERENCES public.warehouse(warehouse_code),
    from_zone_id    BIGINT       REFERENCES public.storage_zone(zone_id),
    to_zone_id      BIGINT       REFERENCES public.storage_zone(zone_id),
    qty             NUMERIC(18,4) NOT NULL CHECK (qty > 0),
    ref_doc_type    VARCHAR(30),
    ref_doc_no      VARCHAR(30),
    location_code   VARCHAR(20)  REFERENCES public.location(location_code),
    reason          TEXT,
    txn_date        DATE         NOT NULL DEFAULT CURRENT_DATE,
    created_by      BIGINT       NOT NULL REFERENCES public.users(id),
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.stock_count (
    count_id        BIGSERIAL    PRIMARY KEY,
    count_no        VARCHAR(30)  NOT NULL UNIQUE,
    warehouse_code  VARCHAR(20)  NOT NULL REFERENCES public.warehouse(warehouse_code),
    count_date      DATE         NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','IN_PROGRESS','COMPLETED')),
    remarks         TEXT,
    created_by      BIGINT       NOT NULL REFERENCES public.users(id),
    approved_by     BIGINT       REFERENCES public.users(id),
    approved_at     TIMESTAMP,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.stock_count_line (
    line_id         BIGSERIAL    PRIMARY KEY,
    count_id        BIGINT       NOT NULL REFERENCES public.stock_count(count_id) ON DELETE CASCADE,
    mat_code        VARCHAR(20)  NOT NULL REFERENCES public.material_code(mat_code),
    zone_id         BIGINT       REFERENCES public.storage_zone(zone_id),
    qty_system      NUMERIC(18,4),
    qty_counted     NUMERIC(18,4),
    qty_diff        NUMERIC(18,4) GENERATED ALWAYS AS (qty_counted - qty_system) STORED,
    remarks         TEXT
);

-- ─── Borrow / Return ──────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.borrow (
    borrow_id       BIGSERIAL    PRIMARY KEY,
    borrow_no       VARCHAR(30)  NOT NULL UNIQUE,
    borrow_type     VARCHAR(20)  NOT NULL DEFAULT 'BORROW' CHECK (borrow_type IN ('BORROW','RETURN')),
    warehouse_code  VARCHAR(20)  NOT NULL REFERENCES public.warehouse(warehouse_code),
    borrower_id     BIGINT       NOT NULL REFERENCES public.users(id),
    location_code   VARCHAR(20)  REFERENCES public.location(location_code),
    borrow_date     DATE         NOT NULL DEFAULT CURRENT_DATE,
    expected_return DATE,
    actual_return   DATE,
    status          VARCHAR(20)  NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','RETURNED','PARTIAL')),
    remarks         TEXT,
    created_by      BIGINT       NOT NULL REFERENCES public.users(id),
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.borrow_line (
    line_id         BIGSERIAL    PRIMARY KEY,
    borrow_id       BIGINT       NOT NULL REFERENCES public.borrow(borrow_id) ON DELETE CASCADE,
    mat_code        VARCHAR(20)  NOT NULL REFERENCES public.material_code(mat_code),
    mat_type        VARCHAR(20)  NOT NULL DEFAULT 'RETURNABLE' CHECK (mat_type IN ('RETURNABLE','CONSUMABLE')),
    qty_borrowed    NUMERIC(18,4) NOT NULL CHECK (qty_borrowed > 0),
    qty_returned    NUMERIC(18,4) NOT NULL DEFAULT 0,
    condition_out   VARCHAR(50)  CHECK (condition_out IN ('GOOD','FAIR','DAMAGED')),
    condition_in    VARCHAR(50)  CHECK (condition_in IN ('GOOD','FAIR','DAMAGED')),
    remarks         TEXT
);

-- ─── Purchase Request ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.purchase_request (
    pr_id           BIGSERIAL    PRIMARY KEY,
    pr_no           VARCHAR(30)  NOT NULL UNIQUE,
    pr_date         DATE         NOT NULL DEFAULT CURRENT_DATE,
    requested_by    BIGINT       NOT NULL REFERENCES public.users(id),
    location_code   VARCHAR(20)  NOT NULL REFERENCES public.location(location_code),
    warehouse_code  VARCHAR(20)  REFERENCES public.warehouse(warehouse_code),
    required_date   DATE,
    status          VARCHAR(30)  NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
                        'DRAFT','PENDING_APPROVAL','APPROVED','REJECTED',
                        'STOCK_CHECK','PARTIALLY_FILLED','FULFILLED','CANCELLED')),
    priority        VARCHAR(20)  DEFAULT 'NORMAL' CHECK (priority IN ('LOW','NORMAL','HIGH','URGENT')),
    remarks         TEXT,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.purchase_request_line (
    line_id         BIGSERIAL    PRIMARY KEY,
    pr_id           BIGINT       NOT NULL REFERENCES public.purchase_request(pr_id) ON DELETE CASCADE,
    line_no         INTEGER      NOT NULL,
    mat_code        VARCHAR(20)  NOT NULL REFERENCES public.material_code(mat_code),
    qty_requested   NUMERIC(18,4) NOT NULL CHECK (qty_requested > 0),
    qty_reserved    NUMERIC(18,4) NOT NULL DEFAULT 0,
    qty_to_order    NUMERIC(18,4) NOT NULL DEFAULT 0,
    remarks         TEXT,
    status          VARCHAR(30)  NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','RESERVED','ORDERED','FULFILLED','CANCELLED')),
    UNIQUE (pr_id, line_no)
);

-- ─── RFQ ──────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.rfq (
    rfq_id          BIGSERIAL    PRIMARY KEY,
    rfq_no          VARCHAR(30)  NOT NULL UNIQUE,
    rfq_date        DATE         NOT NULL DEFAULT CURRENT_DATE,
    supplier_code   VARCHAR(20)  NOT NULL REFERENCES public.supplier(supplier_code),
    pr_id           BIGINT       REFERENCES public.purchase_request(pr_id),
    status          VARCHAR(20)  NOT NULL DEFAULT 'SENT' CHECK (status IN ('SENT','RECEIVED','SELECTED','REJECTED')),
    remarks         TEXT,
    created_by      BIGINT       NOT NULL REFERENCES public.users(id),
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.rfq_line (
    line_id         BIGSERIAL    PRIMARY KEY,
    rfq_id          BIGINT       NOT NULL REFERENCES public.rfq(rfq_id) ON DELETE CASCADE,
    line_no         INTEGER      NOT NULL,
    mat_code        VARCHAR(20)  NOT NULL REFERENCES public.material_code(mat_code),
    qty             NUMERIC(18,4) NOT NULL,
    unit_price      NUMERIC(18,4),
    currency        VARCHAR(10)  DEFAULT 'THB',
    lead_time_days  INTEGER,
    remarks         TEXT,
    UNIQUE (rfq_id, line_no)
);

-- ─── Purchase Order ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.purchase_order (
    po_id           BIGSERIAL    PRIMARY KEY,
    po_no           VARCHAR(30)  NOT NULL UNIQUE,
    po_date         DATE         NOT NULL DEFAULT CURRENT_DATE,
    supplier_code   VARCHAR(20)  NOT NULL REFERENCES public.supplier(supplier_code),
    pr_id           BIGINT       REFERENCES public.purchase_request(pr_id),
    rfq_id          BIGINT       REFERENCES public.rfq(rfq_id),
    warehouse_code  VARCHAR(20)  NOT NULL REFERENCES public.warehouse(warehouse_code),
    currency        VARCHAR(10)  NOT NULL DEFAULT 'THB',
    total_amount    NUMERIC(18,4) NOT NULL DEFAULT 0,
    vat_amount      NUMERIC(18,4) NOT NULL DEFAULT 0,
    net_amount      NUMERIC(18,4) NOT NULL DEFAULT 0,
    expected_date   DATE,
    status          VARCHAR(30)  NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
                        'DRAFT','PENDING_APPROVAL','APPROVED','REJECTED',
                        'SENT','PARTIALLY_RECEIVED','RECEIVED','CANCELLED')),
    payment_terms   VARCHAR(100),
    delivery_address TEXT,
    remarks         TEXT,
    created_by      BIGINT       NOT NULL REFERENCES public.users(id),
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.purchase_order_line (
    line_id         BIGSERIAL    PRIMARY KEY,
    po_id           BIGINT       NOT NULL REFERENCES public.purchase_order(po_id) ON DELETE CASCADE,
    line_no         INTEGER      NOT NULL,
    mat_code        VARCHAR(20)  NOT NULL REFERENCES public.material_code(mat_code),
    pr_line_id      BIGINT       REFERENCES public.purchase_request_line(line_id),
    qty_ordered     NUMERIC(18,4) NOT NULL CHECK (qty_ordered > 0),
    qty_received    NUMERIC(18,4) NOT NULL DEFAULT 0,
    unit_price      NUMERIC(18,4) NOT NULL CHECK (unit_price >= 0),
    amount          NUMERIC(18,4) GENERATED ALWAYS AS (qty_ordered * unit_price) STORED,
    remarks         TEXT,
    status          VARCHAR(20)  NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','PARTIAL','RECEIVED','CANCELLED')),
    UNIQUE (po_id, line_no)
);

-- ─── GRN ──────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.grn (
    grn_id          BIGSERIAL    PRIMARY KEY,
    grn_no          VARCHAR(30)  NOT NULL UNIQUE,
    grn_date        DATE         NOT NULL DEFAULT CURRENT_DATE,
    po_id           BIGINT       NOT NULL REFERENCES public.purchase_order(po_id),
    warehouse_code  VARCHAR(20)  NOT NULL REFERENCES public.warehouse(warehouse_code),
    supplier_code   VARCHAR(20)  NOT NULL REFERENCES public.supplier(supplier_code),
    delivery_note   VARCHAR(100),
    status          VARCHAR(20)  NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','CONFIRMED','POSTED')),
    quality_status  VARCHAR(20)  NOT NULL DEFAULT 'PENDING' CHECK (quality_status IN ('PENDING','PASSED','FAILED','PARTIAL')),
    received_by     BIGINT       NOT NULL REFERENCES public.users(id),
    confirmed_by    BIGINT       REFERENCES public.users(id),
    confirmed_at    TIMESTAMP,
    remarks         TEXT,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.grn_line (
    line_id         BIGSERIAL    PRIMARY KEY,
    grn_id          BIGINT       NOT NULL REFERENCES public.grn(grn_id) ON DELETE CASCADE,
    line_no         INTEGER      NOT NULL,
    po_line_id      BIGINT       NOT NULL REFERENCES public.purchase_order_line(line_id),
    mat_code        VARCHAR(20)  NOT NULL REFERENCES public.material_code(mat_code),
    zone_id         BIGINT       REFERENCES public.storage_zone(zone_id),
    qty_received    NUMERIC(18,4) NOT NULL CHECK (qty_received >= 0),
    qty_accepted    NUMERIC(18,4) NOT NULL DEFAULT 0,
    qty_rejected    NUMERIC(18,4) NOT NULL DEFAULT 0,
    quality_remarks TEXT,
    UNIQUE (grn_id, line_no)
);

-- ─── Approval ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.approval_config (
    config_id         BIGSERIAL    PRIMARY KEY,
    doc_type          VARCHAR(30)  NOT NULL,
    step_no           INTEGER      NOT NULL,
    step_name         VARCHAR(200) NOT NULL,
    approver_role_id  BIGINT       REFERENCES public.roles(id),
    min_amount        NUMERIC(18,4),
    max_amount        NUMERIC(18,4),
    is_active         BOOLEAN      NOT NULL DEFAULT TRUE,
    UNIQUE (doc_type, step_no)
);

CREATE TABLE IF NOT EXISTS public.approval_request (
    approval_id     BIGSERIAL    PRIMARY KEY,
    doc_type        VARCHAR(30)  NOT NULL,
    doc_id          BIGINT       NOT NULL,
    doc_no          VARCHAR(30)  NOT NULL,
    step_no         INTEGER      NOT NULL,
    requested_by    BIGINT       NOT NULL REFERENCES public.users(id),
    assigned_to     BIGINT       REFERENCES public.users(id),
    status          VARCHAR(20)  NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED','ESCALATED')),
    due_date        DATE,
    amount          NUMERIC(18,4),
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public.approval_log (
    log_id          BIGSERIAL    PRIMARY KEY,
    approval_id     BIGINT       NOT NULL REFERENCES public.approval_request(approval_id),
    doc_type        VARCHAR(30)  NOT NULL,
    doc_id          BIGINT       NOT NULL,
    doc_no          VARCHAR(30)  NOT NULL,
    step_no         INTEGER      NOT NULL,
    action          VARCHAR(20)  NOT NULL CHECK (action IN ('SUBMIT','APPROVE','REJECT','RETURN','CANCEL','ESCALATE','COMMENT')),
    action_by       BIGINT       NOT NULL REFERENCES public.users(id),
    action_at       TIMESTAMP    NOT NULL DEFAULT NOW(),
    comments        TEXT,
    old_status      VARCHAR(20),
    new_status      VARCHAR(20),
    ip_address      INET,
    user_agent      TEXT
);

CREATE TABLE IF NOT EXISTS public.erp_audit_log (
    audit_id        BIGSERIAL    PRIMARY KEY,
    table_name      VARCHAR(100) NOT NULL,
    record_id       BIGINT       NOT NULL,
    action          VARCHAR(10)  NOT NULL CHECK (action IN ('INSERT','UPDATE','DELETE')),
    changed_by      BIGINT       REFERENCES public.users(id),
    changed_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    old_data        JSONB,
    new_data        JSONB,
    session_info    JSONB
);

CREATE TABLE IF NOT EXISTS public.pr_status_log (
    log_id          BIGSERIAL    PRIMARY KEY,
    pr_id           BIGINT       NOT NULL REFERENCES public.purchase_request(pr_id),
    from_status     VARCHAR(30),
    to_status       VARCHAR(30)  NOT NULL,
    changed_by      BIGINT       NOT NULL REFERENCES public.users(id),
    changed_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    remarks         TEXT
);

CREATE TABLE IF NOT EXISTS public.po_status_log (
    log_id          BIGSERIAL    PRIMARY KEY,
    po_id           BIGINT       NOT NULL REFERENCES public.purchase_order(po_id),
    from_status     VARCHAR(30),
    to_status       VARCHAR(30)  NOT NULL,
    changed_by      BIGINT       NOT NULL REFERENCES public.users(id),
    changed_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    remarks         TEXT
);

-- ─── Indexes ──────────────────────────────────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_inventory_mat          ON public.inventory(mat_code);
CREATE INDEX IF NOT EXISTS idx_inventory_wh           ON public.inventory(warehouse_code);
CREATE INDEX IF NOT EXISTS idx_inv_txn_mat            ON public.inventory_transaction(mat_code);
CREATE INDEX IF NOT EXISTS idx_inv_txn_date           ON public.inventory_transaction(txn_date);
CREATE INDEX IF NOT EXISTS idx_inv_txn_type           ON public.inventory_transaction(txn_type);
CREATE INDEX IF NOT EXISTS idx_inv_txn_ref            ON public.inventory_transaction(ref_doc_type, ref_doc_no);
CREATE INDEX IF NOT EXISTS idx_pr_status              ON public.purchase_request(status);
CREATE INDEX IF NOT EXISTS idx_pr_requested_by        ON public.purchase_request(requested_by);
CREATE INDEX IF NOT EXISTS idx_po_status              ON public.purchase_order(status);
CREATE INDEX IF NOT EXISTS idx_po_supplier            ON public.purchase_order(supplier_code);
CREATE INDEX IF NOT EXISTS idx_grn_po                 ON public.grn(po_id);
CREATE INDEX IF NOT EXISTS idx_approval_req_doc       ON public.approval_request(doc_type, doc_id);
CREATE INDEX IF NOT EXISTS idx_approval_req_status    ON public.approval_request(status);
CREATE INDEX IF NOT EXISTS idx_approval_log_doc       ON public.approval_log(doc_type, doc_id);
CREATE INDEX IF NOT EXISTS idx_approval_log_at        ON public.approval_log(action_at);
CREATE INDEX IF NOT EXISTS idx_erp_audit_table        ON public.erp_audit_log(table_name, record_id);
CREATE INDEX IF NOT EXISTS idx_erp_audit_at           ON public.erp_audit_log(changed_at);

-- ─── Views ────────────────────────────────────────────────────────────────────
CREATE OR REPLACE VIEW public.v_inventory_full AS
SELECT
    i.inventory_id,
    i.mat_code,
    v.mat_name_th,
    v.mat_name_en,
    v.group_name,
    v.subgroup_name,
    v.spec_description,
    v.brand_name,
    v.unit_name,
    i.warehouse_code,
    w.warehouse_name,
    sz.zone_code,
    sz.zone_name,
    i.qty_on_hand,
    i.qty_reserved,
    i.qty_on_order,
    (i.qty_on_hand - i.qty_reserved) AS qty_available,
    i.reorder_point,
    i.min_stock,
    i.max_stock,
    CASE
        WHEN i.qty_on_hand <= COALESCE(i.min_stock, 0) THEN 'CRITICAL'
        WHEN i.qty_on_hand <= COALESCE(i.reorder_point, 0) THEN 'LOW'
        ELSE 'OK'
    END AS stock_status,
    i.last_counted_at,
    i.updated_at
FROM public.inventory i
JOIN public.v_material_full v ON v.mat_code = i.mat_code
JOIN public.warehouse w ON w.warehouse_code = i.warehouse_code
LEFT JOIN public.storage_zone sz ON sz.zone_id = i.zone_id;

CREATE OR REPLACE VIEW public.v_pending_approvals AS
SELECT
    ar.approval_id,
    ar.doc_type,
    ar.doc_no,
    ar.step_no,
    ac.step_name,
    ar.status,
    ar.amount,
    ar.due_date,
    u_req.full_name  AS requested_by_name,
    u_ass.full_name  AS assigned_to_name,
    ar.requested_by,
    ar.assigned_to,
    ar.created_at
FROM public.approval_request ar
JOIN public.approval_config ac ON ac.doc_type = ar.doc_type AND ac.step_no = ar.step_no
JOIN public.users u_req ON u_req.id = ar.requested_by
LEFT JOIN public.users u_ass ON u_ass.id = ar.assigned_to
WHERE ar.status = 'PENDING';

CREATE OR REPLACE VIEW public.v_pr_full AS
SELECT
    pr.pr_id,
    pr.pr_no,
    pr.pr_date,
    pr.required_date::text,
    pr.status          AS pr_status,
    pr.priority,
    u.full_name        AS requested_by_name,
    l.location_name,
    l.location_type,
    w.warehouse_name,
    (SELECT COUNT(*) FROM public.purchase_request_line prl WHERE prl.pr_id = pr.pr_id) AS line_count,
    pr.remarks,
    pr.requested_by,
    pr.created_at
FROM public.purchase_request pr
JOIN public.users u ON u.id = pr.requested_by
JOIN public.location l ON l.location_code = pr.location_code
LEFT JOIN public.warehouse w ON w.warehouse_code = pr.warehouse_code;

CREATE OR REPLACE VIEW public.v_po_full AS
SELECT
    po.po_id,
    po.po_no,
    po.po_date,
    po.expected_date::text,
    po.status          AS po_status,
    s.supplier_name,
    s.contact_name     AS supplier_contact,
    po.currency,
    po.total_amount,
    po.vat_amount,
    po.net_amount,
    pr.pr_no,
    w.warehouse_name,
    u.full_name        AS created_by_name,
    po.created_at
FROM public.purchase_order po
JOIN public.supplier s ON s.supplier_code = po.supplier_code
JOIN public.warehouse w ON w.warehouse_code = po.warehouse_code
JOIN public.users u ON u.id = po.created_by
LEFT JOIN public.purchase_request pr ON pr.pr_id = po.pr_id;

-- ─── Default seed data ────────────────────────────────────────────────────────

-- Approval config (PR: 2 steps, PO: 1 step)
INSERT INTO public.approval_config (doc_type, step_no, step_name, is_active) VALUES
    ('PR', 1, 'Senior Team Project Review', TRUE),
    ('PR', 2, 'Manager Approval',           TRUE),
    ('PO', 1, 'Manager / Director / MD Approval', TRUE),
    ('GRN', 1, 'Stock Confirmation',        TRUE)
ON CONFLICT (doc_type, step_no) DO NOTHING;

-- Default admin user (password: Admin@1234)
INSERT INTO public.users (username, full_name, email, password_hash, is_active)
VALUES ('admin', 'System Administrator', 'admin@erp.local',
        '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQyCrK3BEJXJGmPPjFTjLnOoK', TRUE)
ON CONFLICT (username) DO NOTHING;

COMMIT;

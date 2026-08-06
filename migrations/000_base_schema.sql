-- =============================================================================
-- ERP v1 (base) — Reconstructed base schema
-- Runs BEFORE 001_master_ddl.sql (docker-entrypoint-initdb.d applies files in
-- alphabetical order). Reconstructed from docs/database.md (ERP_V12 dump
-- reference) — NOT the original source. Review column types/constraints
-- against database.md yourself before applying.
--
-- Scope: only tables/views that 001_master_ddl.sql (and 002-015) reference
-- via FK or JOIN but never CREATE themselves. Does NOT re-create
-- purchase_request/purchase_order/grn/etc. — those already have
-- CREATE TABLE IF NOT EXISTS in 001_master_ddl.sql. See the note at the
-- bottom of this file about the po_id/id PK conflict between 001 and
-- migrations 014/015 — that is a separate, unresolved problem this file
-- does not attempt to fix.
-- PostgreSQL 14+
-- =============================================================================

BEGIN;

-- ─── Departments ──────────────────────────────────────────────────────────────
-- Not FK-referenced by any migration (roles.dept_code / users.department are
-- plain varchar, no FK) — included for completeness per database.md only.
CREATE TABLE IF NOT EXISTS public.departments (
    id         BIGSERIAL    PRIMARY KEY,
    dept_code  VARCHAR(20)  NOT NULL UNIQUE,
    dept_name  VARCHAR(200) NOT NULL,
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    sort_order INTEGER      NOT NULL DEFAULT 0,
    created_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- ─── Roles ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.roles (
    id          BIGSERIAL    PRIMARY KEY,
    role_code   VARCHAR(30)  NOT NULL UNIQUE,
    role_name   VARCHAR(100) NOT NULL,
    description TEXT,
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    level       INTEGER      NOT NULL DEFAULT 0,
    department  VARCHAR(100),
    dept_code   VARCHAR(20),
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by  BIGINT,
    updated_by  BIGINT
);

-- ─── Users ────────────────────────────────────────────────────────────────────
-- 001_master_ddl.sql only ALTERs this table (adds location_code, employee_code,
-- department) — base columns below reconstructed from database.md.
CREATE TABLE IF NOT EXISTS public.users (
    id            BIGSERIAL    PRIMARY KEY,
    username      VARCHAR(50)  NOT NULL UNIQUE,
    full_name     VARCHAR(200) NOT NULL,
    email         VARCHAR(100),
    password_hash TEXT,
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    employee_code VARCHAR(30),
    department    VARCHAR(100),
    dept_code     VARCHAR(20),
    location_code VARCHAR(20),
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by    BIGINT,
    updated_by    BIGINT
);

-- FKs added after both tables exist (roles/users have created_by/updated_by
-- pointing at users.id, and users itself has no FK dependency on roles).
ALTER TABLE public.roles
    ADD CONSTRAINT roles_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id),
    ADD CONSTRAINT roles_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);

ALTER TABLE public.users
    ADD CONSTRAINT users_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id),
    ADD CONSTRAINT users_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);

-- ─── user_roles ───────────────────────────────────────────────────────────────
-- Not FK-referenced by any migration file directly, but documented in
-- database.md as the users<->roles join table. Included for completeness.
CREATE TABLE IF NOT EXISTS public.user_roles (
    user_id    BIGINT    NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    role_id    BIGINT    NOT NULL REFERENCES public.roles(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by BIGINT REFERENCES public.users(id),
    PRIMARY KEY (user_id, role_id)
);

-- ─── Cost hierarchy ───────────────────────────────────────────────────────────
-- Required because material_code.cost_subgroup_id references cost_subgroup
-- (per database.md's material_code FK list).
CREATE TABLE IF NOT EXISTS public.cost_subject (
    id           BIGSERIAL    PRIMARY KEY,
    subject_code VARCHAR(20)  NOT NULL UNIQUE,
    subject_name VARCHAR(200) NOT NULL,
    is_active    BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by   BIGINT REFERENCES public.users(id),
    updated_by   BIGINT REFERENCES public.users(id)
);

CREATE TABLE IF NOT EXISTS public.cost_job (
    id         BIGSERIAL    PRIMARY KEY,
    subject_id BIGINT       NOT NULL REFERENCES public.cost_subject(id),
    job_code   VARCHAR(20)  NOT NULL UNIQUE,
    job_name   VARCHAR(200) NOT NULL,
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by BIGINT REFERENCES public.users(id),
    updated_by BIGINT REFERENCES public.users(id)
);

CREATE TABLE IF NOT EXISTS public.cost_group (
    id         BIGSERIAL    PRIMARY KEY,
    job_id     BIGINT       NOT NULL REFERENCES public.cost_job(id),
    group_code VARCHAR(20)  NOT NULL UNIQUE,
    group_name VARCHAR(200) NOT NULL,
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by BIGINT REFERENCES public.users(id),
    updated_by BIGINT REFERENCES public.users(id)
);

CREATE TABLE IF NOT EXISTS public.cost_subgroup (
    id            BIGSERIAL    PRIMARY KEY,
    group_id      BIGINT       NOT NULL REFERENCES public.cost_group(id),
    subgroup_code VARCHAR(20)  NOT NULL UNIQUE,
    subgroup_name VARCHAR(200) NOT NULL,
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by    BIGINT REFERENCES public.users(id),
    updated_by    BIGINT REFERENCES public.users(id)
);

-- ─── Unit ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS public.unit (
    id         SERIAL      PRIMARY KEY,
    unit_code  VARCHAR(20) NOT NULL UNIQUE,
    unit_name  VARCHAR(50) NOT NULL,
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP   NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP   NOT NULL DEFAULT NOW(),
    created_by BIGINT REFERENCES public.users(id),
    updated_by BIGINT REFERENCES public.users(id)
);

-- ─── Material hierarchy: mat_group → subgroup → mat_name → spec_size → brand ──
CREATE TABLE IF NOT EXISTS public.mat_group (
    id         BIGSERIAL    PRIMARY KEY,
    group_code VARCHAR(20)  NOT NULL UNIQUE,
    group_name VARCHAR(200) NOT NULL,
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by BIGINT REFERENCES public.users(id),
    updated_by BIGINT REFERENCES public.users(id)
);

CREATE TABLE IF NOT EXISTS public.subgroup (
    id            BIGSERIAL    PRIMARY KEY,
    group_id      BIGINT       NOT NULL REFERENCES public.mat_group(id),
    subgroup_code VARCHAR(20)  NOT NULL UNIQUE,
    subgroup_name VARCHAR(200) NOT NULL,
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by    BIGINT REFERENCES public.users(id),
    updated_by    BIGINT REFERENCES public.users(id)
);

-- ⚠️ AMBIGUITY (see note #1 at bottom of file): database.md's mat_name table
-- has a single `mat_name` column, but 001_master_ddl.sql's v_inventory_full
-- view expects v_material_full to expose BOTH `mat_name_th` and `mat_name_en`.
-- Kept as single `mat_name` here (per database.md); v_material_full below
-- aliases it to both — flagged, not silently resolved.
CREATE TABLE IF NOT EXISTS public.mat_name (
    id            BIGSERIAL    PRIMARY KEY,
    mat_name_code VARCHAR(20)  NOT NULL UNIQUE,
    subgroup_id   BIGINT       NOT NULL REFERENCES public.subgroup(id),
    mat_name      VARCHAR(300) NOT NULL,
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by    BIGINT REFERENCES public.users(id),
    updated_by    BIGINT REFERENCES public.users(id)
);

CREATE TABLE IF NOT EXISTS public.spec_size (
    id               BIGSERIAL PRIMARY KEY,
    spec_code        VARCHAR(20) NOT NULL UNIQUE,
    mat_name_id      BIGINT      NOT NULL REFERENCES public.mat_name(id),
    spec_description VARCHAR(300),
    is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMP   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP   NOT NULL DEFAULT NOW(),
    created_by       BIGINT REFERENCES public.users(id),
    updated_by       BIGINT REFERENCES public.users(id)
);

CREATE TABLE IF NOT EXISTS public.brand (
    id         SERIAL       PRIMARY KEY,
    brand_code VARCHAR(20)  NOT NULL UNIQUE,
    spec_id    INTEGER      NOT NULL REFERENCES public.spec_size(id),
    brand_name VARCHAR(100) NOT NULL,
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by BIGINT REFERENCES public.users(id),
    updated_by BIGINT REFERENCES public.users(id)
);

-- ─── material_code (master material record) ──────────────────────────────────
-- ⚠️ AMBIGUITY (see note #2 at bottom): database.md lists material_code's FK
-- columns (group_id, subgroup_id, mat_name_id, spec_id, brand_id, unit_id,
-- cost_subgroup_id) without marking which are nullable. Assumed brand_id and
-- cost_subgroup_id are nullable (not every material has a brand or a cost
-- code assigned at creation) and the rest NOT NULL — verify against real data.
CREATE TABLE IF NOT EXISTS public.material_code (
    id               BIGSERIAL   PRIMARY KEY,
    mat_code         VARCHAR(20) NOT NULL UNIQUE,
    group_id         BIGINT      NOT NULL REFERENCES public.mat_group(id),
    subgroup_id      BIGINT      NOT NULL REFERENCES public.subgroup(id),
    mat_name_id      BIGINT      NOT NULL REFERENCES public.mat_name(id),
    spec_id          BIGINT      NOT NULL REFERENCES public.spec_size(id),
    brand_id         BIGINT      REFERENCES public.brand(id),
    unit_id          BIGINT      NOT NULL REFERENCES public.unit(id),
    cost_subgroup_id BIGINT      REFERENCES public.cost_subgroup(id),
    is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMP   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP   NOT NULL DEFAULT NOW(),
    created_by       BIGINT REFERENCES public.users(id),
    updated_by       BIGINT REFERENCES public.users(id)
);

-- ─── Project ──────────────────────────────────────────────────────────────────
-- Required by migrations/009_memo.sql: memo.project_code REFERENCES
-- public.project(project_code) — no migration ever creates this table.
CREATE TABLE IF NOT EXISTS public.project (
    id              SERIAL        PRIMARY KEY,
    project_code    VARCHAR(20)   NOT NULL UNIQUE,
    project_name    VARCHAR(200)  NOT NULL,
    location_code   VARCHAR(20),
    start_date      DATE,
    end_date        DATE,
    status          VARCHAR(20)   NOT NULL DEFAULT 'ACTIVE',
    is_active       BOOLEAN       NOT NULL DEFAULT TRUE,
    owner_id        BIGINT REFERENCES public.users(id),
    budget_amount   NUMERIC(18,4) NOT NULL DEFAULT 0,
    consultant_name VARCHAR(200),
    created_at      TIMESTAMP     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP     NOT NULL DEFAULT NOW(),
    created_by      BIGINT REFERENCES public.users(id),
    updated_by      BIGINT REFERENCES public.users(id)
);

-- ─── v_material_full ──────────────────────────────────────────────────────────
-- Required by 001_master_ddl.sql's v_inventory_full: "JOIN public.v_material_full
-- v ON v.mat_code = i.mat_code" — must exist before 001 runs.
-- Column list reconstructed from how 001's v_inventory_full consumes it
-- (v.mat_name_th, v.mat_name_en, v.group_name, v.subgroup_name,
-- v.spec_description, v.brand_name, v.unit_name) — see ambiguity note #1.
CREATE OR REPLACE VIEW public.v_material_full AS
SELECT
    mc.id,
    mc.mat_code,
    mn.mat_name        AS mat_name_th,
    mn.mat_name         AS mat_name_en,
    mg.group_name,
    sg.subgroup_name,
    ss.spec_description,
    b.brand_name,
    u.unit_name,
    mc.is_active,
    mc.created_at,
    mc.updated_at
FROM public.material_code mc
JOIN public.mat_group mg   ON mg.id = mc.group_id
JOIN public.subgroup sg    ON sg.id = mc.subgroup_id
JOIN public.mat_name mn    ON mn.id = mc.mat_name_id
JOIN public.spec_size ss   ON ss.id = mc.spec_id
LEFT JOIN public.brand b   ON b.id = mc.brand_id
JOIN public.unit u         ON u.id = mc.unit_id;

COMMIT;

-- =============================================================================
-- NOTES — read before applying
-- =============================================================================
--
-- 1. AMBIGUITY: mat_name_th / mat_name_en split.
--    database.md documents mat_name as a single `mat_name` column, but
--    001_master_ddl.sql's v_inventory_full expects v_material_full to expose
--    two separate columns (mat_name_th, mat_name_en). This file resolves it
--    by aliasing the one column to both names — this is almost certainly NOT
--    what the real ERP_V12 dump does (it may have a genuine th/en split that
--    database.md simply didn't capture). Confirm against the live DB (or
--    original dump) before trusting this in production.
--
-- 2. AMBIGUITY: nullability of material_code.brand_id / cost_subgroup_id /
--    unit_id. database.md lists these as FK columns without marking
--    NOT NULL/nullable. Assumed brand_id and cost_subgroup_id nullable,
--    unit_id required — verify.
--
-- 3. UNRESOLVED (not fixed by this file): 001_master_ddl.sql creates
--    purchase_request/purchase_order/grn/borrow/rfq/stock_count/approval_*/
--    pr_status_log/po_status_log/erp_audit_log with old-style PK names
--    (pr_id, po_id, grn_id, borrow_id, rfq_id, count_id, config_id,
--    approval_id, log_id, audit_id, txn_id, inventory_id). Migrations 014
--    and 015 (po_attachment, po_approver_id) instead reference
--    "purchase_order(id)" — the real ERP_V12 schema (database.md) uses `id`
--    as PK everywhere. Running 001 fresh will:
--      a) succeed at CREATE TABLE (IF NOT EXISTS doesn't conflict with this
--         file, since this file does not create those tables), but
--      b) 001's own v_inventory_full/v_pending_approvals/v_pr_full/v_po_full
--         view definitions reference i.inventory_id, ar.approval_id,
--         pr.pr_id, po.po_id — none of which would exist if those tables
--         were ever built to the real `id`-PK shape instead. As currently
--         written, 001 creates its OWN pr_id/po_id-style tables, so its own
--         views will work internally — but migration 014/015 will then FAIL
--         ("column po_id... / relation purchase_order has no column id")
--         since they expect `id`.
--    This is a defect in 001_master_ddl.sql / 014 / 015 themselves, not
--    something a base-schema file can resolve. Decide separately whether to
--    patch 001 to use `id` PKs (matching 014/015 and real production), or
--    patch 014/015 to use po_id (matching 001) — I have not touched 001,
--    014, or 015.
-- =============================================================================

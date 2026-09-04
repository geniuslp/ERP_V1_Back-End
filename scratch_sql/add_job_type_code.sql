-- Job Type field for PR and PO.
-- Run this manually in pgAdmin. Per project convention there are no migration files —
-- this lives in scratch_sql/ for reference only, it is NOT auto-applied.
--
-- Constraint name on purchase_order.work_type confirmed against the live DB on 2026-09-03:
--   SELECT conname FROM pg_constraint WHERE conrelid = 'purchase_order'::regclass;
-- → chk_purchase_order_work_type (used below, not guessed).
--
-- Live data checked before writing this (2026-09-03): purchase_order.work_type had 10 rows
-- 'P', 3 rows NULL (no E/S/F/G/H rows existed); purchase_request.job_code had 7 rows 'MP',
-- 0 NULL. NULL backfill default 'G' (General Code) confirmed with the business owner.

BEGIN;

-- ── purchase_request.job_code — already exists as a nullable varchar, just needs
-- backfilling and NOT NULL added ────────────────────────────────────────────────
UPDATE purchase_request
SET job_code = 'G'
WHERE job_code IS NULL OR job_code = '';

ALTER TABLE purchase_request
    ALTER COLUMN job_code TYPE VARCHAR(10),
    ALTER COLUMN job_code SET NOT NULL;

-- ── purchase_order — rename work_type (P|E|S|F|G|H) to job_code, translate its
-- values into the new two-letter scheme, backfill, and make NOT NULL ───────────
-- Order matters: the column must be widened to VARCHAR(10) BEFORE the CASE UPDATE writes
-- 2-letter codes into it — work_type is VARCHAR(1) live, so writing 'MP' etc. before
-- widening would fail with "value too long for type character varying(1)".
ALTER TABLE purchase_order
    DROP CONSTRAINT IF EXISTS chk_purchase_order_work_type;

ALTER TABLE purchase_order RENAME COLUMN work_type TO job_code;

ALTER TABLE purchase_order
    ALTER COLUMN job_code TYPE VARCHAR(10);

UPDATE purchase_order SET job_code = CASE job_code
    WHEN 'P' THEN 'MP'
    WHEN 'E' THEN 'ME'
    WHEN 'S' THEN 'MS'
    WHEN 'F' THEN 'MF'
    WHEN 'G' THEN 'MG'
    WHEN 'H' THEN 'MH'
    ELSE job_code
END
WHERE job_code IN ('P','E','S','F','G','H');

UPDATE purchase_order
SET job_code = 'G'
WHERE job_code IS NULL OR job_code = '';

ALTER TABLE purchase_order
    ALTER COLUMN job_code SET NOT NULL;

-- ── Fixed 12-value CHECK constraint on both tables, matching handlers.JobTypes ──
ALTER TABLE purchase_request
    DROP CONSTRAINT IF EXISTS purchase_request_job_code_check;
ALTER TABLE purchase_request
    ADD CONSTRAINT purchase_request_job_code_check
    CHECK (job_code IN ('MP','ME','MS','MF','MG','MH','FS','FP','FB','DE','RE','G'));

ALTER TABLE purchase_order
    DROP CONSTRAINT IF EXISTS purchase_order_job_code_check;
ALTER TABLE purchase_order
    ADD CONSTRAINT purchase_order_job_code_check
    CHECK (job_code IN ('MP','ME','MS','MF','MG','MH','FS','FP','FB','DE','RE','G'));

COMMIT;

-- ── Row-count sanity check — run this AFTER the migration to confirm no rows were
-- dropped/corrupted (expect po_count=13, pr_count=7 — same as pre-migration counts — and
-- all four null/invalid columns = 0) ────────────────────────────────────────────
-- SELECT
--   (SELECT COUNT(*) FROM purchase_order)   AS po_count,
--   (SELECT COUNT(*) FROM purchase_request) AS pr_count,
--   (SELECT COUNT(*) FROM purchase_order WHERE job_code IS NULL)   AS po_null_job_code,
--   (SELECT COUNT(*) FROM purchase_request WHERE job_code IS NULL) AS pr_null_job_code,
--   (SELECT COUNT(*) FROM purchase_order WHERE job_code NOT IN ('MP','ME','MS','MF','MG','MH','FS','FP','FB','DE','RE','G'))   AS po_invalid_job_code,
--   (SELECT COUNT(*) FROM purchase_request WHERE job_code NOT IN ('MP','ME','MS','MF','MG','MH','FS','FP','FB','DE','RE','G')) AS pr_invalid_job_code;

-- Job type codes renamed (2026-10-26):
--   MP -> MS (Metal Structure)      ME -> EE (Electrical System)   MS -> SN (Sanitary System)
--   MF -> FF (Fire Protection)      MG -> GS (Gas System)          MH -> AC (HVAC / BAS / Clean-R.)
-- FS, FP, FB, DE, RE, G, OH are unchanged. Legacy single letters in work_order.work_system (P, E, S) are
-- translated to the same new codes (P -> MS, E -> EE, S -> SN).
--
-- NOT re-runnable: MS means "Sanitary" before and "Metal Structure" after, so a second run would turn
-- Metal into Sanitary. The guard below aborts if job_cost_key() already exists (it is created at the end).
--
-- cost_job / cost_subject keep their own single-letter codes (P/E/S/F/G/H). The old code was
-- subject_code || job_code (MP = subject M + job P); the new codes no longer spell that, so
-- job_cost_key() maps a job type code back to the old "subject+job" key for those joins.

BEGIN;

DO $$
BEGIN
    IF to_regprocedure('public.job_cost_key(text)') IS NOT NULL THEN
        RAISE EXCEPTION '030_job_code_rename was already applied (job_cost_key exists) - aborting';
    END IF;
END $$;

-- 1. constraints off while values change
ALTER TABLE public.purchase_request DROP CONSTRAINT IF EXISTS purchase_request_job_code_check;
ALTER TABLE public.purchase_order   DROP CONSTRAINT IF EXISTS purchase_order_job_code_check;
ALTER TABLE public.work_order       DROP CONSTRAINT IF EXISTS chk_wo_work_system;

-- 2. old -> new, through a temporary prefix so the MS <-> SN swap never collides (project_job_share has a PK on
--    (project_id, job_code) and Postgres checks it row by row).
CREATE TEMP TABLE job_code_map (old_code text PRIMARY KEY, new_code text NOT NULL) ON COMMIT DROP;
INSERT INTO job_code_map VALUES
    ('MP','MS'), ('ME','EE'), ('MS','SN'), ('MF','FF'), ('MG','GS'), ('MH','AC');

UPDATE public.project_job_share s SET job_code = 'tmp_' || m.new_code FROM job_code_map m WHERE s.job_code = m.old_code;
UPDATE public.project_job_share SET job_code = substr(job_code, 5) WHERE job_code LIKE 'tmp\_%';

UPDATE public.purchase_request t SET job_code = m.new_code FROM job_code_map m WHERE t.job_code = m.old_code;
UPDATE public.purchase_order   t SET job_code = m.new_code FROM job_code_map m WHERE t.job_code = m.old_code;
UPDATE public.memo             t SET job_code = m.new_code FROM job_code_map m WHERE t.job_code = m.old_code;
UPDATE public.ic_project_movement t SET job_code = m.new_code FROM job_code_map m WHERE t.job_code = m.old_code;

-- arrays: keep the order of the elements
UPDATE public.project p
SET job_codes = ARRAY(
    SELECT COALESCE(m.new_code, x.code)
    FROM unnest(p.job_codes) WITH ORDINALITY AS x(code, ord)
    LEFT JOIN job_code_map m ON m.old_code = x.code
    ORDER BY x.ord)
WHERE p.job_codes && ARRAY(SELECT old_code FROM job_code_map);

CREATE TEMP TABLE ws_map (old_code text PRIMARY KEY, new_code text NOT NULL) ON COMMIT DROP;
INSERT INTO ws_map SELECT old_code, new_code FROM job_code_map;
INSERT INTO ws_map VALUES ('P','MS'), ('E','EE'), ('S','SN');

UPDATE public.work_order w
SET work_system = ARRAY(
    SELECT DISTINCT ON (COALESCE(m.new_code, x.code)) COALESCE(m.new_code, x.code)
    FROM unnest(w.work_system) WITH ORDINALITY AS x(code, ord)
    LEFT JOIN ws_map m ON m.old_code = x.code
    ORDER BY COALESCE(m.new_code, x.code), x.ord)
WHERE w.work_system && ARRAY(SELECT old_code FROM ws_map);

-- 3. constraints back, with the new list
ALTER TABLE public.purchase_request ADD CONSTRAINT purchase_request_job_code_check
    CHECK (job_code IN ('MS','EE','SN','FF','GS','AC','FS','FP','FB','DE','RE','G'));
ALTER TABLE public.purchase_order ADD CONSTRAINT purchase_order_job_code_check
    CHECK (job_code IN ('MS','EE','SN','FF','GS','AC','FS','FP','FB','DE','RE','G'));
ALTER TABLE public.work_order ADD CONSTRAINT chk_wo_work_system
    CHECK (cardinality(work_system) >= 1 AND work_system <@ ARRAY['MS','EE','SN','FF','GS','AC']::text[]);

-- 4. job type code -> the old "subject_code || job_code" key used to find cost_subject / cost_job rows
CREATE OR REPLACE FUNCTION public.job_cost_key(code text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE code
        WHEN 'MS' THEN 'MP' WHEN 'EE' THEN 'ME' WHEN 'SN' THEN 'MS'
        WHEN 'FF' THEN 'MF' WHEN 'GS' THEN 'MG' WHEN 'AC' THEN 'MH'
        ELSE code END
$$;

COMMIT;

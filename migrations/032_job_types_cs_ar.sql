-- Two new big job types (2026-10-27):  CS = Civil&Structure (cost job letter C),  AR = Architec (cost job letter A).
-- Treated like the other engineering-system jobs (MS, EE, SN, FF, GS, AC): they take a budget share and can be a WO "work system".
-- Idempotent: every statement does nothing once its rows exist. Cost Code data comes from the owner's sheet:
--   subject M: group 00 "Subcontractor" -> subgroup 000 (Major)
--   subject S: group 30 "Subcontractor" -> subgroup 300 (Head), 301
--   for both job C (Civil&Structure) and job A (Architec). Full cost codes: MC00000, SC30300, SC30301, MA00000, SA30300, SA30301.

BEGIN;

-- 1. cost data -------------------------------------------------------------------------------------------------------
WITH jobs(job_code, job_name) AS (VALUES ('C', 'Civil&Structure'), ('A', 'Architec')),
     subj AS (SELECT id, subject_code FROM public.cost_subject WHERE subject_code IN ('M', 'S'))
INSERT INTO public.cost_job (subject_id, job_code, job_name, is_active)
SELECT s.id, j.job_code, j.job_name, true
FROM subj s CROSS JOIN jobs j
ON CONFLICT (subject_id, job_code) DO NOTHING;

INSERT INTO public.cost_group (job_id, group_code, group_name, is_active)
SELECT cj.id, CASE cs.subject_code WHEN 'M' THEN '00' ELSE '30' END, 'Subcontractor', true
FROM public.cost_job cj
JOIN public.cost_subject cs ON cs.id = cj.subject_id
WHERE cs.subject_code IN ('M', 'S') AND cj.job_code IN ('C', 'A')
ON CONFLICT (job_id, group_code) DO NOTHING;

-- M / 00 / 000  = the Major row;  S / 30 / 300 = Head;  S / 30 / 301 = the normal subgroup
INSERT INTO public.cost_subgroup (group_id, subgroup_code, subgroup_name, is_active, is_head, is_major)
SELECT cg.id, v.subgroup_code, 'Subcontractor', true, v.is_head, v.is_major
FROM public.cost_group cg
JOIN public.cost_job cj     ON cj.id = cg.job_id
JOIN public.cost_subject cs ON cs.id = cj.subject_id
JOIN (VALUES ('M', '000', false, true), ('S', '300', true, false), ('S', '301', false, false))
       AS v(subject_code, subgroup_code, is_head, is_major) ON v.subject_code = cs.subject_code
WHERE cj.job_code IN ('C', 'A')
ON CONFLICT (group_id, subgroup_code) DO NOTHING;

-- 2. allowed job codes -----------------------------------------------------------------------------------------------
ALTER TABLE public.purchase_request DROP CONSTRAINT IF EXISTS purchase_request_job_code_check;
ALTER TABLE public.purchase_order   DROP CONSTRAINT IF EXISTS purchase_order_job_code_check;
ALTER TABLE public.work_order       DROP CONSTRAINT IF EXISTS chk_wo_work_system;

ALTER TABLE public.purchase_request ADD CONSTRAINT purchase_request_job_code_check
    CHECK (job_code IN ('MS','EE','SN','FF','GS','AC','CS','AR','FS','FP','FB','DE','RE','G'));
ALTER TABLE public.purchase_order ADD CONSTRAINT purchase_order_job_code_check
    CHECK (job_code IN ('MS','EE','SN','FF','GS','AC','CS','AR','FS','FP','FB','DE','RE','G'));
ALTER TABLE public.work_order ADD CONSTRAINT chk_wo_work_system
    CHECK (cardinality(work_system) >= 1 AND work_system <@ ARRAY['MS','EE','SN','FF','GS','AC','CS','AR']::text[]);

-- 3. job type code -> the "subject_code || job_code" key used to find cost_subject / cost_job rows (see 030) ----------
CREATE OR REPLACE FUNCTION public.job_cost_key(code text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE code
        WHEN 'MS' THEN 'MP' WHEN 'EE' THEN 'ME' WHEN 'SN' THEN 'MS'
        WHEN 'FF' THEN 'MF' WHEN 'GS' THEN 'MG' WHEN 'AC' THEN 'MH'
        WHEN 'CS' THEN 'MC' WHEN 'AR' THEN 'MA'
        ELSE code END
$$;

COMMIT;

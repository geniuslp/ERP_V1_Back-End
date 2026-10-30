-- General Code is stored as "G" everywhere (PR / PO CHECK, order types, frontend). The project job list and
-- /master/job-types still said "OH", so a project could hold OH but PR / PO / WO / Memo / IC never offered G.
-- Rename OH -> G where it was stored, and let a work order's work_system hold G too. Idempotent.
BEGIN;

UPDATE project
   SET job_codes = (SELECT ARRAY(SELECT DISTINCT CASE WHEN x = 'OH' THEN 'G' ELSE x END
                                   FROM unnest(job_codes) AS x))
 WHERE 'OH' = ANY(job_codes);

UPDATE project_job_share SET job_code = 'G' WHERE job_code = 'OH'
   AND NOT EXISTS (SELECT 1 FROM project_job_share s WHERE s.project_id = project_job_share.project_id AND s.job_code = 'G');
DELETE FROM project_job_share WHERE job_code = 'OH';

UPDATE memo SET job_code = 'G' WHERE job_code = 'OH';
UPDATE ic_project_movement SET job_code = 'G' WHERE job_code = 'OH';

ALTER TABLE work_order DROP CONSTRAINT IF EXISTS chk_wo_work_system;
ALTER TABLE work_order ADD CONSTRAINT chk_wo_work_system
    CHECK (cardinality(work_system) >= 1 AND work_system <@ ARRAY['MS','EE','SN','FF','GS','AC','CS','AR','G']::text[]);

COMMIT;

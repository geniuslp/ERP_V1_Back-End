-- Project tier, derived from the project value and kept in sync by the database itself.
-- Value = project value (project_kind VO -> vo_amount, otherwise budget_amount); when that is empty
-- or 0 the contract_amount is used; when neither is set the tier is NULL.
--   1: < 1M   2: 1M - < 10M   3: 10M - < 100M   4: 100M - < 500M   5: >= 500M
BEGIN;

ALTER TABLE project ADD COLUMN IF NOT EXISTS project_tier smallint GENERATED ALWAYS AS (
  CASE
    WHEN COALESCE(NULLIF(CASE WHEN project_kind = 'VO' THEN vo_amount ELSE budget_amount END, 0), NULLIF(contract_amount, 0)) IS NULL THEN NULL
    WHEN COALESCE(NULLIF(CASE WHEN project_kind = 'VO' THEN vo_amount ELSE budget_amount END, 0), NULLIF(contract_amount, 0)) < 1000000 THEN 1
    WHEN COALESCE(NULLIF(CASE WHEN project_kind = 'VO' THEN vo_amount ELSE budget_amount END, 0), NULLIF(contract_amount, 0)) < 10000000 THEN 2
    WHEN COALESCE(NULLIF(CASE WHEN project_kind = 'VO' THEN vo_amount ELSE budget_amount END, 0), NULLIF(contract_amount, 0)) < 100000000 THEN 3
    WHEN COALESCE(NULLIF(CASE WHEN project_kind = 'VO' THEN vo_amount ELSE budget_amount END, 0), NULLIF(contract_amount, 0)) < 500000000 THEN 4
    ELSE 5
  END
) STORED;

COMMIT;

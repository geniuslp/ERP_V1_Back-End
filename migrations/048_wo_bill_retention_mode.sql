-- How the retention (เงินประกันผลงาน, row 5 of the contractor bill) was worked out on a bill:
--   EVERY = each bill: % of that bill's amount (the old behaviour, also what existing rows are)
--   ONCE  = the WO's total retention (contract value x %) split evenly over all installments (incl. Advance)
-- Only the choice is stored; the amount itself is still the RETENTION line of the bill. Idempotent, own transaction.
BEGIN;
ALTER TABLE public.wo_contractor_log_bill
    ADD COLUMN IF NOT EXISTS retention_mode VARCHAR(10) NOT NULL DEFAULT 'EVERY';
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'wo_contractor_log_bill_retention_mode_chk') THEN
        ALTER TABLE public.wo_contractor_log_bill
            ADD CONSTRAINT wo_contractor_log_bill_retention_mode_chk CHECK (retention_mode IN ('EVERY','ONCE'));
    END IF;
END $$;
COMMIT;

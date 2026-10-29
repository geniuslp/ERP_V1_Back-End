-- Free-text note for the "2. เงินงวดนี้" line of a contractor bill (e.g. which installment it is).
BEGIN;
ALTER TABLE wo_contractor_log_bill ADD COLUMN IF NOT EXISTS progress_note varchar(500);
COMMIT;

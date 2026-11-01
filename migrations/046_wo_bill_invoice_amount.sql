-- Amount drawn on each invoice of a contractor bill ("มูลค่า/INV ที่เบิก"). Existing rows stay NULL.
BEGIN;
ALTER TABLE wo_contractor_log_bill_invoice ADD COLUMN IF NOT EXISTS invoice_amount numeric(15,2);
COMMIT;

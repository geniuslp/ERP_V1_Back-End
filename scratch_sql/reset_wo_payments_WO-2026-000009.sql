-- Reset the payment records of WO-2026-000009 (finance payments page -> "รายละเอียดการจ่ายเงิน").
-- Removes: payment_log rows (doc_type 'WO', incl. reversal rows) and the paid state of its installments.
-- Also removes the contractor-log bills of this WO (wo_contractor_log_bill + lines + invoices, via ON DELETE CASCADE):
-- the "Contract / Advance / Retention" summary is computed from SAVED bills, so Contract only returns to 0 once they are gone.
-- Keeps:   the installment plan, the WO, the contractor-log header (wo_contractor_log) and its terms (Advance % / Retention %).
-- Run in pgAdmin on the target DB. Check the three previews first; the script ends with ROLLBACK -
-- change it to COMMIT only after the "after" counts look right.

BEGIN;

-- before
SELECT id, wo_no, status FROM work_order WHERE wo_no = 'WO-2026-000009';
SELECT id, amount_paid, paid_date, reverses_id, remark FROM payment_log
 WHERE doc_type = 'WO' AND doc_id = (SELECT id FROM work_order WHERE wo_no = 'WO-2026-000009') ORDER BY id;
SELECT id, installment_no, amount, is_paid, paid_at, payment_log_id FROM payment_installment
 WHERE doc_type = 'WO' AND doc_id = (SELECT id FROM work_order WHERE wo_no = 'WO-2026-000009') ORDER BY installment_no;

-- installments first (they reference payment_log)
UPDATE payment_installment
   SET is_paid = false, paid_at = NULL, paid_by = NULL, payment_log_id = NULL, updated_at = now()
 WHERE doc_type = 'WO' AND doc_id = (SELECT id FROM work_order WHERE wo_no = 'WO-2026-000009');

DELETE FROM payment_log
 WHERE doc_type = 'WO' AND doc_id = (SELECT id FROM work_order WHERE wo_no = 'WO-2026-000009');

DELETE FROM wo_contractor_log_bill
 WHERE log_id IN (SELECT id FROM wo_contractor_log WHERE wo_id = (SELECT id FROM work_order WHERE wo_no = 'WO-2026-000009'));

-- after: both should be 0 / all is_paid = false
SELECT count(*) AS payment_rows_left FROM payment_log
 WHERE doc_type = 'WO' AND doc_id = (SELECT id FROM work_order WHERE wo_no = 'WO-2026-000009');
SELECT installment_no, amount, is_paid FROM payment_installment
 WHERE doc_type = 'WO' AND doc_id = (SELECT id FROM work_order WHERE wo_no = 'WO-2026-000009') ORDER BY installment_no;

SELECT count(*) AS bills_left FROM wo_contractor_log_bill
 WHERE log_id IN (SELECT id FROM wo_contractor_log WHERE wo_id = (SELECT id FROM work_order WHERE wo_no = 'WO-2026-000009'));

ROLLBACK;  -- replace with COMMIT; once verified

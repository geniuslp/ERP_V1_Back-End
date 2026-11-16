-- Copy of WO-2026-000009 as WO-2026-000020: header + lines + cost codes + payment installments + penalties + retention.
-- work_order_line.amount is a generated column (qty/price/discount), so it is recalculated, not copied.
-- NOT copied: contractor-log bills, payment_log, attachments, edit log and status log (history / payment records / files).
-- Run in pgAdmin on the DB where WO-2026-000009 exists. The script ends with ROLLBACK - check the counts, then change it to COMMIT.
-- Note: wo_no normally comes from sequence wo_seq. If the next number the system hands out is 000020 it will clash with this
-- copy (unique wo_no); see the last query for the sequence position.

BEGIN;

-- before: source must exist (1 row) and the new number must be free (0 rows)
SELECT id, wo_no, status, contract_amount FROM work_order WHERE wo_no = 'WO-2026-000009';
SELECT id, wo_no FROM work_order WHERE wo_no = 'WO-2026-000020';

WITH src AS (
    SELECT id FROM work_order WHERE wo_no = 'WO-2026-000009'
), new_wo AS (
    INSERT INTO work_order (wo_no, wo_date, employer_name, project_code, project_scope_text, supplier_name, contact_person, supplier_address, supplier_phone, contract_type, work_system, contract_description, contract_amount, vat_rate, wht_rate, advance_pct, advance_amount, progress_payment_note, retention_pct, advance_deduct_pct, other_deduction_note, start_date, duration_days, end_date, penalty_pct_per_day, warranty_years, ref_no, other_terms, cost_code, status, entered_by, entered_at, section_head_id, section_head_signed_at, authorized_by, authorized_at, subcontractor_signed_name, subcontractor_signed_at, remarks, created_at, updated_at, created_by, updated_by, use_discount, discount_type, use_vat, use_wht, total_amount, discount_amount, vat_amount, wht_amount, net_amount, supplier_id, project_name_text, advance_deduct_amount, approver_id, has_penalty, warranty_note, pr_id)
    SELECT 'WO-2026-000020', w.wo_date, w.employer_name, w.project_code, w.project_scope_text, w.supplier_name, w.contact_person, w.supplier_address, w.supplier_phone, w.contract_type, w.work_system, w.contract_description, w.contract_amount, w.vat_rate, w.wht_rate, w.advance_pct, w.advance_amount, w.progress_payment_note, w.retention_pct, w.advance_deduct_pct, w.other_deduction_note, w.start_date, w.duration_days, w.end_date, w.penalty_pct_per_day, w.warranty_years, w.ref_no, w.other_terms, w.cost_code, w.status, w.entered_by, w.entered_at, w.section_head_id, w.section_head_signed_at, w.authorized_by, w.authorized_at, w.subcontractor_signed_name, w.subcontractor_signed_at, w.remarks, w.created_at, w.updated_at, w.created_by, w.updated_by, w.use_discount, w.discount_type, w.use_vat, w.use_wht, w.total_amount, w.discount_amount, w.vat_amount, w.wht_amount, w.net_amount, w.supplier_id, w.project_name_text, w.advance_deduct_amount, w.approver_id, w.has_penalty, w.warranty_note, w.pr_id
      FROM work_order w WHERE w.id = (SELECT id FROM src)
    RETURNING id
)
, c_work_order_line AS (
    INSERT INTO work_order_line (wo_id, sort_order, cost_code, description, qty, unit_price, disc, disc_type, vat_rate, wht_rate, created_at, created_by, remarks)
    SELECT (SELECT id FROM new_wo), sort_order, cost_code, description, qty, unit_price, disc, disc_type, vat_rate, wht_rate, created_at, created_by, remarks
      FROM work_order_line WHERE wo_id = (SELECT id FROM src)
    RETURNING 1
)
, c_work_order_cost_code AS (
    INSERT INTO work_order_cost_code (wo_id, cost_code, remarks, created_at, created_by)
    SELECT (SELECT id FROM new_wo), cost_code, remarks, created_at, created_by
      FROM work_order_cost_code WHERE wo_id = (SELECT id FROM src)
    RETURNING 1
)
, c_work_order_payment_installment AS (
    INSERT INTO work_order_payment_installment (wo_id, installment_no, description, percent_of_contract, amount, due_date, payment_status, paid_date, remarks, created_at, updated_at, created_by, updated_by)
    SELECT (SELECT id FROM new_wo), installment_no, description, percent_of_contract, amount, due_date, payment_status, paid_date, remarks, created_at, updated_at, created_by, updated_by
      FROM work_order_payment_installment WHERE wo_id = (SELECT id FROM src)
    RETURNING 1
)
, c_work_order_penalty AS (
    INSERT INTO work_order_penalty (wo_id, description, percent_per_day, contract_start_date, contract_end_date, remarks, created_at, updated_at, created_by, updated_by)
    SELECT (SELECT id FROM new_wo), description, percent_per_day, contract_start_date, contract_end_date, remarks, created_at, updated_at, created_by, updated_by
      FROM work_order_penalty WHERE wo_id = (SELECT id FROM src)
    RETURNING 1
)
, c_work_order_retention AS (
    INSERT INTO work_order_retention (wo_id, description, percent_of_contract, amount, remarks, created_at, updated_at, created_by, updated_by)
    SELECT (SELECT id FROM new_wo), description, percent_of_contract, amount, remarks, created_at, updated_at, created_by, updated_by
      FROM work_order_retention WHERE wo_id = (SELECT id FROM src)
    RETURNING 1
)
SELECT (SELECT id FROM new_wo) AS new_wo_id,
       (SELECT count(*) FROM c_work_order_line) AS lines_copied,
       (SELECT count(*) FROM c_work_order_cost_code) AS cost_codes_copied,
       (SELECT count(*) FROM c_work_order_payment_installment) AS installments_copied,
       (SELECT count(*) FROM c_work_order_penalty) AS penalties_copied,
       (SELECT count(*) FROM c_work_order_retention) AS retention_rows_copied;

-- after: the copy as stored
SELECT id, wo_no, status, contract_amount FROM work_order WHERE wo_no = 'WO-2026-000020';

-- sequence position (next WO number = last_value + 1 when is_called)
SELECT last_value, is_called FROM wo_seq;

ROLLBACK;  -- replace with COMMIT; once verified

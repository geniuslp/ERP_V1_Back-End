-- STEP 1: run this in the database where WO-2026-000009 EXISTS (pgAdmin, read only).
-- It returns the INSERT script for WO-2026-000020 - one statement per row, in order.
-- STEP 2: copy the whole result column (all rows) and run it in the TARGET database. It ends with ROLLBACK:
--         check the printed counts, then change the last line to COMMIT.
-- Copied: header, lines, cost codes, payment installments, penalties, retention.
-- NOT copied: contractor-log bills, payment_log, attachments, edit/status logs. work_order_line.amount is a generated column (recalculated).
-- The target DB must not already have WO-2026-000020 (wo_no is unique); its wo_seq may later hand out 20 and clash.

SELECT stmt FROM (
SELECT 0 AS ord, 'BEGIN;' AS stmt
UNION ALL SELECT 10, 'CREATE TEMP TABLE _new (id bigint) ON COMMIT DROP;'
UNION ALL
SELECT 20 AS ord, format('WITH x AS (INSERT INTO work_order (wo_no, wo_date, employer_name, project_code, project_scope_text, supplier_name, contact_person, supplier_address, supplier_phone, contract_type, work_system, contract_description, contract_amount, vat_rate, wht_rate, advance_pct, advance_amount, progress_payment_note, retention_pct, advance_deduct_pct, other_deduction_note, start_date, duration_days, end_date, penalty_pct_per_day, warranty_years, ref_no, other_terms, cost_code, status, entered_by, entered_at, section_head_id, section_head_signed_at, authorized_by, authorized_at, subcontractor_signed_name, subcontractor_signed_at, remarks, created_at, updated_at, created_by, updated_by, use_discount, discount_type, use_vat, use_wht, total_amount, discount_amount, vat_amount, wht_amount, net_amount, supplier_id, project_name_text, advance_deduct_amount, approver_id, has_penalty, warranty_note, pr_id) VALUES (%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L) RETURNING id) INSERT INTO _new SELECT id FROM x;', 'WO-2026-000020', w.wo_date, w.employer_name, w.project_code, w.project_scope_text, w.supplier_name, w.contact_person, w.supplier_address, w.supplier_phone, w.contract_type, w.work_system, w.contract_description, w.contract_amount, w.vat_rate, w.wht_rate, w.advance_pct, w.advance_amount, w.progress_payment_note, w.retention_pct, w.advance_deduct_pct, w.other_deduction_note, w.start_date, w.duration_days, w.end_date, w.penalty_pct_per_day, w.warranty_years, w.ref_no, w.other_terms, w.cost_code, w.status, w.entered_by, w.entered_at, w.section_head_id, w.section_head_signed_at, w.authorized_by, w.authorized_at, w.subcontractor_signed_name, w.subcontractor_signed_at, w.remarks, w.created_at, w.updated_at, w.created_by, w.updated_by, w.use_discount, w.discount_type, w.use_vat, w.use_wht, w.total_amount, w.discount_amount, w.vat_amount, w.wht_amount, w.net_amount, w.supplier_id, w.project_name_text, w.advance_deduct_amount, w.approver_id, w.has_penalty, w.warranty_note, w.pr_id) AS stmt
  FROM work_order w WHERE w.wo_no = 'WO-2026-000009'
UNION ALL
SELECT 30, format('INSERT INTO work_order_line (wo_id, sort_order, cost_code, description, qty, unit_price, disc, disc_type, vat_rate, wht_rate, created_at, created_by, remarks) VALUES ((SELECT id FROM _new), %L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L);', t.sort_order, t.cost_code, t.description, t.qty, t.unit_price, t.disc, t.disc_type, t.vat_rate, t.wht_rate, t.created_at, t.created_by, t.remarks)
  FROM work_order_line t JOIN work_order w ON w.id = t.wo_id WHERE w.wo_no = 'WO-2026-000009'
UNION ALL
SELECT 40, format('INSERT INTO work_order_cost_code (wo_id, cost_code, remarks, created_at, created_by) VALUES ((SELECT id FROM _new), %L,%L,%L,%L);', t.cost_code, t.remarks, t.created_at, t.created_by)
  FROM work_order_cost_code t JOIN work_order w ON w.id = t.wo_id WHERE w.wo_no = 'WO-2026-000009'
UNION ALL
SELECT 50, format('INSERT INTO work_order_payment_installment (wo_id, installment_no, description, percent_of_contract, amount, due_date, payment_status, paid_date, remarks, created_at, updated_at, created_by, updated_by) VALUES ((SELECT id FROM _new), %L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L,%L);', t.installment_no, t.description, t.percent_of_contract, t.amount, t.due_date, t.payment_status, t.paid_date, t.remarks, t.created_at, t.updated_at, t.created_by, t.updated_by)
  FROM work_order_payment_installment t JOIN work_order w ON w.id = t.wo_id WHERE w.wo_no = 'WO-2026-000009'
UNION ALL
SELECT 60, format('INSERT INTO work_order_penalty (wo_id, description, percent_per_day, contract_start_date, contract_end_date, remarks, created_at, updated_at, created_by, updated_by) VALUES ((SELECT id FROM _new), %L,%L,%L,%L,%L,%L,%L,%L,%L);', t.description, t.percent_per_day, t.contract_start_date, t.contract_end_date, t.remarks, t.created_at, t.updated_at, t.created_by, t.updated_by)
  FROM work_order_penalty t JOIN work_order w ON w.id = t.wo_id WHERE w.wo_no = 'WO-2026-000009'
UNION ALL
SELECT 70, format('INSERT INTO work_order_retention (wo_id, description, percent_of_contract, amount, remarks, created_at, updated_at, created_by, updated_by) VALUES ((SELECT id FROM _new), %L,%L,%L,%L,%L,%L,%L,%L);', t.description, t.percent_of_contract, t.amount, t.remarks, t.created_at, t.updated_at, t.created_by, t.updated_by)
  FROM work_order_retention t JOIN work_order w ON w.id = t.wo_id WHERE w.wo_no = 'WO-2026-000009'
UNION ALL SELECT 90, 'SELECT w.id, w.wo_no, w.status, w.contract_amount, (SELECT count(*) FROM work_order_line WHERE wo_id = w.id) AS lines, (SELECT count(*) FROM work_order_cost_code WHERE wo_id = w.id) AS cost_codes, (SELECT count(*) FROM work_order_payment_installment WHERE wo_id = w.id) AS installments, (SELECT count(*) FROM work_order_penalty WHERE wo_id = w.id) AS penalties, (SELECT count(*) FROM work_order_retention WHERE wo_id = w.id) AS retention FROM work_order w WHERE w.id = (SELECT id FROM _new);'
UNION ALL SELECT 100, 'ROLLBACK;  -- replace with COMMIT; once verified'
) s ORDER BY ord;

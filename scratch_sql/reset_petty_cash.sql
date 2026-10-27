-- RESET PETTY CASH DATA (เงินสดย่อย) - written 2026-10-27, NOT part of the migrations
--
-- DESTRUCTIVE AND IRREVERSIBLE. Take a backup first and check which database you are connected to:
--     SELECT current_database(), inet_server_addr();
--     pg_dump "$DATABASE_URL" -Fc -f ~/erp_db_before_pc_reset_$(date +%F).dump
--
-- Removes
--   petty_cash_requisition, petty_cash_requisition_line, petty_cash_attachment, petty_cash_status_log
--   the approval requests / logs of doc_type PETTY_CASH
--   the AP payments recorded for petty cash (payment_log / payment_installment with doc_type 'PC')
--   and restarts the ids and the petty cash running number (pc_number_counter) from 1.
--
-- Does NOT touch: Memo / PR / PO / WO / IC, master data, users, permissions, approval_config,
--   uploaded attachment FILES on disk (only their database rows go).
--   Petty cash writes nothing to stock (project_stock / stock_item), so no stock needs fixing.
--   Project paid / spent totals are computed from the petty cash rows live, so they drop to match.

BEGIN;

-- AP payments recorded against petty cash documents (installments point at the payment log, so they go first)
DELETE FROM public.payment_installment WHERE doc_type = 'PC';
DELETE FROM public.payment_log         WHERE doc_type = 'PC';

DELETE FROM public.approval_log     WHERE doc_type = 'PETTY_CASH';
DELETE FROM public.approval_request WHERE doc_type = 'PETTY_CASH';

-- No table outside this list references them, so TRUNCATE works (no CASCADE on purpose: if one does on your
-- database it fails and nothing is deleted - run ROLLBACK; and tell me which table it names)
TRUNCATE TABLE
    public.petty_cash_attachment, public.petty_cash_status_log, public.petty_cash_requisition_line,
    public.petty_cash_requisition,
    public.pc_number_counter
RESTART IDENTITY;

-- check before committing: every count below must be 0
SELECT 'petty_cash_requisition' t, count(*) FROM public.petty_cash_requisition
UNION ALL SELECT 'petty_cash_requisition_line', count(*) FROM public.petty_cash_requisition_line
UNION ALL SELECT 'payment_log (PC)', count(*) FROM public.payment_log WHERE doc_type = 'PC'
UNION ALL SELECT 'approval_request (PETTY_CASH)', count(*) FROM public.approval_request WHERE doc_type = 'PETTY_CASH';

COMMIT;   -- or ROLLBACK; if anything looks wrong

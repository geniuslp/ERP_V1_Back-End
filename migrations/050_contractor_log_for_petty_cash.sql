-- The contractor work log (bills / invoices / attachments) now also serves a petty cash requisition, so the finance
-- petty cash detail page looks and works like the PO one. The header row points at EXACTLY ONE of work order / PO /
-- petty cash; bills, lines, invoices and attachments hang off log_id and need no change.
-- Idempotent, own transaction.
BEGIN;

ALTER TABLE public.wo_contractor_log
    ADD COLUMN IF NOT EXISTS pc_id BIGINT REFERENCES public.petty_cash_requisition(id);
CREATE INDEX IF NOT EXISTS idx_wo_contractor_log_pc ON public.wo_contractor_log (pc_id);

-- 049's "wo_id xor po_id" check becomes "exactly one of wo_id / po_id / pc_id".
ALTER TABLE public.wo_contractor_log DROP CONSTRAINT IF EXISTS wo_contractor_log_one_doc_chk;
ALTER TABLE public.wo_contractor_log
    ADD CONSTRAINT wo_contractor_log_one_doc_chk
    CHECK ((wo_id IS NOT NULL)::int + (po_id IS NOT NULL)::int + (pc_id IS NOT NULL)::int = 1);

COMMIT;

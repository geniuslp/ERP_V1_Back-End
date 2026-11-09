-- The contractor work log (บันทึกงานของผู้รับเหมา: bills / invoices / attachments) now also serves a purchase order,
-- so the finance PO detail page works like the WO one. The header row points at EITHER a work order OR a PO;
-- bills, lines, invoices and attachments hang off log_id and need no change. Table keeps its wo_ name on purpose.
-- Idempotent, own transaction.
BEGIN;

ALTER TABLE public.wo_contractor_log ALTER COLUMN wo_id DROP NOT NULL;
ALTER TABLE public.wo_contractor_log
    ADD COLUMN IF NOT EXISTS po_id BIGINT REFERENCES public.purchase_order(id);
CREATE INDEX IF NOT EXISTS idx_wo_contractor_log_po ON public.wo_contractor_log (po_id);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'wo_contractor_log_one_doc_chk') THEN
        ALTER TABLE public.wo_contractor_log
            ADD CONSTRAINT wo_contractor_log_one_doc_chk CHECK ((wo_id IS NOT NULL) <> (po_id IS NOT NULL));
    END IF;
END $$;

COMMIT;

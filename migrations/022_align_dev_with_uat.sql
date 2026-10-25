-- Align dev with the UAT schema dump of 2026-10-25 (schema_snapshots/uat_schema_2026-10-25.sql).
-- Only the differences where UAT is the wider / more complete side. All changes widen a type or add an
-- index, so no existing value can be lost. Safe to re-run (ALTER ... TYPE to the same type is a no-op).
ALTER TABLE public.borrow              ALTER COLUMN project_code     TYPE varchar(50);
ALTER TABLE public.project             ALTER COLUMN warehouse_code   TYPE varchar(50);
ALTER TABLE public.requisition         ALTER COLUMN container_no     TYPE varchar(100);
ALTER TABLE public.stock_transfer_line ALTER COLUMN mat_code         TYPE varchar(30);
ALTER TABLE public.stock_transfer_line ALTER COLUMN unit             TYPE varchar(50);
ALTER TABLE public.memo                ALTER COLUMN delivery_location TYPE text;

ALTER TABLE public.ic_po_receive_document ALTER COLUMN score_ontime   TYPE numeric;
ALTER TABLE public.ic_po_receive_document ALTER COLUMN score_quality  TYPE numeric;
ALTER TABLE public.ic_po_receive_document ALTER COLUMN score_quantity TYPE numeric;
ALTER TABLE public.ic_po_receive_document ALTER COLUMN rated_at       TYPE timestamptz;

CREATE INDEX IF NOT EXISTS idx_stock_transfer_line_transfer ON public.stock_transfer_line USING btree (transfer_id);
CREATE INDEX IF NOT EXISTS idx_work_order_line_wo_id        ON public.work_order_line USING btree (wo_id);

-- Clean-break migration: supplier_code (string) -> supplier.id (integer FK)
-- across purchase_order, grn, rfq. work_order gets an ADDITIONAL, decoupled
-- supplier_id (nullable, no behavior change to its existing free-text
-- supplier_code/supplier_name/supplier_address/supplier_phone fields).
-- No backfill: old rows in purchase_order/grn/rfq get supplier_id = NULL.

BEGIN;

-- ── purchase_order ──────────────────────────────────────────────────────────
ALTER TABLE purchase_order
    ADD COLUMN IF NOT EXISTS supplier_id integer REFERENCES supplier(id);
CREATE INDEX IF NOT EXISTS idx_po_supplier_id ON purchase_order(supplier_id);

DROP INDEX IF EXISTS idx_po_supplier;
ALTER TABLE purchase_order DROP COLUMN IF EXISTS supplier_code;

-- ── grn ──────────────────────────────────────────────────────────────────────
ALTER TABLE grn
    ADD COLUMN IF NOT EXISTS supplier_id integer REFERENCES supplier(id);
CREATE INDEX IF NOT EXISTS idx_grn_supplier_id ON grn(supplier_id);

ALTER TABLE grn DROP COLUMN IF EXISTS supplier_code;

-- ── rfq ──────────────────────────────────────────────────────────────────────
ALTER TABLE rfq
    ADD COLUMN IF NOT EXISTS supplier_id integer REFERENCES supplier(id);
CREATE INDEX IF NOT EXISTS idx_rfq_supplier_id ON rfq(supplier_id);

ALTER TABLE rfq DROP COLUMN IF EXISTS supplier_code;

-- ── work_order ───────────────────────────────────────────────────────────────
-- Decoupled, optional link — supplier_code/supplier_name/supplier_address/
-- supplier_phone stay exactly as they are (free text, no FK, supplier_name
-- still NOT NULL and independently required). This new column is purely
-- additive for reporting/future use; nothing existing changes.
ALTER TABLE work_order
    ADD COLUMN IF NOT EXISTS supplier_id integer REFERENCES supplier(id);
CREATE INDEX IF NOT EXISTS idx_work_order_supplier_id ON work_order(supplier_id);

-- ── supplier ─────────────────────────────────────────────────────────────────
-- Now safe: no other table's FK still points at supplier.supplier_code.
ALTER TABLE supplier DROP COLUMN IF EXISTS supplier_code;

COMMIT;

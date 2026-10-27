-- WH01 is not a real warehouse (the warehouses are HO, BO, FAC-P, FAC-S) and is no longer used: every stock_item row
-- that still says 'WH01' belongs to FAC-S (owner's decision, 2026-10-27). The column default was 'WH01', so new stock
-- items (material import, auto-created items) would have gone back to WH01; it becomes 'FAC-S' too.
--
-- updated_at / updated_by are NOT changed on purpose: the stock pages sort by newest updated_at.
-- Idempotent. Other tables hold no WH01 (checked on dev). The backup table stock_item_qty_backup_20261011 is left as it is.
--
-- Undo (only while no real FAC-S rows exist besides these): UPDATE stock_item SET warehouse_code='WH01' WHERE ...; there is no
-- marker on the converted rows, so take a backup first on UAT / prod:  pg_dump "$DATABASE_URL" -Fc -f ~/erp_db_before_wh01.dump

BEGIN;

UPDATE public.stock_item SET warehouse_code = 'FAC-S' WHERE warehouse_code = 'WH01';

ALTER TABLE public.stock_item ALTER COLUMN warehouse_code SET DEFAULT 'FAC-S';

COMMIT;

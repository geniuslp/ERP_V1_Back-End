-- Found by the rehearsal diff (UAT dump + migrations 017-022 vs dev dump, 2026-10-25): three CHECK constraints
-- where UAT is ahead of dev. A regex diff of the dumps missed these; only a full text diff showed them.
-- Dev is made to match UAT (the wider list / the stricter check UAT already enforces).

-- order_type: UAT also allows office_equipment, asset_equipment, asset_tool (the OH order types the app sends).
ALTER TABLE public.purchase_order
    DROP CONSTRAINT IF EXISTS purchase_order_order_type_check,
    ADD CONSTRAINT purchase_order_order_type_check CHECK (order_type::text = ANY (ARRAY[
        'stock','cost','office_equipment','asset_equipment','asset_tool',
        'asset_office_equipment','asset_machine','asset_tools']));

ALTER TABLE public.purchase_request
    DROP CONSTRAINT IF EXISTS purchase_request_order_type_check,
    ADD CONSTRAINT purchase_request_order_type_check CHECK (order_type::text = ANY (ARRAY[
        'stock','cost','office_equipment','asset_equipment','asset_tool',
        'asset_office_equipment','asset_machine','asset_tools']));

-- Work order line withholding-tax rate: only 1 / 3 / 5 (or unset). Same rule as woAllowedWhtRates in work_order.go.
ALTER TABLE public.work_order_line DROP CONSTRAINT IF EXISTS chk_wol_wht_rate;
ALTER TABLE public.work_order_line
    ADD CONSTRAINT chk_wol_wht_rate CHECK (wht_rate IS NULL OR wht_rate = ANY (ARRAY[1::numeric, 3::numeric, 5::numeric]));

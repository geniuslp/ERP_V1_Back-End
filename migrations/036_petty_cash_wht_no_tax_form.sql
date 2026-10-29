-- Petty cash no longer asks for ภ.ง.ด.3 / ภ.ง.ด.53: the WHT type (ค่าขนส่ง / ค่าเช่า / ค่าจ้าง-ค่าบริการ)
-- is the only choice. tax_form_code stays as an optional column (old rows keep their value).
BEGIN;
ALTER TABLE petty_cash_requisition_line DROP CONSTRAINT IF EXISTS chk_pcl_wht_fields;
ALTER TABLE petty_cash_requisition_line ADD CONSTRAINT chk_pcl_wht_fields
  CHECK ((NOT has_wht) OR ((wht_income_type_id IS NOT NULL) AND (wht_rate IS NOT NULL)));
COMMIT;

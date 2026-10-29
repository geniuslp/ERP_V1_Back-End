-- Petty cash "ประเภทหัก ณ ที่จ่าย" dropdown is reduced to three choices:
--   ค่าขนส่ง 1%, ค่าเช่า 5%, ค่าจ้าง/ค่าบริการ 3%.
-- Rows are kept (existing lines may reference them); the others are only deactivated.
-- Lines on the old 3% types (CONTRACT / PROFESSIONAL / COMMISSION) are moved to SERVICE so they
-- still resolve to an active option with the same rate.
BEGIN;

UPDATE wht_income_type SET label_th = 'ค่าขนส่ง',            sort_order = 1 WHERE code = 'TRANSPORT';
UPDATE wht_income_type SET label_th = 'ค่าเช่า',             sort_order = 2 WHERE code = 'RENT';
UPDATE wht_income_type SET label_th = 'ค่าจ้าง/ค่าบริการ',   sort_order = 3 WHERE code = 'SERVICE';

UPDATE petty_cash_requisition_line
   SET wht_income_type_id = (SELECT id FROM wht_income_type WHERE code = 'SERVICE')
 WHERE wht_income_type_id IN (SELECT id FROM wht_income_type WHERE code IN ('CONTRACT', 'PROFESSIONAL', 'COMMISSION'));

UPDATE wht_income_type SET is_active = false
 WHERE code NOT IN ('TRANSPORT', 'RENT', 'SERVICE');

COMMIT;

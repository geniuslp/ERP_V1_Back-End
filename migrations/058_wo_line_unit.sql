-- Per-line "หน่วย" (unit) on a work order line: free text chosen from a fixed list in the UI (เหมา, งาน, รอบ,
-- ครั้ง, ตรม, เครื่อง). Additive and idempotent; existing lines get NULL (shown as blank / "-").
BEGIN;

ALTER TABLE work_order_line
    ADD COLUMN IF NOT EXISTS unit VARCHAR(20);

COMMIT;

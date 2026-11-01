-- Per-line "รายละเอียด" on a work order line (same as purchase_request_line.remarks): free text, optional.
-- Additive and idempotent; existing lines get NULL.
BEGIN;

ALTER TABLE work_order_line
    ADD COLUMN IF NOT EXISTS remarks TEXT;

COMMIT;

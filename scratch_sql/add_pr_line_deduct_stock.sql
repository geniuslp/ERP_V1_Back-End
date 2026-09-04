BEGIN;

ALTER TABLE purchase_request_line
    ADD COLUMN IF NOT EXISTS deduct_stock boolean NOT NULL DEFAULT true;

COMMIT;

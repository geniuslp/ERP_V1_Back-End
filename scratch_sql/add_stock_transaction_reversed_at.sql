BEGIN;

ALTER TABLE stock_transaction ADD COLUMN IF NOT EXISTS reversed_at timestamp NULL;

COMMIT;

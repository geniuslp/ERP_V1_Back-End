-- A work order can be created from an approved PR (pr_type PO_WO / WO_ONLY), like a PO is created from a PR.
-- work_order.pr_id keeps that link; nullable, so every existing WO and every WO created without a PR is unchanged.
-- Idempotent.
BEGIN;

ALTER TABLE work_order
    ADD COLUMN IF NOT EXISTS pr_id BIGINT REFERENCES purchase_request(id);

CREATE INDEX IF NOT EXISTS idx_work_order_pr_id ON work_order(pr_id) WHERE pr_id IS NOT NULL;

COMMIT;

-- Add approver_id to purchase_order, mirroring memo's per-document approver model:
-- a specific person is chosen from the role-eligible pool (GET /master/eligible-approvers)
-- at PO creation time, rather than any user holding the approving role being eligible.
ALTER TABLE purchase_order
    ADD COLUMN IF NOT EXISTS approver_id BIGINT REFERENCES users(id);

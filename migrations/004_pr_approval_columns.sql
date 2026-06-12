-- Add approver_id to purchase_request for approval tracking
ALTER TABLE purchase_request
    ADD COLUMN IF NOT EXISTS approver_id BIGINT REFERENCES users(id);

-- Rename created_at → uploaded_at in pr_attachment for semantic clarity
ALTER TABLE pr_attachment
    RENAME COLUMN created_at TO uploaded_at;

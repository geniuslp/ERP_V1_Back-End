-- Files attached to a work order itself (WO create/edit page, "ไฟล์แนบ" at the bottom of tab 2). Separate from
-- wo_contractor_log_attachment (files of the finance-side contractor work log); the log's attachments tab lists
-- these read-only. PDF / JPEG, 10 MB each (enforced by the API). Additive and idempotent.
BEGIN;

CREATE TABLE IF NOT EXISTS work_order_attachment (
    id          BIGSERIAL PRIMARY KEY,
    wo_id       BIGINT       NOT NULL REFERENCES work_order(id) ON DELETE CASCADE,
    file_name   VARCHAR(255) NOT NULL,
    file_path   TEXT         NOT NULL,
    file_size   BIGINT,
    file_type   VARCHAR(100),
    uploaded_by BIGINT       REFERENCES users(id),
    uploaded_at TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_work_order_attachment_wo_id ON work_order_attachment(wo_id);

COMMIT;

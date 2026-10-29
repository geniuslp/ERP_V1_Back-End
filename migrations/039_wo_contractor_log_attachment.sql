-- Files (PDF / JPEG) attached to a work order contractor log (บันทึกงานของผู้รับเหมาตามใบสั่งจ้าง).
BEGIN;

CREATE TABLE IF NOT EXISTS wo_contractor_log_attachment (
    id          bigserial PRIMARY KEY,
    log_id      bigint       NOT NULL REFERENCES wo_contractor_log(id) ON DELETE CASCADE,
    file_name   varchar(255) NOT NULL,
    file_path   text         NOT NULL,
    file_size   bigint,
    file_type   varchar(100),
    uploaded_by bigint,
    uploaded_at timestamp    NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wo_contractor_log_attachment_log ON wo_contractor_log_attachment (log_id);

COMMIT;

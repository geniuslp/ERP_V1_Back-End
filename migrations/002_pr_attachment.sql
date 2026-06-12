CREATE TABLE IF NOT EXISTS pr_attachment (
    id          BIGSERIAL    PRIMARY KEY,
    pr_id       BIGINT       NOT NULL REFERENCES purchase_request(pr_id) ON DELETE CASCADE,
    file_name   VARCHAR(255) NOT NULL,
    file_path   TEXT         NOT NULL,
    file_size   BIGINT       NOT NULL DEFAULT 0,
    file_type   VARCHAR(100) NOT NULL DEFAULT '',
    uploaded_by BIGINT       REFERENCES users(id),
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pr_attachment_pr_id ON pr_attachment(pr_id);

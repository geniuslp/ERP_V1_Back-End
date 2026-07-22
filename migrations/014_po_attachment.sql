CREATE TABLE IF NOT EXISTS po_attachment (
    id          BIGSERIAL    PRIMARY KEY,
    po_id       BIGINT       NOT NULL REFERENCES purchase_order(id) ON DELETE CASCADE,
    file_name   VARCHAR(255) NOT NULL,
    file_path   TEXT         NOT NULL,
    file_size   BIGINT       NOT NULL DEFAULT 0,
    file_type   VARCHAR(100) NOT NULL DEFAULT '',
    uploaded_by BIGINT       REFERENCES users(id),
    uploaded_at TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_po_attachment_po_id ON po_attachment(po_id);

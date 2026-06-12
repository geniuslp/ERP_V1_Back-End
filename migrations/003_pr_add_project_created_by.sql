ALTER TABLE purchase_request
    ADD COLUMN IF NOT EXISTS project_code VARCHAR(50),
    ADD COLUMN IF NOT EXISTS created_by   BIGINT REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS updated_by   BIGINT REFERENCES users(id);

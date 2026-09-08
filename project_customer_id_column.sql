BEGIN;

ALTER TABLE project ADD COLUMN IF NOT EXISTS customer_id BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'project_customer_id_fkey'
    ) THEN
        ALTER TABLE project
            ADD CONSTRAINT project_customer_id_fkey
            FOREIGN KEY (customer_id) REFERENCES customer (cus_id);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS project_customer_id_idx ON project (customer_id);

COMMIT;

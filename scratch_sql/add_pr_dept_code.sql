BEGIN;

ALTER TABLE purchase_request
    ADD COLUMN IF NOT EXISTS dept_code varchar(20) NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'purchase_request_dept_code_fkey'
    ) THEN
        ALTER TABLE purchase_request
            ADD CONSTRAINT purchase_request_dept_code_fkey
            FOREIGN KEY (dept_code) REFERENCES departments(dept_code);
    END IF;
END $$;

COMMIT;

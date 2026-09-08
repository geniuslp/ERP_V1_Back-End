BEGIN;

CREATE TABLE IF NOT EXISTS customer (
    cus_id        SERIAL PRIMARY KEY,
    customer_code VARCHAR(20)  NOT NULL,
    customer_name VARCHAR(200) NOT NULL,
    address       TEXT,
    contact       TEXT,
    credit_term   VARCHAR(50),
    remarks       TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by    BIGINT,
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS customer_code_uq ON customer (customer_code);
CREATE INDEX IF NOT EXISTS customer_name_idx ON customer (customer_name);

COMMIT;

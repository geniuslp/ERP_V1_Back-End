-- Payment tracking for PO/WO — polymorphic doc_type/doc_id, mirrors approval_log's
-- pattern (doc_id is a bare bigint, no DB-level FK — two possible target tables,
-- integrity enforced in Go, same as approval_log's callers already do).
-- Append-only, same as approval_log: no UPDATE/DELETE. Corrections are made by
-- inserting a negative reversal row (optionally pointing back via reverses_id)
-- followed by a new correct row.
-- Run separately, not via a migration file, per project convention.
BEGIN;

CREATE TABLE IF NOT EXISTS payment_log (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    doc_type    varchar(30)    NOT NULL,              -- 'PO' | 'WO'
    doc_id      bigint         NOT NULL,               -- purchase_order.id or work_order.id, app-enforced
    doc_no      varchar(30)    NOT NULL,               -- denormalized po_no/wo_no, mirrors approval_log.doc_no
    amount_paid numeric(18,2)  NOT NULL,               -- may be negative for reversal/refund rows
    paid_date   date           NOT NULL DEFAULT CURRENT_DATE,
    paid_by     bigint         NOT NULL,               -- FK -> users.id, who actually made the payment
    remark      text           NULL,
    reverses_id bigint         NULL,                   -- FK -> payment_log.id, optional pointer to the entry this reverses
    created_at  timestamp      NOT NULL DEFAULT now(),
    created_by  bigint         NOT NULL                -- FK -> users.id, who logged this entry
);

DO $$ BEGIN
  ALTER TABLE payment_log
    ADD CONSTRAINT payment_log_doc_type_check
    CHECK (doc_type IN ('PO','WO'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE payment_log
    ADD CONSTRAINT payment_log_amount_paid_check
    CHECK (amount_paid <> 0);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE payment_log
    ADD CONSTRAINT payment_log_paid_by_fkey
    FOREIGN KEY (paid_by) REFERENCES users(id);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE payment_log
    ADD CONSTRAINT payment_log_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE payment_log
    ADD CONSTRAINT payment_log_reverses_id_fkey
    FOREIGN KEY (reverses_id) REFERENCES payment_log(id);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- doc_type+doc_id is the hot lookup path (per-doc payment history, and the
-- project rollup join) — index it, same reasoning as approval_log's
-- (doc_type, doc_id) access pattern.
CREATE INDEX IF NOT EXISTS idx_payment_log_doc ON payment_log (doc_type, doc_id);

COMMIT;

-- Work order numbers become WO-YYYYMM-NNNN (monthly counter, like PO-YYYYMM-NNNN). Existing numbers
-- (WO-YYYY-NNNNNN, from wo_seq) are left untouched; only new work orders use the new format.
-- Idempotent. Dev: run directly. UAT/prod: run this file manually BEFORE deploying the matching API.

CREATE TABLE IF NOT EXISTS public.wo_number_counter (
    year_month character varying(6) NOT NULL PRIMARY KEY,
    last_seq   integer NOT NULL DEFAULT 0
);

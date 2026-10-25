-- บันทึกงานของผู้รับเหมาตามใบสั่งจ้าง, tab 2 ประเภทรับวางบิล: one bill per (log, type, installment).
-- Types: ADV (advance), PROGRESS, RETENTION. The 7 summary lines (สรุปรายการเบิก) live in
-- wo_contractor_log_bill_line; subtotal ("รวมเป็นเงิน (1)") and net_amount ("จ่ายสุทธิ") are recomputed by the
-- backend from those lines on every save. Credit term is read live from supplier.payment_terms (not stored
-- unless the user overrides it on the bill).
CREATE TABLE IF NOT EXISTS public.wo_contractor_log_bill (
    id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    log_id                  BIGINT        NOT NULL REFERENCES public.wo_contractor_log(id) ON DELETE CASCADE,
    bill_type               VARCHAR(20)   NOT NULL CHECK (bill_type IN ('ADV','PROGRESS','RETENTION')),
    installment_no          INTEGER,
    ap_voucher_no           VARCHAR(50),
    pay_to                  VARCHAR(200),
    credit_term_days        INTEGER,
    description             TEXT,
    progress_installment_no INTEGER,
    wht_type                VARCHAR(50),
    subtotal                NUMERIC(18,2) NOT NULL DEFAULT 0,
    net_amount              NUMERIC(18,2) NOT NULL DEFAULT 0,
    status                  VARCHAR(20)   NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','SAVED')),
    created_at              TIMESTAMP     NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMP     NOT NULL DEFAULT NOW(),
    created_by              BIGINT,
    updated_by              BIGINT
);
CREATE INDEX IF NOT EXISTS idx_wo_contractor_log_bill_log ON public.wo_contractor_log_bill (log_id);

CREATE TABLE IF NOT EXISTS public.wo_contractor_log_bill_line (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bill_id   BIGINT        NOT NULL REFERENCES public.wo_contractor_log_bill(id) ON DELETE CASCADE,
    line_code VARCHAR(20)   NOT NULL CHECK (line_code IN ('ADV','PROGRESS','ADV_DEDUCT','VAT','RETENTION','WHT','OTHER')),
    percent   NUMERIC(7,2),
    amount    NUMERIC(18,2) NOT NULL DEFAULT 0,
    UNIQUE (bill_id, line_code)
);

CREATE TABLE IF NOT EXISTS public.wo_contractor_log_bill_invoice (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bill_id       BIGINT       NOT NULL REFERENCES public.wo_contractor_log_bill(id) ON DELETE CASCADE,
    line_no       INTEGER      NOT NULL,
    invoice_no    VARCHAR(50),
    invoice_date  DATE,
    delivery_date DATE,
    bill_date     DATE,
    pay_date      DATE
);
CREATE INDEX IF NOT EXISTS idx_wo_contractor_log_bill_invoice_bill ON public.wo_contractor_log_bill_invoice (bill_id);

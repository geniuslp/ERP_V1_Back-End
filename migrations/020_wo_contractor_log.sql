-- บันทึกงานของผู้รับเหมาตามใบสั่งจ้าง (tab 1 ข้อมูลเอกสาร): one header per record that points at an
-- existing work order, plus the Contract / Advance / Retention figures the user types in.
-- Supplier, WO project and contract description are NOT stored: they are always read from
-- work_order at query time. project_code is the separate PJ dropdown (a project master pick).
CREATE TABLE IF NOT EXISTS public.wo_contractor_log (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    wo_id        BIGINT      NOT NULL REFERENCES public.work_order(id),
    project_code VARCHAR(20) REFERENCES public.project(project_code),
    created_at   TIMESTAMP   NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP   NOT NULL DEFAULT NOW(),
    created_by   BIGINT,
    updated_by   BIGINT
);
CREATE INDEX IF NOT EXISTS idx_wo_contractor_log_wo ON public.wo_contractor_log (wo_id);

CREATE TABLE IF NOT EXISTS public.wo_contractor_log_term (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    log_id     BIGINT        NOT NULL REFERENCES public.wo_contractor_log(id) ON DELETE CASCADE,
    item_type  VARCHAR(20)   NOT NULL CHECK (item_type IN ('CONTRACT','ADVANCE','RETENTION')),
    percent    NUMERIC(7,2),
    amount     NUMERIC(18,2),
    accumulate NUMERIC(18,2),
    balance    NUMERIC(18,2),
    created_at TIMESTAMP     NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP     NOT NULL DEFAULT NOW(),
    UNIQUE (log_id, item_type)
);

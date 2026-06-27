-- Memo module: reference document, no status/workflow

CREATE SEQUENCE IF NOT EXISTS memo_seq START 1;

CREATE TABLE IF NOT EXISTS public.memo (
    id            BIGSERIAL PRIMARY KEY,
    memo_no       VARCHAR(30) NOT NULL UNIQUE,
    title         VARCHAR(255) NOT NULL,
    project_code  VARCHAR(30) REFERENCES public.project(project_code),
    supplier_code VARCHAR(30) REFERENCES public.supplier(supplier_code),
    requested_by  BIGINT NOT NULL REFERENCES public.users(id),
    department    VARCHAR(100),
    note          TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by    BIGINT REFERENCES public.users(id),
    updated_by    BIGINT REFERENCES public.users(id)
);

CREATE TABLE IF NOT EXISTS public.memo_line (
    id              BIGSERIAL PRIMARY KEY,
    memo_id         BIGINT NOT NULL REFERENCES public.memo(id) ON DELETE CASCADE,
    line_no         INT NOT NULL,
    description     VARCHAR(255) NOT NULL,
    unit            VARCHAR(30) NOT NULL,
    quantity        NUMERIC(18,4) NOT NULL,
    estimated_price NUMERIC(18,4) NOT NULL DEFAULT 0,
    line_amount     NUMERIC(18,4) GENERATED ALWAYS AS (quantity * estimated_price) STORED,
    remark          VARCHAR(255)
);

CREATE INDEX IF NOT EXISTS idx_memo_line_memo_id ON public.memo_line(memo_id);
CREATE INDEX IF NOT EXISTS idx_memo_project_code ON public.memo(project_code);
CREATE INDEX IF NOT EXISTS idx_memo_created_at ON public.memo(created_at);

ALTER TABLE public.purchase_request ADD COLUMN IF NOT EXISTS memo_id BIGINT REFERENCES public.memo(id);


BEGIN;

CREATE TABLE IF NOT EXISTS public.memo_attachment (
    id          bigint NOT NULL GENERATED ALWAYS AS IDENTITY,
    memo_id     bigint NOT NULL,
    file_path   text NOT NULL,
    file_name   character varying(255) NOT NULL,
    file_size   bigint NOT NULL,
    file_type   character varying(20),
    uploaded_by bigint,
    uploaded_at timestamp without time zone NOT NULL DEFAULT now(),

    CONSTRAINT memo_attachment_pkey     PRIMARY KEY (id),
    CONSTRAINT memo_attachment_memo_fk  FOREIGN KEY (memo_id) REFERENCES public.memo(id) ON DELETE CASCADE,
    CONSTRAINT memo_attachment_user_fk  FOREIGN KEY (uploaded_by) REFERENCES public.users(id)
);

CREATE INDEX IF NOT EXISTS idx_memo_attachment_memo_id ON public.memo_attachment(memo_id);

COMMENT ON TABLE public.memo_attachment IS 'ไฟล์แนบของ Memo — รูปภาพ, Excel, CSV, PDF';

COMMIT;

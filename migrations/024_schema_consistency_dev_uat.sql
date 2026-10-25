-- Last schema differences between dev and UAT (full text diff of the pgAdmin dumps, 2026-10-25).
-- Idempotent and safe to run on BOTH databases: each statement is a no-op where it is already true.

-- 1. chk_ic_po_receive_scores existed only on dev. The API already rejects scores outside 1-5
--    (goods_receipt.go), and the UAT data has 0 violating rows, so UAT gets the constraint.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_ic_po_receive_scores') THEN
        ALTER TABLE public.ic_po_receive_document ADD CONSTRAINT chk_ic_po_receive_scores CHECK (
            (score_quality  IS NULL OR (score_quality  >= 1 AND score_quality  <= 5)) AND
            (score_quantity IS NULL OR (score_quantity >= 1 AND score_quantity <= 5)) AND
            (score_ontime   IS NULL OR (score_ontime   >= 1 AND score_ontime   <= 5)));
    END IF;
END $$;

-- 2. cost_* foreign keys were NOT VALID on dev (UAT: validated). Dev has 0 orphan rows, so validate them.
--    VALIDATE CONSTRAINT on an already valid constraint does nothing.
ALTER TABLE public.cost_group    VALIDATE CONSTRAINT cost_group_job_id_fkey;
ALTER TABLE public.cost_job      VALIDATE CONSTRAINT cost_job_subject_id_fkey;
ALTER TABLE public.cost_subgroup VALIDATE CONSTRAINT cost_subgroup_group_id_fkey;

-- 3. stock_transfer*.id is GENERATED ALWAYS AS IDENTITY on UAT but a sequence default on dev. Convert dev
--    (the new identity continues after the current max id). Does nothing where it is already an identity.
DO $$
DECLARE
    t text;
    next_id bigint;
BEGIN
    FOREACH t IN ARRAY ARRAY['stock_transfer', 'stock_transfer_line', 'stock_transfer_status_log'] LOOP
        IF (SELECT is_identity FROM information_schema.columns
             WHERE table_schema = 'public' AND table_name = t AND column_name = 'id') = 'NO' THEN
            EXECUTE format('SELECT COALESCE(MAX(id), 0) + 1 FROM public.%I', t) INTO next_id;
            EXECUTE format('ALTER TABLE public.%I ALTER COLUMN id DROP DEFAULT', t);
            EXECUTE format('DROP SEQUENCE IF EXISTS public.%I', t || '_id_seq');
            EXECUTE format('ALTER TABLE public.%I ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (START WITH %s)', t, next_id);
        END IF;
    END LOOP;
END $$;

-- 4. Table comment that only dev had.
COMMENT ON TABLE public.work_order_cost_code IS
    'DEPRECATED as of the WO line-item rework — superseded by work_order_line. Not dropped yet; confirm before removing.';

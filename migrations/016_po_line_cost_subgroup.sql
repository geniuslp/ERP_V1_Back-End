-- Link purchase_order_line to the cost_subgroup -> cost_group -> cost_job chain,
-- mirroring purchase_request_line.cost_subgroup_id, so a job (and its job_code) can be
-- assigned per PO line. Additive and backward compatible: existing rows get NULL.
ALTER TABLE public.purchase_order_line
    ADD COLUMN IF NOT EXISTS cost_subgroup_id BIGINT REFERENCES public.cost_subgroup(id);

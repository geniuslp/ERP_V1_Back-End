-- WO section 8: 8.1 penalty is now a yes/no flag (fixed 1.00% of contract value per day,
-- shown as static text), 8.2 performance guarantee is free text instead of warranty_years.
-- Additive: old rows get has_penalty=false / warranty_note=NULL; warranty_years and
-- penalty_pct_per_day stay in place (legacy readers keep working).
ALTER TABLE public.work_order
    ADD COLUMN IF NOT EXISTS has_penalty   BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS warranty_note TEXT;

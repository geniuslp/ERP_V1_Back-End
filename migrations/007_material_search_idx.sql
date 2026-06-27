-- Speed up the type-ahead material search (GET /api/v1/materials/search)
-- Plain btree indexes help prefix (ILIKE 'q%') ranking and joins.
CREATE INDEX IF NOT EXISTS idx_material_code_code
    ON public.material_code (mat_code);

CREATE INDEX IF NOT EXISTS idx_mat_name_name
    ON public.mat_name (mat_name);

-- Trigram indexes make the substring ILIKE '%q%' filter fast on large datasets.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_material_code_trgm
    ON public.material_code USING gin (mat_code gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_mat_name_trgm
    ON public.mat_name USING gin (mat_name gin_trgm_ops);

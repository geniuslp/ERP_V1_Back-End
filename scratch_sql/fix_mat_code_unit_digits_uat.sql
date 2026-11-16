-- ONE-OFF data fix for UAT (run by the user; never run by Claude).
-- Material PG06003008000030 was created with a 3-digit unit segment ("000"); the unit segment is 2 digits,
-- so the correct code is PG0600300800030. This moves the master row and EVERY row that uses the code.
--
-- How: material_code has only UNIQUE(mat_code) + PK(id) and every other table points at mat_code (3 of the
-- FKs are not deferrable), so the code cannot simply be UPDATEd in place. The script (1) inserts the new
-- master row as a copy of the old one, (2) repoints every base table that has a mat_code column, (3) deletes
-- the old master row, (4) gives the new row the old id back (nothing references material_code.id).
-- Everything runs in ONE transaction and aborts (nothing changes) if the old code is missing or the new
-- code already exists.
--
-- Usage: run the PREVIEW block first and check it; then run the FIX block. It ends with ROLLBACK - after the
-- NOTICE lines look right, change that last ROLLBACK to COMMIT and run it again.

-- ── PREVIEW (read-only) ────────────────────────────────────────────────────────────────────────────
SELECT 'material_code' AS tbl, mat_code, id, unit_id FROM material_code
 WHERE mat_code IN ('PG06003008000030', 'PG0600300800030');

SELECT c.table_name,
       (xpath('/row/n/text()', query_to_xml(format('SELECT count(*) AS n FROM public.%I WHERE mat_code = %L', c.table_name, 'PG06003008000030'), false, true, '')))[1]::text::int AS rows_old,
       (xpath('/row/n/text()', query_to_xml(format('SELECT count(*) AS n FROM public.%I WHERE mat_code = %L', c.table_name, 'PG0600300800030'),  false, true, '')))[1]::text::int AS rows_new
  FROM information_schema.columns c
  JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
 WHERE c.table_schema = 'public' AND c.column_name = 'mat_code' AND c.table_name <> 'material_code'
 ORDER BY c.table_name;

-- ── FIX ────────────────────────────────────────────────────────────────────────────────────────────
BEGIN;

DO $$
DECLARE
  old_code text := 'PG06003008000030';
  new_code text := 'PG0600300800030';
  old_id   int;
  t        record;
  n        bigint;
BEGIN
  SELECT id INTO old_id FROM material_code WHERE mat_code = old_code;
  IF old_id IS NULL THEN
    RAISE EXCEPTION 'old code % not found in material_code - nothing changed', old_code;
  END IF;
  IF EXISTS (SELECT 1 FROM material_code WHERE mat_code = new_code) THEN
    RAISE EXCEPTION 'new code % already exists in material_code - merge by hand, nothing changed', new_code;
  END IF;

  INSERT INTO material_code
         (mat_code, group_id, subgroup_id, mat_name_id, spec_id, brand_id, unit_id, is_active,
          created_at, updated_at, created_by, updated_by, cost_subgroup_id)
  SELECT  new_code, group_id, subgroup_id, mat_name_id, spec_id, brand_id, unit_id, is_active,
          created_at, NOW(),       created_by, updated_by, cost_subgroup_id
    FROM material_code WHERE id = old_id;

  FOR t IN
    SELECT c.table_name
      FROM information_schema.columns c
      JOIN information_schema.tables tb ON tb.table_schema = c.table_schema AND tb.table_name = c.table_name AND tb.table_type = 'BASE TABLE'
     WHERE c.table_schema = 'public' AND c.column_name = 'mat_code' AND c.table_name <> 'material_code'
     ORDER BY c.table_name
  LOOP
    EXECUTE format('UPDATE public.%I SET mat_code = $1 WHERE mat_code = $2', t.table_name) USING new_code, old_code;
    GET DIAGNOSTICS n = ROW_COUNT;
    IF n > 0 THEN RAISE NOTICE '%: % row(s) moved', t.table_name, n; END IF;
  END LOOP;

  DELETE FROM material_code WHERE id = old_id;
  UPDATE material_code SET id = old_id WHERE mat_code = new_code;
  RAISE NOTICE 'material_code % -> % (id % kept)', old_code, new_code, old_id;
END $$;

-- Verify inside the transaction: the old code must be gone everywhere, the new one present.
SELECT 'material_code' AS tbl, mat_code, id FROM material_code
 WHERE mat_code IN ('PG06003008000030', 'PG0600300800030');

ROLLBACK;  -- change to COMMIT once the NOTICE lines and the SELECT above look right

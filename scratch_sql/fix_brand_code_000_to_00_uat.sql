-- ONE-OFF data fix for UAT (run by the user; never run by Claude). Run AFTER fix_mat_code_unit_digits_uat.sql.
-- A material code is: P + subgroup(3) + mat_name(3) + spec(3) + BRAND(2) + unit(3). PG06003008000030 was built
-- from a brand whose brand_code is '000' (3 digits); brand codes are 2 digits, so the correct code is
-- PG0600300800030 (brand '00'). The first script already corrected the material code and every document;
-- this one corrects the brand row it came from, so the brand master matches the code.
--
-- Changes only brand.brand_code ('000' -> '00') of the brand used by material PG0600300800030.
-- brand has UNIQUE (brand_code, spec_id), and a brand is shared by every material under it, so the script
-- aborts (changes nothing) when: the brand row is not '000', another brand of the same spec already has '00',
-- or another material also uses this brand (its code would then need the same correction - decide by hand).
--
-- Usage: PREVIEW first, then FIX (ends with ROLLBACK; change to COMMIT once the NOTICE lines look right).

-- ── PREVIEW (read-only) ────────────────────────────────────────────────────────────────────────────
SELECT m.mat_code, b.id AS brand_id, b.brand_code, b.brand_name, b.spec_id,
       (SELECT count(*) FROM material_code x WHERE x.brand_id = b.id) AS materials_on_this_brand,
       EXISTS (SELECT 1 FROM brand o WHERE o.spec_id = b.spec_id AND o.brand_code = '00' AND o.id <> b.id) AS brand_00_already_exists
  FROM material_code m
  JOIN brand b ON b.id = m.brand_id
 WHERE m.mat_code = 'PG0600300800030';

-- ── FIX ────────────────────────────────────────────────────────────────────────────────────────────
BEGIN;

DO $$
DECLARE
  the_code text := 'PG0600300800030';
  b        record;
  others   bigint;
BEGIN
  SELECT br.id, br.brand_code, br.spec_id INTO b
    FROM material_code m JOIN brand br ON br.id = m.brand_id
   WHERE m.mat_code = the_code;
  IF b.id IS NULL THEN
    RAISE EXCEPTION 'material % not found - run fix_mat_code_unit_digits_uat.sql first; nothing changed', the_code;
  END IF;
  IF b.brand_code <> '000' THEN
    RAISE EXCEPTION 'brand % has brand_code % (expected 000) - nothing changed', b.id, b.brand_code;
  END IF;
  IF EXISTS (SELECT 1 FROM brand o WHERE o.spec_id = b.spec_id AND o.brand_code = '00' AND o.id <> b.id) THEN
    RAISE EXCEPTION 'spec % already has a brand with code 00 - merge by hand, nothing changed', b.spec_id;
  END IF;
  SELECT count(*) INTO others FROM material_code WHERE brand_id = b.id AND mat_code <> the_code;
  IF others > 0 THEN
    RAISE EXCEPTION 'brand % is also used by % other material(s) - decide by hand, nothing changed', b.id, others;
  END IF;

  UPDATE brand SET brand_code = '00', updated_at = NOW() WHERE id = b.id;
  RAISE NOTICE 'brand id % : brand_code 000 -> 00', b.id;
END $$;

-- Verify inside the transaction.
SELECT m.mat_code, b.brand_code, b.brand_name
  FROM material_code m JOIN brand b ON b.id = m.brand_id
 WHERE m.mat_code = 'PG0600300800030';

ROLLBACK;  -- change to COMMIT once the NOTICE line and the SELECT above look right

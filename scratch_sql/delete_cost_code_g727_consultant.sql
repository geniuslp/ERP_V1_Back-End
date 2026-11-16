-- Delete the G (General Code) cost code 727 "ค่าที่ปรึกษา" (subject OH, job G, group 800, subgroup 727).
-- Only that one cost_subgroup row is deleted; the group, the job and every other subgroup stay.
-- Run on UAT / prod by hand. Safe by design:
--   * it only matches code 727 under group 800 of G whose NAME contains "ที่ปรึกษา" (on dev, 727 is "ค่าเดินทางไกล" and is NOT touched);
--   * it refuses (rolls back with an error) if the cost code is already used by any document / stock / budget row,
--     because every table below references cost_subgroup.id.
-- Step 1 (read-only): see what will be deleted.
SELECT sg.id, cs.subject_code, j.job_code, g.group_code, sg.subgroup_code, sg.subgroup_name, sg.is_active
FROM cost_subgroup sg
JOIN cost_group g    ON g.id = sg.group_id
JOIN cost_job j      ON j.id = g.job_id
JOIN cost_subject cs ON cs.id = j.subject_id
WHERE cs.subject_code = 'OH' AND j.job_code = 'G' AND g.group_code = '800'
  AND sg.subgroup_code = '727' AND sg.subgroup_name ILIKE '%ที่ปรึกษา%';

-- Step 2: delete (all-or-nothing).
BEGIN;

DO $$
DECLARE
    v_id bigint;
    v_n  int;
    v_used text := '';
BEGIN
    SELECT count(*), max(sg.id) INTO v_n, v_id
    FROM cost_subgroup sg
    JOIN cost_group g    ON g.id = sg.group_id
    JOIN cost_job j      ON j.id = g.job_id
    JOIN cost_subject cs ON cs.id = j.subject_id
    WHERE cs.subject_code = 'OH' AND j.job_code = 'G' AND g.group_code = '800'
      AND sg.subgroup_code = '727' AND sg.subgroup_name ILIKE '%ที่ปรึกษา%';

    IF v_n = 0 THEN
        RAISE NOTICE 'nothing to delete: no G 727 cost code with a name containing "ที่ปรึกษา"';
        RETURN;
    END IF;
    IF v_n > 1 THEN
        RAISE EXCEPTION 'expected exactly 1 matching cost code, found %', v_n;
    END IF;

    IF EXISTS (SELECT 1 FROM purchase_order_line        WHERE cost_subgroup_id = v_id) THEN v_used := v_used || ' purchase_order_line'; END IF;
    IF EXISTS (SELECT 1 FROM purchase_request_line      WHERE cost_subgroup_id = v_id) THEN v_used := v_used || ' purchase_request_line'; END IF;
    IF EXISTS (SELECT 1 FROM petty_cash_requisition_line WHERE cost_subgroup_id = v_id) THEN v_used := v_used || ' petty_cash_requisition_line'; END IF;
    IF EXISTS (SELECT 1 FROM project_cost_budget        WHERE cost_subgroup_id = v_id) THEN v_used := v_used || ' project_cost_budget'; END IF;
    IF EXISTS (SELECT 1 FROM material_code              WHERE cost_subgroup_id = v_id) THEN v_used := v_used || ' material_code'; END IF;
    IF EXISTS (SELECT 1 FROM ic_project_cost_item       WHERE cost_subgroup_id = v_id) THEN v_used := v_used || ' ic_project_cost_item'; END IF;
    IF EXISTS (SELECT 1 FROM ic_project_receipt_pool    WHERE cost_subgroup_id = v_id) THEN v_used := v_used || ' ic_project_receipt_pool'; END IF;
    IF EXISTS (SELECT 1 FROM ic_po_return_line          WHERE cost_subgroup_id = v_id) THEN v_used := v_used || ' ic_po_return_line'; END IF;
    IF EXISTS (SELECT 1 FROM ic_project_movement_line   WHERE cost_subgroup_id = v_id OR to_cost_subgroup_id = v_id) THEN v_used := v_used || ' ic_project_movement_line'; END IF;

    IF v_used <> '' THEN
        RAISE EXCEPTION 'cost code id % is already used in:% — not deleted (set it inactive instead)', v_id, v_used;
    END IF;

    DELETE FROM cost_subgroup WHERE id = v_id;
    RAISE NOTICE 'deleted cost_subgroup id %', v_id;
END $$;

COMMIT;

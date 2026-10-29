-- TRANSFER documents posted from a warehouse-linked project recorded unit_cost = 0 (the destination's
-- brand-new cost item had no cost). Re-price those rows from the warehouse stock item's cost so the
-- transferred value shows up in Budget Cost. Only rows still at 0 are touched (idempotent).
BEGIN;

UPDATE ic_project_cost_item_transaction t
   SET unit_cost = si.unit_cost
  FROM ic_project_movement_line l
  JOIN ic_project_movement m ON m.id = l.movement_id AND m.doc_type = 'TRANSFER' AND m.status = 'POSTED'
  JOIN project p ON p.project_code = m.project_code AND p.linked_warehouse_code IS NOT NULL
  JOIN stock_item si ON si.mat_code = l.mat_code
 WHERE t.movement_line_id = l.id
   AND COALESCE(t.unit_cost, 0) = 0
   AND COALESCE(si.unit_cost, 0) > 0;

UPDATE ic_project_cost_item ici
   SET last_unit_cost = x.unit_cost
  FROM (
    SELECT DISTINCT ON (t.project_cost_item_id) t.project_cost_item_id, t.unit_cost
      FROM ic_project_cost_item_transaction t
     WHERE t.ref_type = 'MOVEMENT_TRANSFER' AND t.qty > 0 AND t.unit_cost > 0
     ORDER BY t.project_cost_item_id, t.id DESC
  ) x
 WHERE ici.id = x.project_cost_item_id AND COALESCE(ici.last_unit_cost, 0) = 0;

COMMIT;

-- work_order.retention_pct is what the WO detail page and the printed WO show as "7.3 หักเงินประกันผลงาน". The form no
-- longer sends it (the choice ไม่มี / 5% / 10% is stored as a work_order_retention row), so every WO kept the old column
-- default of 5.00 and printed "5%" whatever the user chose. From now on the API keeps it equal to the first retention row
-- (0 = ไม่มี) every time the payment conditions are saved; this backfills the existing documents the same way.
-- Idempotent; touches only retention_pct. Run BEFORE deploying the matching API.
BEGIN;

UPDATE work_order w
   SET retention_pct = COALESCE((SELECT r.percent_of_contract
                                   FROM work_order_retention r
                                  WHERE r.wo_id = w.id
                                  ORDER BY r.id ASC
                                  LIMIT 1), 0)
 WHERE w.retention_pct IS DISTINCT FROM COALESCE((SELECT r.percent_of_contract
                                                    FROM work_order_retention r
                                                   WHERE r.wo_id = w.id
                                                   ORDER BY r.id ASC
                                                   LIMIT 1), 0);

COMMIT;

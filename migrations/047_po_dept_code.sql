-- Department of a purchase order. Filled from the linked PR's dept_code (PO form auto-fills it); free choice when no PR / the PR has none.
-- Same shape as purchase_request.dept_code: nullable, no FK. Idempotent, run in its own transaction.
BEGIN;

ALTER TABLE purchase_order ADD COLUMN IF NOT EXISTS dept_code VARCHAR(20);

-- Existing POs: department of the PR they were created from (pr_id) ...
UPDATE purchase_order po
   SET dept_code = pr.dept_code
  FROM purchase_request pr
 WHERE pr.id = po.pr_id
   AND po.dept_code IS NULL
   AND NULLIF(TRIM(pr.dept_code), '') IS NOT NULL;

-- ... or, for POs without pr_id, of the PR their lines were split from (a PR never spans two departments).
UPDATE purchase_order po
   SET dept_code = x.dept_code
  FROM (
        SELECT pol.po_id, MIN(pr.dept_code) AS dept_code
          FROM purchase_order_line pol
          JOIN purchase_request_line prl ON prl.id = pol.pr_line_id
          JOIN purchase_request pr ON pr.id = prl.pr_id
         WHERE NULLIF(TRIM(pr.dept_code), '') IS NOT NULL
         GROUP BY pol.po_id
       ) x
 WHERE x.po_id = po.id
   AND po.dept_code IS NULL;

CREATE INDEX IF NOT EXISTS idx_purchase_order_dept_code ON purchase_order (dept_code);

COMMIT;

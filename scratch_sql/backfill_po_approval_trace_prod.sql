-- Backfill the approval trace of POs that were approved through the legacy PUT /po/:id/approve
-- (it only flipped purchase_order.status, so po_status_log / approval_log / approval_request kept no approval).
-- Inferred, NOT original data:
--   approved by = purchase_order.updated_by, or the chosen approver_id when updated_by is empty (PO-202610-0001..0004)
--   approved at = purchase_order.updated_at
-- Rows are tagged in remarks so they can be told apart (and deleted) later. Safe to re-run: skips POs that already have an APPROVED log row.
-- Run in pgAdmin on PROD. Check the preview and the "after" counts first; the script ends with ROLLBACK -
-- change it to COMMIT only after the numbers look right.

BEGIN;

-- before: the POs that will get a row (expect 17)
SELECT po.po_no, po.status, po.approver_id, po.updated_by,
       COALESCE(po.updated_by, po.approver_id) AS will_log_as_user, po.updated_at AS will_log_at
FROM purchase_order po
WHERE po.status IN ('APPROVED','SENT','PARTIALLY_RECEIVED','RECEIVED')
  AND NOT EXISTS (SELECT 1 FROM po_status_log s WHERE s.po_id = po.id AND s.to_status = 'APPROVED')
  AND NOT EXISTS (SELECT 1 FROM approval_log l WHERE l.doc_type = 'PO' AND l.doc_id = po.id AND l.action = 'APPROVE')
ORDER BY po.created_at;

-- 1) the approval row that the history page reads
INSERT INTO po_status_log (po_id, from_status, to_status, changed_by, changed_at, remarks)
SELECT po.id, 'PENDING_APPROVAL', 'APPROVED', COALESCE(po.updated_by, po.approver_id), po.updated_at,
       'backfilled approval (inferred from purchase_order.updated_by/updated_at)'
FROM purchase_order po
WHERE po.status IN ('APPROVED','SENT','PARTIALLY_RECEIVED','RECEIVED')
  AND NOT EXISTS (SELECT 1 FROM po_status_log s WHERE s.po_id = po.id AND s.to_status = 'APPROVED')
  AND NOT EXISTS (SELECT 1 FROM approval_log l WHERE l.doc_type = 'PO' AND l.doc_id = po.id AND l.action = 'APPROVE');

-- 2) close the approval requests that stayed PENDING although the PO is approved
UPDATE approval_request r
   SET status = 'APPROVED', updated_at = po.updated_at
  FROM purchase_order po
 WHERE r.doc_type = 'PO' AND r.doc_id = po.id AND r.status = 'PENDING'
   AND po.status IN ('APPROVED','SENT','PARTIALLY_RECEIVED','RECEIVED');

-- after: 17 backfilled rows, none left without a trace, no approved PO still pending in approval_request
SELECT count(*) AS backfilled_rows FROM po_status_log WHERE remarks LIKE 'backfilled approval%';
SELECT count(*) AS still_without_trace
FROM purchase_order po
WHERE po.status IN ('APPROVED','SENT','PARTIALLY_RECEIVED','RECEIVED')
  AND NOT EXISTS (SELECT 1 FROM po_status_log s WHERE s.po_id = po.id AND s.to_status = 'APPROVED')
  AND NOT EXISTS (SELECT 1 FROM approval_log l WHERE l.doc_type = 'PO' AND l.doc_id = po.id AND l.action = 'APPROVE');
SELECT count(*) AS approved_but_request_pending
FROM approval_request r JOIN purchase_order po ON po.id = r.doc_id
WHERE r.doc_type = 'PO' AND r.status = 'PENDING' AND po.status IN ('APPROVED','SENT','PARTIALLY_RECEIVED','RECEIVED');

ROLLBACK;  -- replace with COMMIT; once verified

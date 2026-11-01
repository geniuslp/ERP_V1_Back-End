-- "แก้ไขล่าสุด" of a purchase order = the last real edit of its content (PUT /po/:id, PUT /po/:id/edit-approved).
-- purchase_order.updated_at / updated_by cannot say that: approve, reject, cancel and goods-receipt updates all
-- bump them too, and approving must not count as an edit. These two columns are written only by real edits.
-- Backfill: the latest po_edit_log row (edit after approval) per PO. Edits of DRAFT POs were never logged, so
-- older POs stay NULL (shown as "-"). Additive and idempotent.
BEGIN;

ALTER TABLE purchase_order
    ADD COLUMN IF NOT EXISTS last_edited_by BIGINT REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS last_edited_at TIMESTAMP;

UPDATE purchase_order po
   SET last_edited_by = l.edited_by,
       last_edited_at = l.edited_at
  FROM (
        SELECT DISTINCT ON (po_id) po_id, edited_by, edited_at
          FROM po_edit_log
         ORDER BY po_id, edited_at DESC, id DESC
       ) l
 WHERE l.po_id = po.id
   AND po.last_edited_at IS NULL;

COMMIT;

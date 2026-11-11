-- A "waiting for your approval" notification (type APPROVAL_REQUEST) is removed as soon as it is no longer needed:
--   * the request leaves PENDING (approved / rejected / cancelled)  -> every approver's notification of that document goes
--   * the request is re-assigned while still PENDING                -> the previous approver's notification goes
-- The result notifications (APPROVED / REJECTED to the requester, FINANCE_PAYABLE) are kept.
-- Each removal is also pg_notify'd as {"action":"remove","id":..,"user_id":..} so open bells drop the row live.
-- Needs 053_notification.sql. Idempotent. Dev: run directly. UAT/prod: run this file manually.

CREATE OR REPLACE FUNCTION public.fn_clear_approval_notification() RETURNS trigger AS $$
DECLARE
    r record;
BEGIN
    IF NOT ((OLD.status = 'PENDING' AND NEW.status <> 'PENDING')
         OR (NEW.status = 'PENDING' AND OLD.assigned_to IS DISTINCT FROM NEW.assigned_to)) THEN
        RETURN NEW;
    END IF;

    FOR r IN
        DELETE FROM public.notification
         WHERE type = 'APPROVAL_REQUEST'
           AND doc_type = NEW.doc_type
           AND doc_id = NEW.doc_id
           AND (NEW.status <> 'PENDING' OR user_id IS DISTINCT FROM NEW.assigned_to)
        RETURNING id, user_id
    LOOP
        PERFORM pg_notify('erp_notification',
            json_build_object('action', 'remove', 'id', r.id, 'user_id', r.user_id)::text);
    END LOOP;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_clear_approval_notification ON public.approval_request;
CREATE TRIGGER trg_clear_approval_notification
    AFTER UPDATE OF status, assigned_to ON public.approval_request
    FOR EACH ROW EXECUTE FUNCTION public.fn_clear_approval_notification();

-- Notifications that were left behind before this trigger existed: requests whose approval is no longer pending.
DELETE FROM public.notification n
 WHERE n.type = 'APPROVAL_REQUEST'
   AND NOT EXISTS (SELECT 1 FROM public.approval_request ar
                    WHERE ar.doc_type = n.doc_type AND ar.doc_id = n.doc_id
                      AND ar.assigned_to = n.user_id AND ar.status = 'PENDING');

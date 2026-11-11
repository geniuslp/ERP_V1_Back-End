-- In-app notifications (bell) delivered live over WebSocket.
-- Rows are written by a trigger on approval_request, so every code path that sends / approves / rejects a document
-- (Memo, PR, PO, WO, petty cash) is covered without touching the handlers:
--   APPROVAL_REQUEST  -> the assigned approver: a document is waiting for them
--   APPROVED/REJECTED -> the requester: their document was decided
--   FINANCE_PAYABLE   -> users who may open MENU_FINANCE_PAYMENTS: a PO / WO / petty cash was approved (ready to pay)
-- The trigger also pg_notify('erp_notification', {id, user_id}); the API LISTENs on that channel and pushes the row
-- to the user's open sockets. Idempotent. Dev: run through cmd/migrate. UAT/prod: run this file manually.

CREATE TABLE IF NOT EXISTS public.notification (
    id          bigserial PRIMARY KEY,
    user_id     bigint       NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    type        varchar(30)  NOT NULL,
    title       varchar(200) NOT NULL,
    message     text,
    doc_type    varchar(30),
    doc_id      bigint,
    doc_no      varchar(50),
    link        varchar(300),
    is_read     boolean      NOT NULL DEFAULT false,
    created_at  timestamp    NOT NULL DEFAULT now(),
    read_at     timestamp
);
CREATE INDEX IF NOT EXISTS idx_notification_user ON public.notification (user_id, is_read, created_at DESC);

-- Insert one notification and wake the API (delivered when the surrounding transaction commits).
CREATE OR REPLACE FUNCTION public.fn_push_notification(
    p_user_id bigint, p_type varchar, p_title varchar, p_message text,
    p_doc_type varchar, p_doc_id bigint, p_doc_no varchar, p_link varchar
) RETURNS void AS $$
DECLARE
    v_id bigint;
BEGIN
    IF p_user_id IS NULL THEN
        RETURN;
    END IF;
    INSERT INTO public.notification (user_id, type, title, message, doc_type, doc_id, doc_no, link)
    VALUES (p_user_id, p_type, p_title, p_message, p_doc_type, p_doc_id, p_doc_no, p_link)
    RETURNING id INTO v_id;
    PERFORM pg_notify('erp_notification', json_build_object('id', v_id, 'user_id', p_user_id)::text);
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION public.fn_notify_approval_request() RETURNS trigger AS $$
DECLARE
    v_label     text;
    v_open      text;   -- link for the approver (opens the page with approve / reject buttons)
    v_view      text;   -- link for everyone else (plain detail page)
    v_fin_type  text;
    r           record;
BEGIN
    v_label := CASE NEW.doc_type
        WHEN 'PR'         THEN 'ใบขอซื้อ (PR)'
        WHEN 'PO'         THEN 'ใบสั่งซื้อ (PO)'
        WHEN 'MEMO'       THEN 'ใบบันทึกขอซื้อ (Memo)'
        WHEN 'WO'         THEN 'ใบสั่งจ้าง (WO)'
        WHEN 'PETTY_CASH' THEN 'ใบเบิกเงินสดย่อย'
        ELSE NULL END;
    IF v_label IS NULL THEN
        RETURN NEW;   -- other doc types are not part of the bell
    END IF;

    v_open := CASE NEW.doc_type
        WHEN 'PR'         THEN '/pr/' || NEW.doc_id || '?for_approval=true'
        WHEN 'PO'         THEN '/po/approval/' || NEW.doc_id || '?for_approval=true'
        WHEN 'MEMO'       THEN '/memo/' || NEW.doc_id || '/approval-detail'
        WHEN 'WO'         THEN '/work-order/' || NEW.doc_id || '?for_approval=true'
        WHEN 'PETTY_CASH' THEN '/petty-cash/' || NEW.doc_id END;
    v_view := CASE NEW.doc_type
        WHEN 'PR'         THEN '/pr/' || NEW.doc_id
        WHEN 'PO'         THEN '/po/approval/' || NEW.doc_id
        WHEN 'MEMO'       THEN '/memo/' || NEW.doc_id
        WHEN 'WO'         THEN '/work-order/' || NEW.doc_id
        WHEN 'PETTY_CASH' THEN '/petty-cash/' || NEW.doc_id END;

    -- 1) a document is waiting for the assigned approver (first request, re-sent, or re-assigned)
    IF NEW.status = 'PENDING' AND NEW.assigned_to IS NOT NULL
       AND (TG_OP = 'INSERT' OR OLD.status IS DISTINCT FROM 'PENDING' OR OLD.assigned_to IS DISTINCT FROM NEW.assigned_to) THEN
        PERFORM public.fn_push_notification(NEW.assigned_to, 'APPROVAL_REQUEST', 'มีเอกสารรออนุมัติ',
            v_label || ' ' || NEW.doc_no || ' รอคุณอนุมัติ',
            NEW.doc_type, NEW.doc_id, NEW.doc_no, v_open);
    END IF;

    -- 2) the decision, to whoever sent the document
    IF TG_OP = 'UPDATE' AND NEW.status IN ('APPROVED', 'REJECTED') AND OLD.status IS DISTINCT FROM NEW.status THEN
        PERFORM public.fn_push_notification(NEW.requested_by, NEW.status,
            CASE NEW.status WHEN 'APPROVED' THEN 'เอกสารของคุณได้รับการอนุมัติ' ELSE 'เอกสารของคุณไม่ได้รับการอนุมัติ' END,
            v_label || ' ' || NEW.doc_no || CASE NEW.status WHEN 'APPROVED' THEN ' ได้รับการอนุมัติแล้ว' ELSE ' ไม่ได้รับการอนุมัติ' END,
            NEW.doc_type, NEW.doc_id, NEW.doc_no, v_view);

        -- 3) approved PO / WO / petty cash -> ready for the finance team to pay
        IF NEW.status = 'APPROVED' AND NEW.doc_type IN ('PO', 'WO', 'PETTY_CASH') THEN
            v_fin_type := CASE NEW.doc_type WHEN 'PETTY_CASH' THEN 'PC' ELSE NEW.doc_type END;
            FOR r IN
                SELECT u.id
                  FROM public.users u
                  JOIN public.menus m ON m.menu_code = 'MENU_FINANCE_PAYMENTS'
                 WHERE u.is_active = true
                   AND COALESCE(
                         (SELECT up.can_read FROM public.user_menu_permissions up
                           WHERE up.user_id = u.id AND up.menu_id = m.id),
                         COALESCE((SELECT bool_or(rp.can_read) FROM public.user_roles ur
                                     JOIN public.role_menu_permissions rp ON rp.role_id = ur.role_id AND rp.menu_id = m.id
                                    WHERE ur.user_id = u.id), false)
                         OR COALESCE((SELECT dp.can_read FROM public.dept_menu_permissions dp
                                       WHERE dp.dept_code = u.dept_code AND dp.menu_id = m.id), false)
                       )
            LOOP
                PERFORM public.fn_push_notification(r.id, 'FINANCE_PAYABLE', 'มีรายการรอจ่ายเงิน',
                    v_label || ' ' || NEW.doc_no || ' อนุมัติแล้ว พร้อมบันทึกการจ่ายเงิน',
                    NEW.doc_type, NEW.doc_id, NEW.doc_no,
                    '/finance/payments/' || v_fin_type || '/' || NEW.doc_id);
            END LOOP;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_notify_approval_request ON public.approval_request;
CREATE TRIGGER trg_notify_approval_request
    AFTER INSERT OR UPDATE OF status, assigned_to ON public.approval_request
    FOR EACH ROW EXECUTE FUNCTION public.fn_notify_approval_request();

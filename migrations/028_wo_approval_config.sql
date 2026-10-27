-- Work order (WO) approval was never configured on dev or UAT: no 'WO' row in approval_doc_types and no
-- approver roles in approval_config. So GET /master/eligible-approvers?doc_type=WO returned nobody, the WO form
-- could not pick an approver, and submit failed ("approval flow not configured").
--
-- 1. Register the doc type so the generic approve/reject route (PUT /approval/WO/:id/approve|reject) can run.
--    work_order_status_log has the same shape as po_status_log.
-- 2. Approver roles = a copy of PO's roles in the SAME database (per the owner: "same as PO"). WO is the
--    "pick a specific approver from the eligible pool" model like PO, so the pool is every active user holding one of
--    these roles, plus approval_delegation rows (doc_type 'WO' or NULL = all).
-- Idempotent: each statement does nothing once WO rows exist. Change the roles later in the approval config screen.
INSERT INTO public.approval_doc_types
    (doc_type, doc_label, is_active, table_name, id_column, status_column, approved_value, rejected_value,
     status_log_table, status_log_fk_column)
SELECT 'WO', 'หนังสือสั่งจ้าง', true, 'work_order', 'id', 'status', 'APPROVED', 'REJECTED',
       'work_order_status_log', 'wo_id'
WHERE NOT EXISTS (SELECT 1 FROM public.approval_doc_types WHERE doc_type = 'WO');

INSERT INTO public.approval_config (doc_type, step_no, step_name, approver_role_id, min_amount, max_amount, is_active)
SELECT 'WO', step_no, step_name, approver_role_id, min_amount, max_amount, is_active
FROM public.approval_config
WHERE doc_type = 'PO'
  AND NOT EXISTS (SELECT 1 FROM public.approval_config WHERE doc_type = 'WO');

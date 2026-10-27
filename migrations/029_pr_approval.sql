-- PR approval (business rule changed 2026-10-26: a PR is no longer COMPLETED on submit; it goes to an approver first).
-- Flow: DRAFT -> PENDING_APPROVAL -> COMPLETED (approved: stock is reserved at this point) | REJECTED.
-- COMPLETED keeps its meaning for everything downstream (PO from PR, lists, history), so only two statuses are new.
-- Existing PRs are untouched (already COMPLETED / DRAFT).

-- 1. statuses + the approver chosen at submit (same "pick one person from the eligible pool" model as PO / Memo / WO)
ALTER TABLE public.purchase_request DROP CONSTRAINT IF EXISTS pr_status_check;
ALTER TABLE public.purchase_request ADD CONSTRAINT pr_status_check CHECK (status::text = ANY (ARRAY[
    'DRAFT','PENDING_APPROVAL','REJECTED','COMPLETED','STOCK_CHECK','PARTIALLY_FILLED','FULFILLED','CANCELLED']));
ALTER TABLE public.purchase_request ADD COLUMN IF NOT EXISTS approver_id BIGINT REFERENCES public.users(id);

-- 2. register PR in the generic approval flow (also makes it appear in the Approval Matrix page).
--    approved_value = COMPLETED: the generic approve route flips the status to it; the PR-specific stock reservation
--    runs in a post-approve hook (pr.go). pr_status_log has the same shape as po_status_log.
INSERT INTO public.approval_doc_types
    (doc_type, doc_label, is_active, table_name, id_column, status_column, approved_value, rejected_value,
     status_log_table, status_log_fk_column)
SELECT 'PR', 'ใบขอซื้อ', true, 'purchase_request', 'id', 'status', 'COMPLETED', 'REJECTED', 'pr_status_log', 'pr_id'
WHERE NOT EXISTS (SELECT 1 FROM public.approval_doc_types WHERE doc_type = 'PR');

-- 3. approver roles = a copy of PO's roles in the same database (owner's choice, as for WO). Edit later in the matrix.
INSERT INTO public.approval_config (doc_type, step_no, step_name, approver_role_id, min_amount, max_amount, is_active)
SELECT 'PR', step_no, step_name, approver_role_id, min_amount, max_amount, is_active
FROM public.approval_config
WHERE doc_type = 'PO'
  AND NOT EXISTS (SELECT 1 FROM public.approval_config WHERE doc_type = 'PR');

-- 4. menu "อนุมัติใบขอซื้อ" under the PR group (permissions are granted in the permission matrix, none created here)
INSERT INTO public.menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
SELECT p.id, 'MENU_PR_APPROVAL', 'อนุมัติใบขอซื้อ', '/pr/approval',
       (SELECT icon_name FROM public.menus WHERE menu_code = 'MENU_PO_APPROVAL'), 4, true
FROM public.menus p
WHERE p.menu_code = 'MENU_PR'
  AND NOT EXISTS (SELECT 1 FROM public.menus x WHERE x.menu_code = 'MENU_PR_APPROVAL');

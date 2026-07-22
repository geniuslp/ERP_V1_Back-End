-- Generic Approve/Reject: extend approval_doc_types with the table/column mapping needed
-- to drive UPDATE + status-log-insert generically instead of per-doc-type Go handlers.
--
-- NOTE: table_name/id_column/status_column/approved_value/rejected_value already existed
-- live on approval_doc_types (added ad hoc in an earlier session, not tracked in migrations
-- until now) with different names than originally proposed (approved_value/rejected_value,
-- not approved_status_value/rejected_status_value). This migration only adds what was
-- missing: status_log_table and status_log_fk_column (needed because po_status_log and
-- memo_status_log are identical in shape except for their FK column name).
ALTER TABLE public.approval_doc_types ADD COLUMN IF NOT EXISTS table_name VARCHAR(50);
ALTER TABLE public.approval_doc_types ADD COLUMN IF NOT EXISTS id_column VARCHAR(50) DEFAULT 'id';
ALTER TABLE public.approval_doc_types ADD COLUMN IF NOT EXISTS status_column VARCHAR(50) DEFAULT 'status';
ALTER TABLE public.approval_doc_types ADD COLUMN IF NOT EXISTS approved_value VARCHAR(30) DEFAULT 'APPROVED';
ALTER TABLE public.approval_doc_types ADD COLUMN IF NOT EXISTS rejected_value VARCHAR(30) DEFAULT 'REJECTED';
ALTER TABLE public.approval_doc_types ADD COLUMN IF NOT EXISTS status_log_table VARCHAR(50);
ALTER TABLE public.approval_doc_types ADD COLUMN IF NOT EXISTS status_log_fk_column VARCHAR(50);

UPDATE public.approval_doc_types
   SET table_name = 'purchase_order', status_log_table = 'po_status_log', status_log_fk_column = 'po_id'
 WHERE doc_type = 'PO';

UPDATE public.approval_doc_types
   SET table_name = 'memo', status_log_table = 'memo_status_log', status_log_fk_column = 'memo_id'
 WHERE doc_type = 'MEMO';

-- GRN intentionally NOT configured here — see generic_approval.go comment: GRN has no
-- approval_request-gated approve/reject flow in the current codebase (only a single-actor
-- Confirm action with no approval_config/approval_request involvement), so there is nothing
-- to migrate to the generic path for it.

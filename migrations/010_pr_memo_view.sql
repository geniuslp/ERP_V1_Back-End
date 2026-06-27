-- Expose memo link on v_pr_full for PR list

CREATE OR REPLACE VIEW public.v_pr_full AS
SELECT
    pr.pr_id,
    pr.pr_no,
    pr.pr_date,
    pr.required_date::text,
    pr.status          AS pr_status,
    pr.priority,
    u.full_name        AS requested_by_name,
    l.location_name,
    l.location_type,
    w.warehouse_name,
    (SELECT COUNT(*) FROM public.purchase_request_line prl WHERE prl.pr_id = pr.pr_id) AS line_count,
    pr.remarks,
    pr.requested_by,
    pr.created_at,
    pr.memo_id,
    m.memo_no,
    m.title             AS memo_title
FROM public.purchase_request pr
JOIN public.users u ON u.id = pr.requested_by
JOIN public.location l ON l.location_code = pr.location_code
LEFT JOIN public.warehouse w ON w.warehouse_code = pr.warehouse_code
LEFT JOIN public.memo m ON m.id = pr.memo_id;

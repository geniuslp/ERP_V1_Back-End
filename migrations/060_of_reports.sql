-- OF reports: new submenu "รายงาน OF" under the OF group (MENU_PR) with 4 report pages, plus the history table
-- for the first real one, "รายการการลดยอด PR" (reduce the requested qty of PR lines that are not fully ordered yet).
-- Only the PR reduction page is implemented; the other three menus are listed but disabled in the UI for now.
-- Permissions are not created here: grant them in the permission matrix (same as MENU_PR_APPROVAL). Safe to re-run.

-- 1. history of every reduction (the PR line itself only keeps the new qty_requested)
CREATE TABLE IF NOT EXISTS public.pr_line_reduction (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    pr_id         BIGINT        NOT NULL REFERENCES public.purchase_request(id),
    pr_line_id    BIGINT        NOT NULL REFERENCES public.purchase_request_line(id),
    mat_code      VARCHAR(20)   NOT NULL,
    qty_before    NUMERIC(18,4) NOT NULL,
    qty_decrement NUMERIC(18,4) NOT NULL CHECK (qty_decrement > 0),
    qty_after     NUMERIC(18,4) NOT NULL CHECK (qty_after > 0),
    reason        TEXT          NOT NULL CHECK (length(btrim(reason)) > 0),
    reduced_by    BIGINT        NOT NULL REFERENCES public.users(id),
    reduced_at    TIMESTAMP     NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_pr_line_reduction_pr   ON public.pr_line_reduction (pr_id);
CREATE INDEX IF NOT EXISTS idx_pr_line_reduction_line ON public.pr_line_reduction (pr_line_id);

-- 2. menus: group "รายงาน OF" (no path, it only holds the 4 pages) under the OF group, then the 4 pages
INSERT INTO public.menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
SELECT p.id, 'MENU_OF_REPORT', 'รายงาน OF', NULL, NULL, 7, true
FROM public.menus p
WHERE p.menu_code = 'MENU_PR'
  AND NOT EXISTS (SELECT 1 FROM public.menus x WHERE x.menu_code = 'MENU_OF_REPORT');

INSERT INTO public.menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
SELECT g.id, v.menu_code, v.menu_name, v.menu_path, NULL, v.sort_order, true
FROM public.menus g
CROSS JOIN (VALUES
    ('MENU_OF_RPT_PR_ITEMS',   'รายการงานใบขอซื้อ PR',                  '/reports/of/pr-items',   1),
    ('MENU_OF_RPT_PETTY_CASH', 'รายงานใบเบิกเงินสดย่อย',                '/reports/of/petty-cash', 2),
    ('MENU_OF_RPT_DELETED',    'รายงานรายการที่ถูกลบ PR / Petty Cash',  '/reports/of/deleted',    3),
    ('MENU_OF_RPT_PR_REDUCE',  'รายการการลดยอด PR',                     '/reports/of/pr-reduce',  4)
) AS v(menu_code, menu_name, menu_path, sort_order)
WHERE g.menu_code = 'MENU_OF_REPORT'
  AND NOT EXISTS (SELECT 1 FROM public.menus x WHERE x.menu_code = v.menu_code);

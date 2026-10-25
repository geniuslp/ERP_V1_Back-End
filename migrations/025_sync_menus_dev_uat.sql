-- Menu rows must be the same on dev and UAT. Compared BY menu_code (ids differ between the two databases).
-- Differences found on 2026-10-25 (besides MENU_WO_CONTRACTOR_LOG / Budget Cost order, see 018-019):
-- Idempotent; no permission rows are created or changed (grant the new menu in the permission matrix).

-- Finance group order: dev 7, UAT 9.
UPDATE public.menus SET sort_order = 7, updated_at = NOW()
WHERE menu_code = 'MENU_FINANCE' AND sort_order IS DISTINCT FROM 7;

-- Old WO group: dev renamed it and switched it off (its children moved under MENU_PO); UAT still had it active.
UPDATE public.menus SET menu_name = 'หนังสือสั่งจ้าง (WO)', is_active = false, updated_at = NOW()
WHERE menu_code = 'MENU_WO' AND (menu_name IS DISTINCT FROM 'หนังสือสั่งจ้าง (WO)' OR is_active IS DISTINCT FROM false);

-- Missing on UAT.
INSERT INTO public.menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
SELECT p.id, 'MENU_WH_REQUISITION_APPROVAL', 'ตรวจสอบ/อนุมัติใบเบิก', '/stock/wh-requisition-approval',
       'CheckSquareOutlined', 10, true
FROM public.menus p
WHERE p.menu_code = 'MENU_STOCK'
  AND NOT EXISTS (SELECT 1 FROM public.menus x WHERE x.menu_code = 'MENU_WH_REQUISITION_APPROVAL');

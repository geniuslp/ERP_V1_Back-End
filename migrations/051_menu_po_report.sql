-- Sub menu under MENU_PO ("PO/WO"): รายงาน (รายงานประวัติใบสั่ง / รายการค้างรับสินค้า).
-- Idempotent. Grant access afterwards in the permission matrix (role / user / department).
INSERT INTO public.menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
SELECT p.id, 'MENU_PO_REPORT', 'รายงานการสั่งซื้อ', '/po/report', NULL,
       COALESCE((SELECT MAX(c.sort_order) FROM public.menus c WHERE c.parent_id = p.id), 0) + 1, true
FROM public.menus p
WHERE p.menu_code = 'MENU_PO'
  AND NOT EXISTS (SELECT 1 FROM public.menus x WHERE x.menu_code = 'MENU_PO_REPORT');

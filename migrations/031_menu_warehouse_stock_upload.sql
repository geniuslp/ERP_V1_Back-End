-- New page: warehouse stock upload (Excel, Update / Add), in the Master group (MENU_MASTER, "Master ข้อมูลหลัก") as its
-- last item. Idempotent. Grant access afterwards in the permission matrix (role / user / department).
INSERT INTO public.menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
SELECT p.id, 'MENU_MASTER_WAREHOUSE_STOCK', 'คลังสินค้า (อัปโหลด Excel)', '/master/warehouse-stock', 'InboxOutlined',
       COALESCE((SELECT MAX(c.sort_order) FROM public.menus c WHERE c.parent_id = p.id), 0) + 1, true
FROM public.menus p
WHERE p.menu_code = 'MENU_MASTER'
  AND NOT EXISTS (SELECT 1 FROM public.menus x WHERE x.menu_code = 'MENU_MASTER_WAREHOUSE_STOCK');

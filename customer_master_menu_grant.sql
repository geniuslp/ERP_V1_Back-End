-- Run AFTER customer_master_schema.sql (table creation) has been confirmed.

BEGIN;

INSERT INTO menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
VALUES (21, 'MENU_MASTER_CUSTOMER', 'ลูกค้า (Customer)', '/master/customer', 'TeamOutlined', 8, true)
ON CONFLICT (menu_code) DO NOTHING;

INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id
FROM roles r
CROSS JOIN menus m
WHERE m.menu_code = 'MENU_MASTER_CUSTOMER'
  AND r.role_code IN ('ADMIN', 'PURCHASING', 'SENIOR_TEAM')
ON CONFLICT (role_id, menu_id) DO NOTHING;

INSERT INTO role_menu_permissions (role_id, menu_id, can_read, can_write, can_update, can_delete)
SELECT r.id, m.id,
       true,
       true,
       true,
       (r.role_code = 'ADMIN' OR r.role_code = 'SENIOR_TEAM')
FROM roles r
CROSS JOIN menus m
WHERE m.menu_code = 'MENU_MASTER_CUSTOMER'
  AND r.role_code IN ('ADMIN', 'PURCHASING', 'SENIOR_TEAM')
ON CONFLICT (role_id, menu_id) DO NOTHING;

COMMIT;

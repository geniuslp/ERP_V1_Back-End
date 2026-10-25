-- Move MENU_WO_CONTRACTOR_LOG one step up, ahead of MENU_PO_COST_BUDGET (Budget Cost).
UPDATE public.menus SET sort_order = 9,  updated_at = NOW() WHERE menu_code = 'MENU_WO_CONTRACTOR_LOG';
UPDATE public.menus SET sort_order = 10, updated_at = NOW() WHERE menu_code = 'MENU_PO_COST_BUDGET';

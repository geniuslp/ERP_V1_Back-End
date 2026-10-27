-- The contractor work log no longer has its own menu: it is opened from the finance payment tracking page
-- (การเงิน > ติดตามการจ่ายเงิน > WO > detail). Remove the menu added by 018.
-- Every permission table references menus(id) ON DELETE CASCADE, so its permission rows go with it.
-- Idempotent: deleting a missing row does nothing.
DELETE FROM public.menus WHERE menu_code = 'MENU_WO_CONTRACTOR_LOG';

-- 019 moved Budget Cost from 9 to 10 to make room for the removed menu; put it back (UAT originally had 9).
UPDATE public.menus SET sort_order = 9, updated_at = NOW()
WHERE menu_code = 'MENU_PO_COST_BUDGET' AND sort_order IS DISTINCT FROM 9;

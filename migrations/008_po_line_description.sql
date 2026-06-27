-- Per-line free-text description for purchase order lines.
-- One-to-one with the line row; kept separate from `remarks` so the two meanings don't collide.
-- Additive and backward compatible: existing rows get description = NULL.
ALTER TABLE public.purchase_order_line ADD COLUMN IF NOT EXISTS description text;

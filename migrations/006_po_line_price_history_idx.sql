-- Speed up the LATERAL price-history lookup in GET /pr/:id/lines-with-po-status
-- (purchase_order_line filtered by mat_code + status, joined to purchase_order ordered by po_date DESC)
CREATE INDEX IF NOT EXISTS idx_po_line_matcode_status
    ON public.purchase_order_line (mat_code, status);

CREATE INDEX IF NOT EXISTS idx_po_date
    ON public.purchase_order (po_date DESC);

-- 061: a PR line may be reduced all the way to 0 (it stays on the document and prints with qty 0).
-- Before: purchase_request_line.qty_requested had to be > 0, so a line could never be fully reduced.
-- Re-runnable.

ALTER TABLE public.purchase_request_line
    DROP CONSTRAINT IF EXISTS purchase_request_line_qty_requested_check;
ALTER TABLE public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_qty_requested_check CHECK (qty_requested >= 0);

ALTER TABLE public.pr_line_reduction
    DROP CONSTRAINT IF EXISTS pr_line_reduction_qty_after_check;
ALTER TABLE public.pr_line_reduction
    ADD CONSTRAINT pr_line_reduction_qty_after_check CHECK (qty_after >= 0);

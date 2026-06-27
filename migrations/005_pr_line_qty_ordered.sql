-- Track cumulative quantity claimed by purchase_order_line rows against each PR line
ALTER TABLE purchase_request_line
    ADD COLUMN IF NOT EXISTS qty_ordered NUMERIC(18,4) NOT NULL DEFAULT 0;

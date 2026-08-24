-- Work Order line-item table + header discount/VAT/WHT flags.
-- Mirrors purchase_order / purchase_order_line's real structure (verified via
-- information_schema.columns / check_constraints — see chat report).
-- Run separately, not via a migration file, per project convention.
BEGIN;

-- 1. Header-level flags on work_order (mirrors purchase_order.use_discount/discount_type/use_vat/use_wht)
ALTER TABLE work_order
  ADD COLUMN IF NOT EXISTS use_discount   boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS discount_type  varchar(10) NOT NULL DEFAULT 'pct',
  ADD COLUMN IF NOT EXISTS use_vat        boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS use_wht        boolean NOT NULL DEFAULT false;

DO $$ BEGIN
  ALTER TABLE work_order
    ADD CONSTRAINT wo_discount_type_check
    CHECK (discount_type IN ('pct','amt'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Header aggregate totals (mirrors purchase_order.total_amount/discount_amount/vat_amount/wht_amount/net_amount)
ALTER TABLE work_order
  ADD COLUMN IF NOT EXISTS total_amount    numeric(18,4) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS discount_amount numeric(18,4) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS vat_amount      numeric(18,4) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS wht_amount      numeric(18,4) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS net_amount      numeric(18,4) NOT NULL DEFAULT 0;

-- Legacy single-value vat_rate/wht_rate columns are left in place, untouched by
-- new code — same "deprecated, don't drop" treatment as work_order.cost_code.

-- 2. Line items table (mirrors purchase_order_line's structure; cost_code replaces mat_code)
CREATE TABLE IF NOT EXISTS work_order_line (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  wo_id        bigint NOT NULL REFERENCES work_order(id),
  sort_order   integer NOT NULL,
  cost_code    varchar(50) NOT NULL,
  description  text,
  qty          numeric(18,4) NOT NULL CHECK (qty > 0),
  unit_price   numeric(18,4) NOT NULL CHECK (unit_price >= 0),
  amount       numeric(18,4) GENERATED ALWAYS AS (qty * unit_price) STORED,
  disc         numeric(18,4) NOT NULL DEFAULT 0,
  disc_type    varchar(10) NOT NULL DEFAULT 'pct' CHECK (disc_type IN ('pct','amt')),
  vat_rate     numeric(5,2),
  wht_rate     numeric(5,2) CHECK (wht_rate IS NULL OR wht_rate = ANY (ARRAY[1,3,5]::numeric[])),
  created_at   timestamp NOT NULL DEFAULT now(),
  created_by   bigint NOT NULL
);

-- 3. Deprecate (not drop) the earlier simple multi-select cost-code table.
COMMENT ON TABLE work_order_cost_code IS
  'DEPRECATED as of the WO line-item rework — superseded by work_order_line. Not dropped yet; confirm before removing.';

COMMIT;

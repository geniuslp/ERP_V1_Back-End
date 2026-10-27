-- Petty cash requisition lines get a unit (หน่วย). Free text like stock_item.unit / the unit master's unit_name,
-- filled from the material master in the line modal when a material is picked, editable. Old rows stay NULL and
-- fall back to the material's master unit when read (COALESCE in petty_cash.go getByID).
ALTER TABLE public.petty_cash_requisition_line
    ADD COLUMN IF NOT EXISTS unit VARCHAR(50);

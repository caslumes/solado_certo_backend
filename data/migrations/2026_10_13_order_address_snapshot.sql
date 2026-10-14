-- The order keeps the address it was shipped to, so the buyer can still edit or delete their addresses.
ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipping_address jsonb;
ALTER TABLE orders ALTER COLUMN shipping_address SET NOT NULL;
ALTER TABLE orders ALTER COLUMN shipping_address_id DROP NOT NULL;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orders_shipping_address_fk' AND confdeltype <> 'n') THEN
        ALTER TABLE orders DROP CONSTRAINT orders_shipping_address_fk;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orders_shipping_address_fk') THEN
        ALTER TABLE orders ADD CONSTRAINT orders_shipping_address_fk
            FOREIGN KEY (shipping_address_id) REFERENCES addresses (id) ON DELETE SET NULL;
    END IF;
END
$$;

-- The first entry of an order's history has no previous status.
ALTER TABLE order_status_history ALTER COLUMN previous_status DROP NOT NULL;

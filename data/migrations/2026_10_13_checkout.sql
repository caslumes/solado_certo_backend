CREATE TABLE IF NOT EXISTS checkouts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id uuid NOT NULL,
    idempotency_key varchar(120) NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT checkouts_buyer_fk FOREIGN KEY (buyer_id) REFERENCES users (id),
    CONSTRAINT checkouts_buyer_idempotency_key_unique UNIQUE (buyer_id, idempotency_key)
);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS checkout_id uuid;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS shipping_option_id uuid;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orders_checkout_fk') THEN
        ALTER TABLE orders ADD CONSTRAINT orders_checkout_fk FOREIGN KEY (checkout_id) REFERENCES checkouts (id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'order_items_shipping_option_fk') THEN
        ALTER TABLE order_items ADD CONSTRAINT order_items_shipping_option_fk FOREIGN KEY (shipping_option_id) REFERENCES shipping_options (id);
    END IF;
END
$$;

ALTER TABLE orders ALTER COLUMN checkout_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_orders_checkout ON orders (checkout_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_carts_user_active ON carts (user_id) WHERE status = 'active';

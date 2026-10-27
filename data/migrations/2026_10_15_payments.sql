ALTER TABLE users ADD COLUMN IF NOT EXISTS tax_id varchar(11);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_tax_id_unique') THEN
        ALTER TABLE users ADD CONSTRAINT users_tax_id_unique UNIQUE (tax_id);
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS payment_customers (
    user_id uuid NOT NULL,
    gateway varchar(60) NOT NULL,
    customer_id varchar(120) NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT payment_customers_pk PRIMARY KEY (user_id, gateway),
    CONSTRAINT payment_customers_user_fk FOREIGN KEY (user_id) REFERENCES users (id)
);

ALTER TABLE payments ADD COLUMN IF NOT EXISTS due_date date;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS failure_reason varchar(120);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_gateway_transaction ON payments (gateway, gateway_transaction_id);

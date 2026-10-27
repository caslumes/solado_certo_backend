ALTER TABLE reviews ADD COLUMN IF NOT EXISTS comfort_rating smallint;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'reviews_comfort_rating_check') THEN
        ALTER TABLE reviews ADD CONSTRAINT reviews_comfort_rating_check CHECK (comfort_rating BETWEEN 1 AND 5);
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_order_items_advertisement ON order_items (advertisement_id);

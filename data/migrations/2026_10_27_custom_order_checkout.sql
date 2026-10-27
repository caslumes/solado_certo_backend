ALTER TABLE order_items ADD COLUMN IF NOT EXISTS custom_order_request_id uuid;

ALTER TABLE custom_order_requests ADD COLUMN IF NOT EXISTS foot_photo_key text;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'order_items_custom_order_request_fk') THEN
        ALTER TABLE order_items ADD CONSTRAINT order_items_custom_order_request_fk FOREIGN KEY (custom_order_request_id) REFERENCES custom_order_requests (id);
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_order_items_custom_order_request ON order_items (custom_order_request_id) WHERE custom_order_request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_custom_order_requests_order ON custom_order_requests (order_id) WHERE order_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_order_documents_custom_order_request ON order_documents (custom_order_request_id, created_at DESC) WHERE custom_order_request_id IS NOT NULL;

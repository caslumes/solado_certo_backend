-- Whether the item needed a prescription when it was bought (RF-035), so changing the ad later
-- doesn't change what an order already requires.
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS prescription_required boolean NOT NULL DEFAULT false;

UPDATE order_items oi SET prescription_required = true
FROM advertisements a
WHERE a.id = oi.advertisement_id AND a.prescription_required AND NOT oi.prescription_required;

-- A prescription belongs to the item that needs it; a fitting report to its custom order request (RF-039).
ALTER TABLE order_documents ADD COLUMN IF NOT EXISTS order_item_id uuid;
ALTER TABLE order_documents ADD COLUMN IF NOT EXISTS custom_order_request_id uuid;
-- Why the file was rejected, and who rejected it (empty when the automatic check did).
ALTER TABLE order_documents ADD COLUMN IF NOT EXISTS rejection_reason text;
ALTER TABLE order_documents ADD COLUMN IF NOT EXISTS validated_by uuid;
-- order_documents.file_url keeps the key of the file in the private storage area.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'order_documents_order_item_fk') THEN
        ALTER TABLE order_documents ADD CONSTRAINT order_documents_order_item_fk FOREIGN KEY (order_item_id) REFERENCES order_items (id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'order_documents_custom_order_request_fk') THEN
        ALTER TABLE order_documents ADD CONSTRAINT order_documents_custom_order_request_fk FOREIGN KEY (custom_order_request_id) REFERENCES custom_order_requests (id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'order_documents_validated_by_fk') THEN
        ALTER TABLE order_documents ADD CONSTRAINT order_documents_validated_by_fk FOREIGN KEY (validated_by) REFERENCES users (id);
    END IF;
END
$$;

-- The latest document of each item decides whether it can be paid and shipped.
CREATE INDEX IF NOT EXISTS idx_order_documents_item_created ON order_documents (order_item_id, created_at DESC) WHERE order_item_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_order_documents_order ON order_documents (order_id);

-- Custom orders (RF-037–039): the price the seller quotes when accepting, the adaptation
-- instructions sent with the fitting report, and a status for asking the buyer for more information.
ALTER TABLE custom_order_requests ADD COLUMN IF NOT EXISTS quoted_price numeric(10,2);
ALTER TABLE custom_order_requests ADD COLUMN IF NOT EXISTS adaptation_instructions text;

ALTER TABLE custom_order_requests DROP CONSTRAINT IF EXISTS custom_order_requests_status_check;
ALTER TABLE custom_order_requests ADD CONSTRAINT custom_order_requests_status_check
    CHECK (status IN ('requested', 'under_review', 'needs_info', 'approved', 'rejected', 'in_production', 'completed', 'cancelled'));

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'custom_order_requests_quoted_price_check') THEN
        ALTER TABLE custom_order_requests ADD CONSTRAINT custom_order_requests_quoted_price_check CHECK (quoted_price > 0);
    END IF;
END
$$;

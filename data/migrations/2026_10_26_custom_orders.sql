ALTER TABLE consents DROP CONSTRAINT IF EXISTS consents_purpose_check;
ALTER TABLE consents ADD CONSTRAINT consents_purpose_check
    CHECK (purpose IN ('podological_profile', 'prescription', 'custom_order'));

ALTER TABLE custom_order_request_history ALTER COLUMN previous_status DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_custom_order_requests_buyer ON custom_order_requests (buyer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_order_requests_advertisement ON custom_order_requests (advertisement_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_order_request_history_request ON custom_order_request_history (request_id, created_at);

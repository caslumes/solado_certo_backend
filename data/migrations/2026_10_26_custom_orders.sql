-- Foot measurements and technical notes sent with a custom order request are shared with the seller,
-- so the request needs its own consent (LGPD art. 11, I).
ALTER TABLE consents DROP CONSTRAINT IF EXISTS consents_purpose_check;
ALTER TABLE consents ADD CONSTRAINT consents_purpose_check
    CHECK (purpose IN ('podological_profile', 'prescription', 'custom_order'));

-- A request's first history entry (NULL -> requested) has no previous status.
ALTER TABLE custom_order_request_history ALTER COLUMN previous_status DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_custom_order_requests_buyer ON custom_order_requests (buyer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_order_requests_advertisement ON custom_order_requests (advertisement_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_order_request_history_request ON custom_order_request_history (request_id, created_at);

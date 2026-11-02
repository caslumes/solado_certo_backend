ALTER TABLE support_tickets ADD COLUMN IF NOT EXISTS resolution text;
ALTER TABLE support_tickets ADD COLUMN IF NOT EXISTS updated_at timestamptz;
UPDATE support_tickets SET updated_at = opened_at WHERE updated_at IS NULL;
ALTER TABLE support_tickets ALTER COLUMN updated_at SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'support_tickets_category_check') THEN
        ALTER TABLE support_tickets ADD CONSTRAINT support_tickets_category_check CHECK (category IN ('order', 'payment', 'adaptation', 'other'));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_support_tickets_opened_by ON support_tickets (opened_by, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_tickets_order ON support_tickets (order_id) WHERE order_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_support_tickets_status ON support_tickets (status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_messages_ticket ON support_messages (ticket_id, created_at);

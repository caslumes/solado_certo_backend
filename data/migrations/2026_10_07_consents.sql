CREATE TABLE IF NOT EXISTS consents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id),
    purpose varchar(40) NOT NULL,
    terms_version varchar(40) NOT NULL,
    granted_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CONSTRAINT consents_purpose_check CHECK (purpose IN ('podological_profile', 'prescription'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_consents_active
    ON consents (user_id, purpose) WHERE revoked_at IS NULL;

ALTER TABLE podological_profiles ALTER COLUMN clinical_condition DROP NOT NULL;

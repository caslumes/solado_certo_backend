CREATE EXTENSION IF NOT EXISTS unaccent;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_ts_config WHERE cfgname = 'portuguese_unaccent') THEN
        CREATE TEXT SEARCH CONFIGURATION portuguese_unaccent (COPY = portuguese);
        ALTER TEXT SEARCH CONFIGURATION portuguese_unaccent
            ALTER MAPPING FOR hword, hword_part, word WITH unaccent, portuguese_stem;
    END IF;
END
$$;

ALTER TABLE advertisements ADD COLUMN IF NOT EXISTS search_vector tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('portuguese_unaccent', title), 'A') ||
    setweight(to_tsvector('portuguese_unaccent', description), 'B')
) STORED;

CREATE INDEX IF NOT EXISTS idx_advertisements_search_vector ON advertisements USING GIN (search_vector);

CREATE TABLE IF NOT EXISTS pain_point_conditions (
    pain_point_id uuid NOT NULL REFERENCES pain_points (id),
    condition_id uuid NOT NULL REFERENCES podological_conditions (id),
    PRIMARY KEY (pain_point_id, condition_id)
);

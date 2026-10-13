-- Technical attributes an ad of the category must fill before it is published (RF-031).
ALTER TABLE categories ADD COLUMN IF NOT EXISTS required_attributes varchar(30)[] NOT NULL DEFAULT '{}';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'categories_required_attributes_check') THEN
        ALTER TABLE categories ADD CONSTRAINT categories_required_attributes_check
            CHECK (required_attributes <@ ARRAY['side', 'size', 'material', 'rigidity', 'usage_indication']::varchar(30)[]);
    END IF;
END
$$;

ALTER TABLE endpoints ADD COLUMN source TEXT NOT NULL DEFAULT 'label' CHECK(source IN ('label', 'standalone'));
ALTER TABLE endpoints ADD COLUMN name TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_endpoint_source ON endpoints(source) WHERE active=1;

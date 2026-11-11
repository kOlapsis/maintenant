ALTER TABLE endpoints ADD COLUMN grandfathered INTEGER NOT NULL DEFAULT 0;
ALTER TABLE endpoints ADD COLUMN limited INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cert_monitors ADD COLUMN grandfathered INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cert_monitors ADD COLUMN limited INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cert_monitors ADD COLUMN inactive_since BIGINT;
UPDATE endpoints SET grandfathered = 1;
UPDATE cert_monitors SET grandfathered = 1 WHERE source IN ('standalone','label');

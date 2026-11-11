ALTER TABLE cert_monitors DROP COLUMN inactive_since;
ALTER TABLE cert_monitors DROP COLUMN limited;
ALTER TABLE cert_monitors DROP COLUMN grandfathered;
ALTER TABLE endpoints DROP COLUMN limited;
ALTER TABLE endpoints DROP COLUMN grandfathered;

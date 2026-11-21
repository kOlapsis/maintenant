ALTER TABLE image_updates ADD COLUMN major_tag TEXT NOT NULL DEFAULT '';
UPDATE alerts SET severity = 'info'
WHERE source = 'update' AND alert_type = 'update_available' AND status <> 'resolved' AND severity = 'critical';

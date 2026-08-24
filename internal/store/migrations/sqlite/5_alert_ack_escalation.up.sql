ALTER TABLE alerts ADD COLUMN acknowledged_at DATETIME;
ALTER TABLE alerts ADD COLUMN acknowledged_by TEXT;
ALTER TABLE alerts ADD COLUMN escalated_at DATETIME;

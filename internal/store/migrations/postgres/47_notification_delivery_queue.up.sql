ALTER TABLE notification_deliveries ADD COLUMN event_type TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_deliveries ADD COLUMN payload TEXT;
ALTER TABLE notification_deliveries ADD COLUMN next_attempt_at BIGINT NOT NULL DEFAULT 0;
UPDATE notification_deliveries SET status = 'failed', last_error = 'abandoned: left pending by a previous version' WHERE status = 'pending';
CREATE INDEX idx_deliveries_due ON notification_deliveries(status, next_attempt_at);

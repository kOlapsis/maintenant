DROP INDEX idx_deliveries_due;
ALTER TABLE notification_deliveries DROP COLUMN next_attempt_at;
ALTER TABLE notification_deliveries DROP COLUMN payload;
ALTER TABLE notification_deliveries DROP COLUMN event_type;

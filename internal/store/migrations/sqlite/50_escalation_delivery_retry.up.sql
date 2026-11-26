ALTER TABLE escalation_deliveries ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE escalation_deliveries ADD COLUMN next_attempt_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE escalation_deliveries ADD COLUMN created_at BIGINT NOT NULL DEFAULT 0;
UPDATE escalation_deliveries SET created_at = attempt_started_at;

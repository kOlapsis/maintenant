ALTER TABLE escalation_deliveries DROP CONSTRAINT escalation_deliveries_status_check;
ALTER TABLE escalation_deliveries ADD CONSTRAINT escalation_deliveries_status_check CHECK(status IN ('pending','sent','failed','abandoned','skipped_maintenance'));
ALTER TABLE status_components DROP COLUMN override_before_maintenance;

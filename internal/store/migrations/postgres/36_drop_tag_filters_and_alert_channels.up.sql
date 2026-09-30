ALTER TABLE alert_triggers DROP COLUMN filter_tags;
ALTER TABLE escalation_policies DROP COLUMN tags_json;
ALTER TABLE containers DROP COLUMN alert_channels;

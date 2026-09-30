ALTER TABLE containers ADD COLUMN alert_channels TEXT;
ALTER TABLE escalation_policies ADD COLUMN tags_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE alert_triggers ADD COLUMN filter_tags TEXT NOT NULL DEFAULT '';

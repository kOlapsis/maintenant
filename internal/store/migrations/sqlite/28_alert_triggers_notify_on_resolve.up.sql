-- Lets a trigger opt out of recovery (alert.resolved) notifications, so a channel can receive failures only.
ALTER TABLE alert_triggers ADD COLUMN notify_on_resolve INTEGER NOT NULL DEFAULT 1;

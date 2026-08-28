-- Gives a notification channel a place for a credential and for its non-secret
-- per-type settings. Telegram is the first to need both: a bot token, which no
-- response may ever carry, and an optional forum topic id.
ALTER TABLE notification_channels ADD COLUMN secret TEXT;
ALTER TABLE notification_channels ADD COLUMN config TEXT;

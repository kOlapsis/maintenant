-- Reverse spec 021: rétablit la table routing_rules et reconstitue les rules
-- depuis les triggers nommés `Rule for *`. Les Default triggers et les triggers
-- créés post-refactor sont perdus (acceptable — limite documentée).

CREATE TABLE IF NOT EXISTS routing_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    source_filter TEXT,
    severity_filter TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_routing_rules_channel ON routing_rules(channel_id);

-- Reconstitue best-effort les routing_rules depuis les triggers `Rule for *`.
INSERT INTO routing_rules (channel_id, source_filter, severity_filter, created_at)
SELECT
    atc.channel_id,
    NULLIF(t.filter_sources, ''),
    NULLIF(t.filter_severities, ''),
    t.created_at
FROM alert_triggers t
JOIN alert_trigger_channels atc ON atc.trigger_id = t.id
WHERE t.name LIKE 'Rule for %';

DROP INDEX IF EXISTS idx_atc_channel;
DROP TABLE alert_trigger_channels;
DROP INDEX IF EXISTS idx_alert_triggers_enabled;
DROP TABLE alert_triggers;

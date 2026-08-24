-- Spec 021: Channels & Alert Triggers — découplage du modèle de routing.
-- Channels deviennent des cibles silencieuses ; AlertTrigger porte la règle (filtres + M:N channels).
-- La migration data préserve le comportement legacy : chaque routing_rule devient un trigger,
-- chaque channel sans rule reçoit un Default trigger catch-all.

CREATE TABLE alert_triggers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    filter_severities TEXT NOT NULL DEFAULT '',
    filter_sources TEXT NOT NULL DEFAULT '',
    filter_scopes TEXT NOT NULL DEFAULT '',
    filter_tags TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_alert_triggers_enabled ON alert_triggers(enabled);

CREATE TABLE alert_trigger_channels (
    trigger_id INTEGER NOT NULL REFERENCES alert_triggers(id) ON DELETE CASCADE,
    channel_id INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    PRIMARY KEY (trigger_id, channel_id)
);

CREATE INDEX idx_atc_channel ON alert_trigger_channels(channel_id);

-- Step 1: pour chaque routing_rule existante, créer un AlertTrigger reproduisant le filtre.
INSERT INTO alert_triggers (name, filter_severities, filter_sources, enabled, created_at, updated_at)
SELECT
    'Rule for ' || c.name || ' #' || r.id,
    COALESCE(r.severity_filter, ''),
    COALESCE(r.source_filter, ''),
    1,
    r.created_at,
    r.created_at
FROM routing_rules r
JOIN notification_channels c ON c.id = r.channel_id;

-- Step 2: lier chaque trigger Rule à son channel d'origine (M:N).
INSERT INTO alert_trigger_channels (trigger_id, channel_id)
SELECT
    t.id,
    r.channel_id
FROM routing_rules r
JOIN notification_channels c ON c.id = r.channel_id
JOIN alert_triggers t ON t.name = 'Rule for ' || c.name || ' #' || r.id;

-- Step 3: pour chaque channel enabled sans aucune routing_rule, créer un Default catch-all trigger.
INSERT INTO alert_triggers (name, filter_severities, filter_sources, enabled, created_at, updated_at)
SELECT
    'Default — all alerts → ' || c.name,
    '',
    '',
    1,
    c.created_at,
    c.created_at
FROM notification_channels c
WHERE c.enabled = 1
  AND NOT EXISTS (SELECT 1 FROM routing_rules r WHERE r.channel_id = c.id);

INSERT INTO alert_trigger_channels (trigger_id, channel_id)
SELECT t.id, c.id
FROM notification_channels c
JOIN alert_triggers t ON t.name = 'Default — all alerts → ' || c.name
WHERE c.enabled = 1
  AND NOT EXISTS (SELECT 1 FROM routing_rules r WHERE r.channel_id = c.id);

-- Step 4: drop la table legacy.
DROP INDEX IF EXISTS idx_routing_rules_channel;
DROP TABLE routing_rules;

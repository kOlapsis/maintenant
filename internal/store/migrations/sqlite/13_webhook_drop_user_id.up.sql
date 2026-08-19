CREATE TABLE webhook_subscriptions_new (
    id                   TEXT NOT NULL PRIMARY KEY,
    name                 TEXT NOT NULL,
    url                  TEXT NOT NULL,
    secret               TEXT,
    event_types          TEXT NOT NULL DEFAULT '["*"]',
    is_active            INTEGER NOT NULL DEFAULT 1,
    last_delivery_status TEXT,
    last_delivery_at     DATETIME,
    failure_count        INTEGER NOT NULL DEFAULT 0,
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO webhook_subscriptions_new
    SELECT id, name, url, secret, event_types, is_active,
           last_delivery_status, last_delivery_at, failure_count, created_at
    FROM webhook_subscriptions;

DROP TABLE webhook_subscriptions;
ALTER TABLE webhook_subscriptions_new RENAME TO webhook_subscriptions;

CREATE TABLE webhook_subscriptions_old (
    id                   TEXT NOT NULL PRIMARY KEY,
    user_id              TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
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

-- Rows with no user cannot be restored; this migration is lossy on rollback.
DROP TABLE webhook_subscriptions;
ALTER TABLE webhook_subscriptions_old RENAME TO webhook_subscriptions;
CREATE INDEX idx_webhook_subs_user ON webhook_subscriptions(user_id);

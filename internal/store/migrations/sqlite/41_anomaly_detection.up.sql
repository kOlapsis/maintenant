CREATE TABLE anomaly_baseline (
    scope_type   TEXT NOT NULL,
    scope_id     TEXT NOT NULL,
    metric       TEXT NOT NULL,
    dimension    TEXT NOT NULL DEFAULT '',
    bucket       INTEGER NOT NULL,
    median       REAL NOT NULL,
    mad          REAL NOT NULL,
    sample_count INTEGER NOT NULL,
    updated_at   BIGINT NOT NULL,
    PRIMARY KEY (scope_type, scope_id, metric, dimension, bucket)
);

CREATE TABLE anomaly_series_state (
    scope_type        TEXT NOT NULL,
    scope_id          TEXT NOT NULL,
    metric            TEXT NOT NULL,
    dimension         TEXT NOT NULL DEFAULT '',
    node_id           TEXT,
    state             TEXT NOT NULL,
    first_seen_at     BIGINT NOT NULL,
    days_observed     REAL NOT NULL,
    active_buckets    INTEGER NOT NULL,
    ready_buckets     INTEGER NOT NULL,
    progress          REAL NOT NULL,
    ready_at          BIGINT,
    last_reset_at     BIGINT,
    last_reset_reason TEXT,
    sensitivity       TEXT NOT NULL,
    current_score     REAL NOT NULL DEFAULT 0,
    score_updated_at  BIGINT,
    updated_at        BIGINT NOT NULL,
    PRIMARY KEY (scope_type, scope_id, metric, dimension)
);
CREATE INDEX idx_anomaly_state_score ON anomaly_series_state(state, current_score);

CREATE TABLE anomaly_event (
    id              TEXT PRIMARY KEY NOT NULL,
    scope_type      TEXT NOT NULL,
    scope_id        TEXT NOT NULL,
    metric          TEXT NOT NULL,
    dimension       TEXT NOT NULL DEFAULT '',
    node_id         TEXT,
    detector        TEXT NOT NULL,
    tier            TEXT NOT NULL,
    started_at      BIGINT NOT NULL,
    ended_at        BIGINT,
    peak_value      REAL NOT NULL,
    baseline_median REAL NOT NULL,
    peak_deviation  REAL NOT NULL,
    alert_id        TEXT,
    suppressed_by   TEXT,
    created_at      BIGINT NOT NULL
);
CREATE INDEX idx_anomaly_event_scope ON anomaly_event(scope_type, scope_id, metric, started_at);
CREATE INDEX idx_anomaly_event_active ON anomaly_event(ended_at) WHERE ended_at IS NULL;

CREATE TABLE endpoint_uptime_daily (
    id             TEXT PRIMARY KEY NOT NULL,
    endpoint_id    TEXT NOT NULL REFERENCES endpoints(id) ON DELETE CASCADE,
    day            BIGINT NOT NULL,
    uptime_percent DOUBLE PRECISION NOT NULL,
    incident_count INTEGER NOT NULL,
    UNIQUE(endpoint_id, day)
);

CREATE TABLE heartbeat_uptime_daily (
    id             TEXT PRIMARY KEY NOT NULL,
    heartbeat_id   TEXT NOT NULL REFERENCES heartbeats(id) ON DELETE CASCADE,
    day            BIGINT NOT NULL,
    uptime_percent DOUBLE PRECISION NOT NULL,
    incident_count INTEGER NOT NULL,
    UNIQUE(heartbeat_id, day)
);

CREATE TABLE container_uptime_daily (
    id             TEXT PRIMARY KEY NOT NULL,
    container_id   TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    day            BIGINT NOT NULL,
    uptime_percent DOUBLE PRECISION NOT NULL,
    incident_count INTEGER NOT NULL,
    UNIQUE(container_id, day)
);

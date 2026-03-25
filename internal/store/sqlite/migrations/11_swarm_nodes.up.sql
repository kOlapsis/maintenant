CREATE TABLE IF NOT EXISTS swarm_nodes (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id               TEXT UNIQUE NOT NULL,
    hostname              TEXT NOT NULL,
    role                  TEXT NOT NULL CHECK(role IN ('manager', 'worker')),
    status                TEXT NOT NULL CHECK(status IN ('ready', 'down', 'disconnected', 'unknown')),
    availability          TEXT NOT NULL CHECK(availability IN ('active', 'pause', 'drain')),
    engine_version        TEXT DEFAULT '',
    address               TEXT DEFAULT '',
    task_count            INTEGER DEFAULT 0,
    first_seen_at         INTEGER NOT NULL,
    last_seen_at          INTEGER NOT NULL,
    last_status_change_at INTEGER NOT NULL
);

CREATE INDEX idx_swarm_nodes_status ON swarm_nodes(status);

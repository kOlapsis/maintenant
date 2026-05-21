-- Pro-only tables. Created on first boot in any mode (cheap, idempotent),
-- but only populated when the binary runs in mode=server with Enterprise license.
--
-- IMPORTANT: this migration relies on `PRAGMA foreign_keys = ON` being set
-- on every connection (already enforced via _foreign_keys=ON in the DSN).

CREATE TABLE IF NOT EXISTS agents (
    agent_id          TEXT PRIMARY KEY NOT NULL,           -- UUID v4 string (lowercase, no braces)
    public_key        BLOB NOT NULL,                       -- raw 32 bytes Ed25519
    hostname          TEXT NOT NULL,                       -- os.Hostname() reported by agent
    label             TEXT NOT NULL DEFAULT '',            -- display-only, NOT unique, max 64 chars
    os_arch           TEXT NOT NULL,                       -- e.g. "linux/amd64"
    agent_version     TEXT NOT NULL,                       -- semver
    detected_runtime  TEXT NOT NULL CHECK (detected_runtime IN ('docker','swarm','kubernetes')),
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','revoked')),
    last_seen_at      DATETIME,                            -- updated on each push, NULL until first event
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    revoked_at        DATETIME,
    revoked_by        TEXT,                                -- user identifier (audit)
    CHECK (length(label) <= 64)
);

CREATE INDEX IF NOT EXISTS idx_agents_status       ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_last_seen_at ON agents(last_seen_at);

-- Enrollment tokens. The `token` column stores the cleartext value because the
-- gRPC RegisterAgent flow needs to match it directly. API responses NEVER
-- expose the cleartext after creation (FR-023a).
CREATE TABLE IF NOT EXISTS enrollment_tokens (
    token_id              TEXT PRIMARY KEY NOT NULL,       -- hex(sha256(token))[:16]
    token                 TEXT NOT NULL UNIQUE,            -- "mnt_enr_" + base32(32 bytes random)
    created_at            DATETIME NOT NULL DEFAULT (datetime('now')),
    expires_at            DATETIME NOT NULL,
    consumed_at           DATETIME,
    consumed_by_agent_id  TEXT       -- agent_id at enrollment time (audit only, no FK — agent may not exist yet)
);

CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_expires_at ON enrollment_tokens(expires_at);

-- Multi-host attribution on existing tables.
-- NULL means "local" (mode=embedded or server with --embedded-agent).
-- ON DELETE CASCADE: when an agent is hard-deleted, all events it pushed are
-- purged in the same transaction.
ALTER TABLE containers        ADD COLUMN agent_id TEXT REFERENCES agents(agent_id) ON DELETE CASCADE;
ALTER TABLE endpoints         ADD COLUMN agent_id TEXT REFERENCES agents(agent_id) ON DELETE CASCADE;
ALTER TABLE heartbeats        ADD COLUMN agent_id TEXT REFERENCES agents(agent_id) ON DELETE CASCADE;
ALTER TABLE resource_snapshots ADD COLUMN agent_id TEXT REFERENCES agents(agent_id) ON DELETE CASCADE;
ALTER TABLE cert_monitors     ADD COLUMN agent_id TEXT REFERENCES agents(agent_id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_containers_agent_id        ON containers(agent_id)         WHERE agent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_endpoints_agent_id         ON endpoints(agent_id)          WHERE agent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_heartbeats_agent_id        ON heartbeats(agent_id)         WHERE agent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_resource_snapshots_agent_id ON resource_snapshots(agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cert_monitors_agent_id     ON cert_monitors(agent_id)      WHERE agent_id IS NOT NULL;

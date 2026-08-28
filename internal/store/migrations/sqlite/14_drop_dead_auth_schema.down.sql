-- Restore the dead auth schema (schema only, no data).
CREATE TABLE IF NOT EXISTS users (
    id           TEXT NOT NULL PRIMARY KEY,
    email        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    is_active    INTEGER NOT NULL DEFAULT 1,
    failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS teams (
    id          TEXT NOT NULL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    role        TEXT NOT NULL CHECK(role IN ('admin', 'editor', 'viewer')),
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS team_memberships (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id    TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, team_id)
);

CREATE TABLE IF NOT EXISTS oauth_clients (
    id             TEXT NOT NULL PRIMARY KEY,
    secret_hash    TEXT NOT NULL DEFAULT '',
    grant_types    TEXT NOT NULL DEFAULT '["password","refresh_token"]',
    response_types TEXT NOT NULL DEFAULT '["token"]',
    scopes         TEXT NOT NULL DEFAULT '["read","write","admin"]',
    redirect_uris  TEXT NOT NULL DEFAULT '[]',
    is_public      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS oauth_access_tokens (
    signature    TEXT NOT NULL PRIMARY KEY,
    request_id   TEXT NOT NULL,
    client_id    TEXT NOT NULL,
    user_id      TEXT REFERENCES users(id) ON DELETE CASCADE,
    scopes       TEXT NOT NULL DEFAULT '[]',
    granted_scopes TEXT NOT NULL DEFAULT '[]',
    session_data TEXT NOT NULL DEFAULT '{}',
    requested_at DATETIME NOT NULL,
    expires_at   DATETIME NOT NULL,
    is_active    INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_oauth_access_tokens_request_id ON oauth_access_tokens(request_id);
CREATE INDEX IF NOT EXISTS idx_oauth_access_tokens_user_id ON oauth_access_tokens(user_id);

CREATE TABLE IF NOT EXISTS oauth_refresh_tokens (
    signature    TEXT NOT NULL PRIMARY KEY,
    request_id   TEXT NOT NULL,
    client_id    TEXT NOT NULL,
    user_id      TEXT REFERENCES users(id) ON DELETE CASCADE,
    scopes       TEXT NOT NULL DEFAULT '[]',
    granted_scopes TEXT NOT NULL DEFAULT '[]',
    session_data TEXT NOT NULL DEFAULT '{}',
    requested_at DATETIME NOT NULL,
    expires_at   DATETIME NOT NULL,
    is_active    INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_oauth_refresh_tokens_request_id ON oauth_refresh_tokens(request_id);
CREATE INDEX IF NOT EXISTS idx_oauth_refresh_tokens_user_id ON oauth_refresh_tokens(user_id);

CREATE TABLE IF NOT EXISTS oauth_client_assertions (
    jti        TEXT NOT NULL PRIMARY KEY,
    expires_at DATETIME NOT NULL
);

CREATE TABLE api_tokens (
    id           TEXT NOT NULL PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    scopes       TEXT NOT NULL DEFAULT '["read"]',
    last_used_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    is_active    INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX idx_api_tokens_hash ON api_tokens(token_hash);
CREATE INDEX idx_api_tokens_user ON api_tokens(user_id);

CREATE TABLE IF NOT EXISTS audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    actor_id    TEXT,
    actor_email TEXT,
    action      TEXT NOT NULL,
    target_type TEXT,
    target_id   TEXT,
    details     TEXT,
    ip_address  TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

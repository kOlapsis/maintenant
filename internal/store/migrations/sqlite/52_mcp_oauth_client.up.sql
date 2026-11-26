CREATE TABLE mcp_oauth_client (
    id            INTEGER PRIMARY KEY NOT NULL CHECK (id = 1),
    secret_fingerprint TEXT NOT NULL
);

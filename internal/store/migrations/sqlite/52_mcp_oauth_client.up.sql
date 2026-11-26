CREATE TABLE mcp_oauth_client (
    id            INTEGER PRIMARY KEY NOT NULL CHECK (id = 1),
    secret_fingerprint TEXT NOT NULL
);
UPDATE mcp_oauth_tokens SET revoked = 1;
UPDATE mcp_oauth_codes SET used = 1;

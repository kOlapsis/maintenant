-- Drop the planned-but-never-implemented auth/user schema.
-- None of these tables are referenced by any Go code.
-- The MCP OAuth flow uses mcp_oauth_codes/mcp_oauth_tokens (migration 4), not these.

-- Drop children first (FK references to users/teams).
DROP TABLE IF EXISTS team_memberships;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS oauth_access_tokens;
DROP TABLE IF EXISTS oauth_refresh_tokens;

-- Drop parents.
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS teams;

-- Drop the rest (no FK dependencies).
DROP TABLE IF EXISTS oauth_clients;
DROP TABLE IF EXISTS oauth_client_assertions;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS settings;

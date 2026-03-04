# MCP Server

Expose maintenant monitoring data to AI coding assistants (Claude Code, Claude Desktop, Cursor, Windsurf) via the [Model Context Protocol](https://modelcontextprotocol.io/). Query container states, resource metrics, endpoint health, and more — directly from your editor.

---

## Overview

maintenant embeds an MCP server that provides 18 tools covering every monitoring dimension. AI assistants can use these tools to diagnose issues, correlate data, and suggest fixes without you ever leaving your editor.

**Transports:**

| Transport | Use case | Auth |
|-----------|----------|------|
| **Stdio** (`--mcp-stdio`) | Local development, Claude Code | None (trusted local) |
| **Streamable HTTP** (`/mcp`) | Remote access, Claude web/mobile, Claude Desktop | OAuth2 (client_id + secret) |

---

## Getting Started

### Claude Code (stdio)

Add to your Claude Code MCP settings:

```json
{
  "mcpServers": {
    "maintenant": {
      "command": "maintenant",
      "args": ["--mcp-stdio"],
      "env": {
        "MAINTENANT_DB": "/path/to/maintenant.db"
      }
    }
  }
}
```

### Claude web / Claude Desktop / Cursor (Streamable HTTP)

1. Enable the MCP server and configure OAuth2 credentials:

```bash
MAINTENANT_MCP=true
MAINTENANT_MCP_CLIENT_ID=my-mcp-client
MAINTENANT_MCP_CLIENT_SECRET=a-strong-random-secret
MAINTENANT_BASE_URL=https://now.example.com
```

2. In Claude's settings, add your maintenant instance as a remote MCP server:
   - **URL**: `https://now.example.com/mcp`
   - **Advanced Settings**: enter the `client_id` and `client_secret` you configured above.

3. Claude will automatically discover the OAuth2 endpoints, authorize, and connect. No manual token exchange required.

---

## Authentication

### Stdio

No authentication. The stdio transport is a local, trusted channel — only the process that spawned maintenant can communicate with it. The `--mcp-stdio` flag is independent of `MAINTENANT_MCP`.

### Streamable HTTP (OAuth2)

When `MAINTENANT_MCP_CLIENT_ID` and `MAINTENANT_MCP_CLIENT_SECRET` are both set, maintenant runs a full OAuth2 authorization server implementing the flow required by the MCP specification (2025-11-25):

1. **Discovery** — The client fetches `/.well-known/oauth-protected-resource` ([RFC 9728](https://www.rfc-editor.org/rfc/rfc9728)) and `/.well-known/oauth-authorization-server` ([RFC 8414](https://www.rfc-editor.org/rfc/rfc8414)) to discover endpoints.
2. **Authorization** — The client redirects to `/oauth/authorize` with PKCE (S256). maintenant validates the client credentials and auto-approves (no consent page).
3. **Token exchange** — The client exchanges the authorization code at `/oauth/token` for an access token (1h) and a refresh token (30d).
4. **Authenticated requests** — The client sends `Authorization: Bearer <token>` on every `/mcp` request.
5. **Token refresh** — When the access token expires, the client silently uses the refresh token to obtain new tokens.

**Access control** is based on knowledge of the client secret. The administrator generates the credentials and shares them with authorized users, who enter them in Claude's Advanced Settings. There is no user login page — maintenant has no user authentication system.

When the OAuth2 variables are absent, the HTTP transport is open. Use your reverse proxy's auth layer to protect it.

### OAuth2 Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/.well-known/oauth-protected-resource` | GET | Protected resource metadata (RFC 9728). Public. |
| `/.well-known/oauth-authorization-server` | GET | Authorization server metadata (RFC 8414). Public. |
| `/oauth/authorize` | GET | Authorization endpoint. Validates client credentials + PKCE, auto-approves, redirects with code. |
| `/oauth/token` | POST | Token endpoint. Exchanges code for tokens (`authorization_code`) or refreshes (`refresh_token`). |

### Security Details

- **PKCE S256** is mandatory on all authorization requests.
- **Tokens are opaque** (random 32 bytes, hex-encoded). They are stored as SHA-256 hashes — even a database leak does not expose usable tokens.
- **Refresh token rotation** — each use of a refresh token invalidates it and issues a new one.
- **Replay detection** — reusing an already-consumed refresh token revokes all tokens in the session (family), forcing re-authorization.
- **Automatic cleanup** — expired tokens and codes are garbage-collected every 15 minutes.
- **Client secret comparison** uses constant-time comparison to prevent timing attacks.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_MCP` | `false` | Enable the Streamable HTTP MCP server on `/mcp`. |
| `MAINTENANT_MCP_CLIENT_ID` | — | OAuth2 client identifier. Required for authentication. |
| `MAINTENANT_MCP_CLIENT_SECRET` | — | OAuth2 client secret. Required for authentication. |
| `MAINTENANT_BASE_URL` | `http://localhost:8080` | Public-facing URL. Used as OAuth2 issuer and in metadata endpoints. |

The `--mcp-stdio` flag is independent of these variables — it runs the MCP server over stdin/stdout and exits when the connection closes.

### Generating Credentials

Use any random string generator for the client ID and secret:

```bash
# Example using openssl
export MAINTENANT_MCP_CLIENT_ID="maintenant-mcp"
export MAINTENANT_MCP_CLIENT_SECRET=$(openssl rand -hex 32)
```

Share the client ID and secret with authorized users. They enter these values in Claude's Advanced Settings when adding the remote MCP server.

---

## Available Tools

### Read Tools

| Tool | Description |
|------|-------------|
| `list_containers` | List all monitored containers with state, health, and metadata |
| `get_container` | Detailed container info with recent state transitions |
| `get_container_logs` | Recent log lines from a container (configurable line count) |
| `list_endpoints` | All HTTP/TCP endpoints with status, response time, uptime |
| `get_endpoint_history` | Check history for a specific endpoint |
| `list_heartbeats` | All heartbeat monitors with status, last ping, periods |
| `list_certificates` | TLS certificates with expiration, issuer, chain validity |
| `list_alerts` | Active alerts (or full history with `active_only: false`) |
| `get_resources` | Host resource summary: CPU, memory, network, disk |
| `get_top_consumers` | Containers ranked by CPU or memory usage |
| `get_updates` | Available image updates for monitored containers |
| `get_health` | maintenant version, runtime, and status |

### Write Tools

| Tool | Description | Edition |
|------|-------------|---------|
| `acknowledge_alert` | Acknowledge an active alert | Extended |
| `create_incident` | Create a status page incident | Extended |
| `update_incident` | Update an existing incident | Extended |
| `create_maintenance` | Schedule a maintenance window | Extended |
| `pause_monitor` | Pause a heartbeat monitor | CE |
| `resume_monitor` | Resume a paused heartbeat monitor | CE |

Write tools marked **Extended** return an error in the Community Edition.

---

## Example Prompts

Once connected, you can ask your AI assistant questions like:

- "Which containers are unhealthy right now?"
- "Show me the logs for the postgres container."
- "What's consuming the most CPU?"
- "Are there any active alerts?"
- "Which certificates expire within 30 days?"
- "Are there image updates available for my containers?"
- "Pause the backup-check heartbeat monitor."

---

## Proxy Configuration

If maintenant runs behind a reverse proxy, the `/mcp` and `/oauth/*` paths require special handling:

- **No request timeout** — MCP uses SSE for server-to-client streaming, which requires long-lived connections.
- **No buffering** — Disable response buffering for `/mcp` to allow real-time SSE delivery.
- **Pass-through for OAuth** — The `/oauth/authorize` endpoint issues 302 redirects. Ensure your proxy does not intercept them.

### Traefik Example

```yaml
labels:
  traefik.http.routers.maintenant-mcp.rule: "Host(`now.example.com`) && PathPrefix(`/mcp`)"
  traefik.http.services.maintenant-mcp.loadbalancer.server.port: "8080"
```

### Caddy Example

```
now.example.com {
    reverse_proxy maintenant:8080
}
```

No special configuration needed — Caddy handles SSE and redirects natively.

---

## Related

- [Container Monitoring](containers.md) — Container states and health exposed via `list_containers`, `get_container`
- [Endpoint Monitoring](endpoints.md) — Endpoint health via `list_endpoints`, `get_endpoint_history`
- [Heartbeat Monitoring](heartbeats.md) — Heartbeat status via `list_heartbeats`, `pause_monitor`
- [Certificate Monitoring](certificates.md) — Certificate expiry via `list_certificates`
- [Resource Metrics](resources.md) — Resource usage via `get_resources`, `get_top_consumers`
- [Alert Engine](alerts.md) — Active alerts via `list_alerts`
- [Update Intelligence](updates.md) — Image updates via `get_updates`

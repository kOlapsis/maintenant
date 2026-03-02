# MCP Server

Expose maintenant monitoring data to AI coding assistants (Claude Code, Claude Desktop, Cursor, Windsurf) via the [Model Context Protocol](https://modelcontextprotocol.io/). Query container states, resource metrics, endpoint health, and more — directly from your editor.

---

## Overview

maintenant embeds an MCP server that provides 18 tools covering every monitoring dimension. AI assistants can use these tools to diagnose issues, correlate data, and suggest fixes without you ever leaving your editor.

**Transports:**

| Transport | Use case | Auth |
|-----------|----------|------|
| **Stdio** (`--mcp-stdio`) | Local development, Claude Code | None (trusted local) |
| **Streamable HTTP** (`/mcp`) | Remote access, Claude Desktop, web clients | Optional JWT |

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

### Claude Desktop / Cursor (Streamable HTTP)

1. Enable the MCP server:

```bash
MAINTENANT_MCP=true
```

2. (Optional) Restrict access to a specific email:

```bash
MAINTENANT_MCP_ALLOWED_EMAIL=you@example.com
```

3. Connect your client to `http://your-maintenant:8080/mcp`.

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

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_MCP` | `false` | Enable the Streamable HTTP MCP server on `/mcp`. |
| `MAINTENANT_MCP_ALLOWED_EMAIL` | — | If set, only allow requests with a JWT containing this email claim. |

The `--mcp-stdio` flag is independent of `MAINTENANT_MCP` — it runs the MCP server over stdin/stdout and exits when the connection closes.

---

## Authentication

### Stdio

No authentication. The stdio transport is a local, trusted channel — only the process that spawned maintenant can communicate with it.

### Streamable HTTP

When `MAINTENANT_MCP_ALLOWED_EMAIL` is set, maintenant requires a `Bearer` JWT in the `Authorization` header. The email claim (or `sub` claim as fallback) must match the configured address. This is the mechanism used by Claude.ai and other OAuth2-capable MCP clients.

When the variable is empty, the HTTP transport is open. Use your reverse proxy's auth layer to protect it.

maintenant serves [RFC 9728 OAuth 2.0 Protected Resource Metadata](https://www.rfc-editor.org/rfc/rfc9728) at `/.well-known/oauth-protected-resource` to help MCP clients discover auth requirements.

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

If maintenant runs behind a reverse proxy, the `/mcp` path requires special handling:

- **No request timeout** — MCP uses SSE for server-to-client streaming, which requires long-lived connections.
- **No buffering** — Disable response buffering for `/mcp` to allow real-time SSE delivery.
- **WebSocket-like headers** — Some proxies need `Connection: keep-alive` and no content-length enforcement.

### Traefik Example

```yaml
labels:
  traefik.http.routers.maintenant-mcp.rule: "Host(`now.example.com`) && PathPrefix(`/mcp`)"
  traefik.http.services.maintenant-mcp.loadbalancer.server.port: "8080"
```

---

## Related

- [Container Monitoring](containers.md) — Container states and health exposed via `list_containers`, `get_container`
- [Endpoint Monitoring](endpoints.md) — Endpoint health via `list_endpoints`, `get_endpoint_history`
- [Heartbeat Monitoring](heartbeats.md) — Heartbeat status via `list_heartbeats`, `pause_monitor`
- [Certificate Monitoring](certificates.md) — Certificate expiry via `list_certificates`
- [Resource Metrics](resources.md) — Resource usage via `get_resources`, `get_top_consumers`
- [Alert Engine](alerts.md) — Active alerts via `list_alerts`
- [Update Intelligence](updates.md) — Image updates via `get_updates`

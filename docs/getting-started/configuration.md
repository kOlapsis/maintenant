# Configuration

maintenant is configured entirely through environment variables. No configuration files required.

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_ADDR` | `127.0.0.1:8080` | HTTP bind address. Use `0.0.0.0:8080` inside containers. |
| `MAINTENANT_DB` | `./maintenant.db` | SQLite database file path. |
| `MAINTENANT_BASE_URL` | `http://localhost:8080` | Public base URL. Used for heartbeat ping URLs and status page links. |
| `MAINTENANT_CORS_ORIGINS` | same-origin | CORS allowed origins (comma-separated). Empty means same-origin only. Set to `*` for wildcard. |
| `MAINTENANT_RUNTIME` | auto-detect | Force container runtime: `docker` or `kubernetes`. Auto-detected by default. |
| `MAINTENANT_MAX_BODY_SIZE` | `1048576` | Maximum request body size in bytes for POST/PUT requests (default: 1 MB). |
| `MAINTENANT_UPDATE_INTERVAL` | `24h` | Update intelligence scan interval. Accepts Go duration format (e.g., `12h`, `30m`). |
| `MAINTENANT_K8S_NAMESPACES` | all | Kubernetes namespace allowlist (comma-separated). Empty monitors all namespaces. |
| `MAINTENANT_K8S_EXCLUDE_NAMESPACES` | none | Kubernetes namespace blocklist (comma-separated). |
| `MAINTENANT_LICENSE_KEY` | — | Pro license key. Enables Pro features when set to a valid key. |
| `MAINTENANT_MCP` | `false` | Enable the MCP server on `/mcp` (Streamable HTTP transport). |
| `MAINTENANT_MCP_CLIENT_ID` | — | OAuth2 client ID for MCP authentication. |
| `MAINTENANT_MCP_CLIENT_SECRET` | — | OAuth2 client secret for MCP authentication. |
| `MAINTENANT_ORGANISATION_NAME` | `Maintenant` | Organisation name displayed on the public status page. |
| `MAINTENANT_SMTP_HOST` | — | SMTP server hostname for email notifications. |
| `MAINTENANT_SMTP_PORT` | `587` | SMTP server port. |
| `MAINTENANT_SMTP_USERNAME` | — | SMTP authentication username. |
| `MAINTENANT_SMTP_PASSWORD` | — | SMTP authentication password. |
| `MAINTENANT_SMTP_FROM` | `maintenant@localhost` | Sender address for email notifications. |
| `MAINTENANT_DISABLE_TELEMETRY` | unset (telemetry on) | Disable anonymous install telemetry. Truthy values: `1`, `t`, `true`, `y`, `yes`, `on` (case-insensitive). |

### Example `.env` File

```bash
# Listen address (use 0.0.0.0 inside containers, 127.0.0.1 on host)
MAINTENANT_ADDR=127.0.0.1:8080

# SQLite database path
MAINTENANT_DB=./maintenant.db

# Public base URL (used for heartbeat ping URLs and status page links)
MAINTENANT_BASE_URL=https://maintenant.example.com

# CORS allowed origins (comma-separated, empty = same-origin only)
# MAINTENANT_CORS_ORIGINS=http://localhost:5173

# Container runtime override (auto-detected by default: docker or kubernetes)
# MAINTENANT_RUNTIME=docker

# Max request body size in bytes (default: 1MB)
# MAINTENANT_MAX_BODY_SIZE=1048576

# Update intelligence scan interval (Go duration, default: 24h)
# MAINTENANT_UPDATE_INTERVAL=24h

# Kubernetes namespaces to monitor (comma-separated, empty = all)
# MAINTENANT_K8S_NAMESPACES=default,production

# Kubernetes namespaces to exclude (comma-separated)
# MAINTENANT_K8S_EXCLUDE_NAMESPACES=kube-system

# Organisation name (displayed on the public status page)
# MAINTENANT_ORGANISATION_NAME=Acme Corp

# MCP Server (Model Context Protocol for AI assistants)
# MAINTENANT_MCP=true
# MAINTENANT_MCP_CLIENT_ID=maintenant-mcp
# MAINTENANT_MCP_CLIENT_SECRET=your-secret-here

# SMTP configuration (required for email notification channels)
# MAINTENANT_SMTP_HOST=smtp.example.com
# MAINTENANT_SMTP_PORT=587
# MAINTENANT_SMTP_USERNAME=alerts@example.com
# MAINTENANT_SMTP_PASSWORD=secret
# MAINTENANT_SMTP_FROM=maintenant@example.com

# Disable anonymous install telemetry (truthy: 1, true, yes, on)
# MAINTENANT_DISABLE_TELEMETRY=1
```

---

## Telemetry

maintenant sends an **anonymous, opt-out** usage snapshot once an hour to `https://metrics.kolapsis.com`. No hostnames, IPs, container names, endpoint URLs, certificates, webhook targets, status-page component names, license keys, or operator-supplied free-form strings of any kind are transmitted.

### What is collected

Each snapshot contains only the following fields:

**Application fields:**

- `edition` — `community` or `pro`
- `containers_total`, `endpoints_total`, `heartbeats_total`, `certificates_total`, `webhooks_total`, `status_components_total` — counts of configured/auto-discovered entities

**Runtime context** (collected automatically by the SHM SDK):

- `sys_os`, `sys_arch`, `sys_cpu_cores`, `sys_go_version`, `sys_mode` (`docker` / `kubernetes` / `standalone`)
- `app_mem_alloc_mb`, `app_goroutines`, `app_uptime_h`
- An opaque installation identifier generated on first run and persisted to `/data/shm/shm_identity.json`

### How to disable it

Set `MAINTENANT_DISABLE_TELEMETRY` to a truthy value before the process starts:

```yaml
services:
  maintenant:
    environment:
      MAINTENANT_DISABLE_TELEMETRY: "1"
```

Truthy values (case-insensitive, whitespace-trimmed): `1`, `t`, `true`, `y`, `yes`, `on`. Anything else — including empty or unset — leaves telemetry enabled.

When disabled, exactly one log line is emitted at startup and nothing else:

```text
INFO  telemetry disabled  reason=opt-out
```

No background goroutine, no DNS lookup of `metrics.kolapsis.com`, no outbound packets.

### Persistent install identity

The SDK persists its identity to `/data/shm/shm_identity.json` on first run. The reference Docker Compose mounts `/data` as a named volume, so `/data/shm` is covered by default. The container entrypoint chowns `/data/shm` at startup, so bind mounts work too without manual permission setup.

For Kubernetes, mount a PVC at `/data/shm`:

```yaml
volumeMounts:
  - name: shm-data
    mountPath: /data/shm
volumeClaimTemplates:
  - metadata:
      name: shm-data
    spec:
      accessModes: [ReadWriteOnce]
      resources:
        requests:
          storage: 100Mi
```

Without persistence, every restart is counted as a fresh install. The privacy impact is zero (the identifier is opaque), but fleet stats are distorted.

If `/data/shm` is unwritable (read-only mount), telemetry disables itself silently with a single WARN line and the process continues normally:

```text
WARN  telemetry disabled  reason=datadir-unwritable  datadir=/data/shm  error=...
```

---

## Pro License

To enable Pro features (Slack/Teams/Email channels, CVE enrichment, incident management, maintenance windows, subscriber notifications, and more), set the `MAINTENANT_LICENSE_KEY` environment variable:

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant-pro:latest
    environment:
      MAINTENANT_LICENSE_KEY: "your-license-key"
```

The license is verified periodically against the license server. If the server is temporarily unreachable, Pro features remain active from cache with a graceful degradation window.

You can check the current license status via the API:

```
GET /api/v1/license/status
```

---

## Security

maintenant does not include built-in authentication — by design. It delegates auth to your reverse proxy and middleware (Authelia, Authentik, OAuth2 Proxy).

```
Internet  →  Reverse Proxy (Traefik / Caddy / nginx)
          →  Auth Provider
          →  maintenant
```

The `/api/v1/*` routes and the dashboard must be behind authentication. The `/ping/` and `/status/` routes must be publicly accessible. If MCP is enabled with OAuth2, the `/mcp`, `/oauth/`, and `/.well-known/` routes should bypass proxy auth (MCP handles its own).

See the **[Security Guide](../security.md)** for the complete route reference, reverse proxy examples (Traefik, Caddy, nginx), built-in protections, MCP authentication details, and deployment hardening checklist.

---

## Database

maintenant uses SQLite in WAL (Write-Ahead Logging) mode with a single-writer pattern. This provides excellent read performance while maintaining data integrity.

- The database file is created automatically on first startup.
- Migrations run automatically — no manual steps required.
- Back up the database by copying the `.db`, `.db-wal`, and `.db-shm` files while maintenant is stopped, or use `sqlite3 backup` while running.

!!! tip "Persistence in Docker"
    Always mount a volume for the database directory to persist data across container restarts:
    ```yaml
    volumes:
      - maintenant-data:/data
    environment:
      MAINTENANT_DB: "/data/maintenant.db"
    ```

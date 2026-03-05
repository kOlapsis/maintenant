# Security

maintenant does not include built-in authentication — by design.

Like Prometheus, Dozzle, and most self-hosted monitoring tools, it delegates authentication to your existing reverse proxy and auth middleware. No need to manage yet another set of user accounts.

```
Internet  →  Reverse Proxy (Traefik / Caddy / nginx)
          →  Auth Provider (Authelia / Authentik / OAuth2 Proxy)
          →  maintenant
```

This page covers every aspect of securing a maintenant deployment: which routes to protect, which to leave open, and how to configure your reverse proxy accordingly.

---

## Route Reference

Not all routes should sit behind authentication. Some must be publicly accessible for maintenant to function correctly.

### Public Routes

These routes **must bypass** your authentication middleware:

| Route | Purpose | Rate Limited |
|-------|---------|:------------:|
| `/ping/{uuid}` | Heartbeat ping — called by cron jobs, CI/CD pipelines, external services | Yes |
| `/ping/{uuid}/start` | Job start signal for duration tracking | Yes |
| `/ping/{uuid}/{exit_code}` | Job completion with exit code | Yes |
| `/status/api` | Status page JSON API | Yes |
| `/status/events` | SSE stream of status changes | Yes |
| `/status/feed.atom` | Atom feed of incidents | Yes |
| `/status/subscribe` | Email subscription to status updates | Yes |
| `/status/confirm` | Subscription confirmation (token-based) | Yes |
| `/status/unsubscribe` | Unsubscribe (token-based) | Yes |

### MCP & OAuth Routes

Only registered when `MAINTENANT_MCP=true`. If MCP uses OAuth2 (recommended), these routes handle the OAuth flow and **must bypass proxy-level auth** — MCP has its own authentication:

| Route | Purpose |
|-------|---------|
| `/mcp` | MCP Streamable HTTP endpoint. Protected by OAuth2 bearer token when credentials are configured. |
| `/.well-known/oauth-authorization-server` | OAuth2 server metadata discovery (RFC 8414) |
| `/.well-known/oauth-protected-resource` | Protected resource metadata (RFC 9728) |
| `/oauth/authorize` | Authorization endpoint (PKCE S256 mandatory) |
| `/oauth/token` | Token exchange and refresh |

!!! warning "Open MCP"
    When `MAINTENANT_MCP=true` is set **without** `MAINTENANT_MCP_CLIENT_ID` and `MAINTENANT_MCP_CLIENT_SECRET`, the `/mcp` endpoint is open — anyone can query your monitoring data. Either configure OAuth2 credentials or protect `/mcp` with your reverse proxy.

### Protected Routes

These routes provide full read/write access to your monitoring system. **Always require authentication** via your reverse proxy:

| Route | Purpose |
|-------|---------|
| `/api/v1/*` | Admin API — containers, endpoints, heartbeats, certificates, alerts, webhooks, status page management, update intelligence, resources |
| `/` | Dashboard (Vue SPA) |

!!! danger "Do not expose `/api/v1/` without authentication"
    The admin API provides unrestricted access to all monitoring data and configuration: creating webhooks, managing heartbeats, viewing container logs, acknowledging alerts, and more. There is no authorization layer — any request that reaches the API is trusted.

---

## Reverse Proxy Setup

### Traefik + Authelia

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    labels:
      traefik.enable: "true"

      # Main router — requires authentication
      traefik.http.routers.maintenant.rule: "Host(`now.example.com`)"
      traefik.http.routers.maintenant.middlewares: "authelia@docker"

      # Public routes — no auth
      traefik.http.routers.maintenant-public.rule: >
        Host(`now.example.com`) &&
        (PathPrefix(`/ping/`) || PathPrefix(`/status/`))
      traefik.http.routers.maintenant-public.priority: "100"

      # MCP + OAuth routes — MCP handles its own auth
      traefik.http.routers.maintenant-mcp.rule: >
        Host(`now.example.com`) &&
        (PathPrefix(`/mcp`) || PathPrefix(`/oauth/`) || PathPrefix(`/.well-known/`))
      traefik.http.routers.maintenant-mcp.priority: "100"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - maintenant-data:/data
    environment:
      MAINTENANT_DB: "/data/maintenant.db"
      MAINTENANT_BASE_URL: "https://now.example.com"
```

### Caddy + Authelia

```
now.example.com {
    # Public routes — no auth
    @public path /ping/* /status/*
    reverse_proxy @public maintenant:8080

    # MCP routes — own OAuth2 auth
    @mcp path /mcp /mcp/* /oauth/* /.well-known/*
    reverse_proxy @mcp maintenant:8080

    # Everything else — requires auth
    forward_auth authelia:9091 {
        uri /api/verify?rd=https://auth.example.com
        copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
    }
    reverse_proxy maintenant:8080
}
```

### nginx + OAuth2 Proxy

```nginx
server {
    listen 443 ssl;
    server_name now.example.com;

    # Public routes — no auth
    location ~ ^/(ping|status)/ {
        proxy_pass http://127.0.0.1:8080;
    }

    # MCP routes — own OAuth2 + SSE support
    location ~ ^/(mcp|oauth|\.well-known)/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_buffering off;
        proxy_read_timeout 86400s;
    }

    # Protected routes — requires auth
    location / {
        auth_request /oauth2/auth;
        error_page 401 = /oauth2/sign_in;
        proxy_pass http://127.0.0.1:8080;
    }

    location /oauth2/ {
        proxy_pass http://127.0.0.1:4180;
    }
}
```

!!! tip "SSE and MCP"
    The `/mcp` endpoint uses SSE for streaming. Disable response buffering and increase read timeouts in your proxy for this path. Caddy handles this natively — no special configuration needed.

---

## Built-in Protections

### Rate Limiting

Per-IP token bucket rate limiter applied to all public-facing routes:

| Setting | Value |
|---------|-------|
| Rate | 10 requests/second per IP |
| Burst | 20 requests |
| Applied to | `/ping/`, `/status/*`, `/mcp` |
| Not applied to | `/api/v1/*` (expected behind auth) |
| 429 response | `{"error":{"code":"rate_limited","message":"Too many requests"}}` with `Retry-After: 1` |

IP detection priority: `X-Real-IP` header → first entry in `X-Forwarded-For` → `RemoteAddr`.

The `/status/subscribe` endpoint has an additional rate limit of 5 requests per IP per hour to prevent subscription abuse.

!!! tip "Trusted proxies"
    Make sure your reverse proxy sets `X-Real-IP` or `X-Forwarded-For` correctly. Without it, all requests appear to come from the proxy's IP and share a single rate limit bucket.

### Request Size Limits

POST and PUT request bodies are limited to **1 MB** by default. Configurable via `MAINTENANT_MAX_BODY_SIZE` (in bytes).

### Request Timeouts

A 10-second timeout is enforced on all non-streaming routes. Streaming paths are exempt:

- `/api/v1/containers/events` (SSE)
- `/api/v1/containers/{id}/logs/stream` (SSE)
- `/status/events` (SSE)
- `/mcp` (MCP Streamable HTTP)

### CORS

Controlled by `MAINTENANT_CORS_ORIGINS`:

| Value | Behavior |
|-------|----------|
| Unset (default) | No CORS headers — same-origin only |
| `*` | `Access-Control-Allow-Origin: *` |
| Comma-separated list | Allowlist with `Vary: Origin` |

The `/status/api` endpoint always returns `Access-Control-Allow-Origin: *` regardless of this setting, since the status page is designed to be embedded anywhere.

---

## MCP Authentication

When `MAINTENANT_MCP_CLIENT_ID` and `MAINTENANT_MCP_CLIENT_SECRET` are both configured, the MCP endpoint is protected by a full OAuth 2.1 implementation:

- **PKCE S256** mandatory on all authorization requests
- **Opaque tokens** — 32-byte random values stored as SHA-256 hashes (a database leak does not expose usable tokens)
- **Client secret** stored as SHA-256 hash with constant-time comparison
- **Access tokens** expire after 1 hour, **refresh tokens** after 30 days
- **Refresh token rotation** — each use invalidates the old token
- **Replay detection** — reusing a consumed refresh token revokes the entire token family
- **Authorization codes** expire in 10 minutes
- **Automatic cleanup** of expired tokens every 15 minutes

The stdio transport (`--mcp-stdio`) requires no authentication — it is a local, trusted channel only accessible to the process that spawned maintenant.

See [MCP Server](features/mcp.md) for full configuration and usage details.

---

## Deployment Hardening

### Docker Socket

maintenant requires access to the Docker socket to discover and monitor containers. Mount it **read-only**:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
  - /proc:/host/proc:ro
```

The Docker socket grants root-equivalent access to the host. maintenant only reads container state, metadata, and logs — it never creates, modifies, or deletes containers. The read-only mount is a defense-in-depth measure.

For stricter isolation, consider using a Docker socket proxy like [Tecnativa/docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) to restrict API access to only the endpoints maintenant needs.

### Network Binding

maintenant binds to `127.0.0.1:8080` by default — **localhost only**. This prevents direct exposure to the network.

Inside a Docker container, use `0.0.0.0:8080` (the Dockerfile sets this automatically) but **never publish the port directly to the host network**. Let the reverse proxy handle external traffic.

!!! danger "Never expose maintenant directly to the internet"
    Without a reverse proxy providing authentication, anyone can access the admin API and read your container logs, metrics, and alerts.

### Database

SQLite in WAL mode. The database file contains all monitoring data, alert history, webhook configurations, and (if MCP OAuth is enabled) hashed tokens.

- Store on a **local filesystem** — NFS and network-mounted volumes cause locking issues with SQLite.
- **Back up** by copying the `.db`, `.db-wal`, and `.db-shm` files while maintenant is stopped, or use `sqlite3 .backup` while running.
- **File permissions** — ensure only the maintenant process can read/write the database file.

### Heartbeat UUIDs

Ping URLs (`/ping/{uuid}`) use UUIDs as the sole access control. Anyone who knows the UUID can send pings.

- Treat heartbeat UUIDs as **secrets**.
- Do not commit them to public repositories.
- Do not log them in CI/CD output.
- Rotate a heartbeat's UUID by deleting and recreating it if you suspect a leak.

### Webhook URLs

When creating webhooks, maintenant enforces **HTTPS-only** URLs. This prevents credentials in webhook payloads from being transmitted in cleartext.

---

## Security Checklist

A quick reference for securing your deployment:

- [ ] Reverse proxy in front of maintenant with authentication enabled
- [ ] `/api/v1/*` and `/` require authentication
- [ ] `/ping/` and `/status/` bypass authentication
- [ ] If MCP is enabled: OAuth2 credentials configured (`MAINTENANT_MCP_CLIENT_ID` + `MAINTENANT_MCP_CLIENT_SECRET`)
- [ ] If MCP is enabled: `/mcp`, `/oauth/*`, `/.well-known/*` bypass proxy auth (MCP handles its own)
- [ ] Docker socket mounted read-only (`:ro`)
- [ ] HTTPS termination at the proxy level
- [ ] `MAINTENANT_BASE_URL` set to your public HTTPS URL
- [ ] Database file has restrictive permissions
- [ ] Heartbeat UUIDs not exposed in public repositories or logs
- [ ] `MAINTENANT_CORS_ORIGINS` set appropriately (not `*` in production, unless intended)

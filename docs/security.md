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
| `/manifest.webmanifest` | PWA manifest — the browser fetches it **without credentials**, so it must not sit behind auth | No |

!!! note "Match the prefix, not the exact slash"
    Route the public matcher on the prefix **without** a trailing slash (`/ping`, `/status`). maintenant 301-redirects the bare path to its canonical form (`/status` → `/status/`); if your matcher only covers `/status/`, the bare `/status` reaches the authenticated router first and returns 401 before the redirect. Note that `/ping` and `/ping/` themselves return **404** — only `/ping/{uuid}` is a real endpoint; the prefix just needs to bypass auth so heartbeats reach the app.

### MCP & OAuth Routes

Only registered when `MAINTENANT_MCP=true`. If MCP uses OAuth2 (recommended), these routes handle the OAuth flow and **must bypass proxy-level auth** — MCP has its own authentication:

| Route | Purpose |
|-------|---------|
| `/mcp` | MCP Streamable HTTP endpoint. Protected by OAuth2 bearer token when credentials are configured. |
| `/.well-known/oauth-authorization-server` | OAuth2 server metadata discovery (RFC 8414) |
| `/.well-known/oauth-protected-resource` | Protected resource metadata (RFC 9728) |
| `/oauth/authorize` | Authorization endpoint (PKCE S256 mandatory) |
| `/oauth/token` | Token exchange and refresh |

!!! warning "MCP without credentials refuses to start"
    `MAINTENANT_MCP=true` **without** `MAINTENANT_MCP_CLIENT_ID` and `MAINTENANT_MCP_CLIENT_SECRET` used to serve `/mcp` to anyone. Since `/mcp` is documented above as bypassing proxy auth, an incomplete `.env` published your monitoring data. maintenant now refuses to listen and names the missing variables.

    To run MCP unauthenticated anyway — a local instance on a trusted network — set `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED=true`. The startup log then carries a `WARN` for as long as it stays that way. `--mcp-stdio` is unaffected: it never listens on the network and needs no credentials.

### Protected Routes

These routes provide full read/write access to your monitoring system. **Always require authentication** via your reverse proxy:

| Route | Purpose |
|-------|---------|
| `/api/v1/*` | Admin API — containers, endpoints, heartbeats, certificates, alerts, webhooks, status page management, update intelligence, resources |
| `/` | Dashboard (Vue SPA) |

!!! danger "Do not expose `/api/v1/` without authentication"
    The admin API provides unrestricted access to all monitoring data and configuration: creating webhooks, managing heartbeats, viewing container logs, acknowledging alerts, and more. There is no **user-level** authorization layer — any authenticated request that reaches the API is trusted, so the reverse proxy is your only access control.

!!! note "A 403 on `/api/v1/*` is edition gating, not the proxy"
    Some routes are reserved for the Pro edition (e.g. `/api/v1/agents`, `/api/v1/swarm/*`, `/api/v1/security/posture`, `/api/v1/cve`). In Community edition they return `403 PRO_REQUIRED` regardless of authentication — this is feature gating, not a reverse-proxy or trust problem. The dashboard hides these features, so a correctly-loaded SPA never calls them. Edition itself is reported by `/api/v1/edition` (always public-readable behind auth).

---

## Reverse Proxy Setup

### Traefik + Authelia

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    read_only: true
    security_opt:
      - no-new-privileges:true
    group_add:
      - "${DOCKER_GID:-983}"
    tmpfs:
      - /tmp:noexec,nosuid,size=64m
    labels:
      traefik.enable: "true"

      # Main router — requires authentication
      traefik.http.routers.maintenant.rule: "Host(`now.example.com`)"
      traefik.http.routers.maintenant.middlewares: "authelia@docker"

      # Public routes — no auth
      traefik.http.routers.maintenant-public.rule: >
        Host(`now.example.com`) &&
        (PathPrefix(`/ping`) || PathPrefix(`/status`) || Path(`/manifest.webmanifest`))
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
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"
      MAINTENANT_BASE_URL: "https://now.example.com"
```

### Caddy + Authelia

```
now.example.com {
    # Public routes — no auth
    @public path /ping /ping/* /status /status/* /manifest.webmanifest
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
    location ~ ^/(ping|status)(/|$) {
        proxy_pass http://127.0.0.1:8080;
    }
    location = /manifest.webmanifest {
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

Two per-IP token bucket rate limiters, one tight for the public surfaces and one loose for the admin API:

| Setting | Public routes | Admin API |
|---------|---------------|-----------|
| Applied to | `/ping/`, `/status/*`, `/mcp` | `/api/v1/*` |
| Rate | 10 requests/second per IP | 50 requests/second per IP |
| Burst | 20 requests | 200 requests |
| 429 response | `{"error":{"code":"rate_limited","message":"Too many requests"}}` with `Retry-After: 1` | Same |

The admin API limit is a flood ceiling, not a quota. A dashboard page load fans out dozens of parallel calls, so the public bucket would reject ordinary use; at 50/s with a burst of 200 the interface never reaches it. It exists so an unauthenticated surface, or a stolen session, cannot hammer expensive routes for free.

IP detection priority: `X-Real-IP` header → first entry in `X-Forwarded-For` → `RemoteAddr`.

The `/status/subscribe` endpoint has an additional rate limit of 5 requests per IP per hour to prevent subscription abuse.

!!! tip "Trusted proxies"
    Make sure your reverse proxy sets `X-Real-IP` or `X-Forwarded-For` correctly. Without it, all requests appear to come from the proxy's IP and share a single rate limit bucket.

### Security Headers

Every response carries:

| Header | Value | Purpose |
|--------|-------|---------|
| `X-Content-Type-Options` | `nosniff` | Stops the browser from re-guessing a declared content type |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Keeps paths (which can carry heartbeat UUIDs) out of cross-origin referrers |
| `X-Frame-Options` | `DENY` | Clickjacking defence for browsers that ignore CSP |
| `Content-Security-Policy` | see below | Defence in depth if an XSS ever lands |

The policy is `default-src 'self'` with `object-src 'none'`, `base-uri 'self'` and `form-action 'self'`. `script-src` is `'self'` plus the SHA-256 of the inline theme bootstrap in `index.html`, computed at startup from the embedded asset — never `'unsafe-inline'`. `style-src` does allow `'unsafe-inline'`, because Vue and uPlot both set element styles at runtime. `img-src` allows `data:` for status page logos and hero images.

!!! note "The public status page stays embeddable"
    `/status` and `/status/*` are served with `frame-ancestors *` and no `X-Frame-Options`, so you can keep iframing the status page. Every other route — the dashboard and `/api/v1/*` — gets `frame-ancestors 'none'` and `X-Frame-Options: DENY`.

HSTS is deliberately **not** set by the application. TLS terminates at your reverse proxy, and the app would have to infer "this was HTTPS" from a forwarded header a client can forge. Set `Strict-Transport-Security` in the proxy.

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

## Secrets at Rest

Everything the database holds that could be replayed is stored hashed, so a copy of the file is not a copy of your credentials:

| Secret | Stored as |
|--------|-----------|
| Agent enrollment tokens | `sha256(token)` + a 14-character display prefix |
| MCP OAuth authorization codes | `sha256(code)` |
| MCP access and refresh tokens | `sha256(token)` |
| MCP client secret | `sha256(secret)`, compared in constant time |
| Agent identity | Ed25519 **public** key only — the private key never leaves the agent host |
| License key | Signature verified with a build-injected public key; disk cache is `0600` |

Status page subscriber tokens (`confirm_token`, `unsub_token`) are still stored in the clear. They only confirm or cancel an email subscription, and the same table holds the subscriber addresses in the clear by nature, so hashing them would not protect the sensitive part of that table.

## Deployment Hardening

### Container Security

The maintenant Docker image runs as **`nobody:nobody`** (uid/gid 65534) — never as root. Combined with the Compose security options, the container operates with minimal privileges:

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    read_only: true                    # immutable root filesystem
    security_opt:
      - no-new-privileges:true         # prevent privilege escalation
    tmpfs:
      - /tmp:noexec,nosuid,size=64m    # scratch space for SQLite WAL
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - maintenant-data:/data
```

| Setting | Purpose |
|---------|---------|
| Entrypoint drops to uid 65534 via `setpriv` | Process runs as `nobody`, not root |
| `read_only: true` | Root filesystem is immutable — no writes outside mounted volumes |
| `no-new-privileges` | Blocks `setuid`/`setgid` binaries and privilege escalation |
| Entrypoint auto-detects the socket group | `nobody` can read the mounted socket without `group_add` |
| `tmpfs /tmp` | Writable scratch space with `noexec` and `nosuid` flags |

!!! tip "Docker socket access is automatic"
    The entrypoint reads the group of the mounted `/var/run/docker.sock` and grants the
    unprivileged user access to it — no `group_add` needed, on plain Compose **and** Swarm.
    Set `DOCKER_GID` (e.g. from `stat -c '%g' /var/run/docker.sock`) only to override the
    detected GID, for a non-standard socket path, or for a socket proxy.

### Docker Socket

maintenant needs access to the Docker API to discover and monitor containers. Its entire API surface is **read-only**: container list/inspect/stats/logs, events, version/info, network metadata and — on Swarm managers — nodes, services and tasks. It never creates, modifies, or deletes anything.

!!! danger "`:ro` on the socket is not a security boundary"
    Mounting the socket read-only only protects the socket *file*. The Docker API behind it
    still accepts writes: any process holding the socket — `:ro` or not — can stop containers,
    start privileged ones, and escalate to root on the host. The only real boundary is a
    filtering proxy in front of the socket.

#### Recommended: Docker Socket Proxy

Run maintenant behind [Tecnativa/docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) and point it at the proxy with `DOCKER_HOST` — maintenant's Docker client honours the variable natively, so no socket mount is needed at all:

```yaml
services:
  socketproxy:
    image: tecnativa/docker-socket-proxy:latest
    environment:
      # The only endpoint groups maintenant needs — all read-only:
      - CONTAINERS=1        # discovery, inspect, stats, logs
      - INFO=1              # runtime + Swarm detection
      - NETWORKS=1          # network metadata
      # EVENTS, PING and VERSION are already enabled by default.
      # POST defaults to 0 -> every write returns 403.
      # On a Swarm manager, also enable: SWARM=1, NODES=1, SERVICES=1, TASKS=1
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks: [dockerapi]
    read_only: true
    tmpfs: [/run, /tmp]   # the proxy writes haproxy.cfg to /tmp
    security_opt:
      - no-new-privileges:true
    restart: unless-stopped

  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    environment:
      DOCKER_HOST: tcp://socketproxy:2375
      # ... your other settings
    volumes:
      - /proc:/host/proc:ro
      - maintenant-data:/data
      # no docker.sock mount
    networks: [dockerapi, web]
    depends_on: [socketproxy]

networks:
  dockerapi:
    internal: true
```

Key points:

- The proxy is the root-equivalent component. Keep it on an **internal network** and never publish port 2375 on the host.
- With this setup, even a fully compromised maintenant could only *read* the Docker API — the proxy answers `403` to every write.
- If maintenant starts before the proxy is reachable, it boots in degraded mode and reconnects automatically once the proxy is up.
- Works identically in **agent mode**: the agent uses the same Docker client, so a per-host socket proxy plus `DOCKER_HOST` replaces the socket mount there too.

#### Alternative: direct socket mount

The simpler setup mounts the socket directly:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
  - /proc:/host/proc:ro
```

This relies on maintenant *behaving* read-only (which it does) rather than *enforcing* it. The non-root user, `read_only: true` filesystem and `no-new-privileges` remain worthwhile defense-in-depth, but be clear about the trade-off: whoever holds the socket holds the host.

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

- [ ] Container runs as non-root (uid 65534, dropped via `setpriv` in the entrypoint — default in the official image)
- [ ] `read_only: true` — immutable root filesystem
- [ ] `no-new-privileges:true` — blocks privilege escalation
- [ ] **Preferred:** Docker API accessed through a [socket proxy](#recommended-docker-socket-proxy) (`DOCKER_HOST=tcp://socketproxy:2375`, no socket mount, writes rejected at the proxy)
- [ ] Otherwise: Docker socket mounted read-only (`:ro`) — its group is auto-detected; `group_add` not required (remember `:ro` does not block API writes)
- [ ] (Optional) `DOCKER_GID` set to override the detected socket GID (non-standard path or unix-socket proxy)
- [ ] Reverse proxy in front of maintenant with authentication enabled
- [ ] `/api/v1/*` and `/` require authentication
- [ ] `/ping`, `/status` (prefix, no trailing slash) and `/manifest.webmanifest` bypass authentication
- [ ] If MCP is enabled: OAuth2 credentials configured (`MAINTENANT_MCP_CLIENT_ID` + `MAINTENANT_MCP_CLIENT_SECRET`)
- [ ] If MCP is enabled: `/mcp`, `/oauth/*`, `/.well-known/*` bypass proxy auth (MCP handles its own)
- [ ] `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED` **not** set (it is only for a local instance on a trusted network)
- [ ] HTTPS termination at the proxy level
- [ ] `Strict-Transport-Security` set at the proxy (the app cannot set it safely)
- [ ] `MAINTENANT_BASE_URL` set to your public HTTPS URL
- [ ] Database file has restrictive permissions
- [ ] Heartbeat UUIDs not exposed in public repositories or logs
- [ ] `MAINTENANT_CORS_ORIGINS` set appropriately (not `*` in production, unless intended)

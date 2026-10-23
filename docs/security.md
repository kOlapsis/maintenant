# Security

maintenant has no user accounts and no login page: the dashboard and the admin API rely on your reverse proxy and its authentication middleware, so there is no separate set of users to manage. Two surfaces carry credentials of their own and are meant to be reachable without the proxy's authentication: the MCP endpoint (OAuth client credentials) and the agent gRPC listener (enrollment token and Ed25519 key).

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
| `/ping/{token}` | Heartbeat ping, called by cron jobs, CI/CD pipelines, external services | Yes |
| `/ping/{token}/start` | Job start signal for duration tracking | Yes |
| `/ping/{token}/{exit_code}` | Job completion with exit code | Yes |
| `/status/` | Status page (the web app, served at `/status`) | Yes |
| `/status/api` | Status page JSON API | Yes |
| `/status/settings.json` | Status page appearance settings | Yes |
| `/status/events` | SSE stream of status changes | Yes |
| `/status/feed.atom` | Atom feed of incidents | Yes |
| `/status/subscribe` | Email subscription to status updates | Yes |
| `/status/confirm` | Subscription confirmation (token-based) | Yes |
| `/status/unsubscribe` | Unsubscribe (token-based) | Yes |
| `/assets/*` | JavaScript and CSS of the web app. Without them the status page stays blank when it shares a host with the protected dashboard | No |

!!! note "Match the prefix, not the exact slash"
    Route the public matcher on the prefix **without** a trailing slash (`/ping`, `/status`). maintenant serves the status page on both `/status` and `/status/`; if your matcher only covers `/status/`, the bare `/status` reaches the authenticated router and returns 401. `/ping` redirects to `/ping/`, which returns **404**: only `/ping/{token}` is a real endpoint, the prefix just needs to bypass auth so heartbeats reach the app.

!!! note "Optional public routes"
    The web app also requests `/manifest.webmanifest`, `/registerSW.js`, `/sw.js` (with its `workbox-*.js` file) and the icons (`/favicon.svg`, `/favicon.ico`, `/apple-touch-icon.png`) from the root. The status page renders without them. Behind authentication, an anonymous visitor only loses the icons and the PWA features (install, offline cache), and the browser console shows failed requests. The manifest is requested with credentials, so signed-in users still get it.

!!! note "Dedicated status host"
    When `MAINTENANT_STATUS_URL` points at its own subdomain, maintenant answers `404` on that host to every path outside the public ones above. Restrict the proxy router of that subdomain the same way: see the [Traefik rule in the status page guide](features/status-page.md#subdomain-deployment).

### MCP & OAuth Routes

Only registered when `MAINTENANT_MCP=true`, and never in [demo mode](#demo-mode). The OAuth routes exist only when both `MAINTENANT_MCP_CLIENT_ID` and `MAINTENANT_MCP_CLIENT_SECRET` are set. They handle the OAuth flow and **must bypass proxy-level auth**, because MCP has its own authentication (see [MCP Authentication](#mcp-authentication)):

| Route | Purpose |
|-------|---------|
| `/mcp` | MCP Streamable HTTP endpoint (`/mcp` and `/mcp/`). Protected by an OAuth2 bearer token when credentials are configured. |
| `/.well-known/oauth-authorization-server` | OAuth2 server metadata discovery (RFC 8414) |
| `/.well-known/oauth-protected-resource` | Protected resource metadata (RFC 9728) |
| `/oauth/authorize` | Authorization endpoint (PKCE S256 mandatory). It does not authenticate a user: see [MCP Authentication](#mcp-authentication). |
| `/oauth/token` | Token exchange and refresh. It checks the client secret. |

!!! warning "MCP without credentials refuses to start"
    `MAINTENANT_MCP=true` **without** `MAINTENANT_MCP_CLIENT_ID` and `MAINTENANT_MCP_CLIENT_SECRET` would serve `/mcp` to anyone. Since `/mcp` bypasses proxy auth, an incomplete `.env` would publish your monitoring data, so maintenant refuses to listen and names the missing variables.

    To run MCP unauthenticated anyway (a local instance on a trusted network), set `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED=true`. The startup log then carries a `WARN` for as long as it stays that way, and only `/mcp` is registered: there are no OAuth routes.

The local `mcp.sock` socket that `--mcp-stdio` relays to is not an HTTP route and never goes through the proxy. It is created only when `MAINTENANT_MCP=true` or `MAINTENANT_MCP_SOCKET=true`, next to the database with mode `0600`, and never in agent mode.

### Protected Routes

These routes provide full read/write access to your monitoring system. **Always require authentication** via your reverse proxy:

| Route | Purpose |
|-------|---------|
| `/api/v1/*` | Admin API: containers, endpoints, heartbeats, certificates, alerts, webhooks, status page management, update intelligence, resources |
| `/` | Dashboard (Vue SPA) |

`/api/v1/health` is part of the admin API: it reports the version, the runtime connection and the storage engine, so it stays behind authentication too. The container health check calls it directly on the listener, not through the proxy, and needs no exception.

!!! danger "Do not expose `/api/v1/` without authentication"
    The admin API provides unrestricted access to all monitoring data and configuration: creating webhooks, managing heartbeats, viewing container logs, acknowledging alerts, and more. There is no **user-level** authorization layer: any request that reaches the API is trusted, so the reverse proxy is your only access control.

!!! note "A 403 on `/api/v1/*` is edition gating, not the proxy"
    Some routes need a paid edition (e.g. `/api/v1/agents` and `/api/v1/security/posture` from Personal, `/api/v1/escalation-policies` from Pro). Below the required edition they return `403 EDITION_REQUIRED` regardless of authentication, naming the capability and the edition that grants it. This is feature gating, not a reverse-proxy or trust problem. The dashboard hides these features, so a correctly-loaded SPA never calls them. The edition and the capability catalogue are reported by `GET /api/v1/edition`, which sits behind your proxy's authentication like the rest of `/api/v1/`.

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
    tmpfs:
      - /tmp:noexec,nosuid,size=64m
    labels:
      traefik.enable: "true"

      # Main router: requires authentication
      traefik.http.routers.maintenant.rule: "Host(`now.example.com`)"
      traefik.http.routers.maintenant.middlewares: "authelia@docker"

      # Public routes: no auth
      traefik.http.routers.maintenant-public.rule: >
        Host(`now.example.com`) &&
        (PathPrefix(`/ping`) || PathPrefix(`/status`) || PathPrefix(`/assets/`))
      traefik.http.routers.maintenant-public.priority: "100"

      # MCP + OAuth routes: MCP handles its own auth
      traefik.http.routers.maintenant-mcp.rule: >
        Host(`now.example.com`) &&
        (PathPrefix(`/mcp`) || PathPrefix(`/oauth/`) || PathPrefix(`/.well-known/`))
      traefik.http.routers.maintenant-mcp.priority: "100"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - /etc/os-release:/host/etc/os-release:ro
      - maintenant-data:/data
    environment:
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"
      MAINTENANT_BASE_URL: "https://now.example.com"
      MAINTENANT_TRUSTED_PROXIES: "172.18.0.0/16"  # subnet of the Docker network shared with Traefik
```

### Caddy + Authelia

```
now.example.com {
    # Public routes: no auth
    @public path /ping /ping/* /status /status/* /assets/*
    handle @public {
        reverse_proxy maintenant:8080
    }

    # MCP and OAuth routes: MCP handles its own auth
    @mcp path /mcp /mcp/* /oauth/* /.well-known/*
    handle @mcp {
        reverse_proxy maintenant:8080
    }

    # Everything else: requires auth
    handle {
        forward_auth authelia:9091 {
            uri /api/verify?rd=https://auth.example.com
            copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
        }
        reverse_proxy maintenant:8080
    }
}
```

Keep the `handle` blocks. Caddy orders directives itself, and a bare `forward_auth` runs before every `reverse_proxy` whatever their order in the file, which would put the public and MCP routes behind authentication too.

Set `MAINTENANT_BASE_URL` and `MAINTENANT_TRUSTED_PROXIES` on maintenant as in the Traefik example; Caddy sets `X-Forwarded-For` itself.

### nginx + OAuth2 Proxy

```nginx
server {
    listen 443 ssl;
    server_name now.example.com;

    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

    # Public routes: no auth
    location ~ ^/(ping|status)(/|$) {
        proxy_pass http://127.0.0.1:8080;
    }
    location /assets/ {
        proxy_pass http://127.0.0.1:8080;
    }

    # MCP and OAuth routes: MCP handles its own auth
    location ~ ^/(mcp|oauth|\.well-known)(/|$) {
        proxy_pass http://127.0.0.1:8080;
        proxy_read_timeout 86400s;
    }

    # Protected routes: requires auth
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

The `(/|$)` ending matters: `/mcp` has no trailing slash, and a pattern that requires one sends it to the authenticated `location /`. `X-Forwarded-For` is what `MAINTENANT_TRUSTED_PROXIES` relies on; `Host` is used by the [cross-origin check](#csrf-protection) and to build the agent enrolment URL.

!!! tip "Streaming routes and proxy timeouts"
    `/api/v1/containers/events`, `/api/v1/containers/{id}/logs/stream`, `/status/events` and every `/mcp` response are streams. maintenant sends `X-Accel-Buffering: no` on them, so nginx relays them as they are written without `proxy_buffering off`. The three event streams (`/api/v1/containers/events`, `/api/v1/containers/{id}/logs/stream` and `/status/events`) send a keep-alive comment every 25 seconds, which stays under the 60 seconds that nginx waits by default (`proxy_read_timeout`) before it closes a quiet connection. `/mcp` sends none, so raise the read timeout on that path, as the example above does. Caddy handles streaming natively, no special configuration needed.

---

### Running behind a proxy

`MAINTENANT_TRUSTED_PROXIES` takes a comma-separated list of CIDRs and bare
addresses. Set it whenever a proxy sits in front of maintenant, whichever proxy
you use:

```bash
MAINTENANT_TRUSTED_PROXIES=10.0.0.0/8,192.168.1.4,2001:db8::/32
```

It is empty by default, and an empty list means **no forwarded header is
believed**: `X-Forwarded-For`, `X-Real-IP` and `X-Forwarded-Host` are not read
at all, and every rate-limit quota is counted against the address that opened
the connection. Anyone able to reach the instance directly can otherwise send a
different header on every request and get a fresh quota each time. Behind a
proxy that is not listed, every visitor looks like the proxy itself: status page
visitors and MCP clients share one public bucket (10 requests per second, burst
of 20), pings with an unknown token share their own bucket of the same size,
and every dashboard user shares the API bucket. Pings with a known token are
counted per token and are not affected. maintenant logs one `WARN` naming
`MAINTENANT_TRUSTED_PROXIES` the first time a request carries `X-Forwarded-For`
or `X-Real-IP` while the list is empty.

The list has to contain the address maintenant sees as the connecting peer,
which depends on where the proxy runs:

| Proxy | Peer address seen by maintenant |
|-------|---------------------------------|
| A container on a Docker network shared with maintenant | The subnet of that network: `docker network inspect <network> --format '{{(index .IPAM.Config 0).Subnet}}'` |
| On the host, with maintenant in a container that publishes its port | The gateway of the container's network, because Docker rewrites host traffic (`172.17.0.1` on the default bridge): `docker network inspect <network> --format '{{(index .IPAM.Config 0).Gateway}}'` |
| On the host, with the native install | `127.0.0.1` |

The proxy must also pass the client address on. Traefik and Caddy set
`X-Forwarded-For` themselves and discard a value sent by the client; nginx needs
the `proxy_set_header` line shown above.

When the list is set, a request is only credited to a forwarded address if its
immediate peer is inside one of the prefixes. `X-Forwarded-For` is then walked
right to left and the first address that is not itself a listed proxy is the
client; entries that do not parse are skipped. `X-Real-IP` is read only when
there is no `X-Forwarded-For` header. When nothing usable is found (every hop is
a listed proxy, or `X-Real-IP` does not parse), the peer address is used.

The same list decides whether `X-Forwarded-Host` may name the gRPC URL handed to
a new agent at enrolment; without a believed `X-Forwarded-Host` that URL is built
from the `Host` header of the request, so the proxy has to forward it.
`X-Forwarded-Proto: grpc` never downgrades that URL to plaintext: the answer
stays `grpcs://` and carries a `public_url_plaintext_refused` warning.

A value that does not parse refuses to start rather than being ignored.

---

## External database credentials

When the server is pointed at a PostgreSQL with `MAINTENANT_DATABASE_URL`, the
connection string carries a password. It is never written to the logs (at any
level, including inside wrapped errors), never returned by the API, never
rendered in the interface, and never sent with the telemetry. Where a target
must be named it appears redacted:
`postgres://maintenant@db.internal:5432/maintenant`. A dedicated test injects a
sentinel password and fails on any output containing it.

**Transport is encrypted by default.** When the connection string carries no
explicit `sslmode` and the host is not `localhost`, `127.0.0.1`, `::1` or a Unix
socket (a Compose service named `db` is not local), the product adds
`sslmode=require`. An explicit value always wins, `disable` included, so a
database on a trusted network can be relaxed deliberately. A server that does
not accept TLS stops the start with a message naming both ways out: enable TLS
on the server, or set `sslmode` explicitly. For a database reached across a
network you do not control, prefer `sslmode=verify-full`: it also checks the
server certificate against its hostname, which `require` does not.

Give the instance a role scoped to its own database. It needs to create its
schema on first start and read/write it afterwards; nothing more.

## Built-in Protections

### Rate Limiting

Token bucket limiters, tight for the public surfaces and loose for the admin API. Each one has its own buckets, so a flood on one surface does not use up the quota of another:

| Limiter | Applied to | Rate | Burst | Counted per |
|---------|------------|------|-------|-------------|
| Public | every `/status/` route, `/mcp`, `/oauth/authorize`, `/oauth/token` | 10 requests/second | 20 | address |
| Pings, unknown token | `/ping/` with a token no heartbeat has | 10 requests/second | 20 | address |
| Pings, known token | `/ping/{token}`, `/ping/{token}/start`, `/ping/{token}/{exit_code}` | 60 per minute | 60 | token, whatever the address |
| Admin API | `/api/*` | 50 requests/second | 200 | address |
| Subscription | `POST /status/subscribe`, in addition to the public limiter | 5 requests/hour | 5 | address |

A flood of invalid tokens from one address therefore never blocks a valid heartbeat ping from the same address, and a refused ping is not recorded. A refused request gets `429` with `{"error":{"code":"rate_limited","message":"Too many requests"}}` and a `Retry-After` header: 1 second for the public, ping and admin API limiters, 720 seconds for subscriptions. The OAuth metadata documents under `/.well-known/` and the web app's own files (`/`, `/assets/*`) are not limited.

The admin API limit is a flood ceiling, not a quota. A dashboard page load fans out dozens of parallel calls, so the public bucket would reject ordinary use; at 50/s with a burst of 200 the interface never reaches it. It exists so an unauthenticated surface, or a stolen session, cannot hammer expensive routes for free.

Which address a quota is counted against is decided by
`MAINTENANT_TRUSTED_PROXIES`: see [Running behind a proxy](#running-behind-a-proxy). Agent connections have a separate per-agent limit, described in [Agent Connections](#agent-connections-grpc).

### Security Headers

| Header | Value | Sent on | Purpose |
|--------|-------|---------|---------|
| `X-Content-Type-Options` | `nosniff` | Every response | Stops the browser from re-guessing a declared content type |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Every response | Keeps paths (which can carry heartbeat ping tokens) out of cross-origin referrers |
| `X-Frame-Options` | `DENY` | Every response except `/status` and `/status/*` | Clickjacking defence for browsers that ignore CSP |
| `Content-Security-Policy` | see below | Every response | Defence in depth if an XSS ever lands |

The policy is:

```
default-src 'self'; script-src 'self' 'sha256-<inline bootstrap>'; style-src 'self' 'unsafe-inline';
img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; worker-src 'self';
manifest-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'
```

`script-src` is `'self'` plus the SHA-256 of each inline script in `index.html` (the theme bootstrap), computed at startup from the embedded asset: never `'unsafe-inline'`. `style-src` does allow `'unsafe-inline'`, because Vue and uPlot both set element styles at runtime. `img-src` allows `data:` for status page logos and hero images, which are served as data URLs.

The route that serves the uploaded status page images (`/api/v1/status-page/assets/{role}`) answers with a stricter policy of its own, `default-src 'none'; style-src 'unsafe-inline'`, which neutralizes scripts in an SVG opened directly.

!!! note "The public status page stays embeddable"
    `/status` and `/status/*` are served with `frame-ancestors *` and no `X-Frame-Options`, so you can keep iframing the status page. Every other route, the dashboard and `/api/v1/*` included, gets `frame-ancestors 'none'` and `X-Frame-Options: DENY`.

HSTS is deliberately **not** set by the application. TLS terminates at your reverse proxy, and the app would have to infer "this was HTTPS" from a forwarded header a client can forge. Set `Strict-Transport-Security` in the proxy.

Every `/api/v1/` answer carries `Cache-Control: no-store`, so neither the browser nor an intermediate cache keeps it; the SSE streams send `no-cache` instead. The web app's service worker keeps a short-lived cache of the other API answers (prefix `api-`) for offline use, but never caches container logs, status page subscribers, escalation policies, agents, security findings or CVEs: those always come from the network. When the authentication proxy session expires, the web app deletes every `api-` cache before sending the browser back to the login page.

### Request Size Limits

Request bodies on `/api/` and `/ping/` are limited to **1 MB** by default, whatever the method. Configurable via `MAINTENANT_MAX_BODY_SIZE` (in bytes; a value that is not a positive whole number stops the startup with an error that names the variable). The public `/status/` routes have a fixed limit of 4 KiB. `/mcp` and `/oauth/*` set no size limit of their own.

### Request Timeouts

The server gives a request 5 seconds to be read (`ReadTimeout`) and closes idle keep-alive connections after 120 seconds. It sets no write timeout, because streaming responses need to stay open; instead, a 10-second timeout is enforced on all non-streaming routes, which answer `503` when it expires. Streaming paths are exempt:

- `/api/v1/containers/events` (SSE)
- `/api/v1/containers/{id}/logs/stream` (SSE)
- `/status/events` (SSE)
- `/mcp` (MCP Streamable HTTP)

### CORS

Controlled by `MAINTENANT_CORS_ORIGINS`:

| Value | Behavior |
|-------|----------|
| Unset (default) | No CORS headers: same-origin only |
| `*` | `Access-Control-Allow-Origin: *`. Writes from other origins are still refused, see [CSRF protection](#csrf-protection) |
| Comma-separated list of origins | Allowlist: the matching `Origin` is echoed with `Vary: Origin`. These origins are also trusted for writes |

Each entry is compared as a whole with the `Origin` header sent by the browser, so it has the form `scheme://host[:port]`, without a path or a trailing slash. When the variable is set, responses also carry `Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS` and `Access-Control-Allow-Headers: Content-Type, Authorization`. `OPTIONS` requests are answered with `204`.

The setting applies to `/api/` and `/ping/`. Four public responses always carry `Access-Control-Allow-Origin: *` regardless of it, since they are designed to be embedded or discovered from anywhere: `/status/api`, `/status/settings.json`, `/.well-known/oauth-authorization-server` and `/.well-known/oauth-protected-resource`.

### CSRF protection

The proxy's session cookie travels with every request a browser makes, including one triggered by a page on another site. `/api/*` therefore refuses a request with an unsafe method (anything but `GET`, `HEAD` and `OPTIONS`) that comes from another origin, with `403` and the error code `CROSS_ORIGIN_REFUSED`. The check is Go's `http.CrossOriginProtection`:

- These pass: same-origin requests, requests the user started directly (`Sec-Fetch-Site: none`), requests carrying neither `Sec-Fetch-Site` nor `Origin` (curl, scripts, other servers), requests whose `Origin` has the same host as the `Host` header when the browser sends no `Sec-Fetch-Site`, and requests from an origin listed in `MAINTENANT_CORS_ORIGINS`.
- Everything else is refused, including requests from another subdomain of the same site (`Sec-Fetch-Site: same-site`).

`MAINTENANT_CORS_ORIGINS` is the list of trusted origins. An entry that is not of the form `scheme://host[:port]` is ignored and logged as a `WARN` at startup. `*` trusts no origin: it never lets another site write. Browsers without `Sec-Fetch-Site` (released before 2023) behind a proxy that rewrites `Host` have their writes refused, so forward the original `Host` as the examples above do.

`/ping/`, `/status/`, `/mcp` and `/oauth/*` are outside this check: they are designed to be called from other origins, and `/mcp` has its own cross-origin check, built into the MCP SDK.

### Host header

A DNS rebinding attack points a domain the attacker controls at the address of your instance, so a page on that domain can read the API and the dashboard from the visitor's browser. maintenant therefore checks the `Host` header: outside the public routes, a request whose host name is not accepted gets `421` with `{"error":{"code":"misdirected_request","message":"Host not allowed: add it to MAINTENANT_BASE_URL or MAINTENANT_ALLOWED_HOSTS"}}`. A request without a `Host` header is refused the same way. The port and the case of the header do not matter.

An IP address is always accepted, IPv4 or IPv6, because a rebinding attack goes through a domain name and never through an IP address. Opening the dashboard at `http://192.168.1.10:8080` therefore works without any setting. Only host names have to be declared.

The accepted host names are `localhost`, the host of `MAINTENANT_ADDR` (unless it is `0.0.0.0` or `::`), and either the entries of `MAINTENANT_ALLOWED_HOSTS` or, when it is not set, the hosts of `MAINTENANT_BASE_URL` and `MAINTENANT_STATUS_URL`:

```bash
MAINTENANT_ALLOWED_HOSTS=now.example.com,monitor.lan
```

The check does not apply to `/ping/`, `/status` and `/status/`, `/assets/`, the installable app files (`/manifest.webmanifest`, `/sw.js`, `/registerSW.js`, `/workbox-*`, the icons), `/mcp`, `/oauth/` and `/.well-known/`: the public routes answer any host so they can be embedded and reached through any name, and `/mcp` checks the host itself.

When maintenant listens beyond the loopback without `MAINTENANT_ALLOWED_HOSTS`, it logs one `WARN` at startup listing the hosts it accepts. The proxy must forward the original `Host` (Traefik and Caddy do by default, nginx needs `proxy_set_header Host $host`), and the Kubernetes probes send `Host: localhost`.

---

## MCP Authentication

When `MAINTENANT_MCP_CLIENT_ID` and `MAINTENANT_MCP_CLIENT_SECRET` are both configured, the MCP endpoint is protected by an OAuth 2.1 authorization server built into maintenant. It is deliberately small: one static client that authenticates with `client_secret_post`, the authorization code grant with PKCE and the refresh token grant. There is no dynamic client registration.

!!! warning "The client secret is the only thing that keeps `/mcp` closed"
    `/oauth/authorize` does not authenticate a user. It issues an authorization code to anyone who presents the configured client ID, a PKCE challenge and an acceptable redirect URI, and the client secret is checked only at `/oauth/token`. Because these routes bypass the proxy, anyone who knows the secret can obtain a token. Use a long random value, for example `openssl rand -hex 32`. maintenant logs a `WARN` at startup when the secret is shorter than 32 characters.

- **PKCE S256** mandatory on all authorization requests
- **Redirect URIs**: loopback addresses (`localhost`, `127.0.0.1`, `::1`, over http or https) are always accepted. Any other redirect URI has to be listed, exactly as sent, in `MAINTENANT_MCP_ALLOWED_REDIRECT_URIS` (comma-separated)
- **Issuer**: `MAINTENANT_BASE_URL` (default `http://<MAINTENANT_ADDR>`) is the issuer and the base of every endpoint in the metadata. Set it to your public HTTPS URL, or clients are sent to an address they cannot reach
- **Opaque tokens**: 32 random bytes; authorization codes, access tokens and refresh tokens are stored as SHA-256 hashes (a database leak does not expose usable tokens)
- **Client secret**: not stored. It is read from the environment at startup, hashed in memory and compared in constant time
- **Authorization codes** expire in 10 minutes and work once; **access tokens** expire after 1 hour, **refresh tokens** after 30 days
- **Refresh token rotation**: each use invalidates the old token
- **Replay detection**: reusing a consumed refresh token revokes the entire token family
- **Automatic cleanup** of expired tokens every 15 minutes
- **Scopes**: a token is `read` unless the client asked for `scope=read write`. A `read` token calls the read-only tools only; a write tool answers `tool <name> requires the write scope; this token only grants read` and writes nothing. Any other scope is refused with `invalid_scope`, a refresh keeps the scope it had, and tokens issued before scopes existed count as `read`
- **Read-only mode**: with `MAINTENANT_MCP_READ_ONLY=true`, the 17 write tools are not registered at all, whatever the scope. Without it, `/mcp` with a `read write` token is an admin API, as powerful as the dashboard
- **Host header**: with OAuth configured, `/mcp` accepts any `Host`, so a proxy on the same machine can forward the public hostname. With `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED` on a loopback listener, a request with a non-loopback `Host` is refused with `403` (the MCP SDK's DNS rebinding protection)

The stdio transport (`--mcp-stdio`) requires no authentication: it relays to the `mcp.sock` Unix socket next to the database, which only the user maintenant runs as can open (mode `0600`, set from its creation). The socket is served only when `MAINTENANT_MCP=true` or `MAINTENANT_MCP_SOCKET=true`, never in agent mode, and gives full access to every registered tool: it carries no token, so scopes do not apply, but read-only mode does.

See [MCP Server](features/mcp.md) for full configuration and usage details.

---

## Agent Connections (gRPC)

In server and embedded modes, from the Personal edition, maintenant opens a second listener for remote agents on `MAINTENANT_GRPC_LISTEN` (default `127.0.0.1:8443`). On Community it does not start, and the startup log says so. This listener is separate from the HTTP one: the reverse proxy's authentication, the HTTP rate limiters, the security headers and the request timeouts do not apply to it. A proxy in front of it has to pass HTTP/2 (gRPC) or TCP through. Expose it only to the networks your agents connect from. An agent has no HTTP listener at all.

- **TLS**: set `MAINTENANT_GRPC_TLS_CERT` and `MAINTENANT_GRPC_TLS_KEY` to serve your own certificate; giving only one of the two refuses to start. Without them, maintenant generates a self-signed certificate once, valid ten years, and keeps it next to the database in `grpc-server.crt` and `grpc-server.key` (mode `0600`); a missing, unreadable or expired pair is regenerated with a `WARN`, and a data directory it cannot write stops the start. The SHA-256 fingerprint of the served certificate is logged at every start and returned when an enrollment token is created; for a self-signed certificate the install commands pass it to the agent with `--grpc-server-fingerprint`, and the agent then refuses any server whose certificate has another fingerprint, before sending anything. `MAINTENANT_GRPC_INSECURE_SKIP_TLS_VERIFY` still disables verification as a workaround, with a `WARN` on every connection. `MAINTENANT_GRPC_TLS_INSECURE=true` serves plaintext HTTP/2 for a proxy that terminates TLS in front of it, and logs a `WARN`. An agent verifies the server against the system roots plus [`MAINTENANT_CA_CERT`](#private-ca)
- **Enrollment**: an agent enrolls once with a one-time token created in the dashboard. The token expires after 24 hours by default (7 days at most) and is stored as a SHA-256 hash; it is consumed in the same transaction that registers the agent and checks the host limit. The agent generates its Ed25519 key pair locally and sends only the public key
- **Every connection** starts with a challenge. The server sends a random 32-byte nonce, and the agent answers with an Ed25519 signature over the nonce, its agent ID and its timestamp. The timestamp must be within 300 seconds of the server clock, and a revoked agent is refused
- **Rate limit**: once authenticated, an agent may send `MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND` events per second (default 1000). Excess events are refused with a `rate_limited` error that carries a retry delay
- **Limits before authentication**: `RegisterAgent` and the opening of a `Push` stream are limited to 10 per minute per source address (burst of 10), refused beyond with `ResourceExhausted`. The quota is keyed on the TCP peer address: agents behind one NAT or TCP proxy share it. A `Push` client has 30 s from the challenge to send a valid authentication response, after which the stream is closed with `DeadlineExceeded`. A connection that does not complete its handshake within 30 s is dropped. An unknown enrollment token is refused by a read, before any write transaction
- **Message and field limits**: a gRPC message larger than 1 MiB (1,048,576 bytes) is refused with `ResourceExhausted`; a connection carries at most 32 concurrent streams and is closed after 5 minutes without any. At enrollment `agent_id` must be a UUID (stored in lower case), `hostname` is limited to 253 characters, `os_arch` to 32 and `agent_version` to 64, otherwise `InvalidArgument` names the field. A container inventory of more than 10,000 containers, or with a container carrying more than 256 labels, is refused whole with an `invalid_argument` stream error and nothing is written

See [Agent setup](guides/agent-setup.md) for the installation steps.

---

## Demo mode

A demo build is read-only. It is chosen at build time (the `DEMO_MODE=true` build argument of the Dockerfile, published as the `:demo` image), not with a runtime setting.

- Every request that is not `GET`, `HEAD` or `OPTIONS` is answered with `403 DEMO_MODE`, except `POST /api/v1/escalation-policies/overlap-probe`.
- `/ping/*`, `/mcp`, `/oauth/*` and the OAuth metadata are closed with the same `403 DEMO_MODE`. MCP is not registered, and the agent gRPC listener refuses every call.
- A request carrying the header `X-Maintenant-Demo-Token` with the value of `MAINTENANT_DEMO_TOKEN` skips all of the above, so treat that token as a write credential. Without `MAINTENANT_DEMO_TOKEN`, nothing skips it.
- A demo instance monitors one remote Docker endpoint only: `DOCKER_HOST` must be `tcp://host:port`, and `KUBERNETES_SERVICE_HOST` and `KUBECONFIG` must be unset, otherwise it refuses to start.

---

## Secrets at Rest

Credentials that maintenant only has to verify are stored hashed, so a copy of the database file is not a copy of those credentials. Credentials it has to present to other systems must be readable by it, so they are stored in clear text:

| Secret | Stored as |
|--------|-----------|
| Agent enrollment tokens | `sha256(token)` + a 14-character display prefix |
| MCP OAuth authorization codes | `sha256(code)` |
| MCP access and refresh tokens | `sha256(token)` |
| MCP client secret | Not stored: read from `MAINTENANT_MCP_CLIENT_SECRET`, hashed in memory, compared in constant time |
| Agent identity | The server keeps the Ed25519 **public** key only. The private key stays on the agent host, in `identity.json` (mode `0600`) in its data directory |
| License key | Signature verified with a build-injected public key; disk cache is `0600` |
| Notification channel URL, headers and credential (the Telegram bot token) | Clear text. The API never returns the credential |
| Webhook signing secret | Clear text. The API never returns it after creation |
| Heartbeat ping tokens and outbound heartbeat URLs | Clear text: the token is the access control of a ping URL, and the dashboard has to show it |

Status page subscriber tokens (`confirm_token`, `unsub_token`) are also stored in the clear. They only confirm or cancel an email subscription, and the same table holds the subscriber addresses in the clear by nature, so hashing them would not protect the sensitive part of that table.

Credentials given through the environment (`MAINTENANT_SMTP_PASSWORD`, `MAINTENANT_DATABASE_URL`, `MAINTENANT_LICENSE_KEY`) are not written to the database.

## Deployment Hardening

### Container Security

The maintenant process in the official image runs as uid/gid 65534 (`nobody`), never as root. The entrypoint itself starts as root for two things: it hands the data directory to that user (the directory itself, not its content) and it looks up the group of the Docker socket. It then drops privileges with `setpriv` before it runs the binary. Combined with the Compose security options, the container operates with minimal privileges:

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    read_only: true                    # immutable root filesystem
    security_opt:
      - no-new-privileges:true         # prevent privilege escalation
    tmpfs:
      - /tmp:noexec,nosuid,size=64m    # writable scratch space
    environment:
      MAINTENANT_ADDR: "0.0.0.0:8080"  # reachable from the proxy's network
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - /etc/os-release:/host/etc/os-release:ro
      - maintenant-data:/data
```

| Setting | Purpose |
|---------|---------|
| Entrypoint drops to uid 65534 via `setpriv` | The process runs as `nobody`, not root |
| `read_only: true` | Root filesystem is immutable: no writes outside mounted volumes |
| `no-new-privileges` | Blocks `setuid`/`setgid` binaries and privilege escalation |
| Entrypoint detects the socket group | `nobody` can read the mounted socket without `group_add` |
| `tmpfs /tmp` | Writable scratch space with `noexec` and `nosuid` flags. The database, its WAL file and SQLite's temporary files live on `/data` (`SQLITE_TMPDIR=/data`) |

!!! tip "Docker socket access is automatic"
    The entrypoint reads the group of the mounted `/var/run/docker.sock` (or `/run/docker.sock`) and gives the unprivileged user that group: no `group_add` needed, on plain Compose **and** Swarm.
    `DOCKER_GID` (e.g. from `stat -c '%g' /var/run/docker.sock`) adds one more group to the detected ones, it does not replace them. Use it for a socket at another path. It also silences the warning below.

    Detection never grants gid 0: root's group is not something an unprivileged runtime
    gets implicitly. On a host whose socket is `root:root` with no `docker` group (Synology DSM
    Container Manager), the entrypoint logs that the socket is unreachable and container
    discovery stays empty. `DOCKER_GID: "0"` unlocks it explicitly, at the cost of
    root-equivalent access to the host; the [socket proxy](#recommended-docker-socket-proxy)
    below is the better answer there.

    When the container starts as a non-root user (`user:` in Compose, `runAsUser` in Kubernetes), the entrypoint runs the binary as it is: it neither changes the owner of the data directory nor detects the socket group. The data directory must then already be writable by that user, and the socket group has to come from `group_add`.

### Docker Socket

maintenant needs access to the Docker API to discover and monitor containers. Its entire API surface is **read-only**: container list/inspect/stats/logs, events, version/info, network metadata, the image list (used by the update scan) and, on Swarm managers, Swarm, nodes, services and tasks. It never creates, modifies, or deletes anything.

!!! danger "`:ro` on the socket is not a security boundary"
    Mounting the socket read-only only protects the socket *file*. The Docker API behind it
    still accepts writes: any process holding the socket, `:ro` or not, can stop containers,
    start privileged ones, and escalate to root on the host. The only real boundary is a
    filtering proxy in front of the socket.

#### Recommended: Docker Socket Proxy

Run maintenant behind [Tecnativa/docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) and point it at the proxy with `DOCKER_HOST`: maintenant's Docker client honours the variable natively, so no socket mount is needed at all:

```yaml
services:
  socketproxy:
    image: tecnativa/docker-socket-proxy:latest
    environment:
      # The only endpoint groups maintenant needs, all read-only:
      CONTAINERS: "1"       # discovery, inspect, stats, logs
      IMAGES: "1"           # update scan: locally built images, exact digests
      INFO: "1"             # runtime + Swarm detection
      NETWORKS: "1"         # network metadata
      # EVENTS, PING and VERSION are already enabled by default.
      # POST defaults to 0 -> every write returns 403.
      # On a Swarm manager, also enable: SWARM, NODES, SERVICES and TASKS.
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
      - /etc/os-release:/host/etc/os-release:ro
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
- With this setup, even a fully compromised maintenant could only *read* the Docker API: the proxy answers `403` to every write.
- Without `IMAGES: "1"` the update scan still runs, but it cannot recognise locally built images or read the exact digest of the running image. The proxy refuses `GET /images/json`, and maintenant logs one warning that names `IMAGES=1` and carries on with the scan.
- If maintenant starts before the proxy is reachable, it boots in degraded mode and reconnects automatically once the proxy is up, retrying with a growing delay (1 second at first, 30 seconds at most) and logging the cause of each failure.
- Works identically in **agent mode**: the agent uses the same Docker client, so a per-host socket proxy plus `DOCKER_HOST` replaces the socket mount there too.

#### Alternative: direct socket mount

The simpler setup mounts the socket directly:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
  - /proc:/host/proc:ro
  - /etc/os-release:/host/etc/os-release:ro
```

This relies on maintenant *behaving* read-only (which it does) rather than *enforcing* it. The non-root user, `read_only: true` filesystem and `no-new-privileges` remain worthwhile defense-in-depth, but be clear about the trade-off: whoever holds the socket holds the host.

### Network Binding

maintenant binds to `127.0.0.1:8080` by default: **localhost only**. This prevents direct exposure to the network.

The image does not change that default. Inside a container, set `MAINTENANT_ADDR=0.0.0.0:8080` (the Compose files of this project do), otherwise a published port cannot reach the process, although the container health check, which probes the loopback address, still passes. Then **never publish the port directly to the host network**: publish it on `127.0.0.1:8080:8080` for a proxy on the host, or leave it unpublished for a proxy on a shared Docker network, and let the reverse proxy handle external traffic.

!!! danger "Never expose maintenant directly to the internet"
    Without a reverse proxy providing authentication, anyone can access the admin API and read your container logs, metrics, and alerts.

### Database

Whichever engine backs it, the database holds all monitoring data, alert
history, webhook configurations, agent identities and enrolment tokens, and (if
MCP OAuth is enabled) hashed tokens. Treat it as the sensitive asset it is.

**SQLite (the default).**

- Store on a **local filesystem**: NFS and network-mounted volumes cause locking issues with SQLite.
- **Back up** by copying the `.db`, `.db-wal`, and `.db-shm` files while maintenant is stopped, or run `sqlite3 <database> ".backup <target>"` while it runs, from a host that has the `sqlite3` client (the image does not ship it).
- **File permissions**: ensure only the maintenant process can read/write the database file.

**PostgreSQL (when you supply one).** Backups, restores and access control are
yours; the product connects, it does not administer. See
[External database credentials](#external-database-credentials) for the role,
the transport defaults and what reaches the logs, and
[PostgreSQL storage](guides/postgresql.md).

**On an agent host**, the data directory (`MAINTENANT_DATA_DIR`, `/var/lib/maintenant` by default) holds the agent's private key in `identity.json` and `spool.db`, a SQLite queue of the events waiting for the server to be reachable. Restrict it like the server's database.

### Heartbeat Ping Tokens

Ping URLs (`/ping/{token}`) use the heartbeat's ping token as the sole access control. Anyone who knows the token can send pings. The token is distinct from the heartbeat `id`, and `/ping/{id}` never resolves a heartbeat.

- Treat ping tokens as **secrets**.
- Do not commit them to public repositories.
- Do not log them in CI/CD output.
- If you suspect a leak, rotate the token with `POST /api/v1/heartbeats/{id}/rotate-token` or the **Rotate token** button of the heartbeat detail panel. The old token answers 404 from the next request on; the heartbeat keeps its id and its history.

### Webhook URLs

maintenant sends requests to addresses an operator typed in: webhook subscriptions, notification channels that call a URL, and outbound heartbeats. To keep those from reaching your internal network or a cloud metadata endpoint, a webhook or channel URL must be **HTTPS** and is checked twice:

- When it is created, the host is resolved and a URL that points at a loopback, private, link-local, carrier-grade NAT (`100.64.0.0/10`), unspecified or multicast address is refused.
- When a request is sent, the same check runs on the address actually dialled, after DNS resolution and on every redirect. A hostname that starts resolving to an internal address later is refused too.

Outbound heartbeats go through the same guard and have no way to disable it. Discord, Slack and Teams URLs cannot be registered as webhook subscriptions: they need a notification channel.

`MAINTENANT_ALLOW_PRIVATE_WEBHOOKS=true` turns off both the HTTPS requirement at creation and the address checks for webhooks and channels, for a lab or a receiver on your own network. Leave it unset in production.

Endpoint and certificate monitors created through the API are not subject to this guard: probing internal addresses is their purpose. Anyone who can create a monitor through the API can make the server probe your network, which is one more reason to keep the API behind authentication.

Setting a container label creates a monitor, and anyone who can start a container can set one, so label-declared monitors are bounded, on the server and on an agent. They send only `GET` or `HEAD`, drop the `Authorization`, `Cookie` and `Proxy-Authorization` headers, and may reach private and loopback addresses but not link-local (including `169.254.169.254`), multicast or unspecified ones. That last rule is checked on the address actually dialled and on every redirect, for HTTP, TCP and certificate checks. `MAINTENANT_LABEL_ENDPOINTS_ALLOW_LINK_LOCAL=true` lifts it, for tests only. See [Docker labels](guides/docker-labels.md#where-a-label-declared-check-may-go).

### Outgoing mail

The email channel and the status page subscription mails go through the one SMTP server set with `MAINTENANT_SMTP_*`. With `MAINTENANT_SMTP_PORT=465`, maintenant uses implicit TLS: the connection is encrypted from the first byte. On any other port the connection starts in plaintext and is upgraded with STARTTLS when the server announces it. A server that announces STARTTLS and then fails the negotiation gets no message: the send fails instead of falling back to plaintext. A server that does not announce STARTTLS gets no message either: the send fails with an error naming `MAINTENANT_SMTP_ALLOW_PLAINTEXT`. Setting `MAINTENANT_SMTP_ALLOW_PLAINTEXT=true` lets such a server receive the message unencrypted, and maintenant logs a warning at startup while it is set. The SMTP client refuses to send a username and password over an unencrypted connection unless the server is `localhost`, so a server without STARTTLS or TLS cannot be used with credentials.

### Private CA

`MAINTENANT_CA_CERT` is the path of a PEM bundle that maintenant trusts in addition to the system roots (it does not replace them). It applies to every outbound TLS verification: HTTP endpoint checks, certificate checks, webhooks and channels, outbound heartbeats, SMTP over STARTTLS, image registries, the licence server, CVE, changelog and end-of-life lookups, and the agent's gRPC connection to its server. A file that cannot be read, or that holds no PEM certificate, stops the process at startup. Use it for an internal PKI instead of disabling verification.

---

## Security Checklist

A quick reference for securing your deployment:

- [ ] Container runs as non-root (uid 65534, dropped via `setpriv` by the entrypoint, which starts as root: the default in the official image)
- [ ] `read_only: true`: immutable root filesystem
- [ ] `no-new-privileges:true`: blocks privilege escalation
- [ ] **Preferred:** Docker API accessed through a [socket proxy](#recommended-docker-socket-proxy) (`DOCKER_HOST=tcp://socketproxy:2375`, no socket mount, writes rejected at the proxy, `IMAGES: "1"` set for the update scan)
- [ ] Otherwise: Docker socket mounted read-only (`:ro`). Its group is auto-detected, `group_add` is not required (remember `:ro` does not block API writes)
- [ ] (Optional) `DOCKER_GID` set to add a group to the detected ones (socket at a non-standard path). `DOCKER_GID=0`, needed on root-owned sockets such as Synology DSM, is root-equivalent: prefer the socket proxy
- [ ] `MAINTENANT_ADDR=0.0.0.0:8080` only inside a container, with the port published on `127.0.0.1` or not published at all
- [ ] Reverse proxy in front of maintenant with authentication enabled
- [ ] `/api/v1/*` (`/api/v1/health` included) and `/` require authentication
- [ ] `/ping`, `/status` (prefix, no trailing slash) and `/assets/` bypass authentication
- [ ] `MAINTENANT_ALLOWED_HOSTS` lists the host names users reach maintenant by when it listens beyond the loopback
- [ ] `MAINTENANT_TRUSTED_PROXIES` lists the address maintenant sees the proxy from, and the proxy passes `X-Forwarded-For` and `Host` on
- [ ] If MCP is enabled: OAuth2 credentials configured (`MAINTENANT_MCP_CLIENT_ID` + `MAINTENANT_MCP_CLIENT_SECRET`), the secret at least 32 random characters
- [ ] If MCP is enabled: `/mcp`, `/oauth/*`, `/.well-known/*` bypass proxy auth (MCP handles its own)
- [ ] If MCP is enabled: `MAINTENANT_MCP_ALLOWED_REDIRECT_URIS` lists only the non-loopback redirect URIs of your clients
- [ ] `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED` **not** set (it is only for a local instance on a trusted network)
- [ ] If agents are used: the gRPC listener (`MAINTENANT_GRPC_LISTEN`) is reachable only from the agents' networks, serves your own certificate (`MAINTENANT_GRPC_TLS_CERT` + `MAINTENANT_GRPC_TLS_KEY`) or the persistent self-signed one with agents pinned by `--grpc-server-fingerprint`, no agent runs with `MAINTENANT_GRPC_INSECURE_SKIP_TLS_VERIFY`, and `MAINTENANT_GRPC_TLS_INSECURE` is not set unless a proxy terminates TLS in front of it
- [ ] `MAINTENANT_ALLOW_PRIVATE_WEBHOOKS` **not** set
- [ ] HTTPS termination at the proxy level
- [ ] `Strict-Transport-Security` set at the proxy (the app cannot set it safely)
- [ ] `MAINTENANT_BASE_URL` set to your public HTTPS URL
- [ ] Database file has restrictive permissions
- [ ] Heartbeat ping tokens not exposed in public repositories or logs
- [ ] `MAINTENANT_CORS_ORIGINS` unset unless a page on another origin calls the API, then listing exactly those origins (`*` does not allow writes)

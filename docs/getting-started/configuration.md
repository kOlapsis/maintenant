# Configuration

maintenant is configured through environment variables, with no configuration file. Every variable has a matching command-line flag (the [flag table](../install.md#configuration-reference) maps one to the other). The precedence is CLI flag, then environment variable, then default. Command-line flags show up in the process list, so pass secrets (license key, SMTP password, MCP secret, enrollment token, database URL) as environment variables.

Three options exist only as flags: `--copy-store-to` and `--yes` (copy an install into PostgreSQL, see [PostgreSQL storage](../guides/postgresql.md)) and `--mcp-stdio` (MCP over stdin and stdout, see [MCP Server](../features/mcp.md)). `maintenant --help` prints every flag with its environment variable and default.

Conventions used below:

- A boolean is true for `1`, `t`, `true`, `y`, `yes` or `on` (case-insensitive). Any other value, including an empty one, is false.
- Durations use Go syntax: `30s`, `5m`, `24h`.
- A value that cannot be parsed falls back to the default. The settings that stop the startup on an invalid value say so in their row.

---

## Environment Variables

### Core

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_ADDR` | `127.0.0.1:8080` | HTTP listen address (`host:port`). The container image keeps this default, so set `0.0.0.0:8080` when running in a container. See [Choosing a Bind Address](#choosing-a-bind-address). An agent opens no HTTP port. |
| `MAINTENANT_BASE_URL` | `http://<MAINTENANT_ADDR>` | Public URL at which this instance is reached. It builds the heartbeat ping URLs, the links in status page emails and the issuer and resource URLs of the [MCP](../features/mcp.md) OAuth server. Set it to your real public URL (for example `https://maintenant.example.com`) as soon as a proxy sits in front. |
| `MAINTENANT_STATUS_URL` | — | Canonical public URL of the status page (for example `https://status.example.com`). `GET /api/v1/edition` reports it as `status_url`, and the admin UI's *View public status page* link opens it, falling back to the relative `/status` when it is not set. |
| `MAINTENANT_ORGANISATION_NAME` | `Maintenant` | Organisation name shown on the public status page. |
| `MAINTENANT_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. Any other value means `info`. Logs are JSON lines on stdout. |
| `MAINTENANT_MAX_BODY_SIZE` | `1048576` | Maximum request body size in bytes, for every HTTP method, on the API and ping routes. It must be a positive whole number: any other value stops the startup with an error that names the variable. |
| `MAINTENANT_UPDATE_INTERVAL` | `24h` | Interval between [Update Intelligence](../features/updates.md) registry scans. An invalid or non-positive value falls back to `24h`. |
| `MAINTENANT_DISABLE_OS_EOL_REFRESH` | `false` | Stop the daily refresh of the operating system end-of-support dates from endoflife.date. The table embedded in the binary is then the only source. See [Host OS End-of-Support](../features/host-os.md). |
| `MAINTENANT_SECURITY_SCORE_THRESHOLD` | — | Personal edition. Raises an alert when the security posture score (0 to 100) drops below this value, and resolves it when the score recovers. The score is evaluated every 5 minutes and each time the posture is computed. A whole number from 0 to 100: unset or `0` disables the alert, and any other value (negative, above 100, not a number) stops the startup. See [Network Security Insights](../features/security.md). |
| `MAINTENANT_CONTAINER_DOWN_AFTER` | unset (off) | Raises a `container_down` alert for a container that has stayed stopped (`exited` or `dead`) for this long, for example `5m`, and resolves it when the container runs again. A container that ended with exit code 0 or 143, or with 137 when the out-of-memory killer was not the cause, counts as completed, not down. Unset or `0` disables the check. An invalid value stops the startup. |
| `MAINTENANT_PROXY_LABELS` | `false` | Create endpoint monitors from the Traefik and Caddy docker-proxy labels of Docker containers. Agents honour it too. See [Reverse proxy labels](../guides/docker-labels.md#reverse-proxy-labels-traefik-caddy). |

### Container runtime

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_RUNTIME` | auto-detect | Force the runtime: `docker` or `kubernetes`. Any other value stops the startup. Without it, detection tries in-cluster Kubernetes (`KUBERNETES_SERVICE_HOST`), then a cluster from `KUBECONFIG` or `~/.kube/config`, then Docker. A kubeconfig whose cluster does not answer falls back to Docker with a warning; set `MAINTENANT_RUNTIME=kubernetes` to wait for that cluster instead. When the chosen runtime is unreachable, the instance starts in degraded mode and keeps running endpoint, certificate and heartbeat monitors. It reconnects in the background, waiting longer after each failed attempt (1 second at first, 30 seconds at most) and logging the cause. |
| `DOCKER_HOST` | local socket | Docker API endpoint (standard Docker SDK variable). Point it at a socket proxy (`tcp://socketproxy:2375`) to run without mounting `/var/run/docker.sock`. See [Security → Docker Socket Proxy](../security.md#recommended-docker-socket-proxy). |
| `DOCKER_GID` | — | Read by the container entrypoint only. A group ID added to the unprivileged runtime's groups so it can read the mounted Docker socket, on top of the socket's own group, which the entrypoint detects for `/var/run/docker.sock` and `/run/docker.sock`. Set it when the socket is mounted elsewhere, or to `0` on hosts whose socket is `root:root` with no `docker` group, such as Synology DSM (this also silences the startup warning about a root-owned socket). Granting gid 0 is root-equivalent access to the host: prefer a [socket proxy](../security.md#recommended-docker-socket-proxy). Ignored when the container starts as a non-root user. |
| `MAINTENANT_K8S_NAMESPACES` | all | Kubernetes namespace allowlist (comma-separated). Empty monitors every namespace except `kube-system`, `kube-public` and `kube-node-lease`. |
| `MAINTENANT_K8S_EXCLUDE_NAMESPACES` | — | Namespaces to exclude (comma-separated), added to the three system namespaces above. Ignored when an allowlist is set. |

### Storage

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_DB` | `./maintenant.db` | Path of the SQLite database file. The image sets `/data/maintenant.db`; the native service runs in `/var/lib/maintenant`, so the default lands there. The directory of this file also holds the license cache, the update-window record, the telemetry identity (`shm/`) and the embedded agent's data (`embedded-agent/`), so keep it on persistent storage, PostgreSQL or not. |
| `MAINTENANT_DATABASE_URL` | — | PostgreSQL 14+ connection string (`postgres://` or `postgresql://`). Empty means SQLite. Server and embedded modes only: an agent refuses it. A database that cannot be opened stops the startup, with no fallback to SQLite. See [Database](#database) and [PostgreSQL storage](../guides/postgresql.md). |

### Network and security

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_CORS_ORIGINS` | unset (same origin) | Comma-separated origins (`https://app.example.com`) allowed to call the API from a browser on another origin. The same list is the set of origins trusted for state-changing requests (POST, PUT, PATCH, DELETE): from any other origin, including another subdomain, a browser request is refused with `403 CROSS_ORIGIN_REFUSED`. `*` allows cross-origin reads but no cross-origin writes. Requests without an `Origin` header (curl, scripts) are not affected, and `/ping/`, `/status/`, `/mcp` and `/oauth/` are outside this check. |
| `MAINTENANT_TRUSTED_PROXIES` | unset (none) | Comma-separated IPs or CIDRs (`10.0.0.0/8,192.168.1.4`) of the reverse proxies whose `X-Forwarded-For`, `X-Real-IP` and `X-Forwarded-Host` headers are believed. Without it no forwarded header is read: every rate limit and quota counts against the address that opened the connection, which is the proxy. An invalid list stops the startup. See [Security](../security.md). |
| `MAINTENANT_CA_CERT` | — | Path to a PEM bundle of extra root CAs, added to the system roots and never replacing them, for hosts signed by a private PKI. It applies to every HTTPS connection maintenant opens itself: HTTP probes and certificate checks, webhooks and notification channels, outbound heartbeats, SMTP STARTTLS and implicit TLS, the license server, OSV, GitHub, endoflife.date, image registries and an agent's gRPC connection to its server. It does not apply to the Docker and Kubernetes APIs or to the PostgreSQL connection (use `sslrootcert` in the connection string). The file must be readable by the runtime user (uid 65534 in the image). An unreadable or invalid bundle stops the startup. |
| `MAINTENANT_ALLOW_PRIVATE_WEBHOOKS` | `false` | **Development only.** Allow `http://` and private, loopback or link-local targets for notification channels and webhook subscriptions (API and MCP). They are otherwise HTTPS-only and checked against private addresses when saved and again when connecting. Outbound heartbeats keep that protection. Never set it in production. |

### MCP

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_MCP` | `false` | Enable the MCP server on `/mcp` (Streamable HTTP transport). |
| `MAINTENANT_MCP_CLIENT_ID` | — | OAuth 2 client ID for `/mcp`. |
| `MAINTENANT_MCP_CLIENT_SECRET` | — | OAuth 2 client secret for `/mcp`. Use at least 32 characters (`openssl rand -hex 32`): a shorter secret logs a warning at startup. |
| `MAINTENANT_MCP_ALLOWED_REDIRECT_URIS` | — | Comma-separated redirect URIs accepted by `/oauth/authorize`, matched exactly. Loopback addresses (`localhost`, `127.0.0.1`, `::1`) are always accepted, so the list is only needed for hosted clients such as `https://claude.ai/api/mcp/auth_callback`. |
| `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED` | `false` | Serve `/mcp` with no authentication. Without it, `MAINTENANT_MCP=true` without both credentials stops the startup. Trusted networks only. |

### Notifications (SMTP)

One SMTP configuration serves the email alert channel (Personal) and the status page subscriber emails (Pro, see [Public Status Page](../features/status-page.md)). Mails are plain text. On port 465 maintenant uses implicit TLS. On any other port it connects in plain text and upgrades with STARTTLS when the server announces it: if the upgrade then fails, the send fails instead of falling back to plain text, and a server that does not announce STARTTLS receives the mail unencrypted. Each send gives up after 30 seconds.

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_SMTP_HOST` | — | SMTP server hostname. Empty means email is not configured. |
| `MAINTENANT_SMTP_PORT` | `587` | SMTP server port. |
| `MAINTENANT_SMTP_USERNAME` | — | Username for `AUTH PLAIN`. Authentication is attempted only when it is set. |
| `MAINTENANT_SMTP_PASSWORD` | — | Password for `AUTH PLAIN`. |
| `MAINTENANT_SMTP_FROM` | `maintenant@localhost` | Sender address. |

### License

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_LICENSE_KEY` | — | Personal or Pro license key, whichever the key grants. See [License](#license). |

### Retention

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_RETENTION_SNAPSHOTS` | `48h` | How long raw resource samples are kept. The 24-hour chart reads them, so values below `24h` are raised to `24h` with a warning. Longer ranges come from the hourly rollup. |
| `MAINTENANT_RETENTION_INTERVAL` | `1h` | Time between two cleanup passes. Values below `1m` are raised to `1m`. |
| `MAINTENANT_RETENTION_BATCH_SIZE` | `1000` | Rows deleted per transaction. Clamped to the range 100 to 100000. |

The other retention windows are fixed: hourly resource rollups 90 days, daily rollups one year, daily uptime 365 days, state transitions 90 days (the last transition of each container is always kept), and 30 days for check results, heartbeat pings and executions, certificate check results, archived containers and inactive endpoints. The resource history range a user can read depends on the edition (7, 30 or 90 days), see [Resource Metrics](../features/resources.md).

### Multi-host

These variables drive [Multi-Host Monitoring](../features/multihost.md). The [Agent Setup Guide](../guides/agent-setup.md) walks through a full deployment.

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_MODE` | `embedded` | `embedded`: the web server and the local runtime, which also accepts remote agents when the edition allows it. `server`: the same, but refuses to start below the Personal edition. `agent`: no web interface, reports to a server. |
| `MAINTENANT_SERVER` | — | Agent: gRPC URL of the server, for example `grpcs://maintenant.example.com:8443`. |
| `MAINTENANT_ENROLLMENT_TOKEN` | — | Agent: enrollment token generated on the server's Agents page. It is used on the first start, and again to enroll under a new identity when the server refuses the stored one (agent revoked or deleted). Without a token, a refused agent stops with exit code 1. |
| `MAINTENANT_LABEL` | — | Agent: display label shown in the UI. |
| `MAINTENANT_NODE_NAME` | — | Kubernetes agent: name of the node it runs on, used to read the node's operating system. Found from the agent's own pod when empty. |
| `MAINTENANT_DATA_DIR` | `/var/lib/maintenant` | Agent: directory holding `identity.json`, the spool database `spool.db` and the liveness file. The container entrypoint and healthcheck read it too. |
| `MAINTENANT_GRPC_LISTEN` | `127.0.0.1:8443` | Server: address of the agent gRPC listener. It starts in `server` and `embedded` modes when the edition allows agents (Personal or Pro). The default is loopback, so remote agents cannot connect until you change it; in a container use `0.0.0.0:8443` and publish the port. |
| `MAINTENANT_GRPC_URL` | — | Server: gRPC URL shown to agents in the enrollment commands. When empty it is derived from the `Host` of the web request (`X-Forwarded-Host` only from a trusted proxy), with the `grpcs://` scheme. |
| `MAINTENANT_GRPC_TLS_CERT` | — | Server: PEM certificate for the gRPC listener. It must be set together with the key: one without the other stops the startup. With neither, a self-signed certificate is generated in memory at every start and a warning is logged. Not for production. |
| `MAINTENANT_GRPC_TLS_KEY` | — | Server: PEM private key matching the certificate above. |
| `MAINTENANT_GRPC_TLS_INSECURE` | `false` | Server: serve gRPC as plain h2c, without TLS. Only behind a reverse proxy that terminates TLS and forwards gRPC. The embedded agent then connects with `grpc://`. |
| `MAINTENANT_GRPC_INSECURE_SKIP_TLS_VERIFY` | `false` | Agent: skip the verification of the server's certificate. Debug only: a private PKI is handled by `MAINTENANT_CA_CERT`. |
| `MAINTENANT_EMBEDDED_AGENT` | `false` | Server mode: also run an agent for the machine that hosts the server. Personal edition or above. |
| `MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND` | `1000` | Server: gRPC calls per second allowed for each agent. |
| `MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS` | `60` | Server: seconds without a heartbeat before an agent is reported stale. |
| `MAINTENANT_AGENT_SPOOL_MAX_MEMORY_BYTES` | `16777216` | Agent: bytes of events held in memory before the spool writes them to disk. `0` writes every event to disk. |
| `MAINTENANT_AGENT_SPOOL_MAX_DISK_BYTES` | `134217728` | Agent: maximum size of `spool.db`. The oldest events are dropped first. `0` means no size limit. |
| `MAINTENANT_AGENT_SPOOL_MAX_AGE_SECONDS` | `86400` | Agent: age past which a spooled event is neither kept nor replayed. `0` turns the age limit off. |

While the server is unreachable, the agent queues events in the spool and replays them on reconnection; the replay feeds history only. Setting both the memory and the disk budget to `0` disables the spool. The three spool variables accept non-negative integers, and an invalid value stops the startup.

### Telemetry and demo

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_DISABLE_TELEMETRY` | `false` | Turn off the anonymous usage snapshot. See [Telemetry](#telemetry). |
| `MAINTENANT_DEMO_TOKEN` | — | Read by demo builds only (the `:demo` image, which is read-only). A request carrying this value in the `X-Maintenant-Demo-Token` header may write to the instance. There is no command-line flag. A demo build only monitors a remote Docker endpoint: `DOCKER_HOST` must start with `tcp://`, and `KUBERNETES_SERVICE_HOST` and `KUBECONFIG` must be unset, or it stops at startup. |

### Other variables the process reads

| Variable | Description |
|----------|-------------|
| `GITHUB_TOKEN` | Optional GitHub token, sent as a bearer token when fetching release notes for the changelog (Personal edition), to raise the API rate limit. Environment only. |
| `DO_NOT_TRACK` | `1` or `true` also stops the telemetry reporter. See [Telemetry](#telemetry). |

The container image also sets three variables you normally leave alone: `MAINTENANT_DB=/data/maintenant.db`, `SQLITE_TMPDIR=/data` (SQLite temporary files go on the data volume, because `/tmp` is usually a small tmpfs) and `MAINTENANT_CONTAINER=1` (the OS identity reader then never falls back to the image's own `/etc/os-release`).

### Example `.env` File

The repository ships a fully commented [`.env.example`](https://github.com/kOlapsis/maintenant/blob/main/.env.example). A typical production set looks like this:

```bash
# Inside a container, keep 0.0.0.0 and control exposure with the port mapping.
MAINTENANT_ADDR=0.0.0.0:8080
MAINTENANT_DB=/data/maintenant.db

# Public URL behind the reverse proxy (heartbeat pings, status emails, MCP OAuth).
MAINTENANT_BASE_URL=https://maintenant.example.com

# Reverse proxy in front: believe its forwarded headers.
MAINTENANT_TRUSTED_PROXIES=172.18.0.0/16

# Alert when a container stays stopped for 5 minutes.
MAINTENANT_CONTAINER_DOWN_AFTER=5m

# Email channel and status page subscribers.
MAINTENANT_SMTP_HOST=smtp.example.com
MAINTENANT_SMTP_USERNAME=alerts@example.com
MAINTENANT_SMTP_PASSWORD=secret
MAINTENANT_SMTP_FROM=maintenant@example.com

# Personal or Pro license.
MAINTENANT_LICENSE_KEY=your-license-key
```

---

## Choosing a Bind Address

`MAINTENANT_ADDR` controls where the maintenant process itself listens. In Docker, the **published** port (the `ports:` mapping or `-p` flag) is a separate decision, and it is the one that determines what your network can reach.

### Running in a container (recommended)

Set `MAINTENANT_ADDR=0.0.0.0:8080`. The image default is `127.0.0.1:8080`, and inside a container that is unreachable from outside: Docker's port mapping reaches the process through the container's bridge interface. A container left on the default stays healthy (the healthcheck probes loopback) while the published port answers nothing. Control exposure with the port mapping instead:

| Goal | Port mapping |
|------|--------------|
| Behind a reverse proxy on the same host | `127.0.0.1:8080:8080`, or no `ports:` at all when the proxy shares a Docker network with maintenant |
| Headless server, direct access from your LAN | `192.168.1.50:8080:8080` (the server's LAN IP) |
| Reachable from every interface | `8080:8080`, only behind a reverse proxy with authentication, or on a trusted network you accept exposing it to |

### Running the binary directly on the host

The default `127.0.0.1:8080` keeps the UI local-only. To reach it from your LAN without a reverse proxy, bind the server's LAN IP: `MAINTENANT_ADDR=192.168.1.50:8080`. Binding `0.0.0.0:8080` also works but listens on every interface of the host, including public ones if the machine has any.

!!! note "\"Port exposed on all interfaces\" on maintenant's own container"
    maintenant's security scanner analyzes **every** discovered container, including its own. If the UI port is published on all interfaces (`8080:8080`), it reports a critical `port_exposed_all_interfaces` finding against itself. That is expected behaviour, not a bug: the port really is reachable from any network interface.

    Either restrict the published port to a specific interface as shown above, or, when the exposure is intentional (for example a trusted home LAN), acknowledge the finding with an audit trail: see [Network Security Insights → Acknowledging Findings](../features/security.md#acknowledging-findings).

---

## Telemetry

maintenant sends an **anonymous, opt-out** usage snapshot to `https://metrics.kolapsis.com`: one at startup, then one every hour. The data sent contains no hostnames, IP addresses, container names, endpoint URLs, certificates, webhook targets, status-page component names, license keys or operator-supplied free-form strings. Agents send no telemetry: only the server and embedded modes do.

### What is collected

**Registration**, once per process start: a random installation ID, the public half of a key pair generated on the first run, the application name (`Maintenant`), its version, the environment (`production`) and the OS and architecture (`linux/amd64`).

**Each snapshot** is signed with that key pair and contains only these fields:

- `edition`: `community`, `personal` or `pro`, the edition actually in force (an expired Pro license reports `community`)
- `storage_engine`: `sqlite` or `postgres`, and nothing else about the database
- `containers_total`, `endpoints_total`, `heartbeats_total`, `certificates_total`, `webhooks_total`, `status_components_total`: counts of configured or auto-discovered entities
- `sys_os`, `sys_arch`, `sys_cpu_cores`, `sys_go_version` and `sys_mode` (`docker`, `kubernetes` or `standalone`)
- `app_mem_alloc_mb` and `app_goroutines`

### How to disable it

Set `MAINTENANT_DISABLE_TELEMETRY` to a truthy value before the process starts:

```yaml
services:
  maintenant:
    environment:
      MAINTENANT_DISABLE_TELEMETRY: "1"
```

Truthy values (case-insensitive, whitespace-trimmed): `1`, `t`, `true`, `y`, `yes`, `on`. Anything else, including empty or unset, leaves telemetry enabled.

When disabled, exactly one log line is emitted at startup and nothing else:

```text
INFO  telemetry disabled  reason=opt-out
```

No background goroutine, no DNS lookup of `metrics.kolapsis.com`, no outbound packets, and no identity file is created.

The reporter also honours the `DO_NOT_TRACK` convention: with `DO_NOT_TRACK=1` or `DO_NOT_TRACK=true`, it logs `[SHM] Telemetry disabled` and sends nothing, although the startup log line still reads `telemetry enabled`. Prefer `MAINTENANT_DISABLE_TELEMETRY`.

### Persistent install identity

The identifier and its key pair live in `shm_identity.json`, in the `shm` directory next to the database file: `/data/shm` in the image, `/var/lib/maintenant/shm` for a native install. The reference Docker Compose mounts `/data` as a named volume, so it is covered by default. The container entrypoint chowns `/data/shm` at startup, so bind mounts work without manual permission setup.

The Kubernetes manifests and the Helm chart mount their volume on `/data`, which already contains `/data/shm`: nothing more to add.

Without persistence, every restart is counted as a fresh install. The privacy impact is zero (the identifier is opaque), but fleet statistics are distorted.

If the directory is unwritable (read-only mount), telemetry disables itself with a single WARN line and the process continues normally:

```text
WARN  telemetry disabled  reason=datadir-unwritable  datadir=/data/shm  error=...
```

---

## License

The core of maintenant is licensed under **Apache 2.0** and needs no key: it is the Community edition. The paid features live in `internal/commercial` and `frontend/src/commercial`, under a commercial license (see [COMMERCIAL-LICENSE.md](https://github.com/kOlapsis/maintenant/blob/main/COMMERCIAL-LICENSE.md)). Both parts ship in the same binary, and a license key opens the commercial part.

Maintenant comes in three editions, in order: **Community**, **Personal**, **Pro**.

| Edition | Price | Unlocks |
|---------|-------|---------|
| Community | Free | Containers, endpoints, heartbeats, certificates, the public status page, and the full Swarm and Kubernetes views, with 7 days of resource history. Capped at 10 endpoints, 5 heartbeats, 5 certificate monitors and 3 status components created by hand (endpoints and certificate monitors discovered from container labels are unlimited), on a single host. |
| Personal | €149 once, for life | Every cap lifted, up to 20 remote hosts, plus email and Telegram alerts, CVE enrichment, risk scoring, security posture, incidents, changelog, 30 days of resource history, advanced trigger filters and OCSP stapling. Covers one person on infrastructure they own or run for themselves, freelancers included. |
| Pro | €29/month or €290/year | Everything above with unlimited hosts, plus Slack and Teams, escalation policies, maintenance windows, status page subscribers and branding, the high availability storage options (`MAINTENANT_SQLITE_SYNCHRONOUS=FULL`, `MAINTENANT_REQUIRE_STATE_DIR`, `MAINTENANT_REQUIRE_EXISTING_DATA`, ignored with a warning in the other editions), 90 days of resource history, and the right to use Maintenant on behalf of others, with support. |

A Personal license never expires and includes one year of product updates.
Every version released inside that year stays licensed for life; a further year
of updates costs €59. See [COMMERCIAL-LICENSE.md](https://github.com/kOlapsis/maintenant/blob/main/COMMERCIAL-LICENSE.md).

To unlock Personal or Pro, set the `MAINTENANT_LICENSE_KEY` environment variable:

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    environment:
      MAINTENANT_LICENSE_KEY: "your-license-key"
```

!!! note "Building from source"
    Official binaries and images carry the license public key at build time. A binary you build yourself does not, so it cannot verify a key: the log says `license manager initialization failed, running as Community Edition` and the instance stays on Community.

### Verification and offline behavior

The key is verified against the license server at startup and then every 24 hours. The signed answer is cached in `.maintenant-license`, in the directory of `MAINTENANT_DB`. When the server cannot be reached, the edition stays active from that cache:

- After 7 days without a successful check, the status message says the server is unreachable and the edition stays active.
- After 30 days, the status message counts down the days left before the fallback.
- After 60 days, the instance falls back to Community, and this applies to a Personal license too: a perpetual license has no end date, but it must reach the license server at least once every 60 days. The edition returns at the next successful check.
- With no cache at all (first start offline), the status is `unreachable` and the instance runs as Community.

A key the server refuses is dropped at once: `expired`, `revoked` or `unknown` (key not recognized) leaves the instance on Community.

A Personal license also bounds the versions it covers. A build released after the update window closed keeps its edition for a 30-day grace period and then falls back to Community; the versions released inside the window stay licensed. `GET /api/v1/license/status` shows `status`, `edition`, `plan`, `verified_at`, `expires_at`, `updates_until` and `update_grace_until`. `status` is one of `active`, `grace` (still granted, renewal due), `expired`, `revoked`, `unknown`, `unreachable`, `update_window_grace`, `update_window_ended`, or `inactive` when no key is set.

```
GET /api/v1/license/status
```

### When the edition drops

Nothing is deleted when an instance falls back to a lower edition; the features close instead.

- Email, Telegram, Slack and Teams channels are **suspended**: they stay in the list, their deliveries are recorded with the status `suspended` and nothing is sent. They are listed under `suspended_channels` in `GET /api/v1/edition` and shown in a banner in the UI. They resume when the edition returns.
- `MAINTENANT_MODE=server` stops the startup below Personal. An instance that loses Personal because its update window closed keeps its multi-host plan running (enrolled agents keep streaming), but enrolling and managing hosts is refused.

---

## Security

maintenant does not include built-in authentication, by design. It delegates auth to your reverse proxy and middleware (Authelia, Authentik, OAuth2 Proxy). Anyone who can reach the port can read everything and change monitors and settings, so never expose it directly to an untrusted network.

```
Internet  →  Reverse Proxy (Traefik / Caddy / nginx)
          →  Auth Provider
          →  maintenant
```

The `/api/v1/*` routes and the dashboard must be behind authentication. The `/ping/` routes (heartbeat pings) and `/status` (match the prefix, not the trailing slash) must be publicly accessible. The public status page loads its scripts and styles from `/assets/`, so when it shares a hostname with the protected dashboard, let `/assets/` through as well, or serve the status page on its own hostname. If MCP is enabled with OAuth2, the `/mcp`, `/oauth/` and `/.well-known/` routes should bypass proxy auth (MCP handles its own). The Docker healthcheck and Kubernetes probes reach the container directly, so `/api/v1/health`, which reports the version, runtime and storage state, does not need to go through the proxy.

See the **[Security Guide](../security.md)** for the complete route reference, reverse proxy examples (Traefik, Caddy, nginx), built-in protections, MCP authentication details, and deployment hardening checklist.

---

## Database

maintenant stores its state in SQLite by default, or in PostgreSQL 14 or newer when `MAINTENANT_DATABASE_URL` is set.

### SQLite

SQLite runs in WAL (Write-Ahead Logging) mode with a single-writer pattern, which gives good read performance while keeping data integrity.

- The database file is created automatically on first startup, at `MAINTENANT_DB`.
- Migrations run automatically at startup, before the HTTP listener opens, so `/api/v1/health` does not answer while a long migration works.
- Back up the database by copying the `.db`, `.db-wal` and `.db-shm` files while maintenant is stopped, or run `sqlite3 maintenant.db ".backup backup.db"` from the host while it runs. The container image does not ship `sqlite3`.
- An agent never uses PostgreSQL: its only database is the local spool, `spool.db`.

!!! tip "Persistence in Docker"
    Always mount a volume on the database directory to persist data across container restarts. The image sets `MAINTENANT_DB` to `/data/maintenant.db`, so a volume on `/data` is enough; the compose files also set it explicitly:
    ```yaml
    volumes:
      - maintenant-data:/data
    environment:
      MAINTENANT_DB: "/data/maintenant.db"
    ```

### PostgreSQL

Set `MAINTENANT_DATABASE_URL` to a PostgreSQL database you already operate, for example `postgres://maintenant:secret@db.internal:5432/maintenant?sslmode=require`. The schema is created on first start. The setting applies to server and embedded modes and is refused in agent mode.

- When the connection string has no `sslmode` and the host is not local (`localhost`, `127.0.0.1`, `::1`, an empty host or a Unix socket), maintenant adds `sslmode=require`. An explicit value wins, `disable` included. If the server does not accept TLS, the startup message says so and suggests enabling TLS on the server or setting `sslmode=disable` explicitly.
- If the database cannot be opened, maintenant stops. It never falls back to the local SQLite file.
- The connection string is redacted (no password, no parameters) wherever it is logged.
- `MAINTENANT_DB` still matters: the license cache, the update-window record, the telemetry identity and the embedded agent's data stay in its directory. Keep that volume with the instance when it moves.
- `--copy-store-to` copies an existing SQLite install into an empty PostgreSQL database and exits. See [PostgreSQL storage](../guides/postgresql.md#migrating-an-existing-install).

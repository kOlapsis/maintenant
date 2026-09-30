# Architecture

## Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                         Single Go Binary                         │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │          Vue 3 + TypeScript + Tailwind (embed.FS)          │  │
│  │           SSE live updates · uPlot charts · PWA            │  │
│  └────────────────────────────────────────────────────────────┘  │
│                              |                                   │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │       REST API v1 · SSE brokers · public status page       │  │
│  │                 MCP server (stdio + HTTP)                  │  │
│  └────────────────────────────────────────────────────────────┘  │
│                              |                                   │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────────┐  │
│  │  Docker / Swarm │  │    Kubernetes   │  │    Agent gRPC    │  │
│  │     runtime     │  │     runtime     │  │      server      │  │
│  └─────────────────┘  └─────────────────┘  └──────────────────┘  │
│                              |                                   │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │     Containers · Endpoints · Heartbeats · Certificates     │  │
│  │     Resources · Alerts · Updates · Security · Webhooks     │  │
│  │    Status page · Outbound heartbeats · Host OS support     │  │
│  └────────────────────────────────────────────────────────────┘  │
│                              |                                   │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │          SQLite (WAL, single writer) by default,           │  │
│  │       PostgreSQL 14+ when the operator supplies one        │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

Remote hosts run the same binary in agent mode and stream to the gRPC server of a server or embedded instance. See [Multi-host agents](#multi-host-agents).

---

## Operating modes

One binary, three modes, selected with `MAINTENANT_MODE` (`--mode`):

| Mode | What runs |
|------|-----------|
| `embedded` (default) | The HTTP server (dashboard, API, MCP, status page), the monitoring of the local runtime and the storage engine. On Personal and above the agent gRPC listener also starts, so agents can enrol against an embedded instance. |
| `server` | Same as `embedded`, but the instance refuses to start below Personal. It is the only mode where `--embedded-agent` has an effect: it runs a local agent inside the process. |
| `agent` | No HTTP server and no store of its own apart from the event spool (an external database URL is refused). The agent detects its local runtime, enrols against a server and streams to it. It starts even when no runtime answers, reports the host alone and retries the runtime in the background. It keeps its identity (`identity.json`), its event spool (`spool.db`) and a liveness file in `MAINTENANT_DATA_DIR`. |

Two more entry points share the binary: `maintenant --mcp-stdio` serves the MCP server over stdin and stdout, and `maintenant healthcheck` backs the image `HEALTHCHECK` (it reads the agent liveness file when there is one, otherwise it calls `/api/v1/health` on the configured address). `--copy-store-to` copies an installation into an empty PostgreSQL database and exits.

---

## Design Philosophy

**Single binary**: The Vue 3 frontend is compiled to static assets and embedded in the Go binary via `embed.FS`. One file to deploy. There is no build per edition: the paid code (`internal/commercial` and `frontend/src/commercial`) is always compiled in and a licence key unlocks it at runtime. The core is Apache 2.0, the paid code is under the commercial licence.

**Zero external dependencies**: SQLite by default, with nothing to install: no Redis, no message queue, no database to administer. An operator watching a fleet *may* point the server at a PostgreSQL they already run, which is the only way agent identities survive losing the server's machine; absent that setting, nothing changes. Agents store their state in SQLite, always.

**Live updates**: State changes are pushed to the browser over Server-Sent Events. The client reconnects with a backoff and its stores refetch after a reconnect. A few widgets also poll (host resources, sparklines, scan progress).

**Read-only**: maintenant never changes your containers. It reads from the Docker socket or the Kubernetes API, and the update and rollback commands it shows are for you to run.

**Label-driven**: Monitoring is declared where the workload is defined: Docker labels, Swarm `deploy.labels` and, for a few settings, Kubernetes annotations. Manual endpoints, heartbeats, certificates, channels and the status page are managed in the interface or through the API, and the instance itself is configured with `MAINTENANT_*` variables or their matching flags.

**Runtime-agnostic**: Docker (with Swarm detection) and Kubernetes sit behind a common `Runtime` interface in `internal/runtime`. At startup the runtime is chosen in this order: `MAINTENANT_RUNTIME`, then `KUBERNETES_SERVICE_HOST` (in cluster), then a kubeconfig whose cluster answers, then the Docker socket. A kubeconfig whose cluster does not answer falls back to Docker with a warning. When the runtime is lost the instance keeps serving in degraded mode and reconnects in the background, retrying with a growing delay (1 second at first, 30 seconds at most) and logging the cause of each failure. A Swarm is detected as soon as Docker connects, so enabling Swarm later needs no restart.

**Editions are data, not builds**: `internal/extension` defines the editions (Community, Personal, Pro), the capabilities, the quotas and the history windows. The tier table that says which edition opens what lives in `internal/commercial/tiers`, and `internal/extpoint` declares the implementations a licensed build plugs into the core (update enrichment, posture scoring, notification channels, status page extras, maintenance suppression, escalation, multi-host). A nil implementation keeps the Community behaviour. `GET /api/v1/edition` publishes the table so the frontend never holds one of its own.

---

## Tech Stack

### Backend

| Technology | Purpose |
|-----------|---------|
| **Go** 1.26.6 | Application runtime (`go` directive of `go.mod`) |
| **`net/http`** (stdlib) | HTTP server, REST API, SSE, cross-origin protection |
| **SQLite** (WAL mode) | Persistence by default, single-writer pattern |
| **`github.com/mattn/go-sqlite3`** | SQLite driver (CGO, the only CGO dependency) |
| **PostgreSQL** 14+ (optional) | Persistence for the server data set, when the operator supplies one |
| **`github.com/jackc/pgx/v5`** | PostgreSQL driver (pure Go) |
| **`github.com/golang-migrate/migrate/v4`** | Embedded schema migrations for both engines |
| **`github.com/moby/moby/client`** and **`api`** | Docker SDK for container discovery, events and Swarm |
| **`k8s.io/client-go`**, **`api`**, **`apimachinery`** | Kubernetes API client |
| **`k8s.io/metrics`** | Kubernetes metrics API |
| **`google.golang.org/grpc`** and **`protobuf`** | Agent protocol (`proto/ingest.proto`) |
| **`github.com/google/go-containerregistry`** | Registry queries for image updates |
| **`github.com/Masterminds/semver/v3`** | Image tag comparison |
| **`github.com/modelcontextprotocol/go-sdk`** | MCP server (AI assistant integration) |
| **`golang.org/x/crypto`** | OCSP response parsing for certificate monitoring |
| **`golang.org/x/time`**, **`golang.org/x/sync`** | Rate limiting, concurrent probes and collectors |
| **`github.com/yuin/goldmark`** and **`github.com/microcosm-cc/bluemonday`** | Markdown rendering and sanitizing for the status page |
| **`github.com/google/uuid`** | UUID generation behind `internal/uid` |
| **`github.com/kolapsis/shm`** | Anonymous usage telemetry SDK (opt-out with `MAINTENANT_DISABLE_TELEMETRY`) |
| **`embed.FS`** | Frontend embedding |

### Frontend

| Technology | Purpose |
|-----------|---------|
| **Vue 3** (3.5) | UI framework (Composition API) |
| **TypeScript** 5.9 | Type safety |
| **Pinia** | State management (SSE-connected stores) |
| **Vue Router** | Client-side routing |
| **Tailwind CSS** 4 | Styling |
| **uPlot** | Lightweight time-series charts (~40 KB) |
| **lucide-vue-next** | Icons |
| **`@tanstack/vue-virtual`** | Virtualized lists |
| **Vite** 7 | Build tooling |
| **vite-plugin-pwa**, **workbox-window** | Progressive Web App support |
| **Vitest** | Unit tests |

---

## Project Structure

```
cmd/maintenant/            Entry point: flag parsing, modes, copy-store, healthcheck
  web/                     Embedded frontend (embed.FS)
proto/                     ingest.proto, the agent protocol
internal/                  Private packages
    app/                   Service wiring, configuration and flag registry, lifecycle,
                           HTTP server assembly, storage supervision
    api/v1/                HTTP handlers, SSE broker, router, middleware
    agent/                 Agent mode: runtime detection, enrollment, collectors,
                           endpoint and certificate probes, event spool
    agentauth/             Byte-exact signing payload of the agent handshake
    agentevent/            Observation time and replay flag carried by agent events
    agentpb/               Generated gRPC and protobuf stubs
    agentproto/            Command helpers and public gRPC URL resolution
    alert/                 Alert engine, notifier, webhook and Discord senders, down detector
      escalation/          Escalation policy, run and service contracts
    certificate/           TLS certificate monitoring
    commercial/            Paid code, commercial licence
      channels/            Email, Telegram, Slack and Teams channels
      escalation/          Escalation service and runner
      license/             Licence verification, cache and update window
      maintenance/         Alert suppression during maintenance windows
      multihost/           Agent gRPC server, sessions, rate limit
      posture/             Security posture scorer
      statuspage/          Incidents, maintenance windows, subscriber mail, personalization
      tiers/               Tier table: capabilities, quotas, history caps per edition
      updates/             CVE, changelog and risk enrichment
    container/             Container model, service, uptime
    docker/                Docker runtime implementation
    endpoint/              Endpoint monitoring (HTTP/TCP)
    eol/                   Host operating system end of support
    event/                 SSE event type constants
    extension/             Editions, capabilities, quotas, history windows, policy contract
    extpoint/              Implementations a licensed build plugs into the core
    heartbeat/             Heartbeat and cron monitoring
    hoststat/              Host CPU, memory and disk from /proc
    kubernetes/            Kubernetes runtime implementation and topology ingest
    mcp/                   MCP server (Model Context Protocol), OAuth for /mcp
    outbound/              Outbound heartbeats
    proxylabels/           Endpoint labels derived from Traefik and Caddy labels
    ratelimit/             Per-IP rate limiting middleware, client IP resolution
    resource/              Resource metrics collection, rollups, history
    retry/                 Exponential backoff helper
    runtime/               Runtime interface and detection
    security/              Security insights (published ports, network mode, privileges)
    ssrf/                  Outbound URL guard for webhooks and channels
    status/                Public status page (handler, subscribers)
    store/                 Store layer, dialect, migrations, writer, copy
    swarm/                 Swarm discovery, nodes, crash loops, rolling updates
    telemetry/             Anonymous usage telemetry
    trust/                 Root certificates for every outbound TLS connection
    uid/                   Entity identifiers (UUIDv7 and deterministic UUIDv5)
    update/                Update intelligence, registry scanning
    webhook/               Webhook dispatcher

frontend/src/
  pages/                   Vue page components
  components/              Reusable UI components
    ui/                    Generic UI primitives
    dashboard/             Dashboard-specific widgets
    heartbeats/            Heartbeat widgets
  commercial/              Paid frontend code, commercial licence
                           (components, composables, services, stores, types)
  stores/                  Pinia stores (SSE-connected)
  services/                API client functions, SSE bus, session guard
  composables/             Vue composables
  layouts/                 Page layouts
  locales/                 Status page translations
  types/                   Shared TypeScript types
  utils/                   Utility functions
  router/                  Vue Router configuration
```

---

## Data Flow

### Container Event

```
Docker / Kubernetes event
  → Runtime.StreamEvents()
    → container.Service.ProcessEvent()
      → store (persist state transition)
      → event callback (internal/app/wiring.go)
        → SSE broker → browsers
                     → webhook dispatcher (observer)
        → status page (component status)
        → alert engine (restart loop, health change)
```

### Endpoint Check

```
Check engine (one ticker per endpoint)
  → HTTP/TCP probe
    → endpoint.Service.ProcessCheckResult()
      → store (persist check result and status)
      → event callback (endpoint.status_changed, internal/app/wiring.go)
        → SSE broker → browsers
                     → webhook dispatcher (observer)
        → status page (component status, on a status change)
      → alert callback (internal/app/wiring.go)
        → alert engine (consecutive failures, untrusted certificate)
        → SSE broker (endpoint.alert, endpoint.recovery)
    → certificate service (auto-detect TLS on HTTPS targets)
```

### Remote agent event

```
Agent collector → spool → gRPC Push stream
  → multihost server (authentication, per-agent rate limit)
    → dispatcher
      → container, endpoint, heartbeat, resource and certificate services
      → Swarm and Kubernetes topology ingest
```

The services store what an agent sends under its `agent_id`. Endpoint results and heartbeat pings go through the same service methods as the ones produced locally, so they follow the same path afterwards (alert thresholds, status page, deadlines).

### Alerts

The alert engine does not listen to the SSE broker. Each service hands it alert events directly: the callbacks wired in `internal/app/wiring.go` push an `alert.Event` into `alertEngine.EventChannel()` and to the status page (`HandleAlertEvent`), next to the SSE broadcast.

```
alert.Event → alert engine
  → deduplicate on source, type, entity
  → silence rules and maintenance windows
  → persist the alert
  → SSE broker (alert.fired, alert.silenced, alert.resolved)
  → escalation (when the edition opens it)
  → alert triggers → channels → notifier queue (workers, retries)
```

Channels are silent by default: an alert reaches a channel only through an alert trigger or an escalation policy. The notifier tries a delivery up to three times, waiting 1 second and then 5 seconds. The notifications of one alert to one channel go through the same worker one after the other, so a recovery never overtakes the alert it resolves.

Webhook subscriptions are a separate path. The webhook dispatcher is an observer of the SSE broker: it reacts to six event types (`container.state_changed`, `endpoint.status_changed`, `heartbeat.status_changed`, `certificate.status_changed`, `alert.fired`, `alert.resolved`) and sends them through the notifier's worker pool. The events sent to one URL are delivered in order. The dispatcher records each delivery on the subscription (last status, consecutive failures) and deactivates it after 10 failures in a row; a success reactivates it.

---

## Multi-host agents

Multi-host monitoring needs Personal or above. The gRPC protocol is defined in `proto/ingest.proto` (service `Ingest`).

- **Enrollment**: an operator creates a one-time enrollment token (24 hours by default, 7 days at most). The agent generates an Ed25519 key pair and calls `RegisterAgent` with the token. When the server revokes the agent or no longer knows it, the agent discards its spool and enrols a fresh identity once with the token it is configured with. Without a usable token it exits with an error that names `MAINTENANT_ENROLLMENT_TOKEN`.
- **Authentication**: every `Push` stream starts with a random 32-byte nonce from the server. The agent answers with a signature over the nonce, its id and a timestamp. A clock difference above 300 seconds, a revoked agent or an unknown agent is refused.
- **Stream**: the agent pushes container events and inventories, endpoint and certificate probe results, resource samples, Swarm and Kubernetes topologies and the host OS identity. The server answers with acknowledgements and errors (`agent_revoked`, `rate_limited`, ...). Commands are the one exception to the push-only stream: the server can ask an agent for container logs.
- **Probing**: the agent probes the endpoints and certificates of its own containers itself, from their labels. The server never dials them.
- **Spool**: events are queued in memory, then in `spool.db`, while the server is unreachable, and replayed after the reconnect. Bounds: `MAINTENANT_AGENT_SPOOL_MAX_MEMORY_BYTES`, `MAINTENANT_AGENT_SPOOL_MAX_DISK_BYTES` and `MAINTENANT_AGENT_SPOOL_MAX_AGE_SECONDS`. State snapshots are never queued, and a replayed event is stored for history without raising live events or alerts.
- **Transport**: TLS. Without `MAINTENANT_GRPC_TLS_CERT` and `MAINTENANT_GRPC_TLS_KEY` the server generates a self-signed certificate and logs a warning. `MAINTENANT_GRPC_TLS_INSECURE` serves plain h2c behind a trusted reverse proxy, and the embedded agent then dials `grpc://` instead of `grpcs://`. A certificate without its key, or the reverse, stops the startup. The listener defaults to `127.0.0.1:8443`.
- **Identity**: the server's own runtime is an agent too, with the sentinel id `00000000-0000-0000-0000-000000000000` (`uid.LocalAgent`), so every entity carries an `agent_id`.

See [Multi-Host Monitoring](features/multihost.md) and [Agent Setup](guides/agent-setup.md).

---

## Storage engines

One storage package, two dialects. SQLite is the default and the only agent
storage; PostgreSQL backs the server data set when an operator supplies a
connection string. Every engine difference goes through a `Dialect`, and there
are six of them: placeholder syntax, batched deletes, opening PRAGMAs, error
classification, write serialization, and SQL-side UUID generation for rollups.
Nothing else in the query files knows which engine it runs on: the UUID
rework had already made the schema portable (TEXT keys, epoch-second BIGINTs,
`ON CONFLICT ... DO UPDATE`).

**Migrations carry one version number across both engines.** SQLite keeps the
full history; PostgreSQL starts from a single baseline numbered at the SQLite
head of the day it was written (28). From 29 onward, a migration is written for
both engines under the same number, or not at all. That rule is enforced by a
test, not by discipline: it migrates a fresh database on each engine and
compares the two heads (tables, columns, types, defaults, indexes, foreign
keys, constraints) and fails on any divergence.

**Why it exists.** The server holds one class of data the fleet cannot rebuild:
agent identities and enrolments. Kept on the machine that runs the process,
that data makes the instance irreplaceable. Detached, a replacement instance
started elsewhere picks the fleet back up with no action on any monitored host.

**What the product does not do:** it never installs, backs up or supervises the
database, and it does not orchestrate failover: no leader election, no mutual
exclusion. Instances register in a table and beat; a second one is *reported*,
never arbitrated. Exclusion belongs to the operator's cluster manager. While the
database is unreachable `/api/v1/health` still answers 200 and reports it in
`storage.connected`, the reads that need the database answer
`503 STORAGE_UNAVAILABLE`, and a `storage.availability_changed` event tells the
interface.

---

## SQLite Architecture

maintenant uses SQLite in WAL (Write-Ahead Logging) mode with a single-writer pattern:

- **One writer goroutine**: All writes are serialized through a channel-based writer to avoid `SQLITE_BUSY` errors
- **Multiple readers**: Read queries run concurrently without blocking
- **Automatic migrations**: Schema migrations run at startup using embedded SQL files. A schema newer than the binary is refused rather than written into, on either engine.
- **Bounded WAL**: Every pooled connection sets `journal_size_limit` (64 MiB) and `wal_autocheckpoint` (1000 pages) through a driver `ConnectHook`; pages freed by retention are returned to the filesystem with `incremental_vacuum` in slices

### Resource history

Containers are sampled every 10 seconds. A rollup runs every 5 minutes and feeds the hourly and daily tables. A history window reads the table that covers it, so the raw samples only need to cover the shortest ranges.

| Window | Read from | Resolution | Opened by |
|--------|-----------|------------|-----------|
| `1h` | raw samples | 10 seconds | Community |
| `6h` | raw samples | 1 minute | Community |
| `24h` | raw samples | 5 minutes | Community |
| `7d` | hourly rollup | 1 hour | Community |
| `30d` | hourly rollup | 1 hour | Personal |
| `90d` | daily rollup | 1 day | Pro |

The edition caps the window, not the live values. The cap is 7 days on Community, 30 days on Personal and 90 days on Pro (`internal/commercial/tiers`, `internal/extension`).

### Retention

A background goroutine prunes old data. A pass runs at startup and then hourly (`MAINTENANT_RETENTION_INTERVAL`), deleting in batches of 1000 rows (`MAINTENANT_RETENTION_BATCH_SIZE`) until each table is drained. If a pass hits its 2-minute-per-table budget it reschedules itself a minute later instead of waiting for the next hour, so a backlog is cleared instead of accumulating.

| Data | Kept |
|------|------|
| Container state transitions | 90 days, except the latest transition of each container, which is kept |
| Archived containers | 30 days after archival, with their update, CVE, pin and risk history |
| Endpoint check results | 30 days |
| Inactive endpoints | 30 days |
| Heartbeat pings and executions | 30 days |
| Certificate check results | 30 days |
| Daily uptime aggregates (endpoints, heartbeats, containers) | 365 days |
| Raw resource samples | 48 hours (`MAINTENANT_RETENTION_SNAPSHOTS`, at least 24 hours) |
| Hourly resource rollups | 90 days |
| Daily resource rollups | 1 year |
| Alerts | 90 days (daily pass) |
| Update scan records and image updates | 30 days (daily pass) |
| Escalation runs and deliveries | 90 days (nightly at 03:00 local time) |
| Unconsumed enrollment tokens | 7 days after expiry (hourly pass) |

Before the raw transitions, check results and heartbeat pings are deleted, the same pass writes their daily uptime aggregates (`endpoint_uptime_daily`, `heartbeat_uptime_daily`, `container_uptime_daily`). A day whose aggregate could not be written keeps its raw rows until a later pass succeeds. The uptime API serves up to 365 days from the aggregates and computes the days not aggregated yet from the raw rows.

---

## HTTP layer

The top-level mux routes `/mcp` (when MCP is enabled) and the OAuth endpoints (when a client id and secret are also set), `/status/*` (public status page), `/api/`, `/ping/` and, for everything else, the embedded single-page application with a fallback to `index.html`. Three per-IP token buckets protect it: 10 requests per second (burst 20) for `/ping/`, `/status/`, `/mcp` and `/oauth/`, 50 per second (burst 200) for `/api/`, and 5 per hour for status page subscriptions. Client addresses are read from forwarded headers only for the proxies listed in `MAINTENANT_TRUSTED_PROXIES`.

Around the mux, from the outside in: security headers and a content security policy, a 10-second timeout for every request except the streaming ones, and the demo guard. The API router adds panic recovery, request logging, a request id, CORS, the cross-origin guard for unsafe methods, and the body size limit. The server itself uses a 5-second read timeout and no write timeout, which SSE requires. The API has no authentication of its own: see [Security](security.md) for what to put in front of it.

---

## SSE Architecture

The SSE brokers are the central hub for real-time updates. There are two, one per audience:

1. **Services** emit events when state changes (container state, heartbeat ping, alert fired, agent connected)
2. **The main broker** (`GET /api/v1/containers/events`) fans out events to every connected dashboard. A client that falls 64 events behind loses events instead of blocking the others.
3. **The status broker** (`GET /status/events`) carries only `status.*` events, so the public status page never sees dashboard events. The same `status.*` events also go to the main broker, for the dashboard.
4. **The webhook dispatcher** observes the main broker and delivers six event types to external URLs.

Both streams send a keep-alive comment every 25 seconds so that a proxy does not cut an idle connection.

The alert engine is not a subscriber: services call it directly (see [Alerts](#alerts)).

Each browser tab keeps a single connection to `/api/v1/containers/events`, shared by all Pinia stores through `sseBus`. The stream is closed after the tab has been hidden for 60 seconds and reopened when it becomes visible, and the stores refetch after a reconnect. Container logs use a separate stream per container, `/api/v1/containers/{id}/logs/stream`. See the [API reference](api/reference.md#sse-event-stream) for the event list.

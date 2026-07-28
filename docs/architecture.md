# Architecture

## Overview

```
┌──────────────────────────────────────────────────────┐
│                  Single Go Binary                    │
│                                                      │
│   ┌────────────────────────────────────────────┐     │
│   │  Vue 3 + TypeScript + Tailwind (embed.FS)  │     │
│   │  Real-time SSE  ·  uPlot charts  ·  PWA    │     │
│   └────────────────────────────────────────────┘     │
│                         |                            │
│   ┌────────────────────────────────────────────┐     │
│   │           REST API v1 + SSE Broker         │     │
│   │           MCP Server (stdio + HTTP)        │     │
│   └────────────────────────────────────────────┘     │
│          |                          |                │
│   ┌─────────────┐  ┌──────────────────────┐         │
│   │   Docker     │  │     Kubernetes       │         │
│   │   Runtime    │  │     Runtime          │         │
│   └─────────────┘  └──────────────────────┘         │
│          |                          |                │
│   ┌────────────────────────────────────────────┐     │
│   │  Containers · Endpoints · Heartbeats ·     │     │
│   │  Certificates · Resources · Alerts ·       │     │
│   │  Updates · Security · Status Page ·        │     │
│   │  Webhooks                                  │     │
│   └────────────────────────────────────────────┘     │
│                         |                            │
│   ┌────────────────────────────────────────────┐     │
│   │     SQLite  (WAL · single-writer · zero    │     │
│   │              external dependencies)        │     │
│   └────────────────────────────────────────────┘     │
└──────────────────────────────────────────────────────┘
```

---

## Design Philosophy

**Single binary** — The Vue 3 frontend is compiled to static assets and embedded in the Go binary via `embed.FS`. One file to deploy, nothing else to configure.

**Zero external dependencies** — SQLite is the only database. No Redis, no Postgres, no message queue. The binary runs anywhere Go compiles.

**Real-time by default** — Every state change is pushed to the browser via Server-Sent Events (SSE). No polling, no stale data.

**Read-only** — maintenant never modifies your containers. It observes the Docker socket or Kubernetes API in read-only mode.

**Label-driven** — Monitoring is configured through Docker labels directly on your containers. No separate config files to maintain.

**Runtime-agnostic** — Docker and Kubernetes are abstracted behind a common `Runtime` interface. maintenant auto-detects the runtime at startup or can be forced via `MAINTENANT_RUNTIME`.

---

## Tech Stack

### Backend

| Technology | Purpose |
|-----------|---------|
| **Go** (>= 1.25) | Application runtime |
| **SQLite** (WAL mode) | Persistence, single-writer pattern |
| **`net/http`** (stdlib) | HTTP server, REST API, SSE |
| **`github.com/docker/docker`** | Docker SDK for container discovery and events |
| **`k8s.io/client-go`** | Kubernetes API client |
| **`k8s.io/metrics`** | Kubernetes metrics API |
| **`github.com/mattn/go-sqlite3`** | SQLite driver (CGO) |
| **`github.com/google/go-containerregistry`** | OCI registry scanning |
| **`github.com/modelcontextprotocol/go-sdk`** | MCP server (AI assistant integration) |
| **`embed.FS`** | Frontend embedding |

### Frontend

| Technology | Purpose |
|-----------|---------|
| **Vue 3** | UI framework (Composition API) |
| **TypeScript 5.9** | Type safety |
| **Pinia** | State management (SSE-connected stores) |
| **Tailwind CSS 4** | Styling |
| **uPlot** | Lightweight time-series charts (~40 KB) |
| **Vite** | Build tooling |
| **vite-plugin-pwa** | Progressive Web App support |

---

## Project Structure

```
cmd/maintenant/            Entry point, service wiring
  web/                     Embedded frontend (embed.FS)
internal/                  Private packages
    alert/                 Alert engine, notifier, formatters (webhook, discord)
    api/v1/                HTTP handlers, SSE broker, router
    certificate/           TLS certificate monitoring
    container/             Container model, service, uptime
    docker/                Docker runtime implementation
    endpoint/              Endpoint monitoring (HTTP/TCP)
    event/                 Event types and dispatching
    extension/             Extension point interfaces + no-ops (used by Pro)
    heartbeat/             Heartbeat/cron monitoring
    kubernetes/            Kubernetes runtime implementation
    license/               License validation and management
    mcp/                   MCP server (Model Context Protocol)
    ratelimit/             Per-IP rate limiting middleware
    resource/              Resource metrics collection
    runtime/               Runtime abstraction interface
    security/              Network security analysis, posture scoring
    status/                Public status page (handler, subscribers)
    store/sqlite/          SQLite store layer, migrations, writer
    update/                Update intelligence, registry scanning
    webhook/               Webhook dispatcher

frontend/src/
  pages/                   Vue page components
  components/              Reusable UI components
    ui/                    Generic UI primitives
    dashboard/             Dashboard-specific widgets
  stores/                  Pinia stores (SSE-connected)
  services/                API client functions
  composables/             Vue composables
  layouts/                 Page layouts
  utils/                   Utility functions
  router/                  Vue Router configuration
```

---

## Data Flow

### Container Event

```
Docker/K8s Event
  → Runtime.StreamEvents()
    → container.Service.ProcessEvent()
      → SQLite (persist state transition)
      → SSE Broker.Broadcast()
        → Browser (real-time update)
        → Alert Engine (evaluate rules)
        → Webhook Dispatcher (deliver to channels)
```

### Endpoint Check

```
Check Engine (ticker)
  → HTTP/TCP request
    → endpoint.Service.ProcessCheckResult()
      → SQLite (persist check result)
      → SSE Broker.Broadcast()
        → Browser (update sparkline)
      → Alert Detector (evaluate thresholds)
        → Alert Engine → Notifier → Webhook channels
      → Certificate Service (auto-detect TLS from HTTPS)
```

---

## SQLite Architecture

maintenant uses SQLite in WAL (Write-Ahead Logging) mode with a single-writer pattern:

- **One writer goroutine** — All writes are serialized through a channel-based writer to avoid `SQLITE_BUSY` errors
- **Multiple readers** — Read queries run concurrently without blocking
- **Automatic migrations** — Schema migrations run at startup using embedded SQL files
- **Resource history** — Charts up to 24h group raw samples; the 7-day range reads the hourly rollup, which holds exactly the buckets it displays. Raw samples are therefore only kept for 48 hours, and they are what dominates database size.
- **Retention cleanup** — Background goroutine prunes old data (transitions: 90 days, check results: 30 days, heartbeat pings: 30 days, resource snapshots: 48 hours, resource hourly: 90 days, resource daily: 1 year). A pass runs at startup and then hourly, deleting in batches of 1000 rows until each table is drained. If a pass hits its 2-minute-per-table budget it reschedules itself a minute later instead of waiting for the next hour, so a backlog is cleared instead of accumulating. Tunable with `MAINTENANT_RETENTION_SNAPSHOTS`, `MAINTENANT_RETENTION_INTERVAL` and `MAINTENANT_RETENTION_BATCH_SIZE`.
- **Bounded WAL** — Every pooled connection sets `journal_size_limit` (64 MiB) and `wal_autocheckpoint` (1000 pages) through a driver `ConnectHook`; pages freed by retention are returned to the filesystem with `incremental_vacuum` in slices

---

## SSE Architecture

The SSE broker is the central hub for real-time updates:

1. **Services** emit events when state changes (container state, check result, alert fired)
2. **SSE Broker** fans out events to all connected browser clients
3. **Webhook Dispatcher** observes the broker and delivers events to external channels
4. **Alert Engine** processes events and generates alerts

Each browser tab maintains a single SSE connection to `/api/v1/containers/events`. The Pinia stores dispatch received events to the appropriate component.

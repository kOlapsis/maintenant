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
│   │  Updates · Status Page · Webhooks          │     │
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
pkg/                       Public packages (importable by Pro)
    app/                   AppBuilder, composable application assembly
    alert/                 Types, interfaces, CE formatters (webhook, discord)
    pro/                   Extension point interfaces + no-ops
internal/                  Private packages
    api/v1/                HTTP handlers, SSE broker, router
    alertengine/           Alert engine, notifier
    container/             Container model, service, uptime
    docker/                Docker runtime implementation
    kubernetes/            Kubernetes runtime implementation
    runtime/               Runtime abstraction interface
    endpoint/              Endpoint monitoring (HTTP/TCP)
    heartbeat/             Heartbeat/cron monitoring
    certificate/           TLS certificate monitoring
    resource/              Resource metrics collection
    update/                Update intelligence, registry
    status/                Public status page (handler, templates)
    mcp/                   MCP server (Model Context Protocol)
    webhook/               Webhook dispatcher
    store/sqlite/          SQLite store layer, migrations, writer

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
- **Retention cleanup** — Background goroutine prunes old data (events: 30 days, alerts: 90 days, updates: 30 days)

---

## SSE Architecture

The SSE broker is the central hub for real-time updates:

1. **Services** emit events when state changes (container state, check result, alert fired)
2. **SSE Broker** fans out events to all connected browser clients
3. **Webhook Dispatcher** observes the broker and delivers events to external channels
4. **Alert Engine** processes events and generates alerts

Each browser tab maintains a single SSE connection to `/api/v1/containers/events`. The Pinia stores dispatch received events to the appropriate component.

# Resource Metrics

Real-time CPU, memory, network I/O, and disk I/O per container. Historical charts, per-container alert thresholds, and a top consumers view for instant triage.

---

## Metrics Collected

maintenant collects the following metrics for each container:

| Metric | Description |
|--------|-------------|
| **CPU usage** | Percentage of allocated CPU used by the container |
| **Memory usage** | Current memory consumption and limit |
| **Network I/O** | Bytes received and transmitted |
| **Disk I/O** | Bytes read and written to block devices |

=== "Docker"

    Metrics are collected via the Docker `ContainerStatsOneShot` API. No additional configuration needed — if maintenant can see the container, it collects metrics.

=== "Kubernetes"

    Metrics are collected from the Kubernetes Metrics API (`metrics.k8s.io`). Requires `metrics-server` to be installed in the cluster.

---

## Historical Charts

maintenant stores metric snapshots and displays them as interactive time-series charts (powered by uPlot). Every edition sees a history: what an edition buys is how far back it goes.

| Window | Edition | Served from | Granularity |
|--------|---------|-------------|-------------|
| 1 hour | Community | raw samples | raw |
| 6 hours | Community | raw samples | 1 minute |
| 24 hours | :material-star-four-points:{ title="Personal" } Personal | raw samples | 5 minutes |
| 7 days | :material-star-four-points:{ title="Personal" } Personal | hourly rollup | 1 hour |
| 30 days | :material-star-four-points:{ title="Personal" } Personal | hourly rollup | 1 hour |
| 90 days | :material-star-four-points:{ title="Pro" } Pro | daily rollup | 1 day |

Each window reads a table kept strictly longer than the window itself, so a retention pass can never shorten a chart while you are looking at it. That is why 90 days comes from the daily rollup, kept a year, and not from the hourly one, kept exactly ninety days.

!!! note "Disk I/O on the 90-day window"
    The daily rollup carries CPU, memory and network, but not the block I/O
    counters. On the 90-day window only, **Disk I/O reads zero**. Every shorter
    window has it.

The cap is a duration, not a list. The window catalogue and the edition that opens each entry are reported by the API, so the interface never holds a copy that could drift:

```
GET /api/v1/edition
```

```json
{
  "resource_history": {
    "max_window": "6h",
    "max_window_seconds": 21600,
    "windows": [
      { "window": "1h",  "seconds": 3600,    "min_edition": "community" },
      { "window": "90d", "seconds": 7776000, "min_edition": "pro" }
    ]
  }
}
```

Access historical data via the API:

```
GET /api/v1/containers/{id}/resources/history?range=24h
```

A window above your edition's cap is refused with `403 EDITION_REQUIRED`, naming the edition that opens it and your current cap. It is never silently shortened to the cap. A window the product does not know is a `400` instead: a bad request, not an edition question.

The same catalogue and the same cap apply to the [top consumers](#top-consumers-view) and to the `get_top_consumers` MCP tool. There is no window that one surface serves and another refuses.

---

## Per-Container Alert Thresholds

Set custom alert thresholds for any container. When a metric exceeds the threshold for a sustained period (debounce), an alert is fired.

### Configure via API

```bash
PUT /api/v1/containers/{id}/resources/alerts
{
  "cpu_threshold": 90,
  "memory_threshold": 85,
  "debounce_seconds": 60
}
```

- **cpu_threshold** — Fire alert when CPU usage exceeds this percentage
- **memory_threshold** — Fire alert when memory usage exceeds this percentage
- **debounce_seconds** — How long the metric must exceed the threshold before alerting (prevents noise from transient spikes)

!!! tip "Debounce to avoid noise"
    Set a reasonable debounce period (60-120 seconds) to avoid alerts from
    short CPU spikes during deployments or startup.

---

## Top Consumers View

The top consumers view shows which containers are using the most resources, sorted by CPU or memory usage. Useful for quick triage when your host is under pressure.

```
GET /api/v1/resources/top?metric=cpu&limit=10           # live ranking, every edition
GET /api/v1/resources/top?metric=cpu&period=30d&limit=10 # ranked over a history window
```

Omitting `period` ranks containers on their latest sample and is open in every edition. Passing one reads history, and the edition cap applies exactly as it does to the per-container charts.

---

## Resource Summary

Get an aggregate view of resource usage across all monitored containers:

```
GET /api/v1/resources/summary
```

---

## Multi-Host

In a [multi-host](multihost.md) deployment (Pro), resource metrics are reported per machine:

- Each agent streams its host's **machine-level CPU, memory and disk**, in addition to per-container stats.
- The resource header gains a **host selector** (hidden when there is a single host). Selecting a host scopes the CPU / MEM / DISK gauges and the top consumers widget to that machine.
- Scope any resources call to one host with `?agent_id=local` (the central server) or `?agent_id=<id>` (a remote agent). Omitting it returns the local server for the summary, and aggregates all hosts for top consumers.

```
GET /api/v1/resources/hosts                       # list hosts + current metrics
GET /api/v1/resources/summary?agent_id=<id>       # summary for one host
GET /api/v1/resources/top?metric=cpu&agent_id=<id># top consumers for one host
```

See [Multi-Host Monitoring](multihost.md) for the full agent/server setup.

---

## Alert Events

| Event | Description | Default Severity |
|-------|-------------|------------------|
| `cpu_threshold` | CPU usage exceeded threshold for debounce period | Warning |
| `memory_threshold` | Memory usage exceeded threshold for debounce period | Warning |

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/containers/{id}/resources/current` | Current resource usage |
| `GET` | `/api/v1/containers/{id}/resources/history` | Historical metrics |
| `GET` | `/api/v1/containers/{id}/resources/alerts` | Get alert config |
| `PUT` | `/api/v1/containers/{id}/resources/alerts` | Set alert thresholds |
| `GET` | `/api/v1/resources/summary` | Aggregate resource summary |
| `GET` | `/api/v1/resources/top` | Top consumers |

---

## Related

- [Container Monitoring](containers.md) — Container states and health checks
- [Alert Engine](alerts.md) — Resource threshold alerts

---

## Changelog

**2026-08-27, history windows per edition.** Until this release the history API
was open to Personal and above as a whole, so a Community instance saw no chart
at all, and the 30-day period of the top consumers endpoint was reachable from
any edition by calling it directly. Both are fixed: Community now sees 1h and
6h, Pro gains a 90-day window, and every surface enforces the cap server-side.
The tiering above is what the product serves, and what the pricing page states.

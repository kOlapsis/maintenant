# Resource Metrics

Real-time CPU, memory, network I/O, and disk I/O per container. Historical charts, per-container alert thresholds, and a top consumers view for instant triage.

---

## Metrics Collected

maintenant collects the following metrics for each running container, every 10 seconds:

| Metric | Description |
|--------|-------------|
| **CPU usage** | Computed like `docker stats`: 100% is one full core, so a container can go above 100% on a multi-core host |
| **Memory usage** | Working set (usage minus the reclaimable file cache) and the container's memory limit. Without a limit, Docker reports the host's memory |
| **Network I/O** | Bytes received and transmitted, summed over all interfaces |
| **Disk I/O** | Bytes read and written to block devices |

On Docker, the first sample of a container only sets the baseline its CPU is measured against, so its first chart point comes one interval later. Ignored containers ([`maintenant.ignore`](containers.md#excluding-containers)) and stopped containers are not sampled.

=== "Docker"

    Metrics are collected via the Docker `ContainerStatsOneShot` API. No additional configuration needed: if maintenant can see the container, it collects metrics.

=== "Kubernetes"

    Metrics are collected from the Kubernetes Metrics API (`metrics.k8s.io`). Requires `metrics-server` to be installed in the cluster. A workload reports the CPU and memory of its pods added together (1000 millicores read as 100%), against the sum of their memory limits (no limit reads as 0). Network and disk I/O are not available.

---

## Historical Charts

maintenant stores metric snapshots and displays them as interactive time-series charts (powered by uPlot). Every edition sees a history: what an edition buys is how far back it goes.

| Window | Edition | Served from | Granularity |
|--------|---------|-------------|-------------|
| 1 hour | Community | raw samples | raw |
| 6 hours | Community | raw samples | 1 minute |
| 24 hours | Community | raw samples | 5 minutes |
| 7 days | Community | hourly rollup | 1 hour |
| 30 days | :material-star-four-points:{ title="Personal" } Personal | hourly rollup | 1 hour |
| 90 days | :material-star-four-points:{ title="Pro" } Pro | daily rollup | 1 day |

Raw samples are kept for 48 hours by default (`MAINTENANT_RETENTION_SNAPSHOTS`, never less than 24 hours, since the 24-hour chart reads them). Every 5 minutes, the raw samples are rolled up into hourly and daily rows. The hourly rows are kept 90 days and the daily rows 365 days.

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
    "max_window": "7d",
    "max_window_seconds": 604800,
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

`range` defaults to `1h`. The response holds the `range`, the `granularity` and the `points` (timestamp, CPU percent, memory used and limit, network and block I/O bytes).

A window above your edition's cap is refused with `403 EDITION_REQUIRED`, which names the edition that opens it (`required_edition`), the window asked for and your current cap (`max_window`). It is never silently shortened to the cap. A window the product does not know is a `400 INVALID_RANGE` instead: a bad request, not an edition question.

The same catalogue and the same cap apply to the [top consumers](#top-consumers-view) and to the `get_top_consumers` MCP tool. There is no window that one surface serves and another refuses.

---

## Per-Container Alert Thresholds

Set custom alert thresholds for any container. When a metric stays at or above its threshold for two samples in a row, an alert is fired.

### Configure via API

```bash
PUT /api/v1/containers/{id}/resources/alerts
{
  "cpu_threshold": 90,
  "mem_threshold": 85,
  "enabled": true
}
```

- **cpu_threshold**: alert when CPU usage reaches this percentage, from 1 to 1000. CPU is measured like `docker stats`, so 200 means two full cores.
- **mem_threshold**: alert when memory usage reaches this percentage of the container's memory limit, from 1 to 100.
- **enabled**: alerts are only evaluated while this is `true`. It has no default, so always send it: a body without it saves the thresholds with the alerts off.

`GET` on the same route returns the saved configuration, or `90` / `90` with `enabled: false` for a container that has none.

The debounce is fixed at **two consecutive samples**, about 20 seconds at the 10-second sampling rate, and cannot be changed. A sample back under the threshold resets the count. On Docker, a container without a memory limit is measured against the host's memory; on Kubernetes, a workload with no memory limit never raises a memory alert.

!!! tip "Transient spikes"
    Two samples already absorb a one-off spike. For a container whose CPU
    legitimately peaks during startup or a deployment, set the threshold above
    the peak, or leave alerts off until it has settled.

---

## Top Consumers View

The top consumers view shows which containers are using the most resources, sorted by CPU or memory usage. Useful for quick triage when your host is under pressure.

```
GET /api/v1/resources/top?metric=cpu&limit=10           # live ranking, every edition
GET /api/v1/resources/top?metric=cpu&period=30d&limit=10 # ranked over a history window
```

`metric` is required (`cpu` or `memory`, otherwise `400 INVALID_METRIC`), and `limit` defaults to 5 with a maximum of 20. Each entry has `container_id`, `container_name`, `value`, `percent` and `rank`.

Omitting `period` ranks containers on their latest sample and is open in every edition. That live ranking covers the containers of the server's own runtime: containers of a remote agent appear in the rankings over a `period`. Passing a `period` reads history (averaged over the window), and the edition cap applies exactly as it does to the per-container charts.

---

## Resource Summary

Get the load of a host, with the number of containers running on it and their network totals:

```
GET /api/v1/resources/summary
```

The CPU, memory and disk gauges are those of the machine itself (read from `/proc` and the root filesystem), not a sum over containers. `available` is `false` when a remote host has not reported recently.

---

## Multi-Host

In a [multi-host](multihost.md) deployment (Personal or Pro), resource metrics are reported per machine:

- Each agent streams its host's **machine-level CPU, memory and disk**, in addition to per-container stats. A host that has not reported for 35 seconds is shown as unavailable.
- The interface gets a **host scope selector** (hidden on a single host). Selecting a host scopes the CPU / MEM / DISK gauges and the top consumers widget to that machine.
- Scope any resources call to one host with `?agent_id=local` (the central server) or `?agent_id=<id>` (a remote agent). Omitting it returns the local server for the summary, and aggregates all hosts for top consumers over a `period`.

```
GET /api/v1/resources/hosts                       # list hosts + current metrics
GET /api/v1/resources/summary?agent_id=<id>       # summary for one host
GET /api/v1/resources/top?metric=cpu&period=24h&agent_id=<id> # top consumers for one host
```

The charts and the alert thresholds work the same way for the containers of a remote agent. While the agent is offline it keeps sampling into its spool, and what it replays afterwards fills the charts but never raises an alert. See [Multi-Host Monitoring](multihost.md) for the full agent/server setup.

---

## Alert Events

| Event | Description | Default Severity |
|-------|-------------|------------------|
| `cpu_threshold` | CPU usage stayed at or above the threshold for two samples in a row | Warning |
| `memory_threshold` | Memory usage stayed at or above the threshold for two samples in a row | Warning |

The alert message reads `Resource cpu threshold exceeded for container <name>` (or `memory`) and carries the current value and the threshold. When a single metric breached, the alert is resolved, with a recovery notification, once that metric is back under its threshold.

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/containers/{id}/resources/current` | Latest sample (`404` when the server has none: containers of a remote agent have no live sample here, only history) |
| `GET` | `/api/v1/containers/{id}/resources/history` | Historical metrics |
| `GET` | `/api/v1/containers/{id}/resources/alerts` | Get alert config |
| `PUT` | `/api/v1/containers/{id}/resources/alerts` | Set alert thresholds |
| `GET` | `/api/v1/resources/summary` | Host load and container totals |
| `GET` | `/api/v1/resources/hosts` | Every host with its current metrics |
| `GET` | `/api/v1/resources/top` | Top consumers |

---

## Related

- [Container Monitoring](containers.md): container states and health checks
- [Alert Engine](alerts.md): resource threshold alerts

---

## Changelog

**2026-08-27, history windows per edition.** Until this release the history API
was open to Personal and above as a whole, so a Community instance saw no chart
at all, and the 30-day period of the top consumers endpoint was reachable from
any edition by calling it directly. Both are fixed: Community now sees up to
7 days, Pro gains a 90-day window, and every surface enforces the cap
server-side. The tiering above is what the product serves, and what the pricing
page states.

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

maintenant stores metric snapshots and displays them as interactive time-series charts (powered by uPlot). Available time ranges:

| Range | Description |
|-------|-------------|
| 1 hour | Fine-grained, per-second resolution |
| 6 hours | Recent activity |
| 24 hours | Full day view |
| 7 days | Weekly trends |

Access historical data via the API:

```
GET /api/v1/containers/{id}/resources/history?range=24h
```

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
GET /api/v1/resources/top?sort=cpu&limit=10
```

---

## Resource Summary

Get an aggregate view of resource usage across all monitored containers:

```
GET /api/v1/resources/summary
```

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

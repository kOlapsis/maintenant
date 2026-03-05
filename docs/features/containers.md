# Container Monitoring

Zero-config auto-discovery for Docker and Kubernetes. Every container is tracked the moment it starts.

![Container Monitoring](../screen-captures/2-containers.png)

---

## How It Works

maintenant connects to your container runtime (Docker socket or Kubernetes API) and watches for container lifecycle events in real time. There is nothing to configure — every container is discovered automatically.

When a new container starts, maintenant immediately begins tracking:

- **State changes** — running, stopped, restarting, paused, exited (with exit codes)
- **Health checks** — Docker `HEALTHCHECK` status (healthy, unhealthy, starting)
- **Restart loops** — Detection and alerting when a container is stuck restarting
- **Uptime** — How long each container has been running

All state transitions are persisted in the database and pushed to the browser via SSE in real time.

---

## Auto-Discovery

=== "Docker"

    maintenant connects to the Docker daemon via the socket (`/var/run/docker.sock`). Mount it as read-only:

    ```yaml
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
    ```

    Every running container is discovered automatically. New containers are picked up the moment they start. maintenant never modifies your containers — it is strictly read-only.

=== "Kubernetes"

    maintenant uses the in-cluster Kubernetes API with a read-only ServiceAccount. It watches:

    - **Pods** — Individual container states, logs, events
    - **Deployments** — Rollout status, replica counts
    - **StatefulSets** — Ordered pod management
    - **DaemonSets** — Node-level workloads

    See the [Kubernetes Guide](../guides/kubernetes.md) for RBAC setup.

---

## Grouping

Containers are automatically grouped for easy navigation.

=== "Docker"

    - **Compose projects** — Containers from the same `docker-compose.yml` are grouped by project name.
    - **Custom groups** — Override with the `maintenant.group` label:

    ```yaml
    labels:
      maintenant.group: "backend"
    ```

=== "Kubernetes"

    Workloads are grouped by type: Deployments, DaemonSets, StatefulSets. Pods within each workload are displayed together.

---

## Health Checks

maintenant reads Docker `HEALTHCHECK` results automatically. No configuration needed — if your container defines a health check, maintenant tracks it.

Health states:

| State | Description |
|-------|-------------|
| `healthy` | Health check passing |
| `unhealthy` | Health check failing |
| `starting` | Container just started, health check not yet run |
| `none` | No health check defined |

When a container transitions to `unhealthy`, maintenant can trigger an alert through the [Alert Engine](alerts.md).

---

## Restart Loop Detection

maintenant detects containers stuck in restart loops. When a container exceeds the configured restart threshold within a time window, an alert is fired.

Configure the threshold per container:

```yaml
labels:
  maintenant.alert.restart_threshold: "5"  # Alert after 5 restarts
```

---

## Log Streaming

maintenant provides real-time log streaming with stdout/stderr demultiplexing. Logs are streamed directly from the container runtime — they are not stored in maintenant's database.

Access logs via the API:

- `GET /api/v1/containers/{id}/logs` — Fetch recent logs
- `GET /api/v1/containers/{id}/logs/stream` — SSE stream of live logs

---

## Excluding Containers

To exclude a container from maintenant monitoring:

```yaml
labels:
  maintenant.ignore: "true"
```

This is useful for infrastructure containers (reverse proxies, sidecars) that add noise to your dashboard.

---

## Related

- [Docker Labels Reference](../guides/docker-labels.md) — Full list of container labels
- [Alert Engine](alerts.md) — Configure alerts for container events
- [Resource Metrics](resources.md) — CPU, memory, and network metrics per container

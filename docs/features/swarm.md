# Docker Swarm Monitoring

Automatic discovery and monitoring of Docker Swarm clusters. Services, tasks, nodes, and rolling updates — all visible without configuration.

---

## How It Works

When maintenant runs on a **Swarm manager node**, it automatically detects Swarm mode via the Docker daemon and discovers all services, tasks, and nodes in the cluster. No configuration is needed — the same "observe without config" experience as standalone Docker containers.

Swarm services appear in the existing container dashboard, grouped by service (and stack when applicable). Standalone containers not managed by Swarm continue to appear normally alongside Swarm services.

---

## Swarm Detection

On startup, maintenant queries the Docker daemon to determine:

1. **Is the engine in Swarm mode?** — If not, standard Docker monitoring is used.
2. **Is this node a manager?** — Only manager nodes have access to the Swarm management API.

| Scenario | Behavior |
|----------|----------|
| Not a Swarm node | Standard Docker container monitoring |
| Swarm worker node | Standard Docker container monitoring + log message |
| Swarm manager node | Full Swarm monitoring (services, tasks, nodes) |

Detection is fully automatic. If the node is demoted from manager to worker at runtime, maintenant gracefully degrades to container-only monitoring and broadcasts a status update.

---

## Service Discovery

maintenant calls the Docker Swarm API (`ServiceList`, `TaskList`) to discover all services and their tasks. Each service is tracked with:

- **Service name** and image
- **Mode** — replicated (fixed replica count) or global (one task per node)
- **Replica health** — desired vs running count (e.g., "3/3 running")
- **Task states** — running, failed, shutdown, pending, preparing
- **Published ports** — with protocol, target port, published port, and publish mode (ingress or host)
- **Attached networks** — overlay, ingress, and custom networks with scope

Services with zero running tasks display "0/N running" correctly.

### Real-Time Updates

Swarm service events (create, update, remove) are streamed in real time via the Docker event API and pushed to the browser via SSE. A new service appears in the dashboard within seconds of creation.

### Startup Reconciliation

On startup (or after a reconnection), maintenant performs a full reconciliation — discovering all services and tasks to ensure the dashboard reflects the current cluster state, even if events were missed.

---

## Grouping

Swarm services are grouped in the dashboard using two mechanisms:

### Stack Grouping

Services deployed via `docker stack deploy myapp` are automatically grouped under their stack name. maintenant reads the `com.docker.stack.namespace` label set by Docker.

```bash
docker stack deploy -c docker-compose.yml myapp
# All services appear grouped under "myapp"
```

### Custom Grouping

Override the default grouping with the `maintenant.group` label on the service:

```yaml
# In your docker-compose.yml (for stack deploy)
services:
  api:
    image: myapp:latest
    deploy:
      labels:
        maintenant.group: "production"
```

!!! note "Label precedence"
    When both `com.docker.stack.namespace` and `maintenant.group` are present, `maintenant.group` takes precedence.

---

## Supported Labels

maintenant reads labels from **Swarm service definitions** (not individual containers). All standard `maintenant.*` labels are supported:

| Label | Values | Description |
|-------|--------|-------------|
| `maintenant.group` | any string | Custom group name (overrides stack namespace) |
| `maintenant.ignore` | `true` | Exclude this service from monitoring |
| `maintenant.alert.severity` | `critical`, `warning`, `info` | Default alert severity |
| `maintenant.alert.restart_threshold` | integer | Restart loop threshold |
| `maintenant.alert.channels` | comma-separated | Route alerts to specific channels |

Labels must be placed in the `deploy.labels` section (service-level), not the top-level `labels` section (container-level):

```yaml
services:
  api:
    image: myapp:latest
    deploy:
      labels:
        maintenant.group: "backend"
        maintenant.alert.severity: "critical"
        maintenant.alert.channels: "ops-webhook"
```

---

## Community vs Enterprise Features

### Community Edition (Free)

Basic Swarm awareness — the "observe without config" equivalent for Swarm:

- Swarm mode detection and auto-discovery
- Service listing with mode, image, and replica counts
- Task states (running, failed, shutdown)
- Service grouping by stack and custom labels
- Published ports with mode (ingress/host)
- Attached network visibility
- Real-time SSE updates on service events
- Startup reconciliation
- Worker node fallback

### Enterprise Edition

Cluster intelligence — analysis, alerting, and a dedicated dashboard:

- **Node health overview** — All nodes with role, status, availability, engine version, task count
- **Node alerting** — Alerts when nodes go down, are drained, or quorum is degraded
- **Crash-loop detection** — Automatic detection of task failure patterns (3+ failures in 5 minutes)
- **Replica health alerting** — Alerts when running replicas fall below desired count
- **Rolling update tracking** — Real-time progress, stall detection, rollback alerts
- **Dedicated Swarm dashboard** — Cluster-wide view with nodes, services, task distribution, and aggregate health

---

## API Endpoints

### Community Edition

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/swarm/info` | Cluster status (active, cluster_id, is_manager, node counts) |
| `GET` | `/api/v1/swarm/services` | List all services with task summary. Supports `?stack=` filter |
| `GET` | `/api/v1/swarm/services/{serviceID}` | Service detail with full task list |

### Enterprise Edition

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/swarm/nodes` | List all nodes with status and task counts |
| `GET` | `/api/v1/swarm/nodes/{nodeID}` | Node detail with task list |
| `GET` | `/api/v1/swarm/services/{serviceID}/update-status` | Rolling update progress |
| `GET` | `/api/v1/swarm/dashboard` | Aggregated cluster dashboard data |

### SSE Events

| Event | Edition | Description |
|-------|---------|-------------|
| `swarm.status` | CE | Cluster status changes (active, manager demotion) |
| `swarm.service_discovered` | CE | New service detected |
| `swarm.service_updated` | CE | Service configuration or task state changed |
| `swarm.service_removed` | CE | Service removed from cluster |
| `swarm.node_status_changed` | Enterprise | Node status or availability changed |
| `swarm.task_failed` | Enterprise | Individual task failure |
| `swarm.crash_loop_detected` | Enterprise | Crash-loop pattern detected on a service |
| `swarm.crash_loop_recovered` | Enterprise | Service recovered from crash-loop |
| `swarm.update_progress` | Enterprise | Rolling update progress tick |
| `swarm.update_completed` | Enterprise | Rolling update finished |

---

## Worker Node Fallback

When maintenant detects it is running on a **worker node** (Swarm active but not a manager), it:

1. Logs a clear message: Swarm monitoring requires a manager node
2. Falls back to standard Docker container monitoring
3. Broadcasts `swarm.status` with `is_manager: false`

There are no errors in the UI — the dashboard shows standalone containers as usual. To enable Swarm monitoring, deploy maintenant on a manager node.

---

## Related

- [Docker Labels Reference](../guides/docker-labels.md) — Full label reference
- [Container Monitoring](containers.md) — Standalone container monitoring
- [Alert Engine](alerts.md) — Alert routing and notification channels
- [Swarm Deployment Guide](../guides/swarm.md) — Deployment requirements and setup

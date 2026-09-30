# Docker Swarm Monitoring

Automatic discovery and monitoring of Docker Swarm clusters: services, tasks, nodes, rolling updates and the alerts that go with them. Every Swarm feature is available in every edition, Community included.

---

## How It Works

When maintenant runs on a **Swarm manager node**, it detects Swarm mode through the Docker daemon and discovers all services, tasks and nodes of the cluster. No configuration is needed: it is the same "observe without config" experience as standalone Docker containers.

The dashboard gets Services, Tasks, Nodes and Cluster Overview pages. Standalone containers that Swarm does not manage keep appearing in the container views next to the tasks.

!!! note "Containers are local, services are cluster-wide"
    A Docker daemon only lists its own containers. The container views of a manager show the tasks that run **on that manager**. The Services, Tasks and Nodes pages cover the whole cluster. To monitor the containers of the other nodes (state, health, resources, logs, endpoints) as well, run an [agent](#swarm-through-an-agent) on them.

---

## Swarm Detection

On startup, maintenant asks the Docker daemon:

1. **Is the engine in Swarm mode?** If not, standard Docker monitoring is used.
2. **Is this node a manager?** Only managers can read services, tasks and nodes.

| Scenario | Behavior |
|----------|----------|
| Not a Swarm node | Standard Docker container monitoring |
| Swarm worker node | Standard Docker container monitoring, plus a log message |
| Swarm manager node | Full Swarm monitoring (services, tasks, nodes, alerts) |

Detection is rechecked every 60 seconds while maintenant runs. If Swarm mode is switched on or the node becomes a manager, maintenant starts Swarm monitoring without a restart. If the node leaves the swarm or is demoted, the Swarm views report `active: false`. Both changes are announced to the browser with a `runtime.context_changed` event, at most 60 seconds after they happen.

Detection is set up only when the Docker daemon answers at startup. If maintenant starts while Docker is unreachable, restart it once Docker is back for Swarm to be detected.

---

## Service Discovery

maintenant lists the services and tasks of the cluster (`ServiceList`, `TaskList`). Each service is tracked with:

- **Service name**, image and stack
- **Mode**: replicated (fixed replica count) or global (one task per node)
- **Replica health**: desired against running count (for example "3/3 running")
- **Task states**: running, failed, shutdown, pending, preparing
- **Published ports**: protocol, target port, published port, and publish mode (ingress or host)
- **Attached networks**: overlay, ingress and custom networks, with scope
- **Rolling update status**

Services with zero running tasks display "0/N running" correctly.

### Real-Time Updates

Service events (create, update, remove) and node events are read from the Docker event stream and pushed to the browser over SSE. Each event refreshes the service in maintenant's cache, which the alerts use, and a node event triggers an immediate node check. The Services and Tasks pages follow the snapshot taken every 30 seconds.

### Reconciliation

On startup, and again after the Docker connection was lost, maintenant rebuilds the full picture of services, tasks and nodes, so the dashboard is right even if events were missed. The services and tasks are also snapshotted every 30 seconds, and the nodes every 60 seconds.

---

## Grouping

The container views group containers as follows, from the most specific rule to the least:

1. `maintenant.group`, when a label sets it.
2. The Compose project of the container, if it has one.
3. The Swarm stack: `com.docker.stack.namespace`, set by `docker stack deploy`.

A container with none of these is listed under "Ungrouped".

```bash
docker stack deploy -c docker-compose.yml myapp
# All services appear grouped under "myapp"
```

Inside a group, task containers are gathered under their **service**. The service row shows how many of its tasks run on the local node, and each task shows its service name and its slot (`slot 2`). A task of a global service has no slot.

Override the group with the `maintenant.group` label on the service:

```yaml
# In your docker-compose.yml (for stack deploy)
services:
  api:
    image: myapp:latest
    deploy:
      labels:
        maintenant.group: "production"
```

---

## Labels

maintenant reads the labels of the **service** (`deploy.labels`) and lays the labels of each task container over them. When a key is set in both places, the container's value wins. Any `maintenant.*` label works on a service: `maintenant.group`, `maintenant.ignore`, `maintenant.alert.severity`, `maintenant.alert.restart_threshold`, `maintenant.endpoint.*`, `maintenant.tls.certificates` and `maintenant.update.*`, and so do the Traefik and Caddy labels when `MAINTENANT_PROXY_LABELS` is on.

```yaml
services:
  api:
    image: myapp:latest
    deploy:
      labels:
        maintenant.group: "backend"
        maintenant.alert.severity: "critical"
```

- The labels of a service are cached for 30 seconds.
- Only a manager can read services. Where maintenant or an agent runs on a worker, the labels of the container (the top-level `labels` of the stack file) are the only ones it sees.
- `maintenant.ignore` on a service hides its tasks and silences the replica, crash-loop and rolling update alerts of that service.
- Alert routing is not a label: channels and triggers decide who is notified. The former `maintenant.alert.channels` label was removed.

See [Swarm service labels](../guides/docker-labels.md#swarm-service-labels) in the labels reference for the details.

---

## Alerts

The server's own Swarm manager raises the alerts below. They go through the [Alert Engine](alerts.md) like any other, with the source `swarm`.

| Alert | Severity | Raised when | Resolved when |
|-------|----------|-------------|---------------|
| `replica_unhealthy` | Warning | A replicated service has fewer running tasks than desired replicas for 5 minutes. The check runs every 60 seconds. A change to the service itself (a scale, an image update) is also evaluated on the spot, without the delay. | The running count reaches the desired count again, the service is scaled to zero, ignored or removed |
| `crash_loop` | Critical | The task containers of a service died 3 times within 5 minutes, whatever their exit code. Only the containers of the node where maintenant runs are seen. | The service had no new failure for 10 minutes, or was removed |
| `update_rollback` | Warning | A rolling update ended in a rollback | The next rolling update of the service completes, or the service is ignored or removed |
| `update_stalled` | Warning | A rolling update is paused | The next rolling update of the service completes, or the service is ignored or removed |
| `node_down` | Critical | A node that was ready becomes `down` or `disconnected` | The node is ready again, or left the cluster |
| `node_drain` | Warning | The availability of a node is set to `drain` | The node leaves `drain`, or left the cluster |
| `quorum_degraded` | Critical | Fewer managers are ready than a majority of the managers (for 3 managers, fewer than 2) | The quorum is back |

- Global services and services with zero desired replicas raise no replica alert.
- Node alerts are per node: two nodes down raise two alerts. Node state is read every 60 seconds and whenever a node event arrives.
- The alert of a service or node that the manager no longer lists is resolved automatically (checked every 30 seconds).
- After a restart, maintenant takes over the Swarm alerts that were open and resolves them once their condition is gone. Nothing is left open forever by a restart.
- Alerts come from the server's own cluster. An agent reports its Swarm for display only and raises none of them.

Each alert also has its own SSE event: `swarm.node_status_changed`, `swarm.task_failed`, `swarm.crash_loop_detected`, `swarm.crash_loop_recovered`, `swarm.update_progress` and `swarm.update_completed`.

---

## Nodes and Cluster Overview

The Nodes page lists every node with its role, status, availability, engine version and number of running tasks. The Cluster Overview page shows the health of the whole cluster: `healthy`, `degraded` (a node is drained or paused, or a service has fewer running tasks than desired) or `unhealthy` (a node is down or disconnected, or a replicated service has no running task). Both pages are open in every edition.

---

## Swarm Through an Agent

An [agent](../guides/agent-setup.md) on a Swarm manager reports the cluster of that manager to a central maintenant: it is detected automatically, or forced with `--runtime=swarm` (`MAINTENANT_RUNTIME=swarm`). Every 30 seconds it sends a snapshot of the services, tasks and nodes, which shows up in the Services, Tasks and Nodes pages with the name of the agent. The Swarm fields of its containers (service, node, slot) are reported too. Agents are a multi-host feature: they need the Personal edition or above.

- Run the agent on a **manager**. On a worker it reports the containers but the topology snapshot fails, because a worker cannot list services.
- The alerts above are not raised for an agent's cluster.
- The API lists take an `agent_id` parameter to pick one agent, or `local` for the server's own swarm.

---

## Worker Node Fallback

When maintenant runs on a **worker node** (Swarm active, not a manager), it:

1. Logs that Swarm monitoring needs a manager node.
2. Falls back to standard Docker container monitoring. Its containers still show the service and slot of their task, taken from the labels Docker sets.
3. Answers `{"active": false}` on `/api/v1/swarm/info` and shows no Swarm page.

The UI shows no error. To get Swarm monitoring, run maintenant on a manager node.

---

## API Endpoints

All Swarm routes are read-only and open in every edition. Except where noted, they take an optional `agent_id` query parameter: `local` for the server's own swarm, an agent ID for one agent, nothing for all of them.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/swarm/info` | Whether the server is a Swarm manager (`active`), the `cluster_id` and `is_manager`. `{"active": false}` otherwise. No `agent_id`. |
| `GET` | `/api/v1/swarm/services` | List services with their replica counts. `?stack=` filters by stack. |
| `GET` | `/api/v1/swarm/services/{serviceID}` | Service detail with its tasks. `404 SWARM_SERVICE_NOT_FOUND` if unknown. |
| `GET` | `/api/v1/swarm/tasks` | List tasks. `?service=`, `?node=` and `?state=` filter them. |
| `GET` | `/api/v1/swarm/nodes` | List nodes with role, status and task count, plus manager and worker counts. |
| `GET` | `/api/v1/swarm/nodes/{nodeID}` | Node detail with the tasks running on it (from the server's own cluster). `404 SWARM_NODE_NOT_FOUND` if unknown. |
| `GET` | `/api/v1/swarm/services/{serviceID}/update-status` | Rolling update progress. Server's own cluster only. |
| `GET` | `/api/v1/swarm/services/{serviceID}/resources` | CPU, memory and network of each running task. The values are `null` for a task that does not run on the node where maintenant runs. Server's own cluster only. |
| `GET` | `/api/v1/swarm/dashboard` | Cluster summary with nodes, services and crash-loop state. Server's own cluster only. |
| `GET` | `/api/v1/swarm/cluster` | Cluster health, node counts by status and task totals. Server's own cluster only. |

The routes marked "server's own cluster only" answer `409 SWARM_NOT_ACTIVE` when the server is not a Swarm manager.

### SSE Events

| Event | Description |
|-------|-------------|
| `swarm.status` | The cluster is active, sent once when maintenant starts on a manager (and when Swarm is switched on while it runs). It is not sent again later: changes arrive as `runtime.context_changed`. |
| `swarm.service_discovered` | New service detected |
| `swarm.service_updated` | Service configuration changed, or its replica alert was raised |
| `swarm.service_removed` | Service removed from the cluster |
| `swarm.node_status_changed` | Node status or availability changed |
| `swarm.task_failed` | A task container died |
| `swarm.crash_loop_detected` | Crash-loop pattern detected on a service |
| `swarm.crash_loop_recovered` | Service recovered from a crash loop |
| `swarm.update_progress` | Rolling update progress tick |
| `swarm.update_completed` | Rolling update finished, or was rolled back |
| `swarm.topology_changed` | A snapshot from an agent was reconciled. Carries the `agent_id`. |
| `runtime.context_changed` | Swarm was switched on or off while maintenant runs |

---

## Related

- [Docker Labels Reference](../guides/docker-labels.md): full label reference
- [Container Monitoring](containers.md): standalone container monitoring
- [Alert Engine](alerts.md): channels, triggers and alert types
- [Swarm Deployment Guide](../guides/swarm.md): deployment requirements and setup
- [Multi-Host Monitoring](multihost.md): agents

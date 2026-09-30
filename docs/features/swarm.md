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

When it connects to Docker, maintenant asks the daemon:

1. **Is the engine in Swarm mode?** If not, standard Docker monitoring is used.
2. **Is this node a manager?** Only managers can read services, tasks and nodes.

| Scenario | Behavior |
|----------|----------|
| Not a Swarm node | Standard Docker container monitoring |
| Swarm worker node | Standard Docker container monitoring, plus a log message |
| Swarm manager node | Full Swarm monitoring (services, tasks, nodes, alerts) |

Detection runs as soon as maintenant is connected to Docker, including when Docker was unreachable at startup, and again every 60 seconds. If Swarm mode is switched on or the node becomes a manager, maintenant starts Swarm monitoring without a restart. If the node leaves the swarm or is demoted, the Swarm views report `active: false`. Both changes are announced to the browser with a `runtime.context_changed` event, at most 60 seconds after they happen.

The same check reads the number of managers and workers and the creation date of the swarm from `docker info`, so they follow the cluster within 60 seconds.

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

Service events (create, update, remove) and node events are read from the Docker event stream. Each service event is pushed to the browser over SSE, refreshes the service in maintenant's cache, which the alerts use, and hands the service to the replica check. An update event also checks the rolling update of the service. A node event triggers an immediate node check. The Services and Tasks pages follow the snapshot taken every 30 seconds.

### Reconciliation

On startup, and each time maintenant connects to Docker again after it could not reach it, maintenant rebuilds the full picture of services, tasks and nodes. The services and tasks are also snapshotted every 30 seconds, and the nodes every 60 seconds, so the dashboard is right even if events were missed.

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
- `maintenant.ignore` on a service hides its tasks and silences the replica, crash-loop and rolling update alerts of that service. It has to be set on the service: on a task container alone (top-level `labels`), it hides that container but the service alerts keep firing.
- Alert routing is not a label: channels and triggers decide who is notified. The former `maintenant.alert.channels` label was removed.

See [Swarm service labels](../guides/docker-labels.md#swarm-service-labels) in the labels reference for the details.

---

## Image Updates

[Update intelligence](updates.md) checks the image of each running task container, with the `maintenant.update.*` labels of its service. The commands it proposes for a task (the update, its rollback and the fix for a CVE) are `docker service update --image <image> <service>`. A standalone `docker stop`, `rm` and `run` would be undone by Swarm, which reschedules the task, so none is generated for a task.

---

## Alerts

The server's own Swarm manager raises the alerts below. They go through the [Alert Engine](alerts.md) like any other, with the source `swarm`.

| Alert | Severity | Raised when | Resolved when |
|-------|----------|-------------|---------------|
| `replica_unhealthy` | Warning | A replicated service has fewer running tasks than desired replicas for 5 minutes. The delay starts when the shortfall is first seen, by the check that runs every 60 seconds or by a service event (a scale, an image update), and a service that recovers before it ends raises nothing. | The running count reaches the desired count again, the service is scaled to zero, ignored or removed |
| `crash_loop` | Critical | 3 tasks of a service failed within 5 minutes, on any node of the cluster. Failed tasks are read from the snapshot taken every 30 seconds. A task that Swarm shut down (a rolling update, a scale-down) does not count, nor one that exited with code 0 or 143 (a clean stop). Exit code 137 counts, because the task list does not say whether the out-of-memory killer sent it. | The service had no new failure for 10 minutes, was removed, or is now ignored with `maintenant.ignore` |
| `update_rollback` | Warning | A rolling update ended in a rollback | The next rolling update of the service completes, or the service is ignored or removed |
| `update_stalled` | Warning | A rolling update is paused | The next rolling update of the service completes, or the service is ignored or removed |
| `node_down` | Critical | A node that was ready becomes `down` or `disconnected` | The node is ready again, or left the cluster |
| `node_drain` | Warning | The availability of a node is set to `drain` | The node leaves `drain`, or left the cluster |
| `quorum_degraded` | Critical | Fewer managers are ready than a majority of the managers (for 3 managers, fewer than 2), or the swarm has no leader, which stops the manager from listing its nodes | The quorum is back: the nodes can be listed again and a majority of managers is ready |

- Global services and services with zero desired replicas raise no replica alert.
- Node alerts are per node: two nodes down raise two alerts. Node state is read every 60 seconds and whenever a node event arrives.
- The alert of a service or node that the manager no longer lists is resolved automatically (checked every 30 seconds).
- After a restart, maintenant takes over the Swarm alerts that were open and resolves them once their condition is gone. Nothing is left open forever by a restart.
- Alerts come from the server's own cluster. An agent reports its Swarm for display only and raises none of them.

Some of these alerts come with an SSE event of their own: `swarm.node_status_changed`, `swarm.task_failed` (one for each failed task that counts toward a crash loop, whether or not the threshold is reached), `swarm.crash_loop_detected`, `swarm.crash_loop_recovered`, `swarm.update_progress` and `swarm.update_completed`. See [SSE Events](#sse-events).

---

## Nodes and Cluster Overview

The Nodes page lists every node with its role, status, availability, engine version and number of running tasks. The Cluster Overview page shows the health of the whole cluster: `healthy`, `degraded` (a node is drained or paused, or a service has fewer running tasks than desired) or `unhealthy` (a node is down or disconnected, or a replicated service has no running task). Both pages are open in every edition.

---

## Swarm Through an Agent

An [agent](../guides/agent-setup.md) on a Swarm manager reports the cluster of that manager to a central maintenant: it is detected automatically, or forced with `--runtime=swarm` (`MAINTENANT_RUNTIME=swarm`). Without the flag, the agent checks the Swarm membership of a Docker host every minute: joining or leaving a Swarm restarts collection under the new runtime, and leaving makes the server drop the cluster it held for that agent. Every 30 seconds it sends a snapshot of the services, tasks and nodes, which shows up in the Services, Tasks and Nodes pages with the name of the agent. The Swarm fields of its containers (service, node, slot) are reported too. Agents are a multi-host feature: they need the Personal edition or above.

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
| `GET` | `/api/v1/swarm/info` | Whether the server is a Swarm manager (`active`), with the `cluster_id`, `is_manager`, `manager_count`, `worker_count` and `created_at` of the swarm. `{"active": false}` otherwise. No `agent_id`. |
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

The manager and worker counts of `/api/v1/swarm/info`, `/api/v1/swarm/cluster`, `/api/v1/swarm/dashboard` and `/api/v1/runtime/status` (in its `metadata`) come from `docker info` and are refreshed every 60 seconds, like the `swarm.status` event and the `get_swarm_info` MCP tool. `/api/v1/swarm/nodes` counts the nodes it lists.

### MCP tools

With [MCP](mcp.md#docker-swarm-read) enabled, `get_swarm_info` returns the cluster (`active`, then `cluster` with its ID, creation date, manager and worker counts and `is_manager`). `list_swarm_services`, `list_swarm_tasks` (also filtered by `service_id`) and `list_swarm_nodes` read the same data as the routes above and take an `agent_id`, or `local`.

### SSE Events

| Event | Description | Payload keys |
|-------|-------------|--------------|
| `swarm.status` | The Swarm state changed: Swarm was activated or deactivated (`active: false`, no other key), or the manager or worker counts changed. It is checked every 60 seconds and is not sent at startup. Activation and deactivation also arrive as `runtime.context_changed`. | `active`, `is_manager`, `cluster_id`, `manager_count`, `worker_count` |
| `swarm.service_discovered` | New service detected | `service_id`, `name`, `mode`, `desired_replicas`, `stack_name`, `image` |
| `swarm.service_updated` | Service configuration changed, or its replica alert was raised (`replica_alert: true`) | `service_id`, `name`, `desired_replicas`, `running_replicas`, `image` (`replica_alert` instead of `image` for the alert) |
| `swarm.service_removed` | Service removed from the cluster | `service_id`, `name` |
| `swarm.node_status_changed` | Node status or availability changed | `node_id`, `hostname`, `role`, `old_status`, `new_status`, `old_availability`, `new_availability` |
| `swarm.node_updated` | A node was discovered, or its role, hostname, engine version or address changed | `node_id`, `hostname`, `role`, `status`, `availability`, `engine_version`, `address`, `task_count` |
| `swarm.task_failed` | A task failed on any node of the cluster (shutdowns and clean exits excluded) | `task_id`, `service_id`, `service_name`, `node_id`, `container_id`, `error`, `exit_code`, `timestamp` |
| `swarm.crash_loop_detected` | Crash-loop pattern detected on a service | `service_id`, `service_name`, `failure_count`, `window_minutes`, `last_error`, `timestamp` |
| `swarm.crash_loop_recovered` | Service recovered from a crash loop | `service_id`, `service_name`, `timestamp` |
| `swarm.update_progress` | Rolling update progress tick | `service_id`, `service_name`, `state`, `tasks_updated`, `tasks_total`, `old_image`, `new_image`, `message`, `timestamp` |
| `swarm.update_completed` | Rolling update finished, or was rolled back (`state` is `completed` or `rollback_completed`) | `service_id`, `service_name`, `state`, `message`, `started_at`, `completed_at` |
| `swarm.topology_changed` | A snapshot from an agent was reconciled | `agent_id` |
| `runtime.context_changed` | Swarm was switched on or off while maintenant runs | `previous`, `current`, `message`, `detected_at` |

---

## Related

- [Docker Labels Reference](../guides/docker-labels.md): full label reference
- [Container Monitoring](containers.md): standalone container monitoring
- [Alert Engine](alerts.md): channels, triggers and alert types
- [Swarm Deployment Guide](../guides/swarm.md): deployment requirements and setup
- [Multi-Host Monitoring](multihost.md): agents

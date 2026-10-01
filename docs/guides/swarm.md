# Docker Swarm Deployment Guide

How to deploy maintenant on a Docker Swarm cluster for full service discovery and cluster monitoring. Every Swarm feature is available in every edition.

---

## Requirements

### Manager Node

maintenant **must run on a Swarm manager node** to access the Swarm management API. Worker nodes cannot call `ServiceList`, `NodeList` or `TaskList`.

If deployed on a worker node, maintenant falls back to standard Docker container monitoring and logs a message explaining the limitation.

### Docker Socket Access

maintenant needs access to the Docker socket:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

On a manager the socket also gives access to the Swarm management API. maintenant only ever reads from it: it never creates, updates or removes a service, node or task.

!!! warning "`:ro` does not make the socket read-only"
    The `:ro` flag protects the socket file, not the Docker API behind it. Whatever holds the socket of a manager can manage the whole swarm. To give maintenant read access only, front the socket with a filtering proxy: see [Docker socket proxy](../security.md#recommended-docker-socket-proxy) (on a manager, also enable `SWARM`, `NODES`, `SERVICES` and `TASKS`).

### Docker Engine Version

maintenant uses the Docker SDK Swarm API, which is available in:

| Engine | Minimum Version |
|--------|----------------|
| Docker Engine | >= 19.03 |
| Docker Desktop | >= 3.0 |

The daemon must expose Docker API 1.40 or later: older engines are refused.

---

## Single-Node Swarm

The simplest setup is a single manager node running all services:

```bash
# Initialize Swarm mode
docker swarm init

# Deploy maintenant as a Swarm service
docker service create \
  --name maintenant \
  --publish published=8080,target=8080 \
  --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock,readonly \
  --mount type=bind,source=/proc,target=/host/proc,readonly \
  --mount type=bind,source=/etc/os-release,target=/host/etc/os-release,readonly \
  --mount type=volume,source=maintenant-data,target=/data \
  --env MAINTENANT_ADDR=0.0.0.0:8080 \
  --env MAINTENANT_DB=/data/maintenant.db \
  --constraint node.role==manager \
  ghcr.io/kolapsis/maintenant:latest
```

Or using a Compose file with `docker stack deploy`:

```yaml
# docker-compose.yml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    ports:
      - "8080:8080"
    read_only: true
    security_opt:
      - no-new-privileges:true
    tmpfs:
      - /tmp:noexec,nosuid,size=64m
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - /etc/os-release:/host/etc/os-release:ro
      - maintenant-data:/data
    environment:
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"
    deploy:
      placement:
        constraints:
          - node.role == manager
      replicas: 1

volumes:
  maintenant-data:
```

```bash
docker stack deploy -c docker-compose.yml maintenant
```

!!! important "Manager constraint"
    Always use `node.role == manager` as a placement constraint. This ensures maintenant runs on a manager node and has access to the Swarm management API.

!!! warning "Published ports use the routing mesh"
    A published port opens on **every node** of the swarm, and maintenant has no authentication of its own. Put an authenticating reverse proxy in front of it, or publish the port in `host` mode on the manager only. See [Security](../security.md).

!!! warning "Don't use `group_add` on Swarm"
    `docker stack deploy` **silently ignores** `group_add`, because it is not part of the Swarm
    service spec. On Swarm, the container would never join the `docker` group and the socket
    would be unreachable (`permission denied … /var/run/docker.sock`).

    You don't need it: the maintenant entrypoint **auto-detects the mounted socket's group**
    and grants the unprivileged runtime user access automatically, on Swarm and plain Compose
    alike. Just mount `/var/run/docker.sock` and you're done.

    One case needs help: a host whose socket belongs to `root:root` with no `docker` group (Synology DSM, some rootless setups). The entrypoint never grants gid 0 by itself. Set `DOCKER_GID: "0"` in `environment`, which Swarm does honour, or use a socket proxy. `DOCKER_GID` adds to the detected group, it does not replace it.

---

## Multi-Node Cluster

For multi-node Swarm clusters, the deployment is the same: maintenant runs on **one** manager node and monitors the entire cluster remotely through the Swarm API.

```
┌─────────────────────────────────────────────┐
│              Swarm Cluster                  │
│                                             │
│  ┌──────────────┐  ┌──────────────────┐    │
│  │ Manager 1    │  │ Manager 2        │    │
│  │ ★ maintenant │  │                  │    │
│  └──────────────┘  └──────────────────┘    │
│                                             │
│  ┌──────────────┐  ┌──────────────────┐    │
│  │ Worker 1     │  │ Worker 2         │    │
│  │              │  │                  │    │
│  └──────────────┘  └──────────────────┘    │
└─────────────────────────────────────────────┘
```

maintenant sees all nodes, services and tasks across the entire cluster from any single manager node. There is no need to deploy maintenant on every node.

!!! note "Single writer"
    maintenant uses SQLite and requires a single writer. Do not scale beyond 1 replica. Use `replicas: 1` in your deploy configuration.

!!! warning "A local volume stays on its node"
    The `maintenant-data` volume above is a local volume: it exists on one node only. If Swarm reschedules the service onto another manager, it starts there with an empty database. Pin the service to one manager (for example `node.hostname == manager1` in the placement constraints), or use a volume driver that follows the task.

### Containers of the other nodes

The Docker API of a node only lists its own containers, so the container views of maintenant show the tasks that run on the manager. The Services, Tasks and Nodes pages cover the whole cluster. To also get state, health, resources, logs and label-declared endpoints for the containers of the other nodes, enrol an [agent](agent-setup.md) on each of them (Personal edition or above). An agent on a manager reports the topology of the cluster as well; on a worker it reports containers only.

---

## What a Multi-Node Cluster Unlocks

Every Swarm view is available in the Community edition, with no license required.
What some of them need is not an edition but **more than one node**: a single-node
swarm has nothing to say about quorum or task placement.

| Feature | Requires |
|---------|----------|
| Node health overview | 2+ nodes |
| Node down/drain alerts | 2+ nodes |
| Quorum monitoring | 3+ manager nodes |
| Task placement tracking | 2+ nodes |
| Crash-loop detection | Any cluster size |
| Rolling update tracking | Any cluster size |
| Dedicated Swarm dashboard | Any cluster size |

The alerts are listed in [Docker Swarm Monitoring](../features/swarm.md#alerts).

### Recommended Setup

To get everything the Swarm views can show:

- **3 manager nodes** for quorum monitoring and high availability
- **1+ worker nodes** for task placement and distribution visibility
- **maintenant on a manager**, with the constraint `node.role == manager`

---

## Swarm Labels for Services

Configure maintenant with labels in your stack file. maintenant reads the service labels (`deploy.labels`) and the container labels (top-level `labels`), and the container's value wins when both set the same key.

```yaml
services:
  api:
    image: myapp:latest
    deploy:
      labels:
        # Grouping
        maintenant.group: "production"

        # Alerting
        maintenant.alert.severity: "critical"
        maintenant.alert.restart_threshold: "5"

  internal-tool:
    image: tool:latest
    deploy:
      labels:
        # Exclude from monitoring
        maintenant.ignore: "true"
```

!!! warning "Service labels vs container labels"
    Only a maintenant running on a **manager** can read service labels. A maintenant or an agent on a worker sees the container labels only. If a label must work everywhere, put it in the top-level `labels` of the service instead of `deploy.labels`.

    The exception is `maintenant.ignore` for the Swarm alerts (replicas, crash loops, rolling updates): they read the service only, so it has to be in `deploy.labels`. On a task container it applies to that container only.

Service labels are cached for 30 seconds, and changing them does not restart the tasks. The running tasks pick the new values up at the next reconciliation: when maintenant starts, when its Docker connection is re-established, or when a new container starts. The labels an agent reads are refreshed every 30 seconds. The Swarm alerts do not wait for a reconciliation: they read the labels of the service about every 30 seconds, so `maintenant.ignore` on a service silences them without recreating the tasks.

A service with several replicas has one container per replica, so an endpoint label creates one monitor per replica on the same URL.

See the [labels reference](docker-labels.md#swarm-service-labels) for every label.

---

## Stack Grouping

Services deployed with `docker stack deploy` are automatically grouped under their stack name. Docker sets the `com.docker.stack.namespace` label on all services in a stack.

```bash
# Deploy two stacks
docker stack deploy -c frontend.yml frontend
docker stack deploy -c backend.yml backend
```

In the maintenant dashboard, services appear grouped:

- **frontend**: nginx, react-app
- **backend**: api, postgres, redis

Override stack grouping per service with `maintenant.group`:

```yaml
services:
  shared-redis:
    image: redis:7
    deploy:
      labels:
        maintenant.group: "infrastructure"
        # This service appears in "infrastructure" instead of its stack name
```

---

## Image Updates

[Update tracking](../features/updates.md) covers the task containers of the manager, and those of an agent on a Swarm node. For a task, the update, rollback and CVE fix commands that maintenant shows are `docker service update --image <image> <service>`: Swarm replaces a task stopped by hand, so the service is what must get the new image. A tag that was republished is deployed by its digest (`nginx:stable@sha256:...`). The `maintenant.update.*` labels can go on the service like any other label: see [Update settings](docker-labels.md#update-settings) and [Update and Rollback Commands](../features/updates.md#update-and-rollback-commands).

---

## Graceful Degradation

maintenant handles edge cases without user intervention:

| Scenario | Behavior |
|----------|----------|
| Worker node deployment | Falls back to container monitoring, logs a message |
| Manager demotion or `docker swarm leave` at runtime | Swarm views report `active: false` and maintenant announces `runtime.context_changed`, within 60 seconds |
| Swarm enabled while maintenant runs | Swarm monitoring starts without a restart, within 60 seconds |
| Docker not reachable when maintenant starts | Swarm is detected as soon as Docker answers, without a restart |
| The swarm has no leader | The manager cannot list its nodes: `quorum_degraded` is raised, and resolved once the quorum is back |
| Docker socket unavailable, at startup or while running | Degraded mode: monitoring is suspended while maintenant retries the connection with backoff (1 to 30 seconds), `runtime.availability_changed` is broadcast and the container logs answer `503 RUNTIME_UNAVAILABLE`. Reconnection resumes Swarm monitoring |
| Swarm events missed (restart, reconnection) | Full reconciliation on startup and after each reconnection, then a snapshot every 30 seconds |
| Restart while Swarm alerts are open | The open alerts are taken over and resolved once their condition is gone |
| Node stops answering | The manager reports it `down`, which raises `node_down` |

---

## Troubleshooting

### maintenant does not show Swarm services

1. Verify maintenant is running on a **manager node**:
    ```bash
    docker node ls
    # The node running maintenant must show "Leader" or "Reachable"
    ```

2. Check that the Docker socket is mounted:
    ```bash
    docker service inspect maintenant --pretty
    # Look for the /var/run/docker.sock mount
    ```

3. Check maintenant logs for the Swarm detection message:
    ```bash
    docker service logs maintenant 2>&1 | grep -i swarm
    ```
    A line containing `detected Swarm mode (worker node)` means maintenant runs on a worker. If there is no Swarm line at all, the node is not in Swarm mode, or Docker does not answer yet: Swarm is detected as soon as Docker answers, and again every 60 seconds, so no restart is needed.

### Node health not showing

Node health needs a swarm manager to query, not a license. The views are open
in every edition. Check what maintenant is connected to:

- maintenant must run **on a manager node**, since workers cannot list nodes
- Constrain the service with `node.role == manager`
- Check the logs for a Swarm detection message

### Services show 0/N running

This is expected when all tasks are pending, failed, or shutting down. Check the service detail view for individual task states and error messages.

### A label on a service has no effect

- Labels in `deploy.labels` are read only on a manager. On a worker, put them in the top-level `labels`.
- Service labels are cached for 30 seconds, and running tasks take new values only at the next reconciliation (see [Swarm Labels for Services](#swarm-labels-for-services)). Recreating the tasks (for example `docker service update --force`) applies them to the new containers.
- `maintenant.alert.channels` no longer exists: alert routing is configured with channels and triggers.

### A crash-looping service raises no `crash_loop` alert

Crash loops are counted from the tasks that Swarm marks `failed`, on every node of the cluster, read from the task list every 30 seconds. The alert needs 3 failed tasks of the same service within 5 minutes. Not counted:

- tasks that Swarm shuts down itself (rolling update, scale down, `docker service update --force`) and containers stopped with SIGTERM (exit code 143);
- tasks that never start (state `rejected`, for example an image that cannot be pulled);
- tasks of a service labelled `maintenant.ignore` in `deploy.labels`.

A container killed with SIGKILL (exit code 137) does count.

---

## Related

- [Docker Swarm Monitoring](../features/swarm.md): feature overview, alerts and API reference
- [Docker Labels Reference](docker-labels.md): full label reference
- [Agent Setup](agent-setup.md): monitor the containers of the other nodes
- [Installation](../getting-started/installation.md): general installation guide

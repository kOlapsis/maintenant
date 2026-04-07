# Docker Swarm Deployment Guide

How to deploy maintenant on a Docker Swarm cluster for full service discovery and cluster monitoring.

---

## Requirements

### Manager Node

maintenant **must run on a Swarm manager node** to access the Swarm management API. Worker nodes do not have access to `ServiceList`, `NodeList`, or `TaskList` endpoints.

If deployed on a worker node, maintenant falls back to standard Docker container monitoring and logs a message explaining the limitation.

### Docker Socket Access

maintenant needs read-only access to the Docker socket:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

The Docker socket provides access to the Swarm management API on manager nodes. maintenant never modifies services, nodes, or tasks — it is strictly read-only.

### Docker Engine Version

maintenant uses the Docker SDK Swarm API, which is available in:

| Engine | Minimum Version |
|--------|----------------|
| Docker Engine | >= 19.03 |
| Docker Desktop | >= 3.0 |

Any Docker Engine version that supports Swarm mode is compatible. The Swarm API has been stable since Docker 1.12, but Docker 19.03+ is recommended for security and feature completeness.

---

## Single-Node Swarm

The simplest setup — a single manager node running all services:

```bash
# Initialize Swarm mode
docker swarm init

# Deploy maintenant as a Swarm service
docker service create \
  --name maintenant \
  --publish published=8080,target=8080 \
  --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock,readonly \
  --mount type=bind,source=/proc,target=/host/proc,readonly \
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
    group_add:
      - "${DOCKER_GID:-983}"
    tmpfs:
      - /tmp:noexec,nosuid,size=64m
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
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

---

## Multi-Node Cluster

For multi-node Swarm clusters, the deployment is the same — maintenant runs on **one** manager node and monitors the entire cluster remotely via the Swarm API.

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

maintenant sees all nodes, services, and tasks across the entire cluster from any single manager node. There is no need to deploy maintenant on every node.

!!! note "Single writer"
    maintenant uses SQLite and requires a single writer. Do not scale beyond 1 replica. Use `replicas: 1` in your deploy configuration.

---

## Enterprise Features

Enterprise features require a multi-node cluster to be meaningful:

| Feature | Requires |
|---------|----------|
| Node health overview | 2+ nodes |
| Node down/drain alerts | 2+ nodes |
| Quorum monitoring | 3+ manager nodes |
| Task placement tracking | 2+ nodes |
| Crash-loop detection | Any cluster size |
| Rolling update tracking | Any cluster size |
| Dedicated Swarm dashboard | Any cluster size |

### Recommended Setup for Enterprise

For the full Enterprise experience:

- **3 manager nodes** — For quorum monitoring and high availability
- **1+ worker nodes** — For task placement and distribution visibility
- **maintenant on a manager** — Constraint to `node.role == manager`
- **Enterprise license** — Activate via the settings page

---

## Swarm Labels for Services

Configure maintenant behavior using labels in the `deploy` section of your Compose file:

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
        maintenant.alert.channels: "ops-webhook,email"

  internal-tool:
    image: tool:latest
    deploy:
      labels:
        # Exclude from monitoring
        maintenant.ignore: "true"
```

!!! warning "Service labels vs container labels"
    Swarm services have two label scopes. maintenant reads **service-level labels** (`deploy.labels`), not container-level labels (top-level `labels`). This is because service labels are accessible via the Swarm API without inspecting individual containers.

---

## Stack Grouping

Services deployed via `docker stack deploy` are automatically grouped by their stack name. Docker sets the `com.docker.stack.namespace` label on all services in a stack.

```bash
# Deploy two stacks
docker stack deploy -c frontend.yml frontend
docker stack deploy -c backend.yml backend
```

In the maintenant dashboard, services appear grouped:

- **frontend** — nginx, react-app
- **backend** — api, postgres, redis

Override stack grouping per-service with `maintenant.group`:

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

## Graceful Degradation

maintenant handles edge cases without user intervention:

| Scenario | Behavior |
|----------|----------|
| Worker node deployment | Falls back to container monitoring, logs a message |
| Manager demotion at runtime | Degrades to container monitoring, broadcasts status update |
| Docker socket unavailable | Standard connection retry behavior |
| Swarm events missed (restart) | Full reconciliation on startup |
| Network partition (nodes unreachable) | Shows last-known state with "last seen" timestamp |

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

### Node health not showing (Enterprise)

Node health monitoring is an Enterprise feature. Verify your license:

- Navigate to the settings page
- Check that the edition shows "Enterprise"
- The `/swarm` dashboard route requires Enterprise

### Services show 0/N running

This is expected when all tasks are pending, failed, or shutting down. Check the service detail view for individual task states and error messages.

---

## Related

- [Docker Swarm Monitoring](../features/swarm.md) — Feature overview and API reference
- [Docker Labels Reference](docker-labels.md) — Full label reference
- [Installation](../getting-started/installation.md) — General installation guide

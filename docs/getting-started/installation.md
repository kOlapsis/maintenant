# Installation

maintenant ships as a single binary with the frontend embedded. No external dependencies required — just deploy and go.

---

## Docker Compose (Recommended)

The fastest way to get started. Create a `docker-compose.yml`:

```yaml
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
      - maintenant-data:/data
    environment:
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"
    restart: unless-stopped

volumes:
  maintenant-data:
```

```bash
docker compose up -d
```

Open **http://localhost:8080**. maintenant auto-discovers all your containers immediately.

!!! note "Docker socket access"
    The entrypoint reads the mounted socket's group and grants the unprivileged user access
    automatically — no `group_add` required, on Compose and Swarm alike. Set `DOCKER_GID` only
    to pin a specific GID (non-standard socket path or socket proxy). See
    [Troubleshooting](../troubleshooting.md#permission-denied-on-varrundockersock) if access fails.

!!! note "Why `0.0.0.0`? And the security finding it triggers"
    `MAINTENANT_ADDR: "0.0.0.0:8080"` is the bind address **inside the container**. It must be
    `0.0.0.0` there — Docker's port mapping reaches the process through the container's bridge
    interface, so binding `127.0.0.1` inside the container would make the UI unreachable. This
    setting alone exposes nothing to your network.

    What decides the actual exposure is the `ports:` mapping. `"8080:8080"` publishes the port on
    **all** host interfaces — and maintenant's own security scanner, which does not exempt
    maintenant's container, will report a critical **"Port exposed on all interfaces"** finding
    for it. That finding is expected, not a bug. To avoid it, publish the port on a specific
    interface instead:

    ```yaml
    ports:
      - "127.0.0.1:8080:8080"     # local only — pair with a reverse proxy
      # or, for direct access from your LAN on a headless server:
      # - "192.168.1.50:8080:8080"  # the server's LAN IP
    ```

    See [Configuration → Choosing a Bind Address](configuration.md#choosing-a-bind-address) for
    the full decision guide, including how to acknowledge the finding when the exposure is
    intentional.

!!! tip "Run without mounting the Docker socket"
    Mounting `docker.sock` — even `:ro` — hands the container root-equivalent access to the
    host; the `:ro` flag does not block Docker API writes. maintenant's API usage is entirely
    read-only, and its client honours `DOCKER_HOST`, so it runs cleanly behind a
    [docker-socket-proxy](../security.md#recommended-docker-socket-proxy) that rejects every
    write with `403`. Recommended for production.

!!! tip "Deploying on a cloud provider"
    A ready-to-use `cloud-init` file provisions any Ubuntu cloud server with Docker and
    maintenant on first boot, with the dashboard bound to loopback. Firewall rules, block volumes,
    private-network agents, load balancer checks and managed Kubernetes are per-provider:

    - [Hetzner Cloud](../guides/hetzner.md)
    - [DigitalOcean](../guides/digitalocean.md)
    - [Scaleway](../guides/scaleway.md)
    - [OVHcloud](../guides/ovhcloud.md)
    - [Vultr](../guides/vultr.md)

!!! tip "Production deployment"
    For production, place maintenant behind a reverse proxy with authentication.
    See the [Configuration](configuration.md) page for a Traefik + Authelia example.

!!! note "Watching a fleet?"
    The server holds what the agents cannot rebuild: their identity and their
    enrolment. On the default local file, losing that machine means re-enrolling
    every host by hand. If you already operate a PostgreSQL, point the server at
    it and the instance becomes replaceable — see
    [PostgreSQL storage](../guides/postgresql.md). An existing install moves
    over with a one-shot service you add next to the instance:

    ```bash
    docker compose stop maintenant
    docker compose run --rm copy-store
    ```

---

## Kubernetes

### Helm (recommended)

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace
```

### Raw manifests

```bash
kubectl create namespace maintenant
kubectl apply -f deploy/kubernetes/
```

Both approaches deploy:

- A **ServiceAccount** with read-only RBAC (pods, logs, services, namespaces, events, deployments, statefulsets, daemonsets, replicasets, pod metrics)
- A **Deployment** with security hardening (non-root, read-only filesystem, all capabilities dropped)
- A **PersistentVolumeClaim** (10Gi) for the SQLite database
- A **ClusterIP Service** on port 80

maintenant auto-detects the in-cluster Kubernetes API. Namespace filtering and workload-level monitoring work out of the box.

!!! note
    The deployment uses `strategy: Recreate` because SQLite requires a single writer.
    Do not scale beyond 1 replica.

For detailed Kubernetes configuration and all Helm values, see the [Kubernetes Guide](../guides/kubernetes.md).

---

## Building from Source

### Requirements

| Tool | Minimum Version |
|------|----------------|
| Go | >= 1.25 |
| Node.js | >= 20 |
| CGO | Enabled |
| Docker | For testing |

### Build Steps

```bash
# Clone the repository
git clone https://github.com/kolapsis/maintenant.git
cd maintenant

# Build the frontend
cd frontend
npm install
npm run build-only
cd ..

# Copy frontend assets into the embed directory
cp -r frontend/dist cmd/maintenant/web/dist/

# Build the Go binary
CGO_ENABLED=1 go build -o maintenant ./cmd/maintenant

# Run
./maintenant
```

The resulting `maintenant` binary includes the entire frontend (embedded via Go's `embed.FS`). There is nothing else to deploy.

### Build with Version Info

```bash
CGO_ENABLED=1 go build \
  -ldflags="-s -w \
    -X main.version=$(git describe --tags --always) \
    -X main.commit=$(git rev-parse --short HEAD) \
    -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o maintenant ./cmd/maintenant
```

---

## Docker Image

The official Docker image uses a multi-stage build:

1. **Stage 1** — Node.js 22 builds the Vue 3 SPA
2. **Stage 2** — Go 1.25 compiles the binary with the frontend embedded
3. **Stage 3** — Alpine 3.21 minimal runtime (runs as `nobody:nobody` uid 65534, read-only binary, health check included)

The image is available at `ghcr.io/kolapsis/maintenant`.

---

## Verifying the Installation

Once maintenant is running, verify it with the health endpoint:

```bash
curl http://localhost:8080/api/v1/health
```

Expected response:

```json
{"status": "ok", "version": "v0.1.0"}
```

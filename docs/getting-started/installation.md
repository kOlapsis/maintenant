# Installation

maintenant ships as a single binary with the frontend embedded. No external dependencies required: just deploy and go.

Two ways in: a container, covered below, or the binary itself as a systemd
service on a host with no container runtime, see **[Native Linux Install](../install.md)**.

```bash
curl -fsSL https://install.maintenant.dev | sudo bash
```

!!! warning "No built-in authentication"
    maintenant has no login of its own. Whoever can reach its port can read everything and change monitors and settings. Keep the port on a trusted network, or put a reverse proxy with authentication in front, see [Security](../security.md).

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
      - /etc/os-release:/host/etc/os-release:ro
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

The mounts:

| Mount | What it is for |
|-------|----------------|
| `/var/run/docker.sock` | Container discovery and events. Optional: without it, endpoint, certificate and heartbeat monitors still run. |
| `/proc:/host/proc:ro` | Host CPU and memory, read from `/host/proc/stat` and `/host/proc/meminfo`. Without the mount, maintenant reads the `/proc` of the container itself. |
| `/etc/os-release:/host/etc/os-release:ro` | The host's operating system, for [Host OS End-of-Support](../features/host-os.md). Inside a container maintenant never reads the image's own `/etc/os-release`, which describes the image and not the host, so without this mount the host OS shows as unknown. |
| `maintenant-data:/data` | The database, the license cache and the telemetry identity. |

!!! note "Docker socket access"
    When the container starts as root (the default), the entrypoint reads the group of the mounted socket (`/var/run/docker.sock` or `/run/docker.sock`) and grants it to the unprivileged runtime user, so no `group_add` is needed, on Compose and Swarm alike. `DOCKER_GID` adds a group on top of the detected one. Set it when the socket is mounted at another path, or to `0` on a host whose socket is owned by `root:root` with no `docker` group (Synology DSM). If the container starts as a non-root user (`user:` in Compose), nothing is detected: add the socket's group with `group_add`. See [Troubleshooting](../troubleshooting.md#permission-denied-on-varrundockersock) if access fails.

!!! note "Why `0.0.0.0`? And the security finding it triggers"
    `MAINTENANT_ADDR: "0.0.0.0:8080"` is the bind address **inside the container**. It must be
    `0.0.0.0` there: Docker's port mapping reaches the process through the container's bridge
    interface, and the image default, `127.0.0.1:8080`, would leave the UI unreachable (the
    container would still report healthy). This setting alone exposes nothing to your network.

    What decides the actual exposure is the `ports:` mapping. `"8080:8080"` publishes the port on
    **all** host interfaces, and maintenant's own security scanner, which does not exempt
    maintenant's container, will report a critical **"Port exposed on all interfaces"** finding
    for it. That finding is expected, not a bug. To avoid it, publish the port on a specific
    interface instead:

    ```yaml
    ports:
      - "127.0.0.1:8080:8080"     # local only, pair with a reverse proxy
      # or, for direct access from your LAN on a headless server:
      # - "192.168.1.50:8080:8080"  # the server's LAN IP
    ```

    See [Configuration → Choosing a Bind Address](configuration.md#choosing-a-bind-address) for
    the full decision guide, including how to acknowledge the finding when the exposure is
    intentional.

!!! tip "Run without mounting the Docker socket"
    Mounting `docker.sock`, even `:ro`, hands the container root-equivalent access to the
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
    See the [Security Guide](../security.md#reverse-proxy-setup) for Traefik + Authelia, Caddy and nginx examples.

!!! note "Watching a fleet?"
    The server holds what the agents cannot rebuild: their identity and their
    enrolment. On the default local file, losing that machine means re-enrolling
    every host by hand. If you already operate a PostgreSQL, point the server at
    it and the instance becomes replaceable, see
    [PostgreSQL storage](../guides/postgresql.md). An existing install moves
    over with a one-shot `copy-store` service you add next to the instance
    (its definition is in that guide):

    ```bash
    docker compose stop maintenant
    docker compose run --rm copy-store
    ```

---

## Kubernetes

The manifests and the chart live under `deploy/`: run the commands below from a clone of the repository.

### Helm (recommended)

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace
```

The chart installs into the release namespace. Its `image.tag` defaults to `latest`: pin a release with `--set image.tag=1.8.0`.

### Raw manifests

```bash
kubectl create namespace maintenant
kubectl apply -f deploy/kubernetes/
```

The raw manifests are fixed to the `maintenant` namespace, in `deployment.yaml` and in the `ClusterRoleBinding` subject of `rbac.yaml`. To use another namespace, change all of them together.

Both approaches deploy:

- A **ServiceAccount** and a read-only **ClusterRole**: `get`, `list` and `watch` on namespaces, nodes, pods, events and services; `get` on pod logs; `get`, `list` and `watch` on deployments, statefulsets and daemonsets (`apps`) and on jobs (`batch`); `get` and `list` on pod and node metrics (`metrics.k8s.io`, which needs metrics-server).
- A **Deployment** with security hardening: non-root (uid and gid 65534, `fsGroup` 65534), read-only root filesystem, all capabilities dropped, no privilege escalation, and an `emptyDir` on `/tmp`. The probes call `/api/v1/health`, with a startup probe that allows 10 minutes for schema migrations.
- A **PersistentVolumeClaim** (10Gi) for the SQLite database, the license cache and the telemetry identity.
- A **ClusterIP Service** on port 80.

maintenant auto-detects the in-cluster Kubernetes API. It monitors every namespace except `kube-system`, `kube-public` and `kube-node-lease` unless you set an allowlist or blocklist, and workload-level monitoring works out of the box. The pod starts as a non-root user, so the image entrypoint runs the binary directly and leaves the volume permissions to `fsGroup`.

!!! note
    The deployment uses `strategy: Recreate` because SQLite requires a single writer.
    Do not scale beyond 1 replica.

For detailed Kubernetes configuration and all Helm values, see the [Kubernetes Guide](../guides/kubernetes.md).

---

## Building from Source

### Requirements

| Tool | Minimum Version |
|------|----------------|
| Go | >= 1.26.6 |
| Node.js | `^20.19.0` or `>= 22.12.0` |
| CGO | Enabled, with a C compiler (SQLite) |
| Docker | For testing |

### Build Steps

```bash
# Clone the repository
git clone https://github.com/kolapsis/maintenant.git
cd maintenant

# Build the frontend
cd frontend
npm ci
npm run build-only
cd ..

# Copy frontend assets into the embed directory
rm -rf cmd/maintenant/web/dist/*
cp -r frontend/dist/. cmd/maintenant/web/dist/

# Build the Go binary
CGO_ENABLED=1 go build -o maintenant ./cmd/maintenant

# Run
./maintenant
```

`make build` does the same, type-checks the frontend first and writes `./bin/maintenant`.

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

!!! note "Licensing a source build"
    Official binaries and images embed the license public key through `-X main.publicKeyB64=…`. A binary built without it cannot verify a `MAINTENANT_LICENSE_KEY`: it logs `license manager initialization failed, running as Community Edition` and stays on Community.

---

## Docker Image

The official Docker image uses a multi-stage build:

1. **Stage 1**: Node.js 22 builds the Vue 3 SPA
2. **Stage 2**: Go 1.26 compiles the binary with the frontend embedded
3. **Stage 3**: Alpine 3.21 minimal runtime (read-only binary, health check included)

The image is available at `ghcr.io/kolapsis/maintenant`, for `linux/amd64` and `linux/arm64`.

### Tags

| Tag | Published | Use |
|-----|-----------|-----|
| `latest` | Every release | The newest release. |
| `1.8.0` | Every release | One exact version, written without the `v` of the Git tag. |
| `1.8` | Every release | The newest patch of a minor version. |
| `1` | Every release | The newest minor and patch of a major version. |
| `main` | Every push to `main` | A development build. |
| `<short sha>` | Every push to `main` | The build of one commit. |
| `demo` | Every non-prerelease release | The read-only public demo build. Not for production use. |

Pin `X.Y.Z` when you want upgrades to happen on your schedule. The release images (`latest` and the version tags) are signed and come with a provenance attestation and an SBOM; the verification commands are in [SECURITY.md](https://github.com/kOlapsis/maintenant/blob/main/SECURITY.md). The `main`, sha and `demo` tags are not signed.

### Entrypoint and user

The image runs `docker-entrypoint.sh`, then `/app/maintenant`. A first argument that starts with `-` is taken as a flag for `maintenant`, so `command: ["--logLevel", "debug"]` works.

- **Started as root (the default)**: the entrypoint creates the data directory if needed and hands it, without recursing and never `/`, to uid 65534. That is the directory of `MAINTENANT_DB` and `/data/shm` for a server, and `MAINTENANT_DATA_DIR` for an agent. The mode, the database path and the data directory are read from `MAINTENANT_MODE`, `MAINTENANT_DB`, `MAINTENANT_DATA_DIR` or the matching `--mode`, `--db` and `--data-dir` flags. It also adds the Docker socket's group, then drops to uid and gid 65534 (`nobody`) with `setpriv`. This is why named volumes and fresh bind mounts, which arrive owned by root, work with no manual `chown`.
- **Started as a non-root user** (`runAsUser` in Kubernetes, `user:` in Compose): the entrypoint runs the binary directly. It changes no ownership and detects no group, so the data volume must already be writable by that user (`fsGroup` in Kubernetes) and the Docker socket's group must be added by the orchestrator (`group_add`).

The image health check runs `/app/maintenant healthcheck` every 30 seconds (5 seconds timeout, 30 seconds start period, 3 retries). For a server it calls `/api/v1/health` on `MAINTENANT_ADDR` (a wildcard address is probed through `127.0.0.1`) and expects a 200. For an agent, which has no port, it checks that the liveness file in `MAINTENANT_DATA_DIR` was refreshed in the last 60 seconds. Schema migrations run before the listener opens, so a very large database can report `unhealthy` until they finish.

---

## Verifying the Installation

Once maintenant is running, verify it with the health endpoint:

```bash
curl http://localhost:8080/api/v1/health
```

Expected response:

```json
{"status":"ok","version":"1.8.0","runtime":{"name":"docker","connected":true},"storage":{"engine":"sqlite","connected":true,"peers":0}}
```

`version` is the running version. `runtime.name` is `docker` or `kubernetes`, and `runtime.connected` is `false` while the instance runs in degraded mode without a reachable runtime. `storage.engine` is `sqlite` or `postgres`, and `storage.peers` counts the other instances working on the same database. The endpoint answers 200 even when the database is momentarily unreachable, so it can back a liveness probe: read the outage from `storage.connected`.

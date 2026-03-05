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
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - maintenant-data:/data
    environment:
      MAINTENANT_DB: "/data/maintenant.db"
    restart: unless-stopped

volumes:
  maintenant-data:
```

```bash
docker compose up -d
```

Open **http://localhost:8080**. maintenant auto-discovers all your containers immediately.

!!! tip "Production deployment"
    For production, place maintenant behind a reverse proxy with authentication.
    See the [Configuration](configuration.md) page for a Traefik + Authelia example.

---

## Kubernetes

Apply the provided manifests from the `deploy/kubernetes/` directory:

```bash
kubectl create namespace maintenant
kubectl apply -f deploy/kubernetes/
```

This deploys:

- A **ServiceAccount** with read-only RBAC (pods, logs, services, namespaces, events, deployments, statefulsets, daemonsets, replicasets, pod metrics)
- A **Deployment** with security hardening (non-root, read-only filesystem, all capabilities dropped)
- A **PersistentVolumeClaim** (1Gi) for the SQLite database
- A **ClusterIP Service** on port 80

maintenant auto-detects the in-cluster Kubernetes API. Namespace filtering and workload-level monitoring work out of the box.

!!! note
    The deployment uses `strategy: Recreate` because SQLite requires a single writer.
    Do not scale beyond 1 replica.

For detailed Kubernetes configuration, see the [Kubernetes Guide](../guides/kubernetes.md).

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
3. **Stage 3** — Alpine 3.21 minimal runtime (non-root user, health check included)

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

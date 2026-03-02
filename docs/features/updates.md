# Update Intelligence

Know when your container images have updates available. maintenant scans OCI registries and compares digests. Stop running `docker pull` blindly.

---

## How It Works

maintenant periodically scans the OCI registry for each monitored container image:

1. **Digest comparison** — Compares the local image digest with the latest available in the registry

---

## Scan Interval

The scan interval is configured via the `MAINTENANT_UPDATE_INTERVAL` environment variable:

```bash
MAINTENANT_UPDATE_INTERVAL=24h  # Default: check once per day
```

Accepts Go duration format: `12h`, `6h`, `30m`, etc.

You can also trigger a manual scan at any time:

```bash
POST /api/v1/updates/scan
```

---

## OCI Registry Scanning

maintenant queries the OCI (Docker) registry API to compare image digests:

- **Docker Hub** — Public and private repositories
- **GitHub Container Registry (GHCR)** — `ghcr.io` images
- **Self-hosted registries** — Any OCI-compliant registry

When a new digest is available for an image tag, maintenant flags it as having an update available.

---

## Version Pinning

Pin a container to its current version to suppress update notifications:

```bash
# Pin current version
POST /api/v1/updates/pin/{container_id}

# Unpin
DELETE /api/v1/updates/pin/{container_id}
```

---

## Update Exclusions

Exclude specific images from update scanning:

```bash
# Create exclusion
POST /api/v1/updates/exclusions
{
  "image": "myregistry.example.com/internal-app"
}

# List exclusions
GET /api/v1/updates/exclusions

# Remove exclusion
DELETE /api/v1/updates/exclusions/{id}
```

---

## Alert Events

| Event | Description | Default Severity |
|-------|-------------|------------------|
| `available` | A new image version is available | Info |

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/updates` | List all available updates |
| `GET` | `/api/v1/updates/summary` | Update summary with counts |
| `POST` | `/api/v1/updates/scan` | Trigger a manual scan |
| `GET` | `/api/v1/updates/scan/{scan_id}` | Get scan status |
| `GET` | `/api/v1/updates/container/{container_id}` | Get update info for a container |
| `GET` | `/api/v1/updates/dry-run` | Preview what a scan would check |
| `POST` | `/api/v1/updates/pin/{container_id}` | Pin current version |
| `DELETE` | `/api/v1/updates/pin/{container_id}` | Unpin version |
| `GET` | `/api/v1/updates/exclusions` | List exclusions |
| `POST` | `/api/v1/updates/exclusions` | Create exclusion |
| `DELETE` | `/api/v1/updates/exclusions/{id}` | Delete exclusion |

---

## Related

- [Container Monitoring](containers.md) — Container states and image info
- [Alert Engine](alerts.md) — Update alerts

# Update Intelligence

Know when your container images have updates available. maintenant scans OCI registries and compares digests. Stop running `docker pull` blindly.

![Update Intelligence](../screen-captures/5-updates.png)

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

## Tag Filtering

### Default Behavior

By default, maintenant determines update candidates from the OCI registry tag list using two strategies:

- **Semver mode** — For version tags (e.g. `1.24`, `3.19.1-alpine`), maintenant compares semver versions and respects variant suffixes (e.g. `-alpine`, `-bookworm`). A container running `nginx:1.24-alpine` will only be compared against other `-alpine` tags.
- **Digest-only mode** — For non-semver channel tags (`latest`, `lts`, `stable`, etc.), maintenant compares the remote image digest against a stored baseline. If the digest changes (the image was rebuilt), an update is reported.

### Tag Filter Labels

Two Docker labels let you override the default update candidate selection:

| Label | Type | Description |
|-------|------|-------------|
| `maintenant.update.tag-include` | Go regex | Only tags matching this pattern are considered as update candidates |
| `maintenant.update.tag-exclude` | Go regex | Tags matching this pattern are excluded from update candidates |

Patterns use Go [`regexp`](https://pkg.go.dev/regexp/syntax) syntax. Without anchors (`^`, `$`), the pattern matches anywhere in the tag string — add them for exact matches.

### Priority Rules

1. **`tag-include` replaces the automatic variant filter** — when set, only matching tags are candidates; the `-alpine`/`-bookworm` variant detection is bypassed.
2. **`tag-exclude` alone preserves the variant filter** — the automatic variant suffix matching still applies; matching tags are removed after.
3. **Exclude applies after include — exclude always wins** — when both labels are set, include filters first, then exclude removes from the result.
4. **Invalid regex → warning + label ignored** — a malformed pattern is logged as a warning and treated as absent; default behavior applies.
5. **Empty string → treated as absent** — an empty label value has no effect.

### Digest-Only Mode

Both `tag-include` and `tag-exclude` are **ignored** for containers using non-semver channel tags (`latest`, `lts`, `stable`, etc.). Digest-only mode compares remote digests directly and does not use a tag list, so filtering has no effect.

### Concrete Examples

**Stay on Node 20 alpine only — never jump to Node 21:**

```yaml
services:
  app:
    image: node:20.12.0-alpine
    labels:
      maintenant.update.tag-include: "^20\\.\\d+\\.\\d+-alpine$$"
```

**Exclude pre-release tags:**

```yaml
services:
  redis:
    image: redis:7.0.0
    labels:
      maintenant.update.tag-exclude: "(rc|beta|alpha)"
```

**Combine include and exclude — Node 20, no pre-releases:**

```yaml
services:
  app:
    image: node:20.0.0
    labels:
      maintenant.update.tag-include: "^20\\."
      maintenant.update.tag-exclude: "(rc|beta|alpha)"
```

**Pin to a major version:**

```yaml
services:
  postgres:
    image: postgres:15.1
    labels:
      maintenant.update.tag-include: "^15\\."
```

**Only stable semver tags (no channel tags that happen to match):**

```yaml
services:
  traefik:
    image: traefik:2.11.0
    labels:
      maintenant.update.tag-include: "^v?[0-9]+\\.[0-9]+\\.[0-9]+$$"
```

**Only slim-bookworm variants:**

```yaml
services:
  python:
    image: python:3.11-slim-bookworm
    labels:
      maintenant.update.tag-include: ".*-slim-bookworm$$"
```

### Troubleshooting

**No update shown after adding `tag-include`:**
Test your regex against the actual tag list in the registry. The pattern must match at least one tag that is newer than the current tag. Use a tool like [regex101.com](https://regex101.com) with Go flavor.

**Filter seems to have no effect:**
Check if the container uses a non-semver channel tag (`latest`, `lts`, `stable`). Digest-only mode containers bypass tag filters.

**Warning in logs: `invalid maintenant.update.tag-include regex, label ignored`:**
The regex pattern has a syntax error. Check for unbalanced brackets or other Go `regexp` syntax issues.

---

## Compose-Aware Commands

When maintenant detects that a container belongs to a Docker Compose project, update and rollback commands automatically include the correct `--project-directory` flag. This ensures commands work reliably even when the compose file lives outside the current working directory.

The compose working directory is extracted from the `com.docker.compose.project.working_dir` label that Docker Compose sets automatically on every container.

---

## CVE Enrichment & Risk Scoring :material-crown:{ title="Pro" }
With maintenant Pro, update intelligence goes beyond digest comparison. Each available update is enriched with vulnerability data:

- **CVE details** — Known vulnerabilities affecting the current and target versions
- **Risk scoring** — Severity-weighted score to prioritize which updates matter most
- **Changelog** — Docker image changelog between current and available versions

```
GET /api/v1/risk
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
- [Docker Labels Reference](../guides/docker-labels.md#update-settings) — Full reference for `maintenant.update.*` labels

# Docker Labels Reference

maintenant uses Docker labels to configure monitoring directly on your containers. No config files, no UI clicks — just add labels to your `docker-compose.yml` and maintenant picks them up automatically.

---

## Container Settings

| Label | Values | Description |
|-------|--------|-------------|
| `maintenant.ignore` | `true` | Exclude this container from monitoring |
| `maintenant.group` | any string | Custom group name (overrides Compose project) |
| `maintenant.alert.severity` | `critical`, `warning`, `info` | Default alert severity for this container |
| `maintenant.alert.restart_threshold` | integer | Number of restarts before triggering a restart loop alert |
| `maintenant.alert.channels` | comma-separated | Route alerts to specific notification channels |

```yaml
labels:
  maintenant.ignore: "true"                    # Exclude from monitoring
  maintenant.group: "backend"                  # Custom group name
  maintenant.alert.severity: "critical"        # Default severity
  maintenant.alert.restart_threshold: "5"      # Restart loop threshold
  maintenant.alert.channels: "slack,email"     # Route to channels
```

---

## Endpoint Monitoring

### Simple — One Endpoint per Container

| Label | Default | Description |
|-------|---------|-------------|
| `maintenant.endpoint.http` | — | HTTP(S) URL to check |
| `maintenant.endpoint.tcp` | — | TCP host:port to check |
| `maintenant.endpoint.interval` | `30s` | Check interval (Go duration) |
| `maintenant.endpoint.timeout` | `10s` | Request timeout |
| `maintenant.endpoint.failure-threshold` | `1` | Consecutive failures before marking as down |
| `maintenant.endpoint.recovery-threshold` | `1` | Consecutive successes before marking as up |

#### HTTP-Specific Options

| Label | Default | Description |
|-------|---------|-------------|
| `maintenant.endpoint.http.method` | `GET` | HTTP method (`GET`, `HEAD`, `POST`, `PUT`, `DELETE`, `PATCH`, `OPTIONS`) |
| `maintenant.endpoint.http.expected-status` | `200` | Expected status codes (comma-separated, e.g., `200,201`) |
| `maintenant.endpoint.http.tls-verify` | `true` | Verify TLS certificates (`false` for self-signed) |
| `maintenant.endpoint.http.headers` | — | Custom headers (JSON `{"K":"V"}` or `K=V,K=V` format) |
| `maintenant.endpoint.http.max-redirects` | — | Maximum number of HTTP redirects to follow |

```yaml
labels:
  maintenant.endpoint.http: "https://api:8443/health"
  maintenant.endpoint.interval: "15s"
  maintenant.endpoint.failure-threshold: "3"
  maintenant.endpoint.http.method: "POST"
  maintenant.endpoint.http.expected-status: "200,201"
  maintenant.endpoint.http.tls-verify: "false"
```

### Indexed — Multiple Endpoints per Container

Use a numeric index between `endpoint` and the type to define multiple endpoints:

```yaml
labels:
  # First endpoint — HTTP health check
  maintenant.endpoint.0.http: "https://app:8443/health"
  maintenant.endpoint.0.interval: "15s"
  maintenant.endpoint.0.failure-threshold: "3"

  # Second endpoint — Redis TCP check
  maintenant.endpoint.1.tcp: "redis:6379"
  maintenant.endpoint.1.interval: "30s"
```

!!! warning "Do not mix simple and indexed"
    Use either simple labels (`maintenant.endpoint.http`) or indexed labels
    (`maintenant.endpoint.0.http`). Do not mix both styles on the same container.

Global config labels (without an index) apply as defaults to all indexed endpoints. Indexed config overrides global config.

---

## Certificate Monitoring

| Label | Description |
|-------|-------------|
| `maintenant.tls.certificates` | Comma-separated list of hostnames to monitor |

Hostnames without a port default to port 443. Schemes (`https://`) and paths are stripped automatically.

```yaml
labels:
  # Monitor three certificates
  maintenant.tls.certificates: "api.example.com,dashboard.example.com:8443,mail.example.com"
```

!!! info "Automatic detection"
    HTTPS endpoints configured via `maintenant.endpoint.http` automatically get
    certificate monitoring. Use `maintenant.tls.certificates` for additional
    domains not covered by endpoint checks.

---

## Update Settings

Control how maintenant tracks image updates for each container.

| Label | Type | Default | Description |
|-------|------|---------|-------------|
| `maintenant.update.enabled` | `true` / `false` | `true` | Enable or disable update tracking for this container |
| `maintenant.update.track` | `patch`, `minor`, `major` | — | Minimum update type to report (`patch` = any update, `major` = major only) |
| `maintenant.update.pin` | tag string | — | Pin updates to this tag string; no updates will be reported |
| `maintenant.update.ignore_major` | `true` / `false` | `false` | Suppress major version updates |
| `maintenant.update.registry` | registry URL | — | Override the registry used for scanning (e.g. `ghcr.io`) |
| `maintenant.update.alert_on` | `available`, `critical` | — | Alert only on specific update types |
| `maintenant.update.digest_only` | `true` / `false` | `false` | Force digest-only comparison (bypasses semver detection) |
| `maintenant.update.tag-include` | Go regex | — | Only tags matching this pattern are update candidates |
| `maintenant.update.tag-exclude` | Go regex | — | Tags matching this pattern are excluded from candidates |

```yaml
labels:
  maintenant.update.enabled: "true"
  maintenant.update.tag-include: "^20\\.\\d+\\.\\d+-alpine$$"  # Node 20 alpine only
  maintenant.update.tag-exclude: "(rc|beta|alpha)"              # No pre-releases
```

See [Tag Filtering](../features/updates.md#tag-filtering) in the Update Intelligence guide for full details, examples, and troubleshooting.

---

## Swarm Service Labels

When running in Docker Swarm mode, maintenant reads labels from **service definitions** (`deploy.labels`), not from individual containers. The same `maintenant.*` labels apply:

```yaml
services:
  api:
    image: myapp:latest
    deploy:
      labels:
        maintenant.group: "production"
        maintenant.alert.severity: "critical"
        maintenant.alert.channels: "ops-webhook"
```

Stack grouping happens automatically via the `com.docker.stack.namespace` label set by `docker stack deploy`. Use `maintenant.group` to override it.

!!! warning "Use deploy.labels, not labels"
    Top-level `labels` in a Compose file are applied to containers. For Swarm services, use `deploy.labels` — these are accessible via the Swarm API and are what maintenant reads.

See the [Docker Swarm Monitoring](../features/swarm.md) guide for full details.

---

## Full Stack Example

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
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"

  api:
    image: myapp:latest
    labels:
      maintenant.group: "production"
      maintenant.endpoint.http: "http://api:3000/health"
      maintenant.endpoint.interval: "15s"
      maintenant.alert.severity: "critical"
      maintenant.alert.channels: "ops-webhook"
      maintenant.update.tag-exclude: "(rc|beta|alpha)"         # No pre-releases

  node:
    image: node:20.12.0-alpine
    labels:
      maintenant.update.tag-include: "^20\\.\\d+\\.\\d+-alpine$$"  # Stay on Node 20 alpine

  postgres:
    image: postgres:16
    labels:
      maintenant.endpoint.tcp: "postgres:5432"
      maintenant.alert.severity: "critical"

  redis:
    image: redis:7-alpine
    labels:
      maintenant.endpoint.tcp: "redis:6379"

  nginx:
    image: nginx:alpine
    labels:
      maintenant.endpoint.0.http: "https://nginx:443/health"
      maintenant.endpoint.1.tcp: "nginx:443"
      maintenant.tls.certificates: "app.example.com,api.example.com"

  backup-runner:
    image: alpine:latest
    labels:
      maintenant.ignore: "true"

volumes:
  maintenant-data:
```

---

## Related

- [Endpoint Monitoring](../features/endpoints.md) — HTTP/TCP check details
- [Certificate Monitoring](../features/certificates.md) — TLS monitoring details
- [Container Monitoring](../features/containers.md) — Container labels (ignore, group)
- [Alert Engine](../features/alerts.md) — Alert routing labels
- [Docker Swarm Monitoring](../features/swarm.md) — Swarm service labels and grouping
- [Update Intelligence — Tag Filtering](../features/updates.md#tag-filtering) — Tag filter labels, priority rules, and examples

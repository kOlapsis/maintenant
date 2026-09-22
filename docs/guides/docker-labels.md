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

!!! note "One-off containers are skipped"

    `docker compose run` copies the service definition onto a throwaway
    container, labels included, but gives it a generated name. Since a
    label-discovered monitor is keyed on the container name, such a container
    would mint a second monitor for a service that already has one, and that
    monitor would outlive the container it came from.

    maintenant skips any container stamped `com.docker.compose.oneoff=True`,
    so a one-off run never appears in your fleet and never creates a monitor.

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

The number of endpoints declared by labels is not capped in any edition: the Community cap of 10 endpoints only counts the ones you add by hand from the interface.

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

## Reverse proxy labels (Traefik, Caddy)

Most containers behind a reverse proxy already describe their public address in labels. With this setting on, maintenant reads those labels and creates one HTTP endpoint per container, so you do not have to repeat its address in `maintenant.endpoint.*` labels.

The feature is **off by default**. Turn it on with the environment variable or the flag:

| CLI flag | Environment variable | Default |
|----------|----------------------|---------|
| `--proxyLabels` | `MAINTENANT_PROXY_LABELS` | `false` |

It applies to the Docker runtime, in standalone and server mode and on every agent. An agent reads its own `MAINTENANT_PROXY_LABELS`: the server's value does not propagate to it.

### Which labels are read

**Traefik** (HTTP routers only):

| Label | Used for |
|-------|----------|
| `traefik.enable` | `false` disables discovery for the container |
| `traefik.http.routers.<name>.rule` | Hostnames from `Host(...)`, plus an optional single `Path(...)` or `PathPrefix(...)` |
| `traefik.http.routers.<name>.tls` and `traefik.http.routers.<name>.tls.*` | Any value other than `false` makes the URL `https` |
| `traefik.http.routers.<name>.entrypoints` | `websecure`, `https` or `443` in the list makes the URL `https` |
| `traefik.http.routers.<name>.middlewares` | A middleware that authenticates puts the route behind authentication |
| `traefik.http.middlewares.<name>.basicauth.*`, `.digestauth.*`, `.forwardauth.*` | Declares that middleware as an authenticating one |

`Host(...)` accepts backticks or quotes, several hostnames (`` Host(`a.example.com`, `b.example.com`) ``) and `||` combinations. When the rule also has exactly one `Path` or `PathPrefix`, that path is appended to the URL. `HostRegexp`, `HostSNI`, and TCP or UDP routers are ignored: there is no single URL to check behind them. A router with neither TLS nor a secure entrypoint gives an `http://` URL.

A middleware counts as authentication when the container declares it with `basicauth`, `digestauth` or `forwardauth`, or when its name contains `auth` in any case. Middlewares like Authelia or Authentik are usually declared on the proxy rather than on the container, so their name is all there is to go on. The `@provider` suffix (`authelia@file`) is ignored when reading the name.

**Caddy docker-proxy**:

| Label | Used for |
|-------|----------|
| `caddy`, `caddy_0`, `caddy_1`, ... | Site addresses, separated by spaces or commas |
| `caddy.tls`, `caddy_<n>.tls` | `internal` turns off TLS verification for the sites of that key |
| `caddy.basicauth*`, `caddy.forward_auth*`, and their `caddy_<n>.` forms | Puts the sites of that key behind authentication |

Wildcards (`*.example.com`), placeholders (`{$DOMAIN}`) and bare ports (`:80`) are skipped. An address written `http://host` or on port `80` gives an `http://` URL; every other address gives `https://`, because Caddy serves HTTPS by default. A port other than `443` is kept in the URL. Subkeys such as `caddy.reverse_proxy` are not site addresses and are never read as such.

### What gets created

**One container gives one endpoint**, `maintenant.endpoint.0.http`. A container usually declares several routes for the same service, a main route plus a websocket route on a path and an API route, and monitoring them all would only fill the endpoint list with near duplicates. So every URL found on the container is collected, and a single one is kept, chosen in this order:

1. **No authentication in front of it.** A route behind basic auth or a forward auth answers `401` to a checker, which is not what you want to watch.
2. **No path.** A URL at the root wins over one with a path; between two paths, the shorter one.
3. **`https` over `http`.**
4. **Lowest URL in alphabetical order**, so the same container always gives the same endpoint across restarts.

Take a container with `` Host(`app.example.com`) `` on one router and `` Host(`app.example.com`) && PathPrefix(`/ws`) `` on another: only `https://app.example.com` is monitored. If the root route is the one behind a basic auth middleware, `https://app.example.com/ws` is taken instead.

The endpoint behaves exactly like one you declare yourself, on the server and on agents alike.

- **Expected status**: the discovered endpoint accepts `2xx,3xx`, since a proxied site often answers with a redirect. Set `maintenant.endpoint.http.expected-status` on the container to use your own value.
- **Other settings**: global endpoint labels without an index, like `maintenant.endpoint.interval`, `maintenant.endpoint.timeout` or `maintenant.endpoint.failure-threshold`, apply to the discovered endpoint.
- **Certificates**: when the selected URL is `https`, its hostname is added to `maintenant.tls.certificates`, merged with any value already there, so its certificate is monitored as well. A site with `tls internal` is left out of that label, because a certificate signed by the proxy itself has nothing useful to track. On the server's own Docker runtime it still gets a monitor: every HTTPS endpoint checked locally is picked up by the automatic detection, which predates this setting.

### When discovery is skipped

Explicit labels always win. Nothing is discovered on a container that:

- declares its own endpoint target: `maintenant.endpoint.http`, `maintenant.endpoint.tcp`, or an indexed `maintenant.endpoint.<n>.http` / `maintenant.endpoint.<n>.tcp`. Global settings such as `maintenant.endpoint.interval` do **not** count as a target;
- has `maintenant.ignore: "true"`;
- opts out with `maintenant.proxy-labels: "false"`.

!!! warning "Docker containers only"
    Discovery reads container labels. Swarm services (`deploy.labels`) and Kubernetes Ingress resources are not read: declare those endpoints with `maintenant.endpoint.*` labels.

### Example: Traefik

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - /etc/os-release:/host/etc/os-release:ro
      - maintenant-data:/data
    environment:
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"
      MAINTENANT_PROXY_LABELS: "true"

  traefik:
    image: traefik:v3.1
    command:
      - --providers.docker=true
      - --providers.docker.exposedbydefault=false
      - --entrypoints.web.address=:80
      - --entrypoints.websecure.address=:443
      - --certificatesresolvers.le.acme.tlschallenge=true
      - --certificatesresolvers.le.acme.email=ops@example.com
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro

  # https://app.example.com (TLS through a certificate resolver)
  app:
    image: myapp:latest
    labels:
      traefik.enable: "true"
      traefik.http.routers.app.rule: "Host(`app.example.com`)"
      traefik.http.routers.app.tls.certresolver: "le"
      maintenant.endpoint.interval: "1m"

  # https://example.com: two hostnames in the rule, the first in alphabetical
  # order is kept
  site:
    image: mysite:latest
    labels:
      traefik.enable: "true"
      traefik.http.routers.site.rule: "Host(`example.com`) || Host(`www.example.com`)"
      traefik.http.routers.site.entrypoints: "websecure"

  # https://api.example.com: the websocket route on /ws loses to the root one
  api:
    image: myapi:latest
    labels:
      traefik.enable: "true"
      traefik.http.routers.api.rule: "Host(`api.example.com`)"
      traefik.http.routers.api.tls: "true"
      traefik.http.routers.api-ws.rule: "Host(`api.example.com`) && PathPrefix(`/ws`)"
      traefik.http.routers.api-ws.tls: "true"
      maintenant.endpoint.http.expected-status: "200"

  # https://grafana.example.com/public: the root route is behind a basic auth,
  # so the auth free route is monitored instead
  grafana:
    image: grafana/grafana:latest
    labels:
      traefik.enable: "true"
      traefik.http.routers.grafana.rule: "Host(`grafana.example.com`)"
      traefik.http.routers.grafana.tls: "true"
      traefik.http.routers.grafana.middlewares: "grafana-auth"
      traefik.http.middlewares.grafana-auth.basicauth.users: "ops:$$2y$$05$$..."
      traefik.http.routers.grafana-public.rule: "Host(`grafana.example.com`) && PathPrefix(`/public`)"
      traefik.http.routers.grafana-public.tls: "true"

  # Not discovered: Traefik is disabled for this container
  admin:
    image: myadmin:latest
    labels:
      traefik.enable: "false"
      traefik.http.routers.admin.rule: "Host(`admin.example.com`)"

volumes:
  maintenant-data:
```

### Example: Caddy docker-proxy

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - /etc/os-release:/host/etc/os-release:ro
      - maintenant-data:/data
    environment:
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"
      MAINTENANT_PROXY_LABELS: "true"

  caddy:
    image: lucaslorentz/caddy-docker-proxy:2.9
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - caddy-data:/data

  # https://app.example.com
  app:
    image: myapp:latest
    labels:
      caddy: "app.example.com"
      caddy.reverse_proxy: "{{upstreams 8080}}"

  # https://example.com: three addresses on the container, the first in
  # alphabetical order is kept
  site:
    image: mysite:latest
    labels:
      caddy: "example.com, www.example.com"
      caddy.reverse_proxy: "{{upstreams 80}}"
      caddy_1: "status.example.com:8443"
      caddy_1.reverse_proxy: "{{upstreams 3000}}"

  # https://metrics.example.com: the admin site is behind a basic auth, so it is
  # not the one monitored
  metrics:
    image: mymetrics:latest
    labels:
      caddy: "admin.example.com"
      caddy.basicauth.ops: "$$2a$$14$$..."
      caddy.reverse_proxy: "{{upstreams 9090}}"
      caddy_1: "metrics.example.com"
      caddy_1.reverse_proxy: "{{upstreams 9090}}"

  # https://grafana.lan, checked without TLS verification (Caddy's local CA)
  grafana:
    image: grafana/grafana:latest
    labels:
      caddy: "grafana.lan"
      caddy.tls: "internal"
      caddy.reverse_proxy: "{{upstreams 3000}}"

  # Not discovered: the container opts out
  staging:
    image: myapp:staging
    labels:
      caddy: "staging.example.com"
      caddy.reverse_proxy: "{{upstreams 8080}}"
      maintenant.proxy-labels: "false"

volumes:
  maintenant-data:
  caddy-data:
```

### Example: agent

An agent probes the endpoints it discovers from its own host, so set the variable on the agent:

```yaml
services:
  maintenant-agent:
    image: ghcr.io/kolapsis/maintenant:latest
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - /etc/os-release:/host/etc/os-release:ro
      - maintenant-agent-data:/var/lib/maintenant
    environment:
      MAINTENANT_PROXY_LABELS: "true"
    command:
      - --mode=agent
      - --server=grpcs://agents.example.com
      - --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX

volumes:
  maintenant-agent-data:
```

---

## Image metadata (OCI labels)

Docker copies the labels of an image onto every container created from it. maintenant reads the standard [OCI image annotations](https://github.com/opencontainers/image-spec/blob/main/annotations.md) from there and shows them in the container detail panel. No setting is needed.

| Shown as | Label read | Fallback |
|----------|------------|----------|
| Version | `org.opencontainers.image.version` | `org.label-schema.version` |
| Description | `org.opencontainers.image.description` | `org.label-schema.description` |
| Source (link) | `org.opencontainers.image.source` | `org.label-schema.vcs-url` |
| Documentation (link) | `org.opencontainers.image.url` | `org.label-schema.url`, then `org.opencontainers.image.documentation` |

Source and documentation are only shown when they are `http://` or `https://` URLs. Each field is hidden when its label is absent. Many public images already carry these labels; for your own, set them in the Dockerfile:

```dockerfile
FROM node:20-alpine
LABEL org.opencontainers.image.version="1.4.2" \
      org.opencontainers.image.description="Acme customer API" \
      org.opencontainers.image.source="https://github.com/acme/api" \
      org.opencontainers.image.url="https://docs.acme.dev/api"
```

The metadata is also reported by agents and exposed on the containers API as `image_version`, `image_description`, `image_source` and `image_url`.

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
- [Multi-Host Monitoring](../features/multihost.md): agents read their own `MAINTENANT_PROXY_LABELS`
- [Alert Engine](../features/alerts.md) — Alert routing labels
- [Docker Swarm Monitoring](../features/swarm.md) — Swarm service labels and grouping
- [Update Intelligence — Tag Filtering](../features/updates.md#tag-filtering) — Tag filter labels, priority rules, and examples

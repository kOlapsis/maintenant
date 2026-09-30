# Docker Labels Reference

maintenant uses Docker labels to configure monitoring directly on your containers. No config files, no UI clicks: add labels to your `docker-compose.yml` and maintenant picks them up automatically.

---

## Where labels are read

| Runtime | What is read |
|---------|--------------|
| **Docker** (the server's own daemon) | The labels of every container. |
| **Agents** (Docker) | The same labels, on the agent's host. The agent probes the endpoints and certificates itself and re-reads the labels every 30 seconds. |
| **Docker Swarm** | On a manager, the labels of the service (`deploy.labels`) under those of the task container. See [Swarm service labels](#swarm-service-labels). |
| **Kubernetes** | Annotations with the same `maintenant.` names on the workloads, for `maintenant.ignore`, `maintenant.group`, `maintenant.alert.*` and `maintenant.update.*` only. See [Kubernetes annotations](#kubernetes-annotations). |

The labels of a container are read when it starts, when maintenant starts and when its runtime reconnects. To change a label on a running container, recreate it (`docker compose up -d` does so when the labels changed). A label whose value maintenant cannot use is ignored, and most of them are logged as a warning.

### Every label maintenant reads

| Label | Purpose | Details |
|-------|---------|---------|
| `maintenant.ignore` | Leave the container out of monitoring | [Container settings](#container-settings) |
| `maintenant.group` | Custom group name | [Container settings](#container-settings) |
| `maintenant.alert.severity` | Severity of the `container_down` alert | [Container settings](#container-settings) |
| `maintenant.alert.restart_threshold` | Restarts before a `restart_loop` alert | [Container settings](#container-settings) |
| `maintenant.endpoint.*` | HTTP and TCP endpoint checks | [Endpoint monitoring](#endpoint-monitoring) |
| `maintenant.tls.certificates` | Certificates to monitor | [Certificate monitoring](#certificate-monitoring) |
| `maintenant.proxy-labels` | Opt a container out of reverse proxy discovery | [Reverse proxy labels](#reverse-proxy-labels-traefik-caddy) |
| `maintenant.update.*` | Image update tracking | [Update settings](#update-settings) |
| `traefik.*`, `caddy*` | Endpoints derived from the proxy labels, when `MAINTENANT_PROXY_LABELS` is on | [Reverse proxy labels](#reverse-proxy-labels-traefik-caddy) |
| `org.opencontainers.image.*`, `org.label-schema.*` | Version, description and links shown in the container panel | [Image metadata](#image-metadata-oci-labels) |
| `com.docker.compose.project`, `.service`, `.project.working_dir`, `.oneoff` | Group, service name, directory used in update commands, and skipping of `docker compose run` containers | Set by Compose |
| `com.docker.stack.namespace`, `com.docker.swarm.service.id`, `.service.name`, `.node.id`, `.task.id`, `.task.name` | Stack group, service, node and slot of a Swarm task | Set by Docker |

Alert routing is not configured with labels. Channels and triggers decide who is notified: see the [Alert Engine](../features/alerts.md). The former `maintenant.alert.channels` label no longer exists and is ignored.

---

## Container Settings

| Label | Values | Description |
|-------|--------|-------------|
| `maintenant.ignore` | `true` or `1` | Leave this container out of monitoring. Any other value, including `True`, does not ignore it. |
| `maintenant.group` | any non-empty string | Group name, instead of the Compose project, the Swarm stack or the Kubernetes namespace. |
| `maintenant.alert.severity` | `critical`, `warning`, `info` | Severity of the `container_down` alert for this container. Default `warning`. |
| `maintenant.alert.restart_threshold` | positive integer | Restarts within 10 minutes before a `restart_loop` alert is raised. Default `3`. |

```yaml
labels:
  maintenant.ignore: "true"                    # Exclude from monitoring
  maintenant.group: "backend"                  # Custom group name
  maintenant.alert.severity: "critical"        # Severity of the container_down alert
  maintenant.alert.restart_threshold: "5"      # Restart loop threshold
```

An ignored container is hidden from the dashboard and raises no alert. It produces no state-change event, no endpoint or certificate monitor, no resource sample, no security insight and no update check.

`maintenant.alert.severity` applies to `container_down`, which only exists when `MAINTENANT_CONTAINER_DOWN_AFTER` is set (see [Configuration](../getting-started/configuration.md)). It does not change the other alerts: `restart_loop` is a warning that becomes critical at three times the threshold, and `health_unhealthy` is a warning. An invalid value is ignored and the severity stays `warning`.

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

Endpoint labels apply to Docker containers: the server's own, those of agents and, on a Swarm manager, those of service tasks. Kubernetes annotations do not declare endpoints.

### Simple: one endpoint per container

| Label | Default | Description |
|-------|---------|-------------|
| `maintenant.endpoint.http` | — | HTTP(S) URL to check. It needs an `http://` or `https://` scheme and a host. |
| `maintenant.endpoint.tcp` | — | `host:port` to check, with a numeric port. |
| `maintenant.endpoint.interval` | `30s` | Check interval (Go duration, for example `15s` or `2m`). |
| `maintenant.endpoint.timeout` | `10s` | Timeout of one check. A timeout longer than the interval is lowered to the interval and logged. |
| `maintenant.endpoint.failure-threshold` | `3` | Consecutive failed checks before the endpoint alert is raised. |
| `maintenant.endpoint.recovery-threshold` | `2` | Consecutive successful checks before the alert is resolved. |

The thresholds only drive the alert. The status of the endpoint turns `down` on the first failed check and `up` on the first successful one. A value that is not a positive duration or integer is ignored and the default stays.

#### HTTP-Specific Options

| Label | Default | Description |
|-------|---------|-------------|
| `maintenant.endpoint.http.method` | `GET` | `GET`, `HEAD`, `POST`, `PUT`, `DELETE`, `PATCH` or `OPTIONS`, in any case. |
| `maintenant.endpoint.http.expected-status` | `2xx` | Accepted status codes, comma-separated. Each item is an exact code (`200`) or a class (`2xx`, from `1xx` to `5xx`), for example `200,301,4xx`. Unusable items are dropped; if none remains, `2xx` applies. |
| `maintenant.endpoint.http.tls-verify` | `true` | Set `false`, `0` or `no` to accept a self-signed or otherwise untrusted certificate. |
| `maintenant.endpoint.http.headers` | — | Custom headers, as JSON (`{"K":"V"}`) or as `K=V,K=V`. |
| `maintenant.endpoint.http.max-redirects` | `5` | Redirects to follow, a non-negative integer. With `0` the first redirect answer is the result and is compared with the expected status. |

```yaml
labels:
  maintenant.endpoint.http: "https://api:8443/health"
  maintenant.endpoint.interval: "15s"
  maintenant.endpoint.failure-threshold: "3"
  maintenant.endpoint.http.method: "POST"
  maintenant.endpoint.http.expected-status: "200,201"
  maintenant.endpoint.http.tls-verify: "false"
```

A malformed target, for example a URL without a scheme or a TCP target without a numeric port, creates no endpoint. maintenant logs a warning and emits an `endpoint.config_error` event.

### Indexed: several endpoints per container

Use a numeric index between `endpoint` and the type to define multiple endpoints:

```yaml
labels:
  # First endpoint: HTTP health check
  maintenant.endpoint.0.http: "https://app:8443/health"
  maintenant.endpoint.0.interval: "15s"
  maintenant.endpoint.0.failure-threshold: "3"

  # Second endpoint: Redis TCP check
  maintenant.endpoint.1.tcp: "redis:6379"
  maintenant.endpoint.1.interval: "30s"
```

!!! warning "Do not mix simple and indexed"
    The simple form is endpoint `0`. If a container declares both `maintenant.endpoint.http` and `maintenant.endpoint.0.http`, the simple one is kept. A simple `http` and a simple `tcp` on the same container compete for the same slot, so declare the second one with an index.

Global config labels (without an index) apply as defaults to all indexed endpoints. Indexed config overrides global config.

The number of endpoints declared by labels is not capped in any edition: the Community cap of 10 endpoints only counts the ones you add by hand from the interface.

An agent applies `interval` and `timeout` to each endpoint it probes.

---

## Certificate Monitoring

| Label | Description |
|-------|-------------|
| `maintenant.tls.certificates` | Comma-separated list of hostnames to monitor |

Hostnames without a port default to port 443. Schemes (`https://`) and paths are stripped automatically. An entry that is not a valid hostname or port is skipped without a message, and duplicates are merged.

```yaml
labels:
  # Monitor three certificates
  maintenant.tls.certificates: "api.example.com,dashboard.example.com:8443,mail.example.com"
```

!!! info "Automatic detection"
    An HTTPS endpoint that the server probes itself, meaning one declared on a container of the server's own Docker runtime, automatically gets certificate monitoring. Use `maintenant.tls.certificates` for additional
    domains not covered by endpoint checks, and for the containers of an agent: an agent probes its endpoints from its own host and does not report their certificates, but it does scan the hosts listed in this label.

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

!!! warning "Docker only"
    Discovery reads Docker container labels and, on a Swarm manager, the `deploy.labels` of the service, which is where Traefik reads its configuration on Swarm. On a worker only the container's own labels are visible. Kubernetes Ingress resources and annotations are never read.

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

| Label | Values | Default | Description |
|-------|--------|---------|-------------|
| `maintenant.update.enabled` | `true` / `false` (also `1`/`0`, `yes`/`no`) | `true` | `false` leaves the container out of update checks. |
| `maintenant.update.track` | `major`, `minor`, `patch`, `digest` | `major` | Widest change that is reported. `major` reports any newer version, `minor` only newer versions of the same major version, `patch` only those of the same major and minor version, `digest` only a new image published under the current tag. |
| `maintenant.update.pin` | any non-empty value | — | Freezes the container: no update is reported for it. The value is not compared with the running tag. |
| `maintenant.update.ignore_major` | `true` / `false` | `false` | `true` behaves as `track: minor` when `track` is absent or `major`. It has no effect with another `track`. |
| `maintenant.update.digest_only` | `true` / `false` | `false` | `true` behaves as `track: digest` and wins over `track` and `ignore_major`. |
| `maintenant.update.alert_on` | `all`, `critical`, `none` | `all` | Which updates raise an `update_available` alert. `critical` keeps only updates of critical severity, which in practice means major updates. `none` tracks the update without alerting: it is still listed on the Updates page. |
| `maintenant.update.registry` | registry host | — | Changes the registry name reported with the update. It does not change where tags are queried, which always comes from the image reference. |
| `maintenant.update.tag-include` | Go regex | — | Only tags matching this pattern are update candidates. |
| `maintenant.update.tag-exclude` | Go regex | — | Tags matching this pattern are removed from the candidates, after `tag-include`. |

An unusable value (an unknown `track`, a boolean that is not a boolean, an invalid regular expression) is logged and ignored, and the default applies.

```yaml
labels:
  maintenant.update.track: "minor"                               # Stay on the current major version
  maintenant.update.alert_on: "critical"                         # Alert on major updates only
  maintenant.update.tag-include: "^20\\.\\d+\\.\\d+-alpine$$"  # Node 20 alpine only
  maintenant.update.tag-exclude: "(rc|beta|alpha)"              # No pre-releases
```

Update tracking covers the running containers of the server's Docker runtime and of Docker agents, and the workloads of the server's own Kubernetes cluster, where the settings are [annotations](#kubernetes-annotations). Images that were built locally and never pulled from a registry are not checked.

See [Tag Filtering](../features/updates.md#tag-filtering) in the Update Intelligence guide for full details, examples, and troubleshooting.

---

## Swarm Service Labels

On a Swarm manager, maintenant reads the labels of the **service** and lays the labels of the **task container** over them. When a key is set in both places, the container's value wins. Every `maintenant.*` label on this page works on a service, and so do the Traefik and Caddy labels when `MAINTENANT_PROXY_LABELS` is on.

```yaml
services:
  api:
    image: myapp:latest
    deploy:
      labels:
        maintenant.group: "production"
        maintenant.alert.severity: "critical"
```

| Where in the stack file | What it sets | Read by |
|-------------------------|--------------|---------|
| `deploy.labels` | Labels of the service | A maintenant or an agent running on a **manager**, which can read services |
| `labels` (top level) | Labels of each task container | Every maintenant and every agent, including those on a worker |

On a worker the service cannot be read, so only the labels of the container count: use the top-level `labels` for anything a worker must see. Service labels are cached for 30 seconds. Changing them with `docker service update --label-add` does not restart the tasks, so running tasks take the new values the next time maintenant reconciles: at startup, when the runtime reconnects, or when a new container starts.

Each replica is a container of its own, so an endpoint label on a service with three replicas creates three monitors on the same URL.

The group of a task is the stack name (`com.docker.stack.namespace`), unless the container carries a Compose project. `maintenant.group` overrides both. `maintenant.ignore` on a service hides its tasks and silences the replica, crash-loop and rolling update alerts of that service.

See the [Docker Swarm Monitoring](../features/swarm.md) guide for full details.

---

## Kubernetes annotations

On Kubernetes, maintenant reads annotations instead of labels, with the same names and values. They go on the metadata of the workload itself, not on its pod template.

| Annotation | Put it on | Effect |
|------------|-----------|--------|
| `maintenant.ignore` | Deployment, StatefulSet, DaemonSet, Job, bare pod, or the pods of a workload (pod template) | `true` or `1`. On a workload, the workload and its pods raise no alert. On a pod, that pod raises no `crash_loop` alert. |
| `maintenant.group` | Deployment, StatefulSet, DaemonSet, bare pod | Group name, instead of the namespace. |
| `maintenant.alert.severity` | Deployment, StatefulSet, DaemonSet, bare pod | Severity of the `container_down` alert. |
| `maintenant.alert.restart_threshold` | Deployment, StatefulSet, DaemonSet, bare pod | Threshold of the `restart_loop` alert. |
| `maintenant.update.*` | Deployment, StatefulSet, DaemonSet, bare pod | Update tracking, see [Update settings](#update-settings). The image checked is the first container of the pod spec. |

```bash
kubectl annotate deployment/api -n production \
  maintenant.group=backend \
  maintenant.update.track=minor
```

Annotations are read on the server's own cluster. A Kubernetes agent reports the topology of its cluster without them. The endpoint and certificate labels have no Kubernetes equivalent.

A changed annotation is picked up when maintenant next reconciles: at startup, when the runtime reconnects, or when a new workload appears. The exception is `maintenant.ignore` on a workload or a pod, which drives the alerts and is re-read every 30 seconds. See the [Kubernetes guide](kubernetes.md#annotations).

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

- [Endpoint Monitoring](../features/endpoints.md): HTTP/TCP check details
- [Certificate Monitoring](../features/certificates.md): TLS monitoring details
- [Container Monitoring](../features/containers.md): ignore, group, restart loops
- [Multi-Host Monitoring](../features/multihost.md): agents read their own `MAINTENANT_PROXY_LABELS`
- [Alert Engine](../features/alerts.md): channels, triggers and alert types
- [Docker Swarm Monitoring](../features/swarm.md): Swarm service labels and grouping
- [Kubernetes Guide](kubernetes.md): annotations, RBAC and Helm
- [Update Intelligence: Tag Filtering](../features/updates.md#tag-filtering): tag filter labels, priority rules, and examples

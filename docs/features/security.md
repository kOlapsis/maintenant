# Network Security Insights

Automatic detection of dangerous network configurations across your containers. No setup required: maintenant analyzes container configurations as they are discovered and flags common misconfigurations.

---

## How It Works

maintenant inspects the network configuration of every monitored container and generates security insights for each detected risk. Insights are kept in memory and rebuilt from the runtime, so there is nothing to configure.

For each container of the Docker runtime that maintenant itself talks to, it checks:

- **Port bindings**: which ports are published and on which host interface
- **Network mode**: whether the container uses host networking
- **Privileged mode**: whether the container runs with elevated privileges

On Kubernetes, it checks which Services open a workload to traffic from outside the cluster.

The analysis runs at startup and again each time a container starts. On Kubernetes it runs every 30 seconds, together with the refresh of the cluster topology. A container that carries the label `maintenant.ignore=true` (or `1`), or a Kubernetes workload with the annotation `maintenant.ignore: "true"`, has no insights and is not scored.

Insights come only from the runtime the server itself monitors. Containers reported by remote agents, and workloads of a cluster watched through a Kubernetes agent, are not analyzed.

---

## Detected Insights

### Docker

| Insight | Severity | Description |
|---------|----------|-------------|
| `port_exposed_all_interfaces` | Critical | A published port is bound to all interfaces (no host IP, `0.0.0.0` or `::`), making it reachable from any network the host is on |
| `database_port_exposed` | Critical | The same, for the container port of a known database: 3306 (MySQL/MariaDB), 5432 (PostgreSQL), 6379 (Redis) or 27017 (MongoDB). It takes the place of `port_exposed_all_interfaces` for that port. |
| `privileged_container` | Critical | Container runs in privileged mode with full host access |
| `host_network_mode` | High | Container shares the host network namespace. The port bindings are not analyzed in that mode. |

A port published on a specific address (`127.0.0.1:8080:80`, or a LAN IP) raises nothing. The detection looks at the container port: `15432:5432` is reported as a PostgreSQL port.

### Kubernetes

Only the cluster the server runs in or is pointed at is analyzed, not the clusters of remote agents. A finding is raised for each port of a Service of type `LoadBalancer` or `NodePort` (with a selector) for each workload the Service selects: Deployments, StatefulSets, DaemonSets and pods that have no controller. Services of other types are ignored. Reading Services needs the `services` permission in the maintenant RBAC (see the [Kubernetes guide](../guides/kubernetes.md)).

| Insight | Severity | Description |
|---------|----------|-------------|
| `service_load_balancer` | Critical | A port of the workload is reachable from outside the cluster through a LoadBalancer Service |
| `service_node_port` | Critical | A port of the workload is published on every cluster node by a NodePort Service |
| `database_port_exposed` | Critical | The target port of such a Service is 3306, 5432, 6379 or 27017. It takes the place of the two insights above. |

The reported port is the numeric target port of the Service, or the Service port when the target is a named port.

---

## CVE Ecosystem Mapping

The CVE lookup is part of [Update Intelligence](updates.md) and needs the Personal edition. To look an image up in [OSV.dev](https://osv.dev), maintenant first has to work out which packages and which ecosystem (`Debian:12`, `Alpine:3.20`, `Ubuntu:22.04`, `Go`) the image belongs to. It tries, in order, and stops at the first answer:

1. **Known image table**: about fifteen common images are mapped directly. `nginx`, `postgres`, `redis`, `mysql`, `mariadb`, `node`, `python`, `golang`, `httpd`, `memcached`, `mongo` and `rabbitmq` map to Debian 12 packages; `traefik`, `grafana` and `prometheus` map to Go.
2. **Image labels from the registry**: for images of Docker Hub, `ghcr.io`, `quay.io`, `gcr.io` and `public.ecr.aws`, the `org.opencontainers.image.base.name` label names the base image (Debian, Ubuntu, Alpine, CentOS or Fedora) and its version. The fetch gives up after 5 seconds.
3. **Tag suffix**: a tag such as `1.25-alpine`, `alpine3.19`, `16-bookworm` or `jammy` names the distribution.
4. **Image name**: `consul`, `vault`, `caddy`, `minio` and `etcd` map to Go; any other name is assumed to be a Debian 12 package of the same name. Names that are two characters or shorter, look like a hash, or are `scratch` get no ecosystem.

An image with no ecosystem is recorded as *unsupported* and does not count in the posture score. A lookup that fails is recorded as an error. The lookup asks OSV.dev about one package, ecosystem and version (the image tag) per image; it does not open the image layers, so read the result as an indication rather than as a scan.

---

## Per-Container Detail

Security insights are displayed directly in the container detail panel. Each insight shows its severity, a description of the risk, and relevant details (port number, interface, service, etc.).

---

## Security Posture Dashboard :material-star-four-points:{ title="Personal" }

With maintenant Personal or Pro, a dedicated Security Posture page aggregates the findings into a scored view of infrastructure health:

- **Global posture score**: the average of the container scores, from 0 to 100, with a colour (green from 80, yellow from 60, orange from 40, red below)
- **Category breakdown**: the number of issues found in each category
- **Per-container risk ranking**: the ten lowest scores, with the worst issue of each and a drill-down to the findings
- **Risk acknowledgment**: dismiss known findings with an audit trail

The score of a container is a weighted average of five categories. Each category starts at 100 and loses points per finding:

| Category | Weight | Points lost |
|----------|-------:|-------------|
| `cves` | 30 | critical 30, high 15, medium 5, low 2 |
| `network_exposure` | 25 | critical 35, high 20, medium 10 (per insight) |
| `tls` | 20 | expired certificate 50, expiring 30 (7 days or less), 20 (14 days or less) or 10, check error 25 |
| `updates` | 15 | major update 25, minor 10, patch 5 |
| `image_age` | 10 | age of the pending update: nothing lost when no update is pending or it was published less than 30 days ago, then a linear drop to 0 at 365 days; 50 when its date is unknown |

A category that does not apply to a container is left out, and the weights of the others are scaled up to make 100. `tls` applies to containers that have a monitored certificate. `cves` applies once the image has been analyzed: an image with no ecosystem is left out, and an image that has not been analyzed yet, or whose analysis failed, leaves the score marked as partial. The other three always apply and score 100 when there is nothing to report. Acknowledged CVEs and acknowledged insights do not count against the score.

---

## Security Alerts

Two alert types come from this feature, both with source `security`:

- **`dangerous_configuration`**, in every edition: one alert per container, raised when the set of insights of the container changes and resolved when it is empty again. Its severity follows the worst insight: critical for critical, warning for high, info for medium. Like any alert, it reaches a channel through an [alert trigger](alerts.md#alert-triggers).
- **`posture_threshold`**, from Personal: set `MAINTENANT_SECURITY_SCORE_THRESHOLD` to a whole number above 0 to be alerted when the infrastructure score falls below it. The alert is a warning, and critical when the score is more than 20 points under the threshold. It resolves when the score is back at the threshold or above. Unset or `0`, there is no threshold alert.

The infrastructure score is only computed when something asks for it (the Security Posture page, the dashboard or a call to `GET /api/v1/security/posture`), and the threshold is checked at that moment. No alert is raised while nobody looks at the score.

---

## Acknowledging Findings :material-star-four-points:{ title="Personal" }

Some findings describe a configuration that is deliberate: a port intentionally published to a trusted LAN, or maintenant's own UI port (the scanner does not exempt maintenant's container; see [Configuration → Choosing a Bind Address](../getting-started/configuration.md#choosing-a-bind-address)). Acknowledging a finding records who accepted the risk and why, removes it from the posture score, and, once every insight on the container is acknowledged, automatically acknowledges the container's active `dangerous_configuration` alert.

- **UI**: container detail panel, *Acknowledge* on the finding. The UI records the acknowledgment as `user` without a reason; use the API to record a name and a reason.
- **API**: `POST /api/v1/security/acknowledgments`. For port findings, `finding_key` is `{port}/{protocol}` (`8080/tcp`); for findings that have no port (`privileged_container`, `host_network_mode`) it is empty; for a CVE, `finding_type` is `cve` and `finding_key` is the CVE identifier. An already acknowledged finding returns `409 ALREADY_ACKNOWLEDGED`.

```json
{
  "container_id": "…",
  "finding_type": "port_exposed_all_interfaces",
  "finding_key": "8080/tcp",
  "acknowledged_by": "ops@example.com",
  "reason": "UI intentionally exposed to the trusted LAN"
}
```

Revoke with `DELETE /api/v1/security/acknowledgments/{id}`: the finding counts against the posture score again.

On Community Edition, address the finding at the source instead: publish the port on a specific interface (`127.0.0.1:8080:8080` behind a reverse proxy, or a LAN IP for direct access).

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/security/insights` | List all insights across all containers |
| `GET` | `/api/v1/security/insights/{container_id}` | Get insights for a specific container |
| `GET` | `/api/v1/security/summary` | Aggregated insight counts by severity and type |
| `GET` | `/api/v1/security/posture` | Global infrastructure posture score :material-star-four-points:{ title="Personal" } |
| `GET` | `/api/v1/security/posture/containers` | Per-container posture scores, worst first (`limit`, `offset`) :material-star-four-points:{ title="Personal" } |
| `GET` | `/api/v1/security/posture/containers/{id}` | Single container posture score :material-star-four-points:{ title="Personal" } |
| `POST` | `/api/v1/security/acknowledgments` | Acknowledge a finding :material-star-four-points:{ title="Personal" } |
| `DELETE` | `/api/v1/security/acknowledgments/{id}` | Revoke an acknowledgment :material-star-four-points:{ title="Personal" } |
| `GET` | `/api/v1/security/acknowledgments` | List acknowledgments, optionally for one `container_id` :material-star-four-points:{ title="Personal" } |

The posture and acknowledgment routes return `403 EDITION_REQUIRED` below Personal. The same data is available to AI assistants through the [MCP server](mcp.md): `get_security_insights` (Community), `list_cve`, `list_risk_scores` and `get_security_posture` (Personal).

---

## Related

- [Container Monitoring](containers.md): Container detail panel shows security insights
- [Update Intelligence](updates.md): CVE enrichment uses ecosystem data from security analysis
- [Alert Engine](alerts.md): `dangerous_configuration` and `posture_threshold` alerts

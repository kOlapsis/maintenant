# Endpoint Monitoring

Define HTTP or TCP checks directly as Docker labels — no config files, no UI clicks. maintenant picks them up automatically when a container starts.

![Endpoint Monitoring](../screen-captures/3-endpoints.png)

---

## How It Works

maintenant reads endpoint definitions from Docker labels on your containers. When a container with endpoint labels starts, maintenant automatically begins monitoring those endpoints at the configured interval.

Each check records:

- **Response time** — How long the endpoint took to respond
- **Status** — Up or down, based on HTTP status code or TCP connection success
- **Uptime history** — 90-day uptime with daily breakdowns and sparkline charts

---

## Quick Start

Add labels to any container in your `docker-compose.yml`:

```yaml
services:
  api:
    image: myapp:latest
    labels:
      maintenant.endpoint.http: "http://api:3000/health"
      maintenant.endpoint.interval: "15s"
```

That is it. maintenant starts checking `http://api:3000/health` every 15 seconds as soon as the container starts.

---

## HTTP Checks

HTTP checks send a request to the configured URL and validate the response status code.

```yaml
labels:
  maintenant.endpoint.http: "https://api:8443/health"
```

### Configuration Options

| Label | Default | Description |
|-------|---------|-------------|
| `maintenant.endpoint.http` | — | URL to check (required for HTTP) |
| `maintenant.endpoint.http.method` | `GET` | HTTP method (`GET`, `POST`, `HEAD`, etc.) |
| `maintenant.endpoint.http.expected-status` | `200` | Expected status codes (comma-separated, e.g., `200,201`) |
| `maintenant.endpoint.http.tls-verify` | `true` | Verify TLS certificates. Set to `false` for self-signed certs. |
| `maintenant.endpoint.interval` | `30s` | Check interval (Go duration format) |
| `maintenant.endpoint.timeout` | `10s` | Request timeout |
| `maintenant.endpoint.failure-threshold` | `1` | Consecutive failures before marking as down |
| `maintenant.endpoint.recovery-threshold` | `1` | Consecutive successes before marking as up |

---

## TCP Checks

TCP checks attempt to establish a connection to the configured host and port.

```yaml
labels:
  maintenant.endpoint.tcp: "postgres:5432"
```

Useful for databases, caches, and services that do not expose HTTP endpoints.

---

## Multiple Endpoints per Container

Use indexed labels to monitor multiple endpoints from a single container:

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

!!! info "Indexed vs simple labels"
    You can use either **simple** labels (`maintenant.endpoint.http`) for a single endpoint
    or **indexed** labels (`maintenant.endpoint.0.http`, `maintenant.endpoint.1.tcp`) for multiple
    endpoints. Do not mix both styles on the same container.

---

## Failure and Recovery Thresholds

Thresholds control how many consecutive check results are needed to change the endpoint status. This prevents flapping from transient network issues.

```yaml
labels:
  maintenant.endpoint.http: "https://api:3000/health"
  maintenant.endpoint.failure-threshold: "3"   # 3 consecutive failures = down
  maintenant.endpoint.recovery-threshold: "2"  # 2 consecutive successes = up
```

- **failure-threshold** — Number of consecutive failures before the endpoint is marked as `down` and an alert is triggered.
- **recovery-threshold** — Number of consecutive successes before the endpoint is marked as `up` again.

---

## Uptime History and Sparklines

maintenant records every check result and computes:

- **Daily uptime percentage** — Available via `GET /api/v1/endpoints/{id}/uptime/daily`
- **Response time trends** — Visualized as sparkline charts in the dashboard
- **90-day history** — Long-term uptime tracking

---

## Related

- [Docker Labels Reference](../guides/docker-labels.md) — Complete label reference
- [TLS Certificate Monitoring](certificates.md) — HTTPS endpoints automatically get certificate monitoring
- [Alert Engine](alerts.md) — Configure alerts for endpoint failures

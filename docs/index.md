# maintenant

**The all-in-one monitoring dashboard your self-hosted stack deserves.**

Drop a single container. Watch everything. Sleep at night.

---

## What is maintenant?

Most self-hosters juggle 3-5 tools to monitor their stack: one for containers, one for uptime, one for certs, one for metrics, and yet another for a status page. maintenant replaces all of them with a single binary, zero external dependencies, and zero configuration to get started.

Deploy one container, and maintenant auto-discovers your entire stack. Docker or Kubernetes — it does not matter.

![Dashboard](screen-captures/1-dashboard.png)

---

## Key Features

- **[Container Monitoring](features/containers.md)** — Zero-config auto-discovery for Docker and Kubernetes. State tracking, health checks, restart loop detection, log streaming.
- **[Docker Swarm Monitoring](features/swarm.md)** — Automatic Swarm service discovery, stack grouping, node health, crash-loop detection, rolling update tracking. CE + Enterprise.
- **[Endpoint Monitoring](features/endpoints.md)** — HTTP and TCP checks defined as Docker labels. Response times, uptime history, sparklines.
- **[Heartbeat & Cron Monitoring](features/heartbeats.md)** — Create a monitor, get a URL, curl from your cron job. Tracks durations, exit codes, missed deadlines.
- **[TLS Certificate Monitoring](features/certificates.md)** — Auto-detection from HTTPS endpoints. Alerts at 30, 14, 7, 3, and 1 day before expiry. Full chain validation.
- **[Resource Metrics](features/resources.md)** — CPU, memory, network I/O, disk I/O per container. Historical charts, alert thresholds, top consumers view.
- **[Update Intelligence](features/updates.md)** — OCI registry scanning, digest comparison. Compose-aware update commands. Know when your images have updates available.
- **[Network Security Insights](features/security.md)** — Automatic detection of exposed ports, dangerous network configurations, and privileged containers. CVE ecosystem mapping via OCI manifest inspection.
- **[Alert Engine](features/alerts.md)** — Unified alerts across all sources. Webhook and Discord channels. Silence rules, exponential backoff. Slack, Teams, and Email with Pro.
- **[Public Status Page](features/status-page.md)** — Component groups, live SSE updates. Incident management, maintenance windows, and subscriber notifications with Pro.
- **[MCP Server](features/mcp.md)** — Expose monitoring data to AI assistants (Claude Code, Cursor) via the Model Context Protocol. 18 tools, stdio and HTTP transports.

---

## Comparison

| | maintenant | Uptime Kuma | Portainer | Dozzle |
|---|:---:|:---:|:---:|:---:|
| Container auto-discovery | **Yes** | No | Yes | Yes |
| HTTP/TCP endpoint checks | **Yes** | Yes | No | No |
| Cron/heartbeat monitoring | **Yes** | Yes | No | No |
| SSL certificate tracking | **Yes** | Yes | No | No |
| CPU/memory/network metrics | **Yes** | No | Limited | No |
| Image update detection | **Yes** | No | Yes | No |
| Network security insights | **Yes** | No | No | No |
| Public status page | **Yes** | Yes | No | No |
| Alerting (webhook, Discord) | **Yes** | Yes | Limited | No |
| Docker Swarm monitoring | **Yes** | No | Limited | No |
| Kubernetes native | **Yes** | No | Yes | No |
| MCP for AI assistants | **Yes** | No | No | No |
| Single binary, zero deps | **Yes** | Node.js | Docker API | Docker API |

---

## Quick Start

Get maintenant running in 30 seconds:

```yaml
# docker-compose.yml
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
    restart: unless-stopped

volumes:
  maintenant-data:
```

```bash
docker compose up -d
```

Open **http://localhost:8080** — your containers are already there. No configuration needed.

For detailed installation instructions, see [Installation](getting-started/installation.md).

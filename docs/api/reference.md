# API Reference

All endpoints are under `/api/v1/`. Responses are JSON. Errors follow a standard format:

```json
{
  "error": {
    "code": "not_found",
    "message": "Container not found"
  }
}
```

---

## Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/health` | Health check, returns `{"status": "ok", "version": "..."}` |
| `GET` | `/api/v1/runtime/status` | Runtime info (docker/kubernetes, connection state) |
| `GET` | `/api/v1/edition` | Edition and feature flags |

---

## Containers

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/containers` | List all containers with groups |
| `GET` | `/api/v1/containers/{id}` | Get container details with uptime stats |
| `GET` | `/api/v1/containers/{id}/transitions` | List state transitions |
| `GET` | `/api/v1/containers/{id}/logs` | Fetch recent logs |
| `GET` | `/api/v1/containers/{id}/logs/stream` | Stream logs in real time (SSE) |
| `DELETE` | `/api/v1/containers/{id}` | Remove a container from monitoring |
| `GET` | `/api/v1/containers/{id}/endpoints` | List endpoints for a container |

---

## Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/endpoints` | List all monitored endpoints |
| `GET` | `/api/v1/endpoints/{id}` | Get endpoint details |
| `GET` | `/api/v1/endpoints/{id}/checks` | List check results |
| `GET` | `/api/v1/endpoints/{id}/uptime/daily` | Daily uptime percentages |

---

## Heartbeats

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/heartbeats` | List all heartbeat monitors |
| `POST` | `/api/v1/heartbeats` | Create a heartbeat monitor |
| `GET` | `/api/v1/heartbeats/{id}` | Get a heartbeat monitor |
| `PUT` | `/api/v1/heartbeats/{id}` | Update a heartbeat monitor |
| `DELETE` | `/api/v1/heartbeats/{id}` | Delete a heartbeat monitor |
| `POST` | `/api/v1/heartbeats/{id}/pause` | Pause deadline checking |
| `POST` | `/api/v1/heartbeats/{id}/resume` | Resume deadline checking |
| `GET` | `/api/v1/heartbeats/{id}/executions` | List executions |
| `GET` | `/api/v1/heartbeats/{id}/pings` | List raw pings |
| `GET` | `/api/v1/heartbeats/{id}/uptime/daily` | Daily uptime percentages |

### Ping Endpoints (Public)

These routes do not require authentication:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET/POST` | `/ping/{uuid}` | Simple ping (success) |
| `GET/POST` | `/ping/{uuid}/start` | Signal job start |
| `GET/POST` | `/ping/{uuid}/{exit_code}` | Ping with exit code (0 = success) |

---

## Certificates

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/certificates` | List all certificate monitors |
| `POST` | `/api/v1/certificates` | Create a standalone certificate monitor |
| `GET` | `/api/v1/certificates/{id}` | Get certificate details |
| `PUT` | `/api/v1/certificates/{id}` | Update a certificate monitor |
| `DELETE` | `/api/v1/certificates/{id}` | Delete a certificate monitor |
| `GET` | `/api/v1/certificates/{id}/checks` | List check history |

---

## Resources

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/containers/{id}/resources/current` | Current CPU, memory, network, I/O | |
| `GET` | `/api/v1/containers/{id}/resources/history` | Historical metrics (`?range=24h`) | Pro |
| `GET` | `/api/v1/containers/{id}/resources/alerts` | Get alert thresholds | |
| `PUT` | `/api/v1/containers/{id}/resources/alerts` | Set alert thresholds | |
| `GET` | `/api/v1/resources/summary` | Aggregate resource summary (`?agent_id=local\|<id>` to scope to a host) | |
| `GET` | `/api/v1/resources/top` | Top consumers (`?sort=cpu&limit=10`, `?agent_id=` to scope to a host) | |
| `GET` | `/api/v1/resources/hosts` | List hosts (local + agents) with current CPU/memory/disk | Pro |

---

## Agents

Multi-host agent management (`--mode=server`). All endpoints require **Pro** — they return `402 pro_required` in Community Edition. See [Multi-Host Monitoring](../features/multihost.md).

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/agents` | List enrolled agents and their connection state |
| `GET` | `/api/v1/agents/{id}` | Get an agent |
| `PATCH` | `/api/v1/agents/{id}` | Update an agent's display label |
| `POST` | `/api/v1/agents/{id}/revoke` | Revoke an agent (closes its stream, stops retries) |
| `DELETE` | `/api/v1/agents/{id}` | Delete an agent and purge all its events |
| `GET` | `/api/v1/agents/metrics` | Aggregate fleet metrics (counts by status) |
| `POST` | `/api/v1/agents/enrollment-tokens` | Create a one-time enrollment token (`{ "ttl_hours": 24 }`) |
| `GET` | `/api/v1/agents/enrollment-tokens` | List enrollment tokens (masked) |
| `GET` | `/api/v1/agents/enrollment-tokens/{token_id}` | Get an enrollment token (masked) |
| `DELETE` | `/api/v1/agents/enrollment-tokens/{token_id}` | Delete an enrollment token |

---

## Alerts

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/alerts` | List all alerts (including resolved) |
| `GET` | `/api/v1/alerts/active` | List active (unresolved) alerts |
| `GET` | `/api/v1/alerts/{id}` | Get alert details |

---

## Notification Channels

Channels are silent by default — they only fire when referenced by an [Alert Trigger](#alert-triggers) or an [Escalation Policy](#escalation-policies-pro).

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/channels` | List notification channels |
| `POST` | `/api/v1/channels` | Create a channel (slack, discord, teams, webhook, email) |
| `PUT` | `/api/v1/channels/{id}` | Update a channel |
| `DELETE` | `/api/v1/channels/{id}` | Delete a channel |
| `POST` | `/api/v1/channels/{id}/test` | Send a test alert |

---

## Alert Triggers

Triggers route alerts to channels based on filters (severity, source, scope, tag).

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/alert-triggers` | List all triggers |
| `POST` | `/api/v1/alert-triggers` | Create a trigger |
| `GET` | `/api/v1/alert-triggers/{id}` | Get trigger details |
| `PUT` | `/api/v1/alert-triggers/{id}` | Update a trigger |
| `DELETE` | `/api/v1/alert-triggers/{id}` | Delete a trigger |

Filters `filter_scopes` and `filter_tags` require Pro; `filter_severities` and `filter_sources` are available on every edition.

---

## Escalation Policies (Pro)

Multi-level escalation chains for unacknowledged alerts.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/escalation-policies` | List all policies |
| `POST` | `/api/v1/escalation-policies` | Create a policy |
| `GET` | `/api/v1/escalation-policies/{id}` | Get a policy |
| `PUT` | `/api/v1/escalation-policies/{id}` | Update a policy |
| `PATCH` | `/api/v1/escalation-policies/{id}/active` | Activate / deactivate |
| `DELETE` | `/api/v1/escalation-policies/{id}` | Delete a policy |
| `POST` | `/api/v1/escalation-policies/overlap-probe` | Detect overlapping policies |
| `GET` | `/api/v1/escalation-policies/{id}/runs` | List recent runs for a policy |
| `GET` | `/api/v1/alerts/{id}/escalation-runs` | List runs for an alert |
| `GET` | `/api/v1/escalation-runs/{id}` | Get run detail and deliveries |

Endpoints return `403 edition_required` on Community.

---

## Silence Rules

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/silence` | List active silence rules |
| `POST` | `/api/v1/silence` | Create a silence rule |
| `DELETE` | `/api/v1/silence/{id}` | Cancel a silence rule |

---

## Webhooks

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/webhooks` | List webhook subscriptions |
| `POST` | `/api/v1/webhooks` | Create a webhook subscription |
| `DELETE` | `/api/v1/webhooks/{id}` | Delete a webhook subscription |
| `POST` | `/api/v1/webhooks/{id}/test` | Send a test payload |

### Delivery format

Each delivery is a `POST` with a JSON body of the shape `{type, timestamp, data}`, where `data` is the raw event payload for that `type`:

```json
{
  "type": "container.state_changed",
  "timestamp": "2026-07-19T19:49:42Z",
  "data": {
    "id": "a1b2c3d4e5f6",
    "state": "running",
    "previous_state": "exited",
    "health_status": "healthy",
    "exit_code": 0,
    "agent_id": "00000000-0000-0000-0000-000000000000"
  }
}
```

The `data` fields depend on `type` (`container.state_changed`, `endpoint.status_changed`, `heartbeat.status_changed`, `certificate.status_changed`, `alert.fired`, `alert.resolved`). Subscribe to specific types or `*` for all.

Headers on every delivery:

| Header | Value |
|--------|-------|
| `X-maintenant-Event` | the event `type` |
| `X-maintenant-Delivery` | a unique delivery UUID |
| `X-maintenant-Signature` | `sha256=<hmac>` — present only when the subscription has a secret |

When a secret is set, verify authenticity by computing `HMAC-SHA256(secret, raw_request_body)` and comparing (constant-time) against the hex digest in `X-maintenant-Signature`. The signature is computed over the exact bytes of the request body.

!!! note "Container recreation is noisy"
    A `docker compose up -d` that recreates a stack legitimately produces several `container.state_changed` deliveries per service (the old container goes `running → exited`, the new one `created → running`, plus health transitions). Filter on `data.state` / `data.previous_state` if you only care about specific transitions.

---

## Status Page (Admin)

### Components

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/status/components` | List components |
| `POST` | `/api/v1/status/components` | Create a component |
| `PUT` | `/api/v1/status/components/{id}` | Update a component |
| `DELETE` | `/api/v1/status/components/{id}` | Delete a component |

### Incidents (Pro)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/status/incidents` | List all incidents |
| `POST` | `/api/v1/status/incidents` | Create an incident |
| `PUT` | `/api/v1/status/incidents/{id}` | Update an incident |
| `DELETE` | `/api/v1/status/incidents/{id}` | Delete an incident |
| `POST` | `/api/v1/status/incidents/{id}/updates` | Add an incident update |

### Maintenance Windows (Pro)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/status/maintenance` | List maintenance windows |
| `POST` | `/api/v1/status/maintenance` | Schedule a maintenance window |
| `PUT` | `/api/v1/status/maintenance/{id}` | Update a maintenance window |
| `DELETE` | `/api/v1/status/maintenance/{id}` | Delete a maintenance window |

### Subscribers (Pro)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/status/subscribers` | List email subscribers |

### SMTP Configuration (Pro)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/status/smtp` | Get SMTP configuration |
| `PUT` | `/api/v1/status/smtp` | Update SMTP configuration |
| `POST` | `/api/v1/status/smtp/test` | Send a test email |

---

## Updates

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/updates` | List available updates (`?status=&update_type=`) |
| `GET` | `/api/v1/updates/summary` | Update summary with counts |
| `POST` | `/api/v1/updates/scan` | Trigger a manual scan |
| `GET` | `/api/v1/updates/scan/{scan_id}` | Get scan status |
| `GET` | `/api/v1/updates/dry-run` | Preview what a scan would check |
| `GET` | `/api/v1/updates/container/{container_id}` | Update details for a container |
| `POST` | `/api/v1/updates/pin/{container_id}` | Pin current version |
| `DELETE` | `/api/v1/updates/pin/{container_id}` | Unpin version |
| `GET` | `/api/v1/updates/exclusions` | List exclusions |
| `POST` | `/api/v1/updates/exclusions` | Create an exclusion |
| `DELETE` | `/api/v1/updates/exclusions/{id}` | Delete an exclusion |

---

## Security

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/security/insights` | List all network security insights |
| `GET` | `/api/v1/security/insights/{container_id}` | Get insights for a specific container |
| `GET` | `/api/v1/security/summary` | Aggregated counts by severity and type |

### Security Posture (Pro)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/security/posture` | Global infrastructure posture score |
| `GET` | `/api/v1/security/posture/containers` | Per-container posture scores (`?limit=&offset=`) |
| `GET` | `/api/v1/security/posture/containers/{id}` | Posture score for a single container |
| `POST` | `/api/v1/security/acknowledgments` | Acknowledge a finding |
| `DELETE` | `/api/v1/security/acknowledgments/{id}` | Revoke an acknowledgment |
| `GET` | `/api/v1/security/acknowledgments` | List acknowledgments (`?container_id=`) |

---

## CVE Intelligence (Pro)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/cve` | List known CVEs across all containers |
| `GET` | `/api/v1/cve/{container_id}` | List CVEs for a specific container |

---

## Risk Scoring (Pro)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/risk` | Risk scores for all containers |
| `GET` | `/api/v1/risk/{container_id}` | Risk score for a specific container |
| `GET` | `/api/v1/risk/{container_id}/history` | Risk score history |

---

## License

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/license/status` | Current license status and edition info |

---

## Dashboard

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/dashboard/sparklines` | Sparkline data for all endpoints |

---

## SSE Event Stream

Connect to the real-time event stream:

```
GET /api/v1/containers/events
```

This is a Server-Sent Events (SSE) endpoint. Each event has a `type` field and a JSON `data` payload.

### Event Types

| Event | Source | Description |
|-------|--------|-------------|
| `container.state_changed` | Container | State transition (running, stopped, etc.) |
| `container.health_changed` | Container | Health check status change |
| `container.restart_alert` | Container | Restart loop detected |
| `endpoint.check_result` | Endpoint | Check completed (up/down, response time) |
| `endpoint.alert` | Endpoint | Consecutive failure threshold reached |
| `endpoint.recovery` | Endpoint | Endpoint recovered |
| `heartbeat.pinged` | Heartbeat | Ping received |
| `heartbeat.deadline_missed` | Heartbeat | Missed deadline |
| `certificate.alert` | Certificate | Expiry warning |
| `certificate.recovery` | Certificate | Certificate renewed |
| `resource.snapshot` | Resource | New metrics snapshot |
| `resource.alert` | Resource | Threshold exceeded |
| `resource.recovery` | Resource | Usage returned to normal |
| `update.scan_started` | Update | Scan in progress |
| `update.scan_completed` | Update | Scan finished |
| `update.detected` | Update | New update found |
| `update.pinned` | Update | Version pinned |
| `update.unpinned` | Update | Version unpinned |

### Status Page SSE

The public status page has its own event stream:

```
GET /status/events
```

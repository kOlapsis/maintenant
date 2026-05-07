# Alert Engine

Unified alerts across all monitoring sources. Webhook and Discord channels included. Silence rules for planned maintenance. Exponential backoff retry on delivery failures.

---

## Alert Sources

maintenant generates alerts from every monitoring subsystem:

| Source | Events | Default Severity |
|--------|--------|------------------|
| **Container** | `restart_loop`, `health_unhealthy` | Warning |
| **Endpoint** | `consecutive_failure` | Critical |
| **Heartbeat** | `deadline_missed` | Critical |
| **Certificate** | `expiring`, `expired`, `chain_invalid` | Critical |
| **Resource** | `cpu_threshold`, `memory_threshold` | Warning |
| **Update** | `available` | Info |

---

## Notification Channels

maintenant delivers alerts to Discord, any HTTP webhook, and more. Each channel type formats the payload natively for the target platform.

### Discord

maintenant sends [Discord embeds](https://discord.com/developers/docs/resources/channel#embed-object) with severity-colored borders.

```bash
POST /api/v1/channels
{
  "name": "ops-discord",
  "type": "discord",
  "url": "https://discord.com/api/webhooks/..."
}
```

### Generic HTTP Webhook

For Slack, Teams, or any other service, use a generic webhook:

```bash
POST /api/v1/channels
{
  "name": "custom-webhook",
  "type": "webhook",
  "url": "https://your-service.example.com/alert"
}
```

maintenant sends a JSON payload with alert details to the configured URL.

### Slack, Teams & Email :material-crown:{ title="Pro" } 
maintenant Pro adds native Slack (Block Kit), Microsoft Teams (MessageCard), and Email (SMTP) channels with platform-specific formatting.

```bash
POST /api/v1/channels
{
  "name": "ops-slack",
  "type": "slack",
  "url": "https://hooks.slack.com/services/..."
}
```

---

## Channels are silent by default

A `notification_channel` represents **where** to send (an URL/email/webhook target). It does not decide *when* to send. After creating a channel, it stays silent until referenced by an [Alert Trigger](#alert-triggers) or by an [Escalation Policy](alert-escalation.md).

This decoupling enables the **reserved-escalation** pattern: a channel that only fires through an escalation policy at a delayed level (e.g. CTO email at T+1h), without receiving the initial alert.

---

## Alert Triggers

Triggers are the routing layer. Each trigger combines a filter and a list of channel destinations: when an alert matches a trigger's filter, the alert is dispatched to all of its channels.

```bash
POST /api/v1/alert-triggers
{
  "name": "Critical containers → ops",
  "filter_severities": "critical",
  "filter_sources": "container",
  "filter_scopes": "",
  "filter_tags": "",
  "enabled": true,
  "channel_ids": [3, 7]
}
```

Filters are CSV strings, combined in AND between fields and OR within a field. An empty filter matches everything. Multiple triggers can share the same channel without duplicating deliveries (the engine de-dupes per alert).

**CE vs Pro filters** :

| Filter | CE | Pro |
|---|---|---|
| `filter_severities` (e.g. `critical,warning`) | ✅ | ✅ |
| `filter_sources` (e.g. `container,endpoint`) | ✅ | ✅ |
| `filter_scopes` (e.g. `container:42,endpoint:7`) | — | ✅ |
| `filter_tags` (e.g. `prod,payments`) | — | ✅ |

Trigger CRUD endpoints :

| Method | Path |
|--------|------|
| `GET` | `/api/v1/alert-triggers` |
| `POST` | `/api/v1/alert-triggers` |
| `GET` | `/api/v1/alert-triggers/{id}` |
| `PUT` | `/api/v1/alert-triggers/{id}` |
| `DELETE` | `/api/v1/alert-triggers/{id}` |

Triggers can also be managed via MCP tools: `list_triggers`, `get_trigger`, `create_trigger`, `update_trigger`, `delete_trigger`.

> **Migration note**: previous versions used `routing_rules` attached to channels. On upgrade, those rules are auto-converted to AlertTriggers (one trigger per rule). Channels without any rule receive a generated `Default — all alerts → {channel name}` trigger to preserve the legacy broadcast behavior. The legacy `/api/v1/channels/{id}/rules*` endpoints have been removed.

---

## Testing Channels

Send a test alert to verify your channel configuration:

```bash
POST /api/v1/channels/{id}/test
```

---

## Silence Rules

Suppress alerts during planned maintenance windows. Silence rules prevent alert delivery without discarding the events.

```bash
# Create a silence rule
POST /api/v1/silence
{
  "reason": "Scheduled database maintenance",
  "starts_at": "2026-03-01T02:00:00Z",
  "ends_at": "2026-03-01T04:00:00Z",
  "matchers": {
    "source": "endpoint",
    "entity_id": "postgres-health"
  }
}

# List active silence rules
GET /api/v1/silence

# Cancel a silence rule
DELETE /api/v1/silence/{id}
```

!!! tip "Use silence rules for deployments"
    Create a silence rule before deploying to avoid alerting on expected
    container restarts and brief endpoint downtime.

---

## Retry and Backoff

When a webhook delivery fails, maintenant retries with exponential backoff:

- Attempt 1: immediate
- Attempt 2: 30 seconds
- Attempt 3: 1 minute
- Attempt 4: 2 minutes
- Attempt 5: 4 minutes

This ensures alerts are delivered even when the receiving service is temporarily unavailable.

---

## Viewing Alerts

### Active Alerts

```
GET /api/v1/alerts/active
```

Returns only currently active (unresolved) alerts.

### Alert History

```
GET /api/v1/alerts
```

Returns all alerts, including resolved ones.

### Single Alert

```
GET /api/v1/alerts/{id}
```

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/alerts` | List all alerts |
| `GET` | `/api/v1/alerts/active` | List active alerts |
| `GET` | `/api/v1/alerts/{id}` | Get alert details |
| `GET` | `/api/v1/channels` | List notification channels |
| `POST` | `/api/v1/channels` | Create a channel |
| `PUT` | `/api/v1/channels/{id}` | Update a channel |
| `DELETE` | `/api/v1/channels/{id}` | Delete a channel |
| `POST` | `/api/v1/channels/{id}/test` | Send test alert |
| `POST` | `/api/v1/channels/{id}/rules` | Create routing rule |
| `DELETE` | `/api/v1/channels/{id}/rules/{rule_id}` | Delete routing rule |
| `GET` | `/api/v1/silence` | List silence rules |
| `POST` | `/api/v1/silence` | Create silence rule |
| `DELETE` | `/api/v1/silence/{id}` | Cancel silence rule |

---

## Related

- [Container Monitoring](containers.md) — Restart loop and health check alerts
- [Endpoint Monitoring](endpoints.md) — Consecutive failure alerts
- [Heartbeat Monitoring](heartbeats.md) — Deadline missed alerts
- [Certificate Monitoring](certificates.md) — Expiry alerts
- [Resource Metrics](resources.md) — Threshold alerts

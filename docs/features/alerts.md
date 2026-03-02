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

### Pro Channels

maintenant Pro adds native Slack (Block Kit), Microsoft Teams (MessageCard), and Email (SMTP) channels with platform-specific formatting.

---

## Routing Rules

Route specific alerts to specific channels. Routing rules filter by source, severity, or entity.

```bash
POST /api/v1/channels/{id}/rules
{
  "source": "endpoint",
  "severity": "critical"
}
```

You can also route alerts per container using Docker labels:

```yaml
labels:
  maintenant.alert.channels: "ops-webhook,ops-discord"
  maintenant.alert.severity: "critical"
```

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

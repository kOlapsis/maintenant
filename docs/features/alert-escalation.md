# Alert Escalation Policies

**Availability**: Maintenant Pro

Escalation policies automatically route unacknowledged alerts through a chain of notification levels, each with a configurable delay and a distinct set of channels.

> **Pattern: reserved-escalation channel.** Since channels are silent by default (they only receive alerts when wired through an [Alert Trigger](alerts.md#alert-triggers)), you can create a channel that exists *only* for an escalation level. The directrice technique's email referenced in Level 3 of a policy, with no trigger using it, will *only* be notified after T+1h of unacknowledged escalation — never at the initial dispatch. This is the cleanest way to model "last-resort" destinations without duplicate notifications.

---

## Concept

When an alert fires and remains unacknowledged, an active escalation policy that matches the alert will start an **escalation run**. The run tracks which notification level is currently due and dispatches notifications to the configured channels at each level's delay. The chain stops as soon as the alert is acknowledged or resolved.

```
Alert fires
    │
    ├─ Level 1 (delay 5 min) → Notify #slack-oncall
    │
    ├─ Level 2 (delay 15 min, if still unacknowledged) → Page +33-6-XX
    │
    └─ Level 3 (delay 1 hour, if still unacknowledged) → Email management
```

---

## Examples

### Single-level policy

Notify the on-call Slack channel 5 minutes after an alert fires.

```json
{
  "name": "Critical pager — Level 1 only",
  "active": true,
  "filters": { "severities": ["critical"] },
  "levels": [
    { "delay_seconds": 300, "channel_ids": [1] }
  ]
}
```

### Multi-level policy

Escalate progressively over an hour.

```json
{
  "name": "Full on-call chain",
  "active": true,
  "filters": { "severities": ["critical", "warning"] },
  "levels": [
    { "delay_seconds": 300,  "channel_ids": [1] },
    { "delay_seconds": 900,  "channel_ids": [2] },
    { "delay_seconds": 3600, "channel_ids": [3] }
  ]
}
```

### Filter by tag

Only escalate alerts tagged `prod`.

```json
{
  "name": "Prod-only chain",
  "active": true,
  "filters": {
    "severities": ["critical"],
    "tags": ["prod"]
  },
  "levels": [
    { "delay_seconds": 300, "channel_ids": [1] }
  ]
}
```

---

## Interactions

### Acknowledgment stops escalation

When an alert is acknowledged, any active escalation run is immediately stopped with status `stopped_by_ack`. A notification is sent on all channels that have already received an escalation notification, informing them the alert has been acknowledged.

### Resolution stops escalation

When an alert resolves (automatically or manually), any active run is stopped with status `stopped_by_resolution`. A similar closure notification is sent on escalated channels.

### Maintenance windows suspend escalation

During a maintenance window that covers the monitored entity, escalation runs are paused (`paused_by_maintenance`) rather than stopped. When the window ends, the run resumes from where it left off — the delay is extended by the paused duration, so no level is skipped.

### Chain exhausted

If all levels fire without an acknowledgment or resolution, the run ends with status `exhausted`. A final notification is sent on the last level's channels.

---

## API

All endpoints require **Pro edition**. Requests in Community Edition return `403 edition_required`.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/escalation-policies` | Create a policy |
| `GET` | `/api/v1/escalation-policies` | List all policies |
| `GET` | `/api/v1/escalation-policies/{id}` | Get a policy |
| `PUT` | `/api/v1/escalation-policies/{id}` | Update a policy |
| `PATCH` | `/api/v1/escalation-policies/{id}/active` | Activate / deactivate |
| `DELETE` | `/api/v1/escalation-policies/{id}` | Delete a policy |
| `POST` | `/api/v1/escalation-policies/overlap-probe` | Detect overlapping policies |
| `GET` | `/api/v1/alerts/{id}/escalation-runs` | List runs for an alert |
| `GET` | `/api/v1/escalation-runs/{id}` | Get run detail + deliveries |
| `GET` | `/api/v1/escalation-policies/{id}/runs` | List recent runs for a policy |

---

## FAQ

**Can I have multiple policies matching the same alert?**
Yes. Each matching active policy starts its own independent run. The overlap-probe endpoint helps you detect potential conflicts before saving a policy.

**What happens if a channel fails to deliver?**
The delivery is recorded with `status=failed` and the error message. The run continues to the next level — a single channel failure does not stop the chain.

**What happens to runs when I downgrade from Pro to Community?**
All active policies are deactivated and all active runs are stopped with `stopped_by_edition_downgrade`. When you upgrade back to Pro, policies are automatically restored to their previous active state.

**What is the audit trail?**
Every notification attempt is recorded in the `escalation_deliveries` table with its status (`sent`, `failed`, `pending`), the channel, the level index, and timestamps. Use `GET /api/v1/escalation-runs/{id}` to retrieve the full delivery history for any run.

**When are old runs purged?**
Runs and their deliveries are purged nightly at 03:00 local time if their `ended_at` is older than 90 days. Active runs are never purged.

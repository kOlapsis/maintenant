# Alert Escalation Policies

**Availability**: Maintenant Pro

Escalation policies automatically route unacknowledged alerts through a chain of notification levels, each with a configurable delay and a distinct set of channels.

> **Pattern: reserved-escalation channel.** Since channels are silent by default (they only receive alerts when wired through an [Alert Trigger](alerts.md#alert-triggers)), you can create a channel that exists *only* for an escalation level. A manager's email referenced in Level 3 of a policy, with no trigger using it, will *only* be notified after 1 hour of unacknowledged escalation, never at the initial dispatch. This is the cleanest way to model "last-resort" destinations without duplicate notifications. Such a channel gets the escalation, acknowledgment and exhaustion notices, but no notice when the alert resolves.

---

## Concept

When an alert fires, every active escalation policy whose filters match it starts an **escalation run**. The run tracks which notification level is due next and sends the notifications of that level when its delay has passed. The chain stops as soon as the alert is acknowledged or resolved.

A level's `delay_seconds` is counted **from the start of the run** (the moment the alert fires), not from the previous level. A level with a delay of 900 fires 15 minutes after the alert, whatever the level before it had. Runs are evaluated once a minute, so a level can fire up to a minute after its delay.

```
Alert fires (T+0)
    │
    ├─ Level 1 (delay 300)  → T+5 min,  if still unacknowledged → Notify #slack-oncall
    │
    ├─ Level 2 (delay 900)  → T+15 min, if still unacknowledged → Page +33-6-XX
    │
    └─ Level 3 (delay 3600) → T+1 hour, if still unacknowledged → Email management
```

A run keeps the policy as it was when the run started. Editing or deactivating a policy changes the alerts raised afterwards, not the runs already in progress; deleting the policy stops them.

Policies only see alerts that are active. An alert raised while a matching [silence rule](alerts.md) or maintenance window is in force is silenced and starts no run. If an alert becomes more severe, policies that match its new severity start a run too; runs already started continue untouched.

---

## Examples

### Single-level policy

Notify the on-call Slack channel 5 minutes after an alert fires.

```json
{
  "name": "Critical pager: Level 1 only",
  "active": true,
  "filters": { "severities": ["critical"] },
  "levels": [
    { "delay_seconds": 300, "channel_ids": ["0195f3c8-5e2a-7b14-9a30-7d1c4f2e8a66"] }
  ]
}
```

### Multi-level policy

Escalate progressively over an hour: the levels fire 5 minutes, 15 minutes and 1 hour after the alert.

```json
{
  "name": "Full on-call chain",
  "active": true,
  "filters": { "severities": ["critical", "warning"] },
  "levels": [
    { "delay_seconds": 300,  "channel_ids": ["0195f3c8-5e2a-7b14-9a30-7d1c4f2e8a66"] },
    { "delay_seconds": 900,  "channel_ids": ["0195f3c9-0b71-7e52-8d04-3a9f6c1b2d40"] },
    { "delay_seconds": 3600, "channel_ids": ["0195f3c9-44d8-7a3c-b6e1-92f0d5a7c318"] }
  ]
}
```

### Filter by entity

Only escalate alerts about one endpoint. A scope names an alert's entity: `kind` is the entity type (`container`, `endpoint`, `heartbeat`, `certificate`) and `ref_id` is the UUID of that entity.

```json
{
  "name": "Checkout endpoint chain",
  "active": true,
  "filters": {
    "severities": ["critical"],
    "scopes": [{ "kind": "endpoint", "ref_id": "0195f3c4-11aa-7f00-8c55-2b9d0e3a4c18" }]
  },
  "levels": [
    { "delay_seconds": 300, "channel_ids": ["0195f3c8-5e2a-7b14-9a30-7d1c4f2e8a66"] }
  ]
}
```

An empty `severities` or `scopes` matches everything. An alert must satisfy both filters to match. The policy editor of the dashboard edits the name, the activation, the severities (`warning`, `critical`) and the levels; it shows the scopes of a policy read-only and keeps them when you save. Scopes are set through the API.

---

## Validation

| Field | Rule |
|-------|------|
| `name` | Required, at most 120 characters |
| `levels` | At least one |
| `levels[].delay_seconds` | Between 60 and 86400 |
| Consecutive levels | Each level is at least 60 seconds after the previous one, so delays must increase |
| `levels[].channel_ids` | At least one channel ID (a UUID) per level |

A request that breaks a rule returns `400 validation_failed` with the field in the message. The channel IDs are not checked against existing channels when the policy is saved: a level that points at a deleted channel skips it when it fires.

---

## Interactions

### Acknowledgment stops escalation

When an alert is acknowledged, any active escalation run is immediately stopped with status `stopped_by_ack`. Every channel that received a notification of the run (a delivery that is `sent` or still `pending`) gets one acknowledgment notice, once, even if it appeared in several levels. The notice reuses the recovery format of the channel, and its message is:

```
Acknowledged by <who> at <time in RFC 3339, UTC>: <original alert message>
```

When no name was recorded, the text is `Acknowledged at <time>: <original alert message>`. A run that has not reached its first level has notified nobody, so it sends no notice.

### Resolution stops escalation

When an alert resolves (automatically or manually), any active run is stopped with status `stopped_by_resolution`. **No notification is sent on resolution**: channels reached by the alert's triggers receive the usual recovery notice, but a channel that only appears in escalation levels does not.

### Maintenance windows suspend escalation

A run is checked whenever a level comes due. If a [maintenance window](status-page.md#maintenance-windows) covers the monitored entity at that moment, the run is paused (`paused_by_maintenance`) rather than stopped, and checked again about every minute. When the window ends, the run resumes and sends the level that was due; the next levels follow with the spacing you configured between them, starting from that moment. The pause therefore pushes back every remaining level by the time the run waited, and no level is skipped.

### Chain exhausted

If all levels fire without an acknowledgment or resolution, the run ends with status `exhausted` at the next evaluation. A final notice is sent on the last level's channels:

```
Escalation exhausted after <N> level(s) without acknowledgment, human action required: <original alert message>
```

### Disabled or failing channels

A delivery to a disabled channel is recorded as `failed` with the error `channel disabled`, and the run goes on with the next level.

---

## API

All endpoints require **Pro edition**. Below Pro they return `403 EDITION_REQUIRED` (with `feature: alert_escalation` and `required_edition: pro`).

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/escalation-policies` | Create a policy |
| `GET` | `/api/v1/escalation-policies` | List all policies (`?active=true` for active ones only), with `limits` |
| `GET` | `/api/v1/escalation-policies/{id}` | Get a policy |
| `PUT` | `/api/v1/escalation-policies/{id}` | Update a policy |
| `PATCH` | `/api/v1/escalation-policies/{id}/active` | Activate / deactivate, body `{"active": true}` |
| `DELETE` | `/api/v1/escalation-policies/{id}` | Delete a policy and stop its active runs |
| `POST` | `/api/v1/escalation-policies/overlap-probe` | Detect overlapping policies |
| `GET` | `/api/v1/alerts/{id}/escalation-runs` | List runs for an alert |
| `GET` | `/api/v1/escalation-runs/{id}` | Get run detail + deliveries |
| `GET` | `/api/v1/escalation-policies/{id}/runs` | List recent runs for a policy (`limit` up to 200, default 50, and `cursor`) |

---

## FAQ

**Can I have multiple policies matching the same alert?**
Yes. Each matching active policy starts its own independent run. The overlap-probe endpoint helps you detect potential conflicts before saving a policy: it takes a policy body and lists the existing policies whose filters can match the same alerts and that share at least one channel.

**What happens if a channel fails to deliver?**
The delivery is recorded with `status=failed` and the error message. The run continues to the next level: a single channel failure does not stop the chain. A delivery left `pending` for more than 2 minutes is retried while the alert is still active, and marked `abandoned` otherwise.

**What happens to runs when I downgrade from Pro?**
Going from Pro to Personal or Community deactivates all active policies and stops all active runs with `stopped_by_edition_downgrade`. When you upgrade back to Pro, policies are automatically restored to their previous active state.

**What is the audit trail?**
Every notification attempt is recorded with its status (`pending`, `sent`, `failed`, `abandoned`), the channel, the level index, and timestamps. Acknowledgment and exhaustion notices are recorded in the same list, with level index `-1` and `-3`. Use `GET /api/v1/escalation-runs/{id}` to retrieve the full delivery history for any run. A run ends with one of these statuses: `stopped_by_ack`, `stopped_by_resolution`, `stopped_by_policy_deletion`, `stopped_by_edition_downgrade` or `exhausted`.

**When are old runs purged?**
Runs and their deliveries are purged nightly at 03:00 local time if their `ended_at` is older than 90 days. Active runs are never purged.

# Alert Engine

Unified alerts across all monitoring sources. Webhook and Discord channels in every edition, email and Telegram with Personal, Slack and Teams with Pro. Silence rules for planned maintenance. Three delivery attempts per notification.

---

## Alert Sources

maintenant raises alerts from every monitoring subsystem. An alert is identified by its source, its type and the entity it concerns, and there is at most one active alert per combination. Every alert carries a severity: `critical`, `warning` or `info`.

| Source | Alert type | Entity | Severity | Resolved when |
|--------|------------|--------|----------|---------------|
| `container` | `restart_loop` | `container` | Warning, Critical at three times the restart threshold | The container runs with fewer restarts than the threshold in the last 10 minutes (rechecked every minute) |
| `container` | `health_unhealthy` | `container` | Warning | The container reports healthy again |
| `container` | `container_down` | `container` | Warning by default, or the `maintenant.alert.severity` of the container | The container is no longer `exited` or `dead` past the threshold |
| `endpoint` | `consecutive_failure` | `endpoint` | Critical | The endpoint succeeds `recovery-threshold` times in a row (default 2) |
| `endpoint` | `certificate_untrusted` | `endpoint` | Warning | A check reaches the host with a trusted certificate chain |
| `heartbeat` | `deadline_missed`, `exit_code_failure` | `heartbeat` | Critical | The next successful ping; pausing or deleting the heartbeat resolves it too |
| `certificate` | `expiring` | `certificate` | Info, Warning or Critical, by days left and issuer | The renewed certificate is past every warning threshold |
| `certificate` | `expired`, `chain_invalid`, `hostname_mismatch` | `certificate` | Critical | The first scan that no longer shows the problem |
| `certificate` | `ocsp_revoked` (Personal) | `certificate` | Critical | The first scan that no longer reports the staple as revoked |
| `resource` | `cpu_threshold`, `memory_threshold` | `container` | Warning | The metric is back under its threshold |
| `update` | `update_available` | `container` | Info, Critical for a major update | A scan no longer finds the update |
| `agent` | `disconnected` | `agent` | Warning | The agent reconnects; revoking or deleting it clears the alert |
| `host` | `os_eol` | `agent` | Warning, then Critical | The host reports a supported version |
| `security` | `dangerous_configuration` | `container` | Critical, Warning or Info, by the worst insight | The container has no insight left |
| `security` | `posture_threshold` (Personal) | `infrastructure` | Warning, Critical more than 20 points under the threshold | The score is back at or above the threshold |
| `swarm` | `replica_unhealthy` | `swarm_service` | Warning | The running replicas match the desired count again |
| `swarm` | `node_down` | `swarm_node` | Critical | The node is `ready` again |
| `swarm` | `node_drain` | `swarm_node` | Warning | The node leaves `drain` |
| `swarm` | `quorum_degraded` | `swarm_cluster` | Critical | Enough managers are ready again and the swarm has a leader |
| `swarm` | `crash_loop` | `swarm_service` | Critical | No task failure for 10 minutes |
| `swarm` | `update_rollback`, `update_stalled` | `swarm_service` | Warning | A rolling update of the service completes |
| `kubernetes` | `replica_health` | `workload` | Warning, Critical with no ready replica | The workload has all its replicas ready |
| `kubernetes` | `crash_loop` | `pod` | Critical | The pod stops crash-looping |
| `kubernetes` | `node_condition` | `node` | Critical when not Ready, Warning under memory, disk or PID pressure | The node has no problem left |

Removing the object an alert is about resolves the alert. A container labelled `maintenant.ignore`, a Swarm service labelled with it and a Kubernetes workload annotated with it raise none. The Swarm service alerts read the label on the service itself (`deploy.labels` in a stack file), not on its task containers.

### Container alerts

- **`restart_loop`** fires when a container comes back up and has restarted at least `maintenant.alert.restart_threshold` times (default 3) in the last 10 minutes.
- **`health_unhealthy`** fires only when a container that was `healthy` becomes `unhealthy`. A container that goes from `starting` straight to `unhealthy` raises nothing.
- **`maintenant.alert.severity`** sets the severity of `container_down` alone. The other container alerts keep the severities above.

### Container down

A container that stops and stays stopped raises no alert on its own: `restart_loop` needs it to come back, and `health_unhealthy` needs a `HEALTHCHECK`. Set `MAINTENANT_CONTAINER_DOWN_AFTER` to a Go duration (`5m`, `30s`, `1h`) and `container_down` fires once a container has been in `exited` or `dead` for that long, at the severity configured on the container (`warning` unless `maintenant.alert.severity` says otherwise). It resolves on its own when the container runs again.

The check is off by default (unset, or `0`), because switching it on alerts retroactively on every container already stopped. A value that is not a valid duration stops maintenant from starting, so a typo cannot leave the check silently off. A container that stops cleanly is recorded as `completed` and never counts as down, so a finished job stays quiet. Clean means exit code 0, or 143 (SIGTERM), or 137 (SIGKILL) unless the kernel's out-of-memory killer sent it: an OOM kill is a crash. The sweep runs every 30 seconds, which is the alert's resolution, not its threshold. It covers the containers of remote agents as well.

### Certificate, endpoint and heartbeat alerts

Certificate alert thresholds and the severity of `expiring` are described in [Certificate Monitoring](certificates.md#alert-thresholds). A scan that cannot connect raises no alert: alerts raised earlier resolve on the next scan that connects and no longer shows the problem. The endpoint and heartbeat thresholds are described in [Endpoint Monitoring](endpoints.md) and [Heartbeat Monitoring](heartbeats.md).

### Resource alerts

CPU and memory are independent alerts, `cpu_threshold` and `memory_threshold`, and each can fire and resolve without the other. A metric has to stay at or over its threshold for two consecutive samples (10 seconds apart on the containers of the server's own runtime) to fire, and one sample under the threshold resolves it. Only containers with thresholds configured and enabled raise them: see [Resource Metrics](resources.md).

### Update alerts

`update_available` fires when a scan finds an update for a running container. Its severity is Critical for a major update and Info for every other one, and the `maintenant.update.alert_on` label can narrow which updates raise it. See [Update Intelligence](updates.md#update-alerts).

### Agent and host alerts

An agent alert fires when a remote agent stops reporting, whether its stream dropped or it stopped sending for longer than `MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS` (60 seconds by default), and resolves on reconnection. Revoking or deleting an agent clears it instead of raising one, and nothing fires while the server itself shuts down.

A host alert fires when the operating system of a monitored host reaches the end of its free security support: warning 30 days before, escalated in place to critical the day after the date, and resolved once the host reports a newer version. See [Host OS End-of-Support](host-os.md).

### Security alerts

`dangerous_configuration` is raised in every edition, once per container, whenever the set of security insights of the container changes. Its severity follows the worst insight: critical for critical, warning for high, info for medium or low. An open alert rises with the worst insight but keeps its severity if the insight becomes milder, and it resolves when the container has no insight left. See [Network Security Insights](security.md).

`posture_threshold` needs Personal and `MAINTENANT_SECURITY_SCORE_THRESHOLD` set to a score above 0. The infrastructure score is checked every 5 minutes and each time the posture is computed: the alert fires when it falls under the threshold, and is Critical when it falls more than 20 points under.

### Swarm and Kubernetes alerts

Swarm alerts come from the Swarm cluster the server is connected to, and Kubernetes alerts from the cluster it runs in. Agents do not raise them.

- **`replica_unhealthy`** fires when a service has had fewer running replicas than desired for 5 minutes in a row. The 5 minutes start at the first service event or periodic check (every 60 seconds) that sees the shortfall. Only services in replicated mode with at least one desired replica are checked.
- **`crash_loop`** (Swarm) fires at 3 task failures in 5 minutes, counted over the tasks of every node of the swarm (read every 30 seconds), and resolves after 10 quiet minutes. A task that Swarm shut down (a rolling update, a scale-down) is not a failure, and neither is one that exited with code 0 or 143. Exit code 137 counts, because the task API does not say whether the kernel's out-of-memory killer sent it.
- **`quorum_degraded`** fires once when fewer managers than the quorum (more than half of them) are ready, or when the managers report that the swarm has no leader.
- **`update_rollback`** and **`update_stalled`** follow the rolling update status of a service: a rollback that completed, or an update that paused.
- **`replica_health`** (Kubernetes) fires after a workload has been under-replicated for 5 minutes, and escalates to Critical when no replica is ready. Jobs and workloads with no desired replica are not checked.
- **`crash_loop`** (Kubernetes) fires for a pod in `CrashLoopBackOff`, or that restarted 3 times in 10 minutes.
- **`node_condition`** is one alert per node. A node whose Ready condition is False or Unknown counts as not ready. If the nodes cannot be listed, no node alert is raised or resolved.

The Kubernetes alerts are evaluated every 30 seconds.

See [Docker Swarm](swarm.md) and the [Kubernetes guide](../guides/kubernetes.md).

---

## Life of an Alert

- **First event.** maintenant stores the alert as `active`, broadcasts `alert.fired` over SSE, sends it to the channels whose [trigger](#alert-triggers) matches and hands it to the [escalation policies](alert-escalation.md) (Pro).
- **Same event again.** While an alert is active, a further event with the same source, type and entity is ignored. One with a higher severity raises the active alert in place: new severity, message, entity name and details, a new `alert.fired` broadcast and a new notification whose message starts with "Severity raised from X to Y", and the escalation policies are evaluated again. The severity of an active alert never goes down. If the alert is already acknowledged, it keeps its acknowledgment: the notification is still sent once, but no escalation starts again.
- **Recovery.** The alert becomes `resolved` and gets a `resolved_at`. A second alert record, `resolved` with severity `info` and the recovery message, is stored and referenced by `resolved_by_id`. Channels receive a resolved notification, routed by the severity, source and entity of the original alert.
- **Entity removed.** When a container is archived, an endpoint, a heartbeat or a certificate is deleted, or a heartbeat is paused, its active alerts are resolved without a recovery record and without a notification (the dashboard still receives `alert.resolved`). At startup, maintenant also resolves any active alert whose container, agent, heartbeat, endpoint or certificate no longer exists, so alerts left behind by an earlier run cannot linger.
- **Silenced.** An alert that fires while a [silence rule](#silence-rules) or a maintenance window matches is stored as `silenced` and broadcast as `alert.silenced`. It is not sent and not escalated.
- **Retention.** Once a day, resolved and silenced alerts are deleted 90 days after their resolution (or after their creation when they have no resolution time). An active alert is never deleted.

---

## Notification Channels

A channel is a destination. maintenant delivers alerts to Discord, Telegram, email, Slack, Teams and any HTTP webhook, and each channel type formats the payload natively for its platform.

| Type | Edition | `url` holds | Payload |
|------|---------|-------------|---------|
| `webhook` (default) | Community | HTTPS URL | maintenant JSON |
| `discord` | Community | Discord webhook URL | Embed with a severity-colored border |
| `email` | Personal | Recipient address | Plain-text mail |
| `telegram` | Personal | Chat id | HTML message |
| `slack` | Pro | Slack incoming webhook URL | Block Kit |
| `teams` | Pro | Teams webhook URL | MessageCard |

### Channel fields

| Field | Description |
|-------|-------------|
| `name` | Required and unique. A duplicate answers `409 DUPLICATE_NAME`. |
| `type` | One of the types above. Defaults to `webhook`. |
| `url` | Required. The destination, as the table says. |
| `headers` | Optional, for the HTTP types. A string holding a JSON object of HTTP headers added to every request, for instance `"{\"Authorization\": \"Bearer …\"}"`. A value that is not such an object is ignored. |
| `secret` | Optional, write-only. The credential of the channel: the Telegram bot token. |
| `config` | Optional JSON object of non-secret settings: `thread_id` for Telegram. |
| `enabled` | Defaults to `true`. A disabled channel keeps its settings and receives nothing. |

A channel is returned with its `id`, these fields, `has_secret` (the secret itself is never returned), `suspended` and `required_edition`. `GET /api/v1/channels` adds `health`: `failing` when the last delivery through the channel failed, `healthy` otherwise.

Updating a channel with `PUT` changes only the fields you send. A `secret` can be replaced but not cleared: delete the channel instead. Creating, updating, enabling or testing a channel of a type the running edition does not open answers `403 EDITION_REQUIRED`.

### Discord

maintenant sends [Discord embeds](https://discord.com/developers/docs/resources/channel#embed-object) with severity-colored borders, the source, severity and entity as fields, and for update alerts the update and rollback commands.

```bash
POST /api/v1/channels
{
  "name": "ops-discord",
  "type": "discord",
  "url": "https://discord.com/api/webhooks/..."
}
```

### Generic HTTP Webhook

For any HTTP endpoint that accepts JSON:

```bash
POST /api/v1/channels
{
  "name": "custom-webhook",
  "type": "webhook",
  "url": "https://your-service.example.com/alert",
  "headers": "{\"Authorization\": \"Bearer …\"}"
}
```

#### Webhook payload

maintenant sends one `POST` per notification, with `Content-Type: application/json` and the channel's own headers. Any 2xx answer counts as delivered; anything else, or no answer within 10 seconds, is retried as described in [Delivery and Retries](#delivery-and-retries).

| Field | Type | Description |
|-------|------|-------------|
| `event` | string | `alert.fired`, `alert.resolved` or `test` |
| `timestamp` | string | When the notification was sent, RFC 3339 UTC |
| `alert.id` | string | UUID of the alert. The resolved notification carries the id of the alert it resolves. |
| `alert.agent_id` | string | UUID of the host the alert belongs to. The server's own runtime is `00000000-0000-0000-0000-000000000000`; remote hosts are listed by `GET /api/v1/agents`. |
| `alert.url` | string | Link to the alert in the UI, `<MAINTENANT_BASE_URL>/alerts/history?alert=<id>`. Set `MAINTENANT_BASE_URL` to the address your team opens: without it, the link is built from the listen address. Absent when that address names no reachable host (`0.0.0.0`, `::` or an empty host), and on `test` notifications. |
| `alert.source` | string | `container`, `endpoint`, `heartbeat`, `certificate`, `resource`, `update`, `security`, `agent`, `host`, `swarm` or `kubernetes` (see [Alert Sources](#alert-sources)) |
| `alert.alert_type` | string | Source-specific type, such as `consecutive_failure` or `restart_loop` |
| `alert.severity` | string | `critical`, `warning` or `info` |
| `alert.status` | string | `active` on `alert.fired`, `resolved` on `alert.resolved` |
| `alert.message` | string | Human-readable description; on `alert.resolved`, the recovery message |
| `alert.entity_type`, `alert.entity_id`, `alert.entity_name` | string | The monitored object: its type, UUID and display name |
| `alert.fired_at` | string | When the condition was detected, RFC 3339 UTC |
| `alert.created_at` | string | When the alert was stored |
| `alert.details` | object | Source-specific values (target, failure count, threshold, last error...). Absent when the source has none. |
| `alert.resolved_at`, `alert.resolved_by_id` | string | On `alert.resolved` only: the recovery time and the UUID of the recovery record |
| `alert.acknowledged_at`, `alert.acknowledged_by`, `alert.escalated_at` | string | Present once the alert has been acknowledged or escalated |

An alert that fires:

```json
{
  "event": "alert.fired",
  "alert": {
    "id": "0198b1c2-7a3e-7f00-9c11-2d4e5f60a7b8",
    "agent_id": "0198a0f4-1c2d-7e3f-8a4b-5c6d7e8f9a0b",
    "url": "https://now.example.com/alerts/history?alert=0198b1c2-7a3e-7f00-9c11-2d4e5f60a7b8",
    "source": "endpoint",
    "alert_type": "consecutive_failure",
    "severity": "critical",
    "status": "active",
    "message": "Endpoint https://api.example.com/health failed 3 consecutive checks",
    "entity_type": "endpoint",
    "entity_id": "0198b1c2-8b4f-7a11-8d22-3e5f6071b8c9",
    "entity_name": "api",
    "fired_at": "2026-03-01T02:00:00Z",
    "created_at": "2026-03-01T02:00:00Z",
    "details": {
      "target": "https://api.example.com/health",
      "failures": 3,
      "threshold": 3,
      "last_error": "connection refused"
    }
  },
  "timestamp": "2026-03-01T02:00:00Z"
}
```

The same alert when it recovers. The id, the host, the entity, `fired_at` and `details` are those of the original alert; the severity and the message come from the recovery:

```json
{
  "event": "alert.resolved",
  "alert": {
    "id": "0198b1c2-7a3e-7f00-9c11-2d4e5f60a7b8",
    "agent_id": "0198a0f4-1c2d-7e3f-8a4b-5c6d7e8f9a0b",
    "url": "https://now.example.com/alerts/history?alert=0198b1c2-7a3e-7f00-9c11-2d4e5f60a7b8",
    "source": "endpoint",
    "alert_type": "consecutive_failure",
    "severity": "info",
    "status": "resolved",
    "message": "Endpoint https://api.example.com/health recovered after 2 consecutive successes",
    "entity_type": "endpoint",
    "entity_id": "0198b1c2-8b4f-7a11-8d22-3e5f6071b8c9",
    "entity_name": "api",
    "fired_at": "2026-03-01T02:00:00Z",
    "created_at": "2026-03-01T02:00:00Z",
    "resolved_at": "2026-03-01T02:04:30Z",
    "resolved_by_id": "0198b1c6-9d01-7c22-b733-4f6a7182c9da",
    "details": {
      "target": "https://api.example.com/health",
      "failures": 3,
      "threshold": 3,
      "last_error": "connection refused"
    }
  },
  "timestamp": "2026-03-01T02:04:30Z"
}
```

A few cases send `alert.fired` again for the same `id`: a [severity raise](#life-of-an-alert) (the message starts with "Severity raised from X to Y") and each level of an [escalation policy](alert-escalation.md) that targets the channel. A receiver that must act once per alert keys on `id`. The **Test** button sends `event: "test"` with an `alert` whose `id`, `agent_id` and `entity_id` are empty and whose `source` and `alert_type` are `test`.

The payload is maintenant's own: Slack and Teams expect theirs, which is why they are native channels. To hand alerts to an AI agent, see [AI Agents](../guides/ai-agents.md).

### Email (SMTP) :material-star-four-points:{ title="Personal" }

Email goes through your own SMTP server, set with environment variables. The channel's `url` is the recipient address, one per channel.

| Variable | Default | Description |
|----------|---------|-------------|
| `MAINTENANT_SMTP_HOST` | (none) | SMTP server. Without it, email deliveries fail with `SMTP not configured`. |
| `MAINTENANT_SMTP_PORT` | `587` | Port. `465` uses TLS from the first byte; any other port starts in clear. |
| `MAINTENANT_SMTP_USERNAME`, `MAINTENANT_SMTP_PASSWORD` | (none) | Credentials, sent with AUTH PLAIN when a username is set. |
| `MAINTENANT_SMTP_FROM` | `maintenant@localhost` | Sender address. |

On a port other than 465, maintenant upgrades the connection with STARTTLS whenever the server announces it, and gives up the delivery if the upgrade fails. A server that does not announce STARTTLS receives the mail in clear, and with credentials set the send then fails unless the server is `localhost`, because the credentials would travel in clear. Server certificates are checked against the system roots plus `MAINTENANT_CA_CERT`. A send gives up after 30 seconds.

The subject reads `[maintenant] ALERT: <message>`, `RESOLVED` or `TEST`, and the body is plain text. The same SMTP server sends the status page mails.

### Telegram :material-star-four-points:{ title="Personal" }

A native Telegram channel: you supply a bot token and a chat id, never a URL.
The destination is fixed, so nothing has to be worked around: neither the
payload format a generic webhook gets wrong, nor the SSRF guard that refuses a
local relay. Available from the Personal edition.

**1. Create the bot.** Message [@BotFather](https://t.me/BotFather), send
`/newbot`, follow the questions. It hands back a token shaped like
`8123456789:AAF-…`. Treat it as a password: it is the bot.

**2. Find the chat id.**

- *Private chat*: message the bot once, then read `message.chat.id` from
  `https://api.telegram.org/bot<token>/getUpdates`.
- *Group or channel*: add the bot to it, post a message, same call. The id is
  **negative**, often prefixed `-100`. Paste it exactly as it appears.

A public channel can also be addressed by its `@username`. The bot's own
`@name` never works as a destination.

**3. Create the channel.**

```bash
POST /api/v1/channels
{
  "name": "oncall-telegram",
  "type": "telegram",
  "url": "-1001234567890",
  "secret": "8123456789:AAF-...",
  "config": { "thread_id": "42" }
}
```

`config.thread_id` is optional and only applies to groups organised in topics;
leave it out and messages land in the general thread.

The token is write-only. It is never returned by the API, never written to a
log, and never appears in an error message; responses carry `has_secret` so the
interface can say a token is on file without holding it. An update that omits
`secret` keeps the stored one. It is stored in the database as it is, so treat
the database and its backups as secrets.

Messages use Telegram's HTML formatting: a severity emoji and the entity on the
first line, so a phone notification says what happened before you open it. A
recovery arrives as a separate message, marked ✅. Anything over Telegram's
4096-character limit is truncated with a visible marker rather than rejected.

When a send fails, the log and the delivery record carry Telegram's own words
(`chat not found`, `bot was kicked from the supergroup chat`, `bot can't
initiate conversation with a user`) because those name the fix, where an HTTP
code does not. On a rate limit, the retry waits at least as long as Telegram
asked.

### Slack & Teams :material-crown:{ title="Pro" }

Native Slack (Block Kit) and Microsoft Teams (MessageCard) channels, with
platform-specific formatting. The `url` is the incoming webhook URL.

```bash
POST /api/v1/channels
{
  "name": "ops-slack",
  "type": "slack",
  "url": "https://hooks.slack.com/services/..."
}
```

### Webhook destinations and the SSRF guard

The HTTP channel types (`webhook`, `discord`, `slack`, `teams`) only accept destinations that cannot be used to reach your internal network:

- The URL must use `https`.
- The host must not resolve to a loopback, private (RFC 1918 and ULA), link-local, carrier-grade NAT (`100.64.0.0/10`), unspecified or multicast address.
- The check runs when the channel is saved, and again at every connection, redirects included, so a host that changes its DNS answer afterwards is refused as well.

A refused destination answers `400 VALIDATION_ERROR` with the reason. To deliver to an internal relay on a development setup, set `MAINTENANT_ALLOW_PRIVATE_WEBHOOKS` to a true value (`1`, `true`, `yes` or `on`): it lifts both the `https` requirement and the address guard, for channels and event webhooks alike. Telegram always calls `api.telegram.org`, and email goes to the SMTP server you configured, so neither depends on it.

### When the edition drops

If a licence lapses or is downgraded, the channels of the types the running edition no longer opens are kept but **suspended**: email and Telegram below Personal, Slack and Teams below Pro.

- Nothing is delivered through a suspended channel. Each delivery is recorded as `suspended` with the required edition as its error, and one warning per channel is logged.
- `GET /api/v1/channels` marks it with `suspended: true` and its `required_edition`. The interface shows a banner on every page and a **Suspended** badge on the channel.
- `GET /api/v1/edition` lists the enabled suspended channels in `suspended_channels`.
- Creating, editing, enabling or testing such a channel is refused with `403 EDITION_REQUIRED`. Disabling it and deleting it stay open: an expired licence never leaves you unable to silence a channel.

Nothing is lost: once the edition is back, the channels deliver again. Move the alerts to a channel your edition includes if you stay on the lower edition.

---

## Channels are silent by default

A `notification_channel` represents **where** to send (a URL, an email address, a chat). It does not decide *when* to send. After creating a channel, it stays silent until it is referenced by an [Alert Trigger](#alert-triggers) or by an [Escalation Policy](alert-escalation.md) (Pro).

This decoupling enables the **reserved-escalation** pattern: a channel that only fires through an escalation policy at a delayed level (e.g. CTO email at T+1h), without receiving the initial alert.

---

## Alert Triggers

Triggers are the routing layer. Each trigger combines a filter and a list of channel destinations: when an alert matches a trigger's filter, the alert is dispatched to all of its channels.

```bash
POST /api/v1/alert-triggers
{
  "name": "Critical containers to ops",
  "filter_severities": "critical",
  "filter_sources": "container",
  "filter_scopes": "",
  "enabled": true,
  "notify_on_resolve": true,
  "channel_ids": ["0198b1c2-7a3e-7f00-9c11-2d4e5f60a7b8", "0198b1c2-8b4f-7a11-8d22-3e5f6071b8c9"]
}
```

| Field | Description |
|-------|-------------|
| `name` | Required, at most 120 characters, unique (`409 name_conflict`). |
| `filter_severities` | Comma-separated severities: `critical`, `warning`, `info`. |
| `filter_sources` | Comma-separated sources: `container`, `endpoint`, `heartbeat`, `certificate`, `resource`, `update`, `security`, `agent`, `host`, `swarm`, `kubernetes`. |
| `filter_scopes` | Comma-separated `entity_type:entity_id` pairs, such as `container:0198b1c2-…`. The entity types are in the table of [alert sources](#alert-sources). The id is the one the alert carries in `entity_id`: a UUID for most entities, a Docker ID for Swarm objects, and for Kubernetes `namespace/Kind/name` for a workload (`production/Deployment/api`), `namespace/name` for a pod and the node name for a node. |
| `enabled` | Defaults to `true` on creation and keeps its value on update when omitted. |
| `notify_on_resolve` | Defaults to `true` and keeps its value on update when omitted. Set it to `false` for a channel that should only receive failures. |
| `channel_ids` | Required, at least one channel UUID, each of an existing channel. |

Filters combine in AND between fields and OR within a field. An empty filter matches everything. A disabled channel is skipped. Multiple triggers can share the same channel without duplicating deliveries: a channel receives an alert once, however many triggers match. A `PUT` needs the `name` and the `channel_ids` again, and a filter left out is cleared. Unknown fields are ignored. An unknown trigger answers `404 trigger_not_found`, and a request that breaks a rule `400 validation_failed`.

A trigger relays both the initial alert and its recovery. The recovery of an alert is matched with the source, severity and scope of the original alert, so a trigger on `critical` receives the recovery of a critical alert.

**Filters by edition:**

| Filter | Community | Personal and Pro |
|---|---|---|
| `filter_severities` | ✅ | ✅ |
| `filter_sources` | ✅ | ✅ |
| `filter_scopes` | — | ✅ |

On Community, setting a scope filter answers `403 EDITION_REQUIRED` (feature `alert_advanced_filters`, required edition `personal`). A trigger that already holds a scope filter, created on a higher edition, keeps it and can be saved again with that same filter; only a new or changed scope is refused.

Trigger CRUD endpoints:

| Method | Path |
|--------|------|
| `GET` | `/api/v1/alert-triggers` |
| `POST` | `/api/v1/alert-triggers` |
| `GET` | `/api/v1/alert-triggers/{id}` |
| `PUT` | `/api/v1/alert-triggers/{id}` |
| `DELETE` | `/api/v1/alert-triggers/{id}` |

Triggers can also be managed via MCP tools: `list_triggers`, `get_trigger`, `create_trigger`, `update_trigger`, `delete_trigger`. Channels have the matching set: `list_channels`, `get_channel`, `create_channel`, `update_channel`, `delete_channel`, `test_channel`.

---

## Testing Channels

Send a test alert to verify your channel configuration:

```bash
POST /api/v1/channels/{id}/test
```

The test sends one notification (no retry) for an alert of source `test`, and answers `200` whether the destination accepted it or not: `{"status": "delivered", "response_code": 204}` when it did, `{"status": "failed", "error": "…"}` otherwise. For an email or Telegram channel `response_code` is `200`. An unknown channel answers `404 NOT_FOUND`, and a channel whose type the edition does not open `403 EDITION_REQUIRED`.

---

## Silence Rules

Suppress alerts during planned maintenance. Silence rules prevent notification without discarding the events: the alerts are still stored, as `silenced`.

```bash
# Create a silence rule, effective immediately
POST /api/v1/silence
{
  "source": "endpoint",
  "entity_type": "endpoint",
  "entity_id": "0198b1c2-8b4f-7a11-8d22-3e5f6071b8c9",
  "reason": "Scheduled database maintenance",
  "duration_seconds": 7200
}

# List every silence rule
GET /api/v1/silence

# List only the rules still in effect
GET /api/v1/silence?active=true

# Cancel a silence rule
DELETE /api/v1/silence/{id}
```

| Field | Description |
|-------|-------------|
| `duration_seconds` | Required, greater than 0. The rule starts when it is created and ends after this many seconds. |
| `source` | Optional. Only alerts of this source. |
| `entity_type` | Optional. Only alerts about this type of entity. |
| `entity_id` | Optional. Only alerts about this entity. |
| `reason` | Optional free text. |

An alert is silenced when it matches every field the rule sets, and a rule that sets none silences everything. The list answers `{"rules": […]}` with, for each rule, `starts_at`, `duration_seconds`, `expires_at` and `is_active`. Cancelling a rule keeps it in the full list, inactive.

A silence only affects alerts that fire while it is in effect. It does not mute an alert that is already active, and an alert that fired during the silence stays silenced after the rule ends. Silenced alerts are listed by `GET /api/v1/alerts?status=silenced`.

!!! tip "Use silence rules for deployments"
    Create a silence rule before deploying to avoid alerting on expected
    container restarts and brief endpoint downtime. To plan a silence ahead of
    time, schedule a [maintenance window](status-page.md) (Pro): it silences the
    alerts of the monitors of its components for its duration.

---

## Acknowledging Alerts

Acknowledging an alert tells maintenant that someone has taken it. It does not resolve it: the alert stays active until its condition clears.

```bash
POST /api/v1/alerts/{id}/acknowledge
{
  "acknowledged_by": "alice"
}
```

`acknowledged_by` is required (`400 INVALID_REQUEST` without it). The answer is the alert with its `acknowledged_at` and `acknowledged_by`. Acknowledging stores the acknowledgment once, broadcasts `alert.acknowledged`, removes the alert from `GET /api/v1/alerts/active` and stops its [escalation](alert-escalation.md). Only an active alert that is not yet acknowledged can be acknowledged: anything else answers `409 CONFLICT`, an unknown id `404 NOT_FOUND`.

The **Active Alerts** list of the Alerts page has an **Acknowledge** button, which records the acknowledgement as `operator`. The MCP tool `acknowledge_alert` goes through the same path, with `mcp` as the default name. Acknowledging every security insight of a container in the security posture (Personal) acknowledges its `dangerous_configuration` alert as well.

---

## Delivery and Retries

Notifications are delivered by ten workers, each fed by its own queue of 256 notifications. If a queue is full, the notification is dropped and a warning is logged.

The notifications of one alert to one channel always go through the same worker, so they reach the channel in order: a recovery never overtakes the alert it resolves, even when the alert is still being retried. Other alerts and other channels are spread over the ten workers.

A delivery makes up to **three attempts**: one at once, one after 1 second and one after 5 seconds. A sender can lengthen the wait, never shorten it: a Telegram rate limit raises it to the delay Telegram asked for. An HTTP attempt times out after 10 seconds and an email attempt after 30. Every failed attempt is logged with its error. After the third failure the delivery is recorded as `failed` and is not retried later. An email channel with no SMTP server configured fails at once, without retries.

Deliveries are kept in the `notification_deliveries` table, which no API lists. A delivery is `pending`, `delivered`, `failed` or `suspended`, and a channel whose last delivery failed shows `health: failing`.

---

## Viewing Alerts

### Active Alerts

```
GET /api/v1/alerts/active
```

Returns the active, not yet acknowledged alerts, grouped by severity: `{"critical": […], "warning": […], "info": […]}`. Silenced alerts are not active.

### Alert History

```
GET /api/v1/alerts?limit=50&status=active&severity=critical
```

Returns alerts, resolved ones included, newest first, as `{"alerts": […], "has_more": true}`.

| Parameter | Description |
|-----------|-------------|
| `limit` | 1 to 200, default 50. Another value answers `400 INVALID_PARAM`. |
| `before` | An RFC 3339 time: only alerts fired before it. Pass the `fired_at` of the last alert you received to read the next page while `has_more` is `true`. A value that is not a time answers `400 INVALID_PARAM`. |
| `before_id` | The `id` of the last alert you received, used together with `before`. Alerts fired in the same second are then neither skipped nor repeated between pages. Without `before` it answers `400 INVALID_PARAM`. |
| `source` | Only this source. |
| `severity` | Only this severity. |
| `status` | `active`, `resolved` or `silenced`. |

Each resolution also stores a recovery record, so the list holds both the original alert and its `resolved` recovery. In these responses `details` is a string holding JSON, while the `alert.fired`, `alert.resolved` and `alert.silenced` events and the webhook bodies carry it as an object.

### Single Alert

```
GET /api/v1/alerts/{id}
```

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/alerts` | List alerts, paginated |
| `GET` | `/api/v1/alerts/active` | List active alerts, grouped by severity |
| `GET` | `/api/v1/alerts/{id}` | Get alert details |
| `POST` | `/api/v1/alerts/{id}/acknowledge` | Acknowledge an alert |
| `GET` | `/api/v1/channels` | List notification channels |
| `POST` | `/api/v1/channels` | Create a channel |
| `PUT` | `/api/v1/channels/{id}` | Update a channel |
| `DELETE` | `/api/v1/channels/{id}` | Delete a channel |
| `POST` | `/api/v1/channels/{id}/test` | Send test alert |
| `GET POST` | `/api/v1/alert-triggers` | List / create triggers |
| `GET PUT DELETE` | `/api/v1/alert-triggers/{id}` | Manage a trigger |
| `GET` | `/api/v1/silence` | List silence rules (`?active=true` for those in effect) |
| `POST` | `/api/v1/silence` | Create silence rule |
| `DELETE` | `/api/v1/silence/{id}` | Cancel silence rule |
| `GET POST` | `/api/v1/escalation-policies` | List / create policies (Pro) |
| `POST` | `/api/v1/escalation-policies/overlap-probe` | Detect overlapping policies before saving (Pro) |
| `GET PUT DELETE` | `/api/v1/escalation-policies/{id}` | Manage a policy (Pro) |
| `PATCH` | `/api/v1/escalation-policies/{id}/active` | Enable or disable a policy (Pro) |
| `GET` | `/api/v1/escalation-policies/{id}/runs` | List the runs of a policy (Pro) |
| `GET` | `/api/v1/escalation-runs/{run_id}` | Get a run (Pro) |
| `GET` | `/api/v1/alerts/{alert_id}/escalation-runs` | List runs for an alert (Pro) |

Alerts are also broadcast on the SSE event stream as `alert.fired`, `alert.resolved`, `alert.silenced` and `alert.acknowledged`, and the changes to channels, triggers and silence rules as `channel.created`, `channel.updated`, `channel.deleted`, `trigger.created`, `trigger.updated`, `trigger.deleted`, `silence.created` and `silence.cancelled`. `alert.fired` and `alert.resolved` can also be delivered to a webhook subscription (see the [API reference](../api/reference.md#webhooks)). The MCP tool `list_alerts` returns the active alerts that are not yet acknowledged, as `GET /api/v1/alerts/active` does, or with `active_only` set to `false` the last 100 alerts, acknowledged, resolved and silenced ones included. `acknowledge_alert` acknowledges one.

---

## Related

- [Alert Escalation](alert-escalation.md): Pro, multi-level escalation chains
- [Container Monitoring](containers.md): Restart loop and health check alerts
- [Endpoint Monitoring](endpoints.md): Consecutive failure alerts
- [Heartbeat Monitoring](heartbeats.md): Deadline missed alerts
- [Certificate Monitoring](certificates.md): Expiry alerts
- [Resource Metrics](resources.md): Threshold alerts
- [Update Intelligence](updates.md): Update alerts
- [Host OS End-of-Support](host-os.md): `host` / `os_eol` alerts
- [Network Security Insights](security.md): `dangerous_configuration` and `posture_threshold` alerts
- [Docker Swarm](swarm.md) and the [Kubernetes guide](../guides/kubernetes.md): Swarm and Kubernetes alerts
- [Status Page](status-page.md#maintenance-windows): Maintenance windows that silence alerts (Pro)

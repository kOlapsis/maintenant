# API Reference

The REST API lives under `/api/v1/`. Requests and responses are JSON unless a route says otherwise. Besides the REST API, the server exposes the public ping endpoints (`/ping/`), the public status page (`/status/`), the MCP endpoint (`/mcp`) with its OAuth routes, and the Server-Sent Events streams. Every route is listed on this page.

---

## Conventions

### Authentication

maintenant has no built-in authentication for the dashboard or the REST API. Every `/api/` route answers anyone who can reach the listener, so put the instance behind a reverse proxy or an authentication proxy (see [Security](../security.md)). Three surfaces are open on purpose:

- `/ping/{uuid}`: heartbeat pings. The UUID is the only secret.
- `/status/`: the public status page, its JSON snapshot, its event stream and its subscription links.
- `/mcp`: protected by its own OAuth 2.0 flow (see [MCP and OAuth routes](#mcp-and-oauth-routes)).

The health check `/api/v1/health` is an ordinary `/api/` route, so it is exposed like the others. It reports the version, the runtime and the storage state.

### Errors

A failed request answers with a JSON body of this shape:

```json
{
  "error": {
    "code": "QUOTA_EXCEEDED",
    "message": "The Community edition is limited to 10 endpoints.",
    "resource": "endpoints",
    "limit": 10,
    "required_edition": "personal"
  }
}
```

`code` and `message` are always present. The other fields (`feature`, `resource`, `limit`, `required_edition`, `window`, `max_window`) only appear on edition and quota refusals, so a client never has to parse the message.

| Code | Status | Meaning |
|------|:------:|---------|
| `EDITION_REQUIRED` | 403 | The running edition does not open this feature. Carries `feature` (the capability) and `required_edition`. A history window the edition does not open also carries `window` and `max_window` (the largest window the edition opens). |
| `QUOTA_EXCEEDED` | 403 | A creation would pass the edition cap on endpoints, heartbeat monitors, certificate monitors or status page components. Carries `resource`, `limit` and `required_edition`, the lowest edition that lifts the cap. |
| `HOST_LIMIT_REACHED` | 409 | `POST /api/v1/agents/enrollment-tokens` when the edition's cap on remote agents is reached (20 on Personal). Same fields as `QUOTA_EXCEEDED`. |
| `STORAGE_UNAVAILABLE` | 503 | The database is unreachable. Only the routes that read through the store answer this, the others answer `INTERNAL_ERROR`. Clients should retry. |
| `INTERNAL_ERROR` | 500 | Unexpected failure, including a recovered panic. |
| `CROSS_ORIGIN_REFUSED` | 403 | A browser sent a write (`POST`, `PUT`, `PATCH`, `DELETE`) from an origin the server does not trust. See [Cross-origin writes](#cross-origin-writes). |
| `DEMO_MODE` | 403 | The instance runs in demo mode. See [Demo mode](#demo-mode). |
| `rate_limited` | 429 | The client IP went over a rate limit. Carries a `Retry-After` header. |

The remaining codes are specific to a route and are listed with it. Most are upper case (`INVALID_JSON`, `NOT_FOUND`), but several handlers use lower case codes: webhooks, risk scoring, alert triggers, escalation policies, the status page administration (whose unexpected failures answer `internal` instead of `INTERNAL_ERROR`) and personalization, the public status page routes and `rate_limited`. Codes are given here exactly as the server returns them.

Three answers are not JSON:

- A route the server did not register (a feature that is not wired, such as the outbound heartbeats in demo mode) and a wrong method on a known path answer the plain-text `404` and `405` of the Go router.
- A request still running after 10 seconds is cut with `503` and the plain-text body `request timeout`. Streams are exempt. The work the request started can still finish, for example a synchronous certificate scan.
- The OAuth routes answer with the RFC 6749 shape (`{"error": "...", "error_description": "..."}`).

### Editions and quotas

`GET /api/v1/edition` returns the edition table the server applies, so a client never needs its own copy. Editions are ordered Community, Personal, Pro.

| Edition | Capabilities opened |
|---------|---------------------|
| Community | `alert_routing`, `swarm_dashboard`, `k8s_cluster`, `resource_history` |
| Personal | Community, plus `multihost`, `cve_enrichment`, `risk_scoring`, `changelog`, `incidents`, `smtp`, `alert_advanced_filters`, `security_posture`, `ocsp_stapling`, `telegram` |
| Pro | Personal, plus `slack`, `teams`, `alert_escalation`, `alert_entity_routing`, `maintenance_windows`, `subscribers`, `personalization` |

| Resource | Community | Personal | Pro |
|----------|:---------:|:--------:|:---:|
| Standalone endpoints | 10 | unlimited | unlimited |
| Heartbeat monitors | 5 | unlimited | unlimited |
| Standalone certificate monitors | 5 | unlimited | unlimited |
| Status page components | 3 | unlimited | unlimited |
| Remote agents | 0 | 20 | unlimited |
| History window (resources) | 7 days | 30 days | 90 days |

The caps are read from the running edition at each creation, so a licence change applies without a restart. The tables under each section give the edition a route needs in the **Edition** column. A dash means the route is open in every edition.

### Host scope

The local runtime is itself an agent, identified by the sentinel id `00000000-0000-0000-0000-000000000000`. Routes that list monitored entities accept an `agent_id` query parameter: `local` selects the server's own runtime, any other value selects that agent. Without it, list routes cover every host, with these exceptions: `GET /api/v1/resources/summary` defaults to the local host, and the single-entity Kubernetes routes (`workloads/{id}`, `pods/{namespace}/{name}`) look in the local runtime unless `agent_id` is given. The parameter is never validated: an unknown agent yields an empty list or a `404`.

### Limits

- **Rate limits**: per client IP, in token buckets. `/api/` allows 50 requests per second (burst 200). `/ping/`, `/status/`, `/mcp` and `/oauth/` share a tighter bucket of 10 requests per second (burst 20). Status page subscriptions are limited to 5 per hour. Client addresses behind a reverse proxy are only read from forwarded headers for the proxies listed in `MAINTENANT_TRUSTED_PROXIES`.
- **Body size**: 1 MiB for `/api/` by default (`MAINTENANT_MAX_BODY_SIZE`), whatever the method. A value that is not a positive whole number of bytes stops the server at startup. Past the limit the JSON decoding fails and the route answers its invalid body error. Public `/status/` routes accept 4 KiB.
- **Timeout**: 10 seconds for every route except the event streams.

### Cross-origin writes

The API refuses a browser write that comes from another origin, unless that origin is listed in `MAINTENANT_CORS_ORIGINS`: the answer is `403 CROSS_ORIGIN_REFUSED`. Same-origin requests, requests whose `Sec-Fetch-Site` is `none` and requests without `Origin` and `Sec-Fetch-Site` headers (curl, scripts) pass. A browser old enough to send no `Sec-Fetch-Site` header is refused behind a proxy that rewrites the `Host` header. The wildcard `*` in `MAINTENANT_CORS_ORIGINS` allows cross-origin reads but no longer cross-origin writes. `/ping/`, `/status/`, `/mcp` and `/oauth/` are not concerned.

### Demo mode

A demo build of maintenant is read-only. Every request other than `GET`, `HEAD` and `OPTIONS` answers `403 DEMO_MODE` (only `POST /api/v1/escalation-policies/overlap-probe`, which changes nothing, is let through). `/ping/`, `/mcp`, `/oauth/` and `/.well-known/oauth-*` answer `403 DEMO_MODE` for every method, and the outbound heartbeat routes are not registered. A request carrying the `X-Maintenant-Demo-Token` header with the value of `MAINTENANT_DEMO_TOKEN` bypasses the guard. `GET /api/v1/edition` reports `"demo": true`.

---

## Health and instance information

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/health` | Health check | — |
| `GET` | `/api/v1/runtime/status` | Active runtime and its connection state | — |
| `GET` | `/api/v1/edition` | Edition, features, quotas and tiers | — |
| `GET` | `/api/v1/license/status` | Licence state | — |

### Health

`GET /api/v1/health` always answers `200`:

```json
{ "status": "ok", "version": "1.8.0", "runtime": { "name": "docker", "connected": true }, "storage": { "engine": "sqlite", "connected": true, "peers": 0 } }
```

`version`, `runtime` and `storage` are left out when they are not known. The `storage` object reports the engine backing this instance, whether it answers, and how many other instances beat on the same database. `engine` is `sqlite` or `postgres`. It never carries the connection string, the host or any credential.

!!! warning "This endpoint answers 200 during a database outage"

    An unreachable database is reported in `storage.connected`, not in the HTTP
    status. `/api/v1/health` is the target of the Kubernetes liveness and
    startup probes: failing it on a blip would restart the instance exactly when
    the database needs to be left alone. Reads that need the database answer
    `503 STORAGE_UNAVAILABLE` in the meantime, which clients should ride out
    rather than treat as data loss.

### Runtime status

`GET /api/v1/runtime/status` returns `runtime` (`docker` or `kubernetes`), `context` (`docker`, `swarm` or `kubernetes`), `connected`, `label` (`Containers`, `Services` or `Workloads`), `detected_at` and `metadata`. On a Swarm manager `metadata` holds `cluster_id`, `is_manager`, `manager_count`, `worker_count` and `service_count`. The manager and worker counts come from `docker info` and are refreshed every 60 seconds. A Swarm worker reports the `docker` context.

### Edition

`GET /api/v1/edition` returns:

| Field | Description |
|-------|-------------|
| `edition` | `community`, `personal` or `pro` |
| `organisation_name` | Value of `MAINTENANT_ORGANISATION_NAME` (default `Maintenant`) |
| `status_url` | Public status page URL: the value of `MAINTENANT_STATUS_URL`, empty when it is not set |
| `demo` | `true` in demo mode |
| `features` | Every capability mapped to a boolean: does the running edition open it. `smtp` is also `false` until SMTP is configured |
| `feature_editions` | Every capability mapped to the lowest edition that opens it |
| `quotas` | For each capped resource present (`agent_hosts`, `endpoints`, `heartbeats`, `certificates`, `status_components`), an object `{ "used": n, "limit": n }`. A limit of `-1` means unlimited |
| `tiers` | The cap of every resource for every edition: `{ "community": {...}, "personal": {...}, "pro": {...} }` |
| `suspended_channels` | `{ "count": n, "channels": [{ "id", "name", "type", "required_edition" }] }`: enabled channels the running edition no longer opens |
| `resource_history` | `{ "max_window", "max_window_seconds", "windows": [{ "window", "seconds", "min_edition" }] }`. The window catalogue is the same in every edition |

### Licence

`GET /api/v1/license/status` always returns the same eight keys: `status`, `edition`, `plan`, `message`, `verified_at`, `expires_at`, `updates_until` and `update_grace_until`. Dates are RFC 3339 strings, or an empty string when the licence has none (a perpetual licence has no `expires_at`). Without a licence key the status is `inactive` and the edition `community`. Other statuses include `active`, `grace`, `expired`, `revoked`, `update_window_grace` and `update_window_ended`.

---

## Containers

All routes are open in every edition.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/containers` | List containers, grouped |
| `GET` | `/api/v1/containers/{id}` | Get a container with its uptime |
| `GET` | `/api/v1/containers/{id}/transitions` | List state transitions |
| `GET` | `/api/v1/containers/{id}/logs` | Fetch recent logs |
| `GET` | `/api/v1/containers/{id}/logs/stream` | Follow logs in real time (SSE) |
| `GET` | `/api/v1/containers/{id}/uptime/daily` | Daily uptime percentages |
| `GET` | `/api/v1/containers/{id}/endpoints` | List the endpoints of a container |
| `DELETE` | `/api/v1/containers/{id}` | Remove a container from monitoring |

`GET /api/v1/containers` query parameters: `archived=true` also returns archived containers, `group` (a custom or orchestration group name), `state` (for example `running`, `exited`, `completed`, `restarting`, `paused`, `created`, `dead`) and `agent_id`. A container that stopped with exit code 0 or 143 is `completed`, and so is one that ended with 137 unless the out-of-memory killer sent it: an OOM kill is `exited`, like any other crash. Containers marked `maintenant.ignore` are never listed. The answer is `{ "groups": [{ "name", "source", "containers": [...] }], "total", "archived_count" }`, plus `"stale": true` when the local runtime is disconnected. `source` is `label`, `namespace`, `compose`, `orchestration` or `default`. Each container carries its identity (`id`, `external_id`, `agent_id`, `name`, `image`), `state`, `health_status` (`null` without a health check), `has_health_check`, group and orchestration fields, `is_ignored`, `alert_severity`, `restart_threshold`, `archived`, timestamps, Kubernetes and Swarm details where they apply, `security_insight_count` and `security_highest_severity`. A container reported by a remote agent also carries `agent_hostname` and `agent_label`, and `stale` plus `agent_offline` when that agent has no live stream.

`GET /api/v1/containers/{id}` returns the same core fields as one flat object, plus `uptime` and, for a Kubernetes workload, `container_names`. `uptime` holds a percentage for `24h` and for every longer window the edition's history cap opens: `7d` on Community, plus `30d` on Personal and `90d` on Pro. Use the daily route for day-by-day history. A Swarm task also carries `swarm_service_id`, `swarm_service_name`, `swarm_node_id` and `swarm_task_slot`.

`GET /api/v1/containers/{id}/transitions` query parameters: `since` and `until` (RFC 3339; `since` defaults to 24 hours ago, an invalid value removes the bound), `limit` (default 50) and `offset`. The answer is `{ "container_id", "transitions", "total", "has_more" }`, newest first.

`GET /api/v1/containers/{id}/logs` query parameters: `lines` (default 100, at most 500) and `timestamps=true`. The answer is `{ "container_id", "container_name", "lines", "total_lines", "truncated" }`. Logs of a remote container are fetched from its agent (15-second deadline).

`GET /api/v1/containers/{id}/logs/stream` is a Server-Sent Events stream with its own `lines` parameter (default 100, at most 500) and, on Kubernetes, `container` to pick a container of the pod. It sends `container.log_line` events (`container_id`, `line`, `stream`, `timestamp`), a keep-alive comment every 25 seconds while the container is silent, and ends with a `container.log_error` event (`container_id`, `error`: `container stopped`, `agent disconnected` or the agent's error). The route exists when the local runtime can read logs or an agent can serve them.

`DELETE /api/v1/containers/{id}` removes the container and its history with a hard delete and answers `204`. It emits `container.archived`.

Errors:

- `404 CONTAINER_NOT_FOUND`: no such container (also on the logs, stream and `endpoints` routes).
- `409 CONTAINER_RUNNING`: the container is running and cannot be deleted.
- `502 RUNTIME_UNAVAILABLE`: the local runtime or agent support is not wired for logs. `502 LOGS_UNAVAILABLE`: the runtime or the agent could not return the logs.
- Logs of a remote container: `503 AGENT_OFFLINE`, `501 AGENT_TOO_OLD` (the agent predates the log command), `429 LOGS_BUSY` (too many concurrent log requests for that agent) and `504 LOGS_TIMEOUT`.
- `logs` and `logs/stream` answer `503 RUNTIME_UNAVAILABLE` when the local runtime is disconnected.

`GET /api/v1/containers/{id}/uptime/daily` (and the same route for endpoints and heartbeats) takes `days` (default 90, at most 365) and answers `{ "monitor_id", "monitor_type", "days": [{ "date", "uptime_percent", "incident_count" }] }`: one entry per UTC day, most recent first. `uptime_percent` is `null` for a day without data. Completed days come from the daily aggregates, which are kept for 365 days; the current day is computed from the raw rows. The day of a heartbeat is weighted by the time the monitor was up: a missed deadline counts as down until the next successful ping, and the days before its first ping have no data.

---

## Endpoints

Endpoint monitors are discovered from labels or created through the API (a standalone endpoint). All routes are open in every edition, except that creating a standalone endpoint is capped.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/endpoints` | List endpoints | — |
| `POST` | `/api/v1/endpoints` | Create a standalone endpoint (10 on Community) | — |
| `GET` | `/api/v1/endpoints/{id}` | Get an endpoint with its uptime | — |
| `PUT` | `/api/v1/endpoints/{id}` | Update a standalone endpoint | — |
| `DELETE` | `/api/v1/endpoints/{id}` | Delete an endpoint | — |
| `GET` | `/api/v1/endpoints/{id}/checks` | List check results | — |
| `POST` | `/api/v1/endpoints/{id}/check` | Probe now and return the refreshed endpoint | — |
| `GET` | `/api/v1/endpoints/{id}/uptime/daily` | Daily uptime percentages | — |

An endpoint has `id`, `endpoint_type` (`http` or `tcp`), `target`, `status` (`up`, `down`, `degraded` or `unknown`), `alert_state` (`normal` or `alerting`), the consecutive counters, the last check fields, `source` (`label` or `standalone`), `name`, `agent_id` and a `config` object (`interval` and `timeout` as duration strings such as `30s`, `failure_threshold`, `recovery_threshold`, `method`, `expected_status`, `tls_verify`, `headers`, `max_redirects`). A new standalone endpoint defaults to a 30-second interval, a 10-second timeout, failure threshold 3, recovery threshold 2, method `GET`, expected status `2xx`, TLS verification on and 5 redirects.

- `GET /api/v1/endpoints` query parameters: `status`, `container` (container name), `type`, `source`, `include_inactive=true` and `agent_id`. The answer is `{ "endpoints": [...], "total" }`. `stale` and `agent_offline` mark an endpoint probed by an agent with no live stream.
- `POST /api/v1/endpoints` body: `name` (required), `target` (required; an absolute `http` or `https` URL for `http`, a `host:port` address with a port from 1 to 65535 for `tcp`), `endpoint_type` (required, `http` or `tcp`), `interval` (duration, at least 5s, default 30s; an interval below 5s in a `maintenant.endpoint.*.interval` label is raised to 5s), `timeout` (duration, at least 1s, default 10s, not above the interval), `method` (default `GET`) and `headers` (object). Thresholds, expected status, TLS verification and redirects cannot be set through the API. Answers `201` with `{ "endpoint": {...} }`.
- `PUT /api/v1/endpoints/{id}` accepts the same fields, all optional; an omitted or empty field keeps its value and `headers` replaces the whole map. The resulting target is validated for the endpoint type as on creation. Only standalone endpoints can be updated.
- `GET /api/v1/endpoints/{id}` answers `{ "endpoint", "uptime": { "1h", "24h", "7d", "30d" } }`.
- `GET /api/v1/endpoints/{id}/checks` takes `limit` (default 50, at most 500), `offset` and `since` (Unix seconds). The answer is `{ "endpoint_id", "checks", "total", "has_more" }`.
- `POST /api/v1/endpoints/{id}/check` probes synchronously and answers `{ "endpoint", "uptime" }`.

Errors:

- `400 INVALID_JSON`, `400 INVALID_INPUT` (the message names the field), `400 NOT_STANDALONE` (updating a label-discovered endpoint).
- `403 QUOTA_EXCEEDED`: creating past the cap (`resource: "endpoints"`, `limit: 10` on Community).
- `404 ENDPOINT_NOT_FOUND`.
- `409 ENDPOINT_LIVE`: deleting an endpoint whose container still runs. It becomes deletable once its container is gone.
- `409 AGENT_PROBED`: `check` on an endpoint an agent probes itself. `409 CHECK_IN_PROGRESS`: a check is already running.

---

## Heartbeats

All routes are open in every edition, except that creating a heartbeat monitor is capped.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/heartbeats` | List heartbeat monitors | — |
| `POST` | `/api/v1/heartbeats` | Create a heartbeat monitor (5 on Community) | — |
| `GET` | `/api/v1/heartbeats/{id}` | Get a heartbeat monitor and its ping snippets | — |
| `PUT` | `/api/v1/heartbeats/{id}` | Update a heartbeat monitor | — |
| `DELETE` | `/api/v1/heartbeats/{id}` | Delete a heartbeat monitor | — |
| `POST` | `/api/v1/heartbeats/{id}/pause` | Pause deadline checking | — |
| `POST` | `/api/v1/heartbeats/{id}/resume` | Resume deadline checking | — |
| `GET` | `/api/v1/heartbeats/{id}/executions` | List executions | — |
| `GET` | `/api/v1/heartbeats/{id}/pings` | List raw pings | — |
| `GET` | `/api/v1/heartbeats/{id}/uptime/daily` | Daily uptime percentages | — |

A heartbeat has `id` (which is also its ping token), `name`, `status` (`new`, `up`, `down`, `started` or `paused`), `alert_state`, `interval_seconds`, `grace_seconds`, the last ping and deadline fields, the consecutive counters and timestamps. The deadline is the last ping plus the interval plus the grace period.

- `POST /api/v1/heartbeats` body: `name` (required, 1 to 255 characters), `interval_seconds` (required, 60 to 604800) and `grace_seconds` (0 to the interval, default 0). Answers `201` with the heartbeat object. The cap counts every heartbeat, paused and agent-relayed ones included.
- `PUT /api/v1/heartbeats/{id}` takes the same fields, all optional.
- `GET /api/v1/heartbeats`: query parameters `status` and `agent_id`; answers `{ "heartbeats", "total" }`.
- `GET /api/v1/heartbeats/{id}` answers `{ "heartbeat", "snippets" }`; `snippets` holds ready-to-paste `curl`, `wget`, `python`, `go`, `bash` and `docker_healthcheck` snippets once `MAINTENANT_BASE_URL` is set.
- `executions` (`limit` default 20, at most 500, and `offset`) answers `{ "executions", "total" }`, newest first; each has an `outcome` of `success`, `failure`, `timeout` or `in_progress`. `pings` (`limit` default 50, at most 500, and `offset`) answers `{ "pings", "total" }` with `expected_at` and `grace_deadline` computed for each ping.
- `pause`, `resume` and `PUT` answer `200` with the heartbeat; `DELETE` answers `204`. Deleting is a hard delete: the monitor, its pings, executions, pauses and daily uptime are removed, and its id answers `404` afterwards, on these routes and on `/ping/`.

Errors:

- `400 INVALID_JSON`, `400 INVALID_INPUT` (including `heartbeat is already paused` on `pause` and `heartbeat is not paused` on `resume`). A ping received while the heartbeat is paused puts it back to monitoring.
- `403 QUOTA_EXCEEDED`: creating past the cap (`resource: "heartbeats"`, `limit: 5` on Community).
- `404 NOT_FOUND`: unknown heartbeat, including on `DELETE`, `pause` and `resume`.

### Ping endpoints (public)

These routes need no credentials: the UUID of the heartbeat is the secret. They share the tight rate limit of `/ping/` and answer `403 DEMO_MODE` in demo mode.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET/POST` | `/ping/{uuid}` | Simple ping (success) |
| `GET/POST` | `/ping/{uuid}/start` | Signal job start |
| `GET/POST` | `/ping/{uuid}/{exit_code}` | Ping with exit code (0 = success) |

- `/ping/{uuid}` and `/ping/{uuid}/start` answer `200 {"ok": true}`. `/ping/{uuid}/{exit_code}` answers `200 {"ok": true, "exit_code": n}`. A non-zero exit code raises an alert.
- Errors: `404 HEARTBEAT_NOT_FOUND` (unknown or deleted heartbeat), `400 INVALID_EXIT_CODE` (not an integer between 0 and 255, checked before the lookup) and `500 INTERNAL_ERROR`.
- A `POST` body of up to 10 KiB is read but never stored.
- The `source_ip` of a ping is the client address resolved like the rate limit does: forwarded headers are believed only when the connection comes from a proxy listed in `MAINTENANT_TRUSTED_PROXIES`.

### Outbound heartbeats

Outbound heartbeats make this instance ping an external URL so another system notices when it stops. The routes are not registered in demo mode. All are open in every edition. See [Outbound Heartbeats](../features/heartbeats.md#outbound-heartbeats).

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/outbound-heartbeats` | List all targets |
| `POST` | `/api/v1/outbound-heartbeats` | Create a target |
| `PUT` | `/api/v1/outbound-heartbeats/{id}` | Replace a target |
| `DELETE` | `/api/v1/outbound-heartbeats/{id}` | Delete a target |
| `POST` | `/api/v1/outbound-heartbeats/{id}/send` | Send a ping now |

A target has `id`, `name`, `url`, `interval_seconds`, `enabled`, `last_sent_at`, `last_status_code`, `last_error`, `created_at` and `updated_at`. The body of `POST` and `PUT` holds `name` (1 to 255 characters), `url` (an absolute HTTPS URL that does not resolve to a private or internal address), `interval_seconds` (30 to 86400) and `enabled` (default `true`; `PUT` replaces the target, so omitting `enabled` enables it). The list answers `{ "outbound_heartbeats": [...] }`. `send` works on a disabled target and answers `200` with the refreshed object: a failed send is recorded in `last_error` and `last_status_code`, not returned as an HTTP error. Errors: `400 INVALID_JSON`, `400 INVALID_INPUT` and `404 NOT_FOUND`.

---

## Certificates

All routes are open in every edition, except that creating a standalone monitor is capped.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/certificates` | List certificate monitors | — |
| `POST` | `/api/v1/certificates` | Create a standalone certificate monitor (5 on Community) | — |
| `GET` | `/api/v1/certificates/{id}` | Get a monitor with its latest check and chain | — |
| `PUT` | `/api/v1/certificates/{id}` | Update a monitor | — |
| `DELETE` | `/api/v1/certificates/{id}` | Delete a monitor | — |
| `GET` | `/api/v1/certificates/{id}/checks` | List check history | — |
| `POST` | `/api/v1/certificates/{id}/check` | Scan now and return the refreshed monitor | — |

A monitor has `id`, `hostname`, `port`, `server_name`, `source` (`auto`, `standalone` or `label`), `status` (`valid`, `expiring`, `expired`, `error` or `unknown`), `check_interval_seconds`, `warning_thresholds` (days, default `[30, 14, 7, 3, 1]`), the last check and next check times, `agent_id` and timestamps.

- `GET /api/v1/certificates` query parameters: `status`, `source` and `agent_id`. Each monitor carries a `latest_check` object, and the answer is `{ "certificates", "total" }`.
- `POST /api/v1/certificates` body: `hostname` (required), `port` (default 443), `server_name` (a bare host name, optional), `check_interval_seconds` (3600 to 604800, default 43200) and `warning_thresholds`. The first TLS check runs before the answer, so `201` returns `{ "certificate", "latest_check" }`.
- `PUT /api/v1/certificates/{id}` accepts `check_interval_seconds` and `warning_thresholds` only; other fields are ignored.
- `GET /api/v1/certificates/{id}` returns `{ "certificate", "latest_check" }` where `latest_check` holds the subject, issuer, SANs, validity dates, `days_remaining`, `chain_valid`, `chain_error`, `hostname_match`, the `chain` and, for a stapled response, `ocsp_stapled`, `ocsp_status`, `ocsp_produced_at`, `ocsp_next_update` and `ocsp_error`.
- `GET /api/v1/certificates/{id}/checks` takes `limit` (default 50, at most 500) and `offset` and answers `{ "monitor_id", "checks", "total", "has_more" }`. Each check lists `ocsp_stapled` and `ocsp_status` only; the other OCSP fields are on `latest_check`. A scan pushed by an endpoint probe or by an agent is stored once per monitor interval, or sooner when its outcome changes, so the history holds one row per interval or change.

Errors:

- `400 INVALID_JSON`, `400 INVALID_INPUT`, `400 CANNOT_DELETE_AUTO` (an auto-detected monitor is removed by removing its label).
- `403 QUOTA_EXCEEDED` (`resource: "certificates"`, `limit: 5` on Community).
- `404 NOT_FOUND`.
- `409 DUPLICATE_MONITOR`, `409 ALREADY_AUTO_DETECTED`, `409 AGENT_SCANNED` (the agent scans this one itself) and `409 CHECK_IN_PROGRESS`.

---

## Resources

All routes are open in every edition. The history window is capped by the edition.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/containers/{id}/resources/current` | Current CPU, memory, network, I/O | — |
| `GET` | `/api/v1/containers/{id}/resources/history` | Historical metrics (`?range=24h`; window `30d` needs Personal, `90d` needs Pro) | — |
| `GET` | `/api/v1/containers/{id}/resources/alerts` | Get alert thresholds | — |
| `PUT` | `/api/v1/containers/{id}/resources/alerts` | Set alert thresholds | — |
| `GET` | `/api/v1/resources/summary` | Aggregate resource summary of one host | — |
| `GET` | `/api/v1/resources/top` | Top consumers (a `period` of `30d` needs Personal, `90d` needs Pro) | — |
| `GET` | `/api/v1/resources/hosts` | List hosts (local and agents) with current CPU, memory and disk | — |

- `current` answers `container_id`, `cpu_percent` (100 is one core), `mem_used`, `mem_limit`, `mem_percent`, `net_rx_bytes`, `net_tx_bytes`, `block_read_bytes`, `block_write_bytes` and `timestamp`. It returns `404 NOT_FOUND` when no sample exists yet. For a container on a remote agent, only a sample received in the last 35 seconds counts.
- `history` takes `range`: `1h` (the default), `6h`, `24h`, `7d`, `30d` or `90d`. The answer is `{ "container_id", "range", "granularity", "points" }` with `granularity` of `raw`, `1m`, `5m`, `1h` or `1d`. Errors: `400 INVALID_RANGE` for an unknown window, `403 EDITION_REQUIRED` for a window the edition does not open (with `feature: "resource_history"`, `required_edition`, `window` and `max_window`).
- `alerts` (GET) answers `container_id`, `cpu_threshold`, `mem_threshold`, `enabled`, `alert_state` and `last_alerted_at`; without configuration it returns 90, 90 and `enabled: false`. `PUT` takes `cpu_threshold` (1 to 1000, required), `mem_threshold` (1 to 100) and `enabled` (omitted means `false`). An alert fires after two consecutive breaching samples, and CPU and memory fire and resolve independently. Errors: `400 INVALID_THRESHOLD`.
- `summary` takes `agent_id` (`local` or absent for the local host) and answers the host totals: `agent_id` (an empty string for the local host), `available`, `total_cpu_percent`, `cpu_count`, `total_mem_used`, `total_mem_limit`, `total_mem_percent`, `total_net_rx_rate`, `total_net_tx_rate`, `container_count`, `disk_total`, `disk_used`, `disk_percent` and `timestamp`. The container count and the network figures cover the containers of that host, including those of a remote agent. `total_net_rx_rate` and `total_net_tx_rate` are throughputs in bytes per second, computed from the last two samples of each container. A container with a single sample, a counter that went backwards (restart) or a runtime that does not report network counters adds nothing to them.
- `top` requires `metric` (`cpu` or `memory`, otherwise `400 INVALID_METRIC`), and takes `limit` (default 5; a larger value is capped at 20), `agent_id` (absent means every host) and `period` (a history window). The hour or day in progress counts in a `period` ranking, not only the closed hours and days. Without `period` the ranking uses the latest samples and is open in every edition. The answer is `{ "metric", "period", "consumers": [{ "container_id", "container_name", "value", "percent", "rank" }] }`. A bad `period` answers `400 INVALID_PERIOD`, or `403 EDITION_REQUIRED` as for `history`.
- `hosts` answers `{ "hosts": [{ "agent_id", "hostname", "label", "is_local", "available", "cpu_percent", "mem_used", "mem_total", "mem_percent", "disk_total", "disk_used", "disk_percent", "container_count" }] }`, the local host first.

---

## Kubernetes

All routes are open in every edition. They are read from the store, per agent, so they also serve clusters monitored by a remote agent. The routes that need metrics-server (`workloads/{id}/resources`, `nodes/{name}/resources`) answer `200` with `"metrics_available": false` and a message when the data is not available, which is always the case for a remote agent.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/kubernetes/namespaces` | List namespaces |
| `GET` | `/api/v1/kubernetes/workloads` | List workloads, grouped by namespace |
| `GET` | `/api/v1/kubernetes/workloads/{id}` | Workload with its pods and events |
| `GET` | `/api/v1/kubernetes/workloads/{id}/resources` | Per-pod CPU and memory from metrics-server |
| `GET` | `/api/v1/kubernetes/pods` | List pods |
| `GET` | `/api/v1/kubernetes/pods/{namespace}/{name}` | Pod with its events |
| `GET` | `/api/v1/kubernetes/nodes` | List nodes |
| `GET` | `/api/v1/kubernetes/nodes/{name}/resources` | Node CPU and memory from metrics-server |
| `GET` | `/api/v1/kubernetes/cluster` | Cluster overview |

- Every route takes `agent_id`.
- `workloads` takes `namespaces` (comma-separated), `kind` (`Deployment`, `StatefulSet`, `DaemonSet` or `Job`) and `status` (`healthy`, `degraded`, `progressing` or `failed`). It answers `{ "groups": [{ "namespace", "workloads" }], "total" }`. A workload has `id` (`namespace/Kind/name`), `name`, `namespace`, `kind`, `images`, `ready_replicas`, `desired_replicas`, `status` and `created_at`.
- `{id}` is the workload id with its slashes URL-encoded (`default%2FDeployment%2Fweb`).
- `pods` takes `namespaces`, `workload` (id prefix), `node` and `status` (for example `Running`, `Pending`). `{ "pods", "total" }`.
- `cluster` answers `namespace_count`, `node_count`, `node_ready_count`, `pod_status`, `workload_count`, `workload_healthy`, `cluster_health` (`healthy` or `degraded`) and a per-namespace summary.
- Errors: `404 K8S_WORKLOAD_NOT_FOUND`, `404 K8S_POD_NOT_FOUND`, `400 INVALID_ID` (bad workload id encoding) and `500 K8S_ERROR`.

---

## Swarm

All routes are open in every edition. Services, tasks and nodes are read from the store, per agent. `info`, `dashboard`, `cluster`, `update-status` and `resources` read the server's own live Swarm and answer `409 SWARM_NOT_ACTIVE` when the local runtime is not a Swarm manager.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/swarm/info` | Swarm state of the local runtime (`{ "active": false }` when there is none) |
| `GET` | `/api/v1/swarm/services` | List services (`?stack=`, `?agent_id=`) |
| `GET` | `/api/v1/swarm/services/{serviceID}` | Service with its tasks |
| `GET` | `/api/v1/swarm/services/{serviceID}/update-status` | Rolling update state (live) |
| `GET` | `/api/v1/swarm/services/{serviceID}/resources` | Per-task CPU, memory and network (live) |
| `GET` | `/api/v1/swarm/tasks` | List tasks (`?service=`, `?node=`, `?state=`, `?agent_id=`) |
| `GET` | `/api/v1/swarm/nodes` | List nodes (`?agent_id=`) |
| `GET` | `/api/v1/swarm/nodes/{nodeID}` | Node with its tasks (live) |
| `GET` | `/api/v1/swarm/dashboard` | Cluster, node and service summary (live) |
| `GET` | `/api/v1/swarm/cluster` | Cluster health and counts (live) |

`dashboard` answers `cluster` (counts and task health), `nodes` and `services`.

`info` answers `active`, `cluster_id`, `is_manager`, `manager_count`, `worker_count` and `created_at`. The counts (also in `dashboard`, `cluster` and `GET /api/v1/runtime/status`) and the creation date come from `docker info` and are refreshed every 60 seconds. The server detects a Swarm as soon as Docker connects, so activating Swarm later needs no restart.

Errors: `404 SWARM_SERVICE_NOT_FOUND`, `404 SWARM_NODE_NOT_FOUND`, `409 SWARM_NOT_ACTIVE`, `409 SWARM_NODES_NOT_AVAILABLE` (node monitoring is not available) and `500 INTERNAL_ERROR`.

---

## Agents

Multi-host agent management. Every route requires **Personal** or above (capability `multihost`) and answers `403 EDITION_REQUIRED` below that. See [Multi-Host Monitoring](../features/multihost.md).

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/agents` | List enrolled agents | Personal |
| `GET` | `/api/v1/agents/{id}` | Get an agent | Personal |
| `PATCH` | `/api/v1/agents/{id}` | Update an agent's display label | Personal |
| `POST` | `/api/v1/agents/{id}/revoke` | Revoke an agent (closes its stream, stops retries) | Personal |
| `DELETE` | `/api/v1/agents/{id}` | Delete an agent and purge all its events | Personal |
| `GET` | `/api/v1/agents/metrics` | Fleet counts | Personal |
| `POST` | `/api/v1/agents/enrollment-tokens` | Create a one-time enrollment token | Personal |
| `GET` | `/api/v1/agents/enrollment-tokens` | List enrollment tokens (masked) | Personal |
| `GET` | `/api/v1/agents/enrollment-tokens/{token_id}` | Get an enrollment token (masked) | Personal |
| `DELETE` | `/api/v1/agents/enrollment-tokens/{token_id}` | Delete an enrollment token | Personal |

An agent has `agent_id`, `hostname`, `label`, `os_arch`, `agent_version`, `detected_runtime` (`docker`, `swarm` or `kubernetes`), `status` (`active` or `revoked`), `connection_state` (`connected` or `disconnected`), `last_seen_at`, `created_at`, `revoked_at`, `revoked_by`, `spool` and `os`. `spool` (`queued`, `draining`, `dropped_since_connect`, `reported_at`) is `null` while the agent is disconnected. `os` holds the host operating system (`id`, `version_id`, `pretty_name`, `source`, `unavailable_reason`, `reported_at`) and its end-of-support state. An agent is `connected` while its stream is live or it was seen within `MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS` (60 by default).

- `GET /api/v1/agents` takes `status` (`active`, `revoked` or `all`) and `connection_state` (`connected` or `disconnected`); the answer is `{ "agents": [...] }`.
- `PATCH` takes `{ "label": "..." }` (required, up to 64 characters, empty clears it) and answers the updated agent.
- `revoke` answers the agent and `delete` answers `204`.
- `POST /api/v1/agents/enrollment-tokens` takes an optional `{ "ttl_hours": n }` (default 24, at most 168). The `201` answer carries `token_id`, `token` (shown only here), `token_masked`, `created_at`, `expires_at`, `install_templates` (`standalone`, `docker_run`, `docker_compose`, `kubernetes`) and `warnings` (for example `public_url_appears_local`, `public_url_plaintext_refused`).
- `GET .../enrollment-tokens` lists unconsumed, unexpired tokens; `include_expired=true` and `include_consumed=true` widen the list. The answer is `{ "tokens": [...] }`.
- `GET /api/v1/agents/metrics` answers `total`, `by_status`, `by_runtime`, `by_connection_state` and `total_events_per_second_observed_5m`.

Errors: `404 NOT_FOUND`, `400 INVALID_JSON`, `400 MISSING_FIELD`, `400 LABEL_TOO_LONG`, `409 TOKEN_CONSUMED` (deleting a consumed token) and `409 HOST_LIMIT_REACHED` (creating a token when the cap is reached: 20 active remote agents on Personal, the error names `pro` as the edition that lifts it).

---

## Alerts

All routes are open in every edition.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/alerts` | List alerts (including resolved), newest first |
| `GET` | `/api/v1/alerts/active` | List active alerts, by severity |
| `GET` | `/api/v1/alerts/{id}` | Get an alert |
| `POST` | `/api/v1/alerts/{id}/acknowledge` | Acknowledge an active alert |

- `GET /api/v1/alerts` takes `source`, `severity` and `status` (`active`, `resolved` or `silenced`), `before` (RFC 3339; returns alerts fired before it), `before_id` and `limit` (1 to 200, default 50). The answer is `{ "alerts": [...], "has_more": bool }`, ordered by `fired_at` then `id`, newest first. To page, pass the `fired_at` and the `id` of the last alert as `before` and `before_id`: alerts fired in the same second as the cursor are neither skipped nor repeated. `before_id` without `before` is refused. Errors: `400 INVALID_PARAM`.
- `GET /api/v1/alerts/active` answers `{ "critical": [...], "warning": [...], "info": [...] }`. Acknowledged and resolved alerts are left out.
- `GET /api/v1/alerts/{id}` answers `404 NOT_FOUND` for an unknown alert.
- A daily job purges the alerts that are not active (resolved or silenced) once they are older than 90 days, counted from their resolution, or from their creation when they never resolved. An alert that is still active is never purged, however old.
- An alert has `id`, `source`, `alert_type`, `severity` (`critical`, `warning` or `info`), `status`, `message`, `entity_type`, `entity_id`, `entity_name`, `details` (a string holding JSON), `fired_at`, `resolved_at`, `resolved_by_id`, `acknowledged_at`, `acknowledged_by`, `escalated_at` (set when an escalation policy notifies a level for the alert) and `created_at`.
- `POST .../acknowledge` takes `{ "acknowledged_by": "..." }` (required). It answers the alert, emits `alert.acknowledged` and stops its running escalation. The dashboard, the MCP server and the security posture acknowledgments all go through this one path, so they behave the same. An acknowledged alert whose severity later rises does not start a new escalation. Errors: `400 INVALID_BODY`, `400 INVALID_REQUEST`, `404 NOT_FOUND` (unknown alert) and `409 CONFLICT` (the alert is not active or is already acknowledged).

---

## Notification Channels

Channels are silent by default. They only fire when referenced by an [Alert Trigger](#alert-triggers) or an [Escalation Policy](#escalation-policies). Every route is open, but the channel type needs an edition.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/channels` | List notification channels |
| `POST` | `/api/v1/channels` | Create a channel |
| `PUT` | `/api/v1/channels/{id}` | Update a channel |
| `DELETE` | `/api/v1/channels/{id}` | Delete a channel |
| `POST` | `/api/v1/channels/{id}/test` | Send a test notification |

| Channel `type` | Destination (`url`) | Edition |
|----------------|---------------------|:-------:|
| `webhook` (default) | HTTPS URL | — |
| `discord` | Discord webhook URL | — |
| `email` | Recipient address (needs `MAINTENANT_SMTP_*`) | Personal |
| `telegram` | Chat id (`-1001234567890`) or public `@username` | Personal |
| `slack` | Slack webhook URL | Pro |
| `teams` | Teams webhook URL | Pro |

A channel has `id`, `name`, `type`, `url`, `headers` (a string holding a JSON object), `config` (a string holding JSON, for example `{"thread_id": "42"}` on Telegram), `has_secret`, `enabled`, `health` (`healthy` or `failing`, on the list only), `suspended`, `required_edition` and timestamps. The secret (a Telegram bot token) is never returned, only `has_secret`; the URL and headers are returned as stored.

- `POST` body: `name` (required), `url` (required), `type`, `headers`, `secret` (required for Telegram), `config` and `enabled` (default `true`). A URL must be HTTPS and must not resolve to a private or internal address, unless `MAINTENANT_ALLOW_PRIVATE_WEBHOOKS` is set. Answers `201` with the channel.
- `PUT` takes the same fields, all optional. A secret cannot be cleared. A request that only sets `enabled` to `false` is always accepted, even after the edition dropped.
- `test` answers `404 NOT_FOUND` for an unknown channel. Otherwise it answers `200`: `{ "status": "delivered", "response_code": n }` or `{ "status": "failed", "error": "..." }`.
- Creating, updating or testing a channel of a type the edition does not open answers `403 EDITION_REQUIRED` (`feature` is the capability: `slack`, `teams`, `smtp` or `telegram`). After a downgrade such a channel is `suspended`: it stops delivering and `GET /api/v1/edition` lists it under `suspended_channels`.
- Errors: `400 INVALID_BODY`, `400 VALIDATION_ERROR`, `404 NOT_FOUND` and `409 DUPLICATE_NAME` on create.

---

## Alert Triggers

Triggers route alerts to channels based on filters. A trigger matches when all of its non-empty filters match.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/alert-triggers` | List all triggers |
| `POST` | `/api/v1/alert-triggers` | Create a trigger |
| `GET` | `/api/v1/alert-triggers/{id}` | Get trigger details |
| `PUT` | `/api/v1/alert-triggers/{id}` | Update a trigger |
| `DELETE` | `/api/v1/alert-triggers/{id}` | Delete a trigger |

A trigger has `id`, `name`, `filter_severities`, `filter_sources` and `filter_scopes` (comma-separated strings, empty matches everything; a scope is `entity_type:entity_id`), `enabled`, `notify_on_resolve`, `channel_ids` and timestamps. The list answers `{ "triggers": [...] }`.

- `POST` body: `name` (required, up to 120 characters), `channel_ids` (required, at least one existing channel), the three filters, `enabled` (default `true`) and `notify_on_resolve` (default `true`, set it to `false` for a channel that should only receive failures).
- `PUT` replaces the name, the filters and the channels; `enabled` and `notify_on_resolve` keep their value when omitted.
- `filter_scopes` requires **Personal** (capability `alert_advanced_filters`). Below Personal, a non-empty value that differs from the stored one answers `403 EDITION_REQUIRED`; keeping or clearing the stored value is allowed. `filter_severities` and `filter_sources` are open in every edition.
- Errors (lower case codes): `400 validation_failed`, `404 trigger_not_found`, `409 name_conflict`. Invalid JSON answers `400 INVALID_BODY`.

---

## Escalation Policies

Multi-level escalation chains for unacknowledged alerts. Every route requires **Pro** (capability `alert_escalation`) and answers `403 EDITION_REQUIRED` below it.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/escalation-policies` | List policies (`?active=true` keeps the active ones) | Pro |
| `POST` | `/api/v1/escalation-policies` | Create a policy | Pro |
| `GET` | `/api/v1/escalation-policies/{id}` | Get a policy | Pro |
| `PUT` | `/api/v1/escalation-policies/{id}` | Replace a policy | Pro |
| `PATCH` | `/api/v1/escalation-policies/{id}/active` | Activate or deactivate | Pro |
| `DELETE` | `/api/v1/escalation-policies/{id}` | Delete a policy | Pro |
| `POST` | `/api/v1/escalation-policies/overlap-probe` | Detect overlapping policies | Pro |
| `GET` | `/api/v1/escalation-policies/{id}/runs` | List recent runs of a policy | Pro |
| `GET` | `/api/v1/alerts/{alert_id}/escalation-runs` | List the runs of an alert | Pro |
| `GET` | `/api/v1/escalation-runs/{run_id}` | Get a run with its deliveries | Pro |

A policy is `{ "name", "active", "filters": { "severities", "scopes": [{ "kind", "ref_id" }] }, "levels": [{ "delay_seconds", "channel_ids" }] }`. Validation: a name of 1 to 120 characters, one to 5 levels, each delay between 60 and 86400 seconds and at least 60 seconds after the previous level, and at least one channel per level. `active` has no default: leaving it out creates or replaces the policy as inactive. The delays of the levels count from the start of the run.

- `GET /api/v1/escalation-policies` answers `{ "policies": [...], "limits": { "max_active", "max_levels", "current_active" } }`. `max_active` is `-1` (no cap on active policies) and `max_levels` is 5.
- `overlap-probe` takes a policy body and answers `{ "overlapping": [{ "policy_id", "policy_name", "shared_channels", "filter_intersection" }] }`: policies whose filters intersect and that share at least one channel.
- `PATCH .../active` takes `{ "active": bool }` and answers `{ "id", "active", "updated_at" }`.
- `runs` takes `limit` (default 50, at most 200) and `cursor` (a run id; returns older runs) and answers `{ "runs": [...] }`. A run has a `status` (`active`, `paused_by_maintenance`, `stopped_by_ack`, `stopped_by_resolution`, `stopped_by_policy_deletion`, `stopped_by_policy_disabled`, `stopped_by_edition_downgrade` or `exhausted`); its deliveries have a `status` of `pending`, `sent`, `failed` or `abandoned`. Deactivating a policy (`PUT` with `active` false, or `PATCH .../active`) stops its running runs with `stopped_by_policy_disabled`.
- Errors (lower case codes): `400 validation_failed`, `400 invalid_body`, `404 policy_not_found`, `404 run_not_found` and `500 internal_error`.

---

## Silence Rules

All routes are open in every edition.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/silence` | List silence rules (`?active=true` keeps the ones in effect) |
| `POST` | `/api/v1/silence` | Create a silence rule |
| `DELETE` | `/api/v1/silence/{id}` | Cancel a silence rule |

- `POST` body: `duration_seconds` (required, above zero) and optional `entity_type`, `entity_id`, `source` and `reason`. All three filters empty silence everything; otherwise an alert must match every filter that is set. Answers `201` with the rule.
- The list answers `{ "rules": [...] }`. A rule has `id`, `entity_type`, `entity_id`, `source`, `reason`, `starts_at`, `duration_seconds`, `expires_at`, `is_active`, `cancelled_at` and `created_at`.
- `DELETE` cancels the rule (`is_active` becomes `false`, the rule stays listed) and answers `204`, even for an unknown id.
- Errors: `400 INVALID_BODY`, `400 VALIDATION_ERROR`.

---

## Webhooks

Webhook subscriptions deliver raw events to a URL. To notify a chat service, use a [channel](#notification-channels) instead: a webhook URL of Discord, Slack or Teams is refused. All routes are open in every edition.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/webhooks` | List webhook subscriptions |
| `POST` | `/api/v1/webhooks` | Create a webhook subscription |
| `DELETE` | `/api/v1/webhooks/{id}` | Delete a webhook subscription |
| `POST` | `/api/v1/webhooks/{id}/test` | Send a test payload |

- `POST` body: `name` (required, 1 to 100 characters), `url` (required, HTTPS, not resolving to a private or internal address unless `MAINTENANT_ALLOW_PRIVATE_WEBHOOKS` is set), `secret` (optional, enables the signature) and `event_types` (default `["*"]`). Valid event types are `*`, `container.state_changed`, `endpoint.status_changed`, `heartbeat.status_changed`, `certificate.status_changed`, `alert.fired` and `alert.resolved`. Answers `201` with the subscription (the secret is never returned).
- The list answers `{ "webhooks": [...] }`. A subscription has `id`, `name`, `url`, `event_types`, `is_active`, `last_delivery_status` (`delivered` or `failed`), `last_delivery_at`, `failure_count` (consecutive failures) and `created_at`.
- After 10 consecutive failed deliveries the subscription is deactivated (`is_active` becomes `false`). A successful delivery, a test included, resets `failure_count` and reactivates it.
- `test` sends `{"type": "test", "timestamp": "...", "data": {"message": "maintenant webhook test"}}` synchronously, with the headers and the signature of a real delivery, and records the outcome like one. It answers `200` with `{ "status": "delivered", "http_status": n }` or, on failure, `{ "status": "failed", "error": "..." }` plus `http_status` when the target answered.
- Errors use lower case codes: `400 invalid_json`, `400 invalid_input`, `404 not_found` and `500 internal_error`.

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

The `data` fields are those of the [SSE event](#sse-event-stream) of the same type. A subscription to `container.state_changed` also receives the `container.discovered` payloads, under that type, and a subscription to `endpoint.status_changed` also receives the `endpoint.discovered` and `endpoint.removed` payloads. Subscribe to specific types or `*` for all. A delivery is attempted up to three times, with a 1 second then a 5 second wait between attempts. The events sent to one URL are delivered one at a time in the order they happened, so a retry never lets a later event overtake an earlier one.

Headers on every delivery:

| Header | Value |
|--------|-------|
| `X-maintenant-Event` | the event `type` |
| `X-maintenant-Delivery` | a unique delivery UUID |
| `X-maintenant-Signature` | `sha256=<hmac>`, present only when the subscription has a secret |

When a secret is set, verify authenticity by computing `HMAC-SHA256(secret, raw_request_body)` and comparing (constant-time) against the hex digest in `X-maintenant-Signature`. The signature is computed over the exact bytes of the request body.

!!! note "Container recreation is noisy"
    A `docker compose up -d` that recreates a stack legitimately produces several `container.state_changed` deliveries per service (the old container goes `running → exited`, the new one `created → running`, plus health transitions). Filter on `data.state` / `data.previous_state` if you only care about specific transitions.

---

## Status Page (Admin)

The admin routes of the public status page. See [Status Page](../features/status-page.md). The routes below sit under `/api/v1/status/`; the public page is served under `/status/` (see [Public status page](#public-status-page)).

### Components

All routes are open in every edition, except that creating a component is capped.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/status/components` | List components | — |
| `POST` | `/api/v1/status/components` | Create a component (3 on Community) | — |
| `PUT` | `/api/v1/status/components/{id}` | Update a component | — |
| `DELETE` | `/api/v1/status/components/{id}` | Delete a component | — |

The list is a bare JSON array (`null` when empty), ordered by `display_order`. A component has `id`, `composition_mode` (`explicit` or `match-all`), `monitors` (`[{ "type", "id" }]`), `match_all_type`, `display_name`, `display_order`, `visible`, `derived_status`, `status_override`, `effective_status`, `auto_incident`, `needs_attention` and timestamps. Statuses are `operational`, `degraded`, `partial_outage`, `major_outage` and `under_maintenance`.

- `POST` body: `display_name` (required), `composition_mode` (default `explicit`), `monitors` (an `explicit` component needs at least one, each of type `container`, `endpoint`, `heartbeat` or `certificate`), `match_all_type` (required in `match-all` mode, empty otherwise), `display_order`, `visible` (default `true`) and `auto_incident`. `403 QUOTA_EXCEEDED` when the cap is reached (`resource: "status_components"`, `limit: 3` on Community).
- `PUT` takes the same fields, all optional. `composition_mode` and `match_all_type` cannot change, a `match-all` component's monitors cannot be edited, and `status_override` set to an empty string clears the override. A `status_override` must be one of the five statuses above, otherwise the answer is `400 validation`. While a maintenance window runs it forces `under_maintenance` on its components but remembers the override an operator had set: that override comes back when the last window holding the component ends.
- `DELETE` answers `204`.
- Creating, updating or deleting a component emits `status.component_created`, `status.component_updated` or `status.component_deleted` on the dashboard stream, and on the public stream followed by `status.global_changed` when the component is visible (or was visible before the update or delete), so open pages refresh. A hidden component never reaches the public stream.
- Errors (lower case codes): `400 invalid_body`, `400 validation`, `404 not_found` and `500 internal`.

### Incidents

Reading is open in every edition. Writing requires **Personal** (capability `incidents`).

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/status/incidents` | List all incidents | — |
| `POST` | `/api/v1/status/incidents` | Create an incident | Personal |
| `PUT` | `/api/v1/status/incidents/{id}` | Update an incident's title, severity or components | Personal |
| `DELETE` | `/api/v1/status/incidents/{id}` | Delete an incident | Personal |
| `POST` | `/api/v1/status/incidents/{id}/updates` | Add an incident update | Personal |

- `GET` takes `status`, `severity`, `limit` (1 to 100, default 20) and `offset`, and answers `{ "incidents": [...], "total" }`. An incident has `id`, `title`, `severity`, `status`, `is_maintenance`, `components`, `updates` and timestamps.
- `POST` body: `title` and `severity` (`minor`, `major` or `critical`) are required; `status` (`investigating`, `identified`, `monitoring` or `resolved`) defaults to `investigating`; `component_ids` and `message` (the first update) are optional. It emits `status.incident_created` and emails the confirmed subscribers when subscriptions are open.
- `PUT` takes `title`, `severity` and `component_ids`, all optional. Leaving `component_ids` out keeps the linked components, and an empty list removes them.
- `POST .../updates` takes `status` (one of the four statuses above) and `message` (both required). An update with status `resolved` resolves the incident and emits `status.incident_resolved`; any other update emits `status.incident_updated`.
- A `component_ids` list that holds an empty, unknown or repeated id is refused with `400 validation` before anything is written, on create and on update.
- Errors (lower case codes): `400 validation` (a missing field, a value outside the lists above or an invalid component id), `404 not_found`.

### Maintenance windows

Reading is open in every edition. Writing requires **Pro** (capability `maintenance_windows`).

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/status/maintenance` | List maintenance windows | — |
| `POST` | `/api/v1/status/maintenance` | Schedule a maintenance window | Pro |
| `PUT` | `/api/v1/status/maintenance/{id}` | Update a maintenance window | Pro |
| `DELETE` | `/api/v1/status/maintenance/{id}` | Delete a maintenance window | Pro |

- `GET` takes `status` (`upcoming`, `active` or `completed`) and `limit` (1 to 100, default 20) and answers a bare array (`null` when empty). Without `status`, running windows come first, then upcoming ones (soonest first), then completed ones (latest first).
- `POST` body: `title`, `starts_at` and `ends_at` (RFC 3339, `ends_at` not before `starts_at`) are required; `description` and `component_ids` are optional. An empty, unknown or repeated component id answers `400 validation`.
- `PUT` takes the same fields, all optional (leaving `component_ids` out keeps the linked components), and answers `400 validation` when the resulting `ends_at` (the new one, or the stored one when it is left out) is before `starts_at`, and `409 conflict` while the window is active. A scheduler checks the windows every 60 seconds: starting one opens a "Scheduled Maintenance" incident, puts its components under maintenance and emits `status.maintenance_started`; ending it emits `status.maintenance_ended`. When several windows hold the same component, it stays under maintenance until the last one ends.
- `DELETE` on a running window ends it the way its scheduled end would: the incident is resolved, the components are released and `status.maintenance_ended` is emitted.

### Subscribers

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/status/subscribers` | List email subscribers (addresses masked) | Pro |

Answers `{ "subscribers": [{ "id", "email", "confirmed", "created_at" }], "total", "confirmed" }`.

### SMTP test

Status page emails are sent through the SMTP server configured with the `MAINTENANT_SMTP_*` variables. There is no route to read or change that configuration.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `POST` | `/api/v1/status/smtp/test` | Send a test email | Personal |

Body: `{ "to": "address" }`. Answers `200 {"status": "sent"}`. Errors: `400 not_configured` (no SMTP host), `400 invalid_body`, `400 validation` (invalid address) and `502 smtp_failed` (the message is the SMTP error).

### Personalization

Branding of the public page. Every route requires **Pro** (capability `personalization`), reads included, and answers `403 EDITION_REQUIRED` below it. Errors use the codes `validation_error`, `payload_too_large`, `unsupported_mime`, `active_svg`, `not_found` and `internal_error`.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/status-page/settings` | Get the settings | Pro |
| `PUT` | `/api/v1/status-page/settings` | Replace the settings | Pro |
| `GET` | `/api/v1/status-page/assets/{role}` | Download an asset | Pro |
| `PUT` | `/api/v1/status-page/assets/{role}` | Upload an asset (multipart, part `file`, optional `alt_text`) | Pro |
| `DELETE` | `/api/v1/status-page/assets/{role}` | Delete an asset | Pro |
| `GET` | `/api/v1/status-page/footer-links` | List footer links | Pro |
| `POST` | `/api/v1/status-page/footer-links` | Add a footer link | Pro |
| `PUT` | `/api/v1/status-page/footer-links/order` | Reorder footer links (`{ "ids": [...] }`) | Pro |
| `PUT` | `/api/v1/status-page/footer-links/{id}` | Update a footer link | Pro |
| `DELETE` | `/api/v1/status-page/footer-links/{id}` | Delete a footer link | Pro |
| `GET` | `/api/v1/status-page/faq` | List FAQ items | Pro |
| `POST` | `/api/v1/status-page/faq` | Add an FAQ item | Pro |
| `PUT` | `/api/v1/status-page/faq/order` | Reorder FAQ items (`{ "ids": [...] }`) | Pro |
| `PUT` | `/api/v1/status-page/faq/{id}` | Update an FAQ item | Pro |
| `DELETE` | `/api/v1/status-page/faq/{id}` | Delete an FAQ item | Pro |

- **Settings** (`PUT` replaces every field): `title` (1 to 100 characters), `subtitle` (200), `colors` (`bg`, `surface`, `border`, `text`, `accent`, `status_operational`, `status_degraded`, `status_partial`, `status_major`, each `#RRGGBB` or `#RRGGBBAA`), `announcement` (`enabled`, `message_md` up to 1000 characters, `url` over HTTP or HTTPS), `footer_text_md` (500), `locale` (`en` or `fr`), `timezone` (an IANA name or empty) and `date_format` (`relative` or `absolute`). Markdown is rendered to sanitized HTML on the server. The answer adds `version` and, when the palette misses WCAG AA contrast, a `warnings.contrast` list that never blocks the save.
- **Assets**: `role` is `logo` (200 KiB; PNG, JPEG, WebP or SVG), `favicon` (50 KiB; PNG, ICO or SVG) or `hero` (500 KiB; PNG, JPEG or WebP). The type is sniffed from the content. An SVG is read whole and needs no XML declaration, but one that carries a script, an event handler, a `javascript:` link, a `foreignObject` or embedded HTML is refused. An oversized file answers `400 payload_too_large`, a wrong type `400 unsupported_mime` and an SVG with active content `400 active_svg`.
- **Footer links** take `label` (1 to 60 characters) and `url` (HTTP or HTTPS). **FAQ items** take `question` (1 to 200 characters) and `answer_md` (up to 4000). Lists answer `{ "items": [...] }`.

---

## Public status page

These routes need no credentials, are not under `/api/`, share the 10 requests per second bucket of the public surfaces and cap bodies at 4 KiB. The page can be framed from any origin. See [Status Page](../features/status-page.md).

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/status/` | The status page (the dashboard application at the status route). `/status` redirects to it |
| `GET` | `/status/api` | JSON snapshot of the current status |
| `GET` | `/status/events` | Event stream of the status page (SSE) |
| `GET` | `/status/feed.atom` | Atom feed of the ongoing incidents and of those resolved in the last 30 days |
| `GET` | `/status/settings.json` | Branding for the page: colors, assets, footer, FAQ |
| `POST` | `/status/subscribe` | Subscribe an email address to updates |
| `GET` | `/status/confirm?token=` | Confirm a subscription |
| `GET` | `/status/unsubscribe?token=` | Unsubscribe |

- `GET /status/api` answers `global_status`, `global_message`, `updated_at`, `components` (`[{ "id", "name", "status", "monitors" }]`, visible components only; `monitors` is `[{ "type", "id", "name", "status" }]`), `active_incidents`, `upcoming_maintenance` (the next five windows, soonest first), `subscriptions_enabled` and, once the page has been personalized, `personalization_version`. List fields are `null` when empty. It sends `Access-Control-Allow-Origin: *`.
- Incident and maintenance `components` lists on the public surfaces (this route, the stream, the Atom feed and the notification emails) name visible components only: a hidden component is never named, and an incident opened automatically by a hidden component's alert is not created. The Atom feed builds its links from `MAINTENANT_STATUS_URL`, or from `MAINTENANT_BASE_URL` followed by `/status` when it is not set, never from the `Host` header of the request.
- `GET /status/settings.json` returns the branding with images inlined as `data:` URLs (there is no public asset URL). Below Pro it returns the default settings. It sends an `ETag` (`"v<version>"`), answers `304` to a matching `If-None-Match` and sends `Access-Control-Allow-Origin: *`.
- `POST /status/subscribe` takes `{ "email": "..." }` (`Content-Type: application/json`) or a form field `email` (`application/x-www-form-urlencoded`). Any other content type answers `415 unsupported_media_type`. Subscriptions are open when SMTP is configured and the edition is **Pro**; otherwise the route answers `503 subscriptions_unavailable`. The answer is always `200 {"status": "confirmation_sent"}`, whether the address is new, pending or already confirmed, so nothing reveals who is subscribed. A pending address gets a new link, which replaces the old one, and a confirmed address gets no mail. Errors: `400 invalid_email`, `400 invalid_body`, `413 body_too_large`, `429 rate_limited` (5 per hour and per IP, with `Retry-After`) and `500 subscription_failed`.
- `confirm` and `unsubscribe` answer small HTML pages. The link in the confirmation mail is valid for 24 hours. `confirm` answers `503` while subscriptions are closed; `unsubscribe` keeps working.
- Plain-text errors on the page, API and feed routes: `503 Status page not available` when the application is not embedded and `500 Internal Server Error`.

---

## Updates

All routes are open in every edition. Two response fields depend on the edition: `cve_counts` in the summary and the changelog and CVE fields of a container's update (Personal).

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/updates` | List updates (`?status=&update_type=`) |
| `GET` | `/api/v1/updates/summary` | Update summary with counts (`os_counts` for host operating systems) |
| `GET` | `/api/v1/updates/hosts` | Every monitored host with its OS identity and end-of-support status, plus the support table in use |
| `POST` | `/api/v1/updates/scan` | Trigger a manual scan |
| `GET` | `/api/v1/updates/scan/{scan_id}` | Get scan status |
| `GET` | `/api/v1/updates/dry-run` | Dry run: list the available updates |
| `GET` | `/api/v1/updates/container/{container_id}` | Update details for a container |
| `POST` | `/api/v1/updates/pin/{container_id}` | Pin the current version |
| `DELETE` | `/api/v1/updates/pin/{container_id}` | Unpin the version |
| `GET` | `/api/v1/updates/exclusions` | List exclusions |
| `POST` | `/api/v1/updates/exclusions` | Create an exclusion |
| `DELETE` | `/api/v1/updates/exclusions/{id}` | Delete an exclusion |

`{container_id}` is the container's external id (the runtime id; for Kubernetes `namespace/Kind/name`, slashes included).

- `GET /api/v1/updates` takes `status` (`available` or `pinned`) and `update_type` (`major`, `minor`, `patch`, `digest_only` or `unknown`). The answer is `{ "updates": [...], "last_scan", "next_scan" }` (both dates are the zero time until a scan finishes). An update has `id`, `container_id`, `container_name`, `image`, `current_tag`, `current_digest`, `latest_tag`, `latest_digest`, `update_type`, `risk_score`, `status`, `detected_at`, and `pin_reason` when pinned. `risk_score` (Personal and above) adds up at most 75 points: update type (up to 20), CVE severity (30), network exposure (10, from the container's security insights), restarts over the last 24 hours (10) and breaking changes (5). Container criticality and dependents no longer count.
- `summary` answers `last_scan`, `next_scan`, `scan_status` (`running`, `completed`, `failed` or `idle`), `counts` (`critical`, `recommended`, `available`, `up_to_date`, `pinned`), `cve_counts` (Personal) and `os_counts` (`ended`, `ending_soon`, `unknown`, `untracked`, `supported`).
- `hosts` answers `{ "hosts": [...], "eol_table": { "source", "fetched_at", "refresh_enabled", "last_refresh_error" } }`. A host has `agent_id`, `hostname`, `label`, `is_local`, `runtime`, `connection_state` and an `os` object with its `support` state (`unknown`, `untracked`, `supported`, `security_only`, `ending_soon` or `ended`).
- `scan` answers `202 {"status": "running", "started_at": "..."}` without a scan id: the id arrives in the `update.scan_started` event. `409 SCAN_IN_PROGRESS` when a scan is running.
- `scan/{scan_id}` answers `scan_id`, `status`, `started_at`, `containers_scanned`, `updates_found`, `errors` and `completed_at`; `404 NOT_FOUND` for an unknown scan.
- `dry-run` answers `{ "would_update": [{ "container_id", "container_name", "image", "current_tag", "latest_tag", "update_type" }] }`, the updates with status `available`.
- `container/{container_id}` adds `pinned`, `pin_reason`, `update_command`, `rollback_command`, `tag_include` and `tag_exclude`. The commands are shell commands that depend on the workload: Compose or standalone Docker commands, `docker service update --image <reference> <service>` for a Swarm task, `kubectl set image` for a Kubernetes workload or a bare pod. For a Swarm task or a Kubernetes workload whose tag was republished under the same name, the command sets `<tag>@<digest>`, because setting an unchanged reference rolls nothing out. On Personal and above it also carries `source_url`, `previous_digest`, `changelog_url`, `changelog_summary`, `has_breaking_changes` and `active_cves`. Errors: `404 NOT_FOUND`.
- `pin` takes an optional `{ "reason": "..." }` and answers `200 { "container_id", "pinned_tag", "pinned_digest", "reason", "pinned_at" }`; `404 NOT_FOUND` when the container has no update data. `DELETE` always answers `204`.
- `exclusions` (POST) takes `pattern` (required) and `pattern_type` (`image` or `tag`), and answers `201`. Posting a pattern that already exists answers `200` with the stored exclusion. Errors: `400 INVALID_JSON`, `400 INVALID_PATTERN`, `400 INVALID_TYPE`.

---

## Security

Insights are open in every edition.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/security/insights` | List all insights, by container | — |
| `GET` | `/api/v1/security/insights/{container_id}` | Insights of a container | — |
| `GET` | `/api/v1/security/summary` | Aggregated counts by severity and type | — |

The list answers `{ "containers": [...], "summary": {...} }` and the summary route answers the same `summary` object (`total_containers_monitored`, `total_containers_affected`, `total_insights`, `by_severity`, `by_type`). An insight has `type` (`port_exposed_all_interfaces`, `database_port_exposed`, `privileged_container`, `host_network_mode`, `service_load_balancer` or `service_node_port`), `severity` (`critical`, `high` or `medium`), `container_id`, `container_name`, `title`, `description`, `details` and `detected_at`. `{container_id}` is the maintenant container id; an unknown one answers `404 CONTAINER_NOT_FOUND`.

### Security Posture

Requires **Personal** (capability `security_posture`). Every route answers `403 EDITION_REQUIRED` below it.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/security/posture` | Global infrastructure posture score | Personal |
| `GET` | `/api/v1/security/posture/containers` | Per-container posture scores (`?limit=&offset=`) | Personal |
| `GET` | `/api/v1/security/posture/containers/{container_id}` | Posture score for one container | Personal |
| `POST` | `/api/v1/security/acknowledgments` | Acknowledge a finding | Personal |
| `GET` | `/api/v1/security/acknowledgments` | List acknowledgments (`?container_id=`) | Personal |
| `DELETE` | `/api/v1/security/acknowledgments/{id}` | Revoke an acknowledgment | Personal |

- The global score has `score`, `color` (`green` from 80, `yellow` from 60, `orange` from 40, else `red`), `container_count`, `scored_count`, `is_partial`, `categories` (`tls`, `cves`, `updates`, `network_exposure`, `image_age`), `top_risks` and `computed_at`. Every scoring of the infrastructure evaluates the posture threshold alert (`MAINTENANT_SECURITY_SCORE_THRESHOLD`): a read of this route, the MCP posture tool, and, when a threshold is set, a background check every 5 minutes.
- `containers` takes `limit` (default 50) and `offset` and answers `{ "containers", "total", "limit", "offset" }`, worst score first. All containers are scored in one batch (certificates and updates are read once, not once per container); a scoring failure answers `500 INTERNAL_ERROR` instead of dropping containers from the list.
- `POST acknowledgments` takes `container_id` and `finding_type` (required), `finding_key`, `acknowledged_by` and `reason`. Answers `201`. Errors: `400 INVALID_BODY`, `400 MISSING_FIELDS`, `404 NOT_FOUND`, `409 ALREADY_ACKNOWLEDGED`.

---

## CVE Intelligence

Requires **Personal** (capability `cve_enrichment`). The check is made by the handler, which answers `403 EDITION_REQUIRED` below it.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/cve` | List known CVEs across all containers (`?severity=&container_id=`) | Personal |
| `GET` | `/api/v1/cve/{container_id}` | List CVEs for a specific container | Personal |

`{container_id}` is the external id. The list answers `{ "cves": [{ "cve_id", "cvss_score", "severity", "summary", "first_detected_at", "affected_containers" }], "total", "by_severity" }`; `total` and `by_severity` count container-CVE pairs, not distinct CVEs. The container route answers `{ "container_id", "cves": [...] }`.

---

## Risk Scoring

Requires **Personal** (capability `risk_scoring`). Every route answers `403 EDITION_REQUIRED` below it. Error codes here are lower case.

| Method | Endpoint | Description | Edition |
|--------|----------|-------------|:-------:|
| `GET` | `/api/v1/risk` | Risk scores for all containers | Personal |
| `GET` | `/api/v1/risk/{container_id}` | Risk score of a container; with `?period=24h\|7d\|30d`, its score history | Personal |

The list answers `{ "containers": [{ "container_id", "container_name", "risk_score", "level" }], "host_risk_score", "host_risk_level" }`. `level` is `critical` from 81, `high` from 61, `moderate` from 31, else `low`. With `period`, the container route answers `{ "container_id", "history": [{ "score", "recorded_at" }] }`. Errors: `400 invalid_period`, `404 not_found` (no risk data), `500 internal`.

---

## Dashboard

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/dashboard/sparklines` | Response times of the last 20 checks of every active endpoint |

Answers an object keyed `endpoint:<endpoint_id>`, each value an array of response times in milliseconds, oldest first. Open in every edition.

---

## MCP and OAuth routes

The MCP server is off unless `MAINTENANT_MCP` is enabled, and none of these routes exist in demo mode. When `MAINTENANT_MCP_CLIENT_ID` and `MAINTENANT_MCP_CLIENT_SECRET` are set, `/mcp` requires an OAuth access token and the `/.well-known/` and `/oauth/` routes exist. Without them the instance refuses to start, unless `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED` leaves `/mcp` open, in which case only `/mcp` is served. See [MCP Server](../features/mcp.md).

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET`, `POST`, `DELETE` | `/mcp`, `/mcp/` | MCP over streamable HTTP. Needs `Authorization: Bearer <token>` when OAuth is configured |
| `GET` | `/.well-known/oauth-authorization-server` | Authorization server metadata |
| `GET` | `/.well-known/oauth-protected-resource` | Protected resource metadata for `/mcp` |
| `GET` | `/oauth/authorize` | Authorization endpoint (authorization code with PKCE) |
| `POST` | `/oauth/token` | Token endpoint |

- `/oauth/authorize` takes `response_type=code`, `client_id`, `redirect_uri`, `code_challenge`, `code_challenge_method=S256` and an optional `state`. It asks for no login: a valid request is approved at once and redirected with a `code`. The redirect URI must be a loopback address or one of `MAINTENANT_MCP_ALLOWED_REDIRECT_URIS`. The code lives 10 minutes and works once.
- `/oauth/token` takes form fields. `grant_type=authorization_code` needs `code`, `client_id`, `client_secret`, `redirect_uri` and `code_verifier`; `grant_type=refresh_token` needs `refresh_token`, `client_id` and `client_secret`. The client authenticates with its secret in the form (`client_secret_post`). The answer is `{ "access_token", "token_type": "Bearer", "expires_in": 3600, "refresh_token" }`: access tokens last 1 hour, refresh tokens 30 days, and each refresh token is used once.
- OAuth errors use the RFC 6749 shape: `invalid_request`, `invalid_client` (401), `invalid_grant`, `unsupported_grant_type`, `unsupported_response_type`, `unauthorized_client` and `server_error`. `/mcp` answers `401` with a `WWW-Authenticate` header pointing to the protected resource metadata when the token is missing, expired or revoked.
- The issuer of the metadata is `MAINTENANT_BASE_URL`.

---

## SSE Event Stream

Connect to the real-time event stream:

```
GET /api/v1/containers/events
```

This is a Server-Sent Events (SSE) endpoint. The server writes each event as:

```
event: container.state_changed
data: {"id":"a1b2c3d4e5f6","state":"running",...}
```

The SSE `event:` field is the event type, and `data:` holds the JSON payload only (there is no `{type, data}` envelope). There is no `id:` field and no event on connection: a client only receives events broadcast after it connects, and refetches state after a reconnect. While the stream is silent, the server sends the comment `: keepalive` every 25 seconds so that a proxy does not cut it; clients ignore comments. The `status.*` events of the status page are sent on this stream as well. A client that falls 64 events behind loses events instead of slowing the others. The response sets `Cache-Control: no-cache` and `X-Accel-Buffering: no`.

### Event Types

| Event | Emitted when | Payload keys |
|-------|--------------|--------------|
| `container.discovered` | A container the store did not know appears (not for ignored containers) | the container object |
| `container.state_changed` | A container changes state | `id`, `state`, `previous_state`, `health_status`, `exit_code`, `timestamp`, `agent_id` |
| `container.health_changed` | A container's health status changes | `id`, `container_name`, `health_status`, `previous_health`, `timestamp`, `agent_id` |
| `container.archived` | A container is destroyed, vanished while offline, missing from an agent inventory or deleted through the API | `id`, `archived_at`, `agent_id` (the API delete sends `id` only) |
| `container.restart_alert` | A restart loop is detected | `container_id`, `container_name`, `restart_count`, `threshold`, `severity`, `timestamp`, `agent_id` |
| `container.restart_recovery` | The restart rate is back under the threshold | `container_id`, `container_name`, `timestamp`, `agent_id` |
| `endpoint.discovered` | An endpoint is discovered from labels or created through the API | `endpoint_id`, `container_name`, `endpoint_type`, `target`, plus `agent_id` for an agent's endpoint and `source` (`standalone`) and `name` for a standalone one |
| `endpoint.status_changed` | An endpoint changes status after a check, or becomes `unknown` because its container stopped | `endpoint_id`, `container_name`, `target`, `previous_status`, `new_status`, `error`, `timestamp`, plus `response_time_ms`, `http_status` and `agent_id` after a check |
| `endpoint.removed` | An endpoint is deactivated or deleted | `endpoint_id`, `reason` (`label_removed`, `container_destroyed`, `container_gone` or `user_deleted`), plus `container_name` and `agent_id` when they apply |
| `endpoint.alert` | An endpoint reaches its failure threshold | `endpoint_id`, `container_name`, `target`, `consecutive_failures`, `threshold`, `last_error`, `timestamp` |
| `endpoint.recovery` | An alerting endpoint reaches its recovery threshold | `endpoint_id`, `container_name`, `target`, `consecutive_successes`, `threshold`, `timestamp` |
| `endpoint.config_error` | An endpoint label cannot be parsed | `endpoint_id` (`null`), `container_name`, `label_key`, `error`, `timestamp`, plus `agent_id` for an agent's container |
| `heartbeat.created` | A heartbeat monitor is created | `heartbeat_id`, `name`, `status` |
| `heartbeat.ping_received` | A ping arrives (`ping_type`: `success`, `start` or `exit_code`) | `heartbeat_id`, `ping_type`, `status`, plus `exit_code` or `agent_id` |
| `heartbeat.status_changed` | A heartbeat changes status (ping, missed deadline, pause, resume) | `heartbeat_id`, `old_status`, `new_status`, `agent_id` on a ping |
| `heartbeat.alert` | A deadline is missed or a ping carries a non-zero exit code | `heartbeat_id`, `name`, `alert_type`, `details` |
| `heartbeat.recovery` | A failing heartbeat recovers | `heartbeat_id`, `name`, `agent_id` on a ping |
| `heartbeat.deleted` | A heartbeat monitor is deleted | `heartbeat_id` |
| `certificate.created` | A certificate monitor is created or auto-detected | `monitor_id`, `hostname`, `port`, `source`, plus `server_name` and `agent_id` when they apply |
| `certificate.check_completed` | A check finishes, including failed ones | `monitor_id`, `hostname`, `status`, `checked_at`, plus `subject_cn`, `issuer_cn`, `not_after`, `days_remaining`, `chain_valid`, `hostname_match` when known |
| `certificate.status_changed` | A monitor changes status | `monitor_id`, `hostname`, `previous_status`, `new_status`, `days_remaining`, `timestamp` |
| `certificate.alert` | Expiry threshold, invalid chain, hostname mismatch, revoked OCSP response or expiry | `monitor_id`, `hostname`, `port`, `alert_type`, `severity`, `timestamp` and details |
| `certificate.recovery` | A certificate alert clears: the certificate was renewed, or the chain, hostname or OCSP problem is gone | `monitor_id`, `hostname`, `port`, `previous_alert_type` (`expiring`, `expired`, `chain_invalid`, `hostname_mismatch` or `ocsp_revoked`), `days_remaining`, `timestamp`, plus `new_not_after` and `server_name` when they apply |
| `certificate.deleted` | A certificate monitor is deleted | `monitor_id`, `hostname` |
| `resource.snapshot` | A sample is stored (live samples only) | `container_id`, `cpu_percent`, `mem_used`, `mem_limit`, `mem_percent`, `net_rx_bytes`, `net_tx_bytes`, `block_read_bytes`, `block_write_bytes`, `timestamp`, `agent_id` |
| `resource.alert` | CPU or memory stays over its threshold (one event per metric) | `container_id`, `container_name`, `alert_type` (`cpu` or `memory`), `current_value`, `threshold`, `timestamp` |
| `resource.recovery` | CPU or memory returns to normal (one event per metric) | `container_id`, `container_name`, `recovered_type` (`cpu` or `memory`), `current_value`, `threshold`, `timestamp` |
| `alert.fired` | An alert is raised, or its severity escalates | the alert: `id`, `source`, `alert_type`, `severity`, `status`, `message`, `entity_type`, `entity_id`, `entity_name`, `details` (an object), `fired_at`, `created_at` |
| `alert.silenced` | An alert is raised while a silence rule or a maintenance window matches | same as `alert.fired` |
| `alert.resolved` | An alert resolves | same as `alert.fired`, with `resolved_at` |
| `alert.acknowledged` | An alert is acknowledged, from the REST route, the MCP server or a security posture acknowledgment | the alert as stored: `details` is a string holding JSON |
| `channel.created`, `channel.updated` | A channel changes (REST or MCP) | the channel object |
| `channel.deleted` | A channel is deleted | `id` |
| `trigger.created`, `trigger.updated` | A trigger changes | the trigger object |
| `trigger.deleted` | A trigger is deleted | `id` |
| `silence.created` | A silence rule is created | the rule object |
| `silence.cancelled` | A silence rule is cancelled | `id` |
| `runtime.context_changed` | Swarm is activated or deactivated (deactivating Swarm also stops the Swarm manager loops) | `previous`, `current`, `message`, `detected_at` |
| `runtime.availability_changed` | The runtime connects or is lost (the Docker event stream closing once the daemon stops answering counts as a loss), and once when the server starts | `name`, `connected` |
| `storage.availability_changed` | The database becomes unreachable, or answers again | `engine`, `connected` |
| `update.scan_started` | A scan starts | `scan_id`, `started_at` |
| `update.scan_completed` | A scan ends | `scan_id`, `updates_found`, `errors` |
| `update.detected` | A scan finds an update (on every scan, for each container) | `container_id`, `container_uid`, `container_name`, `image`, `current_tag`, `latest_tag`, `update_type`, `risk_score`, `alert_on`, plus `update_command`, `rollback_command` |
| `update.resolved` | An update is no longer pending | `container_id`, `container_uid`, `container_name` |
| `security.insights_changed` | A container's insights change | `container_id`, `container_name`, `highest_severity`, `count`, `change` |
| `security.insights_resolved` | All insights of a container are gone | `container_id`, `container_name` |
| `security.posture_changed` | The posture score moved by 5 points or more, or changed colour (needs `MAINTENANT_SECURITY_SCORE_THRESHOLD`; evaluated at every scoring) | `score`, `previous_score`, `color` |
| `swarm.service_discovered` | A Swarm service is created | `service_id`, `name`, `mode`, `desired_replicas`, `stack_name`, `image` |
| `swarm.service_updated` | A Swarm service is updated, or stays under-replicated for 5 minutes | `service_id`, `name`, `desired_replicas`, `running_replicas`, plus `image` for an update and `replica_alert` for under-replication |
| `swarm.service_removed` | A Swarm service is removed | `service_id`, `name` |
| `swarm.status` | The Swarm state this node manages changes: Swarm is activated (`active: true`) or deactivated (`active: false`, no other key), or the manager or worker counts move. It is not sent at startup, before any client listens | `active`, plus `is_manager`, `cluster_id`, `manager_count`, `worker_count` when active |
| `swarm.node_status_changed` | A node's status or availability changes | `node_id`, `hostname`, `role`, `old_status`, `new_status`, `old_availability`, `new_availability` |
| `swarm.node_updated` | A node joins, or its role, host name, engine version or address changes | `node_id`, `hostname`, `role`, `status`, `availability`, `engine_version`, `address`, `task_count` |
| `swarm.task_failed` | Swarm marks a task failed, on any node, with an exit code other than 0 and 143 (137 counts). Tasks stopped by a rolling update or a scale-down do not count | `task_id`, `service_id`, `service_name`, `node_id`, `container_id`, `error`, `exit_code`, `timestamp` |
| `swarm.crash_loop_detected` | A service has 3 failed tasks within 5 minutes | `service_id`, `service_name`, `failure_count`, `window_minutes`, `last_error`, `timestamp` |
| `swarm.crash_loop_recovered` | A crash-looping service is stable for 10 minutes | `service_id`, `service_name`, `timestamp` |
| `swarm.update_progress` | A rolling update progresses | `service_id`, `service_name`, `state`, `tasks_updated`, `tasks_total`, `new_image`, `message`, `timestamp` |
| `swarm.update_completed` | A rolling update completes or rolls back | `service_id`, `service_name`, `state`, `message`, `started_at`, `completed_at` |
| `swarm.topology_changed` | A remote agent reported a new Swarm topology | `agent_id` |
| `kubernetes.topology_changed` | A remote agent reported a new Kubernetes topology | `agent_id` |
| `kubernetes.workload_changed` | An alert of a workload of the server's own cluster is raised or resolved | `id`, `namespace`, `name` |
| `kubernetes.pod_changed` | An alert of a pod of the server's own cluster is raised or resolved | `namespace`, `name` |
| `kubernetes.node_changed` | An alert of a node of the server's own cluster is raised or resolved | `name` |
| `agent.created` | An agent enrols | `agent_id`, `hostname`, `label`, `runtime`, `status` |
| `agent.updated` | An agent's label or host OS changes | `agent_id`, `label` for a label change, the agent object for a host OS change |
| `agent.revoked`, `agent.deleted` | An agent is revoked or deleted | `agent_id` |
| `agent.connected`, `agent.disconnected` | An agent stream opens or closes (or goes stale) | `agent_id` |

Events carrying `agent_id` use the sentinel `00000000-0000-0000-0000-000000000000` for the local runtime. Data that a remote agent replays from its spool after a reconnection is stored for history and does not raise events.

!!! note "Events the server defines but does not send"

    `runtime.status`, `update.pinned` and `update.unpinned` are declared but never emitted.

### Container log stream

`GET /api/v1/containers/{id}/logs/stream` is a separate stream per container (see [Containers](#containers)). It sends `container.log_line` and, at the end, `container.log_error`.

### Status page stream

The public status page has its own event stream, separate from the one above:

```
GET /status/events
```

It carries only the status page events, so the public page never receives dashboard data. The dashboard stream above receives the same events without the visibility filter: it names every component.

| Event | Emitted when | Payload keys |
|-------|--------------|--------------|
| `status.component_changed` | A monitor linked to a component changes state. Sent to the public stream only for a visible component | `component_id`, `name`, `status`, `monitors` |
| `status.component_created`, `status.component_updated`, `status.component_deleted` | An administrator creates, edits or deletes a component | `component_id` only. Sent to the public stream only when the component is visible, or was before the change |
| `status.global_changed` | Right after every `status.component_changed`, `status.component_created`, `status.component_updated` and `status.component_deleted` | `status`, `message` |
| `status.incident_created` | An incident is created, manually or automatically | `id`, `title`, `severity`, `status`, `components` (visible components only on the public stream) |
| `status.incident_updated` | An update is added to an incident | `id`, `status`, `message` |
| `status.incident_resolved` | An incident is resolved | `id`, `title` |
| `status.maintenance_started` | A maintenance window starts | `id`, `title`, `components` (visible components only on the public stream) |
| `status.maintenance_ended` | A maintenance window ends | `id`, `title`, `components` (visible components only on the public stream) |

# Heartbeat & Cron Monitoring

Monitor cron jobs, scheduled tasks, and any periodic process. Create a monitor, get a unique URL, add one `curl` to your script. maintenant tracks start/finish times, durations, exit codes, and alerts you when a job misses its deadline.

---

## How It Works

1. **Create a heartbeat monitor** through the API or dashboard: give it a name, an interval (how often the job runs, from 60 seconds to 7 days) and a grace period (extra time allowed, from 0 up to the interval).
2. **Get a unique ping URL**: maintenant generates a UUID-based URL for this monitor.
3. **Ping the URL** from your cron job or script: maintenant records the ping and moves the deadline to *now + interval + grace*.
4. **Get alerted** if the deadline is missed: the job did not report in on time.

Deadlines are checked every 15 seconds. A new heartbeat has the status `new` and is not watched until its first ping, since the deadline starts from a ping: a job that never runs at all does not raise an alert. Send one ping when you create the monitor, or run the job once.

---

## Ping URL Format

Every heartbeat monitor gets a unique URL:

```
{BASE_URL}/ping/{uuid}
```

Where `{BASE_URL}` is your `MAINTENANT_BASE_URL` environment variable. The ready-made snippets (curl, wget, Python, Go, Bash and a Docker healthcheck) shown in a heartbeat's detail panel, and returned by `GET /api/v1/heartbeats/{id}`, are built from it. It defaults to `http://` plus `MAINTENANT_ADDR` (`http://127.0.0.1:8080`), so set it to the public address of your instance or the snippets will not work from another machine.

### Simple Ping

Report that the job ran successfully:

```bash
curl -fsS -o /dev/null https://now.example.com/ping/{uuid}
```

### Ping with Exit Code

Report the job's exit code so maintenant can track failures:

```bash
curl -fsS -o /dev/null https://now.example.com/ping/{uuid}/$?
```

- Exit code `0` = success
- Any other exit code (an integer from 1 to 255) = failure
- Anything else answers `400 INVALID_EXIT_CODE`

### Start/Finish Pings

Track job duration by sending a start ping before the job and a finish ping after:

```bash
# Signal job start
curl -fsS -o /dev/null https://now.example.com/ping/{uuid}/start

# Run the actual job
/usr/local/bin/my-backup.sh
EXIT_CODE=$?

# Signal job finish with exit code
curl -fsS -o /dev/null https://now.example.com/ping/{uuid}/${EXIT_CODE}
```

maintenant calculates the duration between start and finish pings. A start ping also moves the deadline, so a job that starts and never finishes raises an alert once the deadline passes. Starting again while a run is still open closes the previous run as `timeout`.

All ping routes accept `GET` and `POST`. A `POST` body of up to 10 KB is accepted but not stored. An unknown UUID answers `404 HEARTBEAT_NOT_FOUND`.

---

## Cron Job Examples

### Basic Cron Entry

```bash
# Run backup every day at 2 AM, report to maintenant
0 2 * * * /usr/local/bin/backup.sh && curl -fsS -o /dev/null https://now.example.com/ping/{uuid}/$?
```

### With Duration Tracking

```bash
# Report start and finish with exit code
0 2 * * * curl -fsS -o /dev/null https://now.example.com/ping/{uuid}/start; /usr/local/bin/backup.sh; curl -fsS -o /dev/null https://now.example.com/ping/{uuid}/$?
```

### Systemd Timer

```bash
# In your service ExecStartPost or a wrapper script
ExecStartPost=/usr/bin/curl -fsS -o /dev/null https://now.example.com/ping/{uuid}/0
```

---

## What maintenant Tracks

For each heartbeat monitor, maintenant records:

| Metric | Description |
|--------|-------------|
| **Last ping** | Timestamp of the most recent ping |
| **Exit code** | Exit code reported by the job (0 = success) |
| **Duration** | Time between start and finish pings |
| **Status** | `new` (no ping yet), `started` (a start ping is waiting for its finish), `up` (pinging on time), `down` (deadline missed), `paused` |
| **Next deadline** | Last ping (or start ping) + interval + grace |
| **Execution history** | Past executions with their outcome: `success`, `failure`, `timeout` or `in_progress` |

A ping that reports a non-zero exit code leaves the status at `up`, since the job did report in, and raises the `exit_code_failure` alert instead. Only a missed deadline turns the status to `down`.

Pings and executions are kept for 30 days. `GET /api/v1/heartbeats/{id}/uptime/daily?days=90` returns one value per UTC day for up to 365 days (`days` defaults to 90): the share of finished runs that succeeded that day, where a plain ping or an exit code `0` succeeds and a start ping is not counted. A day with no finished run has no value, and a missed deadline produces no ping, so it does not lower the percentage by itself. Completed days are aggregated once they end, which is what keeps the 365 days available after the raw pings are purged.

---

## Deadline Missed Alerts

When a heartbeat monitor does not receive a ping within its configured deadline, maintenant fires a `deadline_missed` alert with **Critical** severity. A ping reporting a non-zero exit code fires an `exit_code_failure` alert instead, also Critical. Both resolve automatically, with a recovery notification, on the next successful ping; deleting or pausing a heartbeat clears its active alerts too.

This means your cron job either:

- Failed to run at all
- Ran but crashed before reaching the `curl` ping
- Is taking longer than expected

!!! tip "Set reasonable deadlines"
    The deadline is the interval plus the grace period. Set the grace slightly
    longer than your expected job duration: a job that runs every 5 minutes with
    a 1-minute runtime should have a grace of 1 to 2 minutes to avoid false
    positives.

---

## Managing Heartbeats

### Pause and Resume

Temporarily disable a heartbeat monitor during planned maintenance:

```bash
# Pause: stops deadline checking and clears active alerts
POST /api/v1/heartbeats/{id}/pause

# Resume: back to up, with a new deadline counted from now
POST /api/v1/heartbeats/{id}/resume
```

Resuming a heartbeat that is not paused answers `400 INVALID_INPUT`. A ping that arrives while the heartbeat is paused puts it back to `up`.

### CRUD Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/heartbeats` | List all heartbeat monitors (filters `status`, `agent_id`) |
| `POST` | `/api/v1/heartbeats` | Create a new heartbeat monitor (`name`, `interval_seconds`, `grace_seconds`) |
| `GET` | `/api/v1/heartbeats/{id}` | Get a specific monitor, with its snippets |
| `PUT` | `/api/v1/heartbeats/{id}` | Update a monitor |
| `DELETE` | `/api/v1/heartbeats/{id}` | Delete a monitor: its ping URL answers 404 from then on |
| `GET` | `/api/v1/heartbeats/{id}/executions` | Execution history |
| `GET` | `/api/v1/heartbeats/{id}/pings` | Raw pings |
| `GET` | `/api/v1/heartbeats/{id}/uptime/daily` | Daily uptime |

Community is limited to **5 heartbeats**; Personal and Pro have no cap. Above the cap, creation answers `403 QUOTA_EXCEEDED`.

---

## Outbound Heartbeats

A monitoring tool cannot report its own outage. Outbound heartbeats solve this by letting another maintenant instance watch this one: this instance pings a heartbeat monitor on the other instance at a fixed interval, and if the pings stop, the other instance raises a `deadline_missed` alert.

### Setting It Up

1. On the **other** instance, create a heartbeat monitor with an interval that matches the one you will use below, plus some grace time. Copy its ping URL.
2. On **this** instance, open **Outbound heartbeats** in the sidebar menu and click **New target**.
3. Paste the ping URL, give the target a name and choose an interval.
4. Click **Send now** to check the target right away. The row shows the HTTP status it received, or the error.

The first send happens within seconds of creating the target. Two instances can watch each other this way, each with one incoming and one outgoing heartbeat.

### How Sends Work

- Each target is called with a `GET` request, with a 10-second timeout and the `User-Agent` `maintenant/<version> outbound-heartbeat`.
- The interval goes from 30 seconds to 24 hours.
- A 2xx response counts as a success. Any other status, or a network error, is recorded as the target's last error and logged as a warning.
- A disabled target keeps its settings but is no longer called.
- An extra root CA set with `MAINTENANT_CA_CERT` applies, so the other instance may use an internal PKI.
- The page is not available in demo mode.

!!! note "HTTPS and public addresses only"
    Targets must use `https://` and resolve to a public address. Loopback, private,
    link-local and carrier-grade NAT ranges are refused when the target is saved and
    again at every connection, including redirects and DNS answers that change
    between the two, so the feature cannot be used to reach services on your
    internal network. `MAINTENANT_ALLOW_PRIVATE_WEBHOOKS` does not lift this
    restriction. The other instance therefore has to be reachable over HTTPS from
    this one.

### API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/outbound-heartbeats` | List all targets |
| `POST` | `/api/v1/outbound-heartbeats` | Create a target |
| `PUT` | `/api/v1/outbound-heartbeats/{id}` | Update a target |
| `DELETE` | `/api/v1/outbound-heartbeats/{id}` | Delete a target |
| `POST` | `/api/v1/outbound-heartbeats/{id}/send` | Send a ping now |

```json
{
  "name": "Watched by monitoring.example.com",
  "url": "https://monitoring.example.com/ping/4f7c2d9e-1b3a-4c5d-8e6f-0a1b2c3d4e5f",
  "interval_seconds": 60,
  "enabled": true
}
```

---

## Public Ping Endpoints

The `/ping/` routes are designed to be publicly accessible. maintenant applies no authentication of its own to them, since your cron jobs and external services need to reach them directly.

!!! warning "Reverse proxy configuration"
    Make sure your reverse proxy allows unauthenticated access to `/ping/` paths.
    See [Security](../security.md#public-routes) for details.

---

## Related

- [Alert Engine](alerts.md): `deadline_missed` alerts for heartbeat monitors
- [API Reference](../api/reference.md): full heartbeat API endpoints

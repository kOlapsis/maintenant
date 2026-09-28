# Heartbeat & Cron Monitoring

Monitor cron jobs, scheduled tasks, and any periodic process. Create a monitor, get a unique URL, add one `curl` to your script. maintenant tracks start/finish times, durations, exit codes, and alerts you when a job misses its deadline.

---

## How It Works

1. **Create a heartbeat monitor** through the API or dashboard — give it a name and a deadline (e.g., "every 5 minutes").
2. **Get a unique ping URL** — maintenant generates a UUID-based URL for this monitor.
3. **Ping the URL** from your cron job or script — maintenant records the ping and resets the deadline timer.
4. **Get alerted** if the deadline is missed — the job did not report in on time.

---

## Ping URL Format

Every heartbeat monitor gets a unique URL:

```
{BASE_URL}/ping/{uuid}
```

Where `{BASE_URL}` is your `MAINTENANT_BASE_URL` environment variable.

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
- Any other exit code = failure

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

maintenant calculates the duration between start and finish pings.

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
| **Status** | `up` (pinging on time), `down` (deadline missed), `paused` |
| **Execution history** | Full list of past executions with timestamps and results |

---

## Deadline Missed Alerts

When a heartbeat monitor does not receive a ping within its configured deadline, maintenant fires a `deadline_missed` alert with **Critical** severity. A ping reporting a non-zero exit code fires an `exit_code_failure` alert instead, also Critical. Both resolve automatically, with a recovery notification, on the next successful ping; deleting or pausing a heartbeat clears its active alerts too.

This means your cron job either:

- Failed to run at all
- Ran but crashed before reaching the `curl` ping
- Is taking longer than expected

!!! tip "Set reasonable deadlines"
    Set the deadline slightly longer than your expected job duration.
    A job that runs every 5 minutes with a 1-minute runtime should have
    a deadline of about 6-7 minutes to avoid false positives.

---

## Managing Heartbeats

### Pause and Resume

Temporarily disable a heartbeat monitor during planned maintenance:

```bash
# Pause — stops deadline checking
POST /api/v1/heartbeats/{id}/pause

# Resume — resets the deadline timer
POST /api/v1/heartbeats/{id}/resume
```

### CRUD Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/heartbeats` | List all heartbeat monitors |
| `POST` | `/api/v1/heartbeats` | Create a new heartbeat monitor |
| `GET` | `/api/v1/heartbeats/{id}` | Get a specific monitor |
| `PUT` | `/api/v1/heartbeats/{id}` | Update a monitor |
| `DELETE` | `/api/v1/heartbeats/{id}` | Delete a monitor |

---

## Outbound Heartbeats

A monitoring tool cannot report its own outage. Outbound heartbeats solve this by letting another maintenant instance watch this one: this instance pings a heartbeat monitor on the other instance at a fixed interval, and if the pings stop, the other instance raises a `deadline_missed` alert.

### Setting It Up

1. On the **other** instance, create a heartbeat monitor with an interval that matches the one you will use below, plus some grace time. Copy its ping URL.
2. On **this** instance, open **Heartbeats → Outgoing** and click **New target**.
3. Paste the ping URL, give the target a name and choose an interval.
4. Click **Send now** to check the target right away. The row shows the HTTP status it received, or the error.

Two instances can watch each other this way, each with one incoming and one outgoing heartbeat.

### How Sends Work

- Each target is called with a `GET` request, with a 10-second timeout and the `User-Agent` `maintenant/<version> outbound-heartbeat`.
- The interval goes from 30 seconds to 24 hours.
- A 2xx response counts as a success. Any other status, or a network error, is recorded as the target's last error and logged as a warning.
- A disabled target keeps its settings but is no longer called.

!!! note "HTTPS and public addresses only"
    Targets must use `https://` and resolve to a public address. Loopback and
    private ranges are refused when the target is saved and again when it is
    called, so the feature cannot be used to reach services on your internal
    network. The other instance therefore has to be reachable over HTTPS from
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

The `/ping/` routes are designed to be publicly accessible. They do not require authentication, since your cron jobs and external services need to reach them directly.

!!! warning "Reverse proxy configuration"
    Make sure your reverse proxy allows unauthenticated access to `/ping/` paths.
    See [Security](../security.md#public-routes) for details.

---

## Related

- [Alert Engine](alerts.md) — `deadline_missed` alerts for heartbeat monitors
- [API Reference](../api/reference.md) — Full heartbeat API endpoints

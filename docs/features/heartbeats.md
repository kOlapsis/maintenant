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

When a heartbeat monitor does not receive a ping within its configured deadline, maintenant fires a `deadline_missed` alert with **Critical** severity.

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

## Public Ping Endpoints

The `/ping/` routes are designed to be publicly accessible. They do not require authentication, since your cron jobs and external services need to reach them directly.

!!! warning "Reverse proxy configuration"
    Make sure your reverse proxy allows unauthenticated access to `/ping/` paths.
    See [Configuration](../getting-started/configuration.md#public-routes) for details.

---

## Related

- [Alert Engine](alerts.md) — `deadline_missed` alerts for heartbeat monitors
- [API Reference](../api/reference.md) — Full heartbeat API endpoints

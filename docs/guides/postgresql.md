# PostgreSQL storage

By default, maintenant keeps everything in a SQLite file next to the binary.
That is the right choice while one instance watches one host: nothing to
install, nothing to administer, and a backup is a file copy.

It becomes a ceiling once the instance watches a fleet. The server holds one
class of data the agents cannot rebuild: **their identity and their
enrolment**. If the machine carrying the file is lost, so is the link between
the server and every monitored host — bringing up a replacement would mean
re-enrolling the fleet by hand, host by host, while monitoring is blind.

Pointing the server at a PostgreSQL database you already operate removes that:
the instance stops being attached to its machine.

!!! note "What the product does, and what stays yours"

    The product provides one property: **it keeps nothing essential on the
    machine that runs it**. Redundancy, backups, restores and promotion of the
    database are yours, as is detecting a dead instance and restarting it
    elsewhere. maintenant does not detect failures, elect a leader, or move
    anything.

**Agents are never affected.** An agent stores its state in SQLite, always, and
refuses a connection string outright.

## Requirements

- PostgreSQL 14 or newer, reachable from the instance, with an empty database
  and a role that may create tables in it.
- An instance in server or embedded mode. **In agent mode the setting is
  refused**, with a message naming the mode.

## New install

```bash
docker run -d --name maintenant \
  -e MAINTENANT_DATABASE_URL="postgres://maintenant:secret@db.internal:5432/maintenant?sslmode=require" \
  -e MAINTENANT_LICENSE_KEY="…" \
  -v maintenant-data:/data \
  -p 127.0.0.1:8080:8080 \
  ghcr.io/kolapsis/maintenant:latest
```

The schema is created on first start, with no manual step. Check it:

```bash
curl -s localhost:8080/api/v1/health | jq .storage
# { "engine": "postgres", "connected": true, "peers": 0 }
```

Without `MAINTENANT_DATABASE_URL`, nothing changes: SQLite, exactly as before.

### Transport encryption

When the connection string carries no explicit `sslmode` and the host is not
local, the product adds **`sslmode=require`**: an external database is reached
over TLS by default. Any explicit value wins, `disable` included, so a database
on the same host or a private link can be relaxed deliberately. For a database
crossing a network you do not control, prefer `sslmode=verify-full`, which also
checks the server's certificate against its hostname.

The connection string is never written to the logs, the API, the interface or
the telemetry. Where a target has to be named, it appears redacted:
`postgres://maintenant@db.internal:5432/maintenant`.

## What must follow the instance

Two states live in the data directory rather than in the database:

| State | File | If the directory does not follow |
|---|---|---|
| Signed licence cache | `<dataDir>/.maintenant-license` | Re-verified online at startup. Offline: Community until the network returns. |
| Update window record | `<dataDir>/.maintenant-update-window` | A fresh grace window opens, which plays in your favour. |

Anonymous telemetry keeps its own state under `MAINTENANT_TELEMETRY_DATADIR`
(`/data/shm` by default). Losing it only breaks the continuity of anonymous
counters; nothing about the fleet depends on it.

So keep the `/data` volume with the instance when it moves. It is the only case
where a move is not fully transparent, and it is settled by carrying the volume,
which any cluster manager can do.

## Migrating an existing install

An install already running on the local file can move to PostgreSQL without
touching a single monitored machine. One command carries what the fleet cannot
rebuild; everything else is re-reported or recomputed.

```bash
# 1. Stop the instance: the source must not move during the copy.
docker stop maintenant

# 2. Carry what does not rebuild itself.
docker run --rm -v maintenant-data:/data ghcr.io/kolapsis/maintenant:latest \
  --db /data/maintenant.db \
  --copy-store-to "postgres://maintenant:secret@db.internal:5432/maintenant?sslmode=require"

# 3. Restart with the connection string.
docker run -d --name maintenant \
  -e MAINTENANT_DATABASE_URL="postgres://maintenant:secret@db.internal:5432/maintenant?sslmode=require" \
  -v maintenant-data:/data -p 127.0.0.1:8080:8080 \
  ghcr.io/kolapsis/maintenant:latest
```

The agents reconnect on their own, with the identity they already had. Add
`--yes` to skip the confirmation prompt in a script.

### What travels

Agent identities and enrolment tokens, container rows, declared endpoint,
heartbeat and certificate monitors, notification channels and their secrets,
alert triggers, silences, escalation policies, webhook subscriptions,
per-container thresholds, the whole status page (settings, components, assets,
FAQ, footer, **subscribers**), published incidents, planned maintenance
windows, and your operator decisions: update exclusions, version pins,
acknowledged findings.

### What does not, and what you will notice

Everything the fleet rebuilds by itself: agent inventories (re-sent within
30 s), check and resource history, state transitions, active alerts and their
deliveries, CVE and image-update intelligence, ephemeral OAuth tokens.

Two effects are worth knowing **before** you start, and the command says them
before writing anything:

- **An alert acknowledged before the copy comes back unacknowledged** if it is
  still active. The acknowledgement lives on the alert row, which does not
  travel. One click to redo, but it should not surprise you.
- **Curves start from zero**: resource, uptime and check history. Aggregates
  rebuild at their usual intervals.

### Guarantees

| Situation | What happens |
|---|---|
| The target is not empty | Refused before writing anything. Merging would need conflict rules nothing can settle correctly. |
| The copy fails half-way | The whole transaction rolls back, schema included. The target is empty again and you can retry. |
| Whatever happens | The source is opened read-only and never written to. Your original install stays usable. |
| The copy succeeds | Row counts are compared table by table, source against target, and any difference is an error. |

Exit codes: `0` copied and verified, `1` refused (non-empty target, unreadable
or out-of-date source, unreachable target, or you declined), `2` failed
mid-copy and rolled back.

The copy does not switch your configuration: you set
`MAINTENANT_DATABASE_URL` and restart yourself, deliberately. It does not run
in the other direction either — going back means removing the variable, which
returns to the local file as you left it.

## Latency

Every write now costs a round trip to the database, where a local file cost
none. Put the database in the same region — ideally the same network — as the
instance. The measure that matters is the one you feel: the delay between an
event reported by an agent and its appearance on screen should stay in the same
class as before. A database several tens of milliseconds away is fine; one on
another continent is not.

## The probe must not kill the pod

`/api/v1/health` answers **`200` even when the database is momentarily
unreachable** — the outage is reported in `storage.connected`, not in the HTTP
status. This is deliberate: the endpoint is the target of the Kubernetes
liveness and startup probes, and a probe failing on a ten-second blip would
restart the instance exactly when the database needs to be left alone.

Do not replace it with a check that fails on that case.

## Two instances on one database

If a second instance beats on the same database, `peers` is non-zero and a
warning is logged naming their count, hosts and versions. The product does not
arbitrate: **exclusion belongs to your cluster manager.** Saying it is the only
action it takes.

## Testing against PostgreSQL

The store test suite runs on SQLite by default and on PostgreSQL when a test
server is configured. `make test-pg` starts a disposable PostgreSQL 14, points
`MAINTENANT_TEST_DATABASE_URL` at it and runs the suite:

```bash
make test-pg
```

Without that variable, the PostgreSQL cases are skipped, never failed, so
`go test ./...` still works without Docker.

## Going back

Remove `MAINTENANT_DATABASE_URL` and restart: the instance finds its SQLite
database in the state it left it. Whatever was written to PostgreSQL in the
meantime is not there, and the migration command does not run in that
direction.

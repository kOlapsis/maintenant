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

See the dedicated section once the migration command ships. In short: it copies
what the fleet cannot rebuild (agent identities, enrolments, declared monitors,
channels, triggers, status page), announces what it leaves behind before
writing, and refuses a non-empty target.

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

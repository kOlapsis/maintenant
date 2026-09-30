# Troubleshooting

Logs are JSON, one object per line. The messages below are the `msg` field; the detail is in the other fields (`error`, `fix`, `target`).

---

## Permission denied on /var/run/docker.sock

**Symptom:** maintenant starts but shows no containers, and the logs repeat `Docker connection failed, retrying` with this error:

```text
docker ping failed: permission denied while trying to connect to the docker API at unix:///var/run/docker.sock
```

**Why it happens:** the entrypoint starts as root only to prepare the data directory, then drops to uid 65534 (`nobody`) with `setpriv`. maintenant itself never runs as root. The Docker socket on the host is owned by `root:docker`, so the process needs that group, otherwise the kernel refuses the connection whatever the `:ro` flag says.

**Normally this is automatic.** The entrypoint reads the group of the mounted `/var/run/docker.sock` (or `/run/docker.sock`) and adds it to the unprivileged user, on plain Compose **and** Docker Swarm. Just mount the socket:

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    read_only: true
    security_opt:
      - no-new-privileges:true
    tmpfs:
      - /tmp:noexec,nosuid,size=64m
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - /etc/os-release:/host/etc/os-release:ro
      - maintenant-data:/data
    environment:
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"
    restart: unless-stopped
```

!!! warning "Don't rely on `group_add` for Swarm"
    `docker stack deploy` silently ignores `group_add` (it is not part of the Swarm service
    spec), which is why socket access used to fail on Swarm. Auto-detection makes `group_add`
    unnecessary there. See the [Swarm guide](guides/swarm.md).

**If it still fails,** one of these applies:

- The socket is mounted at another path. The entrypoint only looks at `/var/run/docker.sock` and `/run/docker.sock`, so give it the group with `DOCKER_GID`.
- The container is started as a non-root user (`user:` in Compose, `runAsUser` in Kubernetes). The entrypoint then runs the command as is and detects nothing: add the socket's group with `group_add`.
- The socket belongs to group `root`: see the Synology and rootless sections below.

Find the socket's group on the host:

```bash
stat -c '%g' /var/run/docker.sock
# or: getent group docker | cut -d: -f3
```

Create a `.env` file next to your `docker-compose.yml`:

```bash
DOCKER_GID=998   # replace with the number printed above
```

and pass it to the container, either as an environment variable (the entrypoint reads it) or, on plain Compose, with `group_add`:

```yaml
    environment:
      DOCKER_GID: "${DOCKER_GID}"
```

`DOCKER_GID` is added to the group the entrypoint detected, it does not replace it. It has no use behind a socket proxy (`DOCKER_HOST: tcp://socketproxy:2375`): no socket is mounted, so no group is involved.

---

### Synology DSM (Container Manager) and other root-owned sockets

Synology DSM ships no `docker` group: `/var/run/docker.sock` is `root:root`, so the GID the entrypoint detects is `0`. Auto-detection deliberately refuses to hand the unprivileged runtime root's group, so container discovery stays empty and the entrypoint prints this on stderr at every start:

```text
maintenant: /var/run/docker.sock belongs to group root (gid 0) and the runtime drops to an unprivileged user, so the Docker API will be unreachable. Set DOCKER_GID=0 to grant that group explicitly, or front the socket with a docker-socket-proxy (https://docs.maintenant.dev/security/#recommended-docker-socket-proxy).
```

Two ways out:

- **Grant gid 0 explicitly.** Set `DOCKER_GID: "0"` on the service. Only an explicit value unlocks the root group; nothing is granted implicitly. The process then holds root's group, which on a Docker host means root-equivalent access to the Docker API.
- **Front the socket with a proxy** (recommended, and the hardened setup on any host). See [Security: Docker socket proxy](security.md#recommended-docker-socket-proxy). The proxy holds the socket and rejects every write at the HTTP layer. maintenant reaches it over `DOCKER_HOST: "tcp://socketproxy:2375"`, so no socket mount and no GID are involved.

---

### SELinux (Fedora / RHEL / Rocky / CentOS)

If the GID fix above does not resolve the error, SELinux may be blocking the socket access. Check for recent denials:

```bash
ausearch -m AVC -ts recent
```

If you see a denial for `docker.sock`, add the `:z` relabel flag to the socket mount so SELinux applies the correct context:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro,z
```

The `:z` flag relabels the bind mount with a shared label, which grants container processes access while keeping SELinux enforcing.

---

### Docker rootless

With rootless Docker, the socket is not at `/var/run/docker.sock` but at `$XDG_RUNTIME_DIR/docker.sock`, typically `/run/user/<uid>/docker.sock`. Adjust the bind mount accordingly:

```bash
# Find the socket path on the host
echo $XDG_RUNTIME_DIR/docker.sock
# e.g. /run/user/1000/docker.sock
```

```yaml
volumes:
  - /run/user/1000/docker.sock:/var/run/docker.sock:ro
```

Replace `1000` with the UID of the user running the rootless daemon (`id -u` on the host). Then check which group the container sees on the socket:

```bash
docker compose exec maintenant stat -c '%g' /var/run/docker.sock
```

If it prints `0`, the entrypoint has printed the gid 0 message described in the previous section: set `DOCKER_GID: "0"`. In rootless mode the container's root is mapped to the user who runs the daemon. Any other value is the `DOCKER_GID` to set.

---

## No containers, and a "RUNTIME OFFLINE" banner

When the container runtime cannot be reached at startup, maintenant does not exit. It starts in degraded mode: the dashboard, the stored data, endpoint checks, heartbeats, certificate checks and the status page keep working, and container monitoring resumes by itself when the runtime answers.

What you see:

- the red **RUNTIME OFFLINE** banner in the interface, and `"connected": false` under `runtime` in `GET /api/v1/health`. That endpoint still answers `200`, so the container stays `healthy`;
- in the logs, `container runtime unavailable, starting in degraded mode`, then `Docker connection failed, retrying` with the cause in `error` and the delay in `retry_in` (1 s, doubling up to 30 s);
- `container runtime reconnected, resuming container monitoring` once the runtime is back. If it disappears while maintenant runs, the log says `container runtime lost, entering degraded mode` and the same retry loop starts.

The `error` value tells the cause:

| Error (after `docker ping failed:`) | Meaning |
|---|---|
| `permission denied while trying to connect to the docker API at unix:///var/run/docker.sock` | The process is not in the socket's group. See the first section. |
| `failed to connect to the docker API at unix:///var/run/docker.sock; check if the path is correct and if the daemon is running: ... no such file or directory` | Nothing at that path: the socket is not mounted, is mounted elsewhere (rootless), or the daemon is down. |
| `Cannot connect to the Docker daemon at tcp://socketproxy:2375. Is the docker daemon running?` | `DOCKER_HOST` points at a proxy or daemon that refuses the connection, or does not answer. |
| `failed to connect to the docker API at tcp://socketproxy:2375: lookup socketproxy: no such host` | The name does not resolve from maintenant's network. |

If maintenant starts before a socket proxy is up, it boots in degraded mode and reconnects on its own.

---

## Docker is older than 19.03

The Docker client inside maintenant speaks API 1.40 at the lowest, which is Docker Engine 19.03. On an older daemon the connection succeeds, then every call is refused by the daemon with an error of the form `client version <n> is too new. Maximum supported API version is <m>`. The logs fill with `Docker event stream error` and `reconcile discover: container list: Error response from daemon: ...`, and no container appears.

Upgrade the engine to 19.03 or later.

---

## Update checks ignore locally built images behind a socket proxy

Each update scan asks the Docker API for the image list (`GET /images/json`) to learn which digest every container runs and to recognise images built on the host, which have no registry to check. [docker-socket-proxy](security.md#recommended-docker-socket-proxy) answers `403` to that call unless `IMAGES` is enabled, and maintenant ignores the refusal without logging it. The effects:

- an image built locally is not recognised: it is looked up in a registry under its own name, and one built as `nginx:latest` is compared with the public image of that name;
- the rollback commands cannot pin the exact digest the container runs, so they fall back to the digest seen at the previous scan or to the tag.

Add `IMAGES: "1"` to the proxy's environment. It only opens read access: `POST` stays at `0`, so every write is still refused.

---

## The container is healthy but the published port does not answer

**Symptom:** `docker compose ps` shows the container `healthy`, the logs say `starting HTTP server` with `"addr":"127.0.0.1:8080"`, and the browser or `curl` on the published port fails, for instance with `curl: (56) Recv failure: Connection reset by peer`.

**Why it happens:** `MAINTENANT_ADDR` defaults to `127.0.0.1:8080`, and the image does not change it. Inside a container that is the container's own loopback: Docker's port mapping reaches the process through the container's network interface, so nothing answers. The healthcheck runs inside the container and probes the loopback, so it passes.

**Fix:** bind all interfaces inside the container and decide the exposure with the `ports:` mapping:

```yaml
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      MAINTENANT_ADDR: "0.0.0.0:8080"
```

See [Configuration: Choosing a Bind Address](getting-started/configuration.md#choosing-a-bind-address).

---

## The port is already in use

**Symptom:** the container exits at once and, with `restart: unless-stopped`, starts again in a loop. The logs end with:

```text
application error  error="listen on 127.0.0.1:8080: listen tcp 127.0.0.1:8080: bind: address already in use"
```

**Fix:** find the process that holds the port (`ss -ltnp 'sport = :8080'`), stop it, or move maintenant. Inside a container, change the host side of the mapping (`"127.0.0.1:8081:8080"`) and leave `MAINTENANT_ADDR` alone.

With a published port, Docker refuses to start the container before maintenant runs, with `Bind for 127.0.0.1:8080 failed: port is already allocated` when another container has it, or `failed to bind host port ... address already in use` when a host process has it.

The agent gRPC listener fails the same way, with `start agent gRPC server: agentserver: listen 127.0.0.1:8443: ... address already in use`.

---

## Startup fails with "attempt to write a readonly database"

**Symptom:** the logs end with:

```text
failed to initialize application  error="run migrations: create sqlite migrate driver: attempt to write a readonly database"
```

**Why it happens:** a file in the data directory is not writable by uid 65534, typically a `maintenant.db-wal` or `maintenant.db-shm` left by a `sqlite3` session run as root, or a database restored or copied as root. The entrypoint prepares the directory, not the files in it.

**Fix:** give the files back to the service user, with the container stopped:

```bash
chown 65534:65534 /path/to/data/maintenant.db*
```

---

## The container turns unhealthy during an upgrade

Schema migrations run before the HTTP listener opens, so `/api/v1/health` does not answer while they work. On a large database that is minutes, and longer for the one-time UUID conversion of an old install. The image's healthcheck allows a 30 s start period then three failed probes 30 s apart, so Docker reports `unhealthy` about two minutes in.

Docker itself does not restart an unhealthy container, so the migration goes on. Wait for `database migration` with `"status":"migrations applied"` in the logs. What can interrupt it is an orchestrator that restarts unhealthy containers (Swarm, an `autoheal` sidecar, a Kubernetes liveness probe without a startup probe). An interrupted migration is detected at the next start (`dirty migration state detected, auto-recovering`) and replayed from its beginning, so each restart only loses time. On Kubernetes the chart's `startupProbe` covers this: see [Health Probes](guides/kubernetes.md#health-probes).

Migrations also rebuild tables in place and need free disk space of several times the database's size, on the data volume.

---

## Kubernetes: Docker monitored instead of the cluster, or a cluster that does not answer

The runtime is chosen in this order: `MAINTENANT_RUNTIME`, then the in-cluster environment (`KUBERNETES_SERVICE_HOST`), then a kubeconfig (`KUBECONFIG`, else `~/.kube/config`), then Docker. See [Runtime Detection](guides/kubernetes.md#runtime-detection).

**A kubeconfig is found but its cluster does not answer within three seconds.** Without `MAINTENANT_RUNTIME`, maintenant falls back to Docker and logs:

```text
kubeconfig present but its cluster is unreachable, falling back to Docker; set MAINTENANT_RUNTIME=kubernetes to wait for that cluster instead
```

The `kubeconfig` and `error` fields name the file and the failure. If you want Docker, remove or unmount the stale kubeconfig, or set `MAINTENANT_RUNTIME=docker`. If you want the cluster, set `MAINTENANT_RUNTIME=kubernetes`: maintenant then starts in degraded mode and retries until the API server answers (`kubernetes connection failed, retrying`, 1 s doubling up to 30 s). A kubeconfig that cannot be used at all (no cluster or no context in it) is not retried: the instance stays in degraded mode, with only `container runtime unavailable, starting in degraded mode` in the logs and no cause. Fix the file and restart. The in-cluster environment never falls back to Docker.

**The API server goes away while maintenant runs.** It probes the API every 15 s, logs `kubernetes API server unreachable` with `misses` counting up to 3, and after the third miss (about 45 s) enters degraded mode (**RUNTIME OFFLINE**), then reconnects with the same backoff.

---

## An HTTPS host shows as down (or degraded) with "unknown authority"

The host is answering; maintenant just refuses to trust the certificate it presents. Since it is reachable, it is reported as **degraded** rather than down.

If the certificate comes from your own PKI, trust the root instead of disabling verification:

```yaml
environment:
  MAINTENANT_CA_CERT: /etc/maintenant/ca.pem
volumes:
  - ./ca.pem:/etc/maintenant/ca.pem:ro
```

The bundle is added to the system roots and applies to every outbound TLS connection: endpoint probes, certificate checks, webhooks and notification channels, outbound heartbeats, SMTP with STARTTLS, the license server, OSV, GitHub, endoflife.date, image registries, and the agent's connection to its server. If the endpoint is attached to an agent, set it on the **agent**: it is the one performing the probe.

**An unreadable or invalid bundle stops maintenant.** It exits with code 1 right after `maintenant starting`, and with `restart: unless-stopped` it loops. The log says `failed to load extra CA bundle`, and `error` gives the reason:

- `read CA bundle /etc/maintenant/ca.pem: open /etc/maintenant/ca.pem: permission denied`: the container runs as uid **65534**, and a root-owned `0600` file is not readable by it;
- `CA bundle /etc/maintenant/ca.pem contains no valid PEM certificate`: the file is not a PEM bundle.

Do not reach for `SSL_CERT_FILE`. Go treats it as a replacement for the whole system bundle rather than an addition, so setting it drops every public CA, and an unreadable file yields an empty trust store with no error reported.

As a last resort you can turn verification off for a single container endpoint with the label `maintenant.endpoint.http.tls-verify=false`, but that also stops expiry and hostname checks for it.

---

## An agent host flaps: connected, then disconnected, every 60 seconds

**Symptom:** the host on the **Agents** page alternates between `connected` and `disconnected`
at a regular interval, and its containers show up or vanish depending on when you look. The
agent logs show `agent: stream closed, will reconnect` then `agent: reconnecting` in a loop, with
no mention of a proxy, so it reads like an unstable network link.

**Why it happens:** the agent stream is a single gRPC request whose body never ends. A reverse
proxy that caps the duration of a request cuts it at that limit no matter how much traffic
flows on it. On Traefik v3 that cap is `respondingTimeouts.readTimeout`, **60 s by default** on
every entrypoint.

**Fix:** disable the read timeout on the entrypoint that carries gRPC. It is a per-entrypoint
setting, so a dedicated entrypoint is needed to avoid dropping the protection for all HTTPS
traffic:

```yaml
- --entrypoints.grpc.address=:8443
- --entrypoints.grpc.transport.respondingTimeouts.readTimeout=0
- --entrypoints.grpc.transport.respondingTimeouts.idleTimeout=0
```

Agents then need the port in their URL (`--server=grpcs://agents.example.com:8443`). The
equivalent settings are `grpc_read_timeout` on nginx and `timeout tunnel` on HAProxy. Full
configuration, including the ACME side effect of a non-443 entrypoint, is in the
[Agent Setup guide](guides/agent-setup.md#step-1-make-the-grpc-endpoint-reachable).

---

## An agent stops with "agent revoked by server"

**Symptom:** the agent container exits with code 1 and, under a restart policy, loops. The logs show `agent: identity refused by server` (or `agent: identity refused during dial`) and then `agent run failed` with one of:

```text
agent revoked by server (agent <id>): create an enrollment token on the server and restart the agent with MAINTENANT_ENROLLMENT_TOKEN or --enrollment-token set to it
agent unknown to the server, deleted or enrolled with another one (agent <id>): create an enrollment token on the server and restart the agent with MAINTENANT_ENROLLMENT_TOKEN or --enrollment-token set to it
```

**Why it happens:** the server revoked this host, deleted it, or the agent's stored identity belongs to another server (a data volume reused against a different instance). The agent discards its offline spool, but keeps its identity file until a new enrollment succeeds.

**Fix:** generate a new token (**Agents**, **Generate enrollment token**) and restart the agent with it in `MAINTENANT_ENROLLMENT_TOKEN` or `--enrollment-token`. The agent then enrolls again with a new identity, logging `agent: the server refused this identity, enrolling a new one with the configured token`. It does this once per start. A token is single use, so the one from the first enrollment cannot serve again: if it is still configured, the start ends with `enrolling again after the server refused the previous identity: ...`, and you have to replace it. The new identity appears as a new host and counts against the host cap. See [Agent Setup](guides/agent-setup.md).

A dropped connection is different: the stream reconnects with a backoff of up to 60 s and the agent keeps running.

---

## The database keeps growing, or the -wal file is huge

Raw resource samples (every 10 s per container, around 360 an hour) are kept 48 hours by default. `MAINTENANT_RETENTION_SNAPSHOTS` changes that, with 24 hours as the floor. The hourly and daily roll-ups behind the longer history windows are kept 90 days and one year. A retention pass runs at startup, then hourly (`MAINTENANT_RETENTION_INTERVAL`), deleting at most `MAINTENANT_RETENTION_BATCH_SIZE` rows (1000 by default) per transaction. When a pass runs out of its time budget the next one starts a minute later, and the log says `retention cleanup: budget exhausted, resuming shortly`.

Up to and including 1.3.7, the cleanup deleted at most 1000 rows per hour whatever the number of monitored containers, so the purge fell behind past roughly three containers and `resource_snapshots` grew without bound. Upgrading fixes the throughput and drains the backlog.

Check where you stand. The image does not ship a `sqlite3` client, so read the logs and the files first:

```bash
docker compose logs maintenant | grep -E 'sqlite storage|retention cleanup'
docker compose exec maintenant ls -lh /data
```

- `retention cleanup: starting` shows the `snapshot_window` in force;
- `retention cleanup: pass complete` shows what a pass `deleted`, and `backlog_remaining` is `true` while rows are left behind. The `retention cleanup: deleted resource snapshots` line should no longer report a `count` stuck at exactly the batch size;
- `sqlite storage` shows `auto_vacuum` and the `freelist_pages` waiting to be reclaimed;
- `maintenant.db-wal` should not stay above 64 MiB for long.

To query the database, use a `sqlite3` client on the host, on the file behind the volume (`/var/lib/docker/volumes/<project>_maintenant-data/_data/maintenant.db` for a named volume, which root can read), in read-only mode so it never disturbs the running instance:

```bash
sqlite3 -readonly /path/to/maintenant.db "
  SELECT COUNT(*) AS rows, datetime(MIN(timestamp),'unixepoch') AS oldest FROM resource_snapshots;
  PRAGMA auto_vacuum;
  PRAGMA freelist_count;
"
```

`oldest` should stay inside the retention window (48 hours by default).

### Reclaiming disk space already used

`PRAGMA auto_vacuum` (or the `auto_vacuum` field of the `sqlite storage` log line) tells you whether freed pages return to the filesystem:

- **`2` (`incremental`)**: nothing to do. Retention hands freed pages back automatically and
  `freelist_count` shrinks pass after pass.
- **`0` (`none`)**: the database was created before auto-vacuum was enabled and only a full
  `VACUUM` can convert it. maintenant logs a warning at startup in this case. The database stops
  growing regardless, since SQLite reuses freed pages, but the file stays at its high-water mark.

A full `VACUUM` is a manual, offline operation. It takes an exclusive lock for its whole duration
(minutes on a multi-gigabyte database, much longer on an SD card or NAS) and rewrites the file, so
you need **free disk space equal to the current database size** on top of it. Run it with the host's `sqlite3`, on the file behind the volume:

```bash
docker compose stop maintenant
sqlite3 /path/to/maintenant.db "PRAGMA auto_vacuum=INCREMENTAL; VACUUM;"
ls -ln /path/to/maintenant.db*    # the files must still belong to uid 65534
docker compose start maintenant
```

Take a backup first. Running out of disk space mid-VACUUM leaves a journal file behind.

### The -wal file

Every connection sets a `journal_size_limit` of 64 MiB, so the WAL is truncated back after each
checkpoint. Up to 1.3.7 it was unbounded and only shrank when the process restarted.

---

## PostgreSQL storage refuses to start, or goes quiet

These apply when `MAINTENANT_DATABASE_URL` is set. Without it, the instance
uses its local SQLite file and none of this concerns you.

### The instance exits at startup

The message names the cause and what to correct. There is deliberately no
fallback to the local file: starting on an empty local database while the
external one is misconfigured would look like it worked, and silently strand
the fleet.

| Message | What happened | What to check |
|---|---|---|
| the database connection string cannot be read | The value is not a PostgreSQL URL | Expected `postgres://user:password@host:5432/database[?sslmode=require]` |
| the database server does not accept TLS | The connection must use TLS and the server answered that it has none | See below |
| the database does not answer | Nothing listens, or the route is blocked | Host, port, network route, firewall, and that the server is up |
| the database refused the credentials | It answered and said no | User, password, and that this role may connect to this database |
| the database version is not supported | Older than PostgreSQL 14 | Upgrade the server; 14 is the oldest release still supported upstream |
| the database schema was written by a newer release | A newer binary already migrated it | Run that version, or upgrade this one. It will not write into a schema it does not understand |
| MAINTENANT_DATABASE_URL is not accepted in agent mode | An agent was handed a connection string | Drop the setting: an agent always stores its state locally |

None of these messages contain the password. Where the target is named it
appears as `postgres://user@host:5432/database`.

**The server does not accept TLS.** When the connection string has no `sslmode` and the host is not `localhost`, `127.0.0.1`, `::1` or a Unix socket, `sslmode=require` is added. A PostgreSQL container on the same Compose network (host `db`, for instance) counts as a remote host, and the official image has TLS off, so the start fails with `fix` saying that `sslmode=require` was added by default. Either enable TLS on the PostgreSQL server, or, if the network between the two is trusted, write `sslmode=disable` explicitly in the URL. When the URL sets an `sslmode` that requires TLS itself, the same message tells you to enable TLS or lower the `sslmode`. See [PostgreSQL storage](guides/postgresql.md#transport-encryption).

!!! note "The schema check also applies to SQLite"

    A binary older than the schema it opens refuses to start on the local file
    too. This is the one behaviour change for an existing local install, and it
    only triggers on a downgrade: it stops the older binary from writing into a
    schema it does not know, which used to corrupt data silently.

### The database becomes unreachable while running

The instance stays up. It does not restart, does not fall back, and recovers on
its own when the database answers again: the connection pool renews its
connections. You will see:

- a **STORAGE OFFLINE** banner in the interface, and screens keeping what they
  already knew rather than emptying out;
- `503 STORAGE_UNAVAILABLE` on API reads that need the database;
- `storage.connected: false` in `/api/v1/health`, which still answers `200`.

That last point is deliberate and important: `/api/v1/health` is the target of
the Kubernetes liveness and startup probes. **Do not make the probe fail on a
database outage**: it would restart the instance exactly when the database
needs to be left alone. Read `storage.connected` instead.

### Two instances warn about each other

If the log says another instance is working on the same database, and `peers`
is non-zero in `/api/v1/health`, two instances are running against it. The
product does not arbitrate: exclusion is your cluster manager's job. Data is
not corrupted, but purges and alert evaluation run twice. Stop one of them.

---

## 403 EDITION_REQUIRED, QUOTA_EXCEEDED, or "server mode requires the personal edition"

Three refusals with one cause: what you asked for is outside the running edition (Community, Personal or Pro, see [Configuration: License](getting-started/configuration.md#license)). `GET /api/v1/edition` tells which edition is running and, in `feature_editions`, which edition opens each feature.

**`403 EDITION_REQUIRED`:** the feature needs a higher edition. The body names it:

```json
{"error":{"code":"EDITION_REQUIRED","message":"This feature requires the Personal edition.","feature":"multihost","required_edition":"personal"}}
```

On the resource history, the refusal also carries `window` and `max_window`: a window longer than your edition's cap (7 days on Community, 30 on Personal, 90 on Pro) is refused rather than silently shortened.

**`403 QUOTA_EXCEEDED`:** Community caps what you create by hand at 10 endpoints, 5 heartbeats, 5 certificate monitors and 3 status page components. Endpoints and certificate monitors that come from container labels are not counted. The body gives `resource`, `limit` and `required_edition`, for instance `The Community edition is limited to 10 endpoints.` Personal and Pro lift these caps.

**`409 HOST_LIMIT_REACHED`:** Personal accepts 20 remote hosts. The refusal comes when generating an enrollment token, before the install command is run; Pro has no cap.

**`server mode requires the personal edition (current edition: community)`:** `MAINTENANT_MODE=server` is refused at startup on Community, and the process exits (`application error`). Drop the variable, or set `MAINTENANT_LICENSE_KEY`. Agents are different: in the default `embedded` mode, Community starts and logs `agent gRPC listener not started: agents need the personal edition or above`, and nothing listens on the agent port.

If you set a license key and still see Community, check the license with `GET /api/v1/license/status`. An instance that cannot reach the license server for over 60 days also falls back to Community.

---

## 403 CROSS_ORIGIN_REFUSED

**Symptom:** saving something in the interface, or a script run from a browser, fails with:

```json
{"error":{"code":"CROSS_ORIGIN_REFUSED","message":"Cross-origin request refused: add the calling origin to MAINTENANT_CORS_ORIGINS to allow it."}}
```

**Why it happens:** `POST`, `PUT`, `PATCH` and `DELETE` on `/api/` are refused when a browser sends them from another origin than the one maintenant is served from, and that origin is not trusted. A different subdomain of the same site counts as another origin. Requests that carry neither `Origin` nor `Sec-Fetch-Site` (`curl`, scripts) and same-origin browser requests pass. Reads are never refused.

**Fix:**

- Serve the interface and the API from the same address. This is the normal case behind a reverse proxy.
- If another origin must write (a front end on another host), list it in `MAINTENANT_CORS_ORIGINS`, as `scheme://host[:port]` with no path and no trailing slash: `https://dashboard.example.com`. An entry that is not an origin logs `MAINTENANT_CORS_ORIGINS entry is not an origin and matches no request` at startup. The wildcard `*` opens reads to any origin but does **not** allow cross-origin writes.
- Browsers from before 2023 do not send `Sec-Fetch-Site`, and then the `Origin` header is compared with the `Host` header. A proxy that rewrites `Host` makes every write fail for them. Forward the original `Host` (`proxy_set_header Host $host;` in nginx).

`/ping/`, `/status/`, `/mcp` and `/oauth/` are not concerned.

---

## Every visitor shares one rate limit behind a reverse proxy

**Symptom:** behind a reverse proxy, heartbeat pings, the public status page or the MCP endpoint answer `429` with `{"error":{"code":"rate_limited","message":"Too many requests"}}` as soon as a few clients are active.

**Why it happens:** `MAINTENANT_TRUSTED_PROXIES` is empty by default, which means no forwarded header is read. Every request is attributed to the address that opened the connection, that is the proxy, so all clients share one bucket (10 requests per second, burst 20, on `/ping/`, `/status/`, `/mcp` and `/oauth/`; 50 per second, burst 200, on `/api/`). `X-Forwarded-Host` is ignored too, so the gRPC address offered to new agents comes from the request's own `Host` unless `MAINTENANT_GRPC_URL` is set.

**Fix:** list the addresses your proxy connects from:

```bash
MAINTENANT_TRUSTED_PROXIES=172.18.0.1
```

The proxy must send `X-Forwarded-For` or `X-Real-IP`. A proxy on the Docker host reaches a port published on `127.0.0.1` through the Docker network's gateway, so maintenant sees the gateway address, not `127.0.0.1`: list that one (`docker network inspect <project>_default`, field `Gateway`, or the network's subnet). A proxy in a container on the same network is seen under its own address. A value that does not parse stops the start with `invalid proxy configuration`. Details in [Security: Running behind a proxy](security.md#running-behind-a-proxy).

---

## MCP does not start, or /mcp answers 403

**MCP refuses to start.** With `MAINTENANT_MCP=true` and no OAuth credentials, the process exits with:

```text
application error  error="MAINTENANT_MCP=true but MAINTENANT_MCP_CLIENT_ID/MAINTENANT_MCP_CLIENT_SECRET are unset: /mcp is documented as bypassing the reverse-proxy auth, so it would serve containers, logs and alerts to anyone reaching it. Set both, or set MAINTENANT_MCP_ALLOW_UNAUTHENTICATED=true to accept that on a trusted network"
```

Set both variables. The secret is the only thing that keeps `/mcp` closed, so use 32 characters or more (`openssl rand -hex 32`): a shorter one starts but logs `MAINTENANT_MCP_CLIENT_SECRET is shorter than 32 characters`. `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED=true` serves `/mcp` to anyone who can reach it and logs `MCP server enabled WITHOUT auth`. The stdio transport (`--mcp-stdio`) never listens and needs neither.

**`403 Forbidden: invalid Host header "..."`.** Only in the no-authentication mode, and only when maintenant listens on a loopback address (a native install, or `MAINTENANT_ADDR=127.0.0.1:...`) while the request carries another `Host`, which is what a reverse proxy on the same machine forwards. The MCP library refuses it as DNS rebinding protection. Configure the OAuth credentials, which lifts the check (the bearer token guards `/mcp` instead), or have the proxy send a loopback `Host` for `/mcp`.

---

## Host OS shows "unknown"

The hover text on the **OS unknown** badge says why:

| Hover text | Cause | Fix |
|---|---|---|
| Mount /etc/os-release into the container | The container has no `/host/etc/os-release`. Inside a container, `/etc/os-release` describes the image, not the host, so maintenant does not read it. | Add the mount below and restart the container. |
| /etc/os-release unreadable on the host | `/host/etc/os-release` exists but is a directory, cannot be opened, or has no `ID=` line. Docker creates a directory when the host path is missing. | Make sure `/etc/os-release` is a readable regular file on the host, or a symlink to one. |
| Kubernetes node not found | A Kubernetes agent derives the OS from the node's image description. It could not tell which node it runs on, or the node reports none. | Set `MAINTENANT_NODE_NAME` to the node name. Without it, the agent looks for the pod named after its hostname. |
| Update the agent | The agent never reported an OS: it predates that feature. | Upgrade the agent. |
| The agent has not reported an OS identity | Nothing was sent yet. | Give a freshly connected agent a moment: it reports at start, then rereads every hour. |

The fix for the first row:

```yaml
volumes:
  - /proc:/host/proc:ro
  - /etc/os-release:/host/etc/os-release:ro
```

**Not tracked** is another state: the OS is known, but no end-of-life dates are published for that distribution.

Mounting a single file binds the container to the inode present at startup. After an OS
upgrade on the host, restart the container to pick up the new `/etc/os-release`.

# Multi-Host Monitoring :material-star-four-points:{ title="Personal" }

Maintenant can monitor containers and workloads running on multiple remote hosts from a single central server. Remote hosts run a lightweight **agent** process that streams events to the server over a persistent gRPC connection.

| Edition | Remote hosts |
|---------|--------------|
| Community | none |
| Personal | up to **20** |
| Pro | unlimited |

The machine running the server is never counted against the limit, and neither are revoked agents.

---

## Modes

The `maintenant` binary supports three operating modes via the `--mode=` flag (`MAINTENANT_MODE`):

| Mode | Description |
|------|-------------|
| `embedded` | Default. Monitors the local runtime and stores data in local SQLite (or PostgreSQL). |
| `server` | Central server. Receives events from remote agents via gRPC. Exposes the web UI and REST API. Personal or above. |
| `agent` | Remote agent. Monitors the local runtime and pushes events to a central server. Carries no license of its own: the server it enrolls with decides. |

What opens the agent listener is the edition, not the mode: on Personal or Pro, the gRPC listener also starts in `embedded` mode (on `127.0.0.1:8443` by default), and `server` mode adds the boot check below and the embedded agent. On Community there is no agent listener.

- On Community, `--mode=server` exits at startup with an error naming the required edition. `--mode=embedded` runs normally and logs `agent gRPC listener not started: agents need the personal edition or above`.
- `--mode=agent` is not checked against an edition by the agent itself. Enrolling needs a server that runs the agent listener, so in practice the server's edition decides.
- When the update window of a Personal licence closes, the instance falls back to Community features but keeps its multi-host plan running: the listener stays up and enrolled agents keep streaming. Enrolling new hosts and managing agents is refused until the licence is renewed.

---

## Server Setup

### Network interfaces

The server exposes two interfaces:

| Interface | Default | Purpose |
|-----------|---------|---------|
| HTTP | `127.0.0.1:8080` | REST API + web UI (reverse-proxied) |
| gRPC | `127.0.0.1:8443` | Agent event ingestion (TLS or h2c, see below) |

The gRPC port must be reachable from agent hosts. Configure the public URL so that generated install commands point to the right address:

```bash
MAINTENANT_GRPC_URL=grpcs://monitoring.example.com
```

The port is optional and defaults to `443`: only add it (e.g. `:8443`) if agents reach gRPC directly instead of through a reverse proxy/DNS terminating TLS on 443.

If not set, the server infers the URL from the web request that asks for it. `X-Forwarded-Host` is believed only when the request comes from a peer listed in `MAINTENANT_TRUSTED_PROXIES`; otherwise the `Host` header is used, port included unless it is 443. The result is therefore often the address of the web UI, not of gRPC, so set `MAINTENANT_GRPC_URL` explicitly. A warning is shown in the UI if the resolved URL appears to be a private address.

### Making the server replaceable

The server holds what the fleet cannot rebuild: agent identities and their
enrolments. On the default SQLite file, losing the machine means re-enrolling
every host by hand. Point the server at a PostgreSQL you already operate and a
replacement instance, started elsewhere with the same connection string, picks
the fleet back up with no action on any monitored machine.

Two files stay next to the instance rather than in the database: the signed
licence cache and the update window record. Keep the data directory with
the instance when it moves. See [PostgreSQL storage](../guides/postgresql.md).

Agents are unaffected either way: an agent stores its state in SQLite, always.

### Starting the server

Three TLS modes are supported, choose one:

**Behind a reverse proxy (h2c mode)**

Set `MAINTENANT_GRPC_TLS_INSECURE=true`. The listener accepts plaintext HTTP/2 (h2c); TLS is terminated at the proxy. The Docker-internal leg is unencrypted, which is safe within a private network.

```bash
MAINTENANT_GRPC_TLS_INSECURE=true maintenant --mode=server
```

The proxy must also let the stream live: it is a request whose body never ends, so any read timeout on the route (Traefik's `respondingTimeouts.readTimeout`, 60 s by default) severs it periodically. See [Agent Setup: long-lived streams](../guides/agent-setup.md#step-1-make-the-grpc-endpoint-reachable).

**Direct TLS with a custom certificate**

Mount a certificate/key pair covering the public hostname agents will dial:

```bash
maintenant \
  --mode=server \
  --grpc-listen=0.0.0.0:8443 \
  --grpc-tls-cert=/etc/maintenant/tls.crt \
  --grpc-tls-key=/etc/maintenant/tls.key
```

Both files are required: a certificate without its key (or the reverse) stops the server at startup, naming the missing variable, instead of falling back to a self-signed certificate.

If the certificate comes from a private CA, list that CA in `MAINTENANT_CA_CERT` on every agent (see [Trusting a private CA](#trusting-a-private-ca)).

**Self-signed (development only)**

Omit both cert and key. The server generates a self-signed certificate in-memory, logs a warning, and agents must connect with `--grpc-insecure-skip-tls-verify`.

### Embedded agent

With `--embedded-agent` (`MAINTENANT_EMBEDDED_AGENT`), a server also runs an agent of its own that enrolls itself under the label `embedded`, with its state in an `embedded-agent` directory next to the database. It dials the local listener with the scheme the listener serves: over TLS with verification off by default, and over plaintext h2c when `MAINTENANT_GRPC_TLS_INSECURE=true`. It needs `--mode=server` and a Personal or Pro edition, and is not started in demo mode. Unlike the server's own runtime, this agent is an enrolled host and counts toward the host limit.

---

## Agent Enrollment

Agents authenticate with the server using a one-time enrollment token and an Ed25519 keypair generated locally on first boot.

### 1. Generate an enrollment token

From the web UI: **Agents → Generate enrollment token**

Or via API:

```
POST /api/v1/agents/enrollment-tokens
Content-Type: application/json

{ "ttl_hours": 24 }
```

`ttl_hours` defaults to 24 and is capped at 168 (7 days). When the host limit is already reached, the request is refused with `409 HOST_LIMIT_REACHED` before any token is created. The limit is checked again when the agent enrolls, which leaves the token unused if it fails.

The response contains:
- `token`: the cleartext token, shown **once only**
- `install_templates`: a map of ready-to-run install snippets, one per environment. Keys:
  - `standalone`: downloads the release binary and installs it as a systemd service (`curl -fsSL https://install.maintenant.dev | sudo bash -s -- --mode agent …`)
  - `docker_run`: single `docker run` command with the right socket/proc mounts
  - `docker_compose`: `compose.yml` snippet
  - `kubernetes`: Namespace + Secret + ServiceAccount + RBAC + PersistentVolumeClaim + Deployment manifest
- `warnings`: present if the server URL appears to be a private/local address

> The token cannot be retrieved again after creation. If lost, delete it and generate a new one.

!!! note "Only the hash is stored"
    The database keeps `sha256(token)` plus the first 14 characters, which is what every list and detail view displays (`mnt_enr_ab12cd...***`). The cleartext exists in the creation response and nowhere else, so a copy of the database file taken from here on (a backup, the data volume, a `.db` grabbed for debugging) carries nothing replayable.

    Upgrading an existing install rewrites the table in place on first boot and keeps outstanding tokens valid: nothing has to be reissued. Copies of the database made *before* that upgrade still contain the cleartext, and no migration can reach them. If one of those copies may have leaked, delete the affected tokens and issue new ones; otherwise their 7-day maximum TTL retires them on its own.

### 2. Run the install command on the remote host

Pick the snippet matching the host environment (the UI exposes these as tabs in the modal). For a host where the binary is already installed, the invocation is:

```bash
maintenant \
  --mode=agent \
  --server=grpcs://monitoring.example.com \
  --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX \
  --label="prod-worker-01"
```

On first boot the agent:
1. Detects the local runtime (Docker, Swarm, or Kubernetes)
2. Generates an Ed25519 keypair and persists it to `identity.json` (mode `0600`) in its data directory (`MAINTENANT_DATA_DIR`, `/var/lib/maintenant` by default)
3. Calls `RegisterAgent` on the server with the token and public key
4. Marks itself as enrolled and enters the streaming loop

The `--label` flag sets a human-readable display name (max 64 chars), used at enrollment only: rename the host later from the Agents page. If omitted, or longer than 64 characters, the hostname is used.

On Kubernetes, one agent watches the whole cluster and counts as **one host**. It runs as a single-replica Deployment (strategy `Recreate`) with a 1 Gi volume for its data directory, a hardened pod (non-root, read-only root filesystem, no capabilities) and the read-only RBAC of the [Kubernetes guide](../guides/kubernetes.md). The generated manifest sets no label, so the agent appears under the hostname `maintenant-agent`; rename it from the Agents page.

### Refused or revoked agents

When the server refuses the stored identity (the agent was revoked, deleted, or enrolled with another server), the agent stops streaming, discards its spool and:

- if an enrollment token is set (`MAINTENANT_ENROLLMENT_TOKEN` or `--enrollment-token`), enrolls again with a **new identity**. `identity.json` is replaced only once that enrollment succeeds, and the host shows up as a new agent. A token is single use, so this needs a fresh token: update it and restart the agent, there is no need to empty the data volume.
- otherwise exits with code 1 and a message asking for a new token.

Network errors never count as a refusal: the agent keeps retrying with a backoff.

---

## Streaming Protocol

Once enrolled, the agent maintains a persistent bidirectional gRPC stream to the server.

### Authentication handshake (per stream)

Every time the agent connects or reconnects, it performs a challenge-response authentication:

```
Server → AuthChallenge { nonce: 32 random bytes }
Agent  → AuthResponse  { agent_id, timestamp, signature }
         where signature = Ed25519.Sign(private_key,
                             nonce || agent_uuid_bytes || timestamp_be64)
Server → validates signature, checks agent status = active,
         checks |server_time - client_time| ≤ 300s
```

This design requires no PKI infrastructure. The agent's public key is registered once at enrollment.

### Event collection

After authentication the agent collects and pushes events continuously:

| Event type | Frequency |
|------------|-----------|
| Container start/stop/die/pause/destroy | Real-time |
| Full container inventory | Every 30 s, and on every reconnect |
| Per-container resource metrics (CPU, memory, network, disk) | Every 10 s |
| Host-level metrics (machine CPU, memory, disk) | Every 10 s |
| Endpoint probes from container labels | At the `interval` of each endpoint label (30 s by default); labels re-read every 30 s |
| Certificate scans of `maintenant.tls.certificates` labels | Every 60 s |
| Swarm and Kubernetes topology | Every 30 s |
| Host operating system | At start, then hourly when it changed |

A Kubernetes agent sends the cluster topology, the host metrics of the node it runs on and the host operating system. Endpoint and certificate labels are read from Docker containers only.

Endpoints are probed by the agent, from its own network, with the `timeout` and `interval` of their labels. A certificate scan reports the leaf certificate only: expiry alerts work, while the chain, hostname and OCSP checks do not run for certificates scanned by an agent.

### Trusting a private CA

The agent verifies the server certificate against the system roots plus the bundle in `MAINTENANT_CA_CERT`, the same variable as on the server. A server whose gRPC certificate comes from a private CA is therefore reachable without `--grpc-insecure-skip-tls-verify`: mount the CA on each agent. The same bundle also applies to the endpoint probes and certificate scans the agent performs.

### Reconnection

If the connection drops, the agent reconnects automatically with exponential backoff:

```
delay = min(60s, 1s × 2^attempt) ± 25% jitter
```

The attempt counter resets to 0 if the previous stream was stable for more than 30 seconds. Reconnection stops only if the server refuses the identity (`agent_revoked`, or `agent not found` after a deletion): see [Refused or revoked agents](#refused-or-revoked-agents).

### Rate limiting

The server enforces a per-agent limit of **1 000 events/second** (token bucket). If exceeded, the server sends an in-stream error with a `retry_after_ms` hint.

### Clock checks

The server refuses an event observed more than 60 seconds ahead of its own clock, or more than 24 hours in the past. Keep agent clocks in sync (the handshake tolerates 5 minutes of skew, events far less).

---

## Outage Spool

When the stream drops, the agent keeps collecting. Events queue in a local
SQLite database (`<data-dir>/spool.db`) and replay once the connection is back,
so a server restart, a network cut or an HA failover leaves a backlog rather
than a hole in your history.

### What is replayed

| Family | Spooled? | Why |
|--------|----------|-----|
| Container lifecycle events | Yes | The timeline of what happened during the outage |
| Container resource samples | Yes | The series the graphs are drawn from |
| Endpoint probe results | Yes | Uptime history |
| Container inventory | No | A full snapshot; replaying a stale one would archive live containers |
| Swarm / Kubernetes topology | No | Same, a snapshot of current state |
| Host resource samples | No | The server keeps only the latest value per host |
| Host operating system | No | Current state, re-sent on reconnect |
| Certificate scans | No | Current state; the agent rescans within 60 s of reconnecting |

On every reconnect the agent sends a **fresh** inventory and topology before it
starts replaying, so the current view is right immediately.

### Replay is history, not alerting

A replayed event feeds the graphs, the uptime and the container timeline, but it
feeds **only** the history. It never opens an alert or sends a notification: you
are not paged for an incident that is already over. It never overwrites current
state either: a replayed probe does not change an endpoint's status or its
consecutive-failure counter, a replayed container event adds a line to the
timeline without changing the container's state or health, and it does not feed
the restart counter or bring back an archived container. A container that only
appears in the replay (the fresh inventory did not list it, so it is gone) is
recorded as archived history.

Delivery is at-least-once. A stream that breaks mid-replay resends everything
past the last acknowledgement, so the server may see an event twice. Records are
keyed by the event, so a duplicate rewrites the same row instead of adding one.

### Limits

The spool is bounded so it cannot threaten the host it monitors. Past the memory
budget it spills to disk; past the disk budget the **oldest** events are dropped
in favour of the recent ones, and the count is reported to the server. Anything
older than the retention window is never replayed. If the spool database cannot
be opened, the agent logs a warning and keeps the queue in memory only, so what
is queued does not survive a restart.

The defaults absorb roughly an hour of outage on an ordinary host. Raise them if
your agents sit behind a link that fails for longer, and set both budgets to `0`
to disable the spool entirely. Keep the maximum age at 24 hours or less (the
default): the server refuses older events, and a value above the 48-hour window of raw resource samples
makes the agent log a warning at startup.

### Watching a catch-up

An agent replaying its backlog shows a **catching up · N** badge on the Agents page
with its queue depth, and a separate **N lost** marker if it had to drop events. The
badge clears once the agent is back in step.

---

## Per-Host Resource Metrics

In addition to per-container stats, each agent reports the **machine-level** CPU, memory and disk usage of the host it runs on. The central server keeps the latest sample for every host in memory (local server + each agent) and exposes it to the UI. A host that has not reported for 35 seconds shows no current metrics. The latest sample of each container of an agent is kept the same way, for 35 seconds, so the containers of agents appear in the live views of the resources API next to the server's own.

### Host selector and badge

As soon as one agent is enrolled, a **host scope** selector appears at the top of the sidebar with the entries *All resources*, *Local* and one per active agent. Picking a host scopes every list (containers, endpoints, certificates, heartbeats, workloads, pods, services, tasks, nodes), the dashboard gauges (CPU / MEM / DISK) and the **top consumers** widget to that machine. With no agent enrolled the selector is hidden and behaviour is unchanged.

Each container card carries a host badge (hostname / label) so you can tell at a glance which machine a workload runs on. The badge is shown as soon as an agent is enrolled and no host is selected; once you pick a host, every row is on it and the badge is hidden.

### Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/resources/hosts` | Lists every host (local + agents) with its current CPU / memory / disk and count of containers with a current sample. Enrolled agents that have not reported yet appear as unavailable. |
| `GET /api/v1/resources/summary?agent_id=local\|<id>` | Resource summary scoped to one host: container count and network rates from the current container samples, plus the host CPU, memory and disk. Omitting `agent_id` returns the local server. |
| `GET /api/v1/resources/top?...&agent_id=local\|<id>` | Top consumers, live or over a `period`. Omitting `agent_id` ranks the containers of all hosts, the server's own and those of agents. |
| `GET /api/v1/containers/{id}/resources/current` | Current sample of one container, local or on an agent. |

### Requirements

Host CPU and memory are read from `/proc`. When the agent runs inside a container it needs the host `/proc` mounted read-only:

```bash
-v /proc:/host/proc:ro
-v /etc/os-release:/host/etc/os-release:ro
```

The `/etc/os-release` mount identifies the host's distribution for end-of-support alerts; without it, the host shows as "unknown". The generated `docker run` and Compose snippets already include it, so no extra configuration is needed when you use them. On Kubernetes there is nothing to mount: the agent derives the node's identity from its `osImage` instead. Bare-metal/systemd agents read `/proc` and `/etc/os-release` natively. See [Host OS End-of-Support](host-os.md) for the full picture, including states and alert thresholds.

---

## Data Model

All monitored entities (`containers`, `endpoints`, `heartbeats`, `resources`, `certificates`, and the Swarm and Kubernetes topology) carry an `agent_id` column. It is never null:

| Value | Meaning |
|-------|---------|
| `00000000-0000-0000-0000-000000000000` | The server's own runtime (the local sentinel agent, a real row of the `agents` table). Every `embedded` install uses it. |
| `<uuid>` | Event from a remote agent |

The API exposes the sentinel as `local` where it takes an `agent_id` filter. Deleting an agent purges all its associated rows via SQL `ON DELETE CASCADE`.

---

## Agent Management

From **Agents** in the web UI:

| Action | Effect |
|--------|--------|
| **Revoke** | Closes the active stream immediately. Agent receives `PermissionDenied: agent_revoked` and stops retrying. |
| **Delete** | Revokes the stream and purges all historical events for that agent in a single transaction. Irreversible. |
| **Edit label** | Updates the display name (max 64 chars). Open pages follow through the `agent.updated` event. |

Agent status is updated in real time via SSE. The `connection_state` field reflects whether the agent is actively streaming (`connected`) or has not been seen for more than 60 seconds (`disconnected`, the `MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS` default).

When a stream drops or goes silent, a Warning alert (source `agent`, type `disconnected`) is raised, and resolved when the agent reconnects. Revoking or deleting an agent never raises one, and after a server restart the server waits 2 minutes before reporting agents that have not come back yet.

The Agents page shows each agent's runtime, version, spool state and operating system. The same data is available from `GET /api/v1/agents` (filters `status`, `connection_state`) and `GET /api/v1/agents/{id}`.

---

## Security

| Concern | Mechanism |
|---------|-----------|
| Transport encryption | TLS at the gRPC listener, or h2c behind a trusted reverse proxy (`MAINTENANT_GRPC_TLS_INSECURE=true`). Plaintext mode is explicit opt-in, not the default. A private CA is trusted through `MAINTENANT_CA_CERT`. |
| Per-stream authentication | Ed25519 challenge-response, fresh nonce per connection |
| Clock skew tolerance | ±5 minutes between agent and server clocks for the handshake |
| Token exposure | Cleartext shown once at creation, stored hashed, masked in subsequent reads |
| Key compromise | Revoke the agent from the UI; re-enrolling generates a new keypair |
| Edition enforcement | All `/api/v1/agents/*` endpoints return `403 EDITION_REQUIRED` below Personal, naming the capability and the edition that grants it |

### `--grpc-insecure-skip-tls-verify`

Available for development and testing against self-signed certificates. A boot-time warning is logged. **Do not use in production.**

---

## Configuration Reference

| Variable / Flag | Default | Description |
|-----------------|---------|-------------|
| `MAINTENANT_MODE` / `--mode` | `embedded` | `embedded`, `server` or `agent` |
| `MAINTENANT_GRPC_LISTEN` / `--grpc-listen` | `127.0.0.1:8443` | gRPC bind address (server) |
| `MAINTENANT_GRPC_URL` / `--grpc-url` | _(inferred)_ | Public gRPC URL injected into install commands (server) |
| `MAINTENANT_GRPC_TLS_CERT` / `--grpc-tls-cert` | none | Path to TLS certificate (server, direct TLS). Required together with the key |
| `MAINTENANT_GRPC_TLS_KEY` / `--grpc-tls-key` | none | Path to TLS private key (server, direct TLS) |
| `MAINTENANT_GRPC_TLS_INSECURE` / `--grpc-tls-insecure` | `false` | Accept h2c (plaintext HTTP/2), use behind a trusted reverse proxy only |
| `MAINTENANT_EMBEDDED_AGENT` / `--embedded-agent` | `false` | Also run a local agent (server mode, Personal or above) |
| `MAINTENANT_TRUSTED_PROXIES` / `--trustedProxies` | none | Peers whose `X-Forwarded-Host` is believed when inferring the gRPC URL (server) |
| `MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND` / `--agentRateLimitPerSecond` | `1000` | Max events/s per agent (server) |
| `MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS` / `--agentStaleThresholdSeconds` | `60` | Seconds before an agent is considered disconnected (server) |
| `MAINTENANT_SERVER` / `--server` | none | Server gRPC URL (agent, e.g. `grpcs://monitoring.example.com`; port defaults to 443) |
| `MAINTENANT_ENROLLMENT_TOKEN` / `--enrollment-token` | none | One-time enrollment token (agent): first boot, or a new enrollment when the server refuses the stored identity |
| `MAINTENANT_LABEL` / `--label` | _(hostname)_ | Display label for this agent, used at enrollment |
| `MAINTENANT_NODE_NAME` / `--nodeName` | _(found from the pod)_ | Kubernetes node the agent runs on (agent) |
| `MAINTENANT_RUNTIME` / `--runtime` | _(auto-detected)_ | Override runtime detection: `docker`, `swarm` (agent only) or `kubernetes` |
| `MAINTENANT_DATA_DIR` / `--data-dir` | `/var/lib/maintenant` | Directory of the agent's identity, liveness file and spool. The image healthcheck reads the variable, so prefer it over the flag |
| `MAINTENANT_CA_CERT` / `--ca-cert` | none | PEM bundle of extra root CAs added to the system store for every outbound HTTPS and gRPC connection. An agent uses it for its server, its probes and its scans. A bundle that cannot be read, or holds no valid certificate, stops startup |
| `MAINTENANT_GRPC_INSECURE_SKIP_TLS_VERIFY` / `--grpc-insecure-skip-tls-verify` | `false` | Skip TLS cert verification (agent, dev only) |
| `MAINTENANT_PROXY_LABELS` / `--proxyLabels` | `false` | Create endpoints from Traefik and Caddy labels. Read by each agent from its own environment, see [Reverse proxy labels](../guides/docker-labels.md#reverse-proxy-labels-traefik-caddy) |
| `MAINTENANT_AGENT_SPOOL_MAX_MEMORY_BYTES` / `--agentSpoolMaxMemoryBytes` | `16777216` (16 MB) | Buffer held in memory before spilling to disk (agent) |
| `MAINTENANT_AGENT_SPOOL_MAX_DISK_BYTES` / `--agentSpoolMaxDiskBytes` | `134217728` (128 MB) | Spool ceiling; past it the oldest events are dropped. `0` on both budgets disables the spool, and `0` alone lifts the ceiling |
| `MAINTENANT_AGENT_SPOOL_MAX_AGE_SECONDS` / `--agentSpoolMaxAgeSeconds` | `86400` (24 h) | Age past which a queued event is neither kept nor replayed. `0` sets no age limit |

A spool setting that is not a non-negative whole number stops the agent at startup instead of falling back to its default.

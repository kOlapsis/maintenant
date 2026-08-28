# Multi-Host Monitoring :material-star-four-points:{ title="Personal" }

Maintenant can monitor containers and workloads running on multiple remote hosts from a single central server. Remote hosts run a lightweight **agent** process that streams events to the server over a persistent gRPC connection.

---

## Modes

The `maintenant` binary supports three operating modes via the `--mode=` flag:

| Mode | Description |
|------|-------------|
| `embedded` | Default. Monitors the local runtime and stores data in local SQLite. No network exposure. |
| `server` | Central server. Receives events from remote agents via gRPC. Exposes the web UI and REST API. Personal or above. |
| `agent` | Remote agent. Monitors the local runtime and pushes events to a central server. Carries no license of its own: the server it enrolls with decides. |

> Community Edition enforces `--mode=embedded` at boot. Starting with `--mode=server` or `--mode=agent` exits with an error.

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

The port is optional and defaults to `443` — only add it (e.g. `:8443`) if agents reach gRPC directly instead of through a reverse proxy/DNS terminating TLS on 443.

If not set, the server infers the URL from the HTTP request headers (`X-Forwarded-Host`, `Host`). A warning is shown in the UI if the resolved URL appears to be a private address.

### Making the server replaceable

The server holds what the fleet cannot rebuild: agent identities and their
enrolments. On the default SQLite file, losing the machine means re-enrolling
every host by hand. Point the server at a PostgreSQL you already operate and a
replacement instance, started elsewhere with the same connection string, picks
the fleet back up with no action on any monitored machine.

Two files stay next to the instance rather than in the database — the signed
licence cache and the update window record — so keep the data directory with
the instance when it moves. See [PostgreSQL storage](../guides/postgresql.md).

Agents are unaffected either way: an agent stores its state in SQLite, always.

### Starting the server

Three TLS modes are supported — choose one:

**Behind a reverse proxy (h2c mode)**

Set `MAINTENANT_GRPC_TLS_INSECURE=true`. The listener accepts plaintext HTTP/2 (h2c); TLS is terminated at the proxy. The Docker-internal leg is unencrypted, which is safe within a private network.

```bash
MAINTENANT_GRPC_TLS_INSECURE=true maintenant --mode=server
```

The proxy must also let the stream live: it is a request whose body never ends, so any read timeout on the route (Traefik's `respondingTimeouts.readTimeout`, 60 s by default) severs it periodically. See [Agent Setup — long-lived streams](../guides/agent-setup.md#step-1-make-the-grpc-endpoint-reachable).

**Direct TLS with a custom certificate**

Mount a certificate/key pair covering the public hostname agents will dial:

```bash
maintenant \
  --mode=server \
  --grpc-listen=0.0.0.0:8443 \
  --grpc-tls-cert=/etc/maintenant/tls.crt \
  --grpc-tls-key=/etc/maintenant/tls.key
```

**Self-signed (development only)**

Omit both cert and key. The server generates a self-signed certificate in-memory, logs a warning, and agents must connect with `--grpc-insecure-skip-tls-verify`.

---

## Agent Enrollment

Agents authenticate with the server using a one-time enrollment token and an Ed25519 keypair generated locally on first boot.

### 1. Generate an enrollment token

From the web UI: **Agents → Generate enrollment token**

Or via API:

```
POST /api/v1/agents/enrollment-tokens
Authorization: Bearer <session>
Content-Type: application/json

{ "ttl_hours": 24 }
```

The response contains:
- `token` — the cleartext token, shown **once only**
- `install_templates` — a map of ready-to-run install snippets, one per environment. Keys:
  - `standalone` — `curl … | sudo bash` invocation of the install script (binary + systemd unit)
  - `docker_run` — single `docker run` command with the right socket/proc mounts
  - `docker_compose` — `compose.yml` snippet
  - `kubernetes` — Namespace + Secret + RBAC + DaemonSet manifest
- `warnings` — present if the server URL appears to be a private/local address

> The token cannot be retrieved again after creation. If lost, delete it and generate a new one.

!!! note "Only the hash is stored"
    The database keeps `sha256(token)` plus the first 14 characters, which is what every list and detail view displays (`mnt_enr_ab12cd...***`). The cleartext exists in the creation response and nowhere else, so a copy of the database file taken from here on — a backup, the data volume, a `.db` grabbed for debugging — carries nothing replayable.

    Upgrading an existing install rewrites the table in place on first boot and keeps outstanding tokens valid: nothing has to be reissued. Copies of the database made *before* that upgrade still contain the cleartext, and no migration can reach them. If one of those copies may have leaked, delete the affected tokens and issue new ones; otherwise their 7-day maximum TTL retires them on its own.

### 2. Run the install command on the remote host

Pick the snippet matching the host environment (the UI exposes these as tabs in the modal). For a bare-metal/VM host, the standalone snippet is:

```bash
curl -fsSL https://install.maintenant.dev | sudo bash -s -- \
  --mode=agent \
  --server=grpcs://monitoring.example.com \
  --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX
```

For a host where the binary is already installed, the equivalent invocation is:

```bash
maintenant \
  --mode=agent \
  --server=grpcs://monitoring.example.com \
  --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX \
  --label="prod-worker-01"
```

On first boot the agent:
1. Detects the local runtime (Docker, Swarm, or Kubernetes)
2. Generates an Ed25519 keypair and persists it to `identity.json` (mode `0600`)
3. Calls `RegisterAgent` on the server with the token and public key
4. Marks itself as enrolled and enters the streaming loop

The `--label` flag sets a human-readable display name (max 64 chars). If omitted, the hostname is used.

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
| Container start/stop/die/pause | Real-time |
| Per-container resource metrics (CPU, memory, network, disk) | Every 10 s |
| Host-level metrics (machine CPU, memory, disk) | Every 10 s |
| Certificate scans | Every 60 s |

### Reconnection

If the connection drops, the agent reconnects automatically with exponential backoff:

```
delay = min(60s, 1s × 2^attempt) ± 25% jitter
```

The attempt counter resets to 0 if the previous stream was stable for more than 30 seconds. Reconnection stops permanently only if the server responds with `agent_revoked`.

### Rate limiting

The server enforces a per-agent limit of **1 000 events/second** (token bucket). If exceeded, the server sends an in-stream error with a `retry_after_ms` hint.

---

## Per-Host Resource Metrics

In addition to per-container stats, each agent reports the **machine-level** CPU, memory and disk usage of the host it runs on. The central server keeps the latest sample for every host in memory (local server + each agent) and exposes it to the UI.

### Host selector

The dashboard's resource header (CPU / MEM / DISK gauges) shows a **host selector** as soon as more than one host is present. Pick a host to scope the gauges to that machine; the **top consumers** widget follows the same selection. With a single host the selector is hidden and behaviour is unchanged.

Each container card carries a host badge (hostname / label) so you can tell at a glance which machine a workload runs on. The badge is hidden when every visible container lives on the same host — there is nothing to disambiguate.

### Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/resources/hosts` | Lists every host (local + agents) with its current CPU / memory / disk and running-container count. |
| `GET /api/v1/resources/summary?agent_id=local\|<id>` | Resource summary scoped to one host. Omitting `agent_id` returns the local server. |
| `GET /api/v1/resources/top?...&agent_id=local\|<id>` | Top consumers scoped to one host. Omitting `agent_id` aggregates all hosts. |

### Requirements

Host CPU and memory are read from `/proc`. When the agent runs inside a container it needs the host `/proc` mounted read-only:

```bash
-v /proc:/host/proc:ro
```

The generated `docker run`, Compose and Kubernetes install snippets already include this mount, so no extra configuration is needed when you use them. Bare-metal/systemd agents read `/proc` natively.

---

## Data Model

All monitored entities (`containers`, `endpoints`, `heartbeats`, `resources`, `certificates`) carry an `agent_id` column:

| Value | Meaning |
|-------|---------|
| `NULL` | Local origin — mode `embedded` or embedded agent on the server |
| `<uuid>` | Event from a remote agent |

Deleting an agent purges all its associated rows via SQL `ON DELETE CASCADE`.

---

## Agent Management

From **Agents** in the web UI:

| Action | Effect |
|--------|--------|
| **Revoke** | Closes the active stream immediately. Agent receives `PermissionDenied: agent_revoked` and stops retrying. |
| **Delete** | Revokes the stream and purges all historical events for that agent in a single transaction. Irreversible. |
| **Edit label** | Updates the display name (max 64 chars). Takes effect on next UI refresh. |

Agent status is updated in real time via SSE. The `connection_state` field reflects whether the agent is actively streaming (`connected`) or has not been seen for more than 60 seconds (`disconnected`).

---

## Security

| Concern | Mechanism |
|---------|-----------|
| Transport encryption | TLS at the gRPC listener, or h2c behind a trusted reverse proxy (`MAINTENANT_GRPC_TLS_INSECURE=true`). Plaintext mode is explicit opt-in, not the default. |
| Per-stream authentication | Ed25519 challenge-response, fresh nonce per connection |
| Clock skew tolerance | ±5 minutes between agent and server clocks |
| Token exposure | Cleartext shown once at creation, stored hashed, masked in subsequent reads |
| Key compromise | Revoke the agent from the UI; re-enroll generates a new keypair |
| Edition enforcement | All `/api/v1/agents/*` endpoints return `403 EDITION_REQUIRED` below Personal, naming the capability and the edition that grants it |

### `--grpc-insecure-skip-tls-verify`

Available for development and testing against self-signed certificates. A boot-time warning is logged. **Do not use in production.**

---

## Configuration Reference

| Variable / Flag | Default | Description |
|-----------------|---------|-------------|
| `MAINTENANT_GRPC_LISTEN` / `--grpc-listen` | `127.0.0.1:8443` | gRPC bind address (server mode) |
| `MAINTENANT_GRPC_URL` / `--grpc-url` | _(inferred)_ | Public gRPC URL injected into install commands |
| `MAINTENANT_GRPC_TLS_CERT` / `--grpc-tls-cert` | — | Path to TLS certificate (server mode, direct TLS) |
| `MAINTENANT_GRPC_TLS_KEY` / `--grpc-tls-key` | — | Path to TLS private key (server mode, direct TLS) |
| `MAINTENANT_GRPC_TLS_INSECURE` | `false` | Accept h2c (plaintext HTTP/2) — use behind a trusted reverse proxy only |
| `--grpc-insecure-skip-tls-verify` | `false` | Skip TLS cert verification (agent mode, dev only) |
| `--server` | — | Server gRPC URL (agent mode, e.g. `grpcs://monitoring.example.com`; port defaults to 443) |
| `--enrollment-token` | — | One-time enrollment token (agent mode, first boot only) |
| `--label` | _(hostname)_ | Display label for this agent |
| `--runtime` | _(auto-detected)_ | Override runtime detection: `docker`, `swarm`, `kubernetes` |
| `MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND` | `1000` | Max events/s per agent (server mode) |
| `MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS` | `60` | Seconds before an agent is considered disconnected |

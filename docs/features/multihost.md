# Multi-Host Monitoring (Enterprise)

Maintenant can monitor containers and workloads running on multiple remote hosts from a single central server. Remote hosts run a lightweight **agent** process that streams events to the server over a persistent gRPC connection.

---

## Modes

The `maintenant` binary supports three operating modes via the `--mode=` flag:

| Mode | Description |
|------|-------------|
| `embedded` | Default. Monitors the local runtime and stores data in local SQLite. No network exposure. |
| `server` | Central server. Receives events from remote agents via gRPC. Exposes the web UI and REST API. Enterprise only. |
| `agent` | Remote agent. Monitors the local runtime and pushes events to a central server. Enterprise only. |

> Community Edition enforces `--mode=embedded` at boot. Starting with `--mode=server` or `--mode=agent` exits with an error.

---

## Server Setup

### Network interfaces

The server exposes two interfaces:

| Interface | Default | Purpose |
|-----------|---------|---------|
| HTTP | `127.0.0.1:8080` | REST API + web UI (reverse-proxied) |
| gRPC/TLS | `127.0.0.1:8443` | Agent event ingestion |

The gRPC port must be reachable from agent hosts. Configure the public URL so that generated install commands point to the right address:

```bash
MAINTENANT_GRPC_PUBLIC_URL=grpcs://monitoring.example.com:8443
```

If not set, the server infers the URL from the HTTP request headers (`X-Forwarded-Host`, `Host`). A warning is shown in the UI if the resolved URL appears to be a private address.

### Starting the server

```bash
maintenant \
  --mode=server \
  --grpc-listen=127.0.0.1:8443 \
  --grpc-tls-cert=/etc/maintenant/tls.crt \
  --grpc-tls-key=/etc/maintenant/tls.key
```

Plaintext gRPC is not supported. A valid TLS certificate is required.

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
- `install_command` — a ready-to-run command for the remote host
- `warnings` — present if the server URL appears to be a private/local address

> The token cannot be retrieved again after creation. If lost, delete it and generate a new one.

### 2. Run the install command on the remote host

```bash
maintenant \
  --mode=agent \
  --server=grpcs://monitoring.example.com:8443 \
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
| Resource metrics (CPU, memory, network, disk) | Every 10 s |
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

## Data Model

All monitored entities (`containers`, `endpoints`, `heartbeats`, `resources`, `certificates`) carry an `agent_id` column:

| Value | Meaning |
|-------|---------|
| `NULL` | Local origin — mode `embedded` or embedded agent on the server |
| `<uuid>` | Event from a remote agent |

Deleting an agent purges all its associated rows via SQL `ON DELETE CASCADE`.

---

## Agent Management

From **Agents** in the web UI (Enterprise):

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
| Transport encryption | TLS required on gRPC. Plaintext rejected at server boot. |
| Per-stream authentication | Ed25519 challenge-response, fresh nonce per connection |
| Clock skew tolerance | ±5 minutes between agent and server clocks |
| Token exposure | Cleartext shown once at creation, stored hashed, masked in subsequent reads |
| Key compromise | Revoke the agent from the UI; re-enroll generates a new keypair |
| CE enforcement | All `/api/v1/agents/*` endpoints return `402 pro_required` in Community Edition |

### `--grpc-insecure-skip-tls-verify`

Available for development and testing against self-signed certificates. A boot-time warning is logged. **Do not use in production.**

---

## Configuration Reference

| Variable / Flag | Default | Description |
|-----------------|---------|-------------|
| `MAINTENANT_GRPC_LISTEN` / `--grpc-listen` | `127.0.0.1:8443` | gRPC bind address (server mode) |
| `MAINTENANT_GRPC_PUBLIC_URL` / `--grpc-public-url` | _(inferred)_ | Public gRPC URL injected into install commands |
| `--grpc-tls-cert` | — | Path to TLS certificate (server mode) |
| `--grpc-tls-key` | — | Path to TLS private key (server mode) |
| `--grpc-insecure-skip-tls-verify` | `false` | Skip TLS cert verification (agent mode, dev only) |
| `--server` | — | Server gRPC URL (agent mode, e.g. `grpcs://host:8443`) |
| `--enrollment-token` | — | One-time enrollment token (agent mode, first boot only) |
| `--label` | _(hostname)_ | Display label for this agent |
| `--runtime` | _(auto-detected)_ | Override runtime detection: `docker`, `swarm`, `kubernetes` |
| `MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND` | `1000` | Max events/s per agent (server mode) |
| `MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS` | `60` | Seconds before an agent is considered disconnected |

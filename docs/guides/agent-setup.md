# Agent Setup Guide

Run a lightweight **agent** on a remote host to monitor its containers and workloads from a single central server. The agent streams events to the server over a persistent, TLS-encrypted gRPC connection.

This is a step-by-step guide. For the architecture, streaming protocol and full configuration reference, see [Multi-Host Monitoring](../features/multihost.md).

!!! info "Personal or Pro"
    Agent enrollment requires the **Personal** edition or above on the central
    server. Community runs in `embedded` mode only, on a single host, and
    refuses `--mode=server` at boot.

    Personal enrolls up to **20** remote hosts; Pro has no cap. The machine
    running maintenant is never counted against the limit.

---

## Prerequisites

- A Maintenant **server** (`--mode=server`) running the Personal edition or above.
- The server's **gRPC endpoint reachable from the agent host** (see [Step 1](#step-1-make-the-grpc-endpoint-reachable)).
- On the agent host: Docker, a Swarm node, a Kubernetes cluster, or just a bare-metal/VM host. The runtime is auto-detected.
- A free host slot. Personal caps enrolled hosts at 20, Pro has no cap. When the cap is reached, enrollment is rejected with `agent host limit reached`.

---

## Step 1 — Make the gRPC endpoint reachable

The agent dials the server over gRPC (separate from the HTTP/web port). On the server, set:

```bash
MAINTENANT_GRPC_LISTEN=0.0.0.0:8443          # bind on all interfaces, not just loopback
MAINTENANT_GRPC_URL=grpcs://agents.example.com   # the address agents will dial
```

`MAINTENANT_GRPC_URL` is the public address injected into the generated install commands. If you omit it, the server infers it from request headers and warns in the UI when the result looks local/private.

**TLS mode — choose one that matches your deployment:**

=== "Behind a reverse proxy (recommended)"

    Set `MAINTENANT_GRPC_TLS_INSECURE=true` on the server. The gRPC listener accepts plaintext HTTP/2 (h2c) on `:8443`; TLS is terminated at the proxy edge with a Let's Encrypt certificate. Agents dial `grpcs://agents.example.com` (port 443) and validate the public cert normally — no extra flag needed.

    Example Traefik labels on the server container:

    ```yaml
    - traefik.http.routers.maintenant-grpc.rule=Host(`agents.example.com`)
    - traefik.http.routers.maintenant-grpc.entrypoints=websecure
    - traefik.http.routers.maintenant-grpc.tls.certresolver=le
    - traefik.http.routers.maintenant-grpc.service=maintenant-grpc
    - traefik.http.services.maintenant-grpc.loadbalancer.server.port=8443
    - traefik.http.services.maintenant-grpc.loadbalancer.server.scheme=h2c
    ```

    !!! warning "Long-lived streams — disable the proxy's read timeout"
        The agent stream is a single request whose body never ends. Any proxy that caps the
        duration of a request kills it at that limit regardless of the traffic on it, and the
        agent reconnects in a loop — remote containers then appear and disappear depending on
        when the reconnect lands.

        On **Traefik v3** the culprit is `respondingTimeouts.readTimeout`, 60 s by default.
        It is configured **per entrypoint**, so it cannot be relaxed for gRPC alone on a shared
        `websecure` entrypoint — use a dedicated one:

        ```yaml
        # Traefik static configuration (CLI flags)
        - --entrypoints.grpc.address=:8443
        - --entrypoints.grpc.transport.respondingTimeouts.readTimeout=0
        - --entrypoints.grpc.transport.respondingTimeouts.idleTimeout=0
        ```

        ```yaml
        # server container labels — same router, on the grpc entrypoint
        - traefik.http.routers.maintenant-grpc.rule=Host(`agents.example.com`)
        - traefik.http.routers.maintenant-grpc.entrypoints=grpc
        - traefik.http.routers.maintenant-grpc.tls.certresolver=le
        - traefik.http.routers.maintenant-grpc.service=maintenant-grpc
        - traefik.http.services.maintenant-grpc.loadbalancer.server.port=8443
        - traefik.http.services.maintenant-grpc.loadbalancer.server.scheme=h2c
        ```

        Two consequences of a non-443 entrypoint:

        - agents must include the port: `--server=grpcs://agents.example.com:8443`;
        - ACME `tlschallenge` requires port 443, so a hostname served only on this entrypoint
          gets no certificate that way — keep a router for it on `websecure`, or switch to
          `httpchallenge`.

        Other proxies have the same class of setting: nginx `grpc_read_timeout`, HAProxy
        `timeout tunnel`. The agent pushes a full inventory every 30 s, so idle timeouts
        (Traefik's `idleTimeout`, 180 s) are normally not hit — only the read timeout is.

=== "Direct TLS with a custom certificate"

    Mount a certificate/key pair and point to them with env vars:

    ```bash
    MAINTENANT_GRPC_TLS_CERT=/etc/maintenant/tls.crt
    MAINTENANT_GRPC_TLS_KEY=/etc/maintenant/tls.key
    ```

    The certificate must cover the hostname agents will dial. With a valid public certificate (e.g. obtained via a DNS ACME challenge), agents connect without any extra flag. Works well with Traefik TCP passthrough:

    ```yaml
    - traefik.tcp.routers.maintenant-grpc.rule=HostSNI(`agents.example.com`)
    - traefik.tcp.routers.maintenant-grpc.entrypoints=websecure
    - traefik.tcp.routers.maintenant-grpc.tls.passthrough=true
    - traefik.tcp.services.maintenant-grpc.loadbalancer.server.port=8443
    ```

=== "Self-signed (development only)"

    No certificate configuration needed. The server generates a self-signed cert in-memory at startup and logs a warning. Agents must pass `--grpc-insecure-skip-tls-verify`. **Do not use in production.**

---

## Step 2 — Generate an enrollment token

In the web UI: **Agents → Generate enrollment token**.

The modal shows:

- the **cleartext token** — displayed **once only** (stored hashed afterwards),
- a ready-to-run **install snippet** per environment (Standalone, Docker run, Compose, Kubernetes).

!!! warning "One-time secret"
    The token cannot be retrieved again. If you lose it, delete it and generate a new one. A token is consumed on first successful enrollment.

---

## Step 3 — Run the agent on the host

Pick the tab matching the host environment. Replace `grpcs://agents.example.com` and `mnt_enr_XXXX…` with the values from your enrollment modal.

=== "Standalone (binary + systemd)"

    !!! warning "Not released yet"
        The one-line installer that drops the binary and a systemd unit is still being built,
        and the modal's **Standalone** tab says so. Until it ships, run the agent with Docker
        (next tabs) or drive an already-installed binary yourself.

    With the binary in place, the agent is a plain invocation:

    ```bash
    maintenant \
      --mode=agent \
      --server=grpcs://agents.example.com \
      --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX \
      --label="prod-worker-01"
    ```

=== "Docker run"

    ```bash
    docker run -d \
      --name maintenant-agent \
      --restart unless-stopped \
      -v /var/run/docker.sock:/var/run/docker.sock:ro \
      -v /proc:/host/proc:ro \
      -v maintenant-agent-data:/var/lib/maintenant \
      ghcr.io/kolapsis/maintenant:latest \
      --mode=agent \
      --server=grpcs://agents.example.com \
      --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX
    ```

=== "Docker Compose"

    ```yaml
    services:
      maintenant-agent:
        image: ghcr.io/kolapsis/maintenant:latest
        restart: unless-stopped
        volumes:
          - /var/run/docker.sock:/var/run/docker.sock:ro
          - /proc:/host/proc:ro
          - maintenant-agent-data:/var/lib/maintenant
        command:
          - --mode=agent
          - --server=grpcs://agents.example.com
          - --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX

    volumes:
      maintenant-agent-data:
    ```

=== "Kubernetes"

    Deploys as a DaemonSet (one agent per node) with read-only RBAC. The Kubernetes snippet from the modal includes the Namespace, Secret, ServiceAccount, ClusterRole/Binding and DaemonSet. Apply it with:

    ```bash
    kubectl apply -f maintenant-agent.yaml
    ```

    The DaemonSet passes `--runtime=kubernetes` and reads the token from a `Secret`, so the cluster is monitored at the workload/pod level.

!!! tip "Containerized agent without the socket mount"
    The agent uses the same Docker client as the server and honours `DOCKER_HOST`. To avoid
    handing it the raw socket, run a
    [docker-socket-proxy](../security.md#recommended-docker-socket-proxy) on the host and
    replace the `docker.sock` volume with `DOCKER_HOST=tcp://socketproxy:2375` (shared Docker
    network with the proxy). The agent's API usage is read-only, so the proxy's default
    write-blocking applies cleanly.

What happens on first boot:

1. The agent detects the local runtime (Docker, Swarm, or Kubernetes).
2. It generates an Ed25519 keypair and persists it to `identity.json` (mode `0600`) in its data dir.
3. It calls `RegisterAgent` with the token + public key, then enters the streaming loop.

The keypair lives in the data volume (`/var/lib/maintenant`). Keep that volume to preserve the agent's identity across restarts — losing it requires re-enrollment.

---

## Step 4 — Verify

- **Agents** page: the new host appears with `connection_state: connected` (updated live).
- **Dashboard**: a host selector appears on the CPU / MEM / DISK gauges once more than one host is present, and container cards carry a host badge.

If the host stays `disconnected` for more than 60 s, see [Troubleshooting](#troubleshooting).

- **Docker health**: `docker ps` reports the agent container as `healthy` once it is streaming.

The image ships a healthcheck that works in both modes, so no override is needed in your compose file. An agent has no HTTP port: it refreshes a `health` file in its data dir every 15 s, and `/app/maintenant healthcheck` reads it. A server is probed on the address in `MAINTENANT_ADDR` instead.

The check answers "is this agent working", not "does it reach the server". An agent that has lost the server stays `healthy` and keeps retrying, because restarting it would fix nothing. The outage is reported server-side, as a disconnected-agent alert.

---

## Useful flags

| Flag | Purpose |
|------|---------|
| `--server` | Server gRPC URL, e.g. `grpcs://agents.example.com` (port defaults to 443). |
| `--enrollment-token` | One-time token, first boot only. Ignored once enrolled. |
| `--label` | Display name (max 64 chars). Defaults to the hostname. |
| `--runtime` | Override auto-detection: `docker`, `swarm`, or `kubernetes`. |
| `--grpc-insecure-skip-tls-verify` | Skip TLS verification — **development only**, for self-signed servers. |

The full reference (server-side variables, rate limits, stale thresholds) is in [Multi-Host Monitoring → Configuration Reference](../features/multihost.md#configuration-reference).

---

## Troubleshooting

!!! failure "`agent host limit reached`"
    The server's edition has reached its enrolled-host cap. Remove an unused agent (**Agents → Delete**) or upgrade the edition, then retry.

!!! failure "`enrollment token already consumed` / `expired`"
    Tokens are single-use and time-limited. Generate a fresh one and re-run the install command.

!!! failure "Host stays `disconnected`"
    - Confirm the gRPC port/subdomain is reachable from the agent host (firewall, DNS, reverse-proxy route).
    - If the server uses a self-signed certificate, the agent must either trust it or run with `--grpc-insecure-skip-tls-verify` (dev only). With a real (Let's Encrypt) certificate, no flag is needed.

    These cause a *permanent* failure. A host that connects then drops at a fixed interval is a different problem — see below.

!!! failure "The agent reconnects at a fixed interval (60 s behind Traefik)"
    A *periodic* drop points at the reverse proxy, not at routing: the stream is one request
    that never completes, and the proxy closes it when its read timeout expires. Nothing in the
    agent logs names the proxy — it just looks like an unstable link. Disable the read timeout
    on the entrypoint carrying gRPC, see
    [Step 1 — long-lived streams](#step-1-make-the-grpc-endpoint-reachable).

!!! failure "Permission denied on the Docker socket"
    The containerized agent runs unprivileged but auto-detects the mounted socket's group, so
    just mounting `/var/run/docker.sock` is normally enough. If it still can't read the socket
    (non-standard path or socket proxy), pin the GID explicitly:

    ```bash
    # docker run
    -e DOCKER_GID="$(stat -c '%g' /var/run/docker.sock)"
    ```

    ```yaml
    # compose
    environment:
      DOCKER_GID: "<docker-gid>"   # e.g. from: stat -c '%g' /var/run/docker.sock
    ```

!!! warning "`public_url_appears_local` warning in the modal"
    The resolved server URL is a localhost/private address, so remote agents can't reach it. Set `MAINTENANT_GRPC_URL` to a publicly reachable address (see [Step 1](#step-1-make-the-grpc-endpoint-reachable)).

---

## Managing agents

From **Agents** in the web UI:

| Action | Effect |
|--------|--------|
| **Revoke** | Closes the stream immediately; the agent stops retrying (`agent_revoked`). |
| **Delete** | Revokes and purges all of the agent's historical events. Irreversible. |
| **Edit label** | Updates the display name. |

To re-enroll a host after revocation, generate a new token and re-run the install command — the agent generates a fresh keypair.

---

## See also

- [Multi-Host Monitoring](../features/multihost.md) — architecture, streaming protocol, security, full config reference
- [Docker Labels Reference](docker-labels.md) — declare endpoint/heartbeat/TLS checks on monitored containers
- [Kubernetes Guide](kubernetes.md) — cluster-native deployment and RBAC

# Agent Setup Guide

Run a lightweight **agent** on a remote host to monitor its containers and workloads from a single central server. The agent streams events to the server over a persistent, TLS-encrypted gRPC connection.

This is a step-by-step guide. For the architecture, streaming protocol and full configuration reference, see [Multi-Host Monitoring](../features/multihost.md).

!!! info "Personal or Pro"
    Agent enrollment requires the **Personal** edition or above on the central
    server. Community has no agent listener, runs on a single host, and refuses
    `--mode=server` at boot.

    Personal enrolls up to **20** remote hosts; Pro has no cap. The machine
    running maintenant is never counted against the limit, and neither are
    revoked agents. A Kubernetes cluster is one agent, so one host.

---

## Prerequisites

- A Maintenant **server** running the Personal edition or above (`--mode=server`, or the default mode: the agent listener starts in both).
- The server's **gRPC endpoint reachable from the agent host** (see [Step 1](#step-1-make-the-grpc-endpoint-reachable)).
- On the agent host: a Docker engine (a Swarm node included) or a Kubernetes cluster, detected automatically. The runtime does not have to answer at startup: without one, the agent still enrolls and reports the host (metrics and operating system), and starts container monitoring as soon as a runtime answers.
- A free host slot. Personal caps enrolled hosts at 20, Pro has no cap. When the cap is reached, generating a token is refused (`409 HOST_LIMIT_REACHED`) and enrollment is rejected with `agent host limit reached`.

---

## Step 1: Make the gRPC endpoint reachable

The agent dials the server over gRPC (separate from the HTTP/web port). On the server, set:

```bash
MAINTENANT_GRPC_LISTEN=0.0.0.0:8443          # bind on all interfaces, not just loopback
MAINTENANT_GRPC_URL=grpcs://agents.example.com   # the address agents will dial
```

In a container, `0.0.0.0:8443` is the address inside the container: publish the port too (`"8443:8443"`), or let the reverse proxy reach it on the Docker network. A bind to a private IP address of the host does not work from inside a container.

`MAINTENANT_GRPC_URL` is the public address injected into the generated install commands. If you omit it, the server infers it from the request that creates the token: `X-Forwarded-Host` only when the request comes from a peer listed in `MAINTENANT_TRUSTED_PROXIES`, the `Host` header otherwise, port included. That is the address of the web UI, not of gRPC, so set the variable. The token response carries a warning when the inferred address looks wrong: `public_url_appears_local` (shown in the modal) when it is local or private, and `public_url_plaintext_refused` when the proxy forwarded the request as plaintext gRPC.

**TLS mode: choose one that matches your deployment:**

=== "Behind a reverse proxy (recommended)"

    Set `MAINTENANT_GRPC_TLS_INSECURE=true` on the server. The gRPC listener accepts plaintext HTTP/2 (h2c) on `:8443`; TLS is terminated at the proxy edge with a Let's Encrypt certificate. Agents dial `grpcs://agents.example.com` (port 443) and validate the public cert normally: no extra flag needed.

    Example Traefik labels on the server container:

    ```yaml
    - traefik.http.routers.maintenant-grpc.rule=Host(`agents.example.com`)
    - traefik.http.routers.maintenant-grpc.entrypoints=websecure
    - traefik.http.routers.maintenant-grpc.tls.certresolver=le
    - traefik.http.routers.maintenant-grpc.service=maintenant-grpc
    - traefik.http.services.maintenant-grpc.loadbalancer.server.port=8443
    - traefik.http.services.maintenant-grpc.loadbalancer.server.scheme=h2c
    ```

    !!! warning "Long-lived streams: disable the proxy's read timeout"
        The agent stream is a single request whose body never ends. Any proxy that caps the
        duration of a request kills it at that limit regardless of the traffic on it, and the
        agent reconnects in a loop: remote containers then appear and disappear depending on
        when the reconnect lands.

        On **Traefik v3** the culprit is `respondingTimeouts.readTimeout`, 60 s by default.
        It is configured **per entrypoint**, so it cannot be relaxed for gRPC alone on a shared
        `websecure` entrypoint. Use a dedicated one:

        ```yaml
        # Traefik static configuration (CLI flags)
        - --entrypoints.grpc.address=:8443
        - --entrypoints.grpc.transport.respondingTimeouts.readTimeout=0
        - --entrypoints.grpc.transport.respondingTimeouts.idleTimeout=0
        ```

        ```yaml
        # server container labels: same router, on the grpc entrypoint
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
          gets no certificate that way: keep a router for it on `websecure`, or switch to
          `httpchallenge`.

        Other proxies have the same class of setting: nginx `grpc_read_timeout`, HAProxy
        `timeout tunnel`. The agent pushes a full inventory every 30 s, so idle timeouts
        (Traefik's `idleTimeout`, 180 s) are normally not hit, only the read timeout is.

=== "Direct TLS with a custom certificate"

    Mount a certificate/key pair and point to them with env vars:

    ```bash
    MAINTENANT_GRPC_TLS_CERT=/etc/maintenant/tls.crt
    MAINTENANT_GRPC_TLS_KEY=/etc/maintenant/tls.key
    ```

    Both are required: the server refuses to start with only one of them. The certificate must cover the hostname agents will dial. With a valid public certificate (e.g. obtained via a DNS ACME challenge), agents connect without any extra flag. With a certificate from a private CA, give that CA to every agent with `MAINTENANT_CA_CERT` (a PEM bundle added to the system roots, see [Useful flags](#useful-flags)). Works well with Traefik TCP passthrough:

    ```yaml
    - traefik.tcp.routers.maintenant-grpc.rule=HostSNI(`agents.example.com`)
    - traefik.tcp.routers.maintenant-grpc.entrypoints=websecure
    - traefik.tcp.routers.maintenant-grpc.tls.passthrough=true
    - traefik.tcp.services.maintenant-grpc.loadbalancer.server.port=8443
    ```

=== "Self-signed (development only)"

    No certificate configuration needed. The server generates a self-signed cert in-memory at startup and logs a warning. Agents must pass `--grpc-insecure-skip-tls-verify`. **Do not use in production.**

---

## Step 2: Generate an enrollment token

In the web UI: **Agents → Generate enrollment token**.

The modal shows:

- the **cleartext token**, displayed **once only** (stored hashed afterwards),
- a ready-to-run **install snippet** per environment (Docker run, Compose, Kubernetes, Standalone),
- the expiry of the token: 24 hours by default, 7 days at most (`ttl_hours` on `POST /api/v1/agents/enrollment-tokens`).

!!! warning "One-time secret"
    The token cannot be retrieved again. If you lose it, delete it and generate a new one. A token is consumed on first successful enrollment.

---

## Step 3: Run the agent on the host

Pick the tab matching the host environment. Replace `grpcs://agents.example.com` and `mnt_enr_XXXX…` with the values from your enrollment modal.

=== "Standalone (binary + systemd)"

    The installer downloads the release binary, checks it, and installs it as a systemd service. The script is the `install.sh` of the latest release, served at `install.maintenant.dev`:

    ```bash
    curl -fsSL https://install.maintenant.dev | sudo bash -s -- \
      --mode agent \
      --server grpcs://agents.example.com \
      --enrollment-token mnt_enr_XXXXXXXXXXXXXXXX \
      --label "prod-worker-01"
    ```

    The settings are written to `/etc/maintenant/maintenant.env`, and the agent keeps its identity in `/var/lib/maintenant`. If a `docker` group exists, the `maintenant` service user is added to it so it can read the socket: that group is equivalent to root access on the host, and a Docker installed after the script ran is not picked up. To change a setting later, run the script again with the new flag: only the keys you pass are replaced. See [Native Linux Install](../install.md) for every option.

    With the binary already in place, the agent is a plain invocation:

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
      -v /etc/os-release:/host/etc/os-release:ro \
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
          - /etc/os-release:/host/etc/os-release:ro
          - maintenant-agent-data:/var/lib/maintenant
        command:
          - --mode=agent
          - --server=grpcs://agents.example.com
          - --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX

    volumes:
      maintenant-agent-data:
    ```

=== "Kubernetes"

    Deploys **one agent per cluster**, as a single-replica Deployment (strategy `Recreate`) that counts as one host. The Kubernetes snippet from the modal includes the Namespace, Secret, ServiceAccount, ClusterRole/Binding, PersistentVolumeClaim and Deployment. Apply it with:

    ```bash
    kubectl apply -f maintenant-agent.yaml
    ```

    The Deployment passes `--runtime=kubernetes`, reads the token from a `Secret` and the node name from `spec.nodeName`, and keeps the agent identity on a 1 Gi volume mounted on `/var/lib/maintenant`. The pod is hardened: non-root (65534), read-only root filesystem, no capabilities. The cluster is monitored at the workload/pod level.

    The ClusterRole is read-only: `get`, `list` and `watch` on namespaces, nodes, pods, events, services, deployments, statefulsets, daemonsets and jobs, `get` on `pods/log`, and `get` and `list` on the `metrics.k8s.io` pods and nodes. The agent appears under the hostname `maintenant-agent`: rename it from the Agents page.

!!! tip "Containerized agent without the socket mount"
    The agent uses the same Docker client as the server and honours `DOCKER_HOST`. To avoid
    handing it the raw socket, run a
    [docker-socket-proxy](../security.md#recommended-docker-socket-proxy) on the host and
    replace the `docker.sock` volume with `DOCKER_HOST=tcp://socketproxy:2375` (shared Docker
    network with the proxy). The agent's API usage is read-only, so the proxy's default
    write-blocking applies cleanly.

What happens on first boot:

1. The agent detects the local runtime (Docker, Swarm, or Kubernetes). If the runtime does not answer, the agent logs `container runtime unreachable, reporting host metrics only until it answers` and retries in the background with a delay from 1 second to 1 minute. It logs `container runtime reachable, container monitoring started` once it answers.
2. It generates an Ed25519 keypair and persists it to `identity.json` (mode `0600`) in its data dir.
3. It calls `RegisterAgent` with the token + public key, then enters the streaming loop.

The keypair lives in the data volume (`/var/lib/maintenant`). Keep that volume to preserve the agent's identity across restarts. Losing it makes the agent a new one, which needs a new token (see [Managing agents](#managing-agents)). The volume also holds the [outage spool](../features/multihost.md#outage-spool).

The runtime shown on the Agents page is the one the agent detected, reported after enrollment. It follows the host: a runtime that answers late, or a Docker host that joins or leaves a Swarm, updates it without re-enrolling. If the runtime stops answering while the agent runs, the agent logs `container runtime lost, waiting for it to come back`, keeps reporting the host, and resumes container monitoring (with a fresh inventory) when the runtime is back.

---

## Step 4: Verify

- **Agents** page: the new host appears with `connection_state: connected` (updated live).
- **Dashboard**: a **host scope** selector appears in the header as soon as an agent is enrolled, and container cards carry a host badge.

If the host stays `disconnected` for more than 60 s, see [Troubleshooting](#troubleshooting).

- **Docker health**: `docker ps` reports the agent container as `healthy` once it is streaming.

The image ships a healthcheck that works in both modes, so no override is needed in your compose file. An agent has no HTTP port: it refreshes a `health` file in its data dir every 15 s, and `/app/maintenant healthcheck` reads it. A server is probed on the address in `MAINTENANT_ADDR` instead.

The check answers "is this agent working", not "does it reach the server". An agent that has lost the server stays `healthy` and keeps retrying, because restarting it would fix nothing. The outage is reported server-side, as a disconnected-agent alert.

---

## Useful flags

Every flag has an environment variable with the same effect (`--server` is `MAINTENANT_SERVER`, `--enrollment-token` is `MAINTENANT_ENROLLMENT_TOKEN`, and so on); the flag wins when both are set. The flag names are spelled as below.

| Flag | Purpose |
|------|---------|
| `--mode` | `agent` (also `MAINTENANT_MODE=agent`). |
| `--server` | Server gRPC URL, e.g. `grpcs://agents.example.com` (port defaults to 443). |
| `--enrollment-token` | One-time token. Needed on first boot, and again to enroll as a new agent when the server refuses the stored identity. Once enrolled and accepted, it is not used. |
| `--label` | Display name at enrollment. Defaults to the hostname, which also replaces a label longer than 64 characters. |
| `--runtime` | Override auto-detection: `docker`, `swarm`, or `kubernetes`. |
| `--nodeName` | Kubernetes node the agent runs on. Found from the pod when empty. |
| `--data-dir` | Directory of the identity, liveness file and spool (default `/var/lib/maintenant`). The image healthcheck reads `MAINTENANT_DATA_DIR`, so set the variable rather than the flag if you move it. |
| `--ca-cert` | PEM bundle of extra root CAs (`MAINTENANT_CA_CERT`), added to the system roots. Applies to the connection to the server, and to the endpoint probes and certificate scans of the agent. |
| `--proxyLabels` | Create endpoints from Traefik and Caddy labels (`MAINTENANT_PROXY_LABELS`). Each agent reads its own setting, independently of the server. |
| `--agentSpoolMaxMemoryBytes`, `--agentSpoolMaxDiskBytes`, `--agentSpoolMaxAgeSeconds` | Bounds of the outage spool (16 MB, 128 MB and 24 h by default). `0` memory writes every event to disk, `0` disk lifts the size limit, `0` age sets no age limit. The spool is off only when both budgets are `0`. |
| `--grpc-insecure-skip-tls-verify` | Skip TLS verification: **development only**, for self-signed servers. |

The full reference (server-side variables, rate limits, stale thresholds) is in [Multi-Host Monitoring → Configuration Reference](../features/multihost.md#configuration-reference).

---

## Troubleshooting

!!! failure "`agent host limit reached`"
    The server's edition has reached its enrolled-host cap. Remove an unused agent (**Agents → Delete**) or upgrade the edition, then retry.

!!! failure "`enrollment token already consumed` / `expired`"
    Tokens are single-use and time-limited. Generate a fresh one and re-run the install command.

!!! failure "Host stays `disconnected`"
    - Confirm the gRPC port/subdomain is reachable from the agent host (firewall, DNS, reverse-proxy route), and that `MAINTENANT_GRPC_LISTEN` is not left on its loopback default.
    - If the server certificate comes from a private CA, give the agent that CA with `MAINTENANT_CA_CERT`. If it is self-signed, the agent must either trust it the same way or run with `--grpc-insecure-skip-tls-verify` (dev only). With a real (Let's Encrypt) certificate, no flag is needed.
    - If the agent log says the agent is unknown to the server or revoked, see [Managing agents](#managing-agents).

    These cause a *permanent* failure. A host that connects then drops at a fixed interval is a different problem, see below.

!!! failure "The agent reconnects at a fixed interval (60 s behind Traefik)"
    A *periodic* drop points at the reverse proxy, not at routing: the stream is one request
    that never completes, and the proxy closes it when its read timeout expires. Nothing in the
    agent logs names the proxy: it just looks like an unstable link. Disable the read timeout
    on the entrypoint carrying gRPC, see
    [Step 1: long-lived streams](#step-1-make-the-grpc-endpoint-reachable).

!!! failure "Permission denied on the Docker socket"
    The containerized agent runs unprivileged but auto-detects the mounted socket's group, so
    just mounting `/var/run/docker.sock` is normally enough. If it still can't read the socket
    (non-standard path or socket proxy, or a `root:root` socket with no `docker` group as on
    Synology DSM, where the GID to pin is `0`), pin the GID explicitly:

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
| **Revoke** | Closes the stream immediately. The server refuses the agent from then on (`agent_revoked`). |
| **Delete** | Revokes and purges all of the agent's historical events. Irreversible. |
| **Edit label** | Updates the display name. |

A refused agent (revoked, or deleted from the server) stops streaming and drops its spool. To bring the host back, create a new enrollment token and start the agent again with it (`MAINTENANT_ENROLLMENT_TOKEN` or `--enrollment-token`, then recreate the container or restart the service): the agent notices that the server refused its identity, enrolls again with a fresh keypair and replaces `identity.json` only when that succeeds. The host appears as a new agent, and you do not need to empty the data volume. If the token is missing, already used or expired, the agent exits with a message asking for a new one.

---

## See also

- [Multi-Host Monitoring](../features/multihost.md): architecture, streaming protocol, security, full config reference
- [Docker Labels Reference](docker-labels.md): declare endpoint/heartbeat/TLS checks on monitored containers
- [Kubernetes Guide](kubernetes.md): cluster-native deployment and RBAC

# Agent Setup Guide

Run a lightweight **agent** on a remote host to monitor its containers and workloads from a single central server. The agent streams events to the server over a persistent, TLS-encrypted gRPC connection.

This is a step-by-step guide. For the architecture, streaming protocol and full configuration reference, see [Multi-Host Monitoring](../features/multihost.md).

!!! info "Pro feature"
    Agent enrollment requires the **Pro** edition on the central server. Community Edition runs in `embedded` mode only (single host) and rejects `--mode=agent`/`--mode=server` at boot.

---

## Prerequisites

- A Maintenant **server** (`--mode=server`) running the Pro edition.
- The server's **gRPC endpoint reachable from the agent host** (see [Step 1](#step-1-make-the-grpc-endpoint-reachable)).
- On the agent host: Docker, a Swarm node, a Kubernetes cluster, or just a bare-metal/VM host. The runtime is auto-detected.
- A free host slot — each Pro edition caps the number of enrolled hosts. If the cap is reached, enrollment is rejected with `agent host limit reached`.

---

## Step 1 — Make the gRPC endpoint reachable

The agent dials the server over gRPC (separate from the HTTP/web port). On the server, set:

```bash
MAINTENANT_GRPC_LISTEN=0.0.0.0:8443          # bind on all interfaces, not just loopback
MAINTENANT_GRPC_URL=grpcs://agents.example.com   # the address agents will dial
```

`MAINTENANT_GRPC_URL` is the public address injected into the generated install commands. If you omit it, the server infers it from request headers and warns in the UI when the result looks local/private.

!!! tip "Behind a reverse proxy (recommended)"
    The gRPC server always serves its own TLS. To present a public Let's Encrypt certificate instead of a self-signed one, terminate TLS at your reverse proxy on a dedicated subdomain and re-encrypt to the backend on `:8443` over HTTP/2. Agents then dial `grpcs://agents.example.com` (port 443) and validate the certificate normally — no insecure flag needed.

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

    ```bash
    curl -fsSL https://install.maintenant.dev | sudo bash -s -- \
      --mode=agent \
      --server=grpcs://agents.example.com \
      --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX
    ```

    The install script drops the binary and a systemd unit. If the binary is already installed, the equivalent invocation is:

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

!!! failure "Permission denied on the Docker socket"
    The containerized agent runs unprivileged. If it can't read `/var/run/docker.sock`, add the host's `docker` group:

    ```bash
    # docker run
    --group-add "$(getent group docker | cut -d: -f3)"
    ```

    ```yaml
    # compose
    group_add:
      - "<docker-gid>"   # e.g. the GID from: getent group docker
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

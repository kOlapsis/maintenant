# maintenant

Self-hosted monitoring for Docker, Docker Swarm and Kubernetes, shipped as one Go binary that uses about 100 MB of RAM. Run the container and your stack is discovered and watched: no exporters, no query language, no dashboards to build.

Live demo: https://demo.maintenant.dev/
Source, issues and releases: https://github.com/kolapsis/maintenant
Documentation: https://docs.maintenant.dev/

## What it monitors

- Containers and workloads: state, health checks, restart loops, CPU, memory, network and disk, live logs. Docker, Swarm and Kubernetes (Deployments, DaemonSets, StatefulSets).
- Endpoints: HTTP and TCP checks, declared as container labels.
- TLS certificates: expiry and chain validation.
- Cron jobs and heartbeats: missed-deadline detection.
- Image updates: new versions detected, with compose-aware update and rollback commands.
- Network exposure: `0.0.0.0` binds, exposed database ports, host network, privileged containers.
- Alerting: Discord and webhooks in Community, more channels and escalation in paid editions.
- Public status page with incidents.
- A built-in MCP server, so an AI agent can read alerts, logs and state and investigate them.

## Quick start

```yaml
# docker-compose.yml
services:
  maintenant:
    image: kolapsis/maintenant:latest
    ports:
      - "127.0.0.1:8080:8080"
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

volumes:
  maintenant-data:
```

```bash
docker compose up -d
```

Open http://localhost:8080. Your containers are already listed.

The entrypoint reads the group of the mounted Docker socket and grants it to the unprivileged user. A socket owned by group `root` needs `DOCKER_GID=0`. If containers do not show up, see https://docs.maintenant.dev/troubleshooting/.

## Security

The web UI and the API have no authentication of their own. Bind the port to `127.0.0.1` as above, or put an authenticating reverse proxy in front of it, before exposing it to a network. Reverse proxy setup: https://docs.maintenant.dev/security/#reverse-proxy-setup.

The Docker socket is mounted read-only. The image runs as an unprivileged user with a read-only root filesystem.

## Tags

- `latest`: the most recent release.
- `1.11.1`, `1.11`, `1`: a specific release, minor line or major line.

The image is published for `linux/amd64` and `linux/arm64`. It is the same image, with the same digest, as `ghcr.io/kolapsis/maintenant`. It is signed with Sigstore (keyless) and ships with SLSA provenance and an SBOM. Verification commands: https://github.com/kolapsis/maintenant/blob/main/SECURITY.md.

## Editions

- Community: free, Apache 2.0, one host, usable for anything.
- Personal: bought once, for your own infrastructure, up to 20 remote machines.
- Pro: unlimited hosts, escalation, maintenance windows, and the right to monitor other people's infrastructure.

Paid editions are the same image. A license key in `MAINTENANT_LICENSE_KEY` unlocks them. Monitoring data stays on your infrastructure. Details and pricing: https://maintenant.dev/pricing/.

## Other ways to run it

- Kubernetes manifests and Helm chart (`oci://ghcr.io/kolapsis/charts/maintenant`): https://docs.maintenant.dev/guides/kubernetes/
- Native Linux install (systemd, no Docker needed): `curl -fsSL https://install.maintenant.dev | sudo bash`

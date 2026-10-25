# Coolify Deployment Guide

How to run maintenant on a server managed by Coolify, as a Docker Compose resource: the dashboard behind the Coolify proxy with basic authentication, the Docker socket behind a read-only filter, and the heartbeat, status page and MCP routes left reachable.

!!! note "Tested on Coolify 4.5.0"
    This page was checked on Coolify 4.5.0 with its default Traefik proxy. Menu names change between Coolify releases; the Compose file and its labels are what matter.

---

## Why a Docker Compose resource

Coolify offers one-click services from its own catalogue. maintenant is not in it, so you add it yourself as a **Docker Compose** resource built from a pasted file (called **Docker Compose Empty** in older Coolify versions): Coolify deploys it, gives it a domain and a certificate through its proxy, and keeps it on a private network of its own.

maintenant is a single Go binary with the frontend embedded and SQLite as its default store. It uses about 100 MB of RAM and needs no database server, so the resource is one container plus a small filtering proxy for the Docker socket.

It discovers every container on the Docker host Coolify deploys to, including the other resources you run there, with health, restart loops, resources and logs. It only reads; it does not restart or change anything.

---

## Step 1: Create the resource

First generate the user and the bcrypt hash of the password for basic authentication. `htpasswd` comes from the `apache2-utils` package on Debian and Ubuntu:

```bash
htpasswd -nbB admin 'a-long-password'
```

Keep the output, `admin:$2y$05$...`. Store the plaintext password in a password manager; only the hash goes into Coolify.

Then create the resource:

1. In your Coolify project, choose **New resource**, then **Docker Compose** under the Docker sources ("without a Git repository").
2. If Coolify manages several servers, select the one that runs the workload you want to monitor.
3. Paste the file below, replace `<htpasswd output>` with the line you generated and `now.example.com` with your domain, then choose **Create Service**.

```yaml
services:
  socketproxy:
    image: tecnativa/docker-socket-proxy:v0.5.0
    environment:
      CONTAINERS: "1"
      IMAGES: "1"
      INFO: "1"
      NETWORKS: "1"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    read_only: true
    tmpfs:
      - /run
      - /tmp
    security_opt:
      - no-new-privileges:true

  maintenant:
    image: ghcr.io/kolapsis/maintenant:1.11.1
    depends_on:
      - socketproxy
    read_only: true
    security_opt:
      - no-new-privileges:true
    tmpfs:
      - /tmp:noexec,nosuid,size=64m
    volumes:
      - /proc:/host/proc:ro
      - /etc/os-release:/host/etc/os-release:ro
      - maintenant-data:/data
    environment:
      SERVICE_FQDN_MAINTENANT_8080:
      DOCKER_HOST: "tcp://socketproxy:2375"
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"
      MAINTENANT_BASE_URL: "https://${SERVICE_FQDN_MAINTENANT}"
      MAINTENANT_TRUSTED_PROXIES: "${MAINTENANT_TRUSTED_PROXIES:?}"
    labels:
      - 'traefik.http.middlewares.maintenant-auth.basicauth.users=<htpasswd output>'
      - 'coolify.traefik.middlewares=maintenant-auth'
      - 'traefik.http.routers.maintenant-public.rule=Host(`now.example.com`) && (Path(`/status`) || PathPrefix(`/status/`) || PathPrefix(`/ping/`) || PathPrefix(`/assets/`) || PathPrefix(`/screenshots/`) || PathPrefix(`/workbox-`) || PathPrefix(`/favicon`) || PathPrefix(`/icon`) || Path(`/manifest.webmanifest`) || Path(`/sw.js`) || Path(`/registerSW.js`) || Path(`/apple-touch-icon.png`) || Path(`/logo.svg`))'
      - 'traefik.http.routers.maintenant-public.entryPoints=https'
      - 'traefik.http.routers.maintenant-public.tls=true'
      - 'traefik.http.routers.maintenant-public.tls.certresolver=letsencrypt'
      - 'traefik.http.routers.maintenant-public.priority=1000'
      - 'traefik.http.routers.maintenant-public.service=maintenant-public'
      - 'traefik.http.services.maintenant-public.loadbalancer.server.port=8080'

volumes:
  maintenant-data:
```

The image is pinned to a release tag. Pin it, and change the tag when you upgrade: a floating `latest` makes the version running on your server depend on when Coolify last pulled.

What the file does:

- `socketproxy` is a [Tecnativa docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy). It answers `403` to every write to the Docker API, so a compromised maintenant could only read. It has no published port and is reachable only from the resource's own network.
- `maintenant` points `DOCKER_HOST` at the proxy and mounts no Docker socket. See [Security: Docker socket proxy](../security.md#recommended-docker-socket-proxy) for what each endpoint group is for. `IMAGES` is only needed for update checks.
- `read_only` and `no-new-privileges` are the hardening of the [Quick Start](../getting-started/installation.md#docker-compose-recommended). The process runs as uid 65534.
- `SERVICE_FQDN_MAINTENANT_8080` asks Coolify for a domain for the `maintenant` service, routed to container port `8080`. Coolify exposes the domain as `SERVICE_FQDN_MAINTENANT` (without the port), which `MAINTENANT_BASE_URL` uses.
- The first two labels define a basic authentication middleware and tell Coolify to add it to the router it generates for the domain. The other labels add a second router for the public routes, explained in [Step 3](#step-3-keep-the-public-routes-reachable).

Keep each `$` of the hash as it is. With **Escape special characters in labels** enabled, the default under **Edit Compose File**, Coolify writes the `$$` form Compose needs. The same escaping is why the hash and the domain are written out in the labels: a `${VARIABLE}` inside a label reaches Traefik as literal text, not as its value.

The Compose file has no `ports:` entry on purpose. Coolify's proxy reaches the container over the resource network, and a published port would bypass the proxy and its authentication.

---

## Step 2: Set the variable and the domain

Coolify marks `MAINTENANT_TRUSTED_PROXIES` as required and refuses to deploy until it has a value. Open **Environment Variables** on the resource and set it.

**`MAINTENANT_TRUSTED_PROXIES`** is the address range maintenant sees the proxy connect from. Without it, `X-Forwarded-For` is ignored and every visitor shares the proxy's rate limit, including every heartbeat ping. Coolify's proxy joins the resource network, so the right value is that network's subnet. The network only exists after the first deploy: start with `10.0.0.0/8`, the Docker address pool the Coolify installer configures. After the first deploy, run on the server

```bash
docker network inspect <resource-uuid> --format '{{(index .IPAM.Config 0).Subnet}}'
```

where `<resource-uuid>` is the last part of the resource's URL in Coolify and the suffix of its container names, then replace the value with the result, for example `10.0.3.0/24`, and redeploy. Do not keep `10.0.0.0/8`: a caller whose own address falls in the trusted range, a cron job on a private `10.x` network for example, is counted as the proxy. To check, ping a heartbeat and read `/api/v1/heartbeats/<id>/pings`: `source_ip` must be the caller's address, not the proxy's. See [Running behind a proxy](../security.md#running-behind-a-proxy).

Then open **Domains**, edit the `maintenant` domain and set the protocol to `https` and the host to the domain you wrote in the label, for example `now.example.com`. If Coolify warns that the service requires port `8080`, confirm with **Use This Port Anyway**: the **Internal port** column must still show `8080`. Deploy. Coolify's proxy obtains the certificate.

Open the domain: the browser asks for the user and password, then shows the dashboard with the containers of the host.

---

## Step 3: Keep the public routes reachable

Basic authentication on the whole domain also covers routes that must stay open. A cron job cannot answer a login prompt, and the status page is meant for people who have no account. The `maintenant-public` router from Step 1 sends these paths to maintenant without the middleware:

| Route | Needed by |
|-------|-----------|
| `/ping/{uuid}` | Heartbeat pings from cron jobs and pipelines |
| `/status`, `/status/` | The public status page, its API, live updates, feed and subscriptions |
| `/assets/` | The scripts and styles of the status page |
| `/registerSW.js`, `/sw.js`, `/workbox-*.js`, `/manifest.webmanifest` | Files the status page loads at startup. Behind authentication, the browser asks anonymous visitors for a password |
| `/favicon*`, `/icon*`, `/apple-touch-icon.png`, `/logo.svg`, `/screenshots/` | Icons, logo and manifest images |
| `/mcp`, `/oauth/`, `/.well-known/` | The MCP endpoint, only if you enable it (it has its own OAuth credentials) |

The full list and the reasoning are in [Security: Route Reference](../security.md#route-reference). If you enable MCP, add ``|| Path(`/mcp`) || PathPrefix(`/mcp/`) || PathPrefix(`/oauth/`) || PathPrefix(`/.well-known/`)`` inside the parentheses of the rule.

The router has a higher priority than the one Coolify generates, so it wins on these paths. Its entry point (`https`) and certificate resolver (`letsencrypt`) are the names of Coolify's default Traefik configuration. If you changed them in **Servers > your server > Proxy**, use your names. If you change the domain later, change it in the rule too.

Check from outside the server:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' https://now.example.com/api/v1/health
# 401: the dashboard API asks for credentials

curl -sS -o /dev/null -w '%{http_code}\n' https://now.example.com/status
# 200: the status page is reachable without credentials

curl -sS -o /dev/null -w '%{http_code}\n' https://now.example.com/registerSW.js
# 200: so are the files it loads

curl -sS -o /dev/null -w '%{http_code}\n' https://now.example.com/ping/00000000-0000-0000-0000-000000000000
# 404: maintenant answers (no such heartbeat), not the proxy's 401
```

!!! warning "Basic authentication is one shared credential"
    It protects the route; it does not give maintenant users, roles or an audit trail. If you already run an identity provider, Coolify documents putting Authentik in front of a resource with Traefik forward authentication. Use that instead of basic authentication, and keep the same public routes open.

!!! note "On Caddy instead of Traefik"
    If your Coolify server uses its Caddy proxy, the labels above do not apply. Coolify's Caddy documentation covers basic authentication for Compose resources with a `caddy.basicauth` label, and the public routes need the equivalent `handle` blocks. The behaviour is the same: dashboard behind authentication, the routes of the table above open.

---

## Step 4: Monitor your stack

With the dashboard up, the containers of the Docker host are already listed. Add what discovery cannot know.

**Endpoints.** Create HTTP checks from **Endpoints → New endpoint**, or label a container in its own Compose file:

```yaml
    labels:
      maintenant.endpoint.http: "https://app.example.com/health"
      maintenant.endpoint.interval: "30s"
```

With `MAINTENANT_PROXY_LABELS=true` on the maintenant service, containers that already carry Traefik labels get an endpoint from them. See [Endpoint Monitoring](../features/endpoints.md) and [Docker Labels](docker-labels.md).

**TLS certificates.** An HTTPS endpoint checked by the server gets its certificate monitored too. For other domains, use the `maintenant.tls.certificates` label or **Certificates** in the dashboard. See [TLS Certificate Monitoring](../features/certificates.md).

**Scheduled jobs.** Create a heartbeat in the dashboard, copy its ping URL, and call it at the end of the job:

```bash
0 2 * * * /usr/local/bin/backup.sh && curl -fsS -o /dev/null https://now.example.com/ping/<uuid>/$?
```

The ping URL is built from `MAINTENANT_BASE_URL`, which is why Step 1 sets it. Keep it on `https://`: on port 80, Coolify's Traefik answers every path under `/ping` itself with `200 OK` (its own health check), so a ping sent over plain HTTP never reaches maintenant. A heartbeat is not watched until its first ping, so run the job once after creating it. See [Heartbeat & Cron Monitoring](../features/heartbeats.md). Coolify's own scheduled tasks can run the same command.

---

## Persistence and upgrades

The `maintenant-data` volume holds the SQLite database. Coolify prefixes its name with the resource UUID (`<resource-uuid>_maintenant-data`) and keeps it across redeploys. Do not move it to a network filesystem: SQLite needs local file locking.

To upgrade, change the image tag under **Edit Compose File** and redeploy. Schema migrations rebuild tables in place and need free disk space of several times the database size.

For a copy of the database that is safe while maintenant runs, use `sqlite3 ... ".backup"` as described in [Hetzner: Backups and snapshots](hetzner.md#backups-and-snapshots). A server snapshot alone can copy the database mid-write.

---

## Related

- [Security](../security.md): Which routes to protect, which to leave open, reverse proxy setups
- [Installation](../getting-started/installation.md): Docker, Kubernetes and source builds
- [Hetzner Cloud Deployment](hetzner.md): A server with cloud-init, firewall and volume
- [Docker Labels Reference](docker-labels.md): Endpoint, certificate and proxy labels
- [Heartbeat & Cron Monitoring](../features/heartbeats.md): Ping URLs for scheduled jobs

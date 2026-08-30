# DigitalOcean Deployment Guide

How to run maintenant on DigitalOcean: a Droplet provisioned with cloud-init, a Cloud Firewall, a Volume for the database, agents over the VPC, and DOKS for the Kubernetes side.

---

## Why it fits a small Droplet

maintenant is a single Go binary with the frontend embedded and SQLite as its default store. It idles around 17 MB of RAM and needs no database server, no Prometheus, no Redis. A Basic Droplet running your workload has room for it.

| Slug | vCPU | RAM | Typical fit |
|------|------|-----|-------------|
| `s-1vcpu-2gb` | 1 | 2 GiB | One host, a handful of containers and endpoint checks. |
| `s-2vcpu-4gb` | 2 | 4 GiB | Central server for a fleet of agents, or a busy update/CVE scan schedule. |
| Larger | | | Only once you move the store to PostgreSQL and monitor hundreds of containers. |

!!! note "Size slugs are moving"
    DigitalOcean is rolling out a v5 generation (`s5-`, `g5-`) alongside the classic `s-` Basic
    slugs. Rather than trust a slug from a guide, list what your account can actually create:

    ```bash
    doctl compute size list
    ```

---

## Step 1 — Create the Droplet

The repository ships a ready-to-use cloud-config at [`deploy/cloud-init/maintenant.yaml`](https://github.com/kolapsis/maintenant/blob/main/deploy/cloud-init/maintenant.yaml). It installs Docker from the official repository, writes `/opt/maintenant/compose.yml`, and starts the stack on first boot.

```bash
doctl compute droplet create maintenant \
  --image ubuntu-24-04-x64 \
  --size s-2vcpu-4gb \
  --region fra1 \
  --ssh-keys <your-key-fingerprint> \
  --tag-names maintenant \
  --user-data-file deploy/cloud-init/maintenant.yaml \
  --wait
```

Then reach the dashboard, which the cloud-config binds to loopback:

```bash
ssh -L 8080:127.0.0.1:8080 root@$(doctl compute droplet get maintenant --format PublicIPv4 --no-header)
```

Open **http://localhost:8080**. Every container on the host is already discovered.

!!! warning "User data is set once, at creation"
    A Droplet's user data cannot be changed afterwards, and it is capped at 64 KiB. Get the file
    right before you create the Droplet; to change it later, edit
    `/opt/maintenant/compose.yml` on the host directly.

The `--tag-names maintenant` is not decoration: firewall rules below target that tag, so a new
Droplet joins the right rules by carrying the tag.

---

## Step 2 — Cloud Firewall

`droplet create` has no `--firewall` flag, so the firewall is created separately and attached by tag.

```bash
doctl compute firewall create \
  --name maintenant \
  --inbound-rules "protocol:tcp,ports:22,address:$(curl -s https://ipv4.icanhazip.com)/32 protocol:tcp,ports:80,address:0.0.0.0/0 protocol:tcp,ports:443,address:0.0.0.0/0" \
  --outbound-rules "protocol:tcp,ports:all,address:0.0.0.0/0 protocol:udp,ports:53,address:0.0.0.0/0" \
  --tag-names maintenant
```

Note what is **not** in that list: port `8080`. Publish the dashboard through a reverse proxy with authentication, never as a raw port on the public IPv4.

!!! warning "maintenant will flag its own exposure"
    If you change the Compose file to publish `"8080:8080"`, maintenant's own network security
    scanner reports a critical **Port exposed on all interfaces** finding for its own container.
    That is the intended behaviour, not a bug. See the note in
    [Installation → Docker Compose](../getting-started/installation.md#docker-compose-recommended)
    and [Configuration → Choosing a Bind Address](../getting-started/configuration.md#choosing-a-bind-address).

To add a rule later, use `add-rules`, never `update`:

```bash
doctl compute firewall add-rules <firewall-id> \
  --inbound-rules "protocol:tcp,ports:8443,tag:maintenant"
```

`firewall update` resets every attribute you do not pass, which silently drops the rules you
already have.

---

## Step 3 — Put the database on a Volume

The Droplet's disk goes away with the Droplet. A Volume survives a rebuild, resizes, and can be
snapshotted on its own. Volumes start at 1 GiB and cost $0.10 per GiB per month.

**Attach it at creation time.** DigitalOcean formats and mounts a Volume automatically only when
it is attached to a Droplet being created; a Volume attached to an existing Droplet has to be
partitioned and mounted by hand.

```bash
doctl compute volume create maintenant-data \
  --region fra1 --size 20GiB --fs-type ext4

doctl compute droplet create maintenant \
  --image ubuntu-24-04-x64 --size s-2vcpu-4gb --region fra1 \
  --ssh-keys <your-key-fingerprint> --tag-names maintenant \
  --volumes <volume-id> \
  --user-data-file deploy/cloud-init/maintenant.yaml --wait
```

The mount path is `/mnt/<volume-name>`, with hyphens turned into underscores to match systemd
mount unit naming. `maintenant-data` therefore lands on `/mnt/maintenant_data`. Point the Compose
volume at it:

```yaml
# /opt/maintenant/compose.yml
    volumes:
      - /mnt/maintenant_data/maintenant:/data
    environment:
      MAINTENANT_DB: "/data/maintenant.db"
```

!!! warning "Size the volume for migrations, not just for the data"
    Schema migrations rebuild tables in place and transiently need several times the size of the
    database. A volume sized to the current database will fail an upgrade. 10 GiB is a sane floor.

!!! note "Watching more than one host? Consider PostgreSQL"
    The server holds the agents' identity and enrolment, the one thing they cannot rebuild on
    their own. On the default SQLite file, losing that Droplet means re-enrolling every host by
    hand. Pointing the server at a PostgreSQL you already run makes the instance replaceable. See
    [PostgreSQL storage](postgresql.md).

---

## Monitoring several Droplets over the VPC

Every Droplet lands in its region's default VPC, a `/20` range such as `10.102.0.0/20`, and VPC
traffic does not count against bandwidth. That is where agent traffic belongs.

A Droplet reads its own private address from the metadata service:

```bash
curl -s http://169.254.169.254/metadata/v1/interfaces/private/0/ipv4/address
```

Bind the gRPC listener to that address on the server:

```bash
MAINTENANT_GRPC_LISTEN=10.102.0.2:8443
MAINTENANT_GRPC_URL=grpcs://maintenant.internal.example.com:8443
```

Then enrol each other Droplet. Generate a token from **Agents → Add host**: the modal hands you a
ready-made command. Run it on the host, pointing at the private address:

```bash
docker run -d \
  --name maintenant-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /proc:/host/proc:ro \
  -v maintenant-agent-data:/var/lib/maintenant \
  ghcr.io/kolapsis/maintenant:latest \
  --mode=agent \
  --server=grpcs://maintenant.internal.example.com:8443 \
  --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX
```

Allow the gRPC port between tagged Droplets rather than by IP, so a replaced Droplet needs no rule change:

```bash
doctl compute firewall add-rules <firewall-id> \
  --inbound-rules "protocol:tcp,ports:8443,tag:maintenant"
```

!!! important "Private does not mean plaintext"
    Binding on the VPC address keeps the listener off the public internet, but the agent still
    speaks TLS. Use a certificate that covers the internal hostname; a DNS-01 ACME certificate
    works fine for a name that only resolves privately. Do **not** reach for
    `--grpc-insecure-skip-tls-verify` outside a lab. The full matrix of TLS modes is in
    [Agent Setup → Step 1](agent-setup.md#step-1-make-the-grpc-endpoint-reachable).

!!! warning "A VPC does not span regions"
    A VPC network lives in one region. Agents in another region need VPC peering, or they reach
    the server over its public address like any other remote host.

---

## Load Balancer

A Load Balancer hides target failures by design: after 5 failed checks it pulls a Droplet out of
the pool and the public endpoint keeps answering `200`. Checking only the Load Balancer tells you
nothing until every target is down.

Configure two layers of endpoint checks:

| Check | Target | What it tells you |
|-------|--------|-------------------|
| Public | `https://app.example.com` | The service is reachable for users. |
| Per-target | `http://10.102.0.3:8080/health` on each Droplet | Which backend is down behind a still-green Load Balancer. |

The per-target checks run over the VPC from the maintenant Droplet, so they need no public
exposure. Add them from **Endpoints → New endpoint**; see [Endpoint Monitoring](../features/endpoints.md).

The Load Balancer's own defaults are worth knowing when you compare its verdict to maintenant's:
an HTTP check on `/` every 10 seconds, 5 failures to remove a target, 3 successes to bring it
back. Its IP comes from:

```bash
doctl compute load-balancer get <lb-id> --format IP --no-header
```

!!! note "gRPC backends need a TCP check"
    Load Balancer HTTP health checks speak HTTP/1.1, so a gRPC service must be checked over TCP.
    This applies to a Load Balancer fronting maintenant's own gRPC listener.

---

## Kubernetes on DOKS

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace \
  --set persistence.storageClass=do-block-storage-retain \
  --set persistence.size=10Gi
```

The DigitalOcean CSI driver installs four storage classes. `do-block-storage` is the cluster
default and deletes the volume with the PVC; **`do-block-storage-retain` keeps it**, which is what
you want under a database you would rather not lose to a `helm uninstall`. XFS variants
(`do-block-storage-xfs`, `do-block-storage-xfs-retain`) exist if you prefer XFS.

Two DOKS specifics:

- volumes are `ReadWriteOnce`, which suits maintenant: SQLite takes a single writer and the
  deployment never scales past one replica;
- a PVC with no size request gets 16 GiB, and anything under 1 GiB is raised to 1 GiB.

Full RBAC, namespace filtering and Helm values are in the [Kubernetes Guide](kubernetes.md).

---

## Backups and snapshots

A Droplet snapshot copies the disk while the database is being written to. For SQLite in WAL mode
that is a torn copy, not a backup. DigitalOcean says as much: powering the Droplet off first is
recommended precisely because databases do not guarantee on-disk consistency otherwise.

```bash
# On the Droplet: consistent copy while maintenant keeps running
docker compose -f /opt/maintenant/compose.yml exec maintenant \
  sqlite3 /data/maintenant.db ".backup '/data/maintenant.backup.db'"

# Then snapshot the Droplet, or just the volume
doctl compute droplet-action snapshot <droplet-id> \
  --snapshot-name "maintenant-$(date -u +%F)" --wait
doctl compute volume snapshot <volume-id> \
  --snapshot-name "maintenant-data-$(date -u +%F)"
```

If the image has no `sqlite3` binary, stop the stack for the few seconds the copy takes:

```bash
docker compose -f /opt/maintenant/compose.yml stop
cp /mnt/maintenant_data/maintenant/maintenant.db /root/maintenant-$(date -u +%F).db
docker compose -f /opt/maintenant/compose.yml start
```

!!! tip "Where to put the copy"
    Spaces (S3-compatible object storage) is the cheapest off-Droplet destination and reachable
    from the same region without leaving DigitalOcean's network.

---

## Related

- [Installation](../getting-started/installation.md) — Docker, Kubernetes and source builds
- [Hetzner Cloud Deployment](hetzner.md) — The same ground on Hetzner
- [Scaleway Deployment](scaleway.md) — The same ground on Scaleway
- [OVHcloud Deployment](ovhcloud.md) — The same ground on OVHcloud
- [Vultr Deployment](vultr.md) — The same ground on Vultr
- [Agent Setup](agent-setup.md) — Enrolling additional hosts over gRPC
- [Kubernetes Guide](kubernetes.md) — RBAC, Helm values, workload monitoring
- [PostgreSQL Storage](postgresql.md) — Making the server replaceable
- [Endpoint Monitoring](../features/endpoints.md) — HTTP/TCP checks behind a Load Balancer

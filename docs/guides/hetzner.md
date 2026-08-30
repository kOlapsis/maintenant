# Hetzner Cloud Deployment Guide

How to run maintenant on Hetzner Cloud — a single server with `cloud-init`, a private network for the agents, Hetzner Volumes for the database, and Kubernetes clusters built with the usual Hetzner tooling.

---

## Why it fits a Hetzner box

maintenant is a single Go binary with the frontend embedded and SQLite as its default store. It idles around 17 MB of RAM and needs no database server, no Prometheus, no Redis. That makes it a natural fit for a small Hetzner Cloud server that already runs something else — it does not need a machine of its own.

The image is published for `linux/amd64` **and** `linux/arm64`, so the Ampere-based **CAX** line works as well as the Intel/AMD **CX** and **CPX** lines.

| Server type | Arch | Typical fit |
|-------------|------|-------------|
| **CAX11** / **CX22** | arm64 / x86 | One host, up to ~50 containers and a few dozen endpoint checks. Comfortable alongside the monitored workload. |
| **CPX21** / **CAX21** | x86 / arm64 | Central server for a fleet of agents, or a busy update/CVE scan schedule. |
| **CPX31** and up | x86 | Only needed if you move the store to PostgreSQL and monitor hundreds of containers. |

!!! note "The bottleneck is I/O, not CPU"
    Resource sampling and endpoint checks are cheap. What grows is the SQLite database and the
    write rate on it. On a shared-vCPU server, keep the database on a
    [Hetzner Volume](#step-3-put-the-database-on-a-hetzner-volume) rather than on the boot disk if
    you retain long history.

---

## Step 1 — Provision the server

The repository ships a ready-to-use cloud-config at [`deploy/cloud-init/maintenant.yaml`](https://github.com/kolapsis/maintenant/blob/main/deploy/cloud-init/maintenant.yaml). It installs Docker from the official repository, writes `/opt/maintenant/compose.yml`, and starts the stack on first boot.

```bash
hcloud server create \
  --name maintenant \
  --type cx22 \
  --image ubuntu-24.04 \
  --location nbg1 \
  --ssh-key my-key \
  --user-data-from-file deploy/cloud-init/maintenant.yaml
```

Once the server is up:

```bash
# The UI listens on loopback only — reach it through an SSH tunnel
ssh -L 8080:127.0.0.1:8080 root@$(hcloud server ip maintenant)
```

Open **http://localhost:8080**. Every container on the host is already discovered.

!!! tip "Hetzner's Docker app image"
    Hetzner also publishes a `docker-ce` app image. It works, but the cloud-config installs Docker
    itself so the same file stays valid on a plain `ubuntu-24.04` image and on a rescued or rebuilt
    server. Pick one, not both.

---

## Step 2 — Lock the Hetzner Cloud Firewall

The cloud-config publishes the UI on `127.0.0.1:8080` only, so nothing is exposed before you decide it should be. A Cloud Firewall makes that explicit at the network edge:

```bash
hcloud firewall create --name maintenant

# SSH from your address only
hcloud firewall add-rule maintenant \
  --direction in --protocol tcp --port 22 \
  --source-ips "$(curl -s https://ipv4.icanhazip.com)/32" \
  --description "ssh"

# HTTP/HTTPS for the reverse proxy that fronts maintenant
hcloud firewall add-rule maintenant \
  --direction in --protocol tcp --port 80 \
  --source-ips 0.0.0.0/0 --source-ips ::/0
hcloud firewall add-rule maintenant \
  --direction in --protocol tcp --port 443 \
  --source-ips 0.0.0.0/0 --source-ips ::/0

hcloud firewall apply-to-resource maintenant --type server --server maintenant
```

Note what is **not** in that list: port `8080`. Publish the dashboard through a reverse proxy with authentication, never as a raw port on the public IPv4.

!!! warning "maintenant will flag its own exposure"
    If you change the Compose file to publish `"8080:8080"`, maintenant's own network security
    scanner reports a critical **Port exposed on all interfaces** finding for its own container.
    That is the intended behaviour, not a bug — see the note in
    [Installation → Docker Compose](../getting-started/installation.md#docker-compose-recommended)
    and [Configuration → Choosing a Bind Address](../getting-started/configuration.md#choosing-a-bind-address).

---

## Step 3 — Put the database on a Hetzner Volume

The boot disk of a CX22 is 40 GB and disappears with the server. A Volume survives a rebuild and can be resized, which matters once you keep months of history.

```bash
hcloud volume create \
  --name maintenant-data \
  --size 10 \
  --server maintenant \
  --format ext4 \
  --automount
```

Hetzner mounts it at `/mnt/HC_Volume_<volume-id>`. Point the Compose volume at that path:

```yaml
# /opt/maintenant/compose.yml
    volumes:
      - /mnt/HC_Volume_123456789/maintenant:/data
    environment:
      MAINTENANT_DB: "/data/maintenant.db"
```

!!! warning "Size the volume for migrations, not just for the data"
    Schema migrations rebuild tables in place and transiently need several times the size of the
    database. A volume sized to the current database will fail an upgrade. 10 GB is a sane floor.

!!! note "Watching more than one host? Consider PostgreSQL"
    The server holds the agents' identity and enrolment — the one thing they cannot rebuild on
    their own. On the default SQLite file, losing that server means re-enrolling every host by
    hand. Hetzner has no managed PostgreSQL, but if you already run one, pointing the server at it
    makes the instance replaceable. See [PostgreSQL storage](postgresql.md).

---

## Monitoring several Hetzner servers over the private network

A Hetzner Cloud Network is the right place for agent traffic: it never leaves Hetzner's network and costs nothing in egress.

```bash
hcloud network create --name backend --ip-range 10.0.0.0/16
hcloud network add-subnet backend \
  --network-zone eu-central --type cloud --ip-range 10.0.0.0/24

hcloud server attach-to-network maintenant --network backend --ip 10.0.0.2
hcloud server attach-to-network web-01     --network backend --ip 10.0.0.3
```

On the server, bind the gRPC listener to the private address only:

```bash
MAINTENANT_GRPC_LISTEN=10.0.0.2:8443
MAINTENANT_GRPC_URL=grpcs://maintenant.internal.example.com:8443
```

Then enrol each other server. Generate a token from **Agents → Add host**: the modal hands you a ready-made command, with the server address already filled in. Run it on the host, over the private address:

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

The token is consumed on first enrolment and cannot be retrieved again. The full walkthrough, including the Compose and Kubernetes variants, is in [Agent Setup](agent-setup.md).

!!! important "Private does not mean plaintext"
    Binding on `10.0.0.2` keeps the listener off the public internet, but the agent still speaks
    TLS. Use a certificate that covers the internal hostname — a DNS-01 ACME certificate works
    fine for a name that only resolves privately. Do **not** reach for
    `--grpc-insecure-skip-tls-verify` outside a lab. The full matrix of TLS modes is in
    [Agent Setup → Step 1](agent-setup.md#step-1-make-the-grpc-endpoint-reachable).

Also add a Cloud Firewall rule allowing TCP `8443` **from `10.0.0.0/16` only** — Cloud Firewalls filter the public interface, so this is about not opening it there by accident.

---

## Hetzner Load Balancer

A Load Balancer hides target failures by design: it removes an unhealthy target and the public endpoint keeps answering `200`. Checking only the LB tells you nothing until every target is down.

Configure two layers of endpoint checks:

| Check | Target | What it tells you |
|-------|--------|-------------------|
| Public | `https://app.example.com` (LB public IP / DNS) | The service is reachable for users. |
| Per-target | `http://10.0.0.3:8080/health`, `http://10.0.0.4:8080/health` | Which backend is actually down behind a still-green LB. |

The per-target checks run over the private network from the maintenant server, so they need no public exposure. Add them from **Endpoints → New endpoint**; see [Endpoint Monitoring](../features/endpoints.md).

!!! tip "Certificates on managed Load Balancers"
    If the Load Balancer terminates TLS with a Hetzner-managed certificate, add the public
    hostname to [TLS Certificate Monitoring](../features/certificates.md). Managed renewal usually
    works — the point is to be told when it does not.

---

## Kubernetes on Hetzner

maintenant runs unchanged on the clusters produced by the usual Hetzner tooling — [`terraform-hcloud-kube-hetzner`](https://github.com/kube-hetzner/terraform-hcloud-kube-hetzner), [`vitobotta/hetzner-k3s`](https://github.com/vitobotta/hetzner-k3s), [`terraform-hcloud-talos`](https://github.com/hcloud-talos/terraform-hcloud-talos) or a Cluster API deployment.

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace \
  --set persistence.storageClass=hcloud-volumes \
  --set persistence.size=10Gi
```

`hcloud-volumes` is the storage class created by the [Hetzner CSI driver](https://github.com/hetznercloud/csi-driver), which all of the above install by default. Without it, the PVC stays `Pending`.

!!! note "Talos and other socket-less nodes"
    Talos exposes no Docker socket. maintenant detects the in-cluster API on its own; if detection
    needs a nudge, set `runtime: kubernetes` explicitly. Monitoring then happens at the
    workload/pod level, which is what you want on Talos anyway.

Full RBAC, namespace filtering and Helm values are in the [Kubernetes Guide](kubernetes.md).

---

## Backups and snapshots

A Hetzner snapshot copies the disk while the database is being written to. For SQLite in WAL mode that is a torn copy, not a backup.

```bash
# On the server — consistent copy while maintenant keeps running
docker compose -f /opt/maintenant/compose.yml exec maintenant \
  sqlite3 /data/maintenant.db ".backup '/data/maintenant.backup.db'"

# Then snapshot, or ship the file to a Storage Box
hcloud server create-image --type snapshot \
  --description "maintenant $(date -u +%F)" maintenant
```

If the container image has no `sqlite3` binary, stop the stack for the few seconds the copy takes:

```bash
docker compose -f /opt/maintenant/compose.yml stop
cp /mnt/HC_Volume_123456789/maintenant/maintenant.db /root/maintenant-$(date -u +%F).db
docker compose -f /opt/maintenant/compose.yml start
```

!!! tip "Where to put the copy"
    A Hetzner Storage Box over SFTP or BorgBackup is the cheapest off-server destination, and it
    is reachable from the private network on Cloud servers in the same region.

---

## Related

- [Installation](../getting-started/installation.md) — Docker, Kubernetes and source builds
- [DigitalOcean Deployment](digitalocean.md) — The same ground on DigitalOcean
- [Agent Setup](agent-setup.md) — Enrolling additional hosts over gRPC
- [Kubernetes Guide](kubernetes.md) — RBAC, Helm values, workload monitoring
- [PostgreSQL Storage](postgresql.md) — Making the server replaceable
- [Endpoint Monitoring](../features/endpoints.md) — HTTP/TCP checks behind a Load Balancer

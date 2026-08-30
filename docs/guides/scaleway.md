# Scaleway Deployment Guide

How to run maintenant on Scaleway: an Instance provisioned with cloud-init, a security group, a Block Storage volume for the database, agents over a Private Network with internal DNS, and Kapsule for the Kubernetes side.

---

## Why it fits a small Instance

maintenant is a single Go binary with the frontend embedded and SQLite as its default store. It idles around 17 MB of RAM and needs no database server, no Prometheus, no Redis. An Instance already running your workload has room for it.

| Type | vCPU / RAM | Arch | Typical fit |
|------|-----------|------|-------------|
| `DEV1-S` | 2 / 2 GiB | x86 | One host, a handful of containers and endpoint checks. |
| `BASIC2-A2C-4G` | 2 / 4 GiB | arm64 | Same, on Ampere. Cheaper per hour than its x86 equivalent. |
| `PLAY2-NANO` | 2 / 4 GiB | x86 | Central server for a fleet of agents, or a busy update/CVE scan schedule. |
| `PRO2-XXS` and up | 2 / 8 GiB | x86 | Only once you move the store to PostgreSQL and monitor hundreds of containers. |

The image is published for `linux/amd64` **and** `linux/arm64`, and the `ubuntu_noble` label resolves to the architecture of the Instance type you pick, so the ARM types need no change to any command below. Note that ARM types are Block-only and live in fewer zones than the x86 ones.

!!! note "Check what your account can actually create"
    Instance types come and go. List them rather than trusting a type name from a guide:

    ```bash
    scw instance server-type list zone=fr-par-1
    ```

---

## Step 1 — Create the Instance

The repository ships a ready-to-use cloud-config at [`deploy/cloud-init/maintenant.yaml`](https://github.com/kolapsis/maintenant/blob/main/deploy/cloud-init/maintenant.yaml). It installs Docker from the official repository, writes `/opt/maintenant/compose.yml`, and starts the stack on first boot.

```bash
# The SSH key belongs to the Project, it is not an argument of server create
scw iam ssh-key create name=deploy public-key="$(cat ~/.ssh/id_ed25519.pub)"

scw instance server create \
  zone=fr-par-1 \
  type=PLAY2-NANO \
  image=ubuntu_noble \
  name=maintenant \
  ip=new \
  cloud-init=@deploy/cloud-init/maintenant.yaml
```

Then reach the dashboard, which the cloud-config binds to loopback:

```bash
ssh -L 8080:127.0.0.1:8080 root@<instance-public-ip>
```

Open **http://localhost:8080**. Every container on the host is already discovered.

!!! note "Changing the cloud-config later"
    `cloud-init=@file` is a CLI convenience: it stores the file as a user-data key named
    `cloud-init`. You can replace it afterwards and reboot:

    ```bash
    scw instance server update <server-id> cloud-init=@deploy/cloud-init/maintenant.yaml
    ```

    For a change that does not need a reboot, edit `/opt/maintenant/compose.yml` on the host.

---

## Step 2 — Security group

```bash
SG=$(scw instance security-group create \
  name=maintenant \
  inbound-default-policy=drop \
  outbound-default-policy=accept \
  zone=fr-par-1 -o json | jq -r '.id')

# SSH from your address only
scw instance security-group create-rule security-group-id=$SG \
  protocol=TCP direction=inbound action=accept \
  ip-range="$(curl -s https://ipv4.icanhazip.com)/32" dest-port-from=22

# HTTP/HTTPS for the reverse proxy that fronts maintenant
scw instance security-group create-rule security-group-id=$SG \
  protocol=TCP direction=inbound action=accept ip-range=0.0.0.0/0 dest-port-from=80
scw instance security-group create-rule security-group-id=$SG \
  protocol=TCP direction=inbound action=accept ip-range=0.0.0.0/0 dest-port-from=443

scw instance server update <server-id> security-group-id=$SG
```

Note what is **not** in that list: port `8080`. Publish the dashboard through a reverse proxy with authentication, never as a raw port on the public IP.

!!! warning "maintenant will flag its own exposure"
    If you change the Compose file to publish `"8080:8080"`, maintenant's own network security
    scanner reports a critical **Port exposed on all interfaces** finding for its own container.
    That is the intended behaviour, not a bug. See the note in
    [Installation → Docker Compose](../getting-started/installation.md#docker-compose-recommended)
    and [Configuration → Choosing a Bind Address](../getting-started/configuration.md#choosing-a-bind-address).

!!! important "Security groups only filter public traffic"
    A security group has no effect on Private Network traffic. What filters that is the VPC's
    network ACLs, which is where the agent gRPC port belongs.

---

## Step 3 — Put the database on a Block Storage volume

The Instance's local storage goes away with the Instance. A Block Storage volume survives, resizes, and can be snapshotted on its own. Volumes start at 5 GB and come in two flavours: `sbs_5k` (5 000 IOPS) and `sbs_15k`.

```bash
VOL=$(scw block volume create \
  name=maintenant-data \
  perf-iops=5000 \
  from-empty.size=20GB \
  zone=fr-par-1 -o json | jq -r '.id')

scw instance server attach-volume \
  server-id=<server-id> volume-id=$VOL volume-type=sbs_volume
```

!!! warning "The size unit is mandatory"
    `from-empty.size=20` is rejected. Write `20GB` or `20G`. And note it is `from-empty.size`,
    not `size`: volumes moved to the Block Storage API, and the Instances API no longer manages
    them.

**Scaleway does not format or mount the volume for you.** Do it once, then make it permanent:

```bash
lsblk                                    # the new volume shows up as /dev/sdb
mkfs.ext4 /dev/sdb
mkdir -p /mnt/maintenant-data
blkid /dev/sdb                           # note the UUID
echo "UUID=<uuid> /mnt/maintenant-data ext4 defaults,nofail 0 2" >> /etc/fstab
mount -a
```

Then point the Compose volume at it:

```yaml
# /opt/maintenant/compose.yml
    volumes:
      - /mnt/maintenant-data/maintenant:/data
    environment:
      MAINTENANT_DB: "/data/maintenant.db"
```

!!! tip "Mount by UUID, not by device name"
    `/dev/sdb` is the order the kernel happened to enumerate the disks in. Attach a second volume
    and the names can swap, which is how a database ends up pointed at the wrong filesystem. The
    `UUID=` line above is what the Scaleway documentation recommends. Scaleway's own CSI driver
    uses a stable per-volume path, `/dev/disk/by-id/scsi-0SCW_sbs_volume-<volume-uuid>`, if you
    prefer scripting the mount from the volume id.

!!! warning "Size the volume for migrations, not just for the data"
    Schema migrations rebuild tables in place and transiently need several times the size of the
    database. A volume sized to the current database will fail an upgrade. 10 GB is a sane floor.

!!! note "Watching more than one host? Consider PostgreSQL"
    The server holds the agents' identity and enrolment, the one thing they cannot rebuild on
    their own. On the default SQLite file, losing that Instance means re-enrolling every host by
    hand. Pointing the server at a PostgreSQL you already run makes the instance replaceable. See
    [PostgreSQL storage](postgresql.md).

---

## Monitoring several Instances over a Private Network

A Private Network is where agent traffic belongs: it never leaves Scaleway's network, and Private Network traffic is free.

```bash
# The Private Network is regional, the NIC that attaches an Instance to it is zonal
PN=$(scw vpc private-network create name=maintenant region=fr-par -o json | jq -r '.id')

scw instance private-nic create server-id=<server-id> private-network-id=$PN zone=fr-par-1
scw instance private-nic create server-id=<other-server-id> private-network-id=$PN zone=fr-par-1
```

Each attached Instance gets a private IP automatically from a `/22` range, and keeps it across reboots.

**And this is where Scaleway saves you a certificate problem.** A Private Network comes with internal DNS: every attached resource resolves as `<hostname>.<private-network-name>.internal`. The Instance named `maintenant` on the network named `maintenant` is reachable at `maintenant.maintenant.internal` from any other Instance on that network, with no configuration.

So the server binds gRPC to its private address and announces the internal name:

```bash
MAINTENANT_GRPC_LISTEN=<private-ip>:8443
MAINTENANT_GRPC_URL=grpcs://maintenant.maintenant.internal:8443
```

Then enrol each other Instance. Generate a token from **Agents → Add host**: the modal hands you a ready-made command. Run it on the host, pointing at the internal name:

```bash
docker run -d \
  --name maintenant-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /proc:/host/proc:ro \
  -v maintenant-agent-data:/var/lib/maintenant \
  ghcr.io/kolapsis/maintenant:latest \
  --mode=agent \
  --server=grpcs://maintenant.maintenant.internal:8443 \
  --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX
```

!!! important "Private does not mean plaintext"
    The internal name keeps the listener off the public internet, but the agent still speaks TLS,
    and a certificate must cover `maintenant.maintenant.internal`. A DNS-01 ACME certificate
    works for a name that only resolves privately. Do **not** reach for
    `--grpc-insecure-skip-tls-verify` outside a lab. The full matrix of TLS modes is in
    [Agent Setup → Step 1](agent-setup.md#step-1-make-the-grpc-endpoint-reachable).

Three documented traps with internal DNS, all of which look like an agent bug:

- renaming a resource does not update its DNS record until you detach and reattach its NIC;
- a dot inside a resource name breaks resolution entirely;
- resolution does not cross VPCs, so agents in another VPC need peering or the public address.

---

## Load Balancer

A Load Balancer hides target failures by design: after 3 failed health checks it marks a backend `Down` and stops forwarding to it, while the public endpoint keeps answering `200`. Checking only the Load Balancer tells you nothing until every backend is down.

Configure two layers of endpoint checks:

| Check | Target | What it tells you |
|-------|--------|-------------------|
| Public | `https://app.example.com` | The service is reachable for users. |
| Per-backend | `http://<private-ip>:8080/health` on each Instance | Which backend is down behind a still-green Load Balancer. |

The per-backend checks run over the Private Network from the maintenant Instance, so they need no public exposure. Add them from **Endpoints → New endpoint**; see [Endpoint Monitoring](../features/endpoints.md).

Its public address comes from:

```bash
scw lb lb get <lb-id> zone=fr-par-1 -o json | jq -r '.ip[0].ip_address'
```

Defaults worth knowing when you compare its verdict to maintenant's: a check every 3 seconds with a 1 second timeout, 3 consecutive failures to mark a backend down, and more than two consecutive successes to bring it back.

!!! warning "gRPC needs a TCP frontend, not an HTTP one"
    The Load Balancer speaks `tcp` and `http` only, and HTTP/2 is supported **only with TLS**, at
    both ends. Cleartext gRPC (h2c) will not pass through an HTTP frontend. Put maintenant's gRPC
    listener behind a `tcp` frontend, where the health check is TCP by default, or terminate TLS
    end to end.

---

## Kubernetes on Kapsule

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace \
  --set persistence.storageClass=sbs-default \
  --set persistence.size=10Gi
```

`sbs-default` is the class the Scaleway CSI driver uses when a PVC names none; `sbs-5k` and `sbs-15k` pin the IOPS tier explicitly.

!!! warning "`scw-bssd` is gone"
    The old `scw-bssd` class belonged to CSI v0.2, whose support ended in February 2025. A chart
    or a snippet still naming it will leave the PVC `Pending`.

Two Kapsule specifics:

- volumes are **`ReadWriteOnce` only** (the driver advertises no multi-node access mode), which
  suits maintenant: SQLite takes a single writer and the deployment never scales past one replica.
  A `ReadWriteMany` claim needs File Storage, which is not what you want under a database;
- none of the provided classes uses `reclaimPolicy: Retain`, so deleting the PVC deletes the
  volume. If you want the database to outlive a `helm uninstall`, declare your own class:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: sbs-retain
provisioner: csi.scaleway.com
reclaimPolicy: Retain
allowVolumeExpansion: true
```

Size the claim at 5 Gi or more: that is the floor the Block Storage API enforces for the `sbs_5k` and `sbs_15k` tiers.

Full RBAC, namespace filtering and Helm values are in the [Kubernetes Guide](kubernetes.md).

---

## Backups and snapshots

A snapshot copies the volume while the database is being written to. For SQLite in WAL mode that is a torn copy, not a backup. Take an application-level copy first, then snapshot.

```bash
# On the Instance: consistent copy while maintenant keeps running
docker compose -f /opt/maintenant/compose.yml exec maintenant \
  sqlite3 /data/maintenant.db ".backup '/data/maintenant.backup.db'"

# Then snapshot the volume, or image the whole Instance
scw block snapshot create volume-id=<volume-id> \
  name="maintenant-data-$(date -u +%F)" zone=fr-par-1 -w
scw instance server backup <server-id> name="maintenant-$(date -u +%F)" zone=fr-par-1
```

If the image has no `sqlite3` binary, stop the stack for the few seconds the copy takes:

```bash
docker compose -f /opt/maintenant/compose.yml stop
cp /mnt/maintenant-data/maintenant/maintenant.db /root/maintenant-$(date -u +%F).db
docker compose -f /opt/maintenant/compose.yml start
```

!!! note "`scw instance snapshot create` no longer works"
    Snapshots moved to the Block Storage API. The old Instances command is deprecated and does
    nothing, though it still appears in some CLI help output. Use `scw block snapshot create`.

!!! tip "Where to put the copy"
    Object Storage in the same region is the cheapest off-Instance destination. A volume snapshot
    can also be exported there as QCOW2 with
    `scw block snapshot export-to-object-storage`.

---

## Related

- [Installation](../getting-started/installation.md) — Docker, Kubernetes and source builds
- [Hetzner Cloud Deployment](hetzner.md) — The same ground on Hetzner
- [DigitalOcean Deployment](digitalocean.md) — The same ground on DigitalOcean
- [OVHcloud Deployment](ovhcloud.md) — The same ground on OVHcloud
- [Vultr Deployment](vultr.md) — The same ground on Vultr
- [Agent Setup](agent-setup.md) — Enrolling additional hosts over gRPC
- [Kubernetes Guide](kubernetes.md) — RBAC, Helm values, workload monitoring
- [PostgreSQL Storage](postgresql.md) — Making the server replaceable
- [Endpoint Monitoring](../features/endpoints.md) — HTTP/TCP checks behind a Load Balancer

# OVHcloud Deployment Guide

How to run maintenant on OVHcloud Public Cloud: an instance provisioned with cloud-init, security groups, a Block Storage volume for the database, agents over a private network, and Managed Kubernetes.

---

## Public Cloud, not VPS

OVHcloud sells two things that look interchangeable and are not. This guide targets **Public Cloud**, the OpenStack-based offer, for one blunt reason: **a VPS cannot run a cloud-init file**. The rebuild API accepts a `postInstallScript` (a bash script run once on first boot) and nothing else; there is no user-data field, and cloud-init support on VPS is still an open request on OVHcloud's roadmap.

A VPS also has none of what the rest of this guide relies on:

| | VPS | Public Cloud |
|---|---|---|
| cloud-init | no, bash post-install script only | yes |
| Private network (vRack) | not supported | yes |
| Block Storage volumes | a fixed "additional disk" option, not a Cinder volume | yes, attach and detach live |
| Load balancer | no Octavia, only the separate IP Load Balancing product | yes |
| Managed Kubernetes | no | yes |

A VPS is still a fine home for a single maintenant server if you never plan to enrol remote agents: install Docker over SSH and follow the [installation guide](../getting-started/installation.md) directly. Everything below assumes Public Cloud.

!!! warning "No ARM instances at OVHcloud"
    maintenant publishes `linux/arm64` images, and the Hetzner and Scaleway guides use them to cut
    costs. OVHcloud Public Cloud has no ARM instances: the request has been open on their roadmap
    since January 2023. Every flavor below is x86.

---

## Why it fits a small instance

maintenant is a single Go binary with the frontend embedded and SQLite as its default store. It idles around 17 MB of RAM and needs no database server, no Prometheus, no Redis.

`d2-4` (Discovery) is the natural starting point for a monitoring server; `s1-4` is cheaper still if
your project has access to it. Move up the `b3-` or `c3-` ranges only once you switch the store to
PostgreSQL and monitor hundreds of containers.

!!! note "Check flavors and images against your project"
    Flavor names and their exact vCPU/RAM split are not published in a table you can rely on, and
    the Ubuntu image name appears in two forms across OVHcloud's own documentation. List them:

    ```bash
    openstack flavor list
    openstack image list | grep -i ubuntu
    ```

---

## Step 1 — Create the instance

Public Cloud is driven with the standard OpenStack client. Download the RC file from the OVHcloud
Control Panel under **Users & Roles**, then source it. One RC file covers one user and one region.

```bash
source openrc.sh          # prompts for the Horizon password
openstack server list     # confirms the session works
```

The repository ships a ready-to-use cloud-config at [`deploy/cloud-init/maintenant.yaml`](https://github.com/kolapsis/maintenant/blob/main/deploy/cloud-init/maintenant.yaml). It installs Docker from the official repository, writes `/opt/maintenant/compose.yml`, and starts the stack on first boot.

```bash
openstack server create \
  --flavor d2-4 \
  --image "<the Ubuntu 24.04 image name from openstack image list>" \
  --key-name my-key \
  --user-data deploy/cloud-init/maintenant.yaml \
  maintenant
```

Then reach the dashboard, which the cloud-config binds to loopback:

```bash
ssh -L 8080:127.0.0.1:8080 ubuntu@<instance-public-ip>
```

Open **http://localhost:8080**. Every container on the host is already discovered.

---

## Step 2 — Security groups

OpenStack blocks inbound traffic by default: the stock rules allow outgoing traffic only. Create a
group, then apply it to the instance's port.

```bash
openstack security group create maintenant

openstack security group rule create --protocol tcp --dst-port 22 \
  --remote-ip "$(curl -s https://ipv4.icanhazip.com)/32" maintenant
openstack security group rule create --protocol tcp --dst-port 80 maintenant
openstack security group rule create --protocol tcp --dst-port 443 maintenant

openstack port list --server <server-id>
openstack port set --security-group maintenant <port-id>
```

Note what is **not** in that list: port `8080`. Publish the dashboard through a reverse proxy with authentication, never as a raw port on the public IP.

!!! warning "maintenant will flag its own exposure"
    If you change the Compose file to publish `"8080:8080"`, maintenant's own network security
    scanner reports a critical **Port exposed on all interfaces** finding for its own container.
    That is the intended behaviour, not a bug. See the note in
    [Installation → Docker Compose](../getting-started/installation.md#docker-compose-recommended)
    and [Configuration → Choosing a Bind Address](../getting-started/configuration.md#choosing-a-bind-address).

!!! warning "Old private networks silently ignore security groups"
    Private networks created before 6 September 2022 had port security turned off during an
    OpenStack upgrade, so security groups are not enforced on them. A rule that appears to exist
    and filters nothing is worse than no rule. Check and fix:

    ```bash
    openstack port show <port-id> -f value -c port_security_enabled
    openstack network set --enable-port-security <network-id>
    openstack port set --enable-port-security <port-id>
    ```

---

## Step 3 — Put the database on a Block Storage volume

The instance's disk goes away with the instance. A Cinder volume survives, resizes, and can be snapshotted on its own. Volumes run from 10 GB to 12 TB; `high-speed-gen2` is the current performance class, `classic` the cheaper one.

```bash
openstack volume create --type high-speed-gen2 --size 20 maintenant-data
openstack server add volume <server-id> <volume-id>
```

**OVHcloud formats and mounts nothing.** The disk shows up as a virtio device, `/dev/vdb` (the root disk being `/dev/vda`):

```bash
lsblk                                    # the new volume shows up as /dev/vdb
mkfs.ext4 /dev/vdb
mkdir -p /mnt/maintenant-data
blkid /dev/vdb                           # note the UUID
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
    `/dev/vdb` is the order the kernel happened to enumerate the disks in. Attach a second volume
    and the names can swap, which is how a database ends up pointed at the wrong filesystem.

!!! warning "Size the volume for migrations, not just for the data"
    Schema migrations rebuild tables in place and transiently need several times the size of the
    database. A volume sized to the current database will fail an upgrade. The 10 GB floor is also
    the smallest volume OVHcloud sells, so this costs you nothing to respect.

!!! note "Watching more than one host? Consider PostgreSQL"
    The server holds the agents' identity and enrolment, the one thing they cannot rebuild on
    their own. On the default SQLite file, losing that instance means re-enrolling every host by
    hand. Pointing the server at a PostgreSQL you already run makes the instance replaceable. See
    [PostgreSQL storage](postgresql.md).

---

## Monitoring several instances over a private network

Public Cloud private networks are built on the vRack, which recent projects get automatically.
Private traffic is free and unmetered.

```bash
openstack network create maintenant
openstack subnet create maintenant-subnet \
  --network maintenant --subnet-range 10.0.0.0/24
```

!!! important "Ask for DHCP explicitly"
    A subnet can be created with `--no-dhcp`, and then instances get no address on it at all. If
    you want addresses handed out, do not pass that flag. This is the first thing to check when an
    attached instance has no private IP.

**There is no internal DNS.** Unlike some providers, OVHcloud does not resolve instance names on a
private network: the request was closed as not planned. Agents therefore dial a private IP, or a
name you maintain yourself in `/etc/hosts` or your own DNS. Whichever you choose, the name has to
match the certificate the server presents.

Bind the gRPC listener to the private address on the server:

```bash
MAINTENANT_GRPC_LISTEN=10.0.0.2:8443
MAINTENANT_GRPC_URL=grpcs://maintenant.internal.example.com:8443
```

Then enrol each other instance. Generate a token from **Agents → Add host**: the modal hands you a
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

!!! important "Private does not mean plaintext"
    Binding on the private address keeps the listener off the public internet, but the agent still
    speaks TLS. Use a certificate that covers the name you gave the server; a DNS-01 ACME
    certificate works for a name that only resolves privately. Do **not** reach for
    `--grpc-insecure-skip-tls-verify` outside a lab. The full matrix of TLS modes is in
    [Agent Setup → Step 1](agent-setup.md#step-1-make-the-grpc-endpoint-reachable).

---

## Load Balancer

The Public Cloud Load Balancer is Octavia. A load balancer hides target failures by design: it
marks a failing member `ERROR` and stops forwarding to it while the public endpoint keeps
answering `200`. Checking only the load balancer tells you nothing until every member is down.

Configure two layers of endpoint checks:

| Check | Target | What it tells you |
|-------|--------|-------------------|
| Public | `https://app.example.com` | The service is reachable for users. |
| Per-member | `http://10.0.0.3:8080/health` on each instance | Which backend is down behind a still-green load balancer. |

The per-member checks run over the private network from the maintenant instance, so they need no
public exposure. Add them from **Endpoints → New endpoint**; see [Endpoint Monitoring](../features/endpoints.md).

Its public address is a floating IP bound to the load balancer's VIP port:

```bash
openstack loadbalancer show my-lb          # gives vip_port_id
openstack floating ip create Ext-Net
openstack floating ip set --port <vip-port-id> <floating-ip>
```

!!! warning "gRPC needs a TCP listener"
    Octavia listeners here are `HTTP`, `HTTPS`, `TCP`, `UDP`, `SCTP`, `TERMINATED_HTTPS` and
    `PROMETHEUS`. There is no gRPC protocol, and HTTP/2 support was closed as not planned. Put
    maintenant's gRPC listener behind a `TCP` listener with a TCP health check.

Health check thresholds are not published for a standalone load balancer, and the CLI requires you
to set `--delay`, `--timeout` and `--max-retries` yourself. What OVHcloud does document is what its
Kubernetes integration uses: a 5 second delay, a 3 second timeout, and 3 failures to take a member
out. Those are reasonable values to start from when you compare its verdict to maintenant's.

---

## Kubernetes on Managed Kubernetes

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace \
  --set persistence.storageClass=csi-cinder-high-speed \
  --set persistence.size=10Gi
```

`csi-cinder-high-speed` is the cluster default. The other classes are `csi-cinder-classic`,
`csi-cinder-high-speed-gen2`, their `-luks` encrypted variants, and `csi-cinder-classic-multiattach`.

Three MKS specifics matter here:

- **every provided class is `reclaimPolicy: Delete`.** There is no `-retain` variant, whatever
  third-party posts claim. To keep the database after a `helm uninstall`, patch the PV once it
  exists:

  ```bash
  kubectl patch pv <pv-name> -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'
  ```

- **`ReadWriteOnce` only.** The Cinder CSI driver never provisions `ReadWriteMany`, which suits
  maintenant: SQLite takes a single writer and the deployment never scales past one replica. A
  `ReadWriteMany` claim needs NFS-backed storage, which is not what you want under a database;
- **a volume is pinned to its availability zone.** A PVC provisioned in one zone is only reachable
  from nodes in that zone, so the pod follows the volume.

Full RBAC, namespace filtering and Helm values are in the [Kubernetes Guide](kubernetes.md).

---

## Backups and snapshots

A volume snapshot does not require detaching the volume, so it copies the database while it is
being written to. For SQLite in WAL mode that is a torn copy, not a backup. Take an
application-level copy first, then snapshot.

```bash
# On the instance: consistent copy while maintenant keeps running
docker compose -f /opt/maintenant/compose.yml exec maintenant \
  sqlite3 /data/maintenant.db ".backup '/data/maintenant.backup.db'"

# Then snapshot the volume, or image the whole instance
openstack volume snapshot create --volume <volume-id> --force \
  "maintenant-data-$(date -u +%F)"
openstack server image create --name "maintenant-$(date -u +%F)" <server-id>
```

If the image has no `sqlite3` binary, stop the stack for the few seconds the copy takes:

```bash
docker compose -f /opt/maintenant/compose.yml stop
cp /mnt/maintenant-data/maintenant/maintenant.db /root/maintenant-$(date -u +%F).db
docker compose -f /opt/maintenant/compose.yml start
```

!!! note "Snapshot and Backup are two different products"
    A **snapshot** works on an attached volume. A **Volume Backup**, which ships the data to Object
    Storage, requires detaching the volume first, so it means downtime for the monitoring server.
    OVHcloud also suggests keeping I/O low during the operation and avoiding peak hours, since a
    snapshot can take a long time.

---

## Related

- [Installation](../getting-started/installation.md) — Docker, Kubernetes and source builds
- [Hetzner Cloud Deployment](hetzner.md) — The same ground on Hetzner
- [DigitalOcean Deployment](digitalocean.md) — The same ground on DigitalOcean
- [Scaleway Deployment](scaleway.md) — The same ground on Scaleway
- [Agent Setup](agent-setup.md) — Enrolling additional hosts over gRPC
- [Kubernetes Guide](kubernetes.md) — RBAC, Helm values, workload monitoring
- [PostgreSQL Storage](postgresql.md) — Making the server replaceable
- [Endpoint Monitoring](../features/endpoints.md) — HTTP/TCP checks behind a load balancer

# Vultr Deployment Guide

How to run maintenant on Vultr: an instance provisioned with cloud-init, a firewall group, a Block Storage volume for the database, agents over a VPC, and VKE for the Kubernetes side.

---

## Why it fits a small instance

maintenant is a single Go binary with the frontend embedded and SQLite as its default store. It idles under 30 MB of RAM and needs no database server, no Prometheus, no Redis.

| Plan | vCPU / RAM | $/month | Typical fit |
|------|-----------|---------|-------------|
| `vc2-1c-2gb` | 1 / 2 GB | 10 | One host, a handful of containers and endpoint checks. |
| `vc2-2c-4gb` | 2 / 4 GB | 20 | Central server for a fleet of agents, or a busy update/CVE scan schedule. |
| `vhf-2c-4gb` | 2 / 4 GB | 24 | Same, on high-frequency cores. |

!!! warning "No ARM instances on Vultr"
    maintenant publishes `linux/arm64` images, and the Hetzner and Scaleway guides use them to cut
    costs. Vultr has nothing to run them on: every OS image in the catalogue is `x64`, and no
    compute plan reports an Ampere CPU. Only the amd64 half of the image is usable here, on
    instances and on VKE node pools alike.

---

## Step 1: Firewall group and VPC

Create these before the instance: the instance command of the next step takes the firewall group
and the VPC as arguments.

```bash
FW=$(vultr-cli firewall group create --description="maintenant" -o json | jq -r '.firewall_group.id')

# SSH from your address only
vultr-cli firewall rule create $FW \
  --ip-type=v4 --protocol=tcp --subnet="$(curl -s https://ipv4.icanhazip.com)" --size=32 \
  --port=22 --notes="ssh"

# HTTP/HTTPS for the reverse proxy that fronts maintenant
vultr-cli firewall rule create $FW \
  --ip-type=v4 --protocol=tcp --subnet=0.0.0.0 --size=0 --port=80 --notes="http"
vultr-cli firewall rule create $FW \
  --ip-type=v4 --protocol=tcp --subnet=0.0.0.0 --size=0 --port=443 --notes="https"
```

The group id is a **positional argument**, not a flag: the `--id=` form in the CLI's shipped
example does not exist. `--subnet` and `--size` are the network and its prefix length, split
across two arguments rather than written as CIDR.

Note what is **not** in that list: port `8080`. Publish the dashboard through a reverse proxy with authentication, never as a raw port on the public IP.

!!! warning "maintenant will flag its own exposure"
    If you change the Compose file to publish `"8080:8080"`, maintenant's own network security
    scanner reports a critical **Port exposed on all interfaces** finding for its own container.
    That is the intended behaviour, not a bug. See the note in
    [Installation → Docker Compose](../getting-started/installation.md#docker-compose-recommended)
    and [Configuration → Choosing a Bind Address](../getting-started/configuration.md#choosing-a-bind-address).

To attach the group to an existing instance:

```bash
vultr-cli instance update-firewall-group <instance-id> -f $FW
```

If you will enrol other instances later, create the VPC now too. It has to exist before the
instance, because a VPC is only configured on the instance when it is attached at deployment
(see the warning in the VPC section below).

```bash
vultr-cli vpc create --region="ewr" --description="maintenant" \
  --subnet="10.200.0.0" --size=24
```

VPC ranges are RFC1918 only, and you get five VPC networks per location. For a single server, skip
this and drop `--vpc-ids` from the next command.

---

## Step 2: Create the instance

The repository ships a ready-to-use cloud-config at [`deploy/cloud-init/maintenant.yaml`](https://github.com/kolapsis/maintenant/blob/main/deploy/cloud-init/maintenant.yaml). It installs Docker from the official repository, writes `/opt/maintenant/compose.yml`, and starts the stack on first boot.

Find the OS id first. Vultr's own examples all use `1743`, which is Ubuntu **22.04**:

```bash
vultr-cli os list | grep -i "24.04"     # Ubuntu 24.04 LTS x64 is 2284
```

```bash
vultr-cli instance create \
  --region="ewr" \
  --plan="vc2-2c-4gb" \
  --os=2284 \
  --label="maintenant" \
  --host="maintenant" \
  --ssh-keys="<sshkey-uuid>" \
  --vpc-ids="<vpc-uuid>" \
  --firewall-group="<firewall-group-uuid>" \
  --userdata-file=deploy/cloud-init/maintenant.yaml
```

`--firewall-group` takes the id that `$FW` holds after Step 1, and `--vpc-ids` the id printed by `vpc create`.

Then reach the dashboard, which the cloud-config binds to loopback:

```bash
ssh -L 8080:127.0.0.1:8080 root@<instance-ip>
```

Open **http://localhost:8080**. Every container on the host is already discovered.

!!! warning "The Docker socket is root on this instance"
    The cloud-config mounts `/var/run/docker.sock`. `:ro` only protects the socket file: the Docker API behind it still accepts writes, so whoever controls maintenant can start containers on this instance. Before you open the dashboard to anyone else, put a read-only docker-socket-proxy in front of the socket. See [Security: Docker socket proxy](../security.md#recommended-docker-socket-proxy), and enable `IMAGES` on the proxy as well if you use update checks.

Three details that cost time if you get them wrong:

!!! warning "Do not base64-encode the user data yourself"
    The API expects base64, but `vultr-cli` encodes it for you. Encoding it yourself produces a
    double-encoded blob, and cloud-init then does nothing at all, silently. Pass the plain file
    with `--userdata-file`, or plain text with `-u`. The two flags are mutually exclusive.

!!! warning "`-o` is not short for `--os`"
    `--os` takes an integer and has no short form. `-o` is the global `--output` flag. The
    abbreviated example shipped in the CLI's own help (`-o=1743`) is wrong and will not select an
    operating system.

!!! note "The flag is `--firewall-group` here"
    At creation the flag is `--firewall-group`. `--firewall-group-id` exists only on
    `vultr-cli instance update-firewall-group`, which is how you attach one afterwards.

### Behind a reverse proxy

Authentication is the proxy's job: maintenant has none. [Security](../security.md#reverse-proxy-setup) has working Traefik, Caddy and nginx setups. Two settings in `/opt/maintenant/compose.yml` make maintenant behave correctly behind it:

```yaml
    environment:
      MAINTENANT_BASE_URL: "https://maintenant.example.com"
      MAINTENANT_TRUSTED_PROXIES: "172.18.0.1"
```

- `MAINTENANT_BASE_URL` is the public address. It defaults to `http://0.0.0.0:8080` here, which is what heartbeat ping URLs, status page subscriber links and the MCP OAuth issuer would otherwise carry.
- `MAINTENANT_TRUSTED_PROXIES` lists the addresses the proxy connects from. Empty, no forwarded header is read and every visitor is counted as the proxy, so they share one rate limit. A proxy on this instance reaches the port published on `127.0.0.1` through the Docker network's gateway, so list that address (`docker network inspect maintenant_default`, field `Gateway`), not `127.0.0.1`. See [Running behind a proxy](../security.md#running-behind-a-proxy).

---

## Step 3: Put the database on a Block Storage volume

The instance's disk goes away with the instance. A Block Storage volume survives, resizes, and
can be cloned from a snapshot.

**Check the region first.** Block Storage is not available everywhere, and NVMe is available in
far fewer regions than HDD:

```bash
curl -s "https://api.vultr.com/v2/regions?per_page=500" | \
  jq -r '.regions[] | select(.options | index("block_storage_high_perf")) | .id'
```

Then create and attach:

```bash
vultr-cli block-storage create \
  --region="ewr" --size=20 --label="maintenant-data" --block-type="high_perf"

vultr-cli block-storage attach <block-id> --instance=<instance-id> --live
```

!!! warning "Without `--live`, the instance reboots"
    Both `attach` and `detach` restart the instance unless you pass `--live`. On a monitoring
    server that reboot is a gap in your own history.

Minimum sizes are **10 GB for NVMe** (`high_perf`) and **40 GB for HDD** (`storage_opt`), at
$0.10 and $0.025 per GB per month. Vultr's Block Storage FAQ still says 1 GB for NVMe; that figure
is out of date and the API rejects it.

**Vultr formats and mounts nothing.** The first volume appears as `/dev/vdb`:

```bash
lsblk
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

You do not need to create or `chown` the `maintenant` directory: Docker creates it as root if it is missing, and the entrypoint hands it to uid 65534 at start.

The first boot already started maintenant on the named volume `maintenant_maintenant-data`, and the database created there does not follow the new mount: the next start opens an empty database at the new path. That is fine on an instance you have just created. To keep what was collected, copy it across while the stack is stopped (`cp -a` keeps the ownership):

```bash
docker compose -f /opt/maintenant/compose.yml down
mkdir -p /mnt/maintenant-data/maintenant
cp -a /var/lib/docker/volumes/maintenant_maintenant-data/_data/. /mnt/maintenant-data/maintenant/
# edit compose.yml as above, then
docker compose -f /opt/maintenant/compose.yml up -d
```

Do this before you enrol any agent: the server's database holds their identities. Once maintenant runs on the volume, `docker volume rm maintenant_maintenant-data` removes the old copy.

!!! tip "Mount by UUID, not by device name"
    `/dev/vdb` is the order the kernel happened to enumerate the disks in. Attach a second volume
    and the names can swap, which is how a database ends up pointed at the wrong filesystem.
    Vultr's own CSI driver uses a stable per-volume path instead,
    `/dev/disk/by-id/virtio-<mount-id>`, where the mount id is the `MOUNT ID` field of
    `vultr-cli block-storage get`. Either is fine; the bare device name is not.

!!! warning "Size the volume for migrations, not just for the data"
    Schema migrations rebuild tables in place and transiently need several times the size of the
    database. A volume sized to the current database will fail an upgrade. The 10 GB NVMe floor
    already covers this.

!!! note "Watching more than one host? Consider PostgreSQL"
    The server holds the agents' identity and enrolment, the one thing they cannot rebuild on
    their own. On the default SQLite file, losing that instance means re-enrolling every host by
    hand. Pointing the server at a PostgreSQL you already run makes the instance replaceable. See
    [PostgreSQL storage](postgresql.md).

---

## Monitoring several instances over a VPC

The VPC was created in Step 1 and attached to the server when it was deployed. Every other instance
joins it the same way, with `--vpc-ids` on `instance create`.

!!! warning "Attach the VPC at creation, or configure the interface by hand"
    This is the one Vultr detail that will cost you an afternoon. On Linux, cloud-init configures
    the private adapter **only if the VPC is attached while the instance is being deployed**. Pass
    `--vpc-ids` to `instance create`. Attaching a VPC afterwards with `instance vpc attach` leaves
    the interface unconfigured until you write the netplan yourself, and the instance simply has no
    private address in the meantime.

    On Windows and BSD the private adapter is manual even at deployment. On Fedora CoreOS the
    network configuration comes from ignition and cannot be changed after deployment.

Vultr documents no internal DNS for VPC networks, so address the server by its private IP (the
`internal_ip` field of the instance) or by a name you maintain yourself. Whichever you choose, it
has to match the certificate the server presents.

!!! note "Enrolling hosts needs the Personal edition"
    Remote hosts require the Personal edition or above (`MAINTENANT_LICENSE_KEY`). Personal covers up to 20 hosts, Pro has no cap. On Community the server does not open the agent port: it logs `agent gRPC listener not started: agents need the personal edition or above`.

The gRPC listener defaults to `127.0.0.1:8443`. Inside the container that address cannot be reached from the network, for the same reason as the HTTP port. The server must listen on all interfaces **in the container**, and the private address must be published in the Compose file. Edit `/opt/maintenant/compose.yml`:

```diff
   maintenant:
     ports:
       - "127.0.0.1:8080:8080"
+      - "10.200.0.2:8443:8443"
     volumes:
+      - /opt/maintenant/tls:/etc/maintenant/tls:ro
       - maintenant-data:/data
     environment:
       MAINTENANT_ADDR: "0.0.0.0:8080"
       MAINTENANT_DB: "/data/maintenant.db"
+      MAINTENANT_LICENSE_KEY: "<your license key>"
+      MAINTENANT_GRPC_LISTEN: "0.0.0.0:8443"
+      MAINTENANT_GRPC_URL: "grpcs://maintenant.internal.example.com:8443"
+      MAINTENANT_GRPC_TLS_CERT: "/etc/maintenant/tls/server.crt"
+      MAINTENANT_GRPC_TLS_KEY: "/etc/maintenant/tls/server.key"
```

Publish on the private address (`10.200.0.2`), never as a bare `8443:8443`. That address must exist on the host when the container starts, or Docker refuses it with `cannot assign requested address`: this is what you get if the VPC was attached after deployment and the interface is still unconfigured. Apply with `docker compose -f /opt/maintenant/compose.yml up -d`.

The certificate and key are read at startup by uid 65534, so they must be readable by it, and a renewed certificate is only picked up when the container restarts. The certificate has to cover the name in `MAINTENANT_GRPC_URL`. A DNS-01 ACME certificate works for a name that only resolves privately, as long as the domain is yours. With a private CA instead, issue the server certificate from it and give the agents the CA, as shown below. If you put the private IP itself in `MAINTENANT_GRPC_URL`, the enrollment modal shows a "Local address detected" warning: expected here, since the agents reach the server over the VPC.

Open the port to the VPC range only:

```bash
vultr-cli firewall rule create $FW \
  --ip-type=v4 --protocol=tcp --subnet=10.200.0.0 --size=24 --port=8443 \
  --notes="agent gRPC"
```

Then enrol each other instance. In the web UI, open **Agents** and click **Generate enrollment token**: the modal shows the token once, with a ready-made command per environment and the server address taken from `MAINTENANT_GRPC_URL`. Run it on the host, pointing at the private address:

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
  --server=grpcs://maintenant.internal.example.com:8443 \
  --enrollment-token=mnt_enr_XXXXXXXXXXXXXXXX
```

!!! important "Private does not mean plaintext"
    Publishing on the VPC address keeps the listener off the public internet, but the agent still
    speaks TLS. Use a certificate that covers the name you gave the server. Do **not** reach for
    `--grpc-insecure-skip-tls-verify` outside a lab. The full matrix of TLS modes is in
    [Agent Setup → Step 1](agent-setup.md#step-1-make-the-grpc-endpoint-reachable).

    With a private CA, add these two lines to the agent command (the generated one does not
    include them):

    ```bash
      -e MAINTENANT_CA_CERT=/etc/maintenant/ca.pem \
      -v /opt/maintenant/ca.pem:/etc/maintenant/ca.pem:ro \
    ```

    The file must be readable by uid 65534: an unreadable or invalid CA makes the agent stop at
    start with `failed to load extra CA bundle`.

---

## Load Balancer

A load balancer hides target failures by design: it stops forwarding to a backend that fails its health checks while the public endpoint keeps answering `200`. Checking only the load balancer tells you nothing until every backend is down.

Configure two layers of endpoint checks:

| Check | Target | What it tells you |
|-------|--------|-------------------|
| Public | `https://app.example.com` | The service is reachable for users. |
| Per-backend | `http://10.200.0.3:8080/health` on each instance | Which backend is down behind a still-green load balancer. |

The per-backend checks run over the VPC from the maintenant instance, so they need no public
exposure. Add them from **Endpoints → New endpoint**; see [Endpoint Monitoring](../features/endpoints.md).

Its public address comes from:

```bash
vultr-cli load-balancer get <lb-id> -o json | jq -r '.load_balancer.ipv4'
```

!!! warning "The CLI's health check defaults are not the platform's"
    Vultr's platform defaults are a 15 second interval, a 5 second response timeout, and 5
    consecutive failures to take a backend out: roughly 75 seconds. `vultr-cli` sends **15** for
    the timeout and both thresholds instead, which pushes the same removal to about **225
    seconds**, with nothing in the output to say so. If you create the load balancer from the CLI,
    set `--response-timeout`, `--unhealthy-threshold` and `--healthy-threshold` explicitly rather
    than inheriting that.

!!! warning "gRPC needs TCP forwarding"
    Forwarding rules are HTTP, HTTPS and TCP, and nothing in Vultr's documentation mentions gRPC.
    HTTP/2 and HTTP/3 exist but are client-side only and require an active HTTPS rule, so they
    buy nothing for a gRPC backend. Use a TCP forwarding rule with a TCP health check and
    terminate TLS on your own backend. Note also the cap of 15 forwarding rules per load balancer.

---

## Kubernetes on VKE

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace \
  --set persistence.storageClass=vultr-block-storage-retain \
  --set persistence.size=10Gi
```

The Vultr CSI driver ships six storage classes: `vultr-block-storage` and
`vultr-block-storage-hdd`, their `-retain` variants, and two `vultr-vfs-storage` classes. Unlike
most providers here, a `Retain` variant exists out of the box, which is what you want under a
database you would rather not lose to a `helm uninstall`.

!!! danger "Never put SQLite on `vultr-vfs-storage`"
    The VFS classes are the only `ReadWriteMany` ones. SQLite on a shared filesystem corrupts:
    its locking assumes a single writer on a local filesystem. Use a block class, which is
    `ReadWriteOnce`, and matches maintenant's single-replica deployment anyway.

!!! warning "Claim at least 10Gi, or the PVC hangs"
    The block minimums are 10 GB for NVMe and 40 GB for HDD. The CSI driver passes a smaller
    request straight through to the API without clamping it, so a `1Gi` claim is rejected and the
    PVC sits in `Pending` with the reason visible only in `kubectl describe pvc`. A `500Mi` claim
    becomes a size of zero.

Always name `storageClassName` explicitly, as above: which class is the cluster default is not
something Vultr documents in a form worth relying on, and none of the classes in the self-managed
manifest carries the default annotation at all.

!!! note "If you expose maintenant with a `LoadBalancer` Service"
    VKE's cloud controller defaults to a TCP protocol and a TCP health check, which is right for
    gRPC. Setting the `healthcheck-path` annotation silently switches the check to HTTP and breaks
    a gRPC backend, so leave it unset. And do not edit a load balancer created by the controller
    by hand; it will be reconciled away.

Full RBAC, namespace filtering and Helm values are in the [Kubernetes Guide](kubernetes.md).

---

## Backups and snapshots

Vultr snapshots a running instance without stopping it, and says plainly what that means: booting
from one is like rebooting after a non-graceful restart. For SQLite in WAL mode that is a
crash-consistent copy, not a backup. Take an application-level copy first.

The image does not ship a `sqlite3` client. Install one on the instance and copy the database from the file behind the volume, which works while maintenant keeps running:

```bash
apt-get install -y sqlite3
sqlite3 -readonly /mnt/maintenant-data/maintenant/maintenant.db \
  ".backup '/root/maintenant-backup.db'"

# Then snapshot the instance
vultr-cli snapshot create -i <instance-id> -d "maintenant $(date -u +%F)"
```

Without a client, stop the stack for the few seconds the copy takes and copy the database files together:

```bash
docker compose -f /opt/maintenant/compose.yml stop
mkdir -p /root/maintenant-$(date -u +%F)
cp -a /mnt/maintenant-data/maintenant/maintenant.db* /root/maintenant-$(date -u +%F)/
docker compose -f /opt/maintenant/compose.yml start
```

!!! note "Block Storage snapshots exist, but not in the CLI"
    The API can snapshot a Block Storage volume and clone a volume from a snapshot, and the CLI
    can consume one through `block-storage create --snapshot-id`. It cannot create one: there is
    no `block-storage snapshot` subcommand. Use the API directly if you want volume-level
    snapshots on a schedule.

---

## Related

- [Installation](../getting-started/installation.md) — Docker, Kubernetes and source builds
- [Hetzner Cloud Deployment](hetzner.md) — The same ground on Hetzner
- [DigitalOcean Deployment](digitalocean.md) — The same ground on DigitalOcean
- [Scaleway Deployment](scaleway.md) — The same ground on Scaleway
- [OVHcloud Deployment](ovhcloud.md) — The same ground on OVHcloud
- [Agent Setup](agent-setup.md) — Enrolling additional hosts over gRPC
- [Kubernetes Guide](kubernetes.md) — RBAC, Helm values, workload monitoring
- [PostgreSQL Storage](postgresql.md) — Making the server replaceable
- [Endpoint Monitoring](../features/endpoints.md) — HTTP/TCP checks behind a load balancer

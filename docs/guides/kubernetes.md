# Kubernetes Guide

maintenant runs natively on Kubernetes with read-only RBAC, namespace filtering, and workload-level monitoring out of the box.

---

## Deployment

### Helm (recommended)

The chart is in the repository and is not published to a Helm repository, so clone the repository first:

```bash
git clone https://github.com/kolapsis/maintenant.git
cd maintenant

helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace
```

This is the recommended approach for production clusters. See the [Helm section](#helm) below for full options.

On a managed cluster, name the storage class your provider's CSI driver installs, or the PVC stays `Pending`:

| Cluster | Value |
|---------|-------|
| Hetzner (`kube-hetzner`, `hetzner-k3s`, Talos) | `--set persistence.storageClass=hcloud-volumes`, see [Kubernetes on Hetzner](hetzner.md#kubernetes-on-hetzner) |
| DigitalOcean (DOKS) | `--set persistence.storageClass=do-block-storage-retain`, see [Kubernetes on DOKS](digitalocean.md#kubernetes-on-doks) |
| Scaleway (Kapsule) | `--set persistence.storageClass=sbs-default`, see [Kubernetes on Kapsule](scaleway.md#kubernetes-on-kapsule) |
| OVHcloud (MKS) | `--set persistence.storageClass=csi-cinder-high-speed`, see [Kubernetes on Managed Kubernetes](ovhcloud.md#kubernetes-on-managed-kubernetes) |
| Vultr (VKE) | `--set persistence.storageClass=vultr-block-storage-retain`, see [Kubernetes on VKE](vultr.md#kubernetes-on-vke) |

### Raw manifests

From a clone of the repository, create the namespace and apply the manifests. They all carry `namespace: maintenant`, so no `-n` is needed:

```bash
kubectl create namespace maintenant
kubectl apply -f deploy/kubernetes/
```

To use another namespace, change `namespace: maintenant` in `deployment.yaml` and in `rbac.yaml` (the ServiceAccount and the subject of the ClusterRoleBinding).

This creates:

| Resource | Description |
|----------|-------------|
| **ServiceAccount** | `maintenant`, the identity used to reach the API server |
| **ClusterRole** | Read-only access to the resources listed in [RBAC Permissions](#rbac-permissions) |
| **ClusterRoleBinding** | Binds the role to the service account |
| **Deployment** | Single replica, strategy `Recreate`, hardened pod, image `ghcr.io/kolapsis/maintenant:latest` with `imagePullPolicy: Always` |
| **PersistentVolumeClaim** | 10 Gi for SQLite storage: migrations rebuild tables in place and need several times the database size |
| **Service** | ClusterIP on port 80 |

---

## RBAC Permissions

maintenant requests the minimum permissions needed for monitoring. The manifests, the Helm chart and the manifest generated for a Kubernetes agent all carry the same rules:

```yaml
rules:
  # Core resources: read-only
  - apiGroups: [""]
    resources: ["namespaces", "nodes", "pods", "events", "services"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
  # Workloads: read-only
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["get", "list", "watch"]
  # Metrics: read-only
  - apiGroups: ["metrics.k8s.io"]
    resources: ["pods", "nodes"]
    verbs: ["get", "list"]
```

| Resource | Used for |
|----------|----------|
| `namespaces`, `pods`, `apps` workloads, `jobs` | Discovery of workloads and pods, kept current by watching pods, Deployments, StatefulSets and DaemonSets |
| `nodes` | The Nodes page and the `node_condition` alert |
| `events` | The events shown in the detail of a workload or a pod |
| `services` | Security insights for LoadBalancer and NodePort Services |
| `pods/log` | Container logs |
| `metrics.k8s.io` | CPU and memory |

maintenant never creates, modifies, or deletes any resource in your cluster. If the role lacks a resource, maintenant logs an `RBAC: forbidden to list ...` warning and skips that kind of object instead of failing.

!!! info "Metrics Server required"
    Resource metrics (CPU/memory) require [metrics-server](https://github.com/kubernetes-sigs/metrics-server)
    to be installed in the cluster. Workload monitoring works without it.

---

## Security Hardening

The default deployment includes:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 65534       # nobody
  runAsGroup: 65534
  fsGroup: 65534
containers:
  - securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]
```

The container runs as `nobody` (uid 65534) with an immutable root filesystem and all Linux capabilities dropped. `fsGroup` makes the data volume writable for that user. Because the pod already starts as a non-root user, the image entrypoint runs the binary as it is: it does not change ownership of the volume nor look for a Docker socket. A `/tmp` emptyDir is mounted because the root filesystem is read-only. The database and SQLite's temporary files live on the `/data` volume.

This mirrors the Docker Compose hardening (`read_only: true`, `no-new-privileges`, non-root user). See [Security](../security.md) for the full container security reference.

---

## Namespace Filtering

By default, maintenant monitors all namespaces except the system ones: **`kube-system`, `kube-public` and `kube-node-lease` are excluded as long as no allowlist is set**. Use environment variables (or the `--k8sNamespaces` and `--k8sExcludeNamespaces` flags) to change the scope:

### Allowlist

Only monitor specific namespaces:

```yaml
env:
  - name: MAINTENANT_K8S_NAMESPACES
    value: "default,production,staging"
```

With an allowlist, only the listed namespaces are monitored, system namespaces included if you name them.

### Blocklist

Monitor all namespaces except specific ones:

```yaml
env:
  - name: MAINTENANT_K8S_EXCLUDE_NAMESPACES
    value: "cert-manager,monitoring"
```

The blocklist is added to the three system namespaces, which you do not need to list.

If both variables are set, the allowlist wins and the blocklist is ignored.

---

## Workload Monitoring

maintenant groups pods by their owning workload:

| Workload | What maintenant tracks |
|----------|----------------------|
| **Deployment** | Desired and ready replicas, rollout status |
| **StatefulSet** | Desired and ready replicas |
| **DaemonSet** | Desired and ready pods across the nodes |
| **Job** | Active, succeeded and failed pods against the desired completions |
| **Bare pod** | A pod with no controller is tracked on its own, with its phase and restart count |

Each workload appears as a single entry with an aggregated health status: `healthy`, `degraded` (some replicas are ready), `failed` (none is ready), or `progressing` during a rollout. A workload scaled to zero is healthy. Individual pods are accessible in the detail view. ReplicaSets have no entry of their own: the pods of a Deployment's ReplicaSet are attached to the Deployment.

PersistentVolumeClaims are not tracked, and volumes are not part of a StatefulSet's health.

---

## Annotations

Annotations on a workload adjust how maintenant treats it. They use the names of the [Docker labels](docker-labels.md#kubernetes-annotations) and go on the metadata of the Deployment, StatefulSet, DaemonSet or bare pod, not on its pod template. A Job only reads `maintenant.ignore`:

| Annotation | Effect |
|------------|--------|
| `maintenant.ignore: "true"` | Leave the workload out of monitoring. On a workload (or on a Job) its alerts stop, and so do those of its pods. Put it in the pod template to silence the `crash_loop` alert of those pods only. |
| `maintenant.group` | Group name instead of the namespace |
| `maintenant.alert.severity` | Severity of the `container_down` alert |
| `maintenant.alert.restart_threshold` | Threshold of the `restart_loop` alert |
| `maintenant.update.*` | Image update tracking. The image checked is the first container of the pod spec. |

```bash
kubectl annotate deployment/api -n production \
  maintenant.update.track=minor \
  maintenant.alert.severity=critical
```

The alerts below read `maintenant.ignore` every 30 seconds, and the update scan reads `maintenant.update.*` each time it runs. The other annotations are picked up when maintenant reconciles: at startup, when it reconnects to the API server, or when a new workload appears. Only the server's own cluster reads annotations: a Kubernetes agent reports topology without them.

The update command shown for a workload is `kubectl set image <kind>/<name> <container>=<image> -n <namespace>`, where the container is the first one of the pod spec. A tag that was republished is deployed by its digest (`nginx:stable@sha256:...`), and a bare pod is updated in place.

---

## Alerts

The server raises three Kubernetes alerts for the cluster it runs in or is pointed at. The cluster is evaluated every 30 seconds.

| Alert | Severity | Raised when | Resolved when |
|-------|----------|-------------|---------------|
| `replica_health` | Warning, Critical when no replica is ready | A Deployment, StatefulSet or DaemonSet has fewer ready replicas than desired for 5 minutes | All desired replicas are ready, the workload is scaled to zero, ignored or deleted |
| `crash_loop` | Critical | A pod is in `CrashLoopBackOff`, or has restarted 3 times within 10 minutes | The pod stops crash-looping, is ignored or deleted |
| `node_condition` | Critical when the node is not Ready, Warning under memory, disk or PID pressure | A node reports one of those conditions | The conditions clear, or the node leaves the cluster |

- `replica_health` is per workload and escalates in place from Warning to Critical when the last ready replica is lost. A Job never raises it.
- `crash_loop` is per pod.
- `node_condition` is one alert per node, however many conditions it reports. A node whose Ready condition is `Unknown` counts as not Ready.
- If the nodes cannot be listed (for instance an RBAC denial), no `node_condition` alert is raised or resolved.
- After a restart, maintenant takes over the open Kubernetes alerts and resolves those whose condition is gone.
- A Kubernetes agent reports its cluster for display only: these alerts exist for the server's own cluster.

---

## Security Insights

For every workload that a Service of type `LoadBalancer` or `NodePort` selects, maintenant records a critical security insight per port the Service opens outside the cluster: `service_load_balancer`, `service_node_port`, or `database_port_exposed` when the port is a well-known database port. Only the server's own cluster is analyzed, and reading Services needs the `services` permission above. See [Network Security Insights](../features/security.md).

---

## Runtime Detection

maintenant auto-detects Kubernetes in this order:

1. `MAINTENANT_RUNTIME=kubernetes` environment variable (explicit override)
2. `KUBERNETES_SERVICE_HOST` environment variable (set automatically by Kubernetes for in-cluster pods)
3. `KUBECONFIG` environment variable or `~/.kube/config` file (for out-of-cluster development)
4. Docker, when none of the above applies

To force Kubernetes mode:

```yaml
env:
  - name: MAINTENANT_RUNTIME
    value: "kubernetes"
```

When detection finds a kubeconfig but its cluster does not answer, maintenant falls back to Docker and logs a warning. Set `MAINTENANT_RUNTIME=kubernetes` to make it wait for that cluster instead.

### Connection loss

maintenant never gives up on the API server: at startup and after a loss it retries with a backoff from 1 to 30 seconds, for as long as it takes. Pods, Deployments, StatefulSets and DaemonSets are watched, so changes appear as they happen. When the API server stops answering for three probes in a row, 15 seconds apart (about 45 seconds), maintenant goes into degraded mode: monitoring is suspended and `/api/v1/health` reports `runtime.connected: false`. It reconnects by itself and resumes when the API server answers. An error answer such as `403 Forbidden` counts as an answer, not as a loss. A configuration error (no in-cluster configuration and no kubeconfig) is retried as well, every 1 to 60 seconds, and each attempt logs its cause.

---

## Health Probes

The deployment includes startup, liveness and readiness probes:

```yaml
startupProbe:
  httpGet:
    path: /api/v1/health
    port: http
  periodSeconds: 10
  failureThreshold: 60
livenessProbe:
  httpGet:
    path: /api/v1/health
    port: http
  initialDelaySeconds: 5
  periodSeconds: 30
readinessProbe:
  httpGet:
    path: /api/v1/health
    port: http
  initialDelaySeconds: 3
  periodSeconds: 10
```

!!! warning "Do not drop the startup probe"
    Schema migrations run **before** the HTTP listener starts, so `/api/v1/health`
    stays silent for their whole duration: minutes on a large database, and longer
    for the one-time UUID conversion. Without a startup probe the liveness probe
    kills the pod about 95 s in, leaving the migration half-applied and the schema
    marked dirty, and the same thing happens on every restart. The
    `failureThreshold: 60` above grants 10 minutes; raise it if an upgrade needs
    more (`startupProbe.failureThreshold` in the chart).

!!! warning "Do not make the probe fail on a database outage"
    With an external PostgreSQL, `/api/v1/health` deliberately answers `200`
    even when the database is unreachable: the outage is reported in
    `storage.connected`. A probe that failed on it would restart the pod exactly
    when the database needs to be left alone, and restarting fixes nothing: the
    instance recovers on its own once the database answers. If you want to
    alert on it, read `storage.connected`; do not wire it to a probe. See
    [PostgreSQL storage](postgresql.md).

---

## Resource Limits

Default resource requests and limits:

```yaml
resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

Adjust based on the number of monitored workloads. maintenant is lightweight: 50-100 workloads run comfortably within these limits.

---

## Scaling Considerations

!!! warning "Single replica only"
    maintenant uses SQLite with a single-writer pattern. The deployment strategy is set to
    `Recreate`: do not scale beyond 1 replica.

For high availability, ensure your PersistentVolumeClaim uses a storage class with adequate durability. To let a replacement pod pick up the fleet on another node, keep the data in an external PostgreSQL (see [Helm](#with-an-external-postgresql)); the `/data` volume must still follow the instance.

---

## Exposing the Dashboard

maintenant has no authentication of its own: put an authenticating proxy in front of any Ingress that reaches beyond a trusted network. See [Security](../security.md).

### Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: maintenant
  namespace: maintenant
spec:
  rules:
    - host: now.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: maintenant
                port:
                  name: http
```

### Port Forward (Development)

```bash
kubectl port-forward -n maintenant svc/maintenant 8080:80
```

Open **http://localhost:8080**.

---

## Helm

The chart is located in `deploy/helm/maintenant/` (chart version 1.3.0, application version 1.8.0).

### Minimal install

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace
```

### With Ingress

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace \
  --set ingress.enabled=true \
  --set ingress.host=maintenant.example.com \
  --set ingress.className=nginx
```

### With TLS

```yaml
# values-prod.yaml
ingress:
  enabled: true
  className: nginx
  host: maintenant.example.com
  tls:
    - secretName: maintenant-tls
      hosts:
        - maintenant.example.com
```

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace \
  -f values-prod.yaml
```

### License

A Personal or Pro license unlocks the paid features. Pass the license key directly:

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace \
  --set license.key=YOUR_LICENSE_KEY
```

Or reference an existing secret (recommended for GitOps):

```bash
kubectl create secret generic maintenant-license \
  --from-literal=license-key=YOUR_LICENSE_KEY \
  -n maintenant

helm install maintenant ./deploy/helm/maintenant \
  -n maintenant \
  --set license.existingSecret=maintenant-license
```

### With an external PostgreSQL

By default the instance keeps its data in SQLite on the volume. To store the server data in a PostgreSQL that you operate, put the connection string in a Secret and name it:

```bash
kubectl create secret generic maintenant-database \
  --from-literal=url='postgres://maintenant:secret@db.internal:5432/maintenant?sslmode=require' \
  -n maintenant

helm install maintenant ./deploy/helm/maintenant \
  -n maintenant \
  --set database.existingSecret=maintenant-database
```

The chart passes the value as `MAINTENANT_DATABASE_URL`. `database.urlKey` names the key inside the Secret (default `url`). The persistent volume is still needed: it holds the license cache and the update window record. See [PostgreSQL storage](postgresql.md).

### Extra environment and agents

`extraEnv` adds any `MAINTENANT_*` variable to the container:

```yaml
extraEnv:
  - name: MAINTENANT_K8S_EXCLUDE_NAMESPACES
    value: "cert-manager"
  - name: MAINTENANT_LOG_LEVEL
    value: "debug"
```

The chart opens only the HTTP port. To enrol agents from other hosts with a server running in the cluster (Personal edition or above), set `MAINTENANT_GRPC_LISTEN=0.0.0.0:8443` and the gRPC TLS variables through `extraEnv`, and expose that port with a Service or an Ingress of your own. See [Multi-Host Monitoring](../features/multihost.md) and [Agent Setup](agent-setup.md).

### Key values

| Value | Default | Description |
|-------|---------|-------------|
| `image.repository` | `ghcr.io/kolapsis/maintenant` | Image repository |
| `image.tag` | `latest` | Image tag to deploy. Empty uses the chart's application version (`1.8.0`). Release tags have no `v` (for example `1.8.0`). |
| `image.pullPolicy` | `Always` | With `latest`, restart the pod to pull a newer image. Pin `image.tag` to a version for reproducible upgrades. |
| `runtime` | `kubernetes` | `kubernetes` or `docker` (`MAINTENANT_RUNTIME`) |
| `persistence.enabled` | `true` | `false` uses an emptyDir: the data is lost when the pod restarts |
| `persistence.size` | `10Gi` | SQLite volume size. Allow several times the database size: migrations rebuild tables in place. |
| `persistence.storageClass` | `""` | Storage class (cluster default if empty) |
| `persistence.accessMode` | `ReadWriteOnce` | Access mode of the PVC |
| `persistence.existingClaim` | `""` | Use an existing PVC |
| `startupProbe.periodSeconds` | `10` | Startup probe period |
| `startupProbe.failureThreshold` | `60` | Startup probe failures tolerated: 10 minutes of grace for migrations |
| `service.type`, `service.port` | `ClusterIP`, `80` | Service exposing the HTTP port |
| `ingress.enabled` | `false` | Enable Ingress resource |
| `ingress.host` | `maintenant.example.com` | Ingress hostname |
| `ingress.className` | `""` | Ingress class |
| `ingress.annotations`, `ingress.tls` | `{}`, `[]` | Ingress annotations and TLS |
| `license.key` | `""` | Personal or Pro license key |
| `license.existingSecret` | `""` | Existing secret holding the license under the key `license-key` |
| `database.existingSecret` | `""` | Existing secret holding the PostgreSQL connection string |
| `database.urlKey` | `url` | Key of the connection string in that secret |
| `extraEnv` | `[]` | Extra environment variables for the container |
| `resources` | see values.yaml | CPU/memory requests and limits |
| `serviceAccount.create`, `serviceAccount.name` | `true`, `""` | Create the ServiceAccount, or use your own |
| `rbac.create` | `true` | Create the ClusterRole and ClusterRoleBinding (with `serviceAccount.create`) |
| `imagePullSecrets`, `podAnnotations`, `podLabels`, `nodeSelector`, `tolerations`, `affinity` | empty | Standard pod settings |

### Upgrade

```bash
helm upgrade maintenant ./deploy/helm/maintenant -n maintenant
```

### Uninstall

```bash
helm uninstall maintenant -n maintenant
```

!!! warning "The chart's PVC is deleted on uninstall"
    The chart creates the claim (`<release fullname>-data`) as one of its own resources, so `helm uninstall` deletes it, and the data with it unless the storage class keeps volumes after their claim is gone. Back up the database first. To keep the data outside the release, create the claim yourself and set `persistence.existingClaim`.

---

## Monitoring a Cluster with an Agent

To watch a cluster from a maintenant that runs elsewhere, run an agent in it. The enrollment dialog generates the manifest: a single-replica Deployment (strategy `Recreate`) in a `maintenant` namespace, with a 1 Gi volume for its data directory, a hardened pod and the RBAC above. One agent watches the whole cluster and counts as one host. See [Agent Setup](agent-setup.md) and [Multi-Host Monitoring](../features/multihost.md).

---

## Related

- [Installation](../getting-started/installation.md): Docker and source builds
- [Hetzner Cloud Deployment](hetzner.md): clusters built with kube-hetzner, hetzner-k3s or Talos
- [DigitalOcean Deployment](digitalocean.md): DOKS storage classes and Load Balancer checks
- [Scaleway Deployment](scaleway.md): Kapsule storage classes and Private Network DNS
- [OVHcloud Deployment](ovhcloud.md): MKS storage classes and zone-pinned volumes
- [Vultr Deployment](vultr.md): VKE storage classes and the 10Gi PVC floor
- [Docker Labels Reference](docker-labels.md): annotations and labels
- [Configuration](../getting-started/configuration.md): environment variables
- [Container Monitoring](../features/containers.md): how workloads are tracked
- [Resource Metrics](../features/resources.md): CPU/memory from metrics-server

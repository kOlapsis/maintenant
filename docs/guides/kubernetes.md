# Kubernetes Guide

maintenant runs natively on Kubernetes with read-only RBAC, namespace filtering, and workload-level monitoring out of the box.

---

## Deployment

### Helm (recommended)

```bash
helm install maintenant ./deploy/helm/maintenant \
  -n maintenant --create-namespace
```

This is the recommended approach for production clusters. See the [Helm section](#helm) below for full options.

### Raw manifests

Apply the provided manifests:

```bash
kubectl create namespace maintenant
kubectl apply -f deploy/kubernetes/
```

This creates:

| Resource | Description |
|----------|-------------|
| **ServiceAccount** | `maintenant` — identity for API access |
| **ClusterRole** | Read-only access to pods, logs, services, events, workloads, and metrics |
| **ClusterRoleBinding** | Binds the role to the service account |
| **Deployment** | Single replica with security hardening |
| **PersistentVolumeClaim** | 1 Gi for SQLite storage |
| **Service** | ClusterIP on port 80 |

---

## RBAC Permissions

maintenant requests the minimum permissions needed for monitoring:

```yaml
rules:
  # Core resources — read-only
  - apiGroups: [""]
    resources: ["pods", "pods/log", "services", "namespaces", "events"]
    verbs: ["get", "list", "watch"]
  # Workloads — read-only
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
    verbs: ["get", "list", "watch"]
  # Metrics — read-only
  - apiGroups: ["metrics.k8s.io"]
    resources: ["pods"]
    verbs: ["get", "list"]
```

maintenant never creates, modifies, or deletes any resource in your cluster.

!!! info "Metrics Server required"
    Resource metrics (CPU/memory) require [metrics-server](https://github.com/kubernetes-sigs/metrics-server)
    to be installed in the cluster. Container monitoring works without it.

---

## Security Hardening

The default deployment includes:

```yaml
securityContext:
  runAsNonRoot: true
  fsGroup: 65534
containers:
  - securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]
```

A `/tmp` emptyDir is mounted for SQLite WAL temporary files since the root filesystem is read-only.

---

## Namespace Filtering

By default, maintenant monitors all namespaces. Use environment variables to restrict scope:

### Allowlist

Only monitor specific namespaces:

```yaml
env:
  - name: MAINTENANT_K8S_NAMESPACES
    value: "default,production,staging"
```

### Blocklist

Monitor all namespaces except specific ones:

```yaml
env:
  - name: MAINTENANT_K8S_EXCLUDE_NAMESPACES
    value: "kube-system,kube-public,cert-manager"
```

!!! tip "System namespaces"
    `kube-system` and `kube-public` are excluded by default when using the blocklist.
    You do not need to add them explicitly.

If both `MAINTENANT_K8S_NAMESPACES` and `MAINTENANT_K8S_EXCLUDE_NAMESPACES` are set, the allowlist takes precedence.

---

## Workload Monitoring

maintenant groups pods by their owning workload:

| Workload | What maintenant tracks |
|----------|----------------------|
| **Deployment** | Replica count, ready pods, rollout status |
| **StatefulSet** | Ordered pod states, persistent volume claims |
| **DaemonSet** | Node coverage, desired vs ready counts |

Each workload appears as a single entry in the dashboard with aggregated health status. Individual pods are accessible in the detail view.

---

## Runtime Detection

maintenant auto-detects Kubernetes in this order:

1. `MAINTENANT_RUNTIME=kubernetes` environment variable (explicit override)
2. `KUBERNETES_SERVICE_HOST` environment variable (set automatically by Kubernetes for in-cluster pods)
3. `KUBECONFIG` environment variable or `~/.kube/config` file (for out-of-cluster development)

To force Kubernetes mode:

```yaml
env:
  - name: MAINTENANT_RUNTIME
    value: "kubernetes"
```

---

## Health Probes

The deployment includes liveness and readiness probes:

```yaml
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

Adjust based on the number of monitored workloads. maintenant is lightweight — 50-100 workloads run comfortably within these limits.

---

## Scaling Considerations

!!! warning "Single replica only"
    maintenant uses SQLite with a single-writer pattern. The deployment strategy is set to
    `Recreate` — do not scale beyond 1 replica.

For high availability, ensure your PersistentVolumeClaim uses a storage class with adequate durability.

---

## Exposing the Dashboard

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

The chart is located in `deploy/helm/maintenant/`.

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

### Enterprise license

Pass the license key directly:

```bash
helm install maintenant ./deploy/helm/maintenant \
  --set license.key=YOUR_LICENSE_KEY
```

Or reference an existing secret (recommended for GitOps):

```bash
kubectl create secret generic maintenant-license \
  --from-literal=license-key=YOUR_LICENSE_KEY \
  -n maintenant

helm install maintenant ./deploy/helm/maintenant \
  --set license.existingSecret=maintenant-license
```

### Key values

| Value | Default | Description |
|-------|---------|-------------|
| `image.tag` | `""` (chart appVersion) | Image tag to deploy |
| `runtime` | `kubernetes` | `kubernetes` or `docker` |
| `persistence.size` | `1Gi` | SQLite volume size |
| `persistence.storageClass` | `""` | Storage class (cluster default if empty) |
| `persistence.existingClaim` | `""` | Use an existing PVC |
| `ingress.enabled` | `false` | Enable Ingress resource |
| `ingress.host` | `maintenant.example.com` | Ingress hostname |
| `ingress.className` | `""` | Ingress class |
| `license.key` | `""` | Enterprise license key |
| `license.existingSecret` | `""` | Existing secret name for the license key |
| `resources` | see values.yaml | CPU/memory requests and limits |

### Upgrade

```bash
helm upgrade maintenant ./deploy/helm/maintenant -n maintenant
```

### Uninstall

```bash
helm uninstall maintenant -n maintenant
```

!!! warning "PVC not deleted on uninstall"
    Helm does not delete PersistentVolumeClaims on uninstall to prevent accidental data loss.
    Delete it manually if needed: `kubectl delete pvc maintenant-data -n maintenant`

---

## Related

- [Installation](../getting-started/installation.md) — Docker and source builds
- [Configuration](../getting-started/configuration.md) — Environment variables
- [Container Monitoring](../features/containers.md) — How workloads are tracked
- [Resource Metrics](../features/resources.md) — CPU/memory from metrics-server

// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package v1

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/uid"
)

// kubernetesStore is the per-agent store the handler reads from. Implemented by
// *sqlite.KubernetesStore. An empty agentID means "all agents"; "local" is
// resolved to the LocalAgent sentinel by the handler.
type kubernetesStore interface {
	ListNamespaces(ctx context.Context, agentID string) ([]string, error)
	ListWorkloads(ctx context.Context, agentID string, namespaces []string) ([]kubernetes.K8sWorkloadGroup, error)
	GetWorkload(ctx context.Context, agentID, workloadID string) (*kubernetes.K8sWorkload, error)
	ListPods(ctx context.Context, agentID string, namespaces []string, filters kubernetes.PodFilters) ([]kubernetes.K8sPod, error)
	GetPod(ctx context.Context, agentID, namespace, name string) (*kubernetes.K8sPod, error)
	ListNodes(ctx context.Context, agentID string) ([]kubernetes.K8sNode, error)
	ListEventsForObject(ctx context.Context, agentID, kind, namespace, name string) ([]kubernetes.K8sEvent, error)
}

// K8sMetricsProvider is an optional interface for runtimes that can query pod/node
// metrics. Only meaningful for the server's own local cluster (live
// metrics-server); remote agents do not ship metrics, so requests scoped to a
// remote agent report metrics as unavailable.
type K8sMetricsProvider interface {
	MetricsAvailable() bool
	GetPodMetrics(ctx context.Context, namespace, name string) (*kubernetes.PodResourceMetrics, error)
	GetNodeMetrics(ctx context.Context, name string) (*kubernetes.NodeResourceMetrics, error)
}

// KubernetesHandler handles Kubernetes API endpoints. Reads are served from the
// per-agent store (fed by the local runtime under LocalAgent and by remote
// agents), so the views work regardless of the server's own runtime.
type KubernetesHandler struct {
	store   kubernetesStore
	metrics K8sMetricsProvider
	agents  AgentDirectory
}

// NewKubernetesHandler creates a new KubernetesHandler. metrics may be nil when
// the server has no live metrics-capable cluster.
func NewKubernetesHandler(store kubernetesStore, metrics K8sMetricsProvider) *KubernetesHandler {
	return &KubernetesHandler{store: store, metrics: metrics}
}

// SetAgentDirectory wires agent name resolution so multi-host list views can show
// which agent each remote workload/pod belongs to.
func (h *KubernetesHandler) SetAgentDirectory(ad AgentDirectory) {
	h.agents = ad
}

// agentNames resolves agent display identities, or nil when unavailable.
func (h *KubernetesHandler) agentNames(ctx context.Context) map[string]AgentName {
	if h.agents == nil {
		return nil
	}
	names, err := h.agents.AgentNames(ctx)
	if err != nil {
		return nil
	}
	return names
}

// agentScopeParam resolves the agent_id query parameter: "" → all agents, "local"
// → the LocalAgent sentinel, anything else → that agent verbatim.
func agentScopeParam(r *http.Request) string {
	a := r.URL.Query().Get("agent_id")
	if a == "local" {
		return uid.LocalAgent
	}
	return a
}

// isRemoteScope reports whether the scope is a specific remote agent (i.e. not
// "all" and not the local runtime), for which live metrics are unavailable.
func isRemoteScope(agentID string) bool {
	return agentID != "" && agentID != uid.LocalAgent
}

// HandleListNamespaces handles GET /api/v1/kubernetes/namespaces.
func (h *KubernetesHandler) HandleListNamespaces(w http.ResponseWriter, r *http.Request) {
	namespaces, err := h.store.ListNamespaces(r.Context(), agentScopeParam(r))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list namespaces")
		return
	}

	if namespaces == nil {
		namespaces = []string{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"namespaces": namespaces,
		"total":      len(namespaces),
	})
}

// HandleListWorkloads handles GET /api/v1/kubernetes/workloads.
// Query params: agent_id, namespaces (comma-separated), kind, status.
func (h *KubernetesHandler) HandleListWorkloads(w http.ResponseWriter, r *http.Request) {
	namespaces := splitParam(r.URL.Query().Get("namespaces"))
	kindFilter := r.URL.Query().Get("kind")
	statusFilter := r.URL.Query().Get("status")

	groups, err := h.store.ListWorkloads(r.Context(), agentScopeParam(r), namespaces)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list workloads")
		return
	}

	// Apply kind/status filters and build response.
	names := h.agentNames(r.Context())
	total := 0
	respGroups := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		wls := make([]map[string]interface{}, 0, len(g.Workloads))
		for _, wl := range g.Workloads {
			if kindFilter != "" && !strings.EqualFold(wl.Kind, kindFilter) {
				continue
			}
			if statusFilter != "" && !strings.EqualFold(wl.Status, statusFilter) {
				continue
			}
			m := workloadToJSON(wl)
			enrichAgentFields(m, wl.AgentID, names)
			wls = append(wls, m)
			total++
		}
		if len(wls) == 0 {
			continue
		}
		respGroups = append(respGroups, map[string]interface{}{
			"namespace": g.Namespace,
			"workloads": wls,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"groups": respGroups,
		"total":  total,
	})
}

// HandleGetWorkload handles GET /api/v1/kubernetes/workloads/{id}.
// The id path value is URL-encoded (namespace%2FKind%2Fname).
func (h *KubernetesHandler) HandleGetWorkload(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")
	id, err := url.PathUnescape(rawID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid workload id encoding")
		return
	}

	agentID := agentScopeParam(r)
	wl, err := h.store.GetWorkload(r.Context(), agentID, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get workload")
		return
	}
	if wl == nil {
		WriteError(w, http.StatusNotFound, "K8S_WORKLOAD_NOT_FOUND", "Workload "+id+" not found")
		return
	}

	// Pods owning-ref'd to this workload. Note: Deployment pods carry a
	// ReplicaSet workload_ref, so for Deployments this may under-match until the
	// agent resolves refs up to the top-level controller.
	pods, err := h.store.ListPods(r.Context(), agentID, []string{wl.Namespace}, kubernetes.PodFilters{Workload: id})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list workload pods")
		return
	}
	podList := make([]map[string]interface{}, 0, len(pods))
	for _, p := range pods {
		podList = append(podList, podToJSON(p))
	}

	events, err := h.store.ListEventsForObject(r.Context(), agentID, wl.Kind, wl.Namespace, wl.Name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list workload events")
		return
	}
	eventList := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		eventList = append(eventList, eventToJSON(e))
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"workload": workloadToJSON(*wl),
		"pods":     podList,
		"events":   eventList,
	})
}

// HandleListPods handles GET /api/v1/kubernetes/pods.
// Query params: namespaces (comma-separated), workload, node, status.
func (h *KubernetesHandler) HandleListPods(w http.ResponseWriter, r *http.Request) {
	namespaces := splitParam(r.URL.Query().Get("namespaces"))
	filters := kubernetes.PodFilters{
		Workload: r.URL.Query().Get("workload"),
		Node:     r.URL.Query().Get("node"),
		Status:   r.URL.Query().Get("status"),
	}

	pods, err := h.store.ListPods(r.Context(), agentScopeParam(r), namespaces, filters)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list pods")
		return
	}

	names := h.agentNames(r.Context())
	result := make([]map[string]interface{}, 0, len(pods))
	for _, p := range pods {
		m := podToJSON(p)
		enrichAgentFields(m, p.AgentID, names)
		result = append(result, m)
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"pods":  result,
		"total": len(result),
	})
}

// HandleGetPodDetail handles GET /api/v1/kubernetes/pods/{namespace}/{name}.
func (h *KubernetesHandler) HandleGetPodDetail(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	agentID := agentScopeParam(r)
	pod, err := h.store.GetPod(r.Context(), agentID, namespace, name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get pod")
		return
	}
	if pod == nil {
		WriteError(w, http.StatusNotFound, "K8S_POD_NOT_FOUND", "Pod "+namespace+"/"+name+" not found")
		return
	}

	events, err := h.store.ListEventsForObject(r.Context(), agentID, "Pod", namespace, name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list pod events")
		return
	}
	eventList := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		eventList = append(eventList, eventToJSON(e))
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"pod":    podToJSON(*pod),
		"events": eventList,
	})
}

// HandleListNodes handles GET /api/v1/kubernetes/nodes.
func (h *KubernetesHandler) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.store.ListNodes(r.Context(), agentScopeParam(r))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list nodes")
		return
	}

	result := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, nodeDetailToJSON(n))
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"nodes": result,
		"total": len(result),
	})
}

// HandleGetCluster handles GET /api/v1/kubernetes/cluster. The overview is
// aggregated from the per-agent store rather than a live cluster query.
func (h *KubernetesHandler) HandleGetCluster(w http.ResponseWriter, r *http.Request) {
	agentID := agentScopeParam(r)
	ctx := r.Context()

	groups, err := h.store.ListWorkloads(ctx, agentID, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get cluster overview")
		return
	}
	pods, err := h.store.ListPods(ctx, agentID, nil, kubernetes.PodFilters{})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get cluster overview")
		return
	}
	nodes, err := h.store.ListNodes(ctx, agentID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get cluster overview")
		return
	}

	// Per-namespace + global workload counts.
	workloadCount, workloadHealthy := 0, 0
	nsWorkloads := make(map[string]int)
	nsHealthy := make(map[string]bool)
	nsSeen := make(map[string]bool)
	var nsOrder []string
	for _, g := range groups {
		if !nsSeen[g.Namespace] {
			nsSeen[g.Namespace] = true
			nsOrder = append(nsOrder, g.Namespace)
			nsHealthy[g.Namespace] = true
		}
		for _, wl := range g.Workloads {
			workloadCount++
			nsWorkloads[g.Namespace]++
			if wl.Status == "healthy" {
				workloadHealthy++
			} else {
				nsHealthy[g.Namespace] = false
			}
		}
	}

	// Pod status tally + per-namespace pod counts.
	var running, pending, failed, succeeded, unknown int
	nsPods := make(map[string]int)
	for _, p := range pods {
		nsPods[p.Namespace]++
		switch strings.ToLower(p.Status) {
		case "running":
			running++
		case "pending":
			pending++
		case "failed":
			failed++
		case "succeeded":
			succeeded++
		default:
			unknown++
		}
	}

	nodeReady := 0
	for _, n := range nodes {
		if strings.EqualFold(n.Status, "ready") {
			nodeReady++
		}
	}

	nsSummaries := make([]map[string]interface{}, 0, len(nsOrder))
	for _, ns := range nsOrder {
		nsSummaries = append(nsSummaries, map[string]interface{}{
			"name":           ns,
			"workload_count": nsWorkloads[ns],
			"pod_count":      nsPods[ns],
			"healthy":        nsHealthy[ns],
		})
	}

	clusterHealth := "healthy"
	if workloadHealthy < workloadCount || nodeReady < len(nodes) {
		clusterHealth = "degraded"
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"namespace_count":  len(nsOrder),
		"node_count":       len(nodes),
		"node_ready_count": nodeReady,
		"pod_status": map[string]interface{}{
			"running":   running,
			"pending":   pending,
			"failed":    failed,
			"succeeded": succeeded,
			"unknown":   unknown,
		},
		"workload_count":   workloadCount,
		"workload_healthy": workloadHealthy,
		"cluster_health":   clusterHealth,
		"namespaces":       nsSummaries,
	})
}

// HandleGetWorkloadResources handles GET /api/v1/kubernetes/workloads/{id}/resources (Pro).
// Returns per-pod CPU/RAM from metrics-server.
func (h *KubernetesHandler) HandleGetWorkloadResources(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")
	id, err := url.PathUnescape(rawID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid workload id encoding")
		return
	}

	agentID := agentScopeParam(r)
	// Live metrics-server data only exists for the server's own cluster; remote
	// agents do not ship metrics.
	if isRemoteScope(agentID) || h.metrics == nil || !h.metrics.MetricsAvailable() {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"metrics_available": false,
			"message":           "Install metrics-server for resource data",
			"pods":              []interface{}{},
		})
		return
	}

	wl, err := h.store.GetWorkload(r.Context(), agentID, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get workload")
		return
	}
	if wl == nil {
		WriteError(w, http.StatusNotFound, "K8S_WORKLOAD_NOT_FOUND", "Workload "+id+" not found")
		return
	}
	pods, err := h.store.ListPods(r.Context(), agentID, []string{wl.Namespace}, kubernetes.PodFilters{Workload: id})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list workload pods")
		return
	}

	podMetrics := make([]map[string]interface{}, 0, len(pods))
	for _, p := range pods {
		entry := map[string]interface{}{
			"name":      p.Name,
			"namespace": p.Namespace,
			"node_name": p.NodeName,
			"status":    p.Status,
		}

		pm, err := h.metrics.GetPodMetrics(r.Context(), p.Namespace, p.Name)
		if err != nil {
			entry["cpu_millicores"] = nil
			entry["mem_bytes"] = nil
			entry["mem_limit_bytes"] = nil
			entry["mem_percent"] = nil
			entry["timestamp"] = nil
		} else {
			memPercent := 0.0
			if pm.MemLimitBytes > 0 {
				memPercent = float64(pm.MemBytes) / float64(pm.MemLimitBytes) * 100.0
			}
			entry["cpu_millicores"] = pm.CPUMillicores
			entry["mem_bytes"] = pm.MemBytes
			entry["mem_limit_bytes"] = pm.MemLimitBytes
			entry["mem_percent"] = memPercent
			entry["timestamp"] = pm.Timestamp.Format(time.RFC3339)
		}

		podMetrics = append(podMetrics, entry)
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"metrics_available": true,
		"workload_id":       id,
		"pods":              podMetrics,
	})
}

// HandleGetNodeResources handles GET /api/v1/kubernetes/nodes/{name}/resources (Pro).
// Returns node-level CPU/RAM from metrics-server.
func (h *KubernetesHandler) HandleGetNodeResources(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if isRemoteScope(agentScopeParam(r)) || h.metrics == nil || !h.metrics.MetricsAvailable() {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"metrics_available": false,
			"message":           "Install metrics-server for resource data",
		})
		return
	}

	nm, err := h.metrics.GetNodeMetrics(r.Context(), name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get node metrics")
		return
	}

	cpuPercent := 0.0
	if nm.CPUCapacityMillicores > 0 {
		cpuPercent = float64(nm.CPUMillicores) / float64(nm.CPUCapacityMillicores) * 100.0
	}
	memPercent := 0.0
	if nm.MemCapacityBytes > 0 {
		memPercent = float64(nm.MemBytes) / float64(nm.MemCapacityBytes) * 100.0
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"metrics_available":       true,
		"node_name":               name,
		"cpu_millicores":          nm.CPUMillicores,
		"cpu_capacity_millicores": nm.CPUCapacityMillicores,
		"cpu_percent":             cpuPercent,
		"mem_bytes":               nm.MemBytes,
		"mem_capacity_bytes":      nm.MemCapacityBytes,
		"mem_percent":             memPercent,
		"timestamp":               nm.Timestamp.Format(time.RFC3339),
	})
}

// --- JSON serialisation helpers ---

func workloadToJSON(wl kubernetes.K8sWorkload) map[string]interface{} {
	conditions := make([]map[string]interface{}, 0, len(wl.Conditions))
	for _, c := range wl.Conditions {
		conditions = append(conditions, map[string]interface{}{
			"type":            c.Type,
			"status":          c.Status,
			"reason":          c.Reason,
			"message":         c.Message,
			"last_transition": formatTime(c.LastTransition),
		})
	}

	return map[string]interface{}{
		"id":               wl.ID,
		"name":             wl.Name,
		"namespace":        wl.Namespace,
		"kind":             wl.Kind,
		"images":           wl.Images,
		"ready_replicas":   wl.ReadyReplicas,
		"desired_replicas": wl.DesiredReplicas,
		"status":           wl.Status,
		"conditions":       conditions,
		"labels":           wl.Labels,
		"created_at":       formatTime(wl.CreatedAt),
		"last_transition":  formatTime(wl.LastTransition),
	}
}

func podToJSON(p kubernetes.K8sPod) map[string]interface{} {
	containers := make([]map[string]interface{}, 0, len(p.Containers))
	for _, c := range p.Containers {
		cs := map[string]interface{}{
			"name":          c.Name,
			"image":         c.Image,
			"ready":         c.Ready,
			"restart_count": c.RestartCount,
			"state":         c.State,
			"state_reason":  c.StateReason,
			"started_at":    nil,
		}
		if c.StartedAt != nil {
			cs["started_at"] = c.StartedAt.UTC().Format(time.RFC3339)
		}
		containers = append(containers, cs)
	}

	return map[string]interface{}{
		"name":          p.Name,
		"namespace":     p.Namespace,
		"status":        p.Status,
		"status_reason": p.StatusReason,
		"restart_count": p.RestartCount,
		"node_name":     p.NodeName,
		"pod_ip":        p.PodIP,
		"host_ip":       p.HostIP,
		"containers":    containers,
		"workload_ref":  p.WorkloadRef,
		"created_at":    formatTime(p.CreatedAt),
	}
}

func eventToJSON(e kubernetes.K8sEvent) map[string]interface{} {
	return map[string]interface{}{
		"type":       e.Type,
		"reason":     e.Reason,
		"message":    e.Message,
		"source":     e.Source,
		"first_seen": formatTime(e.FirstSeen),
		"last_seen":  formatTime(e.LastSeen),
		"count":      e.Count,
	}
}

func nodeDetailToJSON(n kubernetes.K8sNode) map[string]interface{} {
	conditions := make([]map[string]interface{}, 0, len(n.Conditions))
	for _, c := range n.Conditions {
		conditions = append(conditions, map[string]interface{}{
			"type":            c.Type,
			"status":          c.Status,
			"reason":          c.Reason,
			"message":         c.Message,
			"last_transition": formatTime(c.LastTransition),
		})
	}

	return map[string]interface{}{
		"name":   n.Name,
		"roles":  n.Roles,
		"status": n.Status,
		"capacity": map[string]interface{}{
			"cpu_millicores": n.Capacity.CPUMillicores,
			"memory_bytes":   n.Capacity.MemoryBytes,
			"pods":           n.Capacity.Pods,
		},
		"allocatable": map[string]interface{}{
			"cpu_millicores": n.Allocatable.CPUMillicores,
			"memory_bytes":   n.Allocatable.MemoryBytes,
			"pods":           n.Allocatable.Pods,
		},
		"running_pods":       n.RunningPods,
		"kubernetes_version": n.KubernetesVersion,
		"os_image":           n.OSImage,
		"architecture":       n.Architecture,
		"conditions":         conditions,
		"created_at":         formatTime(n.CreatedAt),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// splitParam splits a comma-separated query parameter, trimming whitespace and
// filtering empty strings.
func splitParam(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

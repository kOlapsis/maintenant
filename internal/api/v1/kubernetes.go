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
)

// KubernetesProvider is the interface the handler needs from the K8s runtime.
type KubernetesProvider interface {
	ListNamespaces(ctx context.Context) ([]string, error)
	ListWorkloads(ctx context.Context, namespaces []string) ([]kubernetes.K8sWorkloadGroup, error)
	GetWorkload(ctx context.Context, id string) (*kubernetes.K8sWorkload, []kubernetes.K8sPod, []kubernetes.K8sEvent, error)
	ListPods(ctx context.Context, namespaces []string, filters kubernetes.PodFilters) ([]kubernetes.K8sPod, error)
	GetPodDetail(ctx context.Context, namespace, name string) (*kubernetes.K8sPod, []kubernetes.K8sEvent, error)
	ListNodes(ctx context.Context) ([]kubernetes.K8sNode, error)
	ClusterOverview(ctx context.Context) (*kubernetes.K8sClusterOverview, error)
}

// K8sMetricsProvider is an optional interface for runtimes that can query pod/node metrics.
type K8sMetricsProvider interface {
	MetricsAvailable() bool
	GetPodMetrics(ctx context.Context, namespace, name string) (*kubernetes.PodResourceMetrics, error)
	GetNodeMetrics(ctx context.Context, name string) (*kubernetes.NodeResourceMetrics, error)
}

// KubernetesHandler handles Kubernetes API endpoints.
type KubernetesHandler struct {
	k8s     KubernetesProvider
	metrics K8sMetricsProvider
}

// NewKubernetesHandler creates a new KubernetesHandler.
func NewKubernetesHandler(provider KubernetesProvider) *KubernetesHandler {
	h := &KubernetesHandler{k8s: provider}
	if mp, ok := provider.(K8sMetricsProvider); ok {
		h.metrics = mp
	}
	return h
}

// HandleListNamespaces handles GET /api/v1/kubernetes/namespaces.
func (h *KubernetesHandler) HandleListNamespaces(w http.ResponseWriter, r *http.Request) {
	namespaces, err := h.k8s.ListNamespaces(r.Context())
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
// Query params: namespaces (comma-separated), kind, status.
func (h *KubernetesHandler) HandleListWorkloads(w http.ResponseWriter, r *http.Request) {
	namespaces := splitParam(r.URL.Query().Get("namespaces"))
	kindFilter := r.URL.Query().Get("kind")
	statusFilter := r.URL.Query().Get("status")

	groups, err := h.k8s.ListWorkloads(r.Context(), namespaces)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list workloads")
		return
	}

	// Apply kind/status filters and build response.
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
			wls = append(wls, workloadToJSON(wl))
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

	wl, pods, events, err := h.k8s.GetWorkload(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get workload")
		return
	}
	if wl == nil {
		WriteError(w, http.StatusNotFound, "K8S_WORKLOAD_NOT_FOUND", "Workload "+id+" not found")
		return
	}

	podList := make([]map[string]interface{}, 0, len(pods))
	for _, p := range pods {
		podList = append(podList, podToJSON(p))
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

	pods, err := h.k8s.ListPods(r.Context(), namespaces, filters)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to list pods")
		return
	}

	result := make([]map[string]interface{}, 0, len(pods))
	for _, p := range pods {
		result = append(result, podToJSON(p))
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

	pod, events, err := h.k8s.GetPodDetail(r.Context(), namespace, name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get pod")
		return
	}
	if pod == nil {
		WriteError(w, http.StatusNotFound, "K8S_POD_NOT_FOUND", "Pod "+namespace+"/"+name+" not found")
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
	nodes, err := h.k8s.ListNodes(r.Context())
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

// HandleGetCluster handles GET /api/v1/kubernetes/cluster.
func (h *KubernetesHandler) HandleGetCluster(w http.ResponseWriter, r *http.Request) {
	overview, err := h.k8s.ClusterOverview(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get cluster overview")
		return
	}

	nsSummaries := make([]map[string]interface{}, 0, len(overview.Namespaces))
	for _, ns := range overview.Namespaces {
		nsSummaries = append(nsSummaries, map[string]interface{}{
			"name":           ns.Name,
			"workload_count": ns.WorkloadCount,
			"pod_count":      ns.PodCount,
			"healthy":        ns.Healthy,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"namespace_count":  overview.NamespaceCount,
		"node_count":       overview.NodeCount,
		"node_ready_count": overview.NodeReadyCount,
		"pod_status": map[string]interface{}{
			"running":   overview.PodStatus.Running,
			"pending":   overview.PodStatus.Pending,
			"failed":    overview.PodStatus.Failed,
			"succeeded": overview.PodStatus.Succeeded,
			"unknown":   overview.PodStatus.Unknown,
		},
		"workload_count":   overview.WorkloadCount,
		"workload_healthy": overview.WorkloadHealthy,
		"cluster_health":   overview.ClusterHealth,
		"namespaces":       nsSummaries,
	})
}

// HandleGetWorkloadResources handles GET /api/v1/kubernetes/workloads/{id}/resources (Enterprise).
// Returns per-pod CPU/RAM from metrics-server.
func (h *KubernetesHandler) HandleGetWorkloadResources(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")
	id, err := url.PathUnescape(rawID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid workload id encoding")
		return
	}

	if h.metrics == nil || !h.metrics.MetricsAvailable() {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"metrics_available": false,
			"message":           "Install metrics-server for resource data",
			"pods":              []interface{}{},
		})
		return
	}

	wl, pods, _, err := h.k8s.GetWorkload(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to get workload")
		return
	}
	if wl == nil {
		WriteError(w, http.StatusNotFound, "K8S_WORKLOAD_NOT_FOUND", "Workload "+id+" not found")
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

// HandleGetNodeResources handles GET /api/v1/kubernetes/nodes/{name}/resources (Enterprise).
// Returns node-level CPU/RAM from metrics-server.
func (h *KubernetesHandler) HandleGetNodeResources(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if h.metrics == nil || !h.metrics.MetricsAvailable() {
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
		"metrics_available":      true,
		"node_name":              name,
		"cpu_millicores":         nm.CPUMillicores,
		"cpu_capacity_millicores": nm.CPUCapacityMillicores,
		"cpu_percent":            cpuPercent,
		"mem_bytes":              nm.MemBytes,
		"mem_capacity_bytes":     nm.MemCapacityBytes,
		"mem_percent":            memPercent,
		"timestamp":              nm.Timestamp.Format(time.RFC3339),
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

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
	"net/http"
	"sort"
	"time"

	"github.com/kolapsis/maintenant/internal/swarm"
)

// SwarmHandler handles Swarm API endpoints.
type SwarmHandler struct {
	cluster        func() *swarm.SwarmCluster
	discovery      func() *swarm.ServiceDiscovery
	detector       func() *swarm.Detector
	nodeStore      swarm.NodeStore
	updateTracker  *swarm.UpdateTracker
	crashLoop      *swarm.CrashLoopDetector
}

// NewSwarmHandler creates a new Swarm API handler.
func NewSwarmHandler(
	clusterFn func() *swarm.SwarmCluster,
	discoveryFn func() *swarm.ServiceDiscovery,
	detectorFn func() *swarm.Detector,
	nodeStore swarm.NodeStore,
	updateTracker *swarm.UpdateTracker,
	crashLoop *swarm.CrashLoopDetector,
) *SwarmHandler {
	return &SwarmHandler{
		cluster:       clusterFn,
		discovery:     discoveryFn,
		detector:      detectorFn,
		nodeStore:     nodeStore,
		updateTracker: updateTracker,
		crashLoop:     crashLoop,
	}
}

// HandleGetInfo handles GET /api/v1/swarm/info.
func (h *SwarmHandler) HandleGetInfo(w http.ResponseWriter, r *http.Request) {
	cluster := h.cluster()
	if cluster == nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"active": false,
		})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"active":        true,
		"cluster_id":    cluster.ID,
		"is_manager":    cluster.IsManager,
		"manager_count": cluster.ManagerCount,
		"worker_count":  cluster.WorkerCount,
		"created_at":    cluster.CreatedAt,
	})
}

// HandleListServices handles GET /api/v1/swarm/services.
func (h *SwarmHandler) HandleListServices(w http.ResponseWriter, r *http.Request) {
	disc := h.discovery()
	if disc == nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"services": []interface{}{},
			"total":    0,
		})
		return
	}

	stackFilter := r.URL.Query().Get("stack")
	services := disc.ListServices()

	// Filter by stack if requested.
	if stackFilter != "" {
		filtered := make([]*swarm.SwarmService, 0)
		for _, s := range services {
			if s.StackName == stackFilter {
				filtered = append(filtered, s)
			}
		}
		services = filtered
	}

	// Sort by name.
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	result := make([]map[string]interface{}, 0, len(services))
	for _, s := range services {
		result = append(result, serviceToJSON(s))
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"services": result,
		"total":    len(result),
	})
}

// HandleGetService handles GET /api/v1/swarm/services/{serviceID}.
func (h *SwarmHandler) HandleGetService(w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("serviceID")

	disc := h.discovery()
	if disc == nil {
		WriteError(w, http.StatusConflict, "SWARM_NOT_ACTIVE", "Swarm mode is not active")
		return
	}

	svc := disc.GetService(serviceID)
	if svc == nil {
		WriteError(w, http.StatusNotFound, "SWARM_SERVICE_NOT_FOUND", "Service "+serviceID+" not found")
		return
	}

	resp := serviceToJSON(svc)
	// Tasks would be populated from discovery cache in a full implementation.
	resp["tasks"] = []interface{}{}

	WriteJSON(w, http.StatusOK, resp)
}

// HandleListNodes handles GET /api/v1/swarm/nodes (Enterprise).
func (h *SwarmHandler) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	if h.nodeStore == nil {
		WriteError(w, http.StatusConflict, "SWARM_NODES_NOT_AVAILABLE", "Node monitoring is not available")
		return
	}

	nodes, err := h.nodeStore.ListNodes(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list nodes")
		return
	}

	managerCount := 0
	workerCount := 0
	result := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, nodeToJSON(n))
		switch n.Role {
		case "manager":
			managerCount++
		case "worker":
			workerCount++
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"nodes":         result,
		"total":         len(result),
		"manager_count": managerCount,
		"worker_count":  workerCount,
	})
}

// HandleGetNodeDetail handles GET /api/v1/swarm/nodes/{nodeID} (Enterprise).
func (h *SwarmHandler) HandleGetNodeDetail(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("nodeID")

	if h.nodeStore == nil {
		WriteError(w, http.StatusConflict, "SWARM_NODES_NOT_AVAILABLE", "Node monitoring is not available")
		return
	}

	node, err := h.nodeStore.GetNodeByNodeID(r.Context(), nodeID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get node")
		return
	}
	if node == nil {
		WriteError(w, http.StatusNotFound, "SWARM_NODE_NOT_FOUND", "Node "+nodeID+" not found")
		return
	}

	resp := nodeToJSON(node)

	// Enrich with tasks running on this node.
	tasks := make([]map[string]interface{}, 0)
	disc := h.discovery()
	if disc != nil {
		for _, svc := range disc.ListServices() {
			for _, t := range disc.GetTasksForService(svc.ServiceID) {
				if t.NodeID == nodeID {
					tasks = append(tasks, map[string]interface{}{
						"task_id":      t.TaskID,
						"service_id":   t.ServiceID,
						"service_name": svc.Name,
						"slot":         t.Slot,
						"state":        t.State,
						"image":        svc.Image,
						"timestamp":    t.Timestamp.Format(time.RFC3339),
					})
				}
			}
		}
	}
	resp["tasks"] = tasks

	WriteJSON(w, http.StatusOK, resp)
}

// HandleGetUpdateStatus handles GET /api/v1/swarm/services/{serviceID}/update-status (Enterprise).
func (h *SwarmHandler) HandleGetUpdateStatus(w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("serviceID")

	disc := h.discovery()
	if disc == nil {
		WriteError(w, http.StatusConflict, "SWARM_NOT_ACTIVE", "Swarm mode is not active")
		return
	}

	svc := disc.GetService(serviceID)
	if svc == nil {
		WriteError(w, http.StatusNotFound, "SWARM_SERVICE_NOT_FOUND", "Service "+serviceID+" not found")
		return
	}

	if h.updateTracker == nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"service_id":    serviceID,
			"service_name":  svc.Name,
			"update_status": nil,
			"progress":      nil,
		})
		return
	}

	progress, err := h.updateTracker.GetUpdateStatus(r.Context(), serviceID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get update status")
		return
	}

	if progress == nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"service_id":    serviceID,
			"service_name":  svc.Name,
			"update_status": nil,
			"progress":      nil,
		})
		return
	}

	us := map[string]interface{}{
		"state":   progress.State,
		"message": progress.Message,
	}
	if progress.StartedAt != nil {
		us["started_at"] = progress.StartedAt.Format(time.RFC3339)
	}
	if progress.CompletedAt != nil {
		us["completed_at"] = progress.CompletedAt.Format(time.RFC3339)
	} else {
		us["completed_at"] = nil
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"service_id":   serviceID,
		"service_name": svc.Name,
		"update_status": us,
		"progress": map[string]interface{}{
			"old_image":     progress.OldImage,
			"new_image":     progress.NewImage,
			"tasks_updated": progress.TasksUpdated,
			"tasks_total":   progress.TasksTotal,
		},
	})
}

// HandleGetDashboard handles GET /api/v1/swarm/dashboard (Enterprise).
func (h *SwarmHandler) HandleGetDashboard(w http.ResponseWriter, r *http.Request) {
	cluster := h.cluster()
	if cluster == nil {
		WriteError(w, http.StatusConflict, "SWARM_NOT_ACTIVE", "Swarm mode is not active")
		return
	}

	disc := h.discovery()

	// Cluster summary.
	serviceCount := 0
	taskCount := 0
	healthyTaskCount := 0
	services := make([]map[string]interface{}, 0)

	if disc != nil {
		svcList := disc.ListServices()
		serviceCount = len(svcList)
		for _, svc := range svcList {
			taskCount += svc.DesiredReplicas
			healthyTaskCount += svc.RunningReplicas

			entry := map[string]interface{}{
				"service_id":       svc.ServiceID,
				"name":             svc.Name,
				"mode":             svc.Mode,
				"desired_replicas": svc.DesiredReplicas,
				"running_replicas": svc.RunningReplicas,
				"update_state":     nil,
				"crash_loop":       false,
			}
			if svc.UpdateStatus != nil {
				entry["update_state"] = svc.UpdateStatus.State
			}
			if h.crashLoop != nil && h.crashLoop.IsCrashLooping(svc.ServiceID) {
				entry["crash_loop"] = true
			}
			services = append(services, entry)
		}
	}

	// Nodes.
	nodeResults := make([]map[string]interface{}, 0)
	if h.nodeStore != nil {
		nodes, err := h.nodeStore.ListNodes(r.Context())
		if err == nil {
			for _, n := range nodes {
				nodeResults = append(nodeResults, map[string]interface{}{
					"node_id":      n.NodeID,
					"hostname":     n.Hostname,
					"role":         n.Role,
					"status":       n.Status,
					"availability": n.Availability,
					"task_count":   n.TaskCount,
				})
			}
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"cluster": map[string]interface{}{
			"cluster_id":        cluster.ID,
			"manager_count":     cluster.ManagerCount,
			"worker_count":      cluster.WorkerCount,
			"service_count":     serviceCount,
			"task_count":        taskCount,
			"healthy_task_count": healthyTaskCount,
		},
		"nodes":         nodeResults,
		"services":      services,
		"recent_events": []interface{}{},
	})
}

func nodeToJSON(n *swarm.SwarmNode) map[string]interface{} {
	return map[string]interface{}{
		"id":                    n.ID,
		"node_id":              n.NodeID,
		"hostname":             n.Hostname,
		"role":                 n.Role,
		"status":               n.Status,
		"availability":         n.Availability,
		"engine_version":       n.EngineVersion,
		"address":              n.Address,
		"task_count":           n.TaskCount,
		"first_seen_at":        n.FirstSeenAt.Format(time.RFC3339),
		"last_seen_at":         n.LastSeenAt.Format(time.RFC3339),
		"last_status_change_at": n.LastStatusChangeAt.Format(time.RFC3339),
	}
}

func serviceToJSON(s *swarm.SwarmService) map[string]interface{} {
	networks := make([]map[string]interface{}, 0, len(s.Networks))
	for _, n := range s.Networks {
		networks = append(networks, map[string]interface{}{
			"network_id":   n.NetworkID,
			"network_name": n.NetworkName,
			"scope":        n.Scope,
		})
	}

	ports := make([]map[string]interface{}, 0, len(s.Ports))
	for _, p := range s.Ports {
		ports = append(ports, map[string]interface{}{
			"protocol":       p.Protocol,
			"target_port":    p.TargetPort,
			"published_port": p.PublishedPort,
			"publish_mode":   p.PublishMode,
		})
	}

	resp := map[string]interface{}{
		"service_id":       s.ServiceID,
		"name":             s.Name,
		"image":            s.Image,
		"mode":             s.Mode,
		"desired_replicas": s.DesiredReplicas,
		"running_replicas": s.RunningReplicas,
		"stack_name":       s.StackName,
		"networks":         networks,
		"ports":            ports,
		"labels":           s.Labels,
		"created_at":       s.CreatedAt.Format(time.RFC3339),
	}

	if s.UpdateStatus != nil {
		us := map[string]interface{}{
			"state":   s.UpdateStatus.State,
			"message": s.UpdateStatus.Message,
		}
		if s.UpdateStatus.StartedAt != nil {
			us["started_at"] = s.UpdateStatus.StartedAt.Format(time.RFC3339)
		}
		if s.UpdateStatus.CompletedAt != nil {
			us["completed_at"] = s.UpdateStatus.CompletedAt.Format(time.RFC3339)
		}
		resp["update_status"] = us
	}

	return resp
}

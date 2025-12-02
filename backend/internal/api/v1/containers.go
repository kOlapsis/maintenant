package v1

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/kolapsis/pulseboard/internal/container"
)

// ContainerNameLister lists container names in a K8s workload pod spec.
type ContainerNameLister interface {
	ListContainerNames(ctx context.Context, externalID string) ([]string, error)
}

// ContainerHandler handles container-related HTTP endpoints.
type ContainerHandler struct {
	service         *container.Service
	uptime          *container.UptimeCalculator
	logFetcher      LogFetcher
	containerLister ContainerNameLister
}

// NewContainerHandler creates a new container handler.
func NewContainerHandler(service *container.Service, uptime *container.UptimeCalculator) *ContainerHandler {
	return &ContainerHandler{service: service, uptime: uptime}
}

// SetLogFetcher sets the log fetcher for the logs endpoint.
func (h *ContainerHandler) SetLogFetcher(lf LogFetcher) {
	h.logFetcher = lf
}

// SetContainerNameLister sets the K8s container name lister for detail endpoints.
func (h *ContainerHandler) SetContainerNameLister(cl ContainerNameLister) {
	h.containerLister = cl
}

// HandleList handles GET /api/v1/containers.
func (h *ContainerHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	opts := container.ListContainersOpts{}

	if r.URL.Query().Get("archived") == "true" {
		opts.IncludeArchived = true
	}
	if g := r.URL.Query().Get("group"); g != "" {
		opts.GroupFilter = g
	}
	if s := r.URL.Query().Get("state"); s != "" {
		opts.StateFilter = s
	}

	groups, total, archivedCount, err := h.service.ListContainersGrouped(r.Context(), opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list containers")
		return
	}

	if groups == nil {
		groups = []*container.ContainerGroup{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"groups":         groups,
		"total":          total,
		"archived_count": archivedCount,
	})
}

// HandleGet handles GET /api/v1/containers/{id}.
func (h *ContainerHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID must be an integer")
		return
	}

	c, err := h.service.GetContainer(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get container")
		return
	}
	if c == nil {
		WriteError(w, http.StatusNotFound, "CONTAINER_NOT_FOUND", "Container not found")
		return
	}

	// Build detail response with uptime
	detail := map[string]interface{}{
		"id":                   c.ID,
		"external_id":          c.ExternalID,
		"name":                 c.Name,
		"image":                c.Image,
		"state":                c.State,
		"health_status":        c.HealthStatus,
		"has_health_check":     c.HasHealthCheck,
		"orchestration_group":  c.OrchestrationGroup,
		"orchestration_unit":   c.OrchestrationUnit,
		"custom_group":         c.CustomGroup,
		"is_ignored":           c.IsIgnored,
		"alert_severity":       c.AlertSeverity,
		"restart_threshold":    c.RestartThreshold,
		"alert_channels":       c.AlertChannels,
		"archived":             c.Archived,
		"first_seen_at":        c.FirstSeenAt,
		"last_state_change_at": c.LastStateChangeAt,
		"archived_at":          c.ArchivedAt,
		"runtime_type":         c.RuntimeType,
		"error_detail":         c.ErrorDetail,
		"controller_kind":      c.ControllerKind,
		"namespace":            c.Namespace,
		"pod_count":            c.PodCount,
		"ready_count":          c.ReadyCount,
	}

	// Add uptime if calculator is available
	if h.uptime != nil {
		uptimeResult, err := h.uptime.Calculate(r.Context(), c.ID, false)
		if err == nil && uptimeResult != nil {
			detail["uptime"] = uptimeResult
		}
	}

	// For K8s workloads, include container names from pod spec
	if c.RuntimeType == "kubernetes" && h.containerLister != nil {
		names, err := h.containerLister.ListContainerNames(r.Context(), c.ExternalID)
		if err == nil && len(names) > 0 {
			detail["container_names"] = names
		}
	}

	WriteJSON(w, http.StatusOK, detail)
}

// HandleTransitions handles GET /api/v1/containers/{id}/transitions.
func (h *ContainerHandler) HandleTransitions(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID must be an integer")
		return
	}

	opts := container.ListTransitionsOpts{
		Limit: 50,
	}

	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err == nil {
			opts.Since = &t
		}
	} else {
		since := time.Now().Add(-24 * time.Hour)
		opts.Since = &since
	}

	if u := r.URL.Query().Get("until"); u != "" {
		t, err := time.Parse(time.RFC3339, u)
		if err == nil {
			opts.Until = &t
		}
	}

	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			opts.Limit = n
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			opts.Offset = n
		}
	}

	transitions, total, err := h.service.ListTransitions(r.Context(), id, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list transitions")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id": id,
		"transitions":  transitions,
		"total":        total,
		"has_more":     opts.Offset+len(transitions) < total,
	})
}

// LogFetcher abstracts Docker log retrieval for the API layer.
type LogFetcher interface {
	FetchLogs(ctx context.Context, containerID string, lines int, timestamps bool) ([]string, error)
}

// HandleLogs handles GET /api/v1/containers/{id}/logs.
func (h *ContainerHandler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID must be an integer")
		return
	}

	c, err := h.service.GetContainer(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get container")
		return
	}
	if c == nil {
		WriteError(w, http.StatusNotFound, "CONTAINER_NOT_FOUND", "Container not found")
		return
	}

	lines := 100
	if l := r.URL.Query().Get("lines"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			lines = n
			if lines > 500 {
				lines = 500
			}
		}
	}

	timestamps := r.URL.Query().Get("timestamps") == "true"

	if h.logFetcher == nil {
		WriteError(w, http.StatusBadGateway, "RUNTIME_UNAVAILABLE",
			"Cannot connect to container runtime for log retrieval.")
		return
	}

	logLines, err := h.logFetcher.FetchLogs(r.Context(), c.ExternalID, lines, timestamps)
	if err != nil {
		WriteError(w, http.StatusBadGateway, "LOGS_UNAVAILABLE", "Cannot retrieve logs from Docker")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id":   c.ID,
		"container_name": c.Name,
		"lines":          logLines,
		"total_lines":    len(logLines),
		"truncated":      len(logLines) >= lines,
	})
}

package v1

import (
	"net/http"
	"strconv"
	"time"

	"github.com/kolapsis/pulseboard/internal/container"
	"github.com/kolapsis/pulseboard/internal/endpoint"
)

// EndpointHandler handles endpoint-related HTTP endpoints.
type EndpointHandler struct {
	service      *endpoint.Service
	containerSvc *container.Service
}

// NewEndpointHandler creates a new endpoint handler.
func NewEndpointHandler(service *endpoint.Service, containerSvc *container.Service) *EndpointHandler {
	return &EndpointHandler{service: service, containerSvc: containerSvc}
}

// HandleListEndpoints handles GET /api/v1/endpoints.
func (h *EndpointHandler) HandleListEndpoints(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := endpoint.ListEndpointsOpts{
		Status:             q.Get("status"),
		ContainerName:      q.Get("container"),
		OrchestrationGroup: q.Get("orchestration_group"),
		EndpointType:       q.Get("type"),
		IncludeInactive:    q.Get("include_inactive") == "true",
	}

	endpoints, err := h.service.ListEndpoints(r.Context(), opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list endpoints")
		return
	}

	if endpoints == nil {
		endpoints = []*endpoint.Endpoint{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"endpoints": endpoints,
		"total":     len(endpoints),
	})
}

// HandleGetEndpoint handles GET /api/v1/endpoints/{id}.
func (h *EndpointHandler) HandleGetEndpoint(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Endpoint ID must be an integer")
		return
	}

	ep, err := h.service.GetEndpoint(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get endpoint")
		return
	}
	if ep == nil {
		WriteError(w, http.StatusNotFound, "ENDPOINT_NOT_FOUND", "Endpoint not found")
		return
	}

	uptime := h.service.CalculateUptime(r.Context(), id)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"endpoint": ep,
		"uptime":   uptime,
	})
}

// HandleListContainerEndpoints handles GET /api/v1/containers/{id}/endpoints.
func (h *EndpointHandler) HandleListContainerEndpoints(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID must be an integer")
		return
	}

	c, err := h.containerSvc.GetContainer(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get container")
		return
	}
	if c == nil {
		WriteError(w, http.StatusNotFound, "CONTAINER_NOT_FOUND", "Container not found")
		return
	}

	endpoints, err := h.service.ListEndpoints(r.Context(), endpoint.ListEndpointsOpts{
		ContainerName: c.Name,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list endpoints")
		return
	}

	var summary []map[string]interface{}
	for _, ep := range endpoints {
		summary = append(summary, map[string]interface{}{
			"id":                    ep.ID,
			"endpoint_type":         ep.EndpointType,
			"target":                ep.Target,
			"status":                ep.Status,
			"last_response_time_ms": ep.LastResponseTimeMs,
			"last_check_at":         ep.LastCheckAt,
		})
	}

	if summary == nil {
		summary = []map[string]interface{}{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id": id,
		"endpoints":    summary,
		"total":        len(summary),
	})
}

// HandleListChecks handles GET /api/v1/endpoints/{id}/checks.
func (h *EndpointHandler) HandleListChecks(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Endpoint ID must be an integer")
		return
	}

	q := r.URL.Query()
	opts := endpoint.ListChecksOpts{
		Limit:  50,
		Offset: 0,
	}

	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			opts.Limit = n
		}
	}
	if o := q.Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			opts.Offset = n
		}
	}
	if s := q.Get("since"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			t := time.Unix(ts, 0)
			opts.Since = &t
		}
	}

	checks, total, err := h.service.ListCheckResults(r.Context(), id, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list check results")
		return
	}

	if checks == nil {
		checks = []*endpoint.CheckResult{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"endpoint_id": id,
		"checks":      checks,
		"total":       total,
		"has_more":    opts.Offset+len(checks) < total,
	})
}

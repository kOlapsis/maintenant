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
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/endpoint"
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
		Source:             q.Get("source"),
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

// createEndpointInput is the JSON body for POST /api/v1/endpoints.
type createEndpointInput struct {
	Name         string            `json:"name"`
	Target       string            `json:"target"`
	EndpointType string            `json:"endpoint_type"`
	Interval     string            `json:"interval,omitempty"`
	Timeout      string            `json:"timeout,omitempty"`
	Method       string            `json:"method,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
}

// HandleCreateEndpoint handles POST /api/v1/endpoints.
func (h *EndpointHandler) HandleCreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var input createEndpointInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Target = strings.TrimSpace(input.Target)

	if input.Name == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "name is required")
		return
	}
	if input.Target == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "target is required")
		return
	}

	epType := endpoint.EndpointType(input.EndpointType)
	if epType != endpoint.TypeHTTP && epType != endpoint.TypeTCP {
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "endpoint_type must be 'http' or 'tcp'")
		return
	}

	if epType == endpoint.TypeHTTP {
		if _, err := url.ParseRequestURI(input.Target); err != nil {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "target must be a valid URL for HTTP endpoints")
			return
		}
	}

	config := endpoint.DefaultConfig()
	if input.Interval != "" {
		d, err := time.ParseDuration(input.Interval)
		if err != nil || d < 5*time.Second {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "interval must be a valid duration >= 5s")
			return
		}
		config.Interval = d
	}
	if input.Timeout != "" {
		d, err := time.ParseDuration(input.Timeout)
		if err != nil || d < 1*time.Second {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "timeout must be a valid duration >= 1s")
			return
		}
		config.Timeout = d
	}
	if config.Timeout >= config.Interval {
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "timeout must be less than interval")
		return
	}
	if input.Method != "" {
		config.Method = strings.ToUpper(input.Method)
	}
	if input.Headers != nil {
		config.Headers = input.Headers
	}

	ep, err := h.service.CreateStandalone(r.Context(), input.Name, input.Target, epType, config)
	if err != nil {
		if errors.Is(err, endpoint.ErrLimitReached) {
			WriteError(w, http.StatusForbidden, "QUOTA_EXCEEDED",
				"Community edition is limited to 10 endpoints. Upgrade to Pro for unlimited monitoring.")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create endpoint")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"endpoint": ep,
	})
}

// updateEndpointInput is the JSON body for PUT /api/v1/endpoints/{id}.
type updateEndpointInput struct {
	Name         string            `json:"name,omitempty"`
	Target       string            `json:"target,omitempty"`
	EndpointType string            `json:"endpoint_type,omitempty"`
	Interval     string            `json:"interval,omitempty"`
	Timeout      string            `json:"timeout,omitempty"`
	Method       string            `json:"method,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
}

// HandleUpdateEndpoint handles PUT /api/v1/endpoints/{id}.
func (h *EndpointHandler) HandleUpdateEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Endpoint ID must be an integer")
		return
	}

	var input updateEndpointInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	// Get current endpoint to merge with input
	existing, err := h.service.GetEndpoint(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get endpoint")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "ENDPOINT_NOT_FOUND", "Endpoint not found")
		return
	}

	name := existing.Name
	if input.Name != "" {
		name = strings.TrimSpace(input.Name)
	}
	target := existing.Target
	if input.Target != "" {
		target = strings.TrimSpace(input.Target)
	}
	epType := existing.EndpointType
	if input.EndpointType != "" {
		epType = endpoint.EndpointType(input.EndpointType)
		if epType != endpoint.TypeHTTP && epType != endpoint.TypeTCP {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "endpoint_type must be 'http' or 'tcp'")
			return
		}
	}

	config := existing.Config
	if input.Interval != "" {
		d, err := time.ParseDuration(input.Interval)
		if err != nil || d < 5*time.Second {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "interval must be a valid duration >= 5s")
			return
		}
		config.Interval = d
	}
	if input.Timeout != "" {
		d, err := time.ParseDuration(input.Timeout)
		if err != nil || d < 1*time.Second {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "timeout must be a valid duration >= 1s")
			return
		}
		config.Timeout = d
	}
	if config.Timeout >= config.Interval {
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "timeout must be less than interval")
		return
	}
	if input.Method != "" {
		config.Method = strings.ToUpper(input.Method)
	}
	if input.Headers != nil {
		config.Headers = input.Headers
	}

	ep, err := h.service.UpdateStandalone(r.Context(), id, name, target, epType, config)
	if err != nil {
		if errors.Is(err, endpoint.ErrEndpointNotFound) {
			WriteError(w, http.StatusNotFound, "ENDPOINT_NOT_FOUND", "Endpoint not found")
			return
		}
		if errors.Is(err, endpoint.ErrNotStandalone) {
			WriteError(w, http.StatusBadRequest, "NOT_STANDALONE",
				"Only standalone endpoints can be updated; label-discovered endpoints are managed via container labels")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update endpoint")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"endpoint": ep,
	})
}

// HandleDeleteEndpoint handles DELETE /api/v1/endpoints/{id}.
func (h *EndpointHandler) HandleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Endpoint ID must be an integer")
		return
	}

	if err := h.service.DeleteStandalone(r.Context(), id); err != nil {
		if errors.Is(err, endpoint.ErrEndpointNotFound) {
			WriteError(w, http.StatusNotFound, "ENDPOINT_NOT_FOUND", "Endpoint not found")
			return
		}
		if errors.Is(err, endpoint.ErrNotStandalone) {
			WriteError(w, http.StatusBadRequest, "NOT_STANDALONE",
				"Only standalone endpoints can be deleted; label-discovered endpoints are managed via container labels")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete endpoint")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

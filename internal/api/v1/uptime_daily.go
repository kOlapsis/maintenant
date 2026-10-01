// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/kolapsis/maintenant/internal/store"
)

// UptimeDailyFetcher abstracts the daily uptime store for testing.
type UptimeDailyFetcher interface {
	GetEndpointDailyUptime(ctx context.Context, endpointID string, days int, now time.Time) ([]store.DailyUptime, error)
	GetHeartbeatDailyUptime(ctx context.Context, heartbeatID string, days int, now time.Time) ([]store.DailyUptime, error)
	GetContainerDailyUptime(ctx context.Context, containerID string, days int, now time.Time) ([]store.DailyUptime, error)
}

// UptimeDailyHandler handles daily uptime aggregation endpoints.
type UptimeDailyHandler struct {
	store UptimeDailyFetcher
}

// NewUptimeDailyHandler creates a new daily uptime handler.
func NewUptimeDailyHandler(store UptimeDailyFetcher) *UptimeDailyHandler {
	return &UptimeDailyHandler{store: store}
}

// HandleEndpointDailyUptime handles GET /api/v1/endpoints/{id}/uptime/daily.
func (h *UptimeDailyHandler) HandleEndpointDailyUptime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Endpoint ID is required")
		return
	}

	days := parseDaysParam(r)

	results, err := h.store.GetEndpointDailyUptime(r.Context(), id, days, time.Now())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch daily uptime")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"monitor_id":   id,
		"monitor_type": "endpoint",
		"days":         results,
	})
}

// HandleHeartbeatDailyUptime handles GET /api/v1/heartbeats/{id}/uptime/daily.
func (h *UptimeDailyHandler) HandleHeartbeatDailyUptime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Heartbeat ID is required")
		return
	}

	days := parseDaysParam(r)

	results, err := h.store.GetHeartbeatDailyUptime(r.Context(), id, days, time.Now())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch daily uptime")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"monitor_id":   id,
		"monitor_type": "heartbeat",
		"days":         results,
	})
}

// HandleContainerDailyUptime handles GET /api/v1/containers/{id}/uptime/daily.
func (h *UptimeDailyHandler) HandleContainerDailyUptime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID is required")
		return
	}

	days := parseDaysParam(r)

	results, err := h.store.GetContainerDailyUptime(r.Context(), id, days, time.Now())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch daily uptime")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"monitor_id":   id,
		"monitor_type": "container",
		"days":         results,
	})
}

// parseDaysParam parses the "days" query parameter with default=90, max=365.
func parseDaysParam(r *http.Request) int {
	days := 90
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			days = n
			if days > 365 {
				days = 365
			}
		}
	}
	return days
}

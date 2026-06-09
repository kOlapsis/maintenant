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
	"strconv"

	"github.com/kolapsis/maintenant/internal/store/sqlite"
)

// UptimeDailyFetcher abstracts the daily uptime store for testing.
type UptimeDailyFetcher interface {
	GetEndpointDailyUptime(ctx context.Context, endpointID string, days int) ([]sqlite.DailyUptime, error)
	GetHeartbeatDailyUptime(ctx context.Context, heartbeatID string, days int) ([]sqlite.DailyUptime, error)
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

	results, err := h.store.GetEndpointDailyUptime(r.Context(), id, days)
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

	results, err := h.store.GetHeartbeatDailyUptime(r.Context(), id, days)
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

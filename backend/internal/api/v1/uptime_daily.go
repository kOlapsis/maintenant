package v1

import (
	"context"
	"net/http"
	"strconv"

	"github.com/kolapsis/pulseboard/internal/store/sqlite"
)

// UptimeDailyFetcher abstracts the daily uptime store for testing.
type UptimeDailyFetcher interface {
	GetEndpointDailyUptime(ctx context.Context, endpointID int64, days int) ([]sqlite.DailyUptime, error)
	GetHeartbeatDailyUptime(ctx context.Context, heartbeatID int64, days int) ([]sqlite.DailyUptime, error)
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
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Endpoint ID must be an integer")
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
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Heartbeat ID must be an integer")
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

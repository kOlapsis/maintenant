package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kolapsis/pulseboard/internal/heartbeat"
)

// HeartbeatHandler handles heartbeat CRUD endpoints.
type HeartbeatHandler struct {
	svc *heartbeat.Service
}

// NewHeartbeatHandler creates a new heartbeat handler.
func NewHeartbeatHandler(svc *heartbeat.Service) *HeartbeatHandler {
	return &HeartbeatHandler{svc: svc}
}

// HandleList handles GET /api/v1/heartbeats
func (h *HeartbeatHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	opts := heartbeat.ListHeartbeatsOpts{
		Status: r.URL.Query().Get("status"),
	}

	heartbeats, err := h.svc.ListHeartbeats(r.Context(), opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list heartbeats")
		return
	}

	if heartbeats == nil {
		heartbeats = []*heartbeat.Heartbeat{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"heartbeats": heartbeats,
		"total":      len(heartbeats),
	})
}

// HandleGet handles GET /api/v1/heartbeats/{id}
func (h *HeartbeatHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid heartbeat ID")
		return
	}

	hb, err := h.svc.GetHeartbeat(r.Context(), id)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Heartbeat not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get heartbeat")
		return
	}

	response := map[string]interface{}{
		"heartbeat": hb,
	}

	// Include snippets
	baseURL := h.svc.BaseURL()
	if baseURL != "" {
		response["snippets"] = heartbeat.GenerateSnippets(baseURL, hb.UUID)
	}

	WriteJSON(w, http.StatusOK, response)
}

// HandleCreate handles POST /api/v1/heartbeats
func (h *HeartbeatHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input heartbeat.CreateHeartbeatInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	newUUID := uuid.New().String()
	hb, err := h.svc.CreateHeartbeat(r.Context(), input, newUUID)
	if err != nil {
		if errors.Is(err, heartbeat.ErrLimitReached) {
			WriteError(w, http.StatusConflict, "HEARTBEAT_LIMIT_REACHED",
				"Community edition allows up to 10 heartbeat monitors. Upgrade to Pro for unlimited monitors.")
			return
		}
		if errors.Is(err, heartbeat.ErrInvalidInput) {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create heartbeat")
		return
	}

	WriteJSON(w, http.StatusCreated, hb)
}

// HandleUpdate handles PUT /api/v1/heartbeats/{id}
func (h *HeartbeatHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid heartbeat ID")
		return
	}

	var input heartbeat.UpdateHeartbeatInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	hb, err := h.svc.UpdateHeartbeat(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Heartbeat not found")
			return
		}
		if errors.Is(err, heartbeat.ErrInvalidInput) {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update heartbeat")
		return
	}

	WriteJSON(w, http.StatusOK, hb)
}

// HandleDelete handles DELETE /api/v1/heartbeats/{id}
func (h *HeartbeatHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid heartbeat ID")
		return
	}

	if err := h.svc.DeleteHeartbeat(r.Context(), id); err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Heartbeat not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete heartbeat")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandlePause handles POST /api/v1/heartbeats/{id}/pause
func (h *HeartbeatHandler) HandlePause(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid heartbeat ID")
		return
	}

	hb, err := h.svc.PauseHeartbeat(r.Context(), id)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Heartbeat not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to pause heartbeat")
		return
	}

	WriteJSON(w, http.StatusOK, hb)
}

// HandleResume handles POST /api/v1/heartbeats/{id}/resume
func (h *HeartbeatHandler) HandleResume(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid heartbeat ID")
		return
	}

	hb, err := h.svc.ResumeHeartbeat(r.Context(), id)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Heartbeat not found")
			return
		}
		if errors.Is(err, heartbeat.ErrInvalidInput) {
			WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to resume heartbeat")
		return
	}

	WriteJSON(w, http.StatusOK, hb)
}

// HandleListExecutions handles GET /api/v1/heartbeats/{id}/executions
func (h *HeartbeatHandler) HandleListExecutions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid heartbeat ID")
		return
	}

	// Verify heartbeat exists
	if _, err := h.svc.GetHeartbeat(r.Context(), id); err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Heartbeat not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get heartbeat")
		return
	}

	opts := heartbeat.ListExecutionsOpts{}
	if v := r.URL.Query().Get("limit"); v != "" {
		opts.Limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		opts.Offset, _ = strconv.Atoi(v)
	}

	executions, total, err := h.svc.ListExecutions(r.Context(), id, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list executions")
		return
	}

	if executions == nil {
		executions = []*heartbeat.HeartbeatExecution{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"executions": executions,
		"total":      total,
	})
}

// EnrichedPing extends HeartbeatPing with computed timing fields.
type EnrichedPing struct {
	*heartbeat.HeartbeatPing
	ExpectedAt    *time.Time `json:"expected_at,omitempty"`
	GraceDeadline *time.Time `json:"grace_deadline,omitempty"`
}

// HandleListPings handles GET /api/v1/heartbeats/{id}/pings
func (h *HeartbeatHandler) HandleListPings(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid heartbeat ID")
		return
	}

	// Get heartbeat to access interval and grace period for enrichment.
	hb, err := h.svc.GetHeartbeat(r.Context(), id)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "Heartbeat not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get heartbeat")
		return
	}

	opts := heartbeat.ListPingsOpts{}
	if v := r.URL.Query().Get("limit"); v != "" {
		opts.Limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		opts.Offset, _ = strconv.Atoi(v)
	}

	pings, total, err := h.svc.ListPings(r.Context(), id, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list pings")
		return
	}

	if pings == nil {
		pings = []*heartbeat.HeartbeatPing{}
	}

	// Enrich pings with expected_at and grace_deadline.
	enriched := enrichPings(pings, hb.IntervalSeconds, hb.GraceSeconds, hb.CreatedAt)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"pings": enriched,
		"total": total,
	})
}

// enrichPings computes expected_at and grace_deadline for each ping.
// Pings are ordered most recent first. We walk backwards from oldest
// to newest, computing expected times based on the heartbeat interval.
func enrichPings(pings []*heartbeat.HeartbeatPing, intervalSec, graceSec int, createdAt time.Time) []EnrichedPing {
	if len(pings) == 0 {
		return []EnrichedPing{}
	}

	interval := time.Duration(intervalSec) * time.Second
	grace := time.Duration(graceSec) * time.Second

	enriched := make([]EnrichedPing, len(pings))

	// Walk from oldest (last element) to newest (first element).
	// The first expected_at is createdAt + interval, or the oldest ping's timestamp
	// (whichever creates the most sensible timeline).
	for i := len(pings) - 1; i >= 0; i-- {
		enriched[i] = EnrichedPing{HeartbeatPing: pings[i]}

		var expectedAt time.Time
		if i == len(pings)-1 {
			// Oldest ping: expected_at is based on heartbeat creation or prior ping
			// Use the ping's own timestamp as the anchor, rounded to interval boundary.
			expectedAt = pings[i].Timestamp.Truncate(interval)
			if expectedAt.Before(pings[i].Timestamp) && expectedAt.Add(interval).After(createdAt) {
				expectedAt = expectedAt.Add(0) // keep as-is
			}
		} else {
			// Subsequent pings: expected_at = previous ping's timestamp + interval
			prevPing := pings[i+1]
			expectedAt = prevPing.Timestamp.Add(interval)
		}

		ea := expectedAt
		enriched[i].ExpectedAt = &ea
		gd := expectedAt.Add(grace)
		enriched[i].GraceDeadline = &gd
	}

	return enriched
}

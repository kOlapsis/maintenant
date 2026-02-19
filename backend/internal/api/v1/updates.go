package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kolapsis/pulseboard/internal/update"
)

// UpdateHandler handles update intelligence HTTP endpoints.
type UpdateHandler struct {
	service *update.Service
	store   update.UpdateStore
}

// NewUpdateHandler creates a new update handler.
func NewUpdateHandler(service *update.Service, store update.UpdateStore) *UpdateHandler {
	return &UpdateHandler{service: service, store: store}
}

// HandleListUpdates handles GET /api/v1/updates.
func (h *UpdateHandler) HandleListUpdates(w http.ResponseWriter, r *http.Request) {
	opts := update.ListImageUpdatesOpts{
		Status:     r.URL.Query().Get("status"),
		UpdateType: r.URL.Query().Get("update_type"),
	}

	updates, err := h.service.ListImageUpdates(r.Context(), opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list updates")
		return
	}

	updateMaps := make([]map[string]interface{}, 0, len(updates))
	for _, u := range updates {
		updateMaps = append(updateMaps, imageUpdateToMap(u))
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"updates":   updateMaps,
		"last_scan": h.service.GetLastScanTime(),
		"next_scan": h.service.GetNextScanTime(),
	})
}

// HandleGetUpdateSummary handles GET /api/v1/updates/summary.
func (h *UpdateHandler) HandleGetUpdateSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.GetUpdateSummary(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get update summary")
		return
	}

	// Determine scan status
	scanStatus := "unknown"
	latest, _ := h.service.GetLatestScanRecord(r.Context())
	if latest != nil {
		scanStatus = string(latest.Status)
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"last_scan":   h.service.GetLastScanTime(),
		"next_scan":   h.service.GetNextScanTime(),
		"scan_status": scanStatus,
		"counts":      summary,
	})
}

// HandleGetContainerUpdate handles GET /api/v1/updates/{container_id}.
func (h *UpdateHandler) HandleGetContainerUpdate(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("container_id")
	if containerID == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Missing container_id")
		return
	}

	u, err := h.service.GetImageUpdateByContainer(r.Context(), containerID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get update")
		return
	}
	if u == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "No update data for container")
		return
	}

	// Check if pinned
	pin, _ := h.store.GetVersionPin(r.Context(), containerID)

	resp := imageUpdateToMap(u)
	resp["pinned"] = pin != nil

	WriteJSON(w, http.StatusOK, resp)
}

// HandleTriggerScan handles POST /api/v1/updates/scan.
func (h *UpdateHandler) HandleTriggerScan(w http.ResponseWriter, r *http.Request) {
	if h.service.IsScanning() {
		WriteError(w, http.StatusConflict, "SCAN_IN_PROGRESS", "A scan is already running")
		return
	}

	_, err := h.service.TriggerScan(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to trigger scan")
		return
	}

	WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":     "running",
		"started_at": time.Now(),
	})
}

// HandleGetScanStatus handles GET /api/v1/updates/scan/{scan_id}.
func (h *UpdateHandler) HandleGetScanStatus(w http.ResponseWriter, r *http.Request) {
	scanIDStr := r.PathValue("scan_id")
	scanID, err := strconv.ParseInt(scanIDStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid scan ID")
		return
	}

	record, err := h.service.GetScanRecord(r.Context(), scanID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get scan status")
		return
	}
	if record == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "Scan not found")
		return
	}

	resp := map[string]interface{}{
		"scan_id":            record.ID,
		"status":             string(record.Status),
		"started_at":         record.StartedAt,
		"containers_scanned": record.ContainersScanned,
		"updates_found":      record.UpdatesFound,
		"errors":             record.Errors,
	}
	if record.CompletedAt != nil {
		resp["completed_at"] = record.CompletedAt
	}

	WriteJSON(w, http.StatusOK, resp)
}

// HandleGetDryRun handles GET /api/v1/updates/dry-run.
func (h *UpdateHandler) HandleGetDryRun(w http.ResponseWriter, r *http.Request) {
	updates, err := h.store.ListImageUpdates(r.Context(), update.ListImageUpdatesOpts{Status: "available"})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get updates")
		return
	}

	wouldUpdate := make([]map[string]interface{}, 0, len(updates))
	for _, u := range updates {
		wouldUpdate = append(wouldUpdate, map[string]interface{}{
			"container_id":   u.ContainerID,
			"container_name": u.ContainerName,
			"image":          u.Image,
			"current_tag":    u.CurrentTag,
			"latest_tag":     u.LatestTag,
			"update_type":    string(u.UpdateType),
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"would_update": wouldUpdate,
	})
}

// HandlePinVersion handles POST /api/v1/updates/{container_id}/pin.
func (h *UpdateHandler) HandlePinVersion(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("container_id")
	if containerID == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Missing container_id")
		return
	}

	var input struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&input)
	}

	// Get current update info
	u, err := h.store.GetImageUpdateByContainer(r.Context(), containerID)
	if err != nil || u == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "No update data for container")
		return
	}

	pin := &update.VersionPin{
		ContainerID:  containerID,
		Image:        u.Image,
		PinnedTag:    u.CurrentTag,
		PinnedDigest: u.CurrentDigest,
		Reason:       input.Reason,
		PinnedAt:     time.Now(),
	}

	if _, err := h.store.InsertVersionPin(r.Context(), pin); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to pin version")
		return
	}

	// Update the image_updates status to pinned
	u.Status = update.UpdateStatusPinned
	h.store.UpdateImageUpdate(r.Context(), u)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id":  containerID,
		"pinned_tag":    pin.PinnedTag,
		"pinned_digest": pin.PinnedDigest,
		"reason":        pin.Reason,
		"pinned_at":     pin.PinnedAt,
	})
}

// HandleUnpinVersion handles DELETE /api/v1/updates/{container_id}/pin.
func (h *UpdateHandler) HandleUnpinVersion(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("container_id")
	if containerID == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Missing container_id")
		return
	}

	if err := h.store.DeleteVersionPin(r.Context(), containerID); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to unpin version")
		return
	}

	// Restore update status to available
	u, _ := h.store.GetImageUpdateByContainer(r.Context(), containerID)
	if u != nil {
		u.Status = update.UpdateStatusAvailable
		h.store.UpdateImageUpdate(r.Context(), u)
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListExclusions handles GET /api/v1/updates/exclusions.
func (h *UpdateHandler) HandleListExclusions(w http.ResponseWriter, r *http.Request) {
	exclusions, err := h.store.ListExclusions(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list exclusions")
		return
	}

	excMaps := make([]map[string]interface{}, 0, len(exclusions))
	for _, e := range exclusions {
		excMaps = append(excMaps, map[string]interface{}{
			"id":           e.ID,
			"pattern":      e.Pattern,
			"pattern_type": string(e.PatternType),
			"created_at":   e.CreatedAt,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"exclusions": excMaps,
	})
}

// HandleCreateExclusion handles POST /api/v1/updates/exclusions.
func (h *UpdateHandler) HandleCreateExclusion(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Pattern     string `json:"pattern"`
		PatternType string `json:"pattern_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if input.Pattern == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PATTERN", "Pattern is required")
		return
	}
	if input.PatternType != "image" && input.PatternType != "tag" {
		WriteError(w, http.StatusBadRequest, "INVALID_TYPE", "Pattern type must be 'image' or 'tag'")
		return
	}

	exc := &update.UpdateExclusion{
		Pattern:     input.Pattern,
		PatternType: update.ExclusionType(input.PatternType),
		CreatedAt:   time.Now(),
	}

	id, err := h.store.InsertExclusion(r.Context(), exc)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create exclusion")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":           id,
		"pattern":      exc.Pattern,
		"pattern_type": string(exc.PatternType),
		"created_at":   exc.CreatedAt,
	})
}

// HandleDeleteExclusion handles DELETE /api/v1/updates/exclusions/{id}.
func (h *UpdateHandler) HandleDeleteExclusion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Invalid exclusion ID")
		return
	}

	if err := h.store.DeleteExclusion(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete exclusion")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func imageUpdateToMap(u *update.ImageUpdate) map[string]interface{} {
	m := map[string]interface{}{
		"id":             u.ID,
		"container_id":   u.ContainerID,
		"container_name": u.ContainerName,
		"image":          u.Image,
		"current_tag":    u.CurrentTag,
		"current_digest": u.CurrentDigest,
		"latest_tag":     u.LatestTag,
		"latest_digest":  u.LatestDigest,
		"update_type":    string(u.UpdateType),
		"risk_score":     u.RiskScore,
		"status":         string(u.Status),
		"detected_at":    u.DetectedAt,
	}
	if u.PublishedAt != nil {
		m["published_at"] = u.PublishedAt
	}
	return m
}

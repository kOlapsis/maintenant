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
	"time"

	"github.com/kolapsis/maintenant/internal/update"
)

// RiskHandler handles risk score HTTP endpoints.
type RiskHandler struct {
	store update.UpdateStore
}

// NewRiskHandler creates a new risk handler.
func NewRiskHandler(store update.UpdateStore) *RiskHandler {
	return &RiskHandler{store: store}
}

// HandleListRiskScores handles GET /api/v1/risk.
func (h *RiskHandler) HandleListRiskScores(w http.ResponseWriter, r *http.Request) {
	updates, err := h.store.ListImageUpdates(r.Context(), update.ListImageUpdatesOpts{})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "Failed to list risk scores")
		return
	}

	maxScore := 0
	containers := make([]map[string]interface{}, 0, len(updates))
	for _, u := range updates {
		level := update.RiskLevelFromScore(u.RiskScore)
		containers = append(containers, map[string]interface{}{
			"container_id":   u.ContainerID,
			"container_name": u.ContainerName,
			"risk_score":     u.RiskScore,
			"level":          string(level),
		})
		if u.RiskScore > maxScore {
			maxScore = u.RiskScore
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"containers":      containers,
		"host_risk_score": maxScore,
		"host_risk_level": string(update.RiskLevelFromScore(maxScore)),
	})
}

// HandleGetContainerRisk handles GET /api/v1/risk/{container_id...}.
// When the ?period= query param is present (24h, 7d, 30d), returns risk score history
// instead of the current risk snapshot.
func (h *RiskHandler) HandleGetContainerRisk(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("container_id")
	if containerID == "" {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Missing container_id")
		return
	}

	if period := r.URL.Query().Get("period"); period != "" {
		h.handleRiskHistory(w, r, containerID, period)
		return
	}

	u, err := h.store.GetImageUpdateByContainer(r.Context(), containerID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "Failed to get risk data")
		return
	}
	if u == nil {
		WriteError(w, http.StatusNotFound, "not_found", "No risk data for container")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id":   u.ContainerID,
		"container_name": u.ContainerName,
		"risk_score":     u.RiskScore,
		"level":          string(update.RiskLevelFromScore(u.RiskScore)),
	})
}

func (h *RiskHandler) handleRiskHistory(w http.ResponseWriter, r *http.Request, containerID, period string) {
	now := time.Now()
	var from time.Time
	switch period {
	case "24h":
		from = now.Add(-24 * time.Hour)
	case "7d":
		from = now.Add(-7 * 24 * time.Hour)
	case "30d":
		from = now.Add(-30 * 24 * time.Hour)
	default:
		WriteError(w, http.StatusBadRequest, "invalid_period", "Period must be 24h, 7d, or 30d")
		return
	}

	history, err := h.store.ListRiskScoreHistory(r.Context(), containerID, from, now)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "Failed to get risk history")
		return
	}

	points := make([]map[string]interface{}, 0, len(history))
	for _, rec := range history {
		points = append(points, map[string]interface{}{
			"score":       rec.Score,
			"recorded_at": rec.RecordedAt,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"container_id": containerID,
		"history":      points,
	})
}

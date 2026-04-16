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
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/security"
)

// PostureHandler handles security posture HTTP endpoints.
type PostureHandler struct {
	scorer       *security.Scorer
	containerSvc *container.Service
	ackStore     security.AcknowledgmentStore
	alertStore   alert.AlertStore
	securitySvc  *security.Service
	broker       *SSEBroker
}

// NewPostureHandler creates a new posture handler.
func NewPostureHandler(scorer *security.Scorer, containerSvc *container.Service, ackStore security.AcknowledgmentStore, alertStore alert.AlertStore, securitySvc *security.Service, broker *SSEBroker) *PostureHandler {
	return &PostureHandler{
		scorer:       scorer,
		containerSvc: containerSvc,
		ackStore:     ackStore,
		alertStore:   alertStore,
		securitySvc:  securitySvc,
		broker:       broker,
	}
}

// HandleGetPosture handles GET /api/v1/security/posture.
func (h *PostureHandler) HandleGetPosture(w http.ResponseWriter, r *http.Request) {
	containers, err := h.containerSvc.ListContainers(r.Context(), container.ListContainersOpts{})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list containers")
		return
	}

	infos := make([]security.ContainerInfo, len(containers))
	for i, c := range containers {
		infos[i] = security.ContainerInfo{
			ID:         c.ID,
			ExternalID: c.ExternalID,
			Name:       c.Name,
		}
	}

	posture, err := h.scorer.ScoreInfrastructure(r.Context(), infos)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to compute posture")
		return
	}

	// Check threshold alerts after scoring
	h.scorer.CheckPostureThreshold(posture.Score, posture.ColorLevel)

	WriteJSON(w, http.StatusOK, posture)
}

// HandleGetContainerPosture handles GET /api/v1/security/posture/containers/{container_id}.
func (h *PostureHandler) HandleGetContainerPosture(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("container_id")
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
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "container not found")
		return
	}

	score, err := h.scorer.ScoreContainer(r.Context(), c.ID, c.ExternalID, c.Name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to compute score")
		return
	}
	if score == nil {
		WriteJSON(w, http.StatusOK, map[string]any{
			"container_id":   c.ID,
			"container_name": c.Name,
			"score":          0,
			"color":          "red",
			"is_partial":     true,
			"categories":     []any{},
			"computed_at":    time.Now().UTC(),
		})
		return
	}

	WriteJSON(w, http.StatusOK, score)
}

// HandleListContainerPostures handles GET /api/v1/security/posture/containers.
func (h *PostureHandler) HandleListContainerPostures(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	containers, err := h.containerSvc.ListContainers(r.Context(), container.ListContainersOpts{})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list containers")
		return
	}

	// Score all containers
	type scoredContainer struct {
		score *security.SecurityScore
	}
	var scored []scoredContainer
	for _, c := range containers {
		s, err := h.scorer.ScoreContainer(r.Context(), c.ID, c.ExternalID, c.Name)
		if err != nil || s == nil {
			continue
		}
		scored = append(scored, scoredContainer{score: s})
	}

	// Sort by score ascending (worst first)
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score.TotalScore < scored[i].score.TotalScore {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	total := len(scored)

	// Apply pagination
	if offset >= len(scored) {
		scored = nil
	} else {
		end := offset + limit
		if end > len(scored) {
			end = len(scored)
		}
		scored = scored[offset:end]
	}

	result := make([]*security.SecurityScore, len(scored))
	for i, s := range scored {
		result[i] = s.score
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"containers": result,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	})
}

// HandleCreateAcknowledgment handles POST /api/v1/security/acknowledgments.
func (h *PostureHandler) HandleCreateAcknowledgment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ContainerID    int64  `json:"container_id"`
		FindingType    string `json:"finding_type"`
		FindingKey     string `json:"finding_key"`
		AcknowledgedBy string `json:"acknowledged_by"`
		Reason         string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
		return
	}

	if req.ContainerID == 0 || req.FindingType == "" {
		WriteError(w, http.StatusBadRequest, "MISSING_FIELDS", "container_id and finding_type are required")
		return
	}

	// Resolve container external ID
	c, err := h.containerSvc.GetContainer(r.Context(), req.ContainerID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get container")
		return
	}
	if c == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "container not found")
		return
	}

	// Check if already acknowledged
	acked, err := h.ackStore.IsAcknowledged(r.Context(), c.ExternalID, req.FindingType, req.FindingKey)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to check acknowledgment")
		return
	}
	if acked {
		WriteError(w, http.StatusConflict, "ALREADY_ACKNOWLEDGED", "finding already acknowledged")
		return
	}

	ack := &security.RiskAcknowledgment{
		ContainerExternalID: c.ExternalID,
		FindingType:         req.FindingType,
		FindingKey:          req.FindingKey,
		AcknowledgedBy:      req.AcknowledgedBy,
		Reason:              req.Reason,
		AcknowledgedAt:      time.Now().UTC(),
	}

	id, err := h.ackStore.InsertAcknowledgment(r.Context(), ack)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			WriteError(w, http.StatusConflict, "ALREADY_ACKNOWLEDGED", "finding already acknowledged")
			return
		}
		slog.Error("failed to create acknowledgment", "error", err, "container_id", req.ContainerID, "finding_type", req.FindingType)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create acknowledgment")
		return
	}
	ack.ID = id

	// Invalidate cached score for this container
	h.scorer.InvalidateCache(c.ID)

	// If all security insights for this container are now acknowledged,
	// acknowledge the corresponding security alert.
	h.tryAcknowledgeSecurityAlert(r.Context(), c, req.AcknowledgedBy)

	WriteJSON(w, http.StatusCreated, ack)
}

// tryAcknowledgeSecurityAlert checks if all security insights for a container
// are acknowledged, and if so, acknowledges the active security alert.
func (h *PostureHandler) tryAcknowledgeSecurityAlert(ctx context.Context, c *container.Container, by string) {
	if h.alertStore == nil || h.securitySvc == nil {
		return
	}

	ci := h.securitySvc.GetContainerInsights(c.ID)
	if ci == nil || len(ci.Insights) == 0 {
		return
	}

	// Check if every insight is acknowledged
	for _, insight := range ci.Insights {
		key := security.InsightFindingKey(insight)
		acked, err := h.ackStore.IsAcknowledged(ctx, c.ExternalID, string(insight.Type), key)
		if err != nil || !acked {
			return
		}
	}

	// All insights are acknowledged — acknowledge the security alert
	a, err := h.alertStore.GetActiveAlert(ctx, alert.SourceSecurity, alert.AlertTypeDangerousConfig, "container", c.ID)
	if err != nil || a == nil || a.AcknowledgedAt != nil {
		return
	}

	now := time.Now().UTC()
	if err := h.alertStore.AcknowledgeAlert(ctx, a.ID, by, now); err != nil {
		return
	}
	a.AcknowledgedAt = &now
	a.AcknowledgedBy = by

	if h.broker != nil {
		h.broker.Broadcast(SSEEvent{Type: event.AlertAcknowledged, Data: a})
	}
}

// HandleDeleteAcknowledgment handles DELETE /api/v1/security/acknowledgments/{id}.
func (h *PostureHandler) HandleDeleteAcknowledgment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "ID must be an integer")
		return
	}

	ack, err := h.ackStore.GetAcknowledgment(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get acknowledgment")
		return
	}
	if ack == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "acknowledgment not found")
		return
	}

	if err := h.ackStore.DeleteAcknowledgment(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete acknowledgment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListAcknowledgments handles GET /api/v1/security/acknowledgments.
func (h *PostureHandler) HandleListAcknowledgments(w http.ResponseWriter, r *http.Request) {
	var containerExternalID string

	if cidStr := r.URL.Query().Get("container_id"); cidStr != "" {
		cid, err := strconv.ParseInt(cidStr, 10, 64)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "INVALID_ID", "container_id must be an integer")
			return
		}
		c, err := h.containerSvc.GetContainer(r.Context(), cid)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get container")
			return
		}
		if c != nil {
			containerExternalID = c.ExternalID
		}
	}

	acks, err := h.ackStore.ListAcknowledgments(r.Context(), containerExternalID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list acknowledgments")
		return
	}

	if acks == nil {
		acks = []*security.RiskAcknowledgment{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"acknowledgments": acks,
	})
}

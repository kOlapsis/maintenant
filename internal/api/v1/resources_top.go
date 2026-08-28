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

	"github.com/kolapsis/maintenant/internal/resource"
)

// ResourceTopService abstracts the resource service for top consumers.
type ResourceTopService interface {
	GetContainerName(containerID string) string
	TopConsumersNow(metric string, limit int, agentID *string) []resource.TopConsumerRow
	GetTopConsumersByPeriod(ctx context.Context, metric, period string, limit int, agentID *string) ([]resource.TopConsumerRow, error)
}

// ResourceTopHandler handles top resource consumers endpoints.
type ResourceTopHandler struct {
	svc ResourceTopService
}

// NewResourceTopHandler creates a new top consumers handler.
func NewResourceTopHandler(svc ResourceTopService) *ResourceTopHandler {
	return &ResourceTopHandler{svc: svc}
}

// TopConsumer represents a ranked container in the top consumers response.
type TopConsumer struct {
	ContainerID   string  `json:"container_id"`
	ContainerName string  `json:"container_name"`
	Value         float64 `json:"value"`
	Percent       float64 `json:"percent"`
	Rank          int     `json:"rank"`
}

// HandleGetTopConsumers handles GET /api/v1/resources/top?metric=cpu|memory&limit=5&period=<window>.
func (h *ResourceTopHandler) HandleGetTopConsumers(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	if metric != "cpu" && metric != "memory" {
		WriteError(w, http.StatusBadRequest, "INVALID_METRIC", "Metric must be 'cpu' or 'memory'")
		return
	}

	limit := 5
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
			if limit > 20 {
				limit = 20
			}
		}
	}

	hostFilter := parseHostFilter(r)

	// No period at all is the realtime ranking, open in every edition: the cap
	// is about history, not about live metrics (FR-024).
	period := r.URL.Query().Get("period")
	if period == "" {
		h.handleRealtimeQuery(w, metric, limit, hostFilter)
		return
	}

	// Same catalogue and same cap as the per-container chart, decided in the
	// same place. This endpoint served its 30d period to any edition until now
	// (FR-014): the refusal below is what closes that.
	window, ok := resolveHistoryWindow(w, period, "INVALID_PERIOD")
	if !ok {
		return
	}

	h.handlePeriodQuery(w, r, metric, window.Name, limit, hostFilter)
}

func (h *ResourceTopHandler) handlePeriodQuery(w http.ResponseWriter, r *http.Request, metric, period string, limit int, agentID *string) {
	rows, err := h.svc.GetTopConsumersByPeriod(r.Context(), metric, period, limit, agentID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch top consumers")
		return
	}

	consumers := make([]TopConsumer, 0, len(rows))
	for i, row := range rows {
		consumers = append(consumers, TopConsumer{
			ContainerID:   row.ContainerID,
			ContainerName: row.ContainerName,
			Value:         row.AvgValue,
			Percent:       row.AvgPercent,
			Rank:          i + 1,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"metric":    metric,
		"period":    period,
		"consumers": consumers,
	})
}

func (h *ResourceTopHandler) handleRealtimeQuery(w http.ResponseWriter, metric string, limit int, agentID *string) {
	rows := h.svc.TopConsumersNow(metric, limit, agentID)

	consumers := make([]TopConsumer, 0, len(rows))
	for i, row := range rows {
		consumers = append(consumers, TopConsumer{
			ContainerID:   row.ContainerID,
			ContainerName: row.ContainerName,
			Value:         row.AvgValue,
			Percent:       row.AvgPercent,
			Rank:          i + 1,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"metric":    metric,
		"consumers": consumers,
	})
}

package v1

import (
	"context"
	"net/http"
	"sort"
	"strconv"

	"github.com/kolapsis/maintenant/internal/resource"
)

// ResourceTopService abstracts the resource service for top consumers.
type ResourceTopService interface {
	GetAllLatestSnapshots() map[int64]*resource.ResourceSnapshot
	GetContainerName(containerID int64) string
	GetTopConsumersByPeriod(ctx context.Context, metric, period string, limit int) ([]resource.TopConsumerRow, error)
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
	ContainerID   int64   `json:"container_id"`
	ContainerName string  `json:"container_name"`
	Value         float64 `json:"value"`
	Percent       float64 `json:"percent"`
	Rank          int     `json:"rank"`
}

// HandleGetTopConsumers handles GET /api/v1/resources/top?metric=cpu|memory&limit=5&period=1h|24h|7d|30d.
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

	period := r.URL.Query().Get("period")
	if period == "1h" || period == "24h" || period == "7d" || period == "30d" {
		h.handlePeriodQuery(w, r, metric, period, limit)
		return
	}

	h.handleRealtimeQuery(w, metric, limit)
}

func (h *ResourceTopHandler) handlePeriodQuery(w http.ResponseWriter, r *http.Request, metric, period string, limit int) {
	rows, err := h.svc.GetTopConsumersByPeriod(r.Context(), metric, period, limit)
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

func (h *ResourceTopHandler) handleRealtimeQuery(w http.ResponseWriter, metric string, limit int) {
	all := h.svc.GetAllLatestSnapshots()

	type entry struct {
		id      int64
		value   float64
		percent float64
	}
	entries := make([]entry, 0, len(all))
	for cID, snap := range all {
		var value, pct float64
		switch metric {
		case "cpu":
			value = snap.CPUPercent
			pct = snap.CPUPercent
		case "memory":
			value = float64(snap.MemUsed)
			pct = 0.0
			if snap.MemLimit > 0 {
				pct = float64(snap.MemUsed) / float64(snap.MemLimit) * 100.0
			}
		}
		entries = append(entries, entry{id: cID, value: value, percent: pct})
	}

	sort.Slice(entries, func(i, j int) bool {
		if metric == "memory" {
			return entries[i].percent > entries[j].percent
		}
		return entries[i].value > entries[j].value
	})

	if limit > len(entries) {
		limit = len(entries)
	}
	consumers := make([]TopConsumer, 0, limit)
	for i := 0; i < limit; i++ {
		e := entries[i]
		name := h.svc.GetContainerName(e.id)
		consumers = append(consumers, TopConsumer{
			ContainerID:   e.id,
			ContainerName: name,
			Value:         e.value,
			Percent:       e.percent,
			Rank:          i + 1,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"metric":    metric,
		"consumers": consumers,
	})
}

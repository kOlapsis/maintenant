package v1

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/kolapsis/pulseboard/internal/resource"
)

// ResourceTopService abstracts the resource service for top consumers.
type ResourceTopService interface {
	GetAllLatestSnapshots() map[int64]*resource.ResourceSnapshot
	GetContainerName(containerID int64) string
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

// HandleGetTopConsumers handles GET /api/v1/resources/top?metric=cpu|memory&limit=5.
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

	all := h.svc.GetAllLatestSnapshots()

	// Build sortable list of consumers.
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

	// Sort descending by value (for cpu) or percent (for memory).
	sort.Slice(entries, func(i, j int) bool {
		if metric == "memory" {
			return entries[i].percent > entries[j].percent
		}
		return entries[i].value > entries[j].value
	})

	// Take top N.
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

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"metric":    metric,
		"consumers": consumers,
	})
}

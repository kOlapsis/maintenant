package v1

import (
	"context"
	"fmt"
	"net/http"
)

// SparklineDataFetcher abstracts the endpoint store for sparkline data.
type SparklineDataFetcher interface {
	GetSparklineData(ctx context.Context, limit int) (map[int64][]float64, error)
}

// SparklineHandler handles the batch sparkline endpoint.
type SparklineHandler struct {
	store SparklineDataFetcher
}

// NewSparklineHandler creates a new sparkline handler.
func NewSparklineHandler(store SparklineDataFetcher) *SparklineHandler {
	return &SparklineHandler{store: store}
}

// HandleGetSparklines handles GET /api/v1/dashboard/sparklines.
func (h *SparklineHandler) HandleGetSparklines(w http.ResponseWriter, r *http.Request) {
	data, err := h.store.GetSparklineData(r.Context(), 20)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch sparkline data")
		return
	}

	result := make(map[string][]float64, len(data))
	for epID, vals := range data {
		result[fmt.Sprintf("endpoint:%d", epID)] = vals
	}

	WriteJSON(w, http.StatusOK, result)
}

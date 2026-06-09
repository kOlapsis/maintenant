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
)

// SparklineDataFetcher abstracts the endpoint store for sparkline data.
type SparklineDataFetcher interface {
	GetSparklineData(ctx context.Context, limit int) (map[string][]float64, error)
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
		result["endpoint:"+epID] = vals
	}

	WriteJSON(w, http.StatusOK, result)
}

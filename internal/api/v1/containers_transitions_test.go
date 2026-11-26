// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
)

type transitionOptsStore struct {
	agentEnrichStore
	calls []container.ListTransitionsOpts
}

func (s *transitionOptsStore) ListTransitionsByContainer(_ context.Context, _ string, opts container.ListTransitionsOpts) ([]*container.StateTransition, int, error) {
	s.calls = append(s.calls, opts)
	return nil, 0, nil
}

func TestHandleTransitions_BoundsTheQuery(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		wantCode  int
		wantLimit int
	}{
		{"default", "", http.StatusOK, 50},
		{"limit above the ceiling", "?since=1970-01-01T00:00:00Z&limit=2000000000", http.StatusOK, 500},
		{"unparsable since", "?since=whatever", http.StatusBadRequest, 0},
		{"unparsable until", "?until=whatever", http.StatusBadRequest, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &transitionOptsStore{}
			h := NewContainerHandler(container.NewService(container.Deps{Store: store, Logger: slog.Default()}), nil)
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/containers/{id}/transitions", h.HandleTransitions)

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/containers/c1/transitions"+tc.query, nil))

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			if tc.wantCode != http.StatusOK {
				var body ErrorResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				assert.Equal(t, "INVALID_PARAM", body.Error.Code)
				assert.Empty(t, store.calls)
				return
			}
			require.Len(t, store.calls, 1)
			assert.Equal(t, tc.wantLimit, store.calls[0].Limit)
			assert.NotNil(t, store.calls[0].Since)
		})
	}
}

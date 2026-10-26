// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13: the browser keeps nothing of /api/v1/.

// FR-007, US3 scenario 3: every /api/v1/ answer is Cache-Control: no-store; the SSE stream keeps no-cache.
func TestAPIAnswersAreNeverStored(t *testing.T) {
	t.Setenv("MAINTENANT_DATA_DIR", t.TempDir())
	cfg, _, logger := storageEnv(t)
	a, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.db.Close() })
	handler := a.buildHTTPServer().Handler

	for _, path := range []string{"/api/v1/health", "/api/v1/edition", "/api/v1/containers", "/api/v1/alerts", "/api/v1/does-not-exist"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Host = "127.0.0.1"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if path == "/api/v1/health" {
				require.Equal(t, http.StatusOK, rec.Code)
			}
			assert.Contains(t, rec.Header().Get("Cache-Control"), "no-store", "status %d", rec.Code)
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/events", nil).WithContext(ctx)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"), "an SSE stream keeps no-cache")
}

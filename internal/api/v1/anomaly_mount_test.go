// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

func anomalyMountRouter(api http.Handler) http.Handler {
	return NewRouter(HandlerDeps{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Anomaly: api,
	}).mux
}

func TestAnomalyMount_RefusedBelowPro(t *testing.T) {
	served := false
	mux := anomalyMountRouter(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { served = true }))

	for _, edition := range []extension.Edition{extension.Community, extension.Personal} {
		t.Run(string(edition), func(t *testing.T) {
			withEdition(t, edition)
			for _, method := range []string{http.MethodGet, http.MethodPut} {
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/anomaly/series/container/compose/app/web", nil))
				require.Equal(t, http.StatusForbidden, rec.Code, method)

				var body struct {
					Error struct {
						Code            string `json:"code"`
						Feature         string `json:"feature"`
						RequiredEdition string `json:"required_edition"`
					} `json:"error"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, "EDITION_REQUIRED", body.Error.Code)
				assert.Equal(t, string(extension.CapAnomalyDetection), body.Error.Feature)
				assert.Equal(t, string(extension.Pro), body.Error.RequiredEdition)
			}
		})
	}
	assert.False(t, served, "a refused request must not reach the anomaly API")
}

func TestAnomalyMount_ServedOnPro(t *testing.T) {
	withEdition(t, extension.Pro)
	var gotPath string
	mux := anomalyMountRouter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/anomaly/summary", nil))
	assert.Equal(t, http.StatusTeapot, rec.Code)
	assert.Equal(t, "/api/v1/anomaly/summary", gotPath)
}

func TestAnomalyMount_AbsentWithoutHandler(t *testing.T) {
	withEdition(t, extension.Pro)
	rec := httptest.NewRecorder()
	anomalyMountRouter(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/anomaly/summary", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

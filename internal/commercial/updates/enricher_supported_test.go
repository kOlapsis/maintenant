// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kolapsis/maintenant/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrich_EvaluatesOnlyWhatOSVCanAnswer(t *testing.T) {
	for _, tc := range []struct {
		image, tag  string
		wantPackage string
		wantVersion string
	}{
		{"traefik", "v3.1.2", "github.com/traefik/traefik/v3", "3.1.2"},
		{"traefik", "2.11.0", "github.com/traefik/traefik/v2", "2.11.0"},
		{"caddy", "2.8.4-alpine", "github.com/caddyserver/caddy/v2", "2.8.4"},
		{"hashicorp/consul", "1.19.2", "github.com/hashicorp/consul", "1.19.2"},
		{"docker.io/hashicorp/vault", "1.17.5", "github.com/hashicorp/vault", "1.17.5"},
		{"traefik", "latest", "", ""},
		{"traefik", "v3", "", ""},
		{"traefik", "1.7.34", "", ""},
		{"ghcr.io/acme/vault", "1.17.5", "", ""},
		{"nginx", "1.25-alpine", "", ""},
		{"nginx", "1.27.0", "", ""},
		{"postgres", "16.3", "", ""},
		{"mongo", "7.0.0", "", ""},
		{"grafana/grafana", "11.0.0", "", ""},
		{"prom/prometheus", "v2.53.0", "", ""},
		{"ghcr.io/acme/curl", "1.0", "", ""},
		{"haproxy", "2.9.0-alpine", "", ""},
	} {
		t.Run(tc.image+":"+tc.tag, func(t *testing.T) {
			var mu sync.Mutex
			var asked []osvQuery
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req osvBatchRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				mu.Lock()
				asked = append(asked, req.Queries...)
				mu.Unlock()
				writeJSON(t, w, osvBatchResponse{Results: []osvBatchResult{{}}})
			}))
			defer server.Close()

			store := newEnricherStubStore()
			enricher := newTestEnricher(store, newTestCVEClient(store, server.URL))
			require.NoError(t, enricher.Enrich(context.Background(), []update.UpdateResult{{
				ContainerID: "c1", ContainerName: "app", Image: tc.image + ":" + tc.tag, CurrentTag: tc.tag,
			}}))

			eval := store.evaluation("c1")
			require.NotNil(t, eval)
			mu.Lock()
			defer mu.Unlock()
			if tc.wantPackage == "" {
				assert.Equal(t, update.CVEUnsupported, eval.Status)
				assert.Empty(t, asked, "nothing may be asked for an image whose answer cannot be right")
				return
			}
			assert.Equal(t, update.CVEEvaluated, eval.Status)
			require.Len(t, asked, 1)
			assert.Equal(t, osvQuery{Package: osvPackage{Name: tc.wantPackage, Ecosystem: "Go"}, Version: tc.wantVersion}, asked[0])
		})
	}
}

// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kolapsis/maintenant/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unreadableCacheStore struct {
	*enricherStubStore
}

func (unreadableCacheStore) IsCVECacheFresh(context.Context, string, string, string) (bool, error) {
	return true, nil
}

func (unreadableCacheStore) GetCVECacheEntries(context.Context, string, string, string) ([]*update.CVECacheEntry, error) {
	return nil, errors.New("database is locked")
}

func TestQueryCVEs_AFollowingPageWithoutResultFailsTheQuery(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			writeJSON(t, w, osvBatchResponse{Results: []osvBatchResult{{NextPageToken: "more"}}})
			return
		}
		writeJSON(t, w, osvBatchResponse{})
	}))
	defer server.Close()

	_, err := newTestCVEClient(&cveStubStore{}, server.URL).QueryCVEs(context.Background(), []ImageCVEQuery{
		{ContainerID: "c1", PackageName: "github.com/traefik/traefik/v3", Ecosystem: "Go", Version: "3.1.0"},
	})
	require.Error(t, err)
}

func TestEnrich_AnIncompleteLookupClosesNothing(t *testing.T) {
	shortAnswer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, osvBatchResponse{})
	}))
	defer shortAnswer.Close()

	recorded := newEnricherStubStore()
	for name, store := range map[string]update.UpdateStore{
		"answer shorter than the question": recorded,
		"unreadable cache":                 unreadableCacheStore{recorded},
	} {
		t.Run(name, func(t *testing.T) {
			recorded.mu.Lock()
			recorded.resolved, recorded.evaluations = nil, map[string]*update.CVEEvaluation{}
			recorded.mu.Unlock()

			enricher := newTestEnricher(store, newTestCVEClient(store, shortAnswer.URL))
			require.NoError(t, enricher.Enrich(context.Background(), []update.UpdateResult{
				{ContainerID: "c1", Image: "traefik", CurrentTag: "v3.1.0"},
			}))

			eval := recorded.evaluation("c1")
			require.NotNil(t, eval)
			assert.Equal(t, update.CVEEvaluationError, eval.Status)
			recorded.mu.Lock()
			defer recorded.mu.Unlock()
			assert.Empty(t, recorded.resolved, "the recorded CVEs stay as they are")
		})
	}
}

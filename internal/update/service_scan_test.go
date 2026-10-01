// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
)

// downRegistry answers like stubRegistry except for the repositories it cannot reach.
type downRegistry struct {
	*stubRegistry
	down map[string]bool
}

func (r *downRegistry) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	if r.down[imageRef] {
		return nil, errors.New("dial tcp: i/o timeout")
	}
	return r.stubRegistry.ListTags(ctx, imageRef)
}

// pendingStore holds the updates earlier scans left pending, by container name.
type pendingStore struct {
	*stubStore
	pending map[string]StaleImageUpdate
}

func (s *pendingStore) ListStaleImageUpdates(_ context.Context, _ string, names []string) ([]StaleImageUpdate, error) {
	var stale []StaleImageUpdate
	for _, name := range names {
		if su, ok := s.pending[name]; ok {
			stale = append(stale, su)
		}
	}
	return stale, nil
}

func (s *pendingStore) DeleteStaleImageUpdates(_ context.Context, _ string, names []string) (int64, error) {
	var deleted int64
	for _, name := range names {
		if _, ok := s.pending[name]; ok {
			delete(s.pending, name)
			deleted++
		}
	}
	return deleted, nil
}

type recordingEnricher struct {
	results []UpdateResult
}

func (e *recordingEnricher) Enrich(_ context.Context, results []UpdateResult) error {
	e.results = append(e.results, results...)
	return nil
}

func TestRunScan_KeepsThePendingUpdateOfAContainerWhoseScanFailed(t *testing.T) {
	store := &pendingStore{stubStore: &stubStore{}, pending: map[string]StaleImageUpdate{
		"db":  {ContainerID: "ctr-db", ContainerName: "db"},
		"web": {ContainerID: "ctr-web", ContainerName: "web"},
	}}
	reg := &downRegistry{
		stubRegistry: &stubRegistry{tags: map[string][]string{"library/nginx": {"1.26.0"}}},
		down:         map[string]bool{"library/postgres": true},
	}

	var resolved []string
	svc := NewService(Deps{
		Store:   store,
		Scanner: newTestScanner(reg, store),
		Containers: stubLister{containers: []ContainerInfo{
			{UID: "uid-db", ExternalID: "ctr-db", Name: "db", Image: "postgres:16.1"},
			{UID: "uid-web", ExternalID: "ctr-web", Name: "web", Image: "nginx:1.26.0"},
		}},
		Logger: testLogger(),
		EventCallback: func(eventType string, data interface{}) {
			if eventType == event.UpdateResolved {
				resolved = append(resolved, data.(map[string]interface{})["container_name"].(string))
			}
		},
	})

	svc.runScan(context.Background())

	assert.Equal(t, []string{"web"}, resolved, "only a container the scan reached can lose its update")
	assert.Contains(t, store.pending, "db", "an unreachable registry must not drop the pending update")
	assert.NotContains(t, store.pending, "web")
}

func TestRunScan_RunsTheCVEPassOnEveryContainer(t *testing.T) {
	store := &stubStore{}
	reg := &downRegistry{
		stubRegistry: &stubRegistry{tags: map[string][]string{
			"library/nginx": {"1.24.0", "1.26.0"},
			"library/redis": {"7.2.4"},
		}},
		down: map[string]bool{"library/postgres": true},
	}
	enricher := &recordingEnricher{}
	svc := NewService(Deps{
		Store:   store,
		Scanner: newTestScanner(reg, store),
		Containers: stubLister{containers: []ContainerInfo{
			{ExternalID: "ctr-web", Name: "web", Image: "nginx:1.24.0"},
			{ExternalID: "ctr-cache", Name: "cache", Image: "redis:7.2.4", RepoDigests: []string{"redis@sha256:running"}},
			{ExternalID: "ctr-db", Name: "db", Image: "ghcr.io/acme/db:16.1"},
			{ExternalID: "ctr-pg", Name: "pg", Image: "postgres:16.1"},
		}},
		Logger:   testLogger(),
		Enricher: enricher,
	})

	svc.runScan(context.Background())

	byID := map[string]UpdateResult{}
	for _, r := range enricher.results {
		byID[r.ContainerID] = r
	}
	require.Len(t, byID, 4, "the CVE pass covers the containers without an update too")
	assert.True(t, byID["ctr-web"].HasUpdate)
	assert.Equal(t, "1.26.0", byID["ctr-web"].LatestTag)

	assert.Equal(t, UpdateResult{
		ContainerID:   "ctr-cache",
		ContainerName: "cache",
		Image:         "redis:7.2.4",
		CurrentTag:    "7.2.4",
		CurrentDigest: "sha256:running",
		Registry:      "registry-1.docker.io",
	}, byID["ctr-cache"])
	assert.Equal(t, "ghcr.io", byID["ctr-db"].Registry)
	assert.False(t, byID["ctr-pg"].HasUpdate, "a failed scan still leaves the running image to analyse")
}

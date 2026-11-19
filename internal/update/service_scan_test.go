// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/uid"
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

func (s *pendingStore) ListStaleImageUpdates(_ context.Context, _ string, _, names []string) ([]StaleImageUpdate, error) {
	var stale []StaleImageUpdate
	for _, name := range names {
		if su, ok := s.pending[name]; ok {
			stale = append(stale, su)
		}
	}
	return stale, nil
}

func (s *pendingStore) DeleteStaleImageUpdates(_ context.Context, _ string, _, names []string) (int64, error) {
	var deleted int64
	for _, name := range names {
		if _, ok := s.pending[name]; ok {
			delete(s.pending, name)
			deleted++
		}
	}
	return deleted, nil
}

type agentPendingStore struct {
	*stubStore
	pending []StaleImageUpdate
}

func (s *agentPendingStore) ListStaleImageUpdates(_ context.Context, _ string, agentIDs, names []string) ([]StaleImageUpdate, error) {
	var stale []StaleImageUpdate
	for _, su := range s.pending {
		if slices.Contains(agentIDs, su.AgentID) && slices.Contains(names, su.ContainerName) {
			stale = append(stale, su)
		}
	}
	return stale, nil
}

func (s *agentPendingStore) DeleteStaleImageUpdates(_ context.Context, _ string, agentIDs, names []string) (int64, error) {
	before := len(s.pending)
	s.pending = slices.DeleteFunc(s.pending, func(su StaleImageUpdate) bool {
		return slices.Contains(agentIDs, su.AgentID) && slices.Contains(names, su.ContainerName)
	})
	return int64(before - len(s.pending)), nil
}

func runResolvingScan(t *testing.T, store *agentPendingStore, containers []ContainerInfo) map[string]map[string]interface{} {
	t.Helper()
	resolved := map[string]map[string]interface{}{}
	svc := NewService(Deps{
		Store:      store,
		Scanner:    newTestScanner(&stubRegistry{}, store),
		Containers: stubLister{containers: containers},
		Logger:     testLogger(),
		EventCallback: func(eventType string, data interface{}) {
			if eventType != event.UpdateResolved {
				return
			}
			m, ok := data.(map[string]interface{})
			require.True(t, ok)
			resolved[m["container_id"].(string)] = m
		},
	})
	svc.runScan(context.Background())
	return resolved
}

func TestRunScan_StoppedContainerOnACoveredHostIsResolved(t *testing.T) {
	store := &agentPendingStore{stubStore: &stubStore{}, pending: []StaleImageUpdate{
		{ContainerID: "ctr-valkey-nas", ContainerName: "valkey", AgentID: "agent-nas"},
		{ContainerID: "ctr-valkey-web", ContainerName: "valkey", AgentID: "agent-web"},
	}}

	resolved := runResolvingScan(t, store, []ContainerInfo{
		{UID: "uid-pg-nas", AgentID: "agent-nas", ExternalID: "ctr-pg-nas", Name: "pg", Image: "postgres:16.4"},
		{UID: "uid-valkey-web", AgentID: "agent-web", ExternalID: "ctr-valkey-web", Name: "valkey", Image: "valkey/valkey:8.1.3"},
	})

	assert.Empty(t, store.pending, "the stopped container's entry goes with the others")
	require.Contains(t, resolved, "ctr-valkey-nas")
	assert.Equal(t, "agent-nas", resolved["ctr-valkey-nas"]["agent_id"])
	assert.Equal(t, uid.Container("agent-nas", "ctr-valkey-nas"), resolved["ctr-valkey-nas"]["container_uid"])
	require.Contains(t, resolved, "ctr-valkey-web")
	assert.Equal(t, "uid-valkey-web", resolved["ctr-valkey-web"]["container_uid"])
}

func TestRunScan_AbsentHostDoesNotHoldBackTheOthers(t *testing.T) {
	absent := StaleImageUpdate{ContainerID: "ctr-valkey-nas", ContainerName: "valkey", AgentID: "agent-nas"}
	store := &agentPendingStore{stubStore: &stubStore{}, pending: []StaleImageUpdate{
		absent,
		{ContainerID: "ctr-valkey-web", ContainerName: "valkey", AgentID: "agent-web"},
		{ContainerID: "ctr-valkey-local", ContainerName: "valkey", AgentID: uid.LocalAgent},
	}}

	resolved := runResolvingScan(t, store, []ContainerInfo{
		{UID: "uid-valkey-web", AgentID: "agent-web", ExternalID: "ctr-valkey-web", Name: "valkey", Image: "valkey/valkey:8.1.3"},
		{UID: "uid-valkey-local", ExternalID: "ctr-valkey-local", Name: "valkey", Image: "valkey/valkey:8.1.3"},
	})

	assert.Equal(t, []StaleImageUpdate{absent}, store.pending, "only the absent host keeps its entry")
	assert.NotContains(t, resolved, absent.ContainerID)
	require.Contains(t, resolved, "ctr-valkey-web")
	assert.Equal(t, "agent-web", resolved["ctr-valkey-web"]["agent_id"])
	require.Contains(t, resolved, "ctr-valkey-local")
	assert.Equal(t, uid.LocalAgent, resolved["ctr-valkey-local"]["agent_id"])
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
		AgentID:       uid.LocalAgent,
		ContainerName: "cache",
		Image:         "redis:7.2.4",
		CurrentTag:    "7.2.4",
		CurrentDigest: "sha256:running",
		Registry:      "registry-1.docker.io",
	}, byID["ctr-cache"])
	assert.Equal(t, "ghcr.io", byID["ctr-db"].Registry)
	assert.False(t, byID["ctr-pg"].HasUpdate, "a failed scan still leaves the running image to analyse")
}

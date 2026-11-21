// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
)

type keyedUpdateStore struct {
	*stubStore
	scans int
	rows  map[string]ImageUpdate
}

func (s *keyedUpdateStore) InsertScanRecord(_ context.Context, _ *ScanRecord) (string, error) {
	s.scans++
	return string(rune('a' + s.scans)), nil
}

func (s *keyedUpdateStore) InsertImageUpdate(_ context.Context, u *ImageUpdate) (string, error) {
	s.rows[u.AgentID+"|"+u.ContainerName+"|"+u.Image+"|"+u.LatestTag] = *u
	return "", nil
}

func (s *keyedUpdateStore) ListStaleImageUpdates(_ context.Context, scanID string, _, names []string) ([]StaleImageUpdate, error) {
	var stale []StaleImageUpdate
	for _, u := range s.rows {
		if u.ScanID != scanID && slices.Contains(names, u.ContainerName) {
			stale = append(stale, StaleImageUpdate{ContainerID: u.ContainerID, ContainerName: u.ContainerName, AgentID: u.AgentID})
		}
	}
	return stale, nil
}

func (s *keyedUpdateStore) DeleteStaleImageUpdates(_ context.Context, scanID string, _, names []string) (int64, error) {
	var deleted int64
	for k, u := range s.rows {
		if u.ScanID != scanID && slices.Contains(names, u.ContainerName) {
			delete(s.rows, k)
			deleted++
		}
	}
	return deleted, nil
}

func TestRunScan_TargetChange_RecoveryIsAnnouncedBeforeTheDetection(t *testing.T) {
	reg := &stubRegistry{tags: map[string][]string{"library/postgres": {"18.7", "19.0"}}, digest: "sha256:new"}
	store := &keyedUpdateStore{
		stubStore: &stubStore{baseline: &DigestBaseline{ContainerID: "ctr1", Tag: "18.7", RemoteDigest: "sha256:new"}},
		rows:      map[string]ImageUpdate{},
	}
	var events []string
	var latest []string
	svc := NewService(Deps{
		Store:      store,
		Scanner:    newTestScanner(reg, store),
		Containers: stubLister{containers: []ContainerInfo{{UID: "uid-db", ExternalID: "ctr1", Name: "db", Image: "postgres:18.7"}}},
		Logger:     testLogger(),
		EventCallback: func(eventType string, data interface{}) {
			if eventType != event.UpdateDetected && eventType != event.UpdateResolved {
				return
			}
			events = append(events, eventType)
			if eventType == event.UpdateDetected {
				latest = append(latest, data.(map[string]interface{})["latest_tag"].(string))
			}
		},
	})

	svc.runScan(context.Background())
	assert.Equal(t, []string{event.UpdateDetected}, events)
	assert.Equal(t, []string{"19.0"}, latest)

	events, latest = nil, nil
	reg.tags["library/postgres"] = []string{"18.7", "18.8", "19.0"}
	svc.runScan(context.Background())

	require.Equal(t, []string{event.UpdateResolved, event.UpdateDetected}, events)
	assert.Equal(t, []string{"18.8"}, latest)
}

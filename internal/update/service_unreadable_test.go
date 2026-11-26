// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
)

// refusingRegistry answers like stubRegistry except for the repositories it refuses to show.
type refusingRegistry struct {
	*stubRegistry
	tagsErr   map[string]error
	digestErr map[string]error
}

func (r *refusingRegistry) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	if err := r.tagsErr[imageRef]; err != nil {
		return nil, err
	}
	return r.stubRegistry.ListTags(ctx, imageRef)
}

func (r *refusingRegistry) ResolveDigests(ctx context.Context, imageRef string) (RemoteDigests, error) {
	repo := imageRef[:strings.LastIndex(imageRef, ":")]
	if err := r.digestErr[repo]; err != nil {
		return RemoteDigests{}, err
	}
	return r.stubRegistry.ResolveDigests(ctx, imageRef)
}

// scanRecordingStore keeps the scan record and the updates a scan writes.
type scanRecordingStore struct {
	*pendingStore
	record   *ScanRecord
	inserted []string
}

func (s *scanRecordingStore) UpdateScanRecord(_ context.Context, r *ScanRecord) error {
	s.record = r
	return nil
}

func (s *scanRecordingStore) InsertImageUpdate(_ context.Context, u *ImageUpdate) (string, error) {
	s.inserted = append(s.inserted, u.ContainerName)
	return "", nil
}

func runRefusedScan(t *testing.T, reg *refusingRegistry, pending []string, containers []ContainerInfo) (*scanRecordingStore, []string) {
	t.Helper()
	store := &scanRecordingStore{pendingStore: &pendingStore{stubStore: &stubStore{}, pending: map[string]StaleImageUpdate{}}}
	for _, name := range pending {
		store.pending[name] = StaleImageUpdate{ContainerID: "ctr-" + name, ContainerName: name}
	}
	var resolved []string
	svc := NewService(Deps{
		Store:      store,
		Scanner:    newTestScanner(reg, store),
		Containers: stubLister{containers: containers},
		Logger:     testLogger(),
		EventCallback: func(eventType string, data interface{}) {
			if eventType == event.UpdateResolved {
				resolved = append(resolved, data.(map[string]interface{})["container_name"].(string))
			}
		},
	})
	svc.runScan(context.Background())
	require.NotNil(t, store.record)
	return store, resolved
}

func TestRunScan_KeepsThePendingUpdateOfAnImageTheRegistryRefuses(t *testing.T) {
	tests := []struct {
		name string
		reg  *refusingRegistry
		ctr  ContainerInfo
	}{
		{
			name: "tag listing",
			reg: &refusingRegistry{
				stubRegistry: &stubRegistry{tags: map[string][]string{"library/nginx": {"1.26.0"}}},
				tagsErr:      map[string]error{"ghcr.io/acme/db": errors.New("GET https://ghcr.io/v2/acme/db/tags/list: UNAUTHORIZED: authentication required")},
			},
			ctr: ContainerInfo{UID: "uid-db", ExternalID: "ctr-db", Name: "db", Image: "ghcr.io/acme/db:16.1"},
		},
		{
			name: "digest of a floating tag",
			reg: &refusingRegistry{
				stubRegistry: &stubRegistry{tags: map[string][]string{
					"library/nginx":   {"1.26.0"},
					"ghcr.io/acme/db": {"stable", "1.0.0"},
				}},
				digestErr: map[string]error{"ghcr.io/acme/db": errors.New("GET https://ghcr.io/v2/acme/db/manifests/stable: UNAUTHORIZED: authentication required")},
			},
			ctr: ContainerInfo{UID: "uid-db", ExternalID: "ctr-db", Name: "db", Image: "ghcr.io/acme/db:stable"},
		},
		{
			name: "digest tracked by label",
			reg: &refusingRegistry{
				stubRegistry: &stubRegistry{tags: map[string][]string{"library/nginx": {"1.26.0"}}},
				digestErr:    map[string]error{"ghcr.io/acme/db": errors.New("MANIFEST_UNKNOWN: manifest unknown")},
			},
			ctr: ContainerInfo{
				UID: "uid-db", ExternalID: "ctr-db", Name: "db", Image: "ghcr.io/acme/db:16.1",
				Labels: map[string]string{"maintenant.update.track": "digest"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, resolved := runRefusedScan(t, tc.reg, []string{"db", "web"}, []ContainerInfo{
				tc.ctr,
				{UID: "uid-web", ExternalID: "ctr-web", Name: "web", Image: "nginx:1.26.0"},
			})

			assert.Contains(t, store.pending, "db")
			assert.NotContains(t, store.pending, "web")
			assert.Equal(t, []string{"web"}, resolved)
			assert.Equal(t, ScanStatusCompleted, store.record.Status)
			assert.Zero(t, store.record.Errors)
			assert.Equal(t, 2, store.record.ContainersScanned)
		})
	}
}

func TestRunScan_ImageUnknownToItsRegistryIsNotAnError(t *testing.T) {
	reg := &refusingRegistry{
		stubRegistry: &stubRegistry{},
		tagsErr:      map[string]error{"library/myapp": errors.New("NAME_UNKNOWN: repository name not known to registry")},
	}

	store, resolved := runRefusedScan(t, reg, nil, []ContainerInfo{
		{UID: "uid-app", ExternalID: "ctr-app", Name: "app", Image: "myapp:1.0.0"},
	})

	assert.Empty(t, resolved)
	assert.Empty(t, store.inserted)
	assert.Empty(t, store.pending)
	assert.Equal(t, ScanStatusCompleted, store.record.Status)
	assert.Zero(t, store.record.Errors)
	assert.Equal(t, 1, store.record.ContainersScanned)
	assert.Zero(t, store.record.UpdatesFound)
}

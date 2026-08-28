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

package update

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
)

// orphanStore records the orphan cleanup calls a scan makes.
type orphanStore struct {
	*stubStore
	orphans     []StaleImageUpdate
	listCalls   int
	deleteCalls int
}

func (s *orphanStore) ListOrphanImageUpdates(_ context.Context) ([]StaleImageUpdate, error) {
	s.listCalls++
	return s.orphans, nil
}

func (s *orphanStore) DeleteOrphanImageUpdates(_ context.Context) (int64, error) {
	s.deleteCalls++
	return int64(len(s.orphans)), nil
}

type stubLister struct {
	containers []ContainerInfo
}

func (l stubLister) ListContainerInfos(_ context.Context) ([]ContainerInfo, error) {
	return l.containers, nil
}

func newOrphanTestService(store UpdateStore, containers []ContainerInfo, onEvent EventCallback) *Service {
	return NewService(Deps{
		Store:         store,
		Scanner:       newTestScanner(&stubRegistry{}, store),
		Containers:    stubLister{containers: containers},
		Logger:        testLogger(),
		EventCallback: onEvent,
	})
}

// Issue #50: a scan must shed the findings of containers no runtime reports
// anymore, and announce each one so the alert engine resolves its alert. The
// entity id comes from the store, since the container is gone from the scan set
// and cannot be looked up there.
func TestRunScan_DropsFindingsOfContainersThatNoLongerExist(t *testing.T) {
	store := &orphanStore{
		stubStore: &stubStore{},
		orphans: []StaleImageUpdate{{
			ContainerID:   "task-old",
			ContainerName: "stack_api.1.oldtask",
			ContainerUID:  "uid-old",
		}},
	}

	var resolved []map[string]interface{}
	svc := newOrphanTestService(store, []ContainerInfo{{
		UID:        "uid-new",
		ExternalID: "task-new",
		Name:       "stack_api.1.newtask",
		Image:      "ghcr.io/acme/api:1.0.0",
	}}, func(eventType string, data interface{}) {
		if eventType != event.UpdateResolved {
			return
		}
		m, ok := data.(map[string]interface{})
		require.True(t, ok)
		resolved = append(resolved, m)
	})

	svc.runScan(context.Background())

	assert.Equal(t, 1, store.deleteCalls, "the scan must purge the orphan findings")
	require.Len(t, resolved, 1, "each dropped finding must announce its recovery")
	assert.Equal(t, "task-old", resolved[0]["container_id"])
	assert.Equal(t, "uid-old", resolved[0]["container_uid"])
	assert.Equal(t, "stack_api.1.oldtask", resolved[0]["container_name"])
}

// A runtime that reports nothing must not be read as "every container is gone".
func TestRunScan_KeepsFindingsWhenTheScanSeesNoContainer(t *testing.T) {
	store := &orphanStore{
		stubStore: &stubStore{},
		orphans: []StaleImageUpdate{{
			ContainerID:   "task-old",
			ContainerName: "stack_api.1.oldtask",
			ContainerUID:  "uid-old",
		}},
	}

	svc := newOrphanTestService(store, nil, nil)
	svc.runScan(context.Background())

	assert.Zero(t, store.listCalls)
	assert.Zero(t, store.deleteCalls, "an empty scan must not wipe the Updates page")
}

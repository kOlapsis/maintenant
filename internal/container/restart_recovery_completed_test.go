// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_CheckRestartRecoveries_CompletedContainerRecovers(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("finished"), StateCompleted)
	c.ID = "finished-id"
	store.seed(c)

	checker := &mockRestartChecker{result: map[string]interface{}{"restarts": 9}}
	var recovered []map[string]interface{}
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.EventCallback = func(eventType string, data interface{}) {
			if eventType == "container.restart_recovery" {
				recovered = append(recovered, data.(map[string]interface{}))
			}
		}
	})
	svc.TrackRestartAlerts([]string{c.ID})

	svc.checkRestartRecoveries(context.Background())

	require.Len(t, recovered, 1, "a container that ended cleanly no longer loops")
	assert.Equal(t, StateCompleted, recovered[0]["state"])
	assert.False(t, isTrackedRestartAlert(svc, c.ID))
}

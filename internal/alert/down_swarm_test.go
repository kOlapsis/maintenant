// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
)

func (f *downFixture) addTask(t *testing.T, name string, state container.ContainerState) string {
	t.Helper()
	c := &container.Container{
		ExternalID:        name + "-external",
		Name:              name,
		State:             state,
		AlertSeverity:     container.SeverityCritical,
		RuntimeType:       "docker",
		ControllerKind:    container.ControllerSwarmService,
		SwarmServiceID:    "svc-web",
		SwarmServiceName:  "web",
		SwarmTaskSlot:     1,
		FirstSeenAt:       time.Now().Add(-time.Hour),
		LastStateChangeAt: time.Now().Add(-time.Hour),
	}
	id, err := f.containers.InsertContainer(context.Background(), c)
	require.NoError(t, err)
	return id
}

func TestDownDetector_SwarmTaskReplacedByARunningOneIsNotDown(t *testing.T) {
	f := newDownFixture(t)
	f.addTask(t, "web.1.old", container.StateExited)
	f.addTask(t, "web.1.new", container.StateRunning)

	assert.Empty(t, f.check(t))
}

func TestDownDetector_SwarmServiceWithNoRunningTaskIsDown(t *testing.T) {
	f := newDownFixture(t)
	f.addTask(t, "web.1.old", container.StateExited)

	events := f.check(t)
	require.Len(t, events, 1)
	f.fire(t, events[0])

	f.addTask(t, "web.1.new", container.StateRunning)
	events = f.check(t)
	require.Len(t, events, 1)
	assert.True(t, events[0].IsRecover)
	assert.Contains(t, events[0].Message, "replaced")
}

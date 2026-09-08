// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package alert_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

const downThreshold = 5 * time.Minute

type downFixture struct {
	containers *store.ContainerStore
	alerts     *store.AlertStoreImpl
	detector   *alert.DownDetector
	events     []alert.Event
}

func newDownFixture(t *testing.T) *downFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db := storetest.Open(t, logger)

	f := &downFixture{
		containers: store.NewContainerStore(db),
		alerts:     store.NewAlertStore(db),
	}
	f.detector = alert.NewDownDetector(f.containers, f.alerts, downThreshold, logger)
	f.detector.SetEmitter(func(evt alert.Event) { f.events = append(f.events, evt) })
	return f
}

// add stores a container in the given state, changed stoppedFor ago.
func (f *downFixture) add(t *testing.T, name string, state container.ContainerState, stoppedFor time.Duration) string {
	t.Helper()
	c := &container.Container{
		ExternalID:        name + "-external",
		Name:              name,
		Image:             "busybox:latest",
		State:             state,
		AlertSeverity:     container.SeverityCritical,
		RuntimeType:       "docker",
		FirstSeenAt:       time.Now().Add(-24 * time.Hour),
		LastStateChangeAt: time.Now().Add(-stoppedFor),
	}
	id, err := f.containers.InsertContainer(context.Background(), c)
	require.NoError(t, err)
	return id
}

// fire replays an event through the alert store the way the engine would, so
// the next pass sees it as already firing.
func (f *downFixture) fire(t *testing.T, evt alert.Event) {
	t.Helper()
	_, err := f.alerts.InsertAlert(context.Background(), &alert.Alert{
		Source:     evt.Source,
		AlertType:  evt.AlertType,
		Severity:   evt.Severity,
		Status:     alert.StatusActive,
		Message:    evt.Message,
		EntityType: evt.EntityType,
		EntityID:   evt.EntityID,
		EntityName: evt.EntityName,
		Details:    "{}",
		FiredAt:    evt.Timestamp,
	})
	require.NoError(t, err)
}

func (f *downFixture) check(t *testing.T) []alert.Event {
	t.Helper()
	f.events = nil
	require.NoError(t, f.detector.Check(context.Background()))
	return f.events
}

func TestDownDetector_FiresPastTheThreshold(t *testing.T) {
	f := newDownFixture(t)
	id := f.add(t, "api", container.StateExited, 10*time.Minute)

	events := f.check(t)
	require.Len(t, events, 1)
	evt := events[0]
	assert.Equal(t, alert.SourceContainer, evt.Source)
	assert.Equal(t, alert.AlertTypeContainerDown, evt.AlertType)
	assert.Equal(t, alert.SeverityCritical, evt.Severity)
	assert.False(t, evt.IsRecover)
	assert.Equal(t, "container", evt.EntityType)
	assert.Equal(t, id, evt.EntityID)
	assert.Equal(t, "api", evt.EntityName)
	assert.Equal(t, int64(downThreshold/time.Second), evt.Details["threshold_seconds"])
}

func TestDownDetector_StaysQuietBeforeTheThreshold(t *testing.T) {
	f := newDownFixture(t)
	f.add(t, "api", container.StateExited, time.Minute)

	assert.Empty(t, f.check(t))
}

// A container that exited with code 0 is recorded as completed: a job that
// finished as intended is not an outage.
func TestDownDetector_IgnoresStatesThatAreNotAnOutage(t *testing.T) {
	for _, state := range []container.ContainerState{
		container.StateRunning,
		container.StateCompleted,
		container.StatePaused,
		container.StateCreated,
		container.StateRestarting,
	} {
		t.Run(string(state), func(t *testing.T) {
			f := newDownFixture(t)
			f.add(t, "job", state, time.Hour)
			assert.Empty(t, f.check(t))
		})
	}
}

func TestDownDetector_DeadCountsAsDown(t *testing.T) {
	f := newDownFixture(t)
	f.add(t, "api", container.StateDead, time.Hour)

	require.Len(t, f.check(t), 1)
}

// The detector asks the alert store what is already firing, so a second pass
// over the same stopped container emits nothing.
func TestDownDetector_DoesNotReFireWhileTheAlertIsActive(t *testing.T) {
	f := newDownFixture(t)
	f.add(t, "api", container.StateExited, 10*time.Minute)

	events := f.check(t)
	require.Len(t, events, 1)
	f.fire(t, events[0])

	assert.Empty(t, f.check(t))
}

func TestDownDetector_ResolvesWhenTheContainerRunsAgain(t *testing.T) {
	f := newDownFixture(t)
	f.add(t, "api", container.StateExited, 10*time.Minute)

	events := f.check(t)
	require.Len(t, events, 1)
	f.fire(t, events[0])

	f.add(t, "api", container.StateRunning, time.Second)

	events = f.check(t)
	require.Len(t, events, 1)
	assert.True(t, events[0].IsRecover)
	assert.Equal(t, alert.SeverityInfo, events[0].Severity)
	assert.Equal(t, alert.AlertTypeContainerDown, events[0].AlertType)
	assert.Equal(t, "api", events[0].EntityName)
}

// Nothing is emitted for a container that never crossed the threshold, so a
// recovery cannot appear out of nowhere.
func TestDownDetector_NoRecoveryWithoutAFiringAlert(t *testing.T) {
	f := newDownFixture(t)
	f.add(t, "api", container.StateRunning, time.Hour)

	assert.Empty(t, f.check(t))
}

// The severity is the one configured on the container, not a constant.
func TestDownDetector_HonoursThePerContainerSeverity(t *testing.T) {
	f := newDownFixture(t)
	c := &container.Container{
		ExternalID:        "warn-external",
		Name:              "warn",
		State:             container.StateExited,
		AlertSeverity:     container.SeverityWarning,
		RuntimeType:       "docker",
		FirstSeenAt:       time.Now().Add(-time.Hour),
		LastStateChangeAt: time.Now().Add(-time.Hour),
	}
	_, err := f.containers.InsertContainer(context.Background(), c)
	require.NoError(t, err)

	events := f.check(t)
	require.Len(t, events, 1)
	assert.Equal(t, alert.SeverityWarning, events[0].Severity)
}

// An unrelated active alert on the same container must not be mistaken for the
// down alert, or the detector would never fire.
func TestDownDetector_IgnoresOtherAlertTypesOnTheSameContainer(t *testing.T) {
	f := newDownFixture(t)
	id := f.add(t, "api", container.StateExited, 10*time.Minute)

	_, err := f.alerts.InsertAlert(context.Background(), &alert.Alert{
		Source:     alert.SourceContainer,
		AlertType:  "restart_loop",
		Severity:   alert.SeverityWarning,
		Status:     alert.StatusActive,
		Message:    "restarting",
		EntityType: "container",
		EntityID:   id,
		EntityName: "api",
		Details:    "{}",
		FiredAt:    time.Now(),
	})
	require.NoError(t, err)

	require.Len(t, f.check(t), 1)
}

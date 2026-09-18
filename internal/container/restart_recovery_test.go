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

package container

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// atomicRestartChecker is a concurrency-safe RestartChecker for tests that
// drive ProcessEvent and the recovery tick from separate goroutines.
type atomicRestartChecker struct {
	mu     sync.Mutex
	result interface{}
}

func (c *atomicRestartChecker) Check(_ context.Context, _ *Container) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.result, nil
}

func isTrackedRestartAlert(svc *Service, id string) bool {
	svc.restartMu.Lock()
	defer svc.restartMu.Unlock()
	_, ok := svc.trackedRestartAlerts[id]
	return ok
}

// ---------------------------------------------------------------------------
// Full cycle: crash-loop tracks the alert, the window passing recovers it,
// and a later tick with nothing tracked stays silent.
// ---------------------------------------------------------------------------

func TestService_RestartRecovery_FullCycleTracksRecoversThenQuiet(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("cycle"), StateExited)
	c.ID = "cycle-id"
	store.seed(c)

	checker := &mockRestartChecker{result: map[string]interface{}{"restarts": 9}}
	var emitted []string
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.EventCallback = func(eventType string, _ interface{}) {
			emitted = append(emitted, eventType)
		}
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))
	assert.Contains(t, emitted, "container.restart_alert")
	assert.True(t, isTrackedRestartAlert(svc, c.ID), "a container above threshold must be tracked")

	// The restart window has now passed quietly.
	checker.result = nil
	emitted = nil

	svc.checkRestartRecoveries(context.Background())
	assert.Equal(t, []string{"container.restart_recovery"}, emitted, "one tick must emit exactly one recovery")
	assert.False(t, isTrackedRestartAlert(svc, c.ID), "the container must be untracked after recovery")

	emitted = nil
	svc.checkRestartRecoveries(context.Background())
	assert.Empty(t, emitted, "a second tick must stay silent once untracked")
}

func TestService_CheckRestartRecoveries_NotRunningStaysTracked(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("stillexited"), StateExited)
	c.ID = "stillexited-id"
	store.seed(c)

	checker := &mockRestartChecker{result: nil}
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
	})
	svc.trackRestartAlert(c.ID)

	svc.checkRestartRecoveries(context.Background())

	assert.Equal(t, 0, checker.calls, "a container that never came back running must not even be re-checked")
	assert.True(t, isTrackedRestartAlert(svc, c.ID), "a container still exited/restarting must stay tracked")
}

func TestService_CheckRestartRecoveries_StillAboveThresholdStaysTracked(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("stillhot"), StateRunning)
	c.ID = "stillhot-id"
	store.seed(c)

	checker := &mockRestartChecker{result: map[string]interface{}{"restarts": 20}}
	var emitted []string
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.EventCallback = func(eventType string, _ interface{}) {
			emitted = append(emitted, eventType)
		}
	})
	svc.trackRestartAlert(c.ID)

	svc.checkRestartRecoveries(context.Background())

	assert.Empty(t, emitted, "a container still above threshold must not be recovered")
	assert.True(t, isTrackedRestartAlert(svc, c.ID), "a container still above threshold must stay tracked")
}

func TestService_CheckRestartRecoveries_ArchivedContainerDroppedSilently(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("gone"), StateRunning)
	c.ID = "gone-id"
	store.seed(c)
	require.NoError(t, store.ArchiveContainer(context.Background(), c.ID, time.Now()))

	checker := &mockRestartChecker{}
	var emitted []string
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.EventCallback = func(eventType string, _ interface{}) {
			emitted = append(emitted, eventType)
		}
	})
	svc.trackRestartAlert(c.ID)

	svc.checkRestartRecoveries(context.Background())

	assert.Empty(t, emitted, "an archived container must be dropped without emitting anything")
	assert.False(t, isTrackedRestartAlert(svc, c.ID), "an archived container must be untracked")
}

func TestService_CheckRestartRecoveries_DeletedContainerDroppedSilently(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("deleted"), StateRunning)
	c.ID = "deleted-id"
	store.seed(c)
	require.NoError(t, store.DeleteContainerByID(context.Background(), c.ID))

	checker := &mockRestartChecker{}
	var emitted []string
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.EventCallback = func(eventType string, _ interface{}) {
			emitted = append(emitted, eventType)
		}
	})
	svc.trackRestartAlert(c.ID)

	svc.checkRestartRecoveries(context.Background())

	assert.Empty(t, emitted, "a deleted container must be dropped without emitting anything")
	assert.False(t, isTrackedRestartAlert(svc, c.ID))
}

func TestService_TrackRestartAlerts_SeedsThenTickRecovers(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("seeded"), StateRunning)
	c.ID = "seeded-id"
	store.seed(c)

	checker := &mockRestartChecker{result: nil}
	var emitted []string
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.EventCallback = func(eventType string, _ interface{}) {
			emitted = append(emitted, eventType)
		}
	})

	svc.TrackRestartAlerts([]string{c.ID})
	svc.checkRestartRecoveries(context.Background())

	assert.Contains(t, emitted, "container.restart_recovery",
		"seeding the tracker at startup then ticking must recover a stabilised container")
}

func TestService_ProcessEvent_TransitionRecoveryUntracksContainer(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("regression"), StateExited)
	c.ID = "regression-id"
	store.seed(c)

	checker := &mockRestartChecker{result: nil}
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
	})
	svc.trackRestartAlert(c.ID)

	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))

	assert.False(t, isTrackedRestartAlert(svc, c.ID), "a transition-driven recovery must untrack the container")

	svc.checkRestartRecoveries(context.Background())
	assert.Equal(t, 1, checker.calls, "the tick must not re-check a container already untracked by the transition")
}

func TestService_RunRestartRecoveryLoop_TicksAndStopsOnContextCancel(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("loop"), StateRunning)
	c.ID = "loop-id"
	store.seed(c)

	checker := &atomicRestartChecker{result: nil}
	var mu sync.Mutex
	var emitted []string
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.RestartRecoveryInterval = 5 * time.Millisecond
		d.EventCallback = func(eventType string, _ interface{}) {
			mu.Lock()
			defer mu.Unlock()
			emitted = append(emitted, eventType)
		}
	})
	svc.trackRestartAlert(c.ID)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.RunRestartRecoveryLoop(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(emitted) > 0
	}, time.Second, 5*time.Millisecond, "the loop must tick and emit a recovery")

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunRestartRecoveryLoop did not stop after context cancellation")
	}
}

func TestService_RestartRecovery_RaceSafeUnderConcurrentProcessEventAndTicks(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("race"), StateExited)
	c.ID = "race-id"
	store.seed(c)

	checker := &atomicRestartChecker{result: map[string]interface{}{"restarts": 5}}
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
	})

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))
			evt := makeTestEvent("die", c.ExternalID)
			evt.ExitCode = "1"
			svc.ProcessEvent(context.Background(), evt)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			svc.checkRestartRecoveries(context.Background())
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			svc.TrackRestartAlerts([]string{c.ID})
		}
	}()

	wg.Wait()
}

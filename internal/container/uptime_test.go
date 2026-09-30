// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ptr is a helper to take the address of a typed value.
func ptr[T any](v T) *T { return &v }

// epoch is a fixed reference time for all tests, giving deterministic windows.
var epoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// mkTransition builds a StateTransition at an offset relative to epoch.
func mkTransition(offsetSeconds float64, state ContainerState, health *HealthStatus) *StateTransition {
	return &StateTransition{
		NewState:  state,
		NewHealth: health,
		Timestamp: epoch.Add(time.Duration(offsetSeconds * float64(time.Second))),
	}
}

// --- computeUptime -----------------------------------------------------------

func TestComputeUptime_NoTransitions_Returns100(t *testing.T) {
	from := epoch
	to := epoch.Add(24 * time.Hour)

	pct := computeUptime(nil, from, to)

	assert.Equal(t, 100.0, pct)
}

func TestComputeUptime_AlwaysRunning_Returns100(t *testing.T) {
	from := epoch
	to := epoch.Add(time.Hour)

	// Single transition to running, timestamped at the window start.
	transitions := []*StateTransition{
		mkTransition(0, StateRunning, nil),
	}

	pct := computeUptime(transitions, from, to)

	assert.Equal(t, 100.0, pct)
}

func TestComputeUptime_AlwaysExited_Returns0(t *testing.T) {
	from := epoch
	to := epoch.Add(time.Hour)

	transitions := []*StateTransition{
		mkTransition(0, StateExited, nil),
	}

	pct := computeUptime(transitions, from, to)

	assert.Equal(t, 0.0, pct)
}

func TestComputeUptime_HalfUpHalfDown(t *testing.T) {
	from := epoch
	to := epoch.Add(time.Hour)
	mid := epoch.Add(30 * time.Minute)

	// Running for the first 30 minutes, then exited for the next 30 minutes.
	transitions := []*StateTransition{
		{NewState: StateRunning, Timestamp: from},
		{NewState: StateExited, Timestamp: mid},
	}

	pct := computeUptime(transitions, from, to)

	// 30 min up out of 60 min total = 50.00%
	assert.Equal(t, 50.0, pct)
}

func TestComputeUptime_MultipleTransitions(t *testing.T) {
	// Window: 0s–3600s (1 hour = 3600 seconds)
	from := epoch
	to := epoch.Add(time.Hour)

	// running 0–900s  (900s up)
	// exited  900–2700s (1800s down)
	// running 2700–3600s (900s up)
	// Total up: 1800s / 3600s = 50.00%
	transitions := []*StateTransition{
		{NewState: StateRunning, Timestamp: epoch.Add(0)},
		{NewState: StateExited, Timestamp: epoch.Add(900 * time.Second)},
		{NewState: StateRunning, Timestamp: epoch.Add(2700 * time.Second)},
	}

	pct := computeUptime(transitions, from, to)

	assert.Equal(t, 50.0, pct)
}

func TestComputeUptime_TransitionBeforeWindow(t *testing.T) {
	// Transition happened 30 minutes before the window starts.
	// It should be clamped to `from`, so the container is considered running
	// for the full window duration.
	from := epoch
	to := epoch.Add(time.Hour)

	transitions := []*StateTransition{
		{NewState: StateRunning, Timestamp: epoch.Add(-30 * time.Minute)},
	}

	pct := computeUptime(transitions, from, to)

	assert.Equal(t, 100.0, pct)
}

func TestComputeUptime_TransitionAfterWindow(t *testing.T) {
	// The last span runs past `to`; it must be clamped.
	// running 0–30m, exited 30m–∞ (but window ends at 60m).
	from := epoch
	to := epoch.Add(time.Hour)

	transitions := []*StateTransition{
		{NewState: StateRunning, Timestamp: epoch},
		// This transition falls outside the window.
		{NewState: StateExited, Timestamp: epoch.Add(90 * time.Minute)},
	}

	// The running span is clamped at `to`, so uptime is 100%.
	pct := computeUptime(transitions, from, to)

	assert.Equal(t, 100.0, pct)
}

func TestComputeUptime_ZeroWidthWindow_Returns0(t *testing.T) {
	t0 := epoch

	pct := computeUptime(nil, t0, t0)

	assert.Equal(t, 0.0, pct)
}

// --- isUp --------------------------------------------------------------------

func TestIsUp_RunningHealthy_IsUp(t *testing.T) {
	tr := &StateTransition{
		NewState:  StateRunning,
		NewHealth: ptr(HealthHealthy),
	}

	assert.True(t, isUp(tr))
}

func TestIsUp_RunningUnhealthy_IsDown(t *testing.T) {
	tr := &StateTransition{
		NewState:  StateRunning,
		NewHealth: ptr(HealthUnhealthy),
	}

	assert.False(t, isUp(tr))
}

func TestIsUp_RunningNoHealth_IsUp(t *testing.T) {
	tr := &StateTransition{
		NewState:  StateRunning,
		NewHealth: nil,
	}

	assert.True(t, isUp(tr))
}

func TestIsUp_ExitedState_IsDown(t *testing.T) {
	// Exited should be down regardless of any health annotation.
	trs := []*StateTransition{
		{NewState: StateExited, NewHealth: nil},
		{NewState: StateExited, NewHealth: ptr(HealthHealthy)},
		{NewState: StateCompleted, NewHealth: nil},
		{NewState: StateDead, NewHealth: nil},
		{NewState: StatePaused, NewHealth: nil},
		{NewState: StateRestarting, NewHealth: nil},
	}

	for _, tr := range trs {
		assert.False(t, isUp(tr), "expected isUp=false for state %q", tr.NewState)
	}
}

// --- UptimeCalculator (integration with mock store) -------------------------

// uptimeStore is a minimal ContainerStore for uptime calculator tests.
type uptimeStore struct {
	transitions map[string][]*StateTransition
	callCount   map[string]int
}

func newUptimeStore(containerID string, data []*StateTransition) *uptimeStore {
	return &uptimeStore{
		transitions: map[string][]*StateTransition{containerID: data},
		callCount:   make(map[string]int),
	}
}

func (m *uptimeStore) GetTransitionsInWindow(_ context.Context, containerID string, _, _ time.Time) ([]*StateTransition, error) {
	m.callCount[containerID]++
	return m.transitions[containerID], nil
}

func (m *uptimeStore) InsertContainer(_ context.Context, _ *Container) (string, error) {
	return "", nil
}
func (m *uptimeStore) UpdateContainer(_ context.Context, _ *Container) error { return nil }
func (m *uptimeStore) GetContainerByExternalID(_ context.Context, _, _ string) (*Container, error) {
	return nil, nil
}
func (m *uptimeStore) GetContainerByID(_ context.Context, _ string) (*Container, error) {
	return nil, nil
}
func (m *uptimeStore) ListContainers(_ context.Context, _ ListContainersOpts) ([]*Container, error) {
	return nil, nil
}
func (m *uptimeStore) ArchiveContainer(_ context.Context, _ string, _ time.Time) error { return nil }
func (m *uptimeStore) DeleteContainerByID(_ context.Context, _ string) error           { return nil }
func (m *uptimeStore) InsertTransition(_ context.Context, _ *StateTransition) (string, error) {
	return "", nil
}
func (m *uptimeStore) ListTransitionsByContainer(_ context.Context, _ string, _ ListTransitionsOpts) ([]*StateTransition, int, error) {
	return nil, 0, nil
}
func (m *uptimeStore) CountRestartsSince(_ context.Context, _ string, _ time.Time) (int, error) {
	return 0, nil
}
func (m *uptimeStore) DeleteTransitionsBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (m *uptimeStore) DeleteArchivedContainersBefore(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func TestUptimeCalculator_WindowsFollowTheHistoryCap(t *testing.T) {
	const day = 24 * time.Hour
	cases := []struct {
		name      string
		maxWindow time.Duration
		want      []string
	}{
		{"7 days", 7 * day, []string{"24h", "7d"}},
		{"30 days", 30 * day, []string{"24h", "7d", "30d"}},
		{"90 days", 90 * day, []string{"24h", "7d", "30d", "90d"}},
		{"below a day", time.Hour, []string{"24h"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const containerID = "2"
			calc := NewUptimeCalculator(newUptimeStore(containerID, nil)) // no transitions: 100% everywhere

			result, err := calc.Calculate(context.Background(), containerID, tc.maxWindow)
			require.NoError(t, err)

			got := map[string]*float64{"24h": result.Hours24, "7d": result.Days7, "30d": result.Days30, "90d": result.Days90}
			for name, v := range got {
				if slices.Contains(tc.want, name) {
					require.NotNil(t, v, "window %s must be computed", name)
					assert.Equal(t, 100.0, *v)
				} else {
					assert.Nil(t, v, "window %s must be left out", name)
				}
			}
		})
	}
}

func TestUptimeCalculator_CachesResult(t *testing.T) {
	const containerID = "3"
	store := newUptimeStore(containerID, nil)
	calc := NewUptimeCalculator(store)

	ctx := context.Background()

	_, err := calc.Calculate(ctx, containerID, 24*time.Hour)
	require.NoError(t, err)

	// The 24h window result is cached. A second call must not hit the store again.
	_, err = calc.Calculate(ctx, containerID, 24*time.Hour)
	require.NoError(t, err)

	// GetTransitionsInWindow should have been called exactly once across both
	// Calculate invocations (first populates cache, second reads from it).
	assert.Equal(t, 1, store.callCount[containerID],
		"store should be queried only once when result is cached within TTL")
}

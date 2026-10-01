// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	cmodel "github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/retry"
	"github.com/kolapsis/maintenant/internal/runtime"
)

func TestRuntimeEventToProto_MapsImageAndState(t *testing.T) {
	out := runtimeEventToProto(runtime.RuntimeEvent{
		Action:     "start",
		ExternalID: "c1",
		Name:       "demo",
		Image:      "adminer:latest",
		Labels:     map[string]string{"k": "v"},
	})
	require.NotNil(t, out)
	assert.Equal(t, "c1", out.ContainerId)
	assert.Equal(t, "demo", out.Name)
	assert.Equal(t, "adminer:latest", out.Image)
	assert.Equal(t, agentpb.ContainerState_CONTAINER_STATE_RUNNING, out.State)
}

func TestRuntimeEventToProto_NilForUnmappedAction(t *testing.T) {
	assert.Nil(t, runtimeEventToProto(runtime.RuntimeEvent{Action: "exec_start"}))
}

func TestRuntimeEventToProto_HealthStatus(t *testing.T) {
	out := runtimeEventToProto(runtime.RuntimeEvent{
		Action:       "health_status",
		ExternalID:   "c1",
		Name:         "demo",
		HealthStatus: "unhealthy",
	})
	require.NotNil(t, out)
	assert.Equal(t, agentpb.ContainerState_CONTAINER_STATE_UNSPECIFIED, out.State)
	assert.Equal(t, "unhealthy", out.HealthStatus)
	assert.True(t, out.HasHealthCheck)
	assert.False(t, out.Destroyed)
}

func TestRuntimeEventToProto_Destroy(t *testing.T) {
	out := runtimeEventToProto(runtime.RuntimeEvent{
		Action:     "destroy",
		ExternalID: "c1",
		Name:       "demo",
		Image:      "adminer:latest",
	})
	require.NotNil(t, out)
	assert.True(t, out.Destroyed)
	assert.Equal(t, agentpb.ContainerState_CONTAINER_STATE_EXITED, out.State,
		"a server without the destroyed field must read this as a stop, not a start")
	assert.Equal(t, "c1", out.ContainerId)
}

func TestSyncInventory_EmptySnapshotIsSentComplete(t *testing.T) {
	ev := inventoryEvent("agent-1", nil)
	inv := ev.GetInventory()
	require.NotNil(t, inv)
	assert.Empty(t, inv.GetContainers())
	assert.True(t, inv.GetComplete(), "a successful discovery that found nothing is still complete")
}

type digestRuntime struct {
	runtime.Runtime
	containers []*cmodel.Container
	digests    map[string][]string
}

func (r digestRuntime) DiscoverAll(context.Context) ([]*cmodel.Container, error) {
	return r.containers, nil
}

func (r digestRuntime) ContainerRepoDigests(context.Context) (map[string][]string, error) {
	return r.digests, nil
}

func TestSyncInventory_CarriesTheRepoDigestsOfTheRunningImages(t *testing.T) {
	sink := &captureSink{}
	spool := NewSpool(t.TempDir(), SpoolConfig{}, testLogger())
	spool.Attach(sink)
	t.Cleanup(func() { _ = spool.Close() })
	rt := digestRuntime{
		containers: []*cmodel.Container{
			{ExternalID: "pulled", Name: "web", State: cmodel.StateRunning},
			{ExternalID: "built", Name: "app", State: cmodel.StateRunning},
			{ExternalID: "unlisted", Name: "db", State: cmodel.StateRunning},
		},
		digests: map[string][]string{"pulled": {"nginx@sha256:running"}, "built": nil},
	}

	require.NoError(t, syncInventory(context.Background(), &Identity{AgentID: "agent-1"}, rt, spool, testLogger()))

	var entries map[string]*agentpb.ContainerEvent
	require.Eventually(t, func() bool {
		for _, ev := range sink.events() {
			if inv := ev.GetInventory(); inv != nil {
				entries = map[string]*agentpb.ContainerEvent{}
				for _, c := range inv.GetContainers() {
					entries[c.GetContainerId()] = c
				}
				return true
			}
		}
		return false
	}, time.Second, 2*time.Millisecond)

	assert.Equal(t, []string{"nginx@sha256:running"}, entries["pulled"].GetRepoDigests().GetDigests())
	require.NotNil(t, entries["built"].GetRepoDigests(), "an image without registry digests is reported as such")
	assert.Empty(t, entries["built"].GetRepoDigests().GetDigests())
	assert.Nil(t, entries["unlisted"].GetRepoDigests(), "an image the runtime did not list stays unreported")
}

type labeledRuntime struct {
	runtime.Runtime
	results []*docker.DiscoveryResult
}

func (r labeledRuntime) DiscoverAllWithLabels(context.Context) ([]*docker.DiscoveryResult, error) {
	return r.results, nil
}

func TestSyncInventory_CarriesTheExitOfStoppedContainers(t *testing.T) {
	sink := &captureSink{}
	spool := NewSpool(t.TempDir(), SpoolConfig{}, testLogger())
	spool.Attach(sink)
	t.Cleanup(func() { _ = spool.Close() })
	rt := labeledRuntime{results: []*docker.DiscoveryResult{
		{Container: &cmodel.Container{ExternalID: "job", State: cmodel.StateCompleted}, Exit: &docker.ExitInfo{Code: 0}},
		{Container: &cmodel.Container{ExternalID: "stopped", State: cmodel.StateCompleted}, Exit: &docker.ExitInfo{Code: 137}},
		{Container: &cmodel.Container{ExternalID: "oom", State: cmodel.StateExited}, Exit: &docker.ExitInfo{Code: 137, OOMKilled: true}},
		{Container: &cmodel.Container{ExternalID: "web", State: cmodel.StateRunning}},
	}}

	require.NoError(t, syncInventory(context.Background(), &Identity{AgentID: "agent-1"}, rt, spool, testLogger()))

	var entries map[string]*agentpb.ContainerEvent
	require.Eventually(t, func() bool {
		for _, ev := range sink.events() {
			if inv := ev.GetInventory(); inv != nil {
				entries = map[string]*agentpb.ContainerEvent{}
				for _, c := range inv.GetContainers() {
					entries[c.GetContainerId()] = c
				}
				return true
			}
		}
		return false
	}, time.Second, 2*time.Millisecond)

	assert.Equal(t, "0", entries["job"].GetStatusMessage())
	assert.Equal(t, "137", entries["stopped"].GetStatusMessage())
	assert.False(t, entries["stopped"].GetOomKilled())
	assert.Equal(t, "137", entries["oom"].GetStatusMessage())
	assert.True(t, entries["oom"].GetOomKilled())
	assert.Empty(t, entries["web"].GetStatusMessage())
}

// lateRuntime is a Docker-like runtime that answers only once it is up.
type lateRuntime struct {
	runtime.Runtime
	up       atomic.Bool
	attempts atomic.Int32
}

func (r *lateRuntime) TryConnect(context.Context) error {
	r.attempts.Add(1)
	if !r.up.Load() {
		return errors.New("docker ping failed")
	}
	return nil
}

func (r *lateRuntime) DiscoverAll(context.Context) ([]*cmodel.Container, error) { return nil, nil }

func (r *lateRuntime) StreamEvents(context.Context) <-chan runtime.RuntimeEvent { return nil }

func TestRunCollector_ReportsTheHostUntilTheRuntimeAnswers(t *testing.T) {
	prev := resourceSampleInterval
	resourceSampleInterval = 10 * time.Millisecond
	t.Cleanup(func() { resourceSampleInterval = prev })

	sink := &captureSink{}
	spool := NewSpool(t.TempDir(), SpoolConfig{}, testLogger())
	spool.Attach(sink)
	t.Cleanup(func() { _ = spool.Close() })

	rt := &lateRuntime{}
	link := newRuntimeLink(rt, RuntimeDocker)
	_, err := link.attach(context.Background())
	require.Error(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		link.keepTrying(ctx, err, retry.New(time.Millisecond, 5*time.Millisecond, 0), testLogger())
	}()
	go func() {
		defer wg.Done()
		assert.NoError(t, runCollector(ctx, &Identity{AgentID: "agent-1"}, link, "", spool, testLogger()))
	}()

	seen := func() (hostOS, hostSample, inventory bool) {
		for _, ev := range sink.events() {
			hostOS = hostOS || ev.GetHostOs() != nil
			hostSample = hostSample || (ev.GetResource() != nil && ev.GetResource().GetContainerId() == "")
			inventory = inventory || ev.GetInventory() != nil
		}
		return
	}

	require.Eventually(t, func() bool {
		hostOS, hostSample, _ := seen()
		return hostOS && hostSample && rt.attempts.Load() >= 3
	}, 5*time.Second, 5*time.Millisecond, "the host must be reported while the runtime keeps being retried")
	_, _, inventory := seen()
	assert.False(t, inventory, "no container inventory before the runtime answers")
	assert.Empty(t, reportedRuntimes(sink), "no runtime is reported before it answers")

	rt.up.Store(true)
	require.Eventually(t, func() bool {
		_, _, inventory := seen()
		return inventory
	}, 5*time.Second, 5*time.Millisecond, "container monitoring must start as soon as the runtime answers")
	assert.Equal(t, []agentpb.Runtime{agentpb.Runtime_RUNTIME_DOCKER}, reportedRuntimes(sink),
		"the runtime that answered is reported, since enrollment may have happened before")

	cancel()
	wg.Wait()
}

func reportedRuntimes(sink *captureSink) []agentpb.Runtime {
	var out []agentpb.Runtime
	for _, ev := range sink.events() {
		if r := ev.GetRuntime(); r != nil {
			out = append(out, r.GetKind())
		}
	}
	return out
}

func TestRunCollector_ReportsJoiningAndLeavingASwarm(t *testing.T) {
	prev := swarmRecheckInterval
	swarmRecheckInterval = 5 * time.Millisecond
	t.Cleanup(func() { swarmRecheckInterval = prev })

	sink := &captureSink{}
	spool := NewSpool(t.TempDir(), SpoolConfig{}, testLogger())
	spool.Attach(sink)
	t.Cleanup(func() { _ = spool.Close() })

	rt := &lateRuntime{}
	rt.up.Store(true)
	var joined atomic.Bool
	link := newRuntimeLink(rt, RuntimeDocker)
	link.inSwarm = func(context.Context) (bool, error) { return joined.Load(), nil }
	_, err := link.attach(context.Background())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runCollector(ctx, &Identity{AgentID: "agent-1"}, link, "", spool, testLogger()) }()

	last := func() agentpb.Runtime {
		runtimes := reportedRuntimes(sink)
		if len(runtimes) == 0 {
			return agentpb.Runtime_RUNTIME_UNSPECIFIED
		}
		return runtimes[len(runtimes)-1]
	}
	require.Eventually(t, func() bool { return last() == agentpb.Runtime_RUNTIME_DOCKER }, 5*time.Second, 2*time.Millisecond)

	joined.Store(true)
	require.Eventually(t, func() bool { return last() == agentpb.Runtime_RUNTIME_SWARM }, 5*time.Second, 2*time.Millisecond,
		"a host that joins a Swarm must be reported as such without a restart")

	joined.Store(false)
	require.Eventually(t, func() bool { return last() == agentpb.Runtime_RUNTIME_DOCKER }, 5*time.Second, 2*time.Millisecond,
		"a host that leaves its Swarm must be reported as Docker again")

	var cleared bool
	for _, ev := range sink.events() {
		if topo := ev.GetSwarm(); topo != nil && len(topo.GetServices())+len(topo.GetTasks())+len(topo.GetNodes()) == 0 {
			cleared = true
		}
	}
	assert.True(t, cleared, "leaving the Swarm must retire the topology the server holds")

	cancel()
	require.NoError(t, <-done)
}

func TestRuntimeEventToProto_DieCarriesTheOOMFlag(t *testing.T) {
	out := runtimeEventToProto(runtime.RuntimeEvent{Action: "die", ExternalID: "c1", ExitCode: "137", OOMKilled: true})
	require.NotNil(t, out)
	assert.Equal(t, "137", out.GetStatusMessage())
	assert.True(t, out.GetOomKilled())
}

func TestContainerStateToProto(t *testing.T) {
	cases := []struct {
		in    cmodel.ContainerState
		want  agentpb.ContainerState
		valid bool
	}{
		{cmodel.StateRunning, agentpb.ContainerState_CONTAINER_STATE_RUNNING, true},
		{cmodel.StateExited, agentpb.ContainerState_CONTAINER_STATE_EXITED, true},
		{cmodel.StateCompleted, agentpb.ContainerState_CONTAINER_STATE_EXITED, true},
		{cmodel.StatePaused, agentpb.ContainerState_CONTAINER_STATE_PAUSED, true},
		{cmodel.StateRestarting, agentpb.ContainerState_CONTAINER_STATE_RESTARTING, true},
		{cmodel.StateCreated, agentpb.ContainerState_CONTAINER_STATE_CREATED, true},
		{cmodel.StateDead, agentpb.ContainerState_CONTAINER_STATE_DEAD, true},
		{cmodel.ContainerState("bogus"), agentpb.ContainerState_CONTAINER_STATE_UNSPECIFIED, false},
	}
	for _, tc := range cases {
		got, ok := containerStateToProto(tc.in)
		assert.Equal(t, tc.valid, ok, "state %q validity", tc.in)
		assert.Equal(t, tc.want, got, "state %q mapping", tc.in)
	}
}

// lostRuntime answers, then closes its first event stream the way a lost Docker daemon does.
type lostRuntime struct {
	lateRuntime
	mu            sync.Mutex
	streams       []chan runtime.RuntimeEvent
	subscriptions int
	connects      int
}

func (r *lostRuntime) Connect(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connects++
	return nil
}

func (r *lostRuntime) StreamEvents(context.Context) <-chan runtime.RuntimeEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptions++
	if len(r.streams) == 0 {
		return nil
	}
	ch := r.streams[0]
	r.streams = r.streams[1:]
	return ch
}

func TestRunCollector_StartsAgainOnceALostRuntimeAnswers(t *testing.T) {
	lost := make(chan runtime.RuntimeEvent)
	close(lost)
	back := make(chan runtime.RuntimeEvent, 1)
	back <- runtime.RuntimeEvent{Action: "start", ExternalID: "c2", Name: "web", Timestamp: time.Now()}
	rt := &lostRuntime{streams: []chan runtime.RuntimeEvent{lost, back}}
	rt.up.Store(true)
	link := newRuntimeLink(rt, RuntimeDocker)
	_, err := link.attach(context.Background())
	require.NoError(t, err)

	sink := &captureSink{}
	spool := NewSpool(t.TempDir(), SpoolConfig{}, testLogger())
	spool.Attach(sink)
	t.Cleanup(func() { _ = spool.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runCollector(ctx, &Identity{AgentID: "agent-1"}, link, "", spool, testLogger()) }()

	require.Eventually(t, func() bool {
		for _, ev := range sink.events() {
			if ev.GetContainer().GetContainerId() == "c2" {
				return true
			}
		}
		return false
	}, 5*time.Second, 5*time.Millisecond, "events after a runtime loss still reach the server")

	inventories := 0
	for _, ev := range sink.events() {
		if ev.GetInventory() != nil {
			inventories++
		}
	}
	assert.Equal(t, 2, inventories, "the inventory is resent once the runtime answers again")
	rt.mu.Lock()
	assert.Equal(t, 1, rt.connects, "the runtime is waited for before collecting again")
	assert.Equal(t, 2, rt.subscriptions, "one subscription per connection")
	rt.mu.Unlock()

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the collector did not stop with its context")
	}
}

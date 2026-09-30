// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	cmodel "github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
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

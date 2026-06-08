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

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	cmodel "github.com/kolapsis/maintenant/internal/container"
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
	assert.Nil(t, runtimeEventToProto(runtime.RuntimeEvent{Action: "health_status"}))
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

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package multihost

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type broadcast struct {
	eventType string
	data      any
}

type recordingBroadcaster struct{ sent []broadcast }

func (b *recordingBroadcaster) BroadcastEvent(eventType string, data any) {
	b.sent = append(b.sent, broadcast{eventType, data})
}

func (b *recordingBroadcaster) take() []broadcast {
	out := b.sent
	b.sent = nil
	return out
}

func TestDispatcher_RuntimeReportUpdatesTheDetectedRuntime(t *testing.T) {
	ctx := context.Background()
	agents := store.NewAgentStore(storetest.Open(t, slog.New(slog.NewTextHandler(io.Discard, nil))))
	insertActiveAgents(t, agents, 1)
	agentID := "00000000-0000-0000-0000-000000000001"

	sink := &recordingBroadcaster{}
	sessions := NewSessions(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	d := NewDispatcher(DispatchDeps{Runtime: runtimeRecorder{store: agents, broadcaster: sink, sessions: sessions}})
	report := func(kind agentpb.Runtime) error {
		return d.Dispatch(ctx, agentID, &agentpb.AgentEvent{
			AgentId: agentID,
			Body:    &agentpb.AgentEvent_Runtime{Runtime: &agentpb.RuntimeMsg{Kind: kind}},
		})
	}
	stored := func() string {
		a, err := agents.Get(ctx, agentID)
		require.NoError(t, err)
		return a.DetectedRuntime
	}

	require.NoError(t, report(agentpb.Runtime_RUNTIME_SWARM))
	assert.Equal(t, "swarm", stored(), "the runtime reported after enrollment replaces the enrolled one")
	assert.Equal(t, []broadcast{{event.AgentUpdated, map[string]any{"agent_id": agentID, "detected_runtime": "swarm"}}}, sink.take())

	require.NoError(t, report(agentpb.Runtime_RUNTIME_SWARM))
	assert.Empty(t, sink.take(), "an unchanged runtime is not announced again")

	require.NoError(t, report(agentpb.Runtime_RUNTIME_DOCKER))
	assert.Equal(t, "docker", stored(), "leaving the Swarm is reported too")
	assert.Len(t, sink.take(), 1)

	require.Error(t, report(agentpb.Runtime_RUNTIME_UNSPECIFIED))
	require.Error(t, report(agentpb.Runtime(42)))
	assert.Equal(t, "docker", stored(), "a runtime the server does not know leaves the stored one alone")
	assert.Empty(t, sink.take())
}

func TestDispatcher_RuntimeReportMarksTheRuntimeUnreachableForTheSession(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	agents := store.NewAgentStore(storetest.Open(t, logger))
	insertActiveAgents(t, agents, 1)
	agentID := "00000000-0000-0000-0000-000000000001"

	sessions := NewSessions(logger, nil)
	d := NewDispatcher(DispatchDeps{Runtime: runtimeRecorder{store: agents, broadcaster: &recordingBroadcaster{}, sessions: sessions}})
	report := func(msg *agentpb.RuntimeMsg) {
		t.Helper()
		require.NoError(t, d.Dispatch(ctx, agentID, &agentpb.AgentEvent{AgentId: agentID, Body: &agentpb.AgentEvent_Runtime{Runtime: msg}}))
	}

	sessions.Open(agentID, func(error) {}, "1.2.3.4:1", nil, nil)
	report(&agentpb.RuntimeMsg{Kind: agentpb.Runtime_RUNTIME_DOCKER})
	assert.False(t, sessions.RuntimeUnreachable(agentID), "a report without the field, as an older agent sends it, leaves the runtime reachable")

	report(&agentpb.RuntimeMsg{Kind: agentpb.Runtime_RUNTIME_DOCKER, Unreachable: true})
	assert.True(t, sessions.RuntimeUnreachable(agentID))
	assert.True(t, sessions.IsConnected(agentID))

	report(&agentpb.RuntimeMsg{Kind: agentpb.Runtime_RUNTIME_DOCKER})
	assert.False(t, sessions.RuntimeUnreachable(agentID), "the runtime is back")

	report(&agentpb.RuntimeMsg{Kind: agentpb.Runtime_RUNTIME_DOCKER, Unreachable: true})
	sessions.Open(agentID, func(error) {}, "1.2.3.4:2", nil, nil)
	assert.False(t, sessions.RuntimeUnreachable(agentID), "a new stream starts from what the agent states on it")

	sessions.Close(agentID, "stream_ended")
	assert.False(t, sessions.RuntimeUnreachable(agentID))
}

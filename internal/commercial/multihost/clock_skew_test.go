// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package multihost

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
)

func eventAhead(ahead time.Duration) *agentpb.AgentEvent {
	return &agentpb.AgentEvent{
		AgentId:    dispatchAgentID,
		ObservedAt: timestamppb.New(time.Now().Add(ahead)),
		Body:       &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{ContainerId: "ctr-ahead"}},
	}
}

func TestDispatcher_AcceptsAnEventWithinTheHandshakeSkew(t *testing.T) {
	h := &mockContainerHandler{}
	d := NewDispatcher(DispatchDeps{Container: h})

	require.NoError(t, d.Dispatch(context.Background(), dispatchAgentID, eventAhead(200*time.Second)))
	assert.NotNil(t, h.calledWithEvent, "an agent the handshake lets in must have its events taken")
	assert.Zero(t, d.RejectedEvents(dispatchAgentID))
}

func TestDispatcher_EventBeyondTheHandshakeSkewIsInvalid(t *testing.T) {
	h := &mockContainerHandler{}
	d := NewDispatcher(DispatchDeps{Container: h})

	err := d.Dispatch(context.Background(), dispatchAgentID, eventAhead(maxClockSkew+time.Minute))
	require.ErrorIs(t, err, agentevent.ErrInvalid, "a clock rejection is final: replaying the event cannot change it")
	assert.Nil(t, h.calledWithEvent)
	assert.Equal(t, uint64(1), d.RejectedEvents(dispatchAgentID))
}

func TestSessions_ReportTheDispatcherRejections(t *testing.T) {
	sessions := NewSessions(slog.New(slog.DiscardHandler), nil)
	d := NewDispatcher(DispatchDeps{})
	New(Deps{Sessions: sessions, Dispatcher: d, Logger: slog.New(slog.DiscardHandler)})

	require.Error(t, d.Dispatch(context.Background(), dispatchAgentID, eventAhead(maxClockSkew+time.Minute)))

	reporter, ok := any(sessions).(interface{ RejectedEvents(string) uint64 })
	require.True(t, ok, "the session registry the API reads must report rejections")
	assert.Equal(t, uint64(1), reporter.RejectedEvents(dispatchAgentID))
	assert.Zero(t, reporter.RejectedEvents("another-agent"))
}

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

package agentserver_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testBroadcaster struct {
	mu     sync.Mutex
	events []struct {
		Type string
		Data any
	}
}

func (b *testBroadcaster) BroadcastEvent(eventType string, data any) {
	b.mu.Lock()
	b.events = append(b.events, struct {
		Type string
		Data any
	}{eventType, data})
	b.mu.Unlock()
}

func (b *testBroadcaster) hasEvent(eventType string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.events {
		if e.Type == eventType {
			return true
		}
	}
	return false
}

func TestSessions_OpenClose_CancelAndSSE(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	broadcaster := &testBroadcaster{}
	sessions := agentserver.NewSessions(logger, broadcaster)

	cancelled := false
	var cause error
	cancel := context.CancelCauseFunc(func(err error) { cancelled, cause = true, err })

	sessions.Open("agent-A", cancel, "1.2.3.4:1234", nil, nil)
	assert.True(t, sessions.IsConnected("agent-A"))
	assert.True(t, broadcaster.hasEvent("agent.connected"), "should emit agent.connected on Open")

	sessions.Close("agent-A", "revoked")
	assert.True(t, cancelled, "cancel should be called on Close")
	assert.ErrorIs(t, cause, agentserver.ErrSessionRevoked,
		"a revocation must reach the agent as permanent, unlike a stale reap")
	assert.False(t, sessions.IsConnected("agent-A"))
	assert.True(t, broadcaster.hasEvent("agent.disconnected"), "should emit agent.disconnected on Close")
}

func TestSessions_Close_NotConnected_NoSSE(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	broadcaster := &testBroadcaster{}
	sessions := agentserver.NewSessions(logger, broadcaster)

	// Closing an agent that was never opened should be a no-op.
	sessions.Close("ghost-agent", "deleted")
	assert.False(t, broadcaster.hasEvent("agent.disconnected"), "no SSE for unknown agent")
}

func TestSessions_Open_ReplacesExistingStream(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	sessions := agentserver.NewSessions(logger, nil)

	firstCancelled := false
	sessions.Open("agent-A", func(error) { firstCancelled = true }, "addr1", nil, nil)
	assert.False(t, firstCancelled)

	// Opening again should cancel the first stream.
	sessions.Open("agent-A", func(error) {}, "addr2", nil, nil)
	assert.True(t, firstCancelled, "first cancel should be called when re-opened")
	assert.True(t, sessions.IsConnected("agent-A"))
}

func TestSessions_ReOpenAfterClose(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	sessions := agentserver.NewSessions(logger, nil)

	sessions.Open("agent-A", func(error) {}, "addr1", nil, nil)
	sessions.Close("agent-A", "revoked")
	assert.False(t, sessions.IsConnected("agent-A"))

	// Re-enroll path: open again after close.
	sessions.Open("agent-A", func(error) {}, "addr2", nil, nil)
	require.True(t, sessions.IsConnected("agent-A"), "should be connected after re-open")
}

func TestSessions_LifecycleAlertHook(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	sessions := agentserver.NewSessions(logger, nil)

	type call struct {
		agentID   string
		reason    string
		connected bool
	}
	var mu sync.Mutex
	var calls []call
	sessions.SetLifecycleAlertHook(func(agentID, reason string, connected bool) {
		mu.Lock()
		calls = append(calls, call{agentID, reason, connected})
		mu.Unlock()
	})

	sessions.Open("agent-A", func(error) {}, "addr", nil, nil)
	sessions.Close("agent-A", "stream_ended")
	// A real stream drop on an agent with no live session must NOT fire (no phantom alert).
	sessions.Close("ghost-drop", "stream_ended")
	// Intentional removal (delete/revoke) of an already-offline agent MUST fire the
	// hook so its lingering disconnect alert is resolved even without a session.
	sessions.Close("ghost-deleted", "deleted")
	sessions.Close("ghost-revoked", "revoked")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, calls, 4, "terminal-reason closes must fire even with no session; a bare drop must not")
	assert.Equal(t, call{"agent-A", "", true}, calls[0], "Open should fire connected=true")
	assert.Equal(t, call{"agent-A", "stream_ended", false}, calls[1], "Close should pass reason and connected=false")
	assert.Equal(t, call{"ghost-deleted", "deleted", false}, calls[2], "delete of offline agent resolves its alert")
	assert.Equal(t, call{"ghost-revoked", "revoked", false}, calls[3], "revoke of offline agent resolves its alert")
}

func TestSessions_EventsPerSecond5m(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	sessions := agentserver.NewSessions(logger, nil)

	// Without any events the rate should be zero.
	assert.Equal(t, 0.0, sessions.EventsPerSecond5m())

	sessions.Open("agent-A", func(error) {}, "addr", nil, nil)
	for range 300 {
		sessions.IncrEvents("agent-A")
	}
	// 300 events in the current 5s bucket → rate = 300/300 = 1.0 events/s
	assert.Equal(t, 1.0, sessions.EventsPerSecond5m())
}

// A session torn down for anything other than a real revocation must reach the
// agent as retryable. Reporting a stale reap or a replaced stream as
// "agent_revoked" made the agent exit for good over a transient hiccup.
func TestSessions_CauseForReason_OnlyRevocationIsPermanent(t *testing.T) {
	assert.ErrorIs(t, agentserver.CauseForReason("revoked"), agentserver.ErrSessionRevoked)
	assert.ErrorIs(t, agentserver.CauseForReason("deleted"), agentserver.ErrSessionRevoked)

	for _, reason := range []string{"stale", "stream_ended", ""} {
		cause := agentserver.CauseForReason(reason)
		assert.NotErrorIs(t, cause, agentserver.ErrSessionRevoked,
			"reason %q must not read as a revocation", reason)
	}
	assert.ErrorIs(t, agentserver.CauseForReason("stale"), agentserver.ErrSessionStale)
}

func TestSessions_ReplacedStreamIsNotARevocation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	sessions := agentserver.NewSessions(logger, &testBroadcaster{})

	var cause error
	sessions.Open("agent-A", func(err error) { cause = err }, "addr1", nil, nil)
	sessions.Open("agent-A", func(error) {}, "addr2", nil, nil)

	assert.ErrorIs(t, cause, agentserver.ErrSessionReplaced)
	assert.NotErrorIs(t, cause, agentserver.ErrSessionRevoked,
		"a reconnect that replaces the old stream must not look like a revocation")
}

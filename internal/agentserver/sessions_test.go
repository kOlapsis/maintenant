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
	cancel := context.CancelFunc(func() { cancelled = true })

	sessions.Open("agent-A", cancel, "1.2.3.4:1234")
	assert.True(t, sessions.IsConnected("agent-A"))
	assert.True(t, broadcaster.hasEvent("agent.connected"), "should emit agent.connected on Open")

	sessions.Close("agent-A", "revoked")
	assert.True(t, cancelled, "cancel should be called on Close")
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
	sessions.Open("agent-A", func() { firstCancelled = true }, "addr1")
	assert.False(t, firstCancelled)

	// Opening again should cancel the first stream.
	sessions.Open("agent-A", func() {}, "addr2")
	assert.True(t, firstCancelled, "first cancel should be called when re-opened")
	assert.True(t, sessions.IsConnected("agent-A"))
}

func TestSessions_ReOpenAfterClose(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	sessions := agentserver.NewSessions(logger, nil)

	sessions.Open("agent-A", func() {}, "addr1")
	sessions.Close("agent-A", "revoked")
	assert.False(t, sessions.IsConnected("agent-A"))

	// Re-enroll path: open again after close.
	sessions.Open("agent-A", func() {}, "addr2")
	require.True(t, sessions.IsConnected("agent-A"), "should be connected after re-open")
}

func TestSessions_EventsPerSecond5m(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	sessions := agentserver.NewSessions(logger, nil)

	// Without any events the rate should be zero.
	assert.Equal(t, 0.0, sessions.EventsPerSecond5m())

	sessions.Open("agent-A", func() {}, "addr")
	for range 300 {
		sessions.IncrEvents("agent-A")
	}
	// 300 events in the current 5s bucket → rate = 300/300 = 1.0 events/s
	assert.Equal(t, 1.0, sessions.EventsPerSecond5m())
}

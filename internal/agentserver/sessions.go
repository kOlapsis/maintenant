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

package agentserver

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// activeStream holds the live state of a connected agent stream.
type activeStream struct {
	cancel      context.CancelFunc
	addr        string
	connectedAt time.Time
	eventsSeen  atomic.Int64
}

// Sessions tracks all currently connected agent streams.
type Sessions struct {
	mu     sync.RWMutex
	active map[string]*activeStream
	logger *slog.Logger
}

// NewSessions creates an empty Sessions registry.
func NewSessions(logger *slog.Logger) *Sessions {
	return &Sessions{
		active: make(map[string]*activeStream),
		logger: logger,
	}
}

// Open registers an active stream for agentID and cancels any pre-existing one.
func (s *Sessions) Open(agentID string, cancel context.CancelFunc, addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.active[agentID]; ok {
		existing.cancel()
	}
	s.active[agentID] = &activeStream{
		cancel:      cancel,
		addr:        addr,
		connectedAt: time.Now(),
	}
	s.logger.Info("agent.connected", "agent_id", agentID, "addr", addr)
}

// Close removes the active stream for agentID and cancels it.
func (s *Sessions) Close(agentID, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if stream, ok := s.active[agentID]; ok {
		stream.cancel()
		delete(s.active, agentID)
		s.logger.Info("agent.stream_closed", "agent_id", agentID, "reason", reason)
	}
}

// IsConnected reports whether agentID currently has an active stream.
func (s *Sessions) IsConnected(agentID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.active[agentID]
	return ok
}

// ListConnected returns all currently connected agent IDs.
func (s *Sessions) ListConnected() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.active))
	for id := range s.active {
		ids = append(ids, id)
	}
	return ids
}

// IncrEvents increments the events_seen counter for agentID.
func (s *Sessions) IncrEvents(agentID string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if stream, ok := s.active[agentID]; ok {
		stream.eventsSeen.Add(1)
	}
}

// EventsSeen returns the events_seen counter for agentID (0 if not connected).
func (s *Sessions) EventsSeen(agentID string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if stream, ok := s.active[agentID]; ok {
		return stream.eventsSeen.Load()
	}
	return 0
}

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

	"github.com/kolapsis/maintenant/internal/event"
)

// activeStream holds the live state of a connected agent stream.
type activeStream struct {
	cancel      context.CancelFunc
	addr        string
	connectedAt time.Time
	eventsSeen  atomic.Int64
}

// ringBuffer tracks events in 60 slots of 5s each (= 5 minutes).
type ringBuffer struct {
	mu     sync.Mutex
	slots  [60]int64
	cursor int
}

func (rb *ringBuffer) add() {
	rb.mu.Lock()
	rb.slots[rb.cursor]++
	rb.mu.Unlock()
}

func (rb *ringBuffer) advance() {
	rb.mu.Lock()
	rb.cursor = (rb.cursor + 1) % 60
	rb.slots[rb.cursor] = 0
	rb.mu.Unlock()
}

// Rate returns events per second averaged over the last 5 minutes.
func (rb *ringBuffer) Rate() float64 {
	rb.mu.Lock()
	var total int64
	for _, v := range rb.slots {
		total += v
	}
	rb.mu.Unlock()
	return float64(total) / 300.0
}

// Sessions tracks all currently connected agent streams.
type Sessions struct {
	mu          sync.RWMutex
	active      map[string]*activeStream
	logger      *slog.Logger
	broadcaster EventBroadcaster
	ring        ringBuffer
}

// NewSessions creates an empty Sessions registry.
// broadcaster may be nil (SSE events will not be emitted).
func NewSessions(logger *slog.Logger, broadcaster EventBroadcaster) *Sessions {
	return &Sessions{
		active:      make(map[string]*activeStream),
		logger:      logger,
		broadcaster: broadcaster,
	}
}

// Open registers an active stream for agentID and cancels any pre-existing one.
func (s *Sessions) Open(agentID string, cancel context.CancelFunc, addr string) {
	s.mu.Lock()
	if existing, ok := s.active[agentID]; ok {
		existing.cancel()
	}
	s.active[agentID] = &activeStream{
		cancel:      cancel,
		addr:        addr,
		connectedAt: time.Now(),
	}
	s.mu.Unlock()

	s.logger.Info("agent.connected", "agent_id", agentID, "addr", addr)
	if s.broadcaster != nil {
		s.broadcaster.BroadcastEvent(event.AgentConnected, map[string]any{
			"agent_id": agentID,
		})
	}
}

// Close removes and cancels the active stream for agentID.
func (s *Sessions) Close(agentID, reason string) {
	s.mu.Lock()
	_, had := s.active[agentID]
	if had {
		s.active[agentID].cancel()
		delete(s.active, agentID)
	}
	s.mu.Unlock()

	if had {
		s.logger.Info("agent.stream_closed", "agent_id", agentID, "reason", reason)
		if s.broadcaster != nil {
			s.broadcaster.BroadcastEvent(event.AgentDisconnected, map[string]any{
				"agent_id": agentID,
			})
		}
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

// IncrEvents increments the events_seen counter for agentID and the ring buffer.
func (s *Sessions) IncrEvents(agentID string) {
	s.mu.RLock()
	if stream, ok := s.active[agentID]; ok {
		stream.eventsSeen.Add(1)
	}
	s.mu.RUnlock()
	s.ring.add()
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

// EventsPerSecond5m returns the average events/s over the last 5 minutes.
func (s *Sessions) EventsPerSecond5m() float64 {
	return s.ring.Rate()
}

// StartRingAdvancer runs the ring buffer advance tick every 5s until ctx is done.
func (s *Sessions) StartRingAdvancer(ctx context.Context) {
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.ring.advance()
			}
		}
	}()
}

// StaleAgentsFn returns agent IDs whose last_seen_at is older than threshold.
// Passed to StartStaleWatcher so sessions doesn't import the sqlite package.
type StaleAgentsFn func(ctx context.Context, threshold time.Duration) ([]string, error)

// StartStaleWatcher emits agent.disconnected SSE for agents that have not been
// seen within threshold but still appear in the sessions map (dead stream).
// Runs until ctx is done.
func (s *Sessions) StartStaleWatcher(ctx context.Context, interval, threshold time.Duration, staleAgents StaleAgentsFn) {
	if s.broadcaster == nil || staleAgents == nil {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				ids, err := staleAgents(ctx, threshold)
				if err != nil {
					s.logger.Error("stale watcher query failed", "err", err)
					continue
				}
				for _, agentID := range ids {
					s.mu.Lock()
					if _, connected := s.active[agentID]; connected {
						s.active[agentID].cancel()
						delete(s.active, agentID)
						s.mu.Unlock()
						s.logger.Info("agent.stream_closed", "agent_id", agentID, "reason", "stale")
						s.broadcaster.BroadcastEvent(event.AgentDisconnected, map[string]any{
							"agent_id": agentID,
						})
					} else {
						s.mu.Unlock()
					}
				}
			}
		}
	}()
}

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
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Reasons a session is torn down. They travel to the Push handler as the stream
// context's cancellation cause, so it can tell the agent whether to give up or
// reconnect — reporting every teardown as a revocation would make an agent that
// was merely reaped as stale exit for good.
var (
	ErrSessionRevoked  = errors.New("agent_revoked")
	ErrSessionReplaced = errors.New("session_replaced")
	ErrSessionStale    = errors.New("session_stale")
	ErrSessionClosed   = errors.New("session_closed")
)

// CauseForReason maps a Close reason to the cause handed to the agent.
func CauseForReason(reason string) error {
	switch reason {
	case "revoked", "deleted":
		return ErrSessionRevoked
	case "stale":
		return ErrSessionStale
	default:
		return ErrSessionClosed
	}
}

// activeStream holds the live state of a connected agent stream.
type activeStream struct {
	cancel      context.CancelCauseFunc
	addr        string
	connectedAt time.Time
	eventsSeen  atomic.Int64

	// send is drained by the stream's own Push goroutine, which is the only
	// writer to the gRPC stream. Callers outside that goroutine (HTTP handlers
	// issuing commands) enqueue here instead of touching the stream.
	send chan *agentpb.ServerMessage
	caps map[string]struct{}

	// pending correlates in-flight command request ids to their reply channel.
	// Its lifetime is the stream's, so a disconnect frees every waiter.
	mu      sync.Mutex
	pending map[string]chan *agentpb.CommandResult
	closed  bool
}

// deliver hands res to the waiter for its request id, dropping it if nobody
// waits (a cancelled or already-completed request).
func (a *activeStream) deliver(res *agentpb.CommandResult) {
	a.mu.Lock()
	ch, ok := a.pending[res.GetRequestId()]
	if ok && res.GetLast() {
		delete(a.pending, res.GetRequestId())
	}
	a.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- res:
	default:
		// Reader fell behind; dropping beats stalling the stream's recv loop.
	}
	if res.GetLast() {
		close(ch)
	}
}

// closeAll releases every waiter, unblocking readers when the stream dies.
func (a *activeStream) closeAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	a.closed = true
	for id, ch := range a.pending {
		close(ch)
		delete(a.pending, id)
	}
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

	// alertHook, if set, is invoked on every connect/disconnect transition so
	// the app layer can raise an alert (connected=false) or clear one
	// (connected=true). reason carries the disconnect cause ("stream_ended",
	// "stale", "revoked", "deleted"). Always called WITHOUT s.mu held.
	alertHook func(agentID, reason string, connected bool)
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

// SetLifecycleAlertHook registers a callback fired on connect/disconnect
// transitions. Must be called once at wiring time, before any agent connects.
func (s *Sessions) SetLifecycleAlertHook(fn func(agentID, reason string, connected bool)) {
	s.alertHook = fn
}

// Open registers an active stream for agentID and cancels any pre-existing one.
// caps are the command families the agent advertised; send is the queue its Push
// goroutine drains to write to the stream (both may be nil for a telemetry-only
// stream, in which case no command can be issued to this agent).
func (s *Sessions) Open(agentID string, cancel context.CancelCauseFunc, addr string, caps []string, send chan *agentpb.ServerMessage) {
	capSet := make(map[string]struct{}, len(caps))
	for _, c := range caps {
		capSet[c] = struct{}{}
	}

	s.mu.Lock()
	if existing, ok := s.active[agentID]; ok {
		existing.cancel(ErrSessionReplaced)
		existing.closeAll()
	}
	s.active[agentID] = &activeStream{
		cancel:      cancel,
		addr:        addr,
		connectedAt: time.Now(),
		send:        send,
		caps:        capSet,
		pending:     make(map[string]chan *agentpb.CommandResult),
	}
	s.mu.Unlock()

	s.logger.Info("agent.connected", "agent_id", agentID, "addr", addr)
	if s.broadcaster != nil {
		s.broadcaster.BroadcastEvent(event.AgentConnected, map[string]any{
			"agent_id": agentID,
		})
	}
	if s.alertHook != nil {
		s.alertHook(agentID, "", true)
	}
}

// Close removes and cancels the active stream for agentID.
func (s *Sessions) Close(agentID, reason string) {
	s.mu.Lock()
	st, had := s.active[agentID]
	if had {
		st.cancel(CauseForReason(reason))
		delete(s.active, agentID)
	}
	s.mu.Unlock()

	if had {
		st.closeAll()
	}

	if had {
		s.logger.Info("agent.stream_closed", "agent_id", agentID, "reason", reason)
		if s.broadcaster != nil {
			s.broadcaster.BroadcastEvent(event.AgentDisconnected, map[string]any{
				"agent_id": agentID,
			})
		}
	}

	// Fire the lifecycle hook on a real stream close (had) OR on intentional
	// removal (revoke/delete), so a pending disconnect alert is resolved even
	// when the agent was already offline with no live session to close.
	if s.alertHook != nil && (had || reason == "revoked" || reason == "deleted") {
		s.alertHook(agentID, reason, false)
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

// CapabilityLogs is advertised by agents able to serve container logs on demand.
const CapabilityLogs = "logs"

// maxInFlightPerAgent caps concurrent commands per agent, so a burst of log
// viewers cannot exhaust the agent's worker budget or our own memory.
const maxInFlightPerAgent = 8

// commandSendTimeout bounds how long we wait for room in the stream's send queue.
// Exceeding it means the agent's writer is wedged, not that it is merely busy.
const commandSendTimeout = 5 * time.Second

// Errors returned by the command path, mapped to HTTP status by the API layer.
var (
	ErrAgentNotConnected = errors.New("agent not connected")
	ErrAgentCannotServe  = errors.New("agent does not support this command")
	ErrTooManyRequests   = errors.New("too many in-flight commands for this agent")
)

// HasCapability reports whether the agent's live stream advertised capability.
func (s *Sessions) HasCapability(agentID, capability string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.active[agentID]
	if !ok {
		return false
	}
	_, has := st.caps[capability]
	return has
}

// SendCommand issues cmd to agentID and returns a channel carrying its replies,
// closed once the terminal result arrives or the stream dies. The returned
// release func MUST be called by the caller (defer): it drops the pending entry
// and, when the command may still be running, tells the agent to stop.
func (s *Sessions) SendCommand(ctx context.Context, agentID, capability string, cmd *agentpb.AgentCommand) (<-chan *agentpb.CommandResult, func(), error) {
	s.mu.RLock()
	st, ok := s.active[agentID]
	s.mu.RUnlock()
	if !ok || st.send == nil {
		return nil, nil, ErrAgentNotConnected
	}
	if _, has := st.caps[capability]; !has {
		return nil, nil, ErrAgentCannotServe
	}

	requestID := cmd.GetRequestId()
	ch := make(chan *agentpb.CommandResult, 32)

	st.mu.Lock()
	if st.closed {
		st.mu.Unlock()
		return nil, nil, ErrAgentNotConnected
	}
	if len(st.pending) >= maxInFlightPerAgent {
		st.mu.Unlock()
		return nil, nil, ErrTooManyRequests
	}
	st.pending[requestID] = ch
	st.mu.Unlock()

	release := func() {
		st.mu.Lock()
		_, stillPending := st.pending[requestID]
		delete(st.pending, requestID)
		st.mu.Unlock()
		// Still pending means the agent never sent a terminal result, so it may
		// be streaming: tell it to stop rather than leak a follow on its side.
		if stillPending {
			// Best-effort: a stream that already died has nothing left to cancel.
			_ = s.enqueue(context.Background(), st, &agentpb.ServerMessage{
				Payload: &agentpb.ServerMessage_Command{Command: &agentpb.AgentCommand{
					RequestId: requestID,
					Command:   &agentpb.AgentCommand_Cancel{Cancel: &agentpb.CancelRequest{}},
				}},
			})
		}
	}

	if err := s.enqueue(ctx, st, &agentpb.ServerMessage{
		Payload: &agentpb.ServerMessage_Command{Command: cmd},
	}); err != nil {
		release()
		return nil, nil, err
	}

	return ch, release, nil
}

// enqueue hands msg to the stream's writer goroutine without ever blocking
// indefinitely: a wedged agent must fail the request, not pin the caller.
func (s *Sessions) enqueue(ctx context.Context, st *activeStream, msg *agentpb.ServerMessage) error {
	timer := time.NewTimer(commandSendTimeout)
	defer timer.Stop()
	select {
	case st.send <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrAgentNotConnected
	}
}

// logsTailDefault and logsTailMax bound a tail request; the agent clamps too, but
// bounding here keeps the uint32 conversion provably safe.
const (
	logsTailDefault = 100
	logsTailMax     = 500
)

// LogsCommand builds a logs command with a fresh request id. Exported because the
// SSE follow path drives SendCommand directly to stream chunks as they arrive.
func LogsCommand(externalID string, lines int, timestamps, follow bool) *agentpb.AgentCommand {
	return &agentpb.AgentCommand{
		RequestId: uid.New(),
		Command: &agentpb.AgentCommand_Logs{Logs: &agentpb.LogsRequest{
			ContainerId: externalID,
			Lines:       clampTail(lines),
			Timestamps:  timestamps,
			Follow:      follow,
		}},
	}
}

// clampTail narrows a caller-supplied tail length to the wire's uint32. Written
// as early returns so both constant bounds directly guard the conversion: with
// the checks written as reassignments instead, static analysis cannot see that
// the converted value is already in range.
func clampTail(lines int) uint32 {
	if lines <= 0 {
		return logsTailDefault
	}
	if lines >= logsTailMax {
		return logsTailMax
	}
	return uint32(lines)
}

// FetchLogs performs a one-shot log tail on agentID, collecting the agent's chunks
// into a single slice. Shared by the REST and MCP read paths.
func (s *Sessions) FetchLogs(ctx context.Context, agentID, externalID string, lines int, timestamps bool) ([]string, error) {
	results, release, err := s.SendCommand(ctx, agentID, CapabilityLogs,
		LogsCommand(externalID, lines, timestamps, false))
	if err != nil {
		return nil, err
	}
	defer release()

	var collected []string
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res, ok := <-results:
			if !ok {
				// Closed with no terminal result: the stream died mid-answer.
				if len(collected) > 0 {
					return collected, nil
				}
				return nil, ErrAgentNotConnected
			}
			if res.GetErrorCode() != "" {
				return nil, errors.New(res.GetErrorMessage())
			}
			collected = append(collected, res.GetLogs().GetLines()...)
			if res.GetLast() {
				return collected, nil
			}
		}
	}
}

// DeliverResult routes a CommandResult from agentID to whoever awaits it.
func (s *Sessions) DeliverResult(agentID string, res *agentpb.CommandResult) {
	s.mu.RLock()
	st, ok := s.active[agentID]
	s.mu.RUnlock()
	if ok {
		st.deliver(res)
	}
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
					if st, connected := s.active[agentID]; connected {
						st.cancel(ErrSessionStale)
						delete(s.active, agentID)
						s.mu.Unlock()
						st.closeAll()
						s.logger.Info("agent.stream_closed", "agent_id", agentID, "reason", "stale")
						s.broadcaster.BroadcastEvent(event.AgentDisconnected, map[string]any{
							"agent_id": agentID,
						})
						if s.alertHook != nil {
							s.alertHook(agentID, "stale", false)
						}
					} else {
						s.mu.Unlock()
					}
				}
			}
		}
	}()
}

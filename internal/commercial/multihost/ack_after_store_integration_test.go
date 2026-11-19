// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

//go:build integration

package multihost_test

import (
	"context"
	"database/sql/driver"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/commercial/multihost"
	"github.com/kolapsis/maintenant/internal/store"
)

// scriptedContainers stores container events, failing the ones the test names.
type scriptedContainers struct {
	mu       sync.Mutex
	arrived  []string
	stored   map[string]int
	failOnce map[string]bool
	invalid  map[string]bool
}

func newScriptedContainers() *scriptedContainers {
	return &scriptedContainers{stored: map[string]int{}, failOnce: map[string]bool{}, invalid: map[string]bool{}}
}

func (s *scriptedContainers) HandleAgentEvent(_ context.Context, _ string, ev *agentpb.ContainerEvent, _ multihost.EventMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := ev.GetContainerId()
	s.arrived = append(s.arrived, id)
	switch {
	case s.invalid[id]:
		return fmt.Errorf("%w: container %s refused", agentevent.ErrInvalid, id)
	case s.failOnce[id]:
		delete(s.failOnce, id)
		return fmt.Errorf("update container %s: %w", id, driver.ErrBadConn)
	}
	s.stored[id]++
	return nil
}

func (s *scriptedContainers) storedCount(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stored[id]
}

func (s *scriptedContainers) arrivals() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.arrived)
}

// containerEvents builds n container events ctr-1..ctr-n, observed now or ahead by the given offset.
func containerEvents(n int, ahead map[int]time.Duration) []*agentpb.AgentEvent {
	out := make([]*agentpb.AgentEvent, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, &agentpb.AgentEvent{
			EventId:    uuid.NewString(),
			ObservedAt: timestamppb.New(time.Now().Add(ahead[i])),
			Body: &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{
				ContainerId: fmt.Sprintf("ctr-%d", i),
				State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
			}},
		})
	}
	return out
}

type spooledAgent struct {
	spool   *agent.Spool
	streams atomic.Int32
	lastAck atomic.Uint64

	mu          sync.Mutex
	disconnects []string
}

func (a *spooledAgent) disconnectReasons() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.disconnects)
}

// startSpooledAgent serves h over gRPC and streams events to it through a real
// agent spool and reconnect loop; onAck sees every acknowledgement first.
func startSpooledAgent(t *testing.T, h *scriptedContainers, events []*agentpb.AgentEvent, onAck func(uint64)) *spooledAgent {
	t.Helper()
	withMultiHostEdition(t)
	logger := quietLogger()

	tlsCfg, err := multihost.PersistentSelfSigned(t.TempDir(), []string{"127.0.0.1"}, logger)
	require.NoError(t, err)
	a := &spooledAgent{}
	sessions := multihost.NewSessions(logger, noopBroadcaster{})
	sessions.SetLifecycleAlertHook(func(_, reason string, connected bool) {
		if !connected {
			a.mu.Lock()
			a.disconnects = append(a.disconnects, reason)
			a.mu.Unlock()
		}
	})
	st := store.NewAgentStore(openIntegrationDB(t))
	srv := multihost.New(multihost.Deps{
		AgentStore: st, Broadcaster: noopBroadcaster{}, Sessions: sessions,
		Limiter: multihost.NewLimiter(1000), Dispatcher: multihost.NewDispatcher(multihost.DispatchDeps{Container: h}),
		Logger: logger,
	})
	addr := freeAddr(t)
	srvCtx, stopServer := context.WithCancel(context.Background())
	t.Cleanup(stopServer)
	require.NoError(t, srv.Start(srvCtx, addr, tlsCfg))

	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	id, err := agent.LoadOrCreate(dir)
	require.NoError(t, err)
	client, err := agent.NewClientWithFingerprint(ctx, "grpcs://"+addr, sha256Hex(tlsCfg.Certificates[0].Certificate[0]), false, logger)
	require.NoError(t, err)
	const token = "mnt_enr_ackafterstore01"
	storeToken(t, st, token)
	require.NoError(t, agent.RunEnrollment(ctx, id, dir, token, "docker", "ack-test", "1.0.0", client))

	a.spool = agent.NewSpool(dir, agent.SpoolConfig{MaxMemoryBytes: 1 << 20, MaxDiskBytes: 64 << 20}, logger)
	for _, evt := range events {
		evt.AgentId = id.AgentID
		require.NoError(t, a.spool.Send(evt))
	}

	hooks := agent.StreamHooks{
		Acked: func(seq uint64) {
			if onAck != nil {
				onAck(seq)
			}
			a.lastAck.Store(max(a.lastAck.Load(), seq))
			a.spool.Acked(seq)
		},
		RateLimited: a.spool.RateLimited,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = agent.RunWithReconnect(ctx, client, id, logger, hooks, func(ctx context.Context, stream *agent.PushStream) error {
			a.streams.Add(1)
			a.spool.Attach(stream)
			defer a.spool.Detach()
			return a.spool.Drain(ctx)
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		_ = a.spool.Close()
		_ = client.Close()
	})
	return a
}

// waitAcked waits until the server acknowledged seq and the agent purged everything.
func (a *spooledAgent) waitAcked(t *testing.T, seq uint64) {
	t.Helper()
	require.Eventually(t, func() bool {
		depth, err := a.spool.Depth()
		return err == nil && depth == 0 && a.lastAck.Load() >= seq
	}, 20*time.Second, 50*time.Millisecond, "the agent must get every event acknowledged (last ack %d)", a.lastAck.Load())
}

// A store failure at event 3 leaves 3 and everything after it unacknowledged, closes
// the stream, and the agent resends 3 once it has reconnected.
func TestIntegration_TransientStoreFailureIsResentNotAcknowledged(t *testing.T) {
	h := newScriptedContainers()
	h.failOnce["ctr-3"] = true
	var mu sync.Mutex
	var early []uint64
	a := startSpooledAgent(t, h, containerEvents(5, nil), func(seq uint64) {
		if h.storedCount("ctr-3") == 0 {
			mu.Lock()
			early = append(early, seq)
			mu.Unlock()
		}
	})
	a.waitAcked(t, 5)

	mu.Lock()
	for _, seq := range early {
		assert.Less(t, seq, uint64(3), "no ack may cover the failed event before it is stored")
	}
	mu.Unlock()
	assert.GreaterOrEqual(t, a.streams.Load(), int32(2), "the failure closes the stream and the agent reconnects")
	assert.Empty(t, a.disconnectReasons(), "a stream closed for a resend must not page the agent as disconnected")
	assert.Equal(t, 1, h.storedCount("ctr-3"))

	arrived := h.arrivals()
	first := slices.Index(arrived, "ctr-3")
	require.GreaterOrEqual(t, first, 0)
	second := slices.Index(arrived[first+1:], "ctr-3")
	require.GreaterOrEqual(t, second, 0, "the agent resends the failed event: %v", arrived)
	assert.NotContains(t, arrived[:first+1+second], "ctr-4", "nothing past the failed event is handled before it is resent: %v", arrived)
}

// A refused event, by its handler or for its observation time, is acknowledged and
// never holds the spool or the stream.
func TestIntegration_RefusedEventsAreAcknowledgedAndSkipped(t *testing.T) {
	h := newScriptedContainers()
	h.invalid["ctr-2"] = true
	a := startSpooledAgent(t, h, containerEvents(5, map[int]time.Duration{4: 10 * time.Minute}), nil)
	a.waitAcked(t, 5)

	assert.Equal(t, int32(1), a.streams.Load(), "a refused event never closes the stream")
	assert.Equal(t, []string{"ctr-1", "ctr-2", "ctr-3", "ctr-5"}, h.arrivals(), "ctr-4 is refused before its handler, every event arrives once")
	for _, id := range []string{"ctr-1", "ctr-3", "ctr-5"} {
		assert.Equal(t, 1, h.storedCount(id), id)
	}
}

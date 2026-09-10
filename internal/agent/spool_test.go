package agent

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testEvent(observed time.Time) *agentpb.AgentEvent {
	return &agentpb.AgentEvent{
		AgentId:    uuid.NewString(),
		EventId:    uuid.NewString(),
		ObservedAt: timestamppb.New(observed),
		Body: &agentpb.AgentEvent_Resource{Resource: &agentpb.ResourceSample{
			ContainerId: "c1",
			CpuPercent:  12.5,
		}},
	}
}

// unthrottle removes the drain pacing so a test that pushes thousands of events
// is not gated by the production rate.
func unthrottle(s *Spool) *Spool {
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	return s
}

func testSpoolConfig() SpoolConfig {
	return SpoolConfig{MaxMemoryBytes: 1 << 20, MaxDiskBytes: 8 << 20, MaxAge: 24 * time.Hour}
}

type captureSink struct {
	mu       sync.Mutex
	sent     []*agentpb.AgentEvent
	statuses []*agentpb.SpoolStatus
	err      error
}

func (c *captureSink) Send(evt *agentpb.AgentEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.sent = append(c.sent, evt)
	return nil
}

func (c *captureSink) SendStatus(st *agentpb.SpoolStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statuses = append(c.statuses, st)
	return nil
}

func (c *captureSink) reportedStatuses() []*agentpb.SpoolStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*agentpb.SpoolStatus(nil), c.statuses...)
}

func (c *captureSink) events() []*agentpb.AgentEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*agentpb.AgentEvent(nil), c.sent...)
}

func (c *captureSink) failWith(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

func TestSpoolStoreOpensIncrementalVacuum(t *testing.T) {
	store, err := openSpoolStore(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	// The DSN has to win over the WAL header write, or a purge never returns
	// space to the filesystem.
	var autoVacuum int
	require.NoError(t, store.db.QueryRow("PRAGMA auto_vacuum").Scan(&autoVacuum))
	assert.Equal(t, 2, autoVacuum)
}

func TestSpoolStoreAppendReadsInOrder(t *testing.T) {
	store, err := openSpoolStore(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	now := time.Now()
	events := []pendingEvent{
		{EventID: "a", ObservedAt: now.Unix(), Payload: []byte("one")},
		{EventID: "b", ObservedAt: now.Unix(), Payload: []byte("two")},
		{EventID: "c", ObservedAt: now.Unix(), Payload: []byte("three")},
	}
	last, err := store.Append(events)
	require.NoError(t, err)
	assert.Equal(t, int64(3), last)

	read, err := store.ReadFrom(0, 10)
	require.NoError(t, err)
	require.Len(t, read, 3)
	assert.Equal(t, []byte("one"), read[0].Payload)
	assert.Equal(t, []byte("three"), read[2].Payload)
	assert.Less(t, read[0].Seq, read[1].Seq)

	partial, err := store.ReadFrom(read[0].Seq, 10)
	require.NoError(t, err)
	assert.Len(t, partial, 2)
}

func TestSpoolStoreDeleteUpTo(t *testing.T) {
	store, err := openSpoolStore(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	now := time.Now().Unix()
	_, err = store.Append([]pendingEvent{
		{EventID: "a", ObservedAt: now, Payload: []byte("one")},
		{EventID: "b", ObservedAt: now, Payload: []byte("two")},
		{EventID: "c", ObservedAt: now, Payload: []byte("three")},
	})
	require.NoError(t, err)

	n, err := store.DeleteUpTo(2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	depth, err := store.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(1), depth)

	// AUTOINCREMENT keeps the cursor moving forward after a purge; a reused
	// rowid would make the drain walk backwards.
	last, err := store.Append([]pendingEvent{{EventID: "d", ObservedAt: now, Payload: []byte("four")}})
	require.NoError(t, err)
	assert.Equal(t, int64(4), last)
}

func TestSpoolStoreDeleteOlderThan(t *testing.T) {
	store, err := openSpoolStore(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	now := time.Now()
	_, err = store.Append([]pendingEvent{
		{EventID: "old", ObservedAt: now.Add(-48 * time.Hour).Unix(), Payload: []byte("old")},
		{EventID: "new", ObservedAt: now.Unix(), Payload: []byte("new")},
	})
	require.NoError(t, err)

	n, err := store.DeleteOlderThan(now.Add(-24 * time.Hour).Unix())
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	read, err := store.ReadFrom(0, 10)
	require.NoError(t, err)
	require.Len(t, read, 1)
	assert.Equal(t, []byte("new"), read[0].Payload)
}

func TestSpoolPersistsQueuedEvents(t *testing.T) {
	dir := t.TempDir()
	spool := NewSpool(dir, testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	require.NoError(t, spool.Send(testEvent(time.Now())))
	require.NoError(t, spool.Send(testEvent(time.Now())))

	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(2), depth)

	require.NoError(t, spool.Close())
	assert.FileExists(t, filepath.Join(dir, spoolFile))

	reopened := NewSpool(dir, testSpoolConfig(), testLogger())
	defer func() { _ = reopened.Close() }()
	depth, err = reopened.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(2), depth)
}

func TestSpoolFallsBackToMemoryWhenStoreUnavailable(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))

	spool := NewSpool(blocked, testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	require.NoError(t, spool.Send(testEvent(time.Now())))
	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(1), depth)
}

func TestSpoolMemoryOnlyDropsOldestPastBudget(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))

	cfg := SpoolConfig{MaxMemoryBytes: 128, MaxDiskBytes: 0, MaxAge: time.Hour}
	spool := NewSpool(blocked, cfg, testLogger())
	defer func() { _ = spool.Close() }()

	for range 50 {
		require.NoError(t, spool.Send(testEvent(time.Now())))
	}

	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Less(t, depth, int64(50))

	dropped, err := spool.Dropped()
	require.NoError(t, err)
	assert.Positive(t, dropped)
}

func TestDisabledSpoolPassesThroughAndSurfacesFailure(t *testing.T) {
	spool := NewSpool(t.TempDir(), SpoolConfig{}, testLogger())
	defer func() { _ = spool.Close() }()

	assert.ErrorIs(t, spool.Send(testEvent(time.Now())), ErrNoStream)

	sink := &captureSink{}
	spool.Attach(sink)
	require.NoError(t, spool.Send(testEvent(time.Now())))
	assert.Len(t, sink.events(), 1)

	sink.failWith(assert.AnError)
	assert.ErrorIs(t, spool.Send(testEvent(time.Now())), assert.AnError)
}

func TestSpoolPurgeExpired(t *testing.T) {
	dir := t.TempDir()
	spool := NewSpool(dir, SpoolConfig{MaxMemoryBytes: 1 << 20, MaxDiskBytes: 8 << 20, MaxAge: time.Hour}, testLogger())
	defer func() { _ = spool.Close() }()

	require.NoError(t, spool.Send(testEvent(time.Now().Add(-2*time.Hour))))
	require.NoError(t, spool.Send(testEvent(time.Now())))
	require.NoError(t, spool.flush())

	require.NoError(t, spool.PurgeExpired(t.Context()))

	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(1), depth)
}

func TestEnabledSpoolAbsorbsSendFailure(t *testing.T) {
	spool := NewSpool(t.TempDir(), testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	sink := &captureSink{}
	sink.failWith(assert.AnError)
	spool.Attach(sink)

	// The collectors must never see the broken stream: their Send stays nil and
	// the events pile up instead of tearing the collector down.
	for range 5 {
		require.NoError(t, spool.Send(testEvent(time.Now())))
	}
	require.NoError(t, spool.flush())

	assert.Error(t, spool.drainOnce(t.Context()))

	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(5), depth)
}

func TestSpoolReplaysInOrderThenPurgesOnAck(t *testing.T) {
	spool := NewSpool(t.TempDir(), testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	base := time.Now().Add(-time.Minute)
	for i := range 5 {
		evt := testEvent(base.Add(time.Duration(i) * time.Second))
		evt.EventId = string(rune('a' + i))
		require.NoError(t, spool.Send(evt))
	}
	require.NoError(t, spool.flush())

	sink := &captureSink{}
	spool.Attach(sink)
	require.NoError(t, spool.drainOnce(t.Context()))

	sent := sink.events()
	require.Len(t, sent, 5)
	for i, evt := range sent {
		assert.True(t, evt.GetReplayed(), "replayed events must be marked")
		assert.Equal(t, string(rune('a'+i)), evt.GetEventId(), "order of production must be preserved")
		assert.Equal(t, uint64(i+1), evt.GetSeq(), "the wire sequence is the spool row the ack will name")
	}

	// Nothing goes until the server says it took it.
	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(5), depth)

	spool.Acked(3)
	depth, err = spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(2), depth)
}

func TestSpoolRewindsToLastAckOnReconnect(t *testing.T) {
	spool := NewSpool(t.TempDir(), testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	for range 4 {
		require.NoError(t, spool.Send(testEvent(time.Now())))
	}
	require.NoError(t, spool.flush())

	first := &captureSink{}
	spool.Attach(first)
	require.NoError(t, spool.drainOnce(t.Context()))
	require.Len(t, first.events(), 4)
	spool.Acked(2)
	spool.Detach()

	// The two events past the ack were never proven delivered, so they go again.
	second := &captureSink{}
	spool.Attach(second)
	require.NoError(t, spool.drainOnce(t.Context()))
	assert.Len(t, second.events(), 2)
}

func bulkEvent(pad string) *agentpb.AgentEvent {
	evt := testEvent(time.Now())
	evt.GetResource().ContainerId = pad
	return evt
}

func TestSpoolDiskBudgetCapsSizeAndDropsOldest(t *testing.T) {
	const budget = 256 << 10
	cfg := SpoolConfig{MaxMemoryBytes: 64 << 10, MaxDiskBytes: budget, MaxAge: time.Hour}
	spool := unthrottle(NewSpool(t.TempDir(), cfg, testLogger()))
	defer func() { _ = spool.Close() }()

	pad := strings.Repeat("x", 512)
	for range 3000 {
		require.NoError(t, spool.Send(bulkEvent(pad)))
	}
	require.NoError(t, spool.flush())

	size, err := spool.store.SizeBytes()
	require.NoError(t, err)
	assert.LessOrEqual(t, size, int64(budget), "the spool must not grow past its disk budget")

	dropped, err := spool.Dropped()
	require.NoError(t, err)
	assert.Positive(t, dropped, "events past the budget must be counted as dropped")

	// What survives is the recent tail: continuity close to the outage is worth
	// more than a truncated older history.
	rows, err := spool.store.ReadFrom(0, 5)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	assert.Greater(t, rows[0].Seq, int64(1), "the oldest rows are the ones that go")
}

func TestSpoolMemoryBufferStopsGrowing(t *testing.T) {
	cfg := SpoolConfig{MaxMemoryBytes: 32 << 10, MaxDiskBytes: 8 << 20, MaxAge: time.Hour}
	spool := unthrottle(NewSpool(t.TempDir(), cfg, testLogger()))
	defer func() { _ = spool.Close() }()

	pad := strings.Repeat("x", 512)
	for range 2000 {
		require.NoError(t, spool.Send(bulkEvent(pad)))
	}

	spool.mu.Lock()
	buffered := spool.bufBytes
	spool.mu.Unlock()
	assert.LessOrEqual(t, buffered, int64(cfg.MaxMemoryBytes),
		"past its budget the buffer must spill to the store instead of growing")

	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(2000), depth, "spilling to disk must not lose events")
}

func TestSpoolReturnsDiskAfterFullDrain(t *testing.T) {
	cfg := SpoolConfig{MaxMemoryBytes: 64 << 10, MaxDiskBytes: 8 << 20, MaxAge: time.Hour}
	spool := unthrottle(NewSpool(t.TempDir(), cfg, testLogger()))
	defer func() { _ = spool.Close() }()

	pad := strings.Repeat("x", 512)
	for range 2000 {
		require.NoError(t, spool.Send(bulkEvent(pad)))
	}
	require.NoError(t, spool.flush())

	full, err := spool.store.SizeBytes()
	require.NoError(t, err)

	sink := &captureSink{}
	spool.Attach(sink)
	require.NoError(t, spool.drainOnce(t.Context()))
	spool.Acked(uint64(len(sink.events())))

	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Zero(t, depth)

	// A spool that keeps the size of its worst outage forever is a bug of its own.
	drained, err := spool.store.SizeBytes()
	require.NoError(t, err)
	assert.Less(t, drained, full/2)
}

func inventoryTestEvent(names ...string) *agentpb.AgentEvent {
	entries := make([]*agentpb.ContainerEvent, 0, len(names))
	for _, n := range names {
		entries = append(entries, &agentpb.ContainerEvent{ContainerId: n, Name: n})
	}
	return &agentpb.AgentEvent{
		EventId:    uuid.NewString(),
		ObservedAt: timestamppb.Now(),
		Body:       &agentpb.AgentEvent_Inventory{Inventory: &agentpb.ContainerInventory{Containers: entries, Complete: true}},
	}
}

func hostSampleTestEvent() *agentpb.AgentEvent {
	evt := testEvent(time.Now())
	evt.GetResource().ContainerId = ""
	return evt
}

func TestSpoolNeverQueuesStateSnapshots(t *testing.T) {
	spool := NewSpool(t.TempDir(), testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	require.NoError(t, spool.Send(inventoryTestEvent("a")))
	require.NoError(t, spool.Send(hostSampleTestEvent()))
	require.NoError(t, spool.Send(&agentpb.AgentEvent{
		EventId: uuid.NewString(), ObservedAt: timestamppb.Now(),
		Body: &agentpb.AgentEvent_Certificate{Certificate: &agentpb.CertificateInfo{Host: "example.test", Port: 443}},
	}))
	require.NoError(t, spool.flush())

	// A stale inventory replayed after an outage would archive live containers.
	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Zero(t, depth, "state snapshots must never enter the queue")

	require.NoError(t, spool.Send(testEvent(time.Now())))
	require.NoError(t, spool.flush())
	depth, err = spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(1), depth, "container samples are still queued")
}

func TestSpoolResendsFreshSnapshotOnReconnect(t *testing.T) {
	spool := NewSpool(t.TempDir(), testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	// Produced while the server was unreachable: only the last one matters.
	require.NoError(t, spool.Send(inventoryTestEvent("old")))
	require.NoError(t, spool.Send(inventoryTestEvent("current")))

	sink := &captureSink{}
	spool.Attach(sink)
	spool.resendSnapshots()

	sent := sink.events()
	require.Len(t, sent, 1, "only the latest snapshot of a kind goes out")
	body := sent[0].GetInventory()
	require.NotNil(t, body)
	require.Len(t, body.GetContainers(), 1)
	assert.Equal(t, "current", body.GetContainers()[0].GetName())
}

func TestSpoolReplaysContainerLifecycleWithoutInventory(t *testing.T) {
	spool := NewSpool(t.TempDir(), testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	born := time.Now().Add(-30 * time.Minute)
	for _, state := range []agentpb.ContainerState{
		agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		agentpb.ContainerState_CONTAINER_STATE_EXITED,
	} {
		require.NoError(t, spool.Send(&agentpb.AgentEvent{
			EventId:    uuid.NewString(),
			ObservedAt: timestamppb.New(born),
			Body: &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{
				ContainerId: "ephemeral", Name: "ephemeral", State: state,
			}},
		}))
	}
	// The inventory taken after it died no longer mentions it.
	require.NoError(t, spool.Send(inventoryTestEvent("survivor")))
	require.NoError(t, spool.flush())

	sink := &captureSink{}
	spool.Attach(sink)
	spool.resendSnapshots()
	require.NoError(t, spool.drainOnce(t.Context()))

	var lifecycle int
	for _, evt := range sink.events() {
		if c := evt.GetContainer(); c != nil && c.GetContainerId() == "ephemeral" {
			lifecycle++
			assert.True(t, evt.GetReplayed())
			assert.Equal(t, born.Unix(), evt.GetObservedAt().AsTime().Unix(),
				"a replayed lifecycle event keeps its real time")
		}
	}
	assert.Equal(t, 2, lifecycle, "a container born and gone during the outage still has its timeline")
}

func TestSpoolAckNeverPurgesPastWhatWasSent(t *testing.T) {
	spool := NewSpool(t.TempDir(), testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	for range 3 {
		require.NoError(t, spool.Send(testEvent(time.Now())))
	}
	require.NoError(t, spool.flush())

	sink := &captureSink{}
	spool.Attach(sink)

	// Snapshots consume stream sequence numbers of their own, so the server can
	// acknowledge a number the drain has not reached.
	spool.Acked(3)

	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(3), depth, "an ack must never drop what was never sent")
}

func TestSpoolKeepsOrderWhileProducingDuringDrain(t *testing.T) {
	spool := unthrottle(NewSpool(t.TempDir(), testSpoolConfig(), testLogger()))
	defer func() { _ = spool.Close() }()

	for i := range 200 {
		evt := testEvent(time.Now())
		evt.EventId = fmt.Sprintf("backlog-%03d", i)
		require.NoError(t, spool.Send(evt))
	}
	require.NoError(t, spool.flush())

	sink := &captureSink{}
	spool.Attach(sink)

	// New events produced mid-catch-up go to the tail, never ahead of the
	// backlog: a replayed start must not land after a live stop.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 50 {
			evt := testEvent(time.Now())
			evt.EventId = fmt.Sprintf("live-%03d", i)
			require.NoError(t, spool.Send(evt))
		}
		require.NoError(t, spool.flush())
	}()
	<-done
	require.NoError(t, spool.drainOnce(t.Context()))

	sent := sink.events()
	require.Len(t, sent, 250)
	var lastSeq uint64
	for _, evt := range sent {
		assert.Greater(t, evt.GetSeq(), lastSeq, "the wire sequence must never go backwards")
		lastSeq = evt.GetSeq()
	}
	assert.Equal(t, "backlog-000", sent[0].GetEventId())
	assert.Equal(t, "live-049", sent[249].GetEventId())
}

func TestSpoolBacksOffAndResendsOnRateLimit(t *testing.T) {
	spool := unthrottle(NewSpool(t.TempDir(), testSpoolConfig(), testLogger()))
	defer func() { _ = spool.Close() }()

	for range 4 {
		require.NoError(t, spool.Send(testEvent(time.Now())))
	}
	require.NoError(t, spool.flush())

	sink := &captureSink{}
	spool.Attach(sink)
	require.NoError(t, spool.drainOnce(t.Context()))
	require.Len(t, sink.events(), 4)
	spool.Acked(2)

	// The server drops a refused event instead of failing the stream, so
	// everything past the last ack has to go again.
	spool.RateLimited(10 * time.Millisecond)
	require.NoError(t, spool.drainOnce(t.Context()))
	assert.Len(t, sink.events(), 4, "the drain must hold while the server asks it to")

	require.Eventually(t, func() bool {
		require.NoError(t, spool.drainOnce(t.Context()))
		return len(sink.events()) == 6
	}, time.Second, 5*time.Millisecond, "the two unacknowledged events must be resent after the pause")
}

func copySpoolFiles(t *testing.T, from, to string) {
	t.Helper()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(filepath.Join(from, spoolFile+suffix))
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(to, spoolFile+suffix), data, 0o600))
	}
}

func TestSpoolSurvivesAbruptStop(t *testing.T) {
	dir := t.TempDir()
	spool := NewSpool(dir, testSpoolConfig(), testLogger())

	for range 10 {
		require.NoError(t, spool.Send(testEvent(time.Now())))
	}
	require.NoError(t, spool.flush())

	// Copying the live files, WAL included, is what an agent killed mid-write
	// leaves behind: no clean close, no checkpoint.
	crashed := t.TempDir()
	copySpoolFiles(t, dir, crashed)

	recovered := NewSpool(crashed, testSpoolConfig(), testLogger())
	defer func() { _ = recovered.Close() }()

	depth, err := recovered.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(10), depth, "committed events must survive a kill")

	rows, err := recovered.store.ReadFrom(0, 20)
	require.NoError(t, err)
	assert.Len(t, rows, 10)
}

func TestSpoolCleanCloseLosesNothing(t *testing.T) {
	dir := t.TempDir()
	spool := NewSpool(dir, testSpoolConfig(), testLogger())

	for range 7 {
		require.NoError(t, spool.Send(testEvent(time.Now())))
	}
	// Not flushed: a clean stop has to push the buffer out on its way down.
	require.NoError(t, spool.Close())

	reopened := NewSpool(dir, testSpoolConfig(), testLogger())
	defer func() { _ = reopened.Close() }()

	depth, err := reopened.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(7), depth)
}

func TestSpoolDropsExpiredOnRestartAndReplaysTheRest(t *testing.T) {
	dir := t.TempDir()
	cfg := SpoolConfig{MaxMemoryBytes: 1 << 20, MaxDiskBytes: 8 << 20, MaxAge: time.Hour}
	spool := NewSpool(dir, cfg, testLogger())

	require.NoError(t, spool.Send(testEvent(time.Now().Add(-3*time.Hour))))
	fresh := testEvent(time.Now())
	fresh.EventId = "fresh"
	require.NoError(t, spool.Send(fresh))
	require.NoError(t, spool.Close())

	restarted := NewSpool(dir, cfg, testLogger())
	defer func() { _ = restarted.Close() }()
	require.NoError(t, restarted.PurgeExpired(t.Context()))

	sink := &captureSink{}
	restarted.Attach(sink)
	require.NoError(t, restarted.drainOnce(t.Context()))

	sent := sink.events()
	require.Len(t, sent, 1, "what is past the retention window must not be replayed")
	assert.Equal(t, "fresh", sent[0].GetEventId())
}

func TestSpoolDiscardRemovesEveryFile(t *testing.T) {
	dir := t.TempDir()
	spool := NewSpool(dir, testSpoolConfig(), testLogger())
	defer func() { _ = spool.Close() }()

	require.NoError(t, spool.Send(testEvent(time.Now())))
	require.NoError(t, spool.flush())
	require.FileExists(t, filepath.Join(dir, spoolFile))

	// A revoked agent has no business keeping the fleet's telemetry on disk.
	require.NoError(t, spool.Discard())
	for _, suffix := range []string{"", "-wal", "-shm"} {
		assert.NoFileExists(t, filepath.Join(dir, spoolFile+suffix))
	}
}

func TestSpoolReportsCatchUpThenGoesQuiet(t *testing.T) {
	spool := unthrottle(NewSpool(t.TempDir(), testSpoolConfig(), testLogger()))
	defer func() { _ = spool.Close() }()

	for range 3 {
		require.NoError(t, spool.Send(testEvent(time.Now())))
	}
	require.NoError(t, spool.flush())

	sink := &captureSink{}
	spool.Attach(sink)

	spool.reportStatus(true)
	first := sink.reportedStatuses()
	require.Len(t, first, 1)
	assert.True(t, first[0].GetDraining())
	assert.Equal(t, uint64(3), first[0].GetQueued())

	require.NoError(t, spool.drainOnce(t.Context()))
	spool.Acked(3)
	spool.reportStatus(false)

	reported := sink.reportedStatuses()
	require.Len(t, reported, 2, "catching up is worth one more word, then silence")
	assert.False(t, reported[1].GetDraining())
	assert.Zero(t, reported[1].GetQueued())

	// An agent in step with the server must not chatter.
	spool.reportStatus(false)
	assert.Len(t, sink.reportedStatuses(), 2)
}

package agent

import (
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

func testSpoolConfig() SpoolConfig {
	return SpoolConfig{MaxMemoryBytes: 1 << 20, MaxDiskBytes: 8 << 20, MaxAge: 24 * time.Hour}
}

type captureSink struct {
	mu   sync.Mutex
	sent []*agentpb.AgentEvent
	err  error
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
	spool := NewSpool(t.TempDir(), cfg, testLogger())
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
	spool := NewSpool(t.TempDir(), cfg, testLogger())
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
	spool := NewSpool(t.TempDir(), cfg, testLogger())
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

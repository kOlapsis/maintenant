package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func memoryOnlySpool(t *testing.T, cfg SpoolConfig) *Spool {
	t.Helper()
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))
	spool := unthrottle(NewSpool(blocked, cfg, testLogger()))
	require.Nil(t, spool.store)
	t.Cleanup(func() { _ = spool.Close() })
	return spool
}

func TestSpoolMemoryOnlyDeliversBufferedEventsToTheSink(t *testing.T) {
	spool := memoryOnlySpool(t, testSpoolConfig())

	const n = 20
	for i := range n {
		evt := testEvent(time.Now())
		evt.EventId = fmt.Sprintf("e%02d", i)
		require.NoError(t, spool.Send(evt))
	}

	sink := &captureSink{}
	spool.Attach(sink)
	require.NoError(t, spool.drainOnce(t.Context()))

	sent := sink.events()
	require.Len(t, sent, n)
	for i, evt := range sent {
		assert.Equal(t, fmt.Sprintf("e%02d", i), evt.GetEventId())
	}

	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Zero(t, depth)

	require.NoError(t, spool.drainOnce(t.Context()))
	assert.Len(t, sink.events(), n, "an event is sent at most once without a store to rewind from")
}

func TestSpoolMemoryOnlyKeepsEventsTheSinkRefused(t *testing.T) {
	spool := memoryOnlySpool(t, testSpoolConfig())

	const n = 5
	for i := range n {
		evt := testEvent(time.Now())
		evt.EventId = fmt.Sprintf("e%02d", i)
		require.NoError(t, spool.Send(evt))
	}

	sink := &captureSink{}
	sink.failWith(errors.New("stream broken"))
	spool.Attach(sink)
	require.Error(t, spool.drainOnce(t.Context()))

	depth, err := spool.Depth()
	require.NoError(t, err)
	assert.Equal(t, int64(n), depth)

	sink.failWith(nil)
	require.NoError(t, spool.drainOnce(t.Context()))
	sent := sink.events()
	require.Len(t, sent, n)
	for i, evt := range sent {
		assert.Equal(t, fmt.Sprintf("e%02d", i), evt.GetEventId())
	}
}

func TestSpoolBufferStaysUnderBudgetWhenAppendFails(t *testing.T) {
	cfg := SpoolConfig{MaxMemoryBytes: 32 << 10, MaxDiskBytes: 8 << 20, MaxAge: time.Hour}
	spool := unthrottle(NewSpool(t.TempDir(), cfg, testLogger()))
	require.NotNil(t, spool.store)
	t.Cleanup(func() { _ = spool.Close() })

	require.NoError(t, spool.store.Close())

	pad := strings.Repeat("x", 512)
	for range 2000 {
		_ = spool.Send(bulkEvent(pad))
	}

	spool.mu.Lock()
	buffered := spool.bufBytes
	spool.mu.Unlock()
	assert.LessOrEqual(t, buffered, cfg.MaxMemoryBytes)
}

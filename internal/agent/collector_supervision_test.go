package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/retry"
)

func TestSuperviseCollectorRestartsAfterAnError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var runs atomic.Int32
	run := func(ctx context.Context) error {
		if runs.Add(1) >= 3 {
			cancel()
			<-ctx.Done()
			return nil
		}
		return errors.New("certificate scan not sent")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		superviseCollector(ctx, run, retry.New(time.Millisecond, 5*time.Millisecond, 0), testLogger())
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not return after the context was cancelled")
	}
	assert.Equal(t, int32(3), runs.Load())
}

func TestSuperviseCollectorStopsRestartingOnceCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var runs atomic.Int32
	run := func(ctx context.Context) error {
		runs.Add(1)
		cancel()
		return errors.New("stopped with the context")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		superviseCollector(ctx, run, retry.New(time.Millisecond, 5*time.Millisecond, 0), testLogger())
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.Fail(t, "supervisor kept waiting after cancellation")
	}
	assert.Equal(t, int32(1), runs.Load())
}

package docker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryConnect_Timeout(t *testing.T) {
	c, err := NewClient("unix:///nonexistent-socket-abc123.sock", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	start := time.Now()
	err = c.TryConnect(context.Background())
	elapsed := time.Since(start)

	assert.Error(t, err, "TryConnect on unreachable socket must return an error")
	assert.Less(t, elapsed, 5*time.Second, "TryConnect must return in ≤ 5s (bounded ~3s)")
	assert.False(t, c.IsConnected(), "IsConnected must be false after failed TryConnect")
}

func TestTryConnect_ContextCancelled(t *testing.T) {
	c, err := NewClient("unix:///nonexistent-socket-abc123.sock", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	start := time.Now()
	err = c.TryConnect(ctx)
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Less(t, elapsed, 2*time.Second, "TryConnect with cancelled context must return immediately")
}

func TestConnectWithRetry_CancelledContext(t *testing.T) {
	c, err := NewClient("unix:///nonexistent-socket-abc123.sock", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = c.ConnectWithRetry(ctx)
	assert.Error(t, err, "ConnectWithRetry must return when ctx is cancelled")
}

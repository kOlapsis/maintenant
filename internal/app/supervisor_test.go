// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/retry"
	"github.com/kolapsis/maintenant/internal/runtime"
)

// flakyRuntime fails its first connections the way an unusable kubeconfig does: at once, with a configuration error.
type flakyRuntime struct {
	runtime.Runtime
	mu        sync.Mutex
	failures  int
	connected bool
	streamed  chan struct{}
	once      sync.Once
}

func (f *flakyRuntime) Name() string { return "kubernetes" }

func (f *flakyRuntime) Connect(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures > 0 {
		f.failures--
		return errors.New("kubernetes config: invalid configuration: no server found for cluster")
	}
	f.connected = true
	return nil
}

func (f *flakyRuntime) IsConnected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func (f *flakyRuntime) SetDisconnected() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected = false
}

func (f *flakyRuntime) DiscoverAll(context.Context) ([]*container.Container, error) { return nil, nil }

func (f *flakyRuntime) StreamEvents(ctx context.Context) <-chan runtime.RuntimeEvent {
	f.once.Do(func() { close(f.streamed) })
	ch := make(chan runtime.RuntimeEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch
}

func TestSupervisorLoop_RetriesAConnectionError(t *testing.T) {
	a, logs := newTestApp(t, nil)
	rt := &flakyRuntime{failures: 2, streamed: make(chan struct{})}
	a.rt = rt

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.supervisorLoop(ctx, nil, retry.New(time.Millisecond, time.Millisecond, 0))
		close(done)
	}()

	select {
	case <-rt.streamed:
	case <-time.After(5 * time.Second):
		t.Fatal("the supervisor stopped at the first connection error instead of retrying")
	}
	assert.Equal(t, 2, strings.Count(logs.String(), "container runtime connection failed, retrying"))
	assert.Contains(t, logs.String(), "no server found for cluster", "the cause is logged")

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the supervisor did not stop with its context")
	}
}

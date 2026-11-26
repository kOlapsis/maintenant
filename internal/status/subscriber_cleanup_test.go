// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type cleaningSubscriberStore struct {
	recordingSubscriberStore
	cleaned chan struct{}
}

func (s *cleaningSubscriberStore) CleanExpiredUnconfirmed(context.Context) (int64, error) {
	s.cleaned <- struct{}{}
	return 0, nil
}

func TestSubscriberServiceStart_CleansAtStart(t *testing.T) {
	store := &cleaningSubscriberStore{cleaned: make(chan struct{}, 1)}
	svc := NewSubscriberService(store, nil, "http://localhost", slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		svc.Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	select {
	case <-store.cleaned:
	case <-time.After(5 * time.Second):
		t.Fatal("no cleanup pass at start")
	}
}

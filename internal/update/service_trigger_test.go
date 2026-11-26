// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type blockingLister struct{ entered chan struct{} }

func (l blockingLister) ListContainerInfos(ctx context.Context) ([]ContainerInfo, error) {
	l.entered <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestTriggerScan_RunsOnlyOnTheContextOfStart(t *testing.T) {
	t.Run("concurrent with Start", func(t *testing.T) {
		store := &stubStore{}
		svc := NewService(Deps{Store: store, Scanner: newTestScanner(&stubRegistry{}, store), Containers: stubLister{}, Logger: testLogger()})
		ctx, cancel := context.WithCancel(context.Background())
		stopped := make(chan struct{})
		go func() {
			svc.Start(ctx)
			close(stopped)
		}()
		_, _ = svc.TriggerScan(context.Background())
		cancel()
		<-stopped
		svc.Wait()
	})

	t.Run("cancelled with the service", func(t *testing.T) {
		store := &stubStore{}
		lister := blockingLister{entered: make(chan struct{}, 1)}
		svc := NewService(Deps{Store: store, Scanner: newTestScanner(&stubRegistry{}, store), Containers: lister, Logger: testLogger()})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stopped := make(chan struct{})
		go func() {
			svc.Start(ctx)
			close(stopped)
		}()
		require.Eventually(t, func() bool {
			svc.mu.RLock()
			defer svc.mu.RUnlock()
			return svc.appCtx != nil
		}, 5*time.Second, time.Millisecond)
		_, err := svc.TriggerScan(context.Background())
		require.NoError(t, err)
		select {
		case <-lister.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the triggered scan never started")
		}

		cancel()
		<-stopped
		waited := make(chan struct{})
		go func() {
			svc.Wait()
			close(waited)
		}()
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
			t.Fatal("the triggered scan outlived the context of Start")
		}
	})
}

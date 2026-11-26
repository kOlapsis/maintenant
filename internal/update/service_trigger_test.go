// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTriggerScan_RunsOnlyOnTheContextOfStart(t *testing.T) {
	store := &stubStore{}
	svc := NewService(Deps{Store: store, Scanner: newTestScanner(&stubRegistry{}, store), Containers: stubLister{}, Logger: testLogger()})

	_, err := svc.TriggerScan(context.Background())
	require.Error(t, err, "before Start no context could ever cancel the scan")

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		svc.Start(ctx)
		close(stopped)
	}()
	require.Eventually(t, func() bool {
		_, err := svc.TriggerScan(context.Background())
		return err == nil
	}, 5*time.Second, time.Millisecond)

	cancel()
	<-stopped
	svc.Wait()
}

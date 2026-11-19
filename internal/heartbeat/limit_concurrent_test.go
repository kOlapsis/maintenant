// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package heartbeat_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type capAt int

func (c capAt) CanCreateHeartbeat(count int) bool { return count < int(c) }
func (capAt) CanStorePayload() bool               { return false }

func TestCreateHeartbeat_ConcurrentCallsFillTheCapOnce(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := heartbeat.NewService(heartbeat.Deps{
		Store:          store.NewHeartbeatStore(storetest.Open(t, logger)),
		Logger:         logger,
		LicenseChecker: capAt(1),
	})

	var created, refused atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.CreateHeartbeat(context.Background(),
				heartbeat.CreateHeartbeatInput{Name: fmt.Sprintf("job-%d", i), IntervalSeconds: 3600, GraceSeconds: 60},
				fmt.Sprintf("00000000-0000-4000-8000-%012d", i))
			switch err {
			case nil:
				created.Add(1)
			case heartbeat.ErrLimitReached:
				refused.Add(1)
			default:
				t.Errorf("call %d: %v", i, err)
			}
		}()
	}
	wg.Wait()

	assert.EqualValues(t, 1, created.Load())
	assert.EqualValues(t, 11, refused.Load())
}

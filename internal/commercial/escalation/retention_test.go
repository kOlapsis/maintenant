// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package escalation

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextRetentionTick_AfterMidnightBefore3AM(t *testing.T) {
	now := time.Date(2026, 1, 15, 1, 0, 0, 0, time.Local)
	d := nextRetentionTick(now)
	assert.Equal(t, 2*time.Hour, d)
}

func TestNextRetentionTick_After3AM(t *testing.T) {
	now := time.Date(2026, 1, 15, 4, 0, 0, 0, time.Local)
	d := nextRetentionTick(now)
	assert.Equal(t, 23*time.Hour, d)
}

func TestNextRetentionTick_ExactlyAt3AM(t *testing.T) {
	now := time.Date(2026, 1, 15, 3, 0, 0, 0, time.Local)
	d := nextRetentionTick(now)
	assert.Equal(t, 24*time.Hour, d)
}

func TestRunRetentionLoop_PurgesOldRuns(t *testing.T) {
	store := &retentionMockStore{}
	svc := NewService(
		store,
		&mockChannelStore{},
		func() extension.Edition { return extension.Pro },
		&mockSuppressor{},
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	)

	var clockMu sync.Mutex
	callCount := 0
	svc.clockFn = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		callCount++
		if callCount <= 1 {
			return time.Date(2026, 1, 15, 2, 59, 59, 0, time.Local)
		}
		return time.Date(2026, 1, 15, 3, 0, 1, 0, time.Local)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go svc.RunRetentionLoop(ctx)

	require.Eventually(t, func() bool {
		return store.called()
	}, 2*time.Second, 50*time.Millisecond)

	before, _ := store.result()
	assert.True(t, before.Before(time.Now()))
}

// retentionMockStore extends mockStore with purge tracking.
type retentionMockStore struct {
	mockStore
	mu           sync.Mutex
	purgeCalled  bool
	purgedBefore time.Time
}

func (m *retentionMockStore) PurgeRunsAndDeliveriesOlderThan(_ context.Context, before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeCalled = true
	m.purgedBefore = before
	return nil
}

func (m *retentionMockStore) called() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.purgeCalled
}

func (m *retentionMockStore) result() (time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.purgedBefore, m.purgeCalled
}

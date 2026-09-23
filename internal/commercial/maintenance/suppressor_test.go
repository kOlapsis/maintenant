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

package maintenance

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore is a minimal mock for the Store interface.
type mockStore struct {
	mu       sync.Mutex
	calls    int
	matched  bool
	windowID string
	endsAt   time.Time
	err      error
	lastCtx  context.Context
}

func (m *mockStore) IsEntitySuppressed(ctx context.Context, monitorType string, monitorID string, now time.Time) (bool, string, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.lastCtx = ctx
	return m.matched, m.windowID, m.endsAt, m.err
}

// testLogHandler captures slog records for assertion.
type testLogHandler struct {
	mu      sync.Mutex
	records []logRecord
}

type logRecord struct {
	level   slog.Level
	message string
	attrs   map[string]string
}

func (h *testLogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *testLogHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]string)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, logRecord{level: r.Level, message: r.Message, attrs: attrs})
	return nil
}
func (h *testLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *testLogHandler) WithGroup(name string) slog.Handler       { return h }
func (h *testLogHandler) count(level slog.Level) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.level == level {
			n++
		}
	}
	return n
}
func (h *testLogHandler) hasAttr(key, value string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if v, ok := r.attrs[key]; ok && v == value {
			return true
		}
	}
	return false
}

func newTestSuppressor(store Store, handler slog.Handler) *Suppressor {
	if handler == nil {
		handler = slog.NewTextHandler(io.Discard, nil)
	}
	return &Suppressor{
		store:  store,
		logger: slog.New(handler),
		clock:  func() time.Time { return time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC) },
	}
}

func TestSuppressor_EmptyEntityID(t *testing.T) {
	store := &mockStore{}
	s := newTestSuppressor(store, nil)
	matched, err := s.IsSuppressed(context.Background(), "src", "container", "")
	require.NoError(t, err)
	assert.False(t, matched)
	assert.Equal(t, 0, store.calls, "store should not be called with empty entity id")
}

func TestSuppressor_UUIDEntityID(t *testing.T) {
	store := &mockStore{matched: false}
	s := newTestSuppressor(store, nil)
	matched, err := s.IsSuppressed(context.Background(), "src", "container", "550e8400-e29b-41d4-a716-446655440000")
	require.NoError(t, err)
	assert.False(t, matched)
	assert.Equal(t, 1, store.calls, "store should be queried with the UUID entity id")
}

func TestSuppressor_StoreError(t *testing.T) {
	lh := &testLogHandler{}
	store := &mockStore{err: errors.New("db failure")}
	s := newTestSuppressor(store, lh)
	matched, err := s.IsSuppressed(context.Background(), "src", "container", "42")
	require.NoError(t, err)
	assert.False(t, matched)
	assert.Equal(t, 1, store.calls)
	assert.Equal(t, 1, lh.count(slog.LevelError), "should log error on store failure")
}

func TestSuppressor_Match(t *testing.T) {
	lh := &testLogHandler{}
	endsAt := time.Date(2026, 1, 15, 14, 0, 0, 0, time.UTC)
	store := &mockStore{matched: true, windowID: "7", endsAt: endsAt}
	s := newTestSuppressor(store, lh)
	matched, err := s.IsSuppressed(context.Background(), "src", "container", "42")
	require.NoError(t, err)
	assert.True(t, matched)
	assert.Equal(t, 1, lh.count(slog.LevelDebug), "should log debug on match")
	assert.True(t, lh.hasAttr("maintenance_id", "7"), "debug log should include maintenance_id")
}

func TestSuppressor_NoMatch(t *testing.T) {
	lh := &testLogHandler{}
	store := &mockStore{matched: false}
	s := newTestSuppressor(store, lh)
	matched, err := s.IsSuppressed(context.Background(), "src", "container", "42")
	require.NoError(t, err)
	assert.False(t, matched)
	assert.Equal(t, 0, lh.count(slog.LevelDebug), "no debug log when no match")
}

func TestSuppressor_EmptyEntityType(t *testing.T) {
	store := &mockStore{}
	s := newTestSuppressor(store, nil)
	matched, err := s.IsSuppressed(context.Background(), "src", "", "42")
	require.NoError(t, err)
	assert.False(t, matched)
	assert.Equal(t, 0, store.calls, "store should not be called with empty entity type")
}

func TestSuppressor_ContextPropagated(t *testing.T) {
	store := &mockStore{}
	s := newTestSuppressor(store, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	// Should still call store (cancelled ctx propagated, store decides)
	s.IsSuppressed(ctx, "src", "container", "42") //nolint:errcheck
	assert.Equal(t, 1, store.calls)
	assert.Equal(t, ctx, store.lastCtx)
}

func TestSuppressor_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	store := &mockStore{matched: false}
	s := newTestSuppressor(store, nil)
	var wg sync.WaitGroup
	const goroutines = 10
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_, _ = s.IsSuppressed(context.Background(), "src", "container", "42")
		}()
	}
	wg.Wait()
	assert.Equal(t, goroutines, store.calls)
}

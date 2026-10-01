// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deliveryRecord struct {
	id        string
	delivered bool
}

type stubSubStore struct {
	subs     []*WebhookSubscription
	mu       sync.Mutex
	recorded []deliveryRecord
}

func (s *stubSubStore) records() []deliveryRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]deliveryRecord(nil), s.recorded...)
}

func (s *stubSubStore) List(context.Context) ([]*WebhookSubscription, error) { return s.subs, nil }
func (s *stubSubStore) GetByID(context.Context, string) (*WebhookSubscription, error) {
	return nil, nil
}
func (s *stubSubStore) Create(context.Context, *WebhookSubscription) error { return nil }
func (s *stubSubStore) Delete(context.Context, string) error               { return nil }
func (s *stubSubStore) RecordDelivery(_ context.Context, id string, delivered bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded = append(s.recorded, deliveryRecord{id: id, delivered: delivered})
	return nil
}
func (s *stubSubStore) ListActive(context.Context) ([]*WebhookSubscription, error) {
	return s.subs, nil
}

type capturedRequest struct {
	body   []byte
	sigHdr string
	evtHdr string
}

// TestDispatcher_DeliversRealEventPayload verifies the fix for issue #35: the
// webhook body carries the actual event data (not an empty synthetic alert),
// and the HMAC signature matches the delivered body.
func TestDispatcher_DeliversRealEventPayload(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var mu sync.Mutex
	var got *capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = &capturedRequest{
			body:   b,
			sigHdr: r.Header.Get("X-maintenant-Signature"),
			evtHdr: r.Header.Get("X-maintenant-Event"),
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	const secret = "s3cr3t"
	store := &stubSubStore{subs: []*WebhookSubscription{{
		ID: "w1", Name: "hook", URL: srv.URL, Secret: secret,
		EventTypes: []string{"*"}, IsActive: true,
	}}}

	notifier := alert.NewNotifier(nil, logger, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notifier.Start(ctx)

	d := NewDispatcher(store, notifier, logger)

	data := map[string]interface{}{
		"id":             "abc123",
		"state":          "running",
		"previous_state": "exited",
		"health_status":  "healthy",
		"agent_id":       "agent-1",
	}
	d.HandleEvent(ctx, event.ContainerStateChanged, data)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return got != nil
	}, 2*time.Second, 10*time.Millisecond, "webhook was never delivered")

	mu.Lock()
	defer mu.Unlock()

	// Header carries the real event type.
	assert.Equal(t, event.ContainerStateChanged, got.evtHdr)

	// Body is the real event payload, not an empty synthetic alert.
	var payload WebhookEvent
	require.NoError(t, json.Unmarshal(got.body, &payload))
	assert.Equal(t, event.ContainerStateChanged, payload.Type)
	assert.NotEmpty(t, payload.Timestamp)

	body, ok := payload.Data.(map[string]interface{})
	require.True(t, ok, "data must be an object, got %T", payload.Data)
	assert.Equal(t, "abc123", body["id"])
	assert.Equal(t, "running", body["state"])
	assert.Equal(t, "exited", body["previous_state"])
	assert.Equal(t, "healthy", body["health_status"])

	// HMAC signature must match the delivered body exactly (issue #35 bug B).
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(got.body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	assert.Equal(t, want, got.sigHdr, "signature must be computed over the delivered body")
}

func TestDispatcher_RecordsTheDeliveryOutcome(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	store := &stubSubStore{subs: []*WebhookSubscription{{ID: "w1", URL: srv.URL, EventTypes: []string{"*"}, IsActive: true}}}
	notifier := alert.NewNotifier(nil, logger, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notifier.Start(ctx)

	NewDispatcher(store, notifier, logger).HandleEvent(ctx, event.AlertFired, map[string]any{"id": "a1"})

	require.Eventually(t, func() bool { return len(store.records()) == 1 }, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, deliveryRecord{id: "w1", delivered: true}, store.records()[0])
}

func TestDispatcher_TestSendsWhatARealDeliverySends(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const secret = "s3cr3t"

	var mu sync.Mutex
	var requests []*http.Request
	var bodies [][]byte
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, r)
		bodies = append(bodies, b)
		code := status
		mu.Unlock()
		w.WriteHeader(code)
	}))
	defer srv.Close()

	sub := &WebhookSubscription{ID: "w1", Name: "hook", URL: srv.URL, Secret: secret, EventTypes: []string{"*"}, IsActive: true}
	store := &stubSubStore{subs: []*WebhookSubscription{sub}}
	notifier := alert.NewNotifier(nil, logger, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notifier.Start(ctx)
	d := NewDispatcher(store, notifier, logger)

	d.HandleEvent(ctx, event.AlertFired, map[string]any{"id": "a1"})
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(requests) == 1 },
		2*time.Second, 10*time.Millisecond)

	code, err := d.Test(ctx, sub)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	mu.Lock()
	require.Len(t, requests, 2)
	live, test := requests[0], requests[1]
	liveKeys, testKeys := slices.Sorted(maps.Keys(live.Header)), slices.Sorted(maps.Keys(test.Header))
	assert.Equal(t, liveKeys, testKeys, "the test carries exactly the headers of a real delivery")
	assert.Equal(t, "test", test.Header.Get("X-maintenant-Event"))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(bodies[1])
	assert.Equal(t, "sha256="+hex.EncodeToString(mac.Sum(nil)), test.Header.Get("X-maintenant-Signature"))
	status = http.StatusInternalServerError
	mu.Unlock()

	code, err = d.Test(ctx, sub)
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)

	records := store.records()
	require.Len(t, records, 3)
	assert.Equal(t, []deliveryRecord{{"w1", true}, {"w1", true}, {"w1", false}}, records)
}

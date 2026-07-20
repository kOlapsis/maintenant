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

package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSubStore struct {
	subs []*WebhookSubscription
}

func (s *stubSubStore) List(context.Context) ([]*WebhookSubscription, error) { return s.subs, nil }
func (s *stubSubStore) GetByID(context.Context, string) (*WebhookSubscription, error) {
	return nil, nil
}
func (s *stubSubStore) Create(context.Context, *WebhookSubscription) error              { return nil }
func (s *stubSubStore) Delete(context.Context, string) error                            { return nil }
func (s *stubSubStore) UpdateDeliveryStatus(context.Context, string, string, int) error { return nil }
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

	notifier := alert.NewNotifier(nil, logger)
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

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

package status

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/ratelimit"
)

type recordingSubscriberStore struct {
	SubscriberStore
	created []string
}

func (s *recordingSubscriberStore) CreateSubscriber(_ context.Context, sub *StatusSubscriber) (string, error) {
	s.created = append(s.created, sub.Email)
	return "id", nil
}

func newSubscribeHandler(t *testing.T) (*Handler, *recordingSubscriberStore) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &recordingSubscriberStore{}
	return &Handler{
		service:     &Service{subscribers: NewSubscriberService(store, nil, "http://localhost", logger)},
		logger:      logger,
		subscribeRL: ratelimit.New(5.0/3600.0, 5, ratelimit.NewClientIPResolver(nil)),
	}, store
}

func postSubscribe(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/status/subscribe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.5:44000"
	rec := httptest.NewRecorder()
	h.HandleSubscribe(rec, req)
	return rec
}

func TestHandleSubscribeRefusesOversizedBody(t *testing.T) {
	h, store := newSubscribeHandler(t)

	body := `{"email":"` + strings.Repeat("a", 1<<20) + `@example.com"}`
	rec := postSubscribe(t, h, body)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
	if len(store.created) != 0 {
		t.Fatalf("the store was written to: %v", store.created)
	}
}

func TestHandleSubscribeRefusesOverlongAddress(t *testing.T) {
	h, store := newSubscribeHandler(t)

	rec := postSubscribe(t, h, `{"email":"`+strings.Repeat("a", 300)+`@example.com"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	if len(store.created) != 0 {
		t.Fatalf("the store was written to: %v", store.created)
	}
}

func TestHandleSubscribeRefusesMalformedAddress(t *testing.T) {
	h, store := newSubscribeHandler(t)

	for _, email := range []string{"", "not-an-address", "a@b@c", "<script>alert(1)</script>"} {
		rec := postSubscribe(t, h, `{"email":`+quoteJSON(email)+`}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%q: got %d, want 400", email, rec.Code)
		}
	}
	if len(store.created) != 0 {
		t.Fatalf("the store was written to: %v", store.created)
	}
}

func TestHandleSubscribeQuotaIsFivePerWindow(t *testing.T) {
	h, store := newSubscribeHandler(t)

	for i := 0; i < 5; i++ {
		if rec := postSubscribe(t, h, `{"email":"ok@example.com"}`); rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, rec.Code)
		}
	}
	if rec := postSubscribe(t, h, `{"email":"ok@example.com"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth request: got %d, want 429", rec.Code)
	}
	if len(store.created) != 5 {
		t.Fatalf("stored %d subscriptions, want 5", len(store.created))
	}
}

// A caller renaming itself on every request must not enlarge the quota map:
// without a trusted proxy the header is never read, so one bucket is created.
func TestHandleSubscribeSpoofedHeadersDoNotGrowTheLimiter(t *testing.T) {
	h, _ := newSubscribeHandler(t)

	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodPost, "/status/subscribe",
			strings.NewReader(`{"email":"ok@example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "198.51.100."+string(rune('0'+i%10)))
		req.Header.Set("X-Real-IP", "203.0.113."+string(rune('0'+i%10)))
		req.RemoteAddr = "203.0.113.5:44000"
		h.HandleSubscribe(httptest.NewRecorder(), req)
	}

	if buckets := h.subscribeRL.Len(); buckets != 1 {
		t.Fatalf("the limiter holds %d buckets, want 1", buckets)
	}
}

func TestRegisterCapsTheStatusBodies(t *testing.T) {
	h, _ := newSubscribeHandler(t)
	mux := http.NewServeMux()
	h.Register(mux, nil)

	req := httptest.NewRequest(http.MethodPost, "/status/subscribe",
		strings.NewReader(`{"email":"`+strings.Repeat("a", 1<<16)+`@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.5:44000"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
}

func quoteJSON(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

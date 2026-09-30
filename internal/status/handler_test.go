// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/ratelimit"
)

type recordingSubscriberStore struct {
	SubscriberStore
	created []string
	deleted []string
}

func (s *recordingSubscriberStore) CreateSubscriber(_ context.Context, sub *StatusSubscriber) (string, error) {
	s.created = append(s.created, sub.Email)
	return "id-" + sub.Email, nil
}

func (s *recordingSubscriberStore) DeleteSubscriber(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}

type sentMail struct {
	to, subject, body string
}

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
	err  error
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, sentMail{to: to, subject: subject, body: body})
	return nil
}

func (m *fakeMailer) mails() []sentMail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]sentMail(nil), m.sent...)
}

func pinEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = original })
}

func newSubscribeHandlerWith(t *testing.T, mailer Mailer) (*Handler, *recordingSubscriberStore) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &recordingSubscriberStore{}
	return &Handler{
		service:     &Service{subscribers: NewSubscriberService(store, mailer, "http://localhost", logger)},
		logger:      logger,
		subscribeRL: ratelimit.New(5.0/3600.0, 5, ratelimit.NewClientIPResolver(nil)),
	}, store
}

func newSubscribeHandler(t *testing.T) (*Handler, *recordingSubscriberStore) {
	t.Helper()
	pinEdition(t, extension.Pro)
	return newSubscribeHandlerWith(t, &fakeMailer{})
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

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type %q, want application/json (body %q)", ct, rec.Body.String())
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if body.Error.Message == "" {
		t.Fatalf("error without a message: %q", rec.Body.String())
	}
	return body.Error.Code
}

func TestHandleSubscribeRefusesOversizedBody(t *testing.T) {
	h, store := newSubscribeHandler(t)

	body := `{"email":"` + strings.Repeat("a", 1<<20) + `@example.com"}`
	rec := postSubscribe(t, h, body)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
	if code := errorCode(t, rec); code != "body_too_large" {
		t.Fatalf("code %q, want body_too_large", code)
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
		if code := errorCode(t, rec); code != "invalid_email" {
			t.Fatalf("%q: code %q, want invalid_email", email, code)
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

func TestHandleSubscribeRateLimitAnswersLikeTheOtherLimiters(t *testing.T) {
	h, _ := newSubscribeHandler(t)

	for i := 0; i < 5; i++ {
		postSubscribe(t, h, `{"email":"ok@example.com"}`)
	}
	rec := postSubscribe(t, h, `{"email":"ok@example.com"}`)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	if code := errorCode(t, rec); code != "rate_limited" {
		t.Fatalf("code %q, want rate_limited", code)
	}
	if got := rec.Header().Get("Retry-After"); got != "720" {
		t.Fatalf("Retry-After %q, want 720: five tokens an hour refill one every twelve minutes", got)
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

func TestHandleSubscribeRefusedWhileSubscriptionsAreClosed(t *testing.T) {
	cases := []struct {
		name    string
		edition extension.Edition
		mailer  Mailer
	}{
		{"no SMTP server", extension.Pro, nil},
		{"edition without subscribers", extension.Personal, &fakeMailer{}},
		{"community", extension.Community, &fakeMailer{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pinEdition(t, tc.edition)
			h, store := newSubscribeHandlerWith(t, tc.mailer)

			rec := postSubscribe(t, h, `{"email":"ok@example.com"}`)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("got %d, want 503", rec.Code)
			}
			if code := errorCode(t, rec); code != "subscriptions_unavailable" {
				t.Fatalf("code %q, want subscriptions_unavailable", code)
			}
			if len(store.created) != 0 {
				t.Fatalf("the store was written to: %v", store.created)
			}
			if h.subscribeRL.Len() != 0 {
				t.Fatal("a refused subscription consumed a rate-limit token")
			}
		})
	}
}

func TestHandleSubscribeDropsTheSubscriberWhenTheConfirmationFails(t *testing.T) {
	pinEdition(t, extension.Pro)
	h, store := newSubscribeHandlerWith(t, &fakeMailer{err: errors.New("connection refused")})

	rec := postSubscribe(t, h, `{"email":"ok@example.com"}`)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", rec.Code)
	}
	if code := errorCode(t, rec); code != "confirmation_failed" {
		t.Fatalf("code %q, want confirmation_failed", code)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "id-ok@example.com" {
		t.Fatalf("deleted %v, want the subscriber just created", store.deleted)
	}
}

func TestHandleConfirmRefusedWhileSubscriptionsAreClosed(t *testing.T) {
	pinEdition(t, extension.Pro)
	h, _ := newSubscribeHandlerWith(t, nil)

	rec := httptest.NewRecorder()
	h.HandleConfirm(rec, httptest.NewRequest(http.MethodGet, "/status/confirm?token=abc", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
}

type emptyComponentStore struct{ ComponentStore }

func (emptyComponentStore) ListVisibleComponents(context.Context) ([]Component, error) {
	return nil, nil
}

func TestStatusAPIReportsWhetherSubscriptionsAreOpen(t *testing.T) {
	cases := []struct {
		name    string
		edition extension.Edition
		mailer  Mailer
		want    bool
	}{
		{"pro with SMTP", extension.Pro, &fakeMailer{}, true},
		{"pro without SMTP", extension.Pro, nil, false},
		{"personal with SMTP", extension.Personal, &fakeMailer{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pinEdition(t, tc.edition)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			svc := NewService(Deps{
				Components:  emptyComponentStore{},
				Logger:      logger,
				Subscribers: NewSubscriberService(&recordingSubscriberStore{}, tc.mailer, "http://localhost", logger),
			})
			h := NewHandler(svc, nil, logger, nil)

			rec := httptest.NewRecorder()
			h.HandleStatusAPI(rec, httptest.NewRequest(http.MethodGet, "/status/api", nil))

			var body StatusAPIResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.SubscriptionsEnabled != tc.want {
				t.Fatalf("subscriptions_enabled %v, want %v", body.SubscriptionsEnabled, tc.want)
			}
		})
	}
}

func quoteJSON(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

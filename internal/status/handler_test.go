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
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/ratelimit"
)

type recordingSubscriberStore struct {
	SubscriberStore
	created   []string
	confirmed map[string]bool
}

func (s *recordingSubscriberStore) UpsertPendingSubscriber(_ context.Context, sub *StatusSubscriber) (bool, error) {
	s.created = append(s.created, sub.Email)
	return !s.confirmed[sub.Email], nil
}

type sentMail struct {
	to, subject, body string
}

type fakeMailer struct {
	sent chan sentMail
	err  error
}

func newFakeMailer() *fakeMailer {
	return &fakeMailer{sent: make(chan sentMail, 64)}
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	if m.err != nil {
		return m.err
	}
	m.sent <- sentMail{to: to, subject: subject, body: body}
	return nil
}

func (m *fakeMailer) next(t *testing.T) sentMail {
	t.Helper()
	select {
	case mail := <-m.sent:
		return mail
	case <-time.After(2 * time.Second):
		t.Fatal("no email was sent")
		return sentMail{}
	}
}

func (m *fakeMailer) none(t *testing.T) {
	t.Helper()
	select {
	case mail := <-m.sent:
		t.Fatalf("unexpected email %q to %s", mail.subject, mail.to)
	case <-time.After(100 * time.Millisecond):
	}
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
	return newSubscribeHandlerWith(t, newFakeMailer())
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
		{"edition without subscribers", extension.Personal, newFakeMailer()},
		{"community", extension.Community, newFakeMailer()},
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

func TestHandleSubscribeAnswersAlikeWhateverTheAddressState(t *testing.T) {
	pinEdition(t, extension.Pro)
	mailer := newFakeMailer()
	h, store := newSubscribeHandlerWith(t, mailer)
	store.confirmed = map[string]bool{"known@example.com": true}

	fresh := postSubscribe(t, h, `{"email":"new@example.com"}`)
	first := mailer.next(t)
	pending := postSubscribe(t, h, `{"email":"new@example.com"}`)
	second := mailer.next(t)
	confirmed := postSubscribe(t, h, `{"email":"known@example.com"}`)
	mailer.none(t)

	for name, rec := range map[string]*httptest.ResponseRecorder{"pending": pending, "confirmed": confirmed} {
		if rec.Code != fresh.Code || rec.Body.String() != fresh.Body.String() || rec.Header().Get("Content-Type") != fresh.Header().Get("Content-Type") {
			t.Fatalf("%s address: got %d %q, a new one got %d %q", name, rec.Code, rec.Body.String(), fresh.Code, fresh.Body.String())
		}
	}
	if fresh.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", fresh.Code)
	}
	if first.to != "new@example.com" || second.to != "new@example.com" {
		t.Fatalf("confirmations went to %s and %s", first.to, second.to)
	}
	if first.body == second.body {
		t.Fatal("a pending address must get a fresh confirmation link")
	}
}

type stalledMailer struct {
	release chan struct{}
}

func (m *stalledMailer) Send(context.Context, string, string, string) error {
	<-m.release
	return nil
}

func TestHandleSubscribeDoesNotWaitForTheMailServer(t *testing.T) {
	pinEdition(t, extension.Pro)
	mailer := &stalledMailer{release: make(chan struct{})}
	t.Cleanup(func() { close(mailer.release) })
	h, _ := newSubscribeHandlerWith(t, mailer)

	done := make(chan int, 1)
	go func() { done <- postSubscribe(t, h, `{"email":"ok@example.com"}`).Code }()

	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the answer waited for the mail server, so its delay tells a new address from a confirmed one")
	}
}

func TestHandleSubscribeAnswersAlikeWhenTheConfirmationFails(t *testing.T) {
	pinEdition(t, extension.Pro)
	mailer := newFakeMailer()
	mailer.err = errors.New("connection refused")
	h, _ := newSubscribeHandlerWith(t, mailer)

	rec := postSubscribe(t, h, `{"email":"ok@example.com"}`)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "confirmation_sent") {
		t.Fatalf("got %d %q: the answer must not depend on the mail server", rec.Code, rec.Body.String())
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
		{"pro with SMTP", extension.Pro, newFakeMailer(), true},
		{"pro without SMTP", extension.Pro, nil, false},
		{"personal with SMTP", extension.Personal, newFakeMailer(), false},
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
			h := NewHandler(svc, nil, logger, nil, "https://status.example.com")

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

func TestStatusAPIDetailsTheMonitorsOfEachComponent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(Deps{
		Components: &mockComponentStore{visibleComponents: []Component{{
			ID: "c1", DisplayName: "API", CompositionMode: CompositionExplicit, Visible: true,
			Monitors: []MonitorRef{{Type: "endpoint", ID: "e1", Name: "https://api.example.com"}},
		}}},
		Logger:        logger,
		MonitorStatus: func(context.Context, string, string) string { return StatusMajorOutage },
	})
	h := NewHandler(svc, nil, logger, nil, "https://status.example.com")

	rec := httptest.NewRecorder()
	h.HandleStatusAPI(rec, httptest.NewRequest(http.MethodGet, "/status/api", nil))

	var body struct {
		Components []struct {
			ID       string       `json:"id"`
			Monitors []MonitorRef `json:"monitors"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := MonitorRef{Type: "endpoint", ID: "e1", Name: "https://api.example.com", Status: StatusMajorOutage}
	if len(body.Components) != 1 || len(body.Components[0].Monitors) != 1 || body.Components[0].Monitors[0] != want {
		t.Fatalf("components %+v, want c1 with the monitor %+v", body.Components, want)
	}
}

type upcomingMaintenanceStore struct {
	MaintenanceStore
	windows []MaintenanceWindow
}

func (s upcomingMaintenanceStore) ListMaintenance(context.Context, string, int) ([]MaintenanceWindow, error) {
	return s.windows, nil
}

type countingIncidentStore struct {
	feedIncidentStore
	recentCalls int
}

func (s *countingIncidentStore) ListRecentIncidents(context.Context, int) ([]Incident, error) {
	s.recentCalls++
	return nil, nil
}

func TestStatusAPINamesOnlyVisibleComponents(t *testing.T) {
	linked := []IncidentCompRef{{ID: "c1", Name: "API", Visible: true}, {ID: "c2", Name: "Internal DB"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(Deps{
		Components:  &mockComponentStore{},
		Logger:      logger,
		Incidents:   feedIncidentStore{active: []Incident{{ID: "inc-1", Title: "Slow", Components: linked}}},
		Maintenance: upcomingMaintenanceStore{windows: []MaintenanceWindow{{ID: "mw-1", Title: "Upgrade", Components: linked}}},
	})
	h := NewHandler(svc, nil, logger, nil, "https://status.example.com")

	rec := httptest.NewRecorder()
	h.HandleStatusAPI(rec, httptest.NewRequest(http.MethodGet, "/status/api", nil))

	var body StatusAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.ActiveIncidents) != 1 || strings.Join(body.ActiveIncidents[0].Components, ",") != "API" {
		t.Fatalf("incidents %+v, want inc-1 naming API only", body.ActiveIncidents)
	}
	if len(body.UpcomingMaint) != 1 || strings.Join(body.UpcomingMaint[0].Components, ",") != "API" {
		t.Fatalf("maintenance %+v, want mw-1 naming API only", body.UpcomingMaint)
	}
}

func TestStatusAPIReadsNoResolvedIncident(t *testing.T) {
	incidents := &countingIncidentStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(Deps{Components: &mockComponentStore{}, Logger: logger, Incidents: incidents})
	h := NewHandler(svc, nil, logger, nil, "https://status.example.com")

	h.HandleStatusAPI(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/status/api", nil))

	if incidents.recentCalls != 0 {
		t.Fatalf("the snapshot read the resolved incidents %d times, and nothing shows them", incidents.recentCalls)
	}
}

func TestHandleSubscribeAcceptsMediaTypeParameters(t *testing.T) {
	h, store := newSubscribeHandler(t)

	for _, ct := range []string{"application/json; charset=utf-8", "Application/JSON"} {
		req := httptest.NewRequest(http.MethodPost, "/status/subscribe", strings.NewReader(`{"email":"ok@example.com"}`))
		req.Header.Set("Content-Type", ct)
		req.RemoteAddr = "203.0.113.5:44000"
		rec := httptest.NewRecorder()
		h.HandleSubscribe(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%q: got %d %q, want 200", ct, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/status/subscribe", strings.NewReader("email=form%40example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.RemoteAddr = "203.0.113.5:44000"
	rec := httptest.NewRecorder()
	h.HandleSubscribe(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("form: got %d %q, want 200", rec.Code, rec.Body.String())
	}
	if want := []string{"ok@example.com", "ok@example.com", "form@example.com"}; strings.Join(store.created, ",") != strings.Join(want, ",") {
		t.Fatalf("stored %v, want %v", store.created, want)
	}
}

func TestHandleSubscribeRefusesAnotherMediaType(t *testing.T) {
	h, store := newSubscribeHandler(t)

	for _, ct := range []string{"", "text/plain", "multipart/form-data; boundary=x", "application/json;;"} {
		req := httptest.NewRequest(http.MethodPost, "/status/subscribe", strings.NewReader(`{"email":"ok@example.com"}`))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		req.RemoteAddr = "203.0.113.5:44000"
		rec := httptest.NewRecorder()
		h.HandleSubscribe(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("%q: got %d, want 415", ct, rec.Code)
		}
		if code := errorCode(t, rec); code != "unsupported_media_type" {
			t.Fatalf("%q: code %q, want unsupported_media_type", ct, code)
		}
	}
	if len(store.created) != 0 {
		t.Fatalf("the store was written to: %v", store.created)
	}
}

func quoteJSON(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

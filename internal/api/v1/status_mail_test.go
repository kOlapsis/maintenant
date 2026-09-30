// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial/statuspage"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/ratelimit"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type recordedMail struct {
	to, subject, body string
}

type recordingMailer struct {
	sent chan recordedMail
	err  error
}

func newRecordingMailer() *recordingMailer {
	return &recordingMailer{sent: make(chan recordedMail, 32)}
}

func (m *recordingMailer) Send(_ context.Context, to, subject, body string) error {
	if m.err != nil {
		return m.err
	}
	m.sent <- recordedMail{to: to, subject: subject, body: body}
	return nil
}

func (m *recordingMailer) next(t *testing.T) recordedMail {
	t.Helper()
	select {
	case mail := <-m.sent:
		return mail
	case <-time.After(2 * time.Second):
		t.Fatal("no email was sent")
		return recordedMail{}
	}
}

func (m *recordingMailer) none(t *testing.T) {
	t.Helper()
	select {
	case mail := <-m.sent:
		t.Fatalf("unexpected email %q to %s", mail.subject, mail.to)
	case <-time.After(150 * time.Millisecond):
	}
}

func serve(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var (
	confirmLink     = regexp.MustCompile(`https://status\.example\.com/status/confirm\?token=[0-9a-f]{64}`)
	unsubscribeLink = regexp.MustCompile(`https://status\.example\.com/status/unsubscribe\?token=[0-9a-f]{64}`)
)

func requestURI(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	require.NoError(t, err)
	return u.RequestURI()
}

func TestStatusPageMailFlow(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	incidents := store.NewIncidentStore(db)
	subscribers := store.NewSubscriberStore(db)
	mailer := newRecordingMailer()
	const baseURL = "https://status.example.com"

	svc := status.NewService(status.Deps{
		Components:  components,
		Logger:      logger,
		Incidents:   incidents,
		Subscribers: status.NewSubscriberService(subscribers, mailer, baseURL, logger),
	})
	svc.SetSubscriberNotifier(statuspage.NewSubscriberNotifier(subscribers, mailer, baseURL, logger))

	public := http.NewServeMux()
	status.NewHandler(svc, http.NotFoundHandler(), logger, ratelimit.New(5.0/3600.0, 5, nil)).Register(public, nil)
	admin := NewRouter(HandlerDeps{
		Logger:            logger,
		StatusComponents:  components,
		StatusIncidents:   incidents,
		StatusSubscribers: subscribers,
		StatusSvc:         svc,
		StatusMailer:      mailer,
	}).Handler()

	var snapshot status.StatusAPIResponse
	require.NoError(t, json.Unmarshal(serve(t, public, http.MethodGet, "/status/api", "").Body.Bytes(), &snapshot))
	require.True(t, snapshot.SubscriptionsEnabled, "the public page offers the subscription form")

	rec := serve(t, public, http.MethodPost, "/status/subscribe", `{"email":"visitor@example.com"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "confirmation_sent")

	confirmation := mailer.next(t)
	assert.Equal(t, "visitor@example.com", confirmation.to)
	link := confirmLink.FindString(confirmation.body)
	require.NotEmpty(t, link, "the confirmation email links to MAINTENANT_BASE_URL/status/confirm: %q", confirmation.body)

	rec = serve(t, admin, http.MethodPost, "/api/v1/status/incidents", `{"title":"Warm-up","severity":"minor"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	mailer.none(t)

	rec = serve(t, public, http.MethodGet, requestURI(t, link), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "confirmed")

	rec = serve(t, admin, http.MethodPost, "/api/v1/status/incidents",
		`{"title":"Database down","severity":"major","message":"We are investigating."}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var inc status.Incident
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inc))

	opened := mailer.next(t)
	assert.Equal(t, "visitor@example.com", opened.to)
	assert.Equal(t, "[major] Database down", opened.subject)
	assert.Contains(t, opened.body, "We are investigating.")
	unsubscribe := unsubscribeLink.FindString(opened.body)
	require.NotEmpty(t, unsubscribe, "every notification carries an unsubscribe link: %q", opened.body)
	mailer.none(t)

	rec = serve(t, admin, http.MethodPost, "/api/v1/status/incidents/"+inc.ID+"/updates",
		`{"status":"monitoring","message":"Fix deployed."}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	update := mailer.next(t)
	assert.Equal(t, "Update: Database down", update.subject)
	assert.Contains(t, update.body, "Fix deployed.")
	mailer.none(t)

	rec = serve(t, admin, http.MethodPost, "/api/v1/status/incidents/"+inc.ID+"/updates",
		`{"status":"resolved","message":"Back to normal."}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	resolved := mailer.next(t)
	assert.Equal(t, "Resolved: Database down", resolved.subject)
	assert.Contains(t, resolved.body, "Back to normal.")
	mailer.none(t)

	rec = serve(t, public, http.MethodGet, requestURI(t, unsubscribe), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	stats, err := subscribers.GetSubscriberStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Total)

	rec = serve(t, admin, http.MethodPost, "/api/v1/status/incidents", `{"title":"After","severity":"minor"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	mailer.none(t)
}

func TestSubscribeRevealsNothingAboutTheAddress(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	subscribers := store.NewSubscriberStore(db)
	mailer := newRecordingMailer()
	svc := status.NewService(status.Deps{
		Components:  store.NewStatusComponentStore(db),
		Logger:      logger,
		Subscribers: status.NewSubscriberService(subscribers, mailer, "https://status.example.com", logger),
	})
	public := http.NewServeMux()
	status.NewHandler(svc, http.NotFoundHandler(), logger, ratelimit.New(5.0/3600.0, 5, nil)).Register(public, nil)
	subscribe := func() *httptest.ResponseRecorder {
		return serve(t, public, http.MethodPost, "/status/subscribe", `{"email":"visitor@example.com"}`)
	}

	fresh := subscribe()
	require.Equal(t, http.StatusOK, fresh.Code, fresh.Body.String())
	firstLink := confirmLink.FindString(mailer.next(t).body)
	require.NotEmpty(t, firstLink)

	pending := subscribe()
	secondLink := confirmLink.FindString(mailer.next(t).body)
	require.NotEmpty(t, secondLink)
	assert.NotEqual(t, firstLink, secondLink, "a pending address gets a fresh link")

	rec := serve(t, public, http.MethodGet, requestURI(t, firstLink), "")
	assert.Equal(t, http.StatusBadRequest, rec.Code, "the replaced link no longer confirms")
	rec = serve(t, public, http.MethodGet, requestURI(t, secondLink), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	confirmed := subscribe()
	mailer.none(t)

	for name, got := range map[string]*httptest.ResponseRecorder{"pending": pending, "confirmed": confirmed} {
		assert.Equal(t, fresh.Code, got.Code, name)
		assert.Equal(t, fresh.Body.String(), got.Body.String(), name)
		assert.Equal(t, fresh.Header().Get("Content-Type"), got.Header().Get("Content-Type"), name)
	}
	stats, err := subscribers.GetSubscriberStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Total)
	assert.Equal(t, 1, stats.Confirmed)
}

func TestStatusSmtpTest(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	router := func(mailer status.Mailer) http.Handler {
		return NewRouter(HandlerDeps{
			Logger:           logger,
			StatusComponents: components,
			StatusSvc:        status.NewService(status.Deps{Components: components, Logger: logger}),
			StatusMailer:     mailer,
		}).Handler()
	}

	t.Run("sends to the given address", func(t *testing.T) {
		mailer := newRecordingMailer()
		rec := serve(t, router(mailer), http.MethodPost, "/api/v1/status/smtp/test", `{"to":"ops@example.com"}`)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "ops@example.com", mailer.next(t).to)
	})

	t.Run("without SMTP", func(t *testing.T) {
		rec := serve(t, router(nil), http.MethodPost, "/api/v1/status/smtp/test", `{"to":"ops@example.com"}`)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "not_configured", decodeRefusal(t, rec).Code)
	})

	t.Run("invalid address", func(t *testing.T) {
		mailer := newRecordingMailer()
		rec := serve(t, router(mailer), http.MethodPost, "/api/v1/status/smtp/test", `{"to":"not an address"}`)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "validation", decodeRefusal(t, rec).Code)
		mailer.none(t)
	})

	t.Run("server failure", func(t *testing.T) {
		mailer := newRecordingMailer()
		mailer.err = errors.New("535 authentication failed")
		rec := serve(t, router(mailer), http.MethodPost, "/api/v1/status/smtp/test", `{"to":"ops@example.com"}`)
		require.Equal(t, http.StatusBadGateway, rec.Code)
		detail := decodeRefusal(t, rec)
		assert.Equal(t, "smtp_failed", detail.Code)
		assert.Contains(t, detail.Message, "535 authentication failed")
	})

	t.Run("the SMTP settings are no longer served nor accepted", func(t *testing.T) {
		h := router(newRecordingMailer())
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			rec := serve(t, h, method, "/api/v1/status/smtp", `{"host":"smtp.example.com"}`)
			assert.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, rec.Code, method)
		}
	})
}

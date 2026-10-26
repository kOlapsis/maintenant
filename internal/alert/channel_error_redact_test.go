// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13: a delivery error never carries the webhook URL.

// secretPaths are the parts of a platform webhook URL that must never reach a log or a record.
var secretPaths = []string{
	"/services/T000/B000/XXXXsecret",
	"/api/webhooks/1234/abcdTOKEN",
	"/webhookb2/guid@guid/IncomingWebhook/id/key",
}

// refusedURL returns a URL on a port nobody listens on, carrying secretPath.
func refusedURL(t *testing.T, secretPath string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return "http://" + addr + secretPath
}

func assertCauseWithoutURL(t *testing.T, msg, url string) {
	t.Helper()
	assert.Contains(t, msg, "connection refused", "the cause must stay")
	assert.NotContains(t, msg, url)
	assert.NotContains(t, msg, "http://", "no URL at all, not even truncated")
	for _, p := range secretPaths {
		assert.NotContains(t, msg, p)
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

func quickRetries(t *testing.T) {
	t.Helper()
	prev := retryBackoffs
	retryBackoffs = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryBackoffs = prev })
}

// FR-002, US1 scenario 2: Send reports the dial error without the URL, as the outbound heartbeats do.
func TestWebhookSender_SendKeepsTheURLOutOfTheError(t *testing.T) {
	n := newTestNotifier()
	for _, p := range secretPaths {
		t.Run(p, func(t *testing.T) {
			url := refusedURL(t, p)
			ch := &NotificationChannel{ID: "c1", Type: "webhook", URL: url}

			err := n.webhook.Send(context.Background(), ch, "alert.fired", &Alert{ID: "a1", Status: StatusActive})
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "send request: "), "got %q", err.Error())
			assert.Contains(t, err.Error(), "dial tcp")
			assertCauseWithoutURL(t, err.Error(), url)
			assertCauseWithoutURL(t, n.webhook.FailureMessage(err), url)
		})
	}
}

// FR-002, US1 scenario 2: the answer of POST /api/v1/channels/{id}/test carries the cause without the URL.
func TestSendTestWebhook_KeepsTheURLOutOfTheError(t *testing.T) {
	n := newTestNotifier()
	for _, typ := range []string{"webhook", "discord"} {
		url := refusedURL(t, secretPaths[1])
		code, err := n.SendTestWebhook(context.Background(), &NotificationChannel{ID: "c2", Type: typ, URL: url})
		require.Error(t, err)
		assert.Equal(t, 0, code)
		assertCauseWithoutURL(t, err.Error(), url)
	}
}

// FR-002, US1 scenario 2: last_error and the notifier's logs carry the cause without the URL.
func TestProcessJob_RecordsAndLogsTheCauseWithoutTheURL(t *testing.T) {
	quickRetries(t)
	logs := &lockedBuffer{}
	rec := &deliveryRecorder{}
	n := NewNotifier(rec, slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelInfo})), true)
	url := refusedURL(t, secretPaths[0])

	job := NotificationJob{
		Delivery: &NotificationDelivery{ID: "d1"},
		Channel:  &NotificationChannel{ID: "c1", Type: "webhook", URL: url},
		Alert:    &Alert{ID: "a1", Status: StatusActive},
	}
	n.processJob(context.Background(), job)

	assert.Equal(t, DeliveryFailed, job.Delivery.Status)
	assertCauseWithoutURL(t, job.Delivery.LastError, url)
	require.NotEmpty(t, rec.updates)
	assertCauseWithoutURL(t, rec.updates[len(rec.updates)-1].LastError, url)
	assert.Contains(t, logs.String(), "connection refused")
	assertCauseWithoutURL(t, logs.String(), url)

	err := n.SendNow(context.Background(), job.Alert, job.Channel)
	require.Error(t, err)
	assertCauseWithoutURL(t, err.Error(), url)
}

// Edge case "erreur d'envoi qui n'est pas une erreur d'URL": an HTTP 500 keeps today's message.
func TestWebhookSender_AnHTTPErrorKeepsItsMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	}))
	t.Cleanup(srv.Close)
	n := newTestNotifier()
	ch := &NotificationChannel{ID: "c3", Type: "webhook", URL: srv.URL + "/hook"}

	err := n.webhook.Send(context.Background(), ch, "alert.fired", &Alert{ID: "a1", Status: StatusActive})
	require.EqualError(t, err, "non-2xx response: 500")

	code, err := n.SendTestWebhook(context.Background(), ch)
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.Equal(t, "HTTP 500: boom", err.Error())
}

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
package channels

import (
	"bytes"
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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

type deliveryRecorder struct {
	alert.ChannelStore
	mu      sync.Mutex
	updates []alert.NotificationDelivery
}

func (r *deliveryRecorder) UpdateDelivery(_ context.Context, d *alert.NotificationDelivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, *d)
	return nil
}

func (r *deliveryRecorder) last() (alert.NotificationDelivery, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.updates) == 0 {
		return alert.NotificationDelivery{}, false
	}
	return r.updates[len(r.updates)-1], true
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// telegramNotifier wires a notifier onto a stub Bot API, the Telegram sender registered as in production.
func telegramNotifier(t *testing.T, store alert.ChannelStore, status int, body string) (*alert.Notifier, *[]string, *syncBuffer) {
	t.Helper()
	var bodies []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	logs := &syncBuffer{}
	n := alert.NewNotifier(store, slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), true)
	n.RegisterChannel("telegram", NewTelegramSender(srv.Client(), srv.URL))
	return n, &bodies, logs
}

// A Telegram channel must not take the generic webhook path: that path posts
// our JSON envelope to ch.URL, which for Telegram is a chat id, not a URL.
func TestEnqueue_TelegramTakesItsOwnPath(t *testing.T) {
	rec := &deliveryRecorder{}
	n, bodies, logs := telegramNotifier(t, rec, http.StatusOK, `{"ok":true}`)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	n.Start(ctx)

	n.Enqueue(alert.NotificationJob{
		Delivery: &alert.NotificationDelivery{ID: "d1", AlertID: "a1", ChannelID: "c1"},
		Channel:  telegramChannel(),
		Alert:    firedAlert(),
	})

	require.Eventually(t, func() bool {
		d, ok := rec.last()
		return ok && d.Status == alert.DeliveryDelivered
	}, 5*time.Second, 10*time.Millisecond)
	cancel()

	require.Len(t, *bodies, 1)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte((*bodies)[0]), &payload))
	assert.Equal(t, "HTML", payload["parse_mode"], "the Telegram payload, not the webhook envelope")
	assert.NotContains(t, payload, "event", "the webhook envelope would carry an event key")

	assert.NotContains(t, logs.String(), sentinelToken, "no log line may carry the token")
	assert.NotContains(t, logs.String(), "sendMessage", "the called URL is never logged")
}

func TestSendNow_TelegramRecoveryUsesTheRecoveryTemplate(t *testing.T) {
	n, bodies, _ := telegramNotifier(t, &deliveryRecorder{}, http.StatusOK, `{"ok":true}`)

	a := firedAlert()
	a.Status = alert.StatusResolved
	require.NoError(t, n.SendNow(context.Background(), a, telegramChannel()))

	require.Len(t, *bodies, 1)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte((*bodies)[0]), &payload))
	text, _ := payload["text"].(string)
	assert.True(t, strings.HasPrefix(text, "\xE2\x9C\x85"), "a recovery opens on the check mark")
	assert.Contains(t, text, "Resolved: api.example.com")
}

// After the retries are spent, the delivery row says what happened, in
// Telegram's words, and without the token.
func TestTelegramSender_FailureMessageIsTelegramsReason(t *testing.T) {
	srv, _, _ := telegramStub(t, http.StatusBadRequest, `{"ok":false,"description":"chat not found"}`)
	s := NewTelegramSender(srv.Client(), srv.URL)

	err := s.Send(context.Background(), telegramChannel(), "alert.fired", firedAlert())
	require.Error(t, err)
	msg := s.FailureMessage(err)
	assert.Contains(t, msg, "chat not found")
	assert.NotContains(t, msg, sentinelToken)
}

// FR-014: the delay Telegram names wins over the product's backoff, never the reverse.
func TestTelegramSender_RetryDelayHonoursTheRateLimit(t *testing.T) {
	s := NewTelegramSender(http.DefaultClient, TelegramAPIBase)
	assert.Equal(t, time.Second, s.RetryDelay(time.Second, errors.New("boom")))
	assert.Equal(t, 30*time.Second, s.RetryDelay(time.Second, &TelegramRateLimitError{RetryAfter: 30 * time.Second}))
}

// The escalation runner calls SendNow, not Enqueue. Sending generic webhook
// JSON to Telegram from that path would fail for everyone on call.
func TestSendNow_TelegramTakesItsOwnPath(t *testing.T) {
	n, bodies, _ := telegramNotifier(t, &deliveryRecorder{}, http.StatusOK, `{"ok":true}`)

	require.NoError(t, n.SendNow(context.Background(), firedAlert(), telegramChannel()))

	require.Len(t, *bodies, 1)
	assert.Contains(t, (*bodies)[0], `"parse_mode":"HTML"`)
}

// The test button must reach Telegram too, and report its reason.
func TestSendTestWebhook_Telegram(t *testing.T) {
	n, bodies, _ := telegramNotifier(t, &deliveryRecorder{}, http.StatusOK, `{"ok":true}`)

	status, err := n.SendTestWebhook(context.Background(), telegramChannel())
	require.NoError(t, err)
	assert.Equal(t, 200, status)
	require.Len(t, *bodies, 1)
	assert.Contains(t, (*bodies)[0], "maintenant Test Notification")
}

func TestSendTestWebhook_TelegramReportsTheReason(t *testing.T) {
	n, _, _ := telegramNotifier(t, &deliveryRecorder{}, http.StatusBadRequest,
		`{"ok":false,"description":"chat not found"}`)

	_, err := n.SendTestWebhook(context.Background(), telegramChannel())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat not found")
	assert.NotContains(t, err.Error(), sentinelToken)
}

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

package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deliveryRecorder is the slice of ChannelStore the notifier actually uses when
// it reports an outcome.
type deliveryRecorder struct {
	ChannelStore
	mu      sync.Mutex
	updates []NotificationDelivery
}

func (r *deliveryRecorder) UpdateDelivery(_ context.Context, d *NotificationDelivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, *d)
	return nil
}

func (r *deliveryRecorder) last() NotificationDelivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.updates[len(r.updates)-1]
}

// telegramNotifier wires a notifier onto a stub Bot API. Both the host and the
// client are handed over: the notifier's own client carries the SSRF guard.
func telegramNotifier(t *testing.T, store ChannelStore, status int, body string) (*Notifier, *[]string, *bytes.Buffer) {
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

	logs := &bytes.Buffer{}
	n := NewNotifier(store, slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), true)
	n.SetTelegramTransport(srv.URL, srv.Client())
	return n, &bodies, logs
}

// A Telegram channel must not take the generic webhook path: that path posts
// our JSON envelope to ch.URL, which for Telegram is a chat id, not a URL.
func TestProcessJob_TelegramTakesItsOwnPath(t *testing.T) {
	rec := &deliveryRecorder{}
	n, bodies, logs := telegramNotifier(t, rec, http.StatusOK, `{"ok":true}`)

	n.processJob(context.Background(), NotificationJob{
		Delivery: &NotificationDelivery{ID: "d1", AlertID: "a1", ChannelID: "c1"},
		Channel:  telegramChannel(),
		Alert:    firedAlert(),
	})

	require.Len(t, *bodies, 1)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte((*bodies)[0]), &payload))
	assert.Equal(t, "HTML", payload["parse_mode"], "the Telegram payload, not the webhook envelope")
	assert.NotContains(t, payload, "event", "the webhook envelope would carry an event key")

	assert.Equal(t, DeliveryDelivered, rec.last().Status)
	assert.NotContains(t, logs.String(), sentinelToken, "no log line may carry the token")
	assert.NotContains(t, logs.String(), "sendMessage", "the called URL is never logged")
}

func TestProcessJob_TelegramRecoveryUsesTheRecoveryTemplate(t *testing.T) {
	rec := &deliveryRecorder{}
	n, bodies, _ := telegramNotifier(t, rec, http.StatusOK, `{"ok":true}`)

	a := firedAlert()
	a.Status = StatusResolved
	n.processJob(context.Background(), NotificationJob{
		Delivery: &NotificationDelivery{ID: "d1", AlertID: "a1", ChannelID: "c1"},
		Channel:  telegramChannel(),
		Alert:    a,
	})

	require.Len(t, *bodies, 1)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte((*bodies)[0]), &payload))
	text, _ := payload["text"].(string)
	assert.True(t, strings.HasPrefix(text, "\xE2\x9C\x85"), "a recovery opens on the check mark")
	assert.Contains(t, text, "Resolved: api.example.com")
}

// After the retries are spent, the delivery row says what happened, in
// Telegram's words, and without the token.
func TestProcessJob_TelegramFailureRecordsTelegramsReason(t *testing.T) {
	shortenBackoffs(t)

	rec := &deliveryRecorder{}
	n, bodies, logs := telegramNotifier(t, rec, http.StatusBadRequest,
		`{"ok":false,"description":"chat not found"}`)

	n.processTelegramJob(context.Background(), NotificationJob{
		Delivery: &NotificationDelivery{ID: "d1", AlertID: "a1", ChannelID: "c1"},
		Channel:  telegramChannel(),
		Alert:    firedAlert(),
	}, event.AlertFired)

	assert.Len(t, *bodies, maxRetries, "the shared retry policy applies here too (FR-013)")

	last := rec.last()
	assert.Equal(t, DeliveryFailed, last.Status)
	assert.Equal(t, maxRetries, last.Attempts)
	assert.Contains(t, last.LastError, "chat not found")
	assert.NotContains(t, last.LastError, sentinelToken)
	assert.NotContains(t, logs.String(), sentinelToken)
}

// FR-014, end to end: Telegram asks for a delay, and the next attempt waits at
// least that long. The delay is small so the test is fast; what matters is that
// it wins over the product's backoff, which shortenBackoffs made shorter still.
func TestProcessJob_TelegramHonoursTheRateLimitDelay(t *testing.T) {
	shortenBackoffs(t)

	rec := &deliveryRecorder{}
	n, bodies, _ := telegramNotifier(t, rec, http.StatusTooManyRequests,
		`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":0}}`)

	start := time.Now()
	n.processTelegramJob(context.Background(), NotificationJob{
		Delivery: &NotificationDelivery{ID: "d1", AlertID: "a1", ChannelID: "c1"},
		Channel:  telegramChannel(),
		Alert:    firedAlert(),
	}, event.AlertFired)
	elapsed := time.Since(start)

	assert.Len(t, *bodies, maxRetries)
	assert.Less(t, elapsed, time.Second, "a zero delay must not be padded")
	assert.Equal(t, DeliveryFailed, rec.last().Status)

	// And the delay is honoured when Telegram names a real one.
	assert.Equal(t, 250*time.Millisecond,
		telegramBackoff(1, &TelegramRateLimitError{RetryAfter: 250 * time.Millisecond}))
}

// shortenBackoffs replaces the product's 1s/5s/25s with something a test can
// wait for. The policy itself is asserted in TestTelegramBackoff.
func shortenBackoffs(t *testing.T) {
	t.Helper()
	original := retryBackoffs
	retryBackoffs = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryBackoffs = original })
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

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestNotifier() *Notifier {
	// allowPrivate=true: tests deliver to httptest servers on 127.0.0.1.
	return NewNotifier(nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), true)
}

func captureServer(t *testing.T, statusCode int) (*httptest.Server, *[]byte, *string) {
	t.Helper()
	var body []byte
	var ct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct = r.Header.Get("Content-Type")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(statusCode)
	}))
	t.Cleanup(srv.Close)
	return srv, &body, &ct
}

func TestSendTestWebhook_Discord(t *testing.T) {
	srv, body, ct := captureServer(t, http.StatusNoContent)

	n := newTestNotifier()
	ch := &NotificationChannel{Type: "discord", URL: srv.URL}

	code, err := n.SendTestWebhook(context.Background(), ch)

	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, code)
	assert.Equal(t, "application/json", *ct)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(*body, &payload))

	embeds, ok := payload["embeds"].([]interface{})
	require.True(t, ok, "payload must have 'embeds' array")
	require.Len(t, embeds, 1)

	embed := embeds[0].(map[string]interface{})
	assert.NotEmpty(t, embed["title"])
	assert.NotEmpty(t, embed["description"])

	// Discord color must be integer 0-16777215
	color, ok := embed["color"].(float64) // JSON numbers unmarshal as float64
	require.True(t, ok, "color must be a JSON number")
	assert.GreaterOrEqual(t, color, float64(0))
	assert.LessOrEqual(t, color, float64(16777215))

	// color must be serialized as integer (no decimal point) in raw JSON
	colorJSON, _ := json.Marshal(int(color))
	assert.Contains(t, string(*body), string(colorJSON), "color must be a JSON integer, not a float")

	// Fields must have non-empty name and value
	fields, ok := embed["fields"].([]interface{})
	require.True(t, ok)
	for i, f := range fields {
		field := f.(map[string]interface{})
		assert.NotEmpty(t, field["name"], "field[%d].name must not be empty", i)
		assert.NotEmpty(t, field["value"], "field[%d].value must not be empty", i)
	}

	t.Logf("Discord payload:\n%s", mustPretty(*body))
}

func TestSendTestWebhook_Non2xx_ReturnsError(t *testing.T) {
	srv, _, _ := captureServer(t, http.StatusBadRequest)

	n := newTestNotifier()
	ch := &NotificationChannel{Type: "discord", URL: srv.URL}

	code, err := n.SendTestWebhook(context.Background(), ch)

	assert.Equal(t, http.StatusBadRequest, code)
	require.Error(t, err, "non-2xx must return an error")
	assert.Contains(t, err.Error(), "400")
}

func TestSendTestWebhook_Generic(t *testing.T) {
	srv, body, _ := captureServer(t, http.StatusOK)

	n := newTestNotifier()
	ch := &NotificationChannel{Type: "webhook", URL: srv.URL}

	_, err := n.SendTestWebhook(context.Background(), ch)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(*body, &payload))
	assert.NotEmpty(t, payload["event"])
	assert.NotEmpty(t, payload["timestamp"])

	t.Logf("Generic webhook payload:\n%s", mustPretty(*body))
}

func mustPretty(b []byte) string {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}

type fakeSender struct {
	mu        sync.Mutex
	ready     error
	fail      error
	sends     int
	delays    []time.Duration
	lastEvent string
}

func (f *fakeSender) Ready() error { return f.ready }

func (f *fakeSender) Send(_ context.Context, _ *NotificationChannel, eventType string, _ *Alert) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends++
	f.lastEvent = eventType
	return f.fail
}

func (f *fakeSender) RetryDelay(wait time.Duration, _ error) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delays = append(f.delays, wait)
	return time.Millisecond
}

func (f *fakeSender) FailureMessage(err error) string { return "fake: " + err.Error() }

func (f *fakeSender) SendTest(context.Context, *NotificationChannel, *Alert) (int, error) {
	return 299, nil
}

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

func testJob(chType string) NotificationJob {
	return NotificationJob{
		Delivery: &NotificationDelivery{ID: "d1"},
		Channel:  &NotificationChannel{ID: "c1", Type: chType},
		Alert:    &Alert{ID: "a1", Status: StatusResolved},
	}
}

func TestProcessJob_RoutesEachTypeToItsSender(t *testing.T) {
	rec := &deliveryRecorder{}
	n := NewNotifier(rec, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	a, b := &fakeSender{}, &fakeSender{}
	n.RegisterChannel("a", a)
	n.RegisterChannel("b", b)

	n.processJob(context.Background(), testJob("a"))
	n.processJob(context.Background(), testJob("b"))
	n.processJob(context.Background(), testJob("b"))

	assert.Equal(t, 1, a.sends)
	assert.Equal(t, 2, b.sends)
	assert.Equal(t, "alert.resolved", b.lastEvent)
	assert.Equal(t, DeliveryDelivered, rec.updates[len(rec.updates)-1].Status)

	code, err := n.SendTestWebhook(context.Background(), &NotificationChannel{Type: "a"})
	require.NoError(t, err)
	assert.Equal(t, 299, code)
}

func TestProcessJob_UsesTheSendersRetryPolicy(t *testing.T) {
	rec := &deliveryRecorder{}
	n := NewNotifier(rec, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	s := &fakeSender{fail: errors.New("refused")}
	n.RegisterChannel("x", s)

	job := testJob("x")
	n.processJob(context.Background(), job)

	assert.Equal(t, maxRetries, s.sends)
	assert.Equal(t, retryBackoffs, s.delays, "the sender is handed every backoff of the product, and no other")
	assert.Equal(t, DeliveryFailed, job.Delivery.Status)
	assert.Equal(t, maxRetries, job.Delivery.Attempts)
	assert.Equal(t, "fake: refused", job.Delivery.LastError)

	require.EqualError(t, n.SendNow(context.Background(), job.Alert, job.Channel), "refused")
}

func TestProcessJob_ReportsTheFinalOutcomeOnce(t *testing.T) {
	n := NewNotifier(&deliveryRecorder{}, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	n.RegisterChannel("ok", &fakeSender{})
	n.RegisterChannel("ko", &fakeSender{fail: errors.New("refused")})

	for chType, wantErr := range map[string]bool{"ok": false, "ko": true} {
		var outcomes []error
		job := testJob(chType)
		job.Done = func(_ context.Context, err error) { outcomes = append(outcomes, err) }
		n.processJob(context.Background(), job)

		require.Len(t, outcomes, 1, "channel %s", chType)
		assert.Equal(t, wantErr, outcomes[0] != nil, "channel %s", chType)
	}
}

// firstAttemptFails refuses the first delivery it is handed and records the events it delivers.
type firstAttemptFails struct {
	mu        sync.Mutex
	refused   bool
	delivered []string
}

func (s *firstAttemptFails) Ready() error { return nil }

func (s *firstAttemptFails) Send(_ context.Context, _ *NotificationChannel, eventType string, _ *Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.refused {
		s.refused = true
		return errors.New("receiver unavailable")
	}
	s.delivered = append(s.delivered, eventType)
	return nil
}

func (s *firstAttemptFails) RetryDelay(time.Duration, error) time.Duration {
	return 50 * time.Millisecond
}

func (s *firstAttemptFails) FailureMessage(err error) string { return err.Error() }

func (s *firstAttemptFails) SendTest(context.Context, *NotificationChannel, *Alert) (int, error) {
	return http.StatusOK, nil
}

func (s *firstAttemptFails) events() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.delivered...)
}

func TestNotifier_ARecoveryNeverOvertakesItsAlert(t *testing.T) {
	n := NewNotifier(&deliveryRecorder{}, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	s := &firstAttemptFails{}
	n.RegisterChannel("x", s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n.Start(ctx)

	ch := &NotificationChannel{ID: "c1", Type: "x"}
	n.Enqueue(NotificationJob{Delivery: &NotificationDelivery{}, Channel: ch, Alert: &Alert{ID: "a1", Status: StatusActive}})
	n.Enqueue(NotificationJob{Delivery: &NotificationDelivery{}, Channel: ch, Alert: &Alert{ID: "a1", Status: StatusResolved}})

	require.Eventually(t, func() bool { return len(s.events()) == 2 }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"alert.fired", "alert.resolved"}, s.events(),
		"the recovery waits for the alert's retried delivery")
}

func withEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = prev })
}

func TestProcessJob_UnreadySenderFailsWithoutAnAttempt(t *testing.T) {
	withEdition(t, extension.Pro)
	rec := &deliveryRecorder{}
	n := NewNotifier(rec, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	s := &fakeSender{ready: errors.New("SMTP not configured")}
	n.RegisterChannel("email", s)
	assert.False(t, n.SMTPConfigured())

	job := testJob("email")
	n.processJob(context.Background(), job)

	assert.Zero(t, s.sends)
	assert.Zero(t, job.Delivery.Attempts)
	assert.Equal(t, "SMTP not configured", job.Delivery.LastError)
	require.EqualError(t, n.SendNow(context.Background(), job.Alert, job.Channel), "SMTP not configured")
	_, err := n.SendTestWebhook(context.Background(), job.Channel)
	require.EqualError(t, err, "SMTP not configured")
}

func TestSendTestWebhook_UnregisteredTypeIsAGenericWebhook(t *testing.T) {
	withEdition(t, extension.Pro)
	srv, body, _ := captureServer(t, http.StatusOK)
	n := newTestNotifier()

	for _, chType := range []string{"slack", "email", "made-up"} {
		_, err := n.SendTestWebhook(context.Background(), &NotificationChannel{Type: chType, URL: srv.URL})
		require.NoError(t, err, chType)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(*body, &payload))
		assert.Equal(t, "test", payload["event"], chType)
	}

	_, ok := n.Validator("webhook")
	assert.False(t, ok)
}

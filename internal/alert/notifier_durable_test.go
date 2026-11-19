// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type hookRequest struct {
	Delivery string
	Event    string
}

// hookServer answers each request with status(n), n counting from 1, and records what it received.
type hookServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []hookRequest
}

func newHookServer(t *testing.T, status func(n int) int) *hookServer {
	t.Helper()
	s := &hookServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p alert.WebhookPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		s.mu.Lock()
		s.requests = append(s.requests, hookRequest{Delivery: r.Header.Get("X-maintenant-Delivery"), Event: p.Event})
		n := len(s.requests)
		s.mu.Unlock()
		w.WriteHeader(status(n))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *hookServer) received() []hookRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]hookRequest(nil), s.requests...)
}

func always(status int) func(int) int { return func(int) int { return status } }

type queueFixture struct {
	ctx      context.Context
	logger   *slog.Logger
	channels *store.ChannelStoreImpl
	payload  *alert.Alert
	clock    *fakeClock
}

func newQueueFixture(t *testing.T) *queueFixture {
	t.Helper()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	alerts := store.NewAlertStore(db)
	id, err := alerts.InsertAlert(ctx, &alert.Alert{
		Source: "endpoint", AlertType: "down", Severity: alert.SeverityCritical, Status: alert.StatusActive,
		Message: "api down", EntityType: "endpoint", EntityID: "e1", EntityName: "api", AgentID: uid.LocalAgent,
		Details: "{}", FiredAt: time.Now().UTC().Truncate(time.Second),
	})
	require.NoError(t, err)
	payload, err := alerts.GetAlert(ctx, id)
	require.NoError(t, err)
	return &queueFixture{
		ctx: ctx, logger: logger, channels: store.NewChannelStore(db), payload: payload,
		clock: &fakeClock{now: time.Now()},
	}
}

func (f *queueFixture) channel(t *testing.T, url string) *alert.NotificationChannel {
	t.Helper()
	id, err := f.channels.InsertChannel(f.ctx, &alert.NotificationChannel{Name: "hook-" + uid.New(), Type: "webhook", URL: url, Enabled: true})
	require.NoError(t, err)
	ch, err := f.channels.GetChannel(f.ctx, id)
	require.NoError(t, err)
	return ch
}

func (f *queueFixture) delivery(t *testing.T, ch *alert.NotificationChannel, eventType string, status string) *alert.NotificationDelivery {
	t.Helper()
	payload := *f.payload
	payload.Status = status
	d := &alert.NotificationDelivery{AlertID: payload.ID, ChannelID: ch.ID, EventType: eventType, Status: alert.DeliveryPending, Payload: &payload}
	_, err := f.channels.InsertDelivery(f.ctx, d)
	require.NoError(t, err)
	return d
}

func (f *queueFixture) notifier() *alert.Notifier {
	n := alert.NewNotifier(f.channels, f.logger, true)
	n.SetClock(f.clock.Now)
	n.SetResumeInterval(5 * time.Millisecond)
	return n
}

func (f *queueFixture) row(id string) *alert.NotificationDelivery {
	rows, err := f.channels.ListDeliveriesByAlert(f.ctx, f.payload.ID)
	if err != nil {
		return nil
	}
	for _, d := range rows {
		if d.ID == id {
			return d
		}
	}
	return nil
}

func (f *queueFixture) countStatus(status string) int {
	rows, err := f.channels.ListDeliveriesByAlert(f.ctx, f.payload.ID)
	if err != nil {
		return -1
	}
	n := 0
	for _, d := range rows {
		if d.Status == status {
			n++
		}
	}
	return n
}

func job(d *alert.NotificationDelivery, ch *alert.NotificationChannel) alert.NotificationJob {
	return alert.NotificationJob{Delivery: d, Channel: ch, Alert: d.Payload}
}

func TestDurableDelivery_RetriedUntilTheDestinationAnswers(t *testing.T) {
	f := newQueueFixture(t)
	srv := newHookServer(t, func(n int) int {
		if n <= 3 {
			return http.StatusServiceUnavailable
		}
		return http.StatusOK
	})
	ch := f.channel(t, srv.URL)
	d := f.delivery(t, ch, alert.EventFired, alert.StatusActive)
	n := f.notifier()
	n.StartUntilCleanup(f.ctx, t)
	n.Enqueue(job(d, ch))

	start := f.clock.Now()
	var scheduled []time.Time
	for i, wait := range []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second} {
		attempts := i + 1
		require.Eventually(t, func() bool {
			r := f.row(d.ID)
			return r != nil && r.Status == alert.DeliveryPending && r.Attempts == attempts
		}, 10*time.Second, 5*time.Millisecond, "attempt %d must be recorded and rescheduled", attempts)
		scheduled = append(scheduled, f.row(d.ID).NextAttemptAt)
		assert.Equal(t, "non-2xx response: 503", f.row(d.ID).LastError)
		assert.Never(t, func() bool { return len(srv.received()) > attempts }, 50*time.Millisecond, 5*time.Millisecond,
			"no attempt before the next one is due")
		f.clock.Advance(wait)
	}

	require.Eventually(t, func() bool {
		r := f.row(d.ID)
		return r != nil && r.Status == alert.DeliveryDelivered
	}, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, 4, f.row(d.ID).Attempts)
	at := func(d time.Duration) time.Time { return time.Unix(start.Add(d).Unix(), 0) }
	assert.Equal(t, []time.Time{at(5 * time.Second), at(15 * time.Second), at(35 * time.Second)}, scheduled,
		"each retry waits twice as long as the one before, from 5 seconds")
	assert.Len(t, srv.received(), 4)
}

func TestDurableDelivery_KeepsItsIDAcrossAttempts(t *testing.T) {
	f := newQueueFixture(t)
	srv := newHookServer(t, func(n int) int {
		if n == 1 {
			return http.StatusServiceUnavailable
		}
		return http.StatusOK
	})
	ch := f.channel(t, srv.URL)
	d := f.delivery(t, ch, alert.EventFired, alert.StatusActive)
	n := f.notifier()
	n.StartUntilCleanup(f.ctx, t)
	n.Enqueue(job(d, ch))

	require.Eventually(t, func() bool { r := f.row(d.ID); return r != nil && r.Attempts == 1 }, 10*time.Second, 5*time.Millisecond)
	f.clock.Advance(5 * time.Second)
	require.Eventually(t, func() bool { return f.countStatus(alert.DeliveryDelivered) == 1 }, 10*time.Second, 5*time.Millisecond)

	got := srv.received()
	require.Len(t, got, 2)
	assert.Equal(t, d.ID, got[0].Delivery, "the delivery id is the row id")
	assert.Equal(t, got[0].Delivery, got[1].Delivery, "and the retry carries the same one, so the receiver can drop a duplicate")
}

func TestDurableDelivery_ResumedAfterARestart(t *testing.T) {
	t.Run("a recent pending delivery is sent", func(t *testing.T) {
		f := newQueueFixture(t)
		srv := newHookServer(t, always(http.StatusOK))
		ch := f.channel(t, srv.URL)
		d := f.delivery(t, ch, alert.EventFired, alert.StatusActive)

		f.notifier().StartUntilCleanup(f.ctx, t)

		require.Eventually(t, func() bool { return f.countStatus(alert.DeliveryDelivered) == 1 }, 10*time.Second, 5*time.Millisecond,
			"a delivery nobody queued since the restart is found in the database")
		assert.Equal(t, []hookRequest{{Delivery: d.ID, Event: "alert.fired"}}, srv.received())
	})

	t.Run("a pending delivery past the age bound is closed unsent", func(t *testing.T) {
		f := newQueueFixture(t)
		srv := newHookServer(t, always(http.StatusOK))
		ch := f.channel(t, srv.URL)
		d := f.delivery(t, ch, alert.EventFired, alert.StatusActive)
		f.clock.Advance(25 * time.Hour)

		f.notifier().StartUntilCleanup(f.ctx, t)

		require.Eventually(t, func() bool { return f.countStatus(alert.DeliveryFailed) == 1 }, 10*time.Second, 5*time.Millisecond)
		assert.Equal(t, "abandoned: not delivered within 24 hours", f.row(d.ID).LastError)
		assert.Empty(t, srv.received(), "a day-old notification is not sent")
	})
}

func TestDurableDelivery_AFullQueueLosesNothing(t *testing.T) {
	f := newQueueFixture(t)
	srv := newHookServer(t, always(http.StatusOK))
	ch := f.channel(t, srv.URL)
	n := f.notifier()

	const total = 256 + 20 // one alert to one channel: a single stream, whose queue holds 256 jobs
	want := map[string]int{}
	for range total {
		d := f.delivery(t, ch, alert.EventSeverityRaised, alert.StatusActive)
		want[d.ID] = 1
		n.Enqueue(job(d, ch))
	}
	n.StartUntilCleanup(f.ctx, t)

	require.Eventually(t, func() bool { return f.countStatus(alert.DeliveryDelivered) == total }, 30*time.Second, 10*time.Millisecond)
	got := map[string]int{}
	for _, r := range srv.received() {
		got[r.Delivery]++
	}
	assert.Equal(t, want, got, "every delivery is sent, once")
}

func TestDurableDelivery_TwoNotifiersSendEachAttemptOnce(t *testing.T) {
	f := newQueueFixture(t)
	srv := newHookServer(t, always(http.StatusOK))
	const total = 20
	want := map[string]int{}
	for range total {
		ch := f.channel(t, srv.URL)
		want[f.delivery(t, ch, alert.EventFired, alert.StatusActive).ID] = 1
	}

	for range 2 {
		n := f.notifier()
		n.SetResumeInterval(time.Millisecond)
		n.StartUntilCleanup(f.ctx, t)
	}

	require.Eventually(t, func() bool { return f.countStatus(alert.DeliveryDelivered) == total }, 10*time.Second, 5*time.Millisecond)
	assert.Never(t, func() bool { return len(srv.received()) > total }, 100*time.Millisecond, 5*time.Millisecond)
	got := map[string]int{}
	for _, r := range srv.received() {
		got[r.Delivery]++
	}
	assert.Equal(t, want, got, "two instances on one database never send the same attempt twice")
}

type settleFailsOnce struct {
	*store.ChannelStoreImpl
	failed atomic.Bool
}

func (s *settleFailsOnce) SettleDelivery(ctx context.Context, d *alert.NotificationDelivery, claim time.Time) (bool, error) {
	if s.failed.CompareAndSwap(false, true) {
		return false, errors.New("database is locked")
	}
	return s.ChannelStoreImpl.SettleDelivery(ctx, d, claim)
}

func TestDurableDelivery_ASentNotificationIsRecordedDespiteAStoreError(t *testing.T) {
	f := newQueueFixture(t)
	srv := newHookServer(t, always(http.StatusOK))
	ch := f.channel(t, srv.URL)
	d := f.delivery(t, ch, alert.EventFired, alert.StatusActive)
	n := alert.NewNotifier(&settleFailsOnce{ChannelStoreImpl: f.channels}, f.logger, true)
	n.SetClock(f.clock.Now)
	n.SetResumeInterval(5 * time.Millisecond)
	n.StartUntilCleanup(f.ctx, t)

	require.Eventually(t, func() bool {
		r := f.row(d.ID)
		return r != nil && r.Status == alert.DeliveryDelivered
	}, 10*time.Second, 5*time.Millisecond, "the write that follows a successful send is tried again")
	f.clock.Advance(3 * time.Minute)
	assert.Never(t, func() bool { return len(srv.received()) > 1 }, 200*time.Millisecond, 5*time.Millisecond,
		"the notification is not sent again once the claim runs out")
}

func TestDurableDelivery_DisablingAChannelCancelsWhatItHadToSend(t *testing.T) {
	f := newQueueFixture(t)
	srv := newHookServer(t, always(http.StatusOK))
	ch := f.channel(t, srv.URL)
	f.delivery(t, ch, alert.EventFired, alert.StatusActive)
	ch.Enabled = false
	require.NoError(t, f.channels.UpdateChannel(f.ctx, ch))

	f.notifier().StartUntilCleanup(f.ctx, t)

	require.Eventually(t, func() bool { return f.countStatus(alert.DeliveryPending) == 0 }, 10*time.Second, 5*time.Millisecond)
	health, err := f.channels.GetChannelHealth(f.ctx, ch.ID)
	require.NoError(t, err)
	assert.Equal(t, "healthy", health, "a channel the user turned off is not failing")
	assert.Empty(t, srv.received())
	assert.Zero(t, f.countStatus(alert.DeliveryFailed), "the cancelled notification leaves no row")
}

// genericFailure fails its first send and, like the email sender, sums every failure up in one fixed message.
type genericFailure struct{ sends atomic.Int32 }

func (s *genericFailure) Ready() error { return nil }

func (s *genericFailure) Send(context.Context, *alert.NotificationChannel, string, *alert.Alert) error {
	if s.sends.Add(1) == 1 {
		return errors.New("smtp dial: connection reset")
	}
	return nil
}

func (s *genericFailure) RetryDelay(wait time.Duration, _ error) time.Duration { return wait }

func (s *genericFailure) FailureMessage(error) string { return "delivery failed after retries" }

func (s *genericFailure) SendTest(context.Context, *alert.NotificationChannel, *alert.Alert) (int, error) {
	return http.StatusOK, nil
}

func TestDurableDelivery_LastErrorIsTheErrorOfTheLastAttempt(t *testing.T) {
	f := newQueueFixture(t)
	id, err := f.channels.InsertChannel(f.ctx, &alert.NotificationChannel{Name: "generic", Type: "generic", URL: "ops@example.com", Enabled: true})
	require.NoError(t, err)
	ch, err := f.channels.GetChannel(f.ctx, id)
	require.NoError(t, err)
	d := f.delivery(t, ch, alert.EventFired, alert.StatusActive)
	n := f.notifier()
	n.RegisterChannel("generic", &genericFailure{})
	n.StartUntilCleanup(f.ctx, t)

	require.Eventually(t, func() bool { r := f.row(d.ID); return r != nil && r.Attempts == 1 }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, "smtp dial: connection reset", f.row(d.ID).LastError, "a retry still to come keeps the cause, not a final verdict")

	f.clock.Advance(5 * time.Second)
	require.Eventually(t, func() bool { return f.countStatus(alert.DeliveryDelivered) == 1 }, 10*time.Second, 5*time.Millisecond)
	assert.Empty(t, f.row(d.ID).LastError, "a delivered notification carries no error")
}

func TestDurableDelivery_TheEventTypeOfTheRowIsTheEventSent(t *testing.T) {
	f := newQueueFixture(t)
	srv := newHookServer(t, always(http.StatusOK))
	reminder := f.delivery(t, f.channel(t, srv.URL), alert.EventReminder, alert.StatusResolved)
	recovery := f.delivery(t, f.channel(t, srv.URL), alert.EventResolved, alert.StatusActive)

	f.notifier().StartUntilCleanup(f.ctx, t)

	require.Eventually(t, func() bool { return len(srv.received()) == 2 }, 10*time.Second, 5*time.Millisecond)
	assert.ElementsMatch(t, []hookRequest{
		{Delivery: reminder.ID, Event: "alert.fired"},
		{Delivery: recovery.ID, Event: "alert.resolved"},
	}, srv.received(), "the stored event type decides, whatever the status in the payload")
}

func TestDurableDelivery_AnAttemptPastItsClaimLeavesTheRowAlone(t *testing.T) {
	f := newQueueFixture(t)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	arrived := make(chan struct{}, 8)
	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		first := requests == 1
		mu.Unlock()
		arrived <- struct{}{}
		if first {
			<-release
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	ch := f.channel(t, srv.URL)
	d := f.delivery(t, ch, alert.EventFired, alert.StatusActive)

	f.notifier().StartUntilCleanup(f.ctx, t)
	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("the first attempt never reached the destination")
	}
	f.clock.Advance(3 * time.Minute)
	f.notifier().StartUntilCleanup(f.ctx, t)
	require.Eventually(t, func() bool {
		r := f.row(d.ID)
		return r != nil && r.Status == alert.DeliveryDelivered
	}, 10*time.Second, 5*time.Millisecond, "a second worker takes the attempt once the first one's claim has run out")

	unblock()
	assert.Never(t, func() bool {
		r := f.row(d.ID)
		return r != nil && r.Status != alert.DeliveryDelivered
	}, 300*time.Millisecond, 5*time.Millisecond, "the late answer of the first attempt must not overwrite the row")
}

func TestDurableDelivery_ARecoveryWaitsForTheAlertItResolves(t *testing.T) {
	f := newQueueFixture(t)
	srv := newHookServer(t, func(n int) int {
		if n == 1 {
			return http.StatusServiceUnavailable
		}
		return http.StatusOK
	})
	ch := f.channel(t, srv.URL)
	fired := f.delivery(t, ch, alert.EventFired, alert.StatusActive)
	resolved := f.delivery(t, ch, alert.EventResolved, alert.StatusResolved)
	n := f.notifier()
	n.StartUntilCleanup(f.ctx, t)
	n.Enqueue(job(fired, ch))
	n.Enqueue(job(resolved, ch))

	require.Eventually(t, func() bool { r := f.row(fired.ID); return r != nil && r.Attempts == 1 }, 10*time.Second, 5*time.Millisecond)
	assert.Never(t, func() bool { return len(srv.received()) > 1 }, 100*time.Millisecond, 5*time.Millisecond,
		"the recovery is held while the alert it resolves waits for its retry")

	f.clock.Advance(5 * time.Second)
	require.Eventually(t, func() bool { return f.countStatus(alert.DeliveryDelivered) == 2 }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, []hookRequest{
		{Delivery: fired.ID, Event: "alert.fired"},
		{Delivery: fired.ID, Event: "alert.fired"},
		{Delivery: resolved.ID, Event: "alert.resolved"},
	}, srv.received())
}

func TestNotifier_WaitReturnsOnceEveryWorkerHasStopped(t *testing.T) {
	f := newQueueFixture(t)
	release := make(chan struct{})
	defer close(release)
	sending := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case sending <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	ch := f.channel(t, srv.URL)
	n := f.notifier()
	ctx, cancel := context.WithCancel(f.ctx)
	n.Start(ctx)
	n.Enqueue(job(f.delivery(t, ch, alert.EventFired, alert.StatusActive), ch))
	<-sending

	cancel()
	wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wcancel()
	require.NoError(t, n.Wait(wctx), "the workers and the resume loop return once the context ends, a send in flight included")

	other := f.channel(t, srv.URL)
	later := f.delivery(t, other, alert.EventFired, alert.StatusActive)
	n.Enqueue(job(later, other))
	assert.Never(t, func() bool {
		r := f.row(later.ID)
		return r == nil || !r.NextAttemptAt.Equal(later.NextAttemptAt)
	}, 100*time.Millisecond, 5*time.Millisecond, "no worker or resume pass is left to take a delivery")
}

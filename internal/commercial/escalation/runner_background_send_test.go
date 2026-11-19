// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	esc "github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/event"
)

type observedSender struct {
	Sender
	mu       sync.Mutex
	finished int
	failures []string
}

func (s *observedSender) SendEventNow(ctx context.Context, eventType string, a *alert.Alert, ch *alert.NotificationChannel) error {
	err := s.Sender.SendEventNow(ctx, eventType, a, ch)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finished++
	if err != nil {
		s.failures = append(s.failures, err.Error())
	}
	return err
}

func (s *observedSender) outcome() (int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finished, append([]string(nil), s.failures...)
}

func slowWebhook(t *testing.T, inbox *webhookInbox, delay time.Duration) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		inbox.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestAcknowledgmentNoticeOutlivesTheRequestThatAcknowledged(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sender := &observedSender{Sender: alert.NewNotifier(newChannelStoreMock(), logger, true)}
	h := newDBHarness(t, sender)
	hook, pager := &webhookInbox{}, &webhookInbox{}
	hookID := h.addChannel("hook", slowWebhook(t, hook, 200*time.Millisecond), true)
	pagerID := h.addChannel("pager", slowWebhook(t, pager, 200*time.Millisecond), true)
	a := h.addAlert()
	h.addPolicy(
		esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{hookID, pagerID}},
		esc.Level{Order: 1, DelaySeconds: 600, ChannelIDs: []string{hookID}},
	)
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))
	h.advance(61 * time.Second)
	h.cycle()
	h.settle()

	at := h.clock()
	acked, err := h.alerts.AcknowledgeAlert(h.ctx, a.ID, "alice", at)
	require.NoError(t, err)
	require.True(t, acked)
	requestCtx, endRequest := context.WithCancel(h.ctx)
	require.NoError(t, h.runner.OnAlertAcknowledged(requestCtx, a.ID, alert.Acknowledgment{By: "alice", At: at}))
	endRequest()

	require.Eventually(t, func() bool {
		finished, _ := sender.outcome()
		return finished == 4
	}, 10*time.Second, 10*time.Millisecond, "the two level sends and the two notices finish")
	_, failures := sender.outcome()
	require.Empty(t, failures, "the notice does not depend on the request that acknowledged")
	h.settle()

	for name, inbox := range map[string]*webhookInbox{"hook": hook, "pager": pager} {
		got := inbox.received()
		require.Len(t, got, 2, name)
		assert.Equal(t, event.AlertAcknowledged, got[1].Event, name)
	}
	notices := 0
	for _, d := range h.deliveries(h.onlyRun(a).ID) {
		if d.LevelIndex == specialLevelAck {
			notices++
			assert.Equal(t, esc.DeliveryStatusSent, d.Status, d.Error)
		}
	}
	assert.Equal(t, 2, notices)
}

func TestRunnerStopEndsBackgroundSendsAndKeepsTheirOutcome(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := newDBHarness(t, alert.NewNotifier(newChannelStoreMock(), logger, true))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	ch := h.addChannel("stuck", srv.URL, true)
	a := h.addAlert()
	h.addPolicy(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{ch}})

	runCtx, stop := context.WithCancel(h.ctx)
	stopped := make(chan struct{})
	go func() {
		h.runner.Run(runCtx)
		close(stopped)
	}()
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))
	h.advance(61 * time.Second)
	h.cycle()

	stop()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return once its context ended")
	}
	ds := h.deliveries(h.onlyRun(a).ID)
	require.Len(t, ds, 1)
	assert.Equal(t, esc.DeliveryStatusFailed, ds[0].Status, "the cancelled send still records its outcome")
	assert.Contains(t, ds[0].Error, "context canceled")
}

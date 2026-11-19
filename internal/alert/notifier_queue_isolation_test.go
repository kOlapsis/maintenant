// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNotifier_ADeadWebhookReceiverDoesNotHoldBackAlerts(t *testing.T) {
	release := make(chan struct{})
	mute := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(mute.Close)

	var mu sync.Mutex
	delivered := map[string]bool{}
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		delivered[string(body)] = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthy.Close)

	n := NewNotifier(&deliveryRecorder{}, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	ctx, cancel := context.WithCancel(context.Background())
	var stuck sync.WaitGroup
	t.Cleanup(func() {
		close(release)
		stuck.Wait()
		cancel()
		_ = n.Wait(context.Background())
	})
	n.Start(ctx)

	// One stuck subscription delivery per URL, so that every stream a subscription may hash to is occupied.
	const streams = 64
	stuck.Add(streams)
	for i := 0; i < streams; i++ {
		n.Enqueue(NotificationJob{
			Delivery: &NotificationDelivery{Status: DeliveryPending},
			Channel:  &NotificationChannel{Name: "sub", Type: "webhook", URL: fmt.Sprintf("%s/%d", mute.URL, i), Enabled: true},
			Body:     []byte(`{"type":"container.state_changed"}`),
			Done:     func(context.Context, error) { stuck.Done() },
		})
	}

	const alerts = 64
	for i := 0; i < alerts; i++ {
		n.Enqueue(NotificationJob{
			Delivery: &NotificationDelivery{},
			Channel:  &NotificationChannel{ID: "c1", Type: "webhook", URL: healthy.URL},
			Alert:    &Alert{ID: fmt.Sprintf("alert-%d", i), Status: StatusActive},
		})
	}

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(delivered) == alerts
	}, 3*time.Second, 10*time.Millisecond, "every alert must be delivered while the webhook receiver is silent")
}

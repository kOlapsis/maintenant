// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statusServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func webhookJob(url string) NotificationJob {
	return NotificationJob{
		Delivery: &NotificationDelivery{ID: "d1"},
		Channel:  &NotificationChannel{ID: "c1", Type: "webhook", URL: url},
		Alert:    &Alert{ID: "a1", Status: StatusActive},
	}
}

func TestProcessJob_ADeniedRequestIsNotRetried(t *testing.T) {
	quickRetries(t)
	srv, hits := statusServer(t, http.StatusBadRequest)
	rec := &deliveryRecorder{}
	n := NewNotifier(rec, slog.New(slog.NewTextHandler(io.Discard, nil)), true)

	job := webhookJob(srv.URL)
	n.processJob(context.Background(), job)

	assert.EqualValues(t, 1, hits.Load())
	assert.Equal(t, DeliveryFailed, job.Delivery.Status)
	assert.Equal(t, 1, job.Delivery.Attempts)
	assert.Equal(t, "non-2xx response: 400", job.Delivery.LastError)
}

func TestProcessJob_RetriesWhatMayWorkLater(t *testing.T) {
	quickRetries(t)
	for _, status := range []int{http.StatusTooManyRequests, http.StatusRequestTimeout, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, hits := statusServer(t, status)
			n := NewNotifier(&deliveryRecorder{}, slog.New(slog.NewTextHandler(io.Discard, nil)), true)

			job := webhookJob(srv.URL)
			n.processJob(context.Background(), job)

			assert.EqualValues(t, maxRetries, hits.Load())
			assert.Equal(t, DeliveryFailed, job.Delivery.Status)
			assert.Equal(t, maxRetries, job.Delivery.Attempts)
		})
	}
}

func TestSendNow_ADeniedRequestIsNotRetried(t *testing.T) {
	quickRetries(t)
	srv, hits := statusServer(t, http.StatusForbidden)
	n := NewNotifier(&deliveryRecorder{}, slog.New(slog.NewTextHandler(io.Discard, nil)), true)

	job := webhookJob(srv.URL)
	require.Error(t, n.SendNow(context.Background(), job.Alert, job.Channel))
	assert.EqualValues(t, 1, hits.Load())
}

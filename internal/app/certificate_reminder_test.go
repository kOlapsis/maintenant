// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func certExpiring(threshold, days int) map[string]any {
	return map[string]any{
		"monitor_id": "cert-1", "hostname": "example.com", "port": 443,
		"alert_type":     certificate.AlertTypeExpiring,
		"severity":       certificate.ExpiringSeverity(days, "Example Corp CA"),
		"threshold_days": threshold,
		"days_remaining": days,
	}
}

func TestCertificateAlertEvent_ThresholdIsAReminderWithTheDaysLeft(t *testing.T) {
	evt := certificateAlertEvent(certExpiring(3, 3))
	assert.True(t, evt.Renotify, "a crossed threshold is a reminder")
	assert.Equal(t, "Certificate for example.com:443 expires in 3 days", evt.Message)
	assert.Equal(t, "Certificate for example.com:443 expires in 1 day", certificateAlertEvent(certExpiring(1, 1)).Message)

	chain := certificateAlertEvent(map[string]any{
		"monitor_id": "cert-1", "hostname": "example.com", "port": 443,
		"alert_type": certificate.AlertTypeChainInvalid, "severity": alert.SeverityCritical,
	})
	assert.False(t, chain.Renotify, "a condition raised on every scan is not a reminder")
}

func TestCertificateReminder_SameSeverityThresholdIsNotifiedThenResolved(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	alerts, channels, triggers := store.NewAlertStore(db), store.NewChannelStore(db), store.NewTriggerStore(db)

	chID, err := channels.InsertChannel(ctx, &alert.NotificationChannel{Name: "hook", Type: "webhook", URL: "https://example.com/hook", Enabled: true})
	require.NoError(t, err)
	_, err = triggers.InsertTrigger(ctx, &alert.AlertTrigger{Name: "all", Enabled: true, NotifyOnResolve: true, ChannelIDs: []string{chID}})
	require.NoError(t, err)

	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alerts,
		ChannelStore: channels,
		TriggerStore: triggers,
		SilenceStore: store.NewSilenceStore(db),
		// Never started: the delivery row is the notification, and no send outlives the test.
		Notifier: alert.NewNotifier(channels, logger, true),
		Logger:   logger,
	})
	engine.Start(ctx)
	emit := func(evt alert.Event) {
		evt.Timestamp = time.Now()
		engine.EventChannel() <- evt
	}
	deliveries := func(id string) int {
		ds, err := channels.ListDeliveriesByAlert(ctx, id)
		require.NoError(t, err)
		return len(ds)
	}

	emit(certificateAlertEvent(certExpiring(7, 7)))
	var id string
	require.Eventually(t, func() bool {
		a, err := alerts.GetActiveAlert(ctx, alert.SourceCertificate, certificate.AlertTypeExpiring, "certificate", "cert-1")
		if err != nil || a == nil {
			return false
		}
		id = a.ID
		return true
	}, 10*time.Second, 5*time.Millisecond)

	emit(certificateAlertEvent(certExpiring(3, 3)))
	require.Eventually(t, func() bool { return deliveries(id) == 2 }, 10*time.Second, 5*time.Millisecond,
		"the 3-day reminder has the severity of the 7-day one and must still be notified")
	got, err := alerts.GetAlert(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Certificate for example.com:443 expires in 3 days", got.Message)

	emit(certificateRecoveryEvent(map[string]any{
		"monitor_id": "cert-1", "hostname": "example.com", "previous_alert_type": certificate.AlertTypeExpiring,
	}))
	require.Eventually(t, func() bool {
		a, err := alerts.GetAlert(ctx, id)
		return err == nil && a.Status == alert.StatusResolved && deliveries(id) == 3
	}, 10*time.Second, 5*time.Millisecond, "the renewal must resolve the reminded alert")
}

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
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// tierNotifier registers the production senders, Telegram and SMTP pointed at local stubs.
func tierNotifier(t *testing.T, store alert.ChannelStore, logs *syncBuffer) (*alert.Notifier, map[string]*alert.NotificationChannel) {
	t.Helper()
	host, port, _, _ := smtpStub(t)
	tg, _, _ := telegramStub(t, http.StatusOK, `{"ok":true}`)
	hook, _, _ := captureServer(t, http.StatusOK)

	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	n := alert.NewNotifier(store, logger, true)
	for chType, s := range NewChannels(extpoint.ChannelDeps{
		HTTPClient: n.HTTPClient(),
		SMTP:       extpoint.SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"},
		Logger:     logger,
	}) {
		n.RegisterChannel(chType, s)
	}
	n.RegisterChannel("telegram", NewTelegramSender(tg.Client(), tg.URL))

	channels := map[string]*alert.NotificationChannel{}
	for _, chType := range []string{"webhook", "discord", "slack", "teams", "telegram", "email"} {
		ch := &alert.NotificationChannel{ID: "ch-" + chType, Name: chType, Type: chType, URL: hook.URL, Enabled: true}
		switch chType {
		case "telegram":
			ch.URL, ch.Secret = "-1001234567890", sentinelToken
		case "email":
			ch.URL = "ops@example.com"
		}
		channels[chType] = ch
	}
	return n, channels
}

var deliveredPerEdition = map[extension.Edition]map[string]bool{
	extension.Community: {"webhook": true, "discord": true, "slack": false, "teams": false, "telegram": false, "email": false},
	extension.Personal:  {"webhook": true, "discord": true, "slack": false, "teams": false, "telegram": true, "email": true},
	extension.Pro:       {"webhook": true, "discord": true, "slack": true, "teams": true, "telegram": true, "email": true},
}

func TestSuspension_SendNowPerEdition(t *testing.T) {
	for edition, perType := range deliveredPerEdition {
		for chType, delivered := range perType {
			t.Run(string(edition)+"/"+chType, func(t *testing.T) {
				withEdition(t, edition)
				n, channels := tierNotifier(t, &deliveryRecorder{}, &syncBuffer{})

				err := n.SendNow(context.Background(), firedAlert(), channels[chType])
				_, testErr := n.SendTestWebhook(context.Background(), channels[chType])
				if delivered {
					assert.NoError(t, err)
					assert.NoError(t, testErr)
					return
				}
				var suspended *alert.SuspendedError
				require.True(t, errors.As(err, &suspended), "got %v", err)
				assert.Equal(t, "suspended: requires the "+requiredTitle(chType)+" edition", err.Error())
				require.True(t, errors.As(testErr, &suspended))
			})
		}
	}
}

func TestSuspension_EnqueueRecordsASuspendedDelivery(t *testing.T) {
	for edition, perType := range deliveredPerEdition {
		for chType, delivered := range perType {
			t.Run(string(edition)+"/"+chType, func(t *testing.T) {
				withEdition(t, edition)
				rec := &deliveryRecorder{}
				n, channels := tierNotifier(t, rec, &syncBuffer{})
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				n.Start(ctx)

				n.Enqueue(alert.NotificationJob{
					Delivery: &alert.NotificationDelivery{ID: "d1", AlertID: "a1", ChannelID: channels[chType].ID},
					Channel:  channels[chType],
					Alert:    firedAlert(),
				})
				require.Eventually(t, func() bool { _, ok := rec.last(); return ok }, 5*time.Second, 5*time.Millisecond)
				cancel()

				d, _ := rec.last()
				if delivered {
					assert.Equal(t, alert.DeliveryDelivered, d.Status)
					return
				}
				assert.Equal(t, alert.DeliverySuspended, d.Status)
				assert.Zero(t, d.Attempts, "a suspended channel is never attempted")
				assert.Equal(t, "suspended: requires the "+requiredTitle(chType)+" edition", d.LastError)
			})
		}
	}
}

func TestSuspension_WarnsOncePerTransition(t *testing.T) {
	logs := &syncBuffer{}
	n, channels := tierNotifier(t, &deliveryRecorder{}, logs)
	count := func() int { return strings.Count(logs.String(), "channel suspended") }

	withEdition(t, extension.Community)
	for range 3 {
		_ = n.SendNow(context.Background(), firedAlert(), channels["slack"])
	}
	assert.Equal(t, 1, count())

	withEdition(t, extension.Pro)
	require.NoError(t, n.SendNow(context.Background(), firedAlert(), channels["slack"]))

	withEdition(t, extension.Community)
	_ = n.SendNow(context.Background(), firedAlert(), channels["slack"])
	assert.Equal(t, 2, count())
}

func requiredTitle(chType string) string {
	if chType == "slack" || chType == "teams" {
		return "Pro"
	}
	return "Personal"
}

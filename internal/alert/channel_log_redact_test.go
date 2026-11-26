// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookSender_DebugLogsCarryNoURL(t *testing.T) {
	srv, _, _ := captureServer(t, 200)
	for _, p := range secretPaths {
		t.Run(p, func(t *testing.T) {
			logs := &lockedBuffer{}
			logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			n := NewNotifier(nil, logger, true)

			require.NoError(t, n.SendNow(context.Background(), &Alert{ID: "a1"}, &NotificationChannel{ID: "c1", Type: "webhook", URL: srv.URL + p}))

			assert.Contains(t, logs.String(), "sending webhook")
			assert.NotContains(t, logs.String(), p)
			assert.NotContains(t, logs.String(), "http://")
		})
	}
}

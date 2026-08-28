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

package v1

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FR-001d — the way out. An operator whose licence expired must still be able
// to silence or remove a gated channel. Refusing the whole update, as the
// handler used to, left deleting the configuration as the only way to stop
// being notified: a trapdoor, not a door.
//
// The table runs for telegram and for slack, because the fix belongs to the
// shared control, not to Telegram.
func TestGatedChannel_ExitDoorStaysOpen(t *testing.T) {
	for _, channelType := range []string{"telegram", "slack", "email"} {
		t.Run(channelType, func(t *testing.T) {
			cases := []struct {
				name       string
				body       string
				wantStatus int
			}{
				{"disable only", `{"enabled":false}`, http.StatusOK},
				{"enable", `{"enabled":true}`, http.StatusForbidden},
				{"disable plus a change", `{"enabled":false,"name":"renamed"}`, http.StatusForbidden},
				{"rename", `{"name":"renamed"}`, http.StatusForbidden},
				{"new destination", `{"url":"-1009999999999"}`, http.StatusForbidden},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					withEditionPinned(t, extension.Community)
					logger := slog.New(slog.NewTextHandler(io.Discard, nil))
					store := &stubChannelStore{ch: &alert.NotificationChannel{
						ID: "1", Name: "oncall", Type: channelType,
						URL: "-1001234567890", Secret: sentinelToken, Enabled: true,
					}}
					h := &AlertHandler{channelStore: store, broker: NewSSEBroker(logger)}

					req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(tc.body))
					req.Header.Set("Content-Type", "application/json")
					req.SetPathValue("id", "1")
					rec := httptest.NewRecorder()

					h.HandleUpdateChannel(rec, req)

					require.Equal(t, tc.wantStatus, rec.Code, "body: %s", rec.Body.String())
					if tc.wantStatus == http.StatusForbidden {
						assert.Contains(t, rec.Body.String(), "EDITION_REQUIRED")
						assert.True(t, store.ch.Enabled, "a refused update must change nothing")
					}
				})
			}
		})
	}
}

// The other way out. Deletion has never been gated; this is what keeps it that
// way, on the type a future change would be most tempted to gate.
func TestGatedChannel_DeleteIsNeverGated(t *testing.T) {
	for _, channelType := range []string{"telegram", "slack", "email"} {
		t.Run(channelType, func(t *testing.T) {
			withEditionPinned(t, extension.Community)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			store := &stubChannelStore{ch: &alert.NotificationChannel{
				ID: "1", Name: "oncall", Type: channelType, URL: "-100123", Enabled: true,
			}}
			h := &AlertHandler{channelStore: store, broker: NewSSEBroker(logger)}

			req := httptest.NewRequest("DELETE", "/api/v1/channels/1", nil)
			req.SetPathValue("id", "1")
			rec := httptest.NewRecorder()

			h.HandleDeleteChannel(rec, req)

			assert.Equal(t, http.StatusNoContent, rec.Code)
		})
	}
}

// A plain channel cannot be turned into a gated one from an edition that does
// not open the target type, and a gated one cannot be edited by claiming it is
// something else.
func TestGatedChannel_TypeChangeIsCheckedBothWays(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("webhook to telegram is refused", func(t *testing.T) {
		withEditionPinned(t, extension.Community)
		store := &stubChannelStore{ch: &alert.NotificationChannel{ID: "1", Type: "webhook", URL: "https://example.com"}}
		h := &AlertHandler{channelStore: store, broker: NewSSEBroker(logger)}

		req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(`{"type":"telegram"}`))
		req.SetPathValue("id", "1")
		rec := httptest.NewRecorder()
		h.HandleUpdateChannel(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "telegram")
	})

	t.Run("telegram to webhook is refused too", func(t *testing.T) {
		withEditionPinned(t, extension.Community)
		store := &stubChannelStore{ch: &alert.NotificationChannel{ID: "1", Type: "telegram", URL: "-100123"}}
		h := &AlertHandler{channelStore: store, broker: NewSSEBroker(logger)}

		req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(`{"type":"webhook","url":"https://example.com"}`))
		req.SetPathValue("id", "1")
		rec := httptest.NewRecorder()
		h.HandleUpdateChannel(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code,
			"editing a gated channel is gated, whatever it claims to become")
	})
}

// FR-001a, the last surface: the test button is an outgoing call, so it is
// gated like creation.
func TestGatedChannel_TestButtonIsGated(t *testing.T) {
	withEditionPinned(t, extension.Community)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// A nil notifier is the proof: a refused test must not reach it.
	store := &stubChannelStore{ch: &alert.NotificationChannel{
		ID: "1", Type: "telegram", URL: "-100123", Secret: sentinelToken,
	}}
	h := &AlertHandler{channelStore: store, broker: NewSSEBroker(logger)}

	req := httptest.NewRequest("POST", "/api/v1/channels/1/test", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.HandleTestChannel(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "EDITION_REQUIRED")
}

// FR-001b: a licence that expires closes the management of a channel, never its
// delivery. Asserted here rather than in the alert package, which cannot import
// the edition registry — and this is exactly what fails the day someone adds a
// capability check to the send path.
func TestGatedChannel_DeliveryContinuesUnderCommunity(t *testing.T) {
	withEditionPinned(t, extension.Community)

	var sent int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent++
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	notifier := alert.NewNotifier(nil, logger, true)
	notifier.SetTelegramTransport(srv.URL, srv.Client())

	ch := &alert.NotificationChannel{
		ID: "1", Name: "oncall", Type: "telegram",
		URL: "-1001234567890", Secret: sentinelToken, Enabled: true,
	}
	err := notifier.SendNow(t.Context(), &alert.Alert{
		ID: "a1", Source: "endpoint", Severity: "critical", Status: "active",
		Message: "Connection refused", EntityName: "api.example.com",
	}, ch)

	require.NoError(t, err)
	assert.Equal(t, 1, sent, "a downgraded instance keeps notifying through channels it already has")
}

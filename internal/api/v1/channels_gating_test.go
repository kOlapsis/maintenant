// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

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
					h := &AlertHandler{notifier: channelNotifier(), channelStore: store, broker: NewSSEBroker(logger)}

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
			h := &AlertHandler{notifier: channelNotifier(), channelStore: store, broker: NewSSEBroker(logger)}

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
		h := &AlertHandler{notifier: channelNotifier(), channelStore: store, broker: NewSSEBroker(logger)}

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
		h := &AlertHandler{notifier: channelNotifier(), channelStore: store, broker: NewSSEBroker(logger)}

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
	h := &AlertHandler{notifier: channelNotifier(), channelStore: store, broker: NewSSEBroker(logger)}

	req := httptest.NewRequest("POST", "/api/v1/channels/1/test", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.HandleTestChannel(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "EDITION_REQUIRED")
}

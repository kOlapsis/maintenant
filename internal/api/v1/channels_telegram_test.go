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
	"bytes"
	"encoding/json"
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

// The token used throughout. If it ever shows up in a response or a log line,
// the test that looks for it fails (FR-005, SC-003).
const sentinelToken = "8123456789:SENTINEL-DO-NOT-LEAK-xxxxxxxxxxxxx"

func withEditionPinned(t *testing.T, e extension.Edition) {
	t.Helper()
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = original })
}

func telegramBody(name string) string {
	return `{"name":"` + name + `","type":"telegram","url":"-1001234567890","secret":"` +
		sentinelToken + `","config":{"thread_id":"42"},"enabled":true}`
}

// FR-001: Telegram opens at Personal. A Community instance is refused, and the
// refusal names Personal rather than pushing every blocked channel to the top
// tier.
func TestTelegramChannel_CreateGating(t *testing.T) {
	cases := []struct {
		edition    extension.Edition
		wantStatus int
	}{
		{extension.Community, http.StatusForbidden},
		{extension.Personal, http.StatusCreated},
		{extension.Pro, http.StatusCreated},
	}

	for _, tc := range cases {
		t.Run(string(tc.edition), func(t *testing.T) {
			withEditionPinned(t, tc.edition)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			h := &AlertHandler{channelStore: &stubChannelStore{}, broker: NewSSEBroker(logger)}

			req := httptest.NewRequest("POST", "/api/v1/channels", strings.NewReader(telegramBody("oncall")))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.HandleCreateChannel(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code, "body: %s", rec.Body.String())
			if tc.wantStatus != http.StatusForbidden {
				return
			}
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "EDITION_REQUIRED", body.Error.Code)
			assert.Equal(t, "telegram", body.Error.Feature)
			assert.Equal(t, "personal", body.Error.RequiredEdition)
		})
	}
}

// SC-003: the token never comes back. Not on create, not on read, not in a log.
func TestTelegramChannel_TokenNeverLeaves(t *testing.T) {
	withEditionPinned(t, extension.Personal)

	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store := &stubChannelStore{}
	h := &AlertHandler{channelStore: store, broker: NewSSEBroker(logger)}

	req := httptest.NewRequest("POST", "/api/v1/channels", strings.NewReader(telegramBody("oncall")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateChannel(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	assert.NotContains(t, rec.Body.String(), sentinelToken, "the created channel carries the token back")
	assert.NotContains(t, rec.Body.String(), `"secret"`, "no secret key at all, empty or not")
	assert.Contains(t, rec.Body.String(), `"has_secret":true`, "the interface still learns a token is on file")
	assert.Contains(t, rec.Body.String(), `"config":"{\"thread_id\":\"42\"}"`)

	// And on the read paths.
	store.ch = &alert.NotificationChannel{
		ID: "1", Name: "oncall", Type: "telegram", URL: "-1001234567890",
		Secret: sentinelToken, Config: `{"thread_id":"42"}`, HasSecret: true, Enabled: true,
	}
	readRec := httptest.NewRecorder()
	readReq := httptest.NewRequest("GET", "/api/v1/channels", nil)
	h.HandleListChannels(readRec, readReq)
	assert.NotContains(t, readRec.Body.String(), sentinelToken)

	assert.NotContains(t, logs.String(), sentinelToken, "no log line may carry the token")
}

// FR-004: an obviously wrong token or chat id is refused on shape, with no call
// to Telegram — the handler has no notifier here, so any call would panic.
func TestTelegramChannel_RefusesMalformedInputWithoutCallingTelegram(t *testing.T) {
	withEditionPinned(t, extension.Personal)

	cases := map[string]string{
		"token without a colon": `{"name":"a","type":"telegram","url":"-100123","secret":"8123456789AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}`,
		"token missing":         `{"name":"a","type":"telegram","url":"-100123"}`,
		"chat id is a name":     `{"name":"a","type":"telegram","url":"my-group","secret":"` + sentinelToken + `"}`,
		"thread id not a number": `{"name":"a","type":"telegram","url":"-100123","secret":"` + sentinelToken +
			`","config":{"thread_id":"general"}}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			h := &AlertHandler{channelStore: &stubChannelStore{}, broker: NewSSEBroker(logger)}

			req := httptest.NewRequest("POST", "/api/v1/channels", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.HandleCreateChannel(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

// FR-006: an update that says nothing about the token keeps it. Clearing it is
// refused: a channel without its credential can no longer send, and would say
// nothing about it.
func TestTelegramChannel_UpdateKeepsTheStoredToken(t *testing.T) {
	withEditionPinned(t, extension.Personal)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	stored := func() *alert.NotificationChannel {
		return &alert.NotificationChannel{
			ID: "1", Name: "oncall", Type: "telegram", URL: "-1001234567890",
			Secret: sentinelToken, Enabled: true,
		}
	}

	t.Run("absent secret is kept", func(t *testing.T) {
		store := &stubChannelStore{ch: stored()}
		h := &AlertHandler{channelStore: store, broker: NewSSEBroker(logger)}

		req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(`{"name":"renamed"}`))
		req.SetPathValue("id", "1")
		rec := httptest.NewRecorder()
		h.HandleUpdateChannel(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Equal(t, sentinelToken, store.ch.Secret, "the stored token must survive a rename")
		assert.NotContains(t, rec.Body.String(), sentinelToken)
		assert.Contains(t, rec.Body.String(), `"has_secret":true`)
	})

	t.Run("empty secret is refused", func(t *testing.T) {
		store := &stubChannelStore{ch: stored()}
		h := &AlertHandler{channelStore: store, broker: NewSSEBroker(logger)}

		req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(`{"secret":""}`))
		req.SetPathValue("id", "1")
		rec := httptest.NewRecorder()
		h.HandleUpdateChannel(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, sentinelToken, store.ch.Secret)
	})

	t.Run("new secret replaces the old one", func(t *testing.T) {
		store := &stubChannelStore{ch: stored()}
		h := &AlertHandler{channelStore: store, broker: NewSSEBroker(logger)}

		fresh := "9876543210:AAFyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy"
		req := httptest.NewRequest("PUT", "/api/v1/channels/1", strings.NewReader(`{"secret":"`+fresh+`"}`))
		req.SetPathValue("id", "1")
		rec := httptest.NewRecorder()
		h.HandleUpdateChannel(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Equal(t, fresh, store.ch.Secret)
	})
}

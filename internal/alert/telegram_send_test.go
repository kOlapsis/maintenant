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

package alert

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sentinelToken = "8123456789:SENTINEL-DO-NOT-LEAK-xxxxxxxxxxxxx"

// telegramStub answers like the Bot API and records what it was sent. The
// client is bare on purpose: the notifier's own client carries the SSRF guard,
// which refuses 127.0.0.1, where httptest listens.
func telegramStub(t *testing.T, status int, body string) (*httptest.Server, *[]*http.Request, *[]string) {
	t.Helper()
	var requests []*http.Request
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		requests = append(requests, r)
		bodies = append(bodies, string(raw))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests, &bodies
}

func telegramChannel() *NotificationChannel {
	return &NotificationChannel{
		ID: "c1", Name: "oncall", Type: "telegram",
		URL: "-1001234567890", Secret: sentinelToken, Enabled: true,
	}
}

func TestSendTelegram_PostsTheExpectedCall(t *testing.T) {
	srv, requests, bodies := telegramStub(t, http.StatusOK, `{"ok":true}`)

	err := SendTelegram(context.Background(), srv.Client(), srv.URL, telegramChannel(), "hello")
	require.NoError(t, err)

	require.Len(t, *requests, 1)
	assert.Equal(t, http.MethodPost, (*requests)[0].Method)
	assert.Equal(t, "/bot"+sentinelToken+"/sendMessage", (*requests)[0].URL.Path)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte((*bodies)[0]), &payload))
	assert.Equal(t, "-1001234567890", payload["chat_id"])
	assert.Equal(t, "HTML", payload["parse_mode"])
	assert.Equal(t, "hello", payload["text"])
	assert.NotContains(t, payload, "message_thread_id",
		"a channel without a topic must not pin the message to one")
}

func TestSendTelegram_CarriesTheTopicWhenTheChannelHasOne(t *testing.T) {
	srv, _, bodies := telegramStub(t, http.StatusOK, `{"ok":true}`)

	ch := telegramChannel()
	ch.Config = `{"thread_id":"42"}`
	require.NoError(t, SendTelegram(context.Background(), srv.Client(), srv.URL, ch, "hello"))

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte((*bodies)[0]), &payload))
	assert.Equal(t, float64(42), payload["message_thread_id"],
		"the topic id is a number for Telegram, not the string we store")
}

// FR-008 and SC-004: the operator reads the phrase that names the fix, not a
// bare status code. And never the token (FR-005).
func TestSendTelegram_SurfacesTelegramsOwnReason(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"revoked token", http.StatusUnauthorized, `{"ok":false,"description":"Unauthorized"}`, "HTTP 401: Unauthorized"},
		{"unknown chat", http.StatusBadRequest, `{"ok":false,"description":"chat not found"}`, "HTTP 400: chat not found"},
		{"kicked", http.StatusForbidden, `{"ok":false,"description":"bot was kicked from the supergroup chat"}`, "HTTP 403: bot was kicked from the supergroup chat"},
		{"no thread", http.StatusBadRequest, `{"ok":false,"description":"message thread not found"}`, "HTTP 400: message thread not found"},
		{"silent refusal", http.StatusBadRequest, `{"ok":false}`, "HTTP 400: no reason given"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _, _ := telegramStub(t, tc.status, tc.body)

			err := SendTelegram(context.Background(), srv.Client(), srv.URL, telegramChannel(), "hello")

			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
			assert.NotContains(t, err.Error(), sentinelToken, "the error must not carry the token")
			assert.NotContains(t, err.Error(), srv.URL, "the error must not carry the called URL")
		})
	}
}

// A 200 with ok:false is still a failure. Telegram uses it, and reading only
// the status code would report a delivery that never happened.
func TestSendTelegram_TreatsOkFalseAsFailure(t *testing.T) {
	srv, _, _ := telegramStub(t, http.StatusOK, `{"ok":false,"description":"chat not found"}`)

	err := SendTelegram(context.Background(), srv.Client(), srv.URL, telegramChannel(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat not found")
}

// FR-014: the delay Telegram asks for is carried by a typed error, so the retry
// loop honours it without reading a message.
func TestSendTelegram_RateLimitCarriesTheDelay(t *testing.T) {
	srv, _, _ := telegramStub(t, http.StatusTooManyRequests,
		`{"ok":false,"description":"Too Many Requests: retry later","parameters":{"retry_after":30}}`)

	err := SendTelegram(context.Background(), srv.Client(), srv.URL, telegramChannel(), "hello")

	var rateLimit *TelegramRateLimitError
	require.ErrorAs(t, err, &rateLimit)
	assert.Equal(t, 30*time.Second, rateLimit.RetryAfter)
	assert.Contains(t, err.Error(), "Too Many Requests")
	assert.NotContains(t, err.Error(), sentinelToken)
}

// FR-013 and FR-014 together: the product's backoff is the floor, Telegram's
// delay raises it, and neither replaces the other.
func TestTelegramBackoff(t *testing.T) {
	assert.Equal(t, retryBackoffs[0], telegramBackoff(1, nil))
	assert.Equal(t, retryBackoffs[1], telegramBackoff(2, nil))

	short := &TelegramRateLimitError{RetryAfter: 100 * time.Millisecond}
	assert.Equal(t, retryBackoffs[0], telegramBackoff(1, short),
		"a delay shorter than our own backoff must not shorten it")

	long := &TelegramRateLimitError{RetryAfter: 30 * time.Second}
	assert.Equal(t, 30*time.Second, telegramBackoff(1, long))
}

// A transport failure must not leak the URL: the error the http client returns
// carries it, and the URL carries the token.
func TestSendTelegram_TransportFailureHidesTheURL(t *testing.T) {
	srv, _, _ := telegramStub(t, http.StatusOK, `{"ok":true}`)
	srv.Close() // nothing is listening any more

	err := SendTelegram(context.Background(), srv.Client(), srv.URL, telegramChannel(), "hello")

	require.Error(t, err)
	assert.NotContains(t, err.Error(), sentinelToken)
	assert.False(t, strings.Contains(err.Error(), srv.URL))
}

// Telegram answers "chat not found" whether the @name belongs to nothing, to a
// private conversation, or to the bot itself. The operator reads the token as
// the suspect and looks in the wrong place, so the destination is named.
func TestSendTelegram_ExplainsAnUnresolvedUsername(t *testing.T) {
	srv, _, _ := telegramStub(t, http.StatusBadRequest, `{"ok":false,"description":"chat not found"}`)

	ch := telegramChannel()
	ch.URL = "@my_bot"
	err := SendTelegram(context.Background(), srv.Client(), srv.URL, ch, "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat not found")
	assert.Contains(t, err.Error(), "public channel")
	assert.Contains(t, err.Error(), "numeric chat id")
}

// The hint is for the @name case only: on a numeric id it would send the
// operator chasing a mistake they did not make.
func TestSendTelegram_LeavesANumericChatIDReasonAlone(t *testing.T) {
	srv, _, _ := telegramStub(t, http.StatusBadRequest, `{"ok":false,"description":"chat not found"}`)

	err := SendTelegram(context.Background(), srv.Client(), srv.URL, telegramChannel(), "hello")
	require.Error(t, err)
	assert.Equal(t, "HTTP 400: chat not found", err.Error())
}

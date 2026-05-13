// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license.

package alert

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestNotifier() *Notifier {
	return NewNotifier(nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}

func captureServer(t *testing.T, statusCode int) (*httptest.Server, *[]byte, *string) {
	t.Helper()
	var body []byte
	var ct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct = r.Header.Get("Content-Type")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(statusCode)
	}))
	t.Cleanup(srv.Close)
	return srv, &body, &ct
}

func TestSendTestWebhook_Discord(t *testing.T) {
	srv, body, ct := captureServer(t, http.StatusNoContent)

	n := newTestNotifier()
	ch := &NotificationChannel{Type: "discord", URL: srv.URL}

	code, err := n.SendTestWebhook(context.Background(), ch)

	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, code)
	assert.Equal(t, "application/json", *ct)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(*body, &payload))

	embeds, ok := payload["embeds"].([]interface{})
	require.True(t, ok, "payload must have 'embeds' array")
	require.Len(t, embeds, 1)

	embed := embeds[0].(map[string]interface{})
	assert.NotEmpty(t, embed["title"])
	assert.NotEmpty(t, embed["description"])

	// Discord color must be integer 0-16777215
	color, ok := embed["color"].(float64) // JSON numbers unmarshal as float64
	require.True(t, ok, "color must be a JSON number")
	assert.GreaterOrEqual(t, color, float64(0))
	assert.LessOrEqual(t, color, float64(16777215))

	// color must be serialized as integer (no decimal point) in raw JSON
	colorJSON, _ := json.Marshal(int(color))
	assert.Contains(t, string(*body), string(colorJSON), "color must be a JSON integer, not a float")

	// Fields must have non-empty name and value
	fields, ok := embed["fields"].([]interface{})
	require.True(t, ok)
	for i, f := range fields {
		field := f.(map[string]interface{})
		assert.NotEmpty(t, field["name"], "field[%d].name must not be empty", i)
		assert.NotEmpty(t, field["value"], "field[%d].value must not be empty", i)
	}

	t.Logf("Discord payload:\n%s", mustPretty(*body))
}

func TestSendTestWebhook_Slack(t *testing.T) {
	srv, body, ct := captureServer(t, http.StatusOK)

	n := newTestNotifier()
	ch := &NotificationChannel{Type: "slack", URL: srv.URL}

	code, err := n.SendTestWebhook(context.Background(), ch)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "application/json", *ct)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(*body, &payload))

	blocks, ok := payload["blocks"].([]interface{})
	require.True(t, ok, "Slack payload must have 'blocks' array")
	require.NotEmpty(t, blocks)

	for i, b := range blocks {
		block := b.(map[string]interface{})
		assert.Equal(t, "section", block["type"], "block[%d].type", i)
		text, ok := block["text"].(map[string]interface{})
		require.True(t, ok, "block[%d].text must be an object", i)
		assert.Equal(t, "mrkdwn", text["type"])
		assert.NotEmpty(t, text["text"])
	}

	t.Logf("Slack payload:\n%s", mustPretty(*body))
}

func TestSendTestWebhook_Teams(t *testing.T) {
	srv, body, ct := captureServer(t, http.StatusOK)

	n := newTestNotifier()
	ch := &NotificationChannel{Type: "teams", URL: srv.URL}

	code, err := n.SendTestWebhook(context.Background(), ch)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "application/json", *ct)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(*body, &payload))

	assert.Equal(t, "MessageCard", payload["@type"])
	assert.NotEmpty(t, payload["@context"])
	assert.NotEmpty(t, payload["title"])
	assert.NotEmpty(t, payload["themeColor"])

	sections, ok := payload["sections"].([]interface{})
	require.True(t, ok, "Teams payload must have 'sections' array")
	require.NotEmpty(t, sections)

	section := sections[0].(map[string]interface{})
	facts, ok := section["facts"].([]interface{})
	require.True(t, ok)
	for i, f := range facts {
		fact := f.(map[string]interface{})
		assert.NotEmpty(t, fact["name"], "fact[%d].name", i)
		assert.NotEmpty(t, fact["value"], "fact[%d].value", i)
	}

	t.Logf("Teams payload:\n%s", mustPretty(*body))
}

func TestSendTestWebhook_Non2xx_ReturnsError(t *testing.T) {
	srv, _, _ := captureServer(t, http.StatusBadRequest)

	n := newTestNotifier()
	ch := &NotificationChannel{Type: "discord", URL: srv.URL}

	code, err := n.SendTestWebhook(context.Background(), ch)

	assert.Equal(t, http.StatusBadRequest, code)
	require.Error(t, err, "non-2xx must return an error")
	assert.Contains(t, err.Error(), "400")
}

func TestSendTestWebhook_Generic(t *testing.T) {
	srv, body, _ := captureServer(t, http.StatusOK)

	n := newTestNotifier()
	ch := &NotificationChannel{Type: "webhook", URL: srv.URL}

	_, err := n.SendTestWebhook(context.Background(), ch)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(*body, &payload))
	assert.NotEmpty(t, payload["event"])
	assert.NotEmpty(t, payload["timestamp"])

	t.Logf("Generic webhook payload:\n%s", mustPretty(*body))
}

func mustPretty(b []byte) string {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}

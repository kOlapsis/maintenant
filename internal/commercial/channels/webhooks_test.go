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
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

func newTestNotifier(smtp extpoint.SMTPConfig) *alert.Notifier {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	n := alert.NewNotifier(nil, logger, true)
	for chType, s := range NewChannels(extpoint.ChannelDeps{HTTPClient: n.HTTPClient(), SMTP: smtp, Logger: logger}) {
		n.RegisterChannel(chType, s)
	}
	return n
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

func TestNewChannels_RegistersEveryPaidType(t *testing.T) {
	got := NewChannels(extpoint.ChannelDeps{HTTPClient: http.DefaultClient, Logger: slog.Default()})
	assert.Len(t, got, 4)
	assert.IsType(t, &emailSender{}, got["email"])
	assert.IsType(t, &telegramSender{}, got["telegram"])
	assert.NotNil(t, got["slack"])
	assert.NotNil(t, got["teams"])
}

func TestSendTestWebhook_Slack(t *testing.T) {
	srv, body, ct := captureServer(t, http.StatusOK)

	n := newTestNotifier(extpoint.SMTPConfig{})
	ch := &alert.NotificationChannel{Type: "slack", URL: srv.URL}

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

}

func TestSendTestWebhook_Teams(t *testing.T) {
	srv, body, ct := captureServer(t, http.StatusOK)

	n := newTestNotifier(extpoint.SMTPConfig{})
	ch := &alert.NotificationChannel{Type: "teams", URL: srv.URL}

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

}

// smtpStub speaks just enough SMTP to accept one message and keep its data.
func smtpStub(t *testing.T) (host, port string, data *strings.Builder, rcpt *[]string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	data = &strings.Builder{}
	rcpt = &[]string{}
	var mu sync.Mutex
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				r := bufio.NewReader(c)
				w := func(s string) { _, _ = io.WriteString(c, s+"\r\n") }
				w("220 stub")
				inData := false
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimRight(line, "\r\n")
					if inData {
						if line == "." {
							inData = false
							w("250 ok")
							continue
						}
						mu.Lock()
						data.WriteString(line + "\n")
						mu.Unlock()
						continue
					}
					cmd := strings.ToUpper(line)
					switch {
					case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
						w("250 stub")
					case strings.HasPrefix(cmd, "RCPT TO:"):
						mu.Lock()
						*rcpt = append(*rcpt, line[len("RCPT TO:"):])
						mu.Unlock()
						w("250 ok")
					case cmd == "DATA":
						inData = true
						w("354 go")
					case cmd == "QUIT":
						w("221 bye")
						return
					default:
						w("250 ok")
					}
				}
			}(conn)
		}
	}()
	host, port, _ = net.SplitHostPort(ln.Addr().String())
	return host, port, data, rcpt
}

func TestSendTestWebhook_Email(t *testing.T) {
	host, port, data, rcpt := smtpStub(t)
	n := newTestNotifier(extpoint.SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})
	require.True(t, n.SMTPConfigured())

	code, err := n.SendTestWebhook(context.Background(), &alert.NotificationChannel{Type: "email", URL: "ops@example.com"})
	require.NoError(t, err)
	assert.Equal(t, 200, code)
	assert.Equal(t, []string{"<ops@example.com>"}, *rcpt)
	assert.Contains(t, data.String(), "Subject: maintenant Test Notification")
}

func TestSendNow_Email(t *testing.T) {
	host, port, data, _ := smtpStub(t)
	n := newTestNotifier(extpoint.SMTPConfig{Host: host, Port: port, From: "maintenant@example.com"})

	err := n.SendNow(context.Background(), firedAlert(), &alert.NotificationChannel{Type: "email", URL: "ops@example.com"})
	require.NoError(t, err)
	assert.Contains(t, data.String(), "Subject: [maintenant] ALERT: Connection refused after 3 attempts")
	assert.Contains(t, data.String(), "Entity: api.example.com (endpoint)")
}

func TestEmail_WithoutSMTP(t *testing.T) {
	n := newTestNotifier(extpoint.SMTPConfig{})
	assert.False(t, n.SMTPConfigured())

	_, err := n.SendTestWebhook(context.Background(), &alert.NotificationChannel{Type: "email", URL: "ops@example.com"})
	require.EqualError(t, err, "SMTP not configured")
	require.EqualError(t, n.SendNow(context.Background(), firedAlert(), &alert.NotificationChannel{Type: "email", URL: "ops@example.com"}),
		"SMTP not configured")
}

func TestEmailFormat(t *testing.T) {
	a := firedAlert()
	a.Source = "update"
	a.Details = `{"update_command":"docker pull x","rollback_command":"docker pull y"}`

	assert.Equal(t, "[maintenant] ALERT: Connection refused after 3 attempts", formatEmailSubject("alert.fired", a))
	assert.Equal(t, "[maintenant] RESOLVED: Connection refused after 3 attempts", formatEmailSubject("alert.resolved", a))
	assert.Equal(t, "[maintenant] TEST: Connection refused after 3 attempts", formatEmailSubject("test", a))

	body := formatEmailBody("alert.fired", a)
	assert.Contains(t, body, "Update command:\n  docker pull x")
	assert.Contains(t, body, "Rollback command:\n  docker pull y")
}

func TestValidators(t *testing.T) {
	n := newTestNotifier(extpoint.SMTPConfig{})

	email, ok := n.Validator("email")
	require.True(t, ok)
	assert.NoError(t, email.ValidateDestination("ops@example.com"))
	assert.EqualError(t, email.ValidateDestination("not an address"), "invalid email address")

	tg, ok := n.Validator("telegram")
	require.True(t, ok)
	assert.NoError(t, tg.ValidateDestination("-1001234567890"))
	assert.Error(t, tg.ValidateDestination("https://example.com"))
	assert.NoError(t, tg.ValidateCredentials(sentinelToken, `{"thread_id":"42"}`))
	assert.EqualError(t, tg.ValidateCredentials(sentinelToken, "{"), "config must be a JSON object")

	_, ok = n.Validator("slack")
	assert.False(t, ok, "webhook-style channels are validated as URLs by the caller")
}

func TestSlackAndTeamsFormats_UpdateCommands(t *testing.T) {
	a := firedAlert()
	a.Source = "update"
	a.Details = `{"update_command":"docker pull x"}`

	raw, err := formatSlackPayload("alert.fired", a)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "*Update command:*")

	raw, err = formatTeamsPayload("alert.fired", a)
	require.NoError(t, err)
	var card map[string]any
	require.NoError(t, json.Unmarshal(raw, &card))
	assert.Equal(t, "EF4444", card["themeColor"])
	assert.Contains(t, string(raw), "Update Command")
}

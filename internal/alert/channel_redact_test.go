// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, C8: the channel model validates and masks its secrets.

// FR-020: headers must be a JSON object whose values are strings and whose names are not empty.
func TestParseHeaders(t *testing.T) {
	m, err := ParseHeaders("")
	require.NoError(t, err)
	assert.Empty(t, m)

	m, err = ParseHeaders("{}")
	require.NoError(t, err)
	assert.Empty(t, m)

	m, err = ParseHeaders(`{"Authorization":"Bearer x","X-Trace":"1"}`)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Authorization": "Bearer x", "X-Trace": "1"}, m)

	for _, raw := range []string{
		"x",
		"[1]",
		`["a"]`,
		"42",
		`"text"`,
		`{"a":1}`,
		`{"a":true}`,
		`{"a":null}`,
		`{"a":{"b":"c"}}`,
		`{"a":["b"]}`,
		`{"":"v"}`,
		`{"a":"b"`,
	} {
		_, err := ParseHeaders(raw)
		assert.Errorf(t, err, "%q must be refused", raw)
	}
}

// FR-021, US3 scenario 2 and 3: each header value becomes *** and has_headers says whether any is stored.
func TestNotificationChannel_RedactMasksHeaderValues(t *testing.T) {
	ch := &NotificationChannel{Type: "webhook", URL: "https://hooks.example.com/abc", Headers: `{"X-Trace":"1","Authorization":"Bearer s3cr3t"}`}
	ch.Redact()
	assert.Equal(t, `{"Authorization":"***","X-Trace":"***"}`, ch.Headers, "same keys, sorted, every value masked")
	assert.True(t, ch.HasHeaders)
	assert.NotContains(t, ch.Headers, "s3cr3t")

	other := &NotificationChannel{Type: "webhook", URL: "https://hooks.example.com/abc", Headers: `{"X-Api-Key":"k-9f8e7d"}`}
	other.Redact()
	assert.Equal(t, `{"X-Api-Key":"***"}`, other.Headers)
	assert.True(t, other.HasHeaders)

	none := &NotificationChannel{Type: "webhook", URL: "https://hooks.example.com/abc"}
	none.Redact()
	assert.False(t, none.HasHeaders)
	assert.Empty(t, none.Headers)

	emptyObject := &NotificationChannel{Type: "webhook", URL: "https://hooks.example.com/abc", Headers: "{}"}
	emptyObject.Redact()
	assert.False(t, emptyObject.HasHeaders, "an empty object holds no header")
	assert.Empty(t, emptyObject.Headers)
}

// FR-023, US3 scenario 4: platform webhook URLs are cut after the host; the other types keep theirs.
func TestNotificationChannel_RedactTruncatesPlatformURLs(t *testing.T) {
	for _, tc := range []struct {
		typ, url, want string
		secret         bool
	}{
		{"slack", "https://hooks.slack.com/services/T000/B000/XXXXsecret", "https://hooks.slack.com", true},
		{"discord", "https://discord.com/api/webhooks/1234/abcdTOKEN", "https://discord.com", true},
		{"teams", "https://acme.webhook.office.com/webhookb2/guid@guid/IncomingWebhook/id/key", "https://acme.webhook.office.com", true},
		{"webhook", "https://hooks.example.com/path/kept", "https://hooks.example.com/path/kept", false},
		{"email", "ops@example.com", "ops@example.com", false},
		{"telegram", "-1001234567890", "-1001234567890", false},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			ch := &NotificationChannel{Type: tc.typ, URL: tc.url}
			ch.Redact()
			assert.Equal(t, tc.want, ch.URL)
			assert.Equal(t, tc.secret, ch.HasURLSecret)
			assert.False(t, ch.HasHeaders)
		})
	}
}

// Assumption "le masquage ne concerne que la lecture": a delivery still sends the stored headers.
func TestWebhookSender_SendsTheStoredHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := newTestNotifier()
	ch := &NotificationChannel{Type: "webhook", URL: srv.URL, Headers: `{"Authorization":"Bearer s3cr3t","X-Trace":"1"}`}

	_, err := n.SendTestWebhook(context.Background(), ch)
	require.NoError(t, err)
	assert.Equal(t, "Bearer s3cr3t", got.Get("Authorization"))
	assert.Equal(t, "1", got.Get("X-Trace"))
}

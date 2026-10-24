// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"context"
	"encoding/json"
	"mime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// Acceptance tests of 049-email-subject-prefix: the subject an email channel sends, and the label it accepts.

type mailbox struct {
	n    *alert.Notifier
	data *strings.Builder
}

func newMailbox(t *testing.T) mailbox {
	t.Helper()
	host, port, data, _ := smtpStub(t)
	return mailbox{
		n:    newTestNotifier(extpoint.SMTPConfig{Host: host, Port: port, From: "maintenant@example.com", AllowPlaintext: true}),
		data: data,
	}
}

// subjects returns the decoded Subject of every message received, in order.
func (m mailbox) subjects(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, line := range m.rawSubjects() {
		decoded, err := new(mime.WordDecoder).DecodeHeader(line)
		require.NoError(t, err)
		out = append(out, decoded)
	}
	return out
}

func (m mailbox) rawSubjects() []string {
	var out []string
	for _, line := range strings.Split(m.data.String(), "\n") {
		if raw, ok := strings.CutPrefix(line, "Subject: "); ok {
			out = append(out, raw)
		}
	}
	return out
}

func subjectConfig(label string) string {
	raw, _ := json.Marshal(map[string]string{"subject_label": label})
	return string(raw)
}

func labelled(label string) *alert.NotificationChannel {
	return &alert.NotificationChannel{ID: "ch-" + label, Name: "ops mail", Type: "email", URL: "ops@example.com", Config: subjectConfig(label)}
}

func emailAlert(message, status string) *alert.Alert {
	a := firedAlert()
	a.Message = message
	a.Status = status
	return a
}

// US1 scenarios 1 and 2, FR-002, SC-002: the label replaces maintenant in the subject of an alert and of its recovery.
func TestEmailSubject_LabelLeadsAlertAndRecovery(t *testing.T) {
	for _, tc := range []struct{ label, message string }{
		{"prod-paris", "Connection refused after 3 attempts"},
		{"staging", "Container web exited with code 137"},
		{"eu west 1", "Certificate expires in 6 days"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			mb := newMailbox(t)
			ch := labelled(tc.label)

			require.NoError(t, mb.n.SendNow(context.Background(), emailAlert(tc.message, alert.StatusActive), ch))
			require.NoError(t, mb.n.SendNow(context.Background(), emailAlert(tc.message, alert.StatusResolved), ch))

			assert.Equal(t, []string{
				"[" + tc.label + "] ALERT: " + tc.message,
				"[" + tc.label + "] RESOLVED: " + tc.message,
			}, mb.subjects(t))
		})
	}
}

// US1 scenario 4, FR-003, SC-003, edge "only spaces": without a label the alert and recovery subjects are today's, byte for byte.
func TestEmailSubject_WithoutLabelStaysAsToday(t *testing.T) {
	for name, config := range map[string]string{
		"no config":    "",
		"empty object": "{}",
		"empty label":  `{"subject_label":""}`,
		"only spaces":  `{"subject_label":"    "}`,
	} {
		t.Run(name, func(t *testing.T) {
			mb := newMailbox(t)
			ch := &alert.NotificationChannel{Type: "email", URL: "ops@example.com", Config: config}

			require.NoError(t, mb.n.SendNow(context.Background(), emailAlert("Connection refused after 3 attempts", alert.StatusActive), ch))
			require.NoError(t, mb.n.SendNow(context.Background(), emailAlert("Connection refused after 3 attempts", alert.StatusResolved), ch))

			assert.Equal(t, []string{
				"[maintenant] ALERT: Connection refused after 3 attempts",
				"[maintenant] RESOLVED: Connection refused after 3 attempts",
			}, mb.rawSubjects())
		})
	}
}

// US1 scenario 3, SC-004: the same alert on a labelled and an unlabelled channel; one rule on "[prod]" sorts every
// mail of the first, test included, and none of the second.
func TestEmailSubject_EachChannelUsesItsOwnLabel(t *testing.T) {
	mb := newMailbox(t)
	ctx := context.Background()
	prod := labelled("prod")
	plain := &alert.NotificationChannel{ID: "plain", Type: "email", URL: "ops@example.com"}
	a := emailAlert("Endpoint api.example.com is down", alert.StatusActive)

	require.NoError(t, mb.n.SendNow(ctx, a, prod))
	require.NoError(t, mb.n.SendNow(ctx, a, plain))
	_, err := mb.n.SendTestWebhook(ctx, prod)
	require.NoError(t, err)
	_, err = mb.n.SendTestWebhook(ctx, plain)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"[prod] ALERT: Endpoint api.example.com is down",
		"[maintenant] ALERT: Endpoint api.example.com is down",
		"[prod] TEST: Test notification",
		"[maintenant] TEST: Test notification",
	}, mb.subjects(t))
}

// US2 scenarios 1 and 2, FR-004: the test email has the alert format, with the channel's label or maintenant.
func TestEmailSubject_TestEmailFollowsTheSameFormat(t *testing.T) {
	for _, tc := range []struct{ name, config, want string }{
		{"staging", subjectConfig("staging"), "[staging] TEST: Test notification"},
		{"qa-lyon", subjectConfig("qa-lyon"), "[qa-lyon] TEST: Test notification"},
		{"no config", "", "[maintenant] TEST: Test notification"},
		{"empty label", subjectConfig(""), "[maintenant] TEST: Test notification"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mb := newMailbox(t)

			code, err := mb.n.SendTestWebhook(context.Background(), &alert.NotificationChannel{Type: "email", URL: "ops@example.com", Config: tc.config})

			require.NoError(t, err)
			assert.Equal(t, 200, code)
			assert.Equal(t, []string{tc.want}, mb.subjects(t))
		})
	}
}

// FR-006, FR-008, edges "accents", "brackets", "spaces at the edges": the label goes in as typed, minus its edge spaces.
func TestEmailSubject_LabelIsTrimmedAndOtherwiseVerbatim(t *testing.T) {
	for _, tc := range []struct{ label, prefix string }{
		{" prod ", "[prod]"},
		{"   staging", "[staging]"},
		{"[prod]", "[[prod]]"},
		{"a  b", "[a  b]"},
		{"préprod", "[préprod]"},
		{"東京", "[東京]"},
		{"  Zürich-東京 ", "[Zürich-東京]"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			mb := newMailbox(t)
			ch := labelled(tc.label)

			require.NoError(t, mb.n.SendNow(context.Background(), emailAlert("Disk 91% full", alert.StatusActive), ch))
			_, err := mb.n.SendTestWebhook(context.Background(), ch)
			require.NoError(t, err)

			assert.Equal(t, []string{tc.prefix + " ALERT: Disk 91% full", tc.prefix + " TEST: Test notification"}, mb.subjects(t))
			for _, raw := range mb.rawSubjects() {
				for _, b := range []byte(raw) {
					require.Less(t, b, byte(0x80), "the Subject header must stay 7-bit (RFC 2047), got %q", raw)
				}
			}
		})
	}
}

// FR-010: the label reaches a delivery from an alert trigger (the notifier's queue) and one from an escalation
// policy (SendNow, which the escalation runner calls with the channel it reads from the store).
func TestEmailSubject_TriggerAndEscalationDeliveriesCarryTheLabel(t *testing.T) {
	mb := newMailbox(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mb.n.Start(ctx)
	ch := labelled("ops-eu")

	done := make(chan error, 1)
	mb.n.Enqueue(alert.NotificationJob{
		Delivery: &alert.NotificationDelivery{},
		Channel:  ch,
		Alert:    emailAlert("Disk 91% full", alert.StatusActive),
		Done:     func(_ context.Context, err error) { done <- err },
	})
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("the queued delivery never finished")
	}
	require.NoError(t, mb.n.SendNow(ctx, emailAlert("Disk 97% full", alert.StatusActive), ch))

	assert.Equal(t, []string{"[ops-eu] ALERT: Disk 91% full", "[ops-eu] ALERT: Disk 97% full"}, mb.subjects(t))
}

// US3 scenarios 1 and 2, FR-006, FR-007: the email validator, which the API and the MCP tools call, takes up to 64
// characters once the edge spaces are gone, and refuses more or any control character, saying which.
func TestEmailSubject_ValidatorRefusesWhatCannotGoInASubject(t *testing.T) {
	v, ok := newTestNotifier(extpoint.SMTPConfig{}).Validator("email")
	require.True(t, ok)

	for name, config := range map[string]string{
		"no config":            "",
		"empty object":         "{}",
		"empty label":          subjectConfig(""),
		"only spaces":          subjectConfig("    "),
		"typical":              subjectConfig("prod-paris"),
		"64 ascii":             subjectConfig(strings.Repeat("a", 64)),
		"64 non-latin":         subjectConfig(strings.Repeat("東", 64)),
		"64 accented":          subjectConfig(strings.Repeat("é", 64)),
		"64 between spaces":    subjectConfig("  " + strings.Repeat("x", 64) + "  "),
		"brackets and accents": subjectConfig("[préprod]"),
	} {
		assert.NoError(t, v.ValidateCredentials("", config), name)
	}

	for name, label := range map[string]string{
		"65 ascii":             strings.Repeat("a", 65),
		"65 non-latin":         strings.Repeat("東", 65),
		"65 with inner spaces": strings.Repeat("ab ", 21) + "ab",
		"65 between spaces":    " " + strings.Repeat("z", 65) + " ",
	} {
		err := v.ValidateCredentials("", subjectConfig(label))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "64", "%s: the message gives the limit", name)
		assert.Regexp(t, `(?i)subject.?label`, err.Error(), "%s: the message names the label", name)
	}

	for name, label := range map[string]string{
		"line feed":          "prod\nparis",
		"carriage return":    "prod\rparis",
		"header injection":   "prod\r\nBcc: evil@example.com",
		"trailing line feed": "prod\n",
		"leading tab":        "\tprod",
		"inner tab":          "prod\tparis",
		"nul":                "prod\x00",
		"delete":             "prod\x7f",
		"C1 next line":       "prod\u0085paris",
	} {
		err := v.ValidateCredentials("", subjectConfig(label))
		require.Error(t, err, name)
		assert.Regexp(t, `(?i)control`, err.Error(), "%s: the message says a control character is the problem", name)
		assert.Regexp(t, `(?i)subject.?label`, err.Error(), "%s: the message names the label", name)
	}
}

// US3 "why", plan constraint: a label already on file with a line break never adds a header to the mail.
func TestEmailSubject_StoredLabelCannotAddAHeader(t *testing.T) {
	mb := newMailbox(t)
	ch := labelled("prod\r\nBcc: evil@example.com")

	_ = mb.n.SendNow(context.Background(), firedAlert(), ch)
	_, _ = mb.n.SendTestWebhook(context.Background(), ch)

	for _, line := range strings.Split(mb.data.String(), "\n") {
		assert.False(t, strings.HasPrefix(strings.ToLower(line), "bcc:"), "the label added a header: %q", line)
	}
}

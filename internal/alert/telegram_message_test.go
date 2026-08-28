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
	"strings"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func firedAlert() *Alert {
	return &Alert{
		ID:         "a1",
		Source:     "endpoint",
		AlertType:  "endpoint_down",
		Severity:   SeverityCritical,
		Status:     StatusActive,
		Message:    "Connection refused after 3 attempts",
		EntityType: "endpoint",
		EntityName: "api.example.com",
		FiredAt:    time.Date(2026, 8, 27, 14, 32, 7, 0, time.UTC),
	}
}

// FR-009: everything the operator needs is in the message, and the severity and
// entity are on the first line — a phone notification shows little (SC-006).
func TestBuildTelegramMessage_Fired(t *testing.T) {
	msg := BuildTelegramMessage(event.AlertFired, firedAlert())

	assert.Contains(t, msg, "<b>Alert: api.example.com</b>")
	assert.Contains(t, msg, "<b>Severity</b> · critical")
	assert.Contains(t, msg, "<b>Source</b> · endpoint")
	assert.Contains(t, msg, "<b>Entity</b> · api.example.com")
	assert.Contains(t, msg, "2026-08-27 14:32:07 UTC")
	assert.Contains(t, msg, "Connection refused after 3 attempts")

	head := msg
	if len(head) > 80 {
		head = head[:80]
	}
	assert.Contains(t, head, "api.example.com",
		"the entity must be readable in a truncated notification (SC-006)")
	assert.True(t, strings.HasPrefix(msg, "\xF0\x9F\x94\xB4"),
		"a critical alert opens on the red circle, before any text")
}

// FR-010: the recovery is distinguishable from its first character, without
// reading the text.
func TestBuildTelegramMessage_Resolved(t *testing.T) {
	a := firedAlert()
	a.Status = StatusResolved
	resolvedAt := time.Date(2026, 8, 27, 14, 41, 52, 0, time.UTC)
	a.ResolvedAt = &resolvedAt
	a.Message = "Back to 200 OK"

	msg := BuildTelegramMessage(event.AlertResolved, a)

	assert.True(t, strings.HasPrefix(msg, "\xE2\x9C\x85"), "recovery opens on the check mark")
	assert.Contains(t, msg, "<b>Resolved: api.example.com</b>")
	assert.Contains(t, msg, "<b>Resolved</b> · 2026-08-27 14:41:52 UTC")
	assert.NotContains(t, msg, "<b>Severity</b>",
		"a recovery has no severity to announce; the alert is over")
}

// FR-011: markup characters in an alert are displayed, and never make Telegram
// reject the whole message.
func TestBuildTelegramMessage_EscapesDynamicValues(t *testing.T) {
	a := firedAlert()
	a.EntityName = "<script>alert(1)</script>"
	a.Message = "a < b && c > d"

	msg := BuildTelegramMessage(event.AlertFired, a)

	assert.Contains(t, msg, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assert.Contains(t, msg, "a &lt; b &amp;&amp; c &gt; d")
	assert.NotContains(t, msg, "<script>")

	// The only tags left are the template's own.
	for _, tag := range []string{"<b>", "</b>"} {
		assert.Contains(t, msg, tag)
	}
	assert.Equal(t, strings.Count(msg, "<b>"), strings.Count(msg, "</b>"))
}

// FR-012: over the limit, the message still goes out, the cut is visible, and
// the header survives intact.
func TestBuildTelegramMessage_TruncatesTheDescription(t *testing.T) {
	a := firedAlert()
	a.Message = strings.Repeat("x", 8000)

	msg := BuildTelegramMessage(event.AlertFired, a)

	require.LessOrEqual(t, len(msg), telegramMaxLen)
	assert.True(t, strings.HasSuffix(msg, telegramTruncationMark), "the cut must be visible")
	assert.Contains(t, msg, "<b>Severity</b> · critical", "the header is never what gets cut")
	assert.Contains(t, msg, "<b>Entity</b> · api.example.com")
}

// A byte-wise cut must not leave half a rune, nor half an HTML entity: "&am"
// would show as literal text where "&amp;" shows an ampersand.
func TestBuildTelegramMessage_TruncationLeavesValidMarkup(t *testing.T) {
	for _, filler := range []string{"é", "&", "<"} {
		a := firedAlert()
		a.Message = strings.Repeat(filler, 6000)

		msg := BuildTelegramMessage(event.AlertFired, a)

		require.LessOrEqual(t, len(msg), telegramMaxLen, "filler %q", filler)
		assert.True(t, utf8ValidString(msg), "filler %q left a partial rune", filler)
		body := strings.TrimSuffix(msg, telegramTruncationMark)
		if i := strings.LastIndexByte(body, '&'); i >= 0 {
			assert.Contains(t, body[i:], ";", "filler %q left a half-written entity", filler)
		}
	}
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == 0xFFFD {
			return false
		}
	}
	return true
}

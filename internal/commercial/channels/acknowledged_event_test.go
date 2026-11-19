// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

func TestAcknowledgedEventIsNeverWordedAsARecovery(t *testing.T) {
	a := firedAlert()
	ackedAt := time.Date(2026, 8, 27, 14, 40, 0, 0, time.UTC)
	a.AcknowledgedAt, a.AcknowledgedBy = &ackedAt, "alice"
	a.Message = "Acknowledged by alice at 2026-08-27T14:40:00Z: " + a.Message

	for name, format := range map[string]alert.PayloadFormatter{
		"slack": formatSlackPayload,
		"teams": formatTeamsPayload,
	} {
		body, err := format(event.AlertAcknowledged, a)
		require.NoError(t, err, name)
		assert.Contains(t, string(body), "Acknowledged: api.example.com", name)
		assert.NotContains(t, string(body), "Resolved", name)
	}

	msg := BuildTelegramMessage(event.AlertAcknowledged, a)
	header, _, _ := strings.Cut(msg, "\n\n")
	assert.Contains(t, header, "Acknowledged: api.example.com")
	assert.Contains(t, msg, "<b>By</b> · alice", "the notice says who acknowledged")
	assert.Contains(t, msg, "<b>Acknowledged</b> · 2026-08-27 14:40:00 UTC", "and when")
	assert.NotContains(t, msg, "<b>Fired</b>", "an acknowledgment does not read as a firing")
	assert.NotContains(t, msg, "<b>Severity</b>")
	assert.NotContains(t, msg, "Resolved")

	assert.Equal(t, "[maintenant] ACKNOWLEDGED: "+a.Message, formatEmailSubject("maintenant", event.AlertAcknowledged, a.Message))
	assert.Contains(t, formatEmailBody(event.AlertAcknowledged, a), "Event: alert.acknowledged")
}

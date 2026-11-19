// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package channels

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

func TestAcknowledgedEventIsNeverWordedAsARecovery(t *testing.T) {
	a := firedAlert()
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
	assert.Contains(t, msg, "Acknowledged: api.example.com")
	assert.NotContains(t, msg, "Resolved")

	assert.Equal(t, "[maintenant] ACKNOWLEDGED: "+a.Message, formatEmailSubject("maintenant", event.AlertAcknowledged, a.Message))
	assert.Contains(t, formatEmailBody(event.AlertAcknowledged, a), "Event: alert.acknowledged")
}

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/certificate"
)

func TestCertificateRecoveryEvent_ResolvesTheRecoveredAlertType(t *testing.T) {
	for _, alertType := range []string{
		certificate.AlertTypeExpiring,
		certificate.AlertTypeExpired,
		certificate.AlertTypeChainInvalid,
		certificate.AlertTypeHostnameMismatch,
		certificate.AlertTypeOCSPRevoked,
	} {
		evt := certificateRecoveryEvent(map[string]any{
			"monitor_id":          "cert-1",
			"hostname":            "app.example.com",
			"previous_alert_type": alertType,
			"agent_id":            "agent-1",
		})
		assert.Equal(t, alertType, evt.AlertType, "the recovery must carry the dedup key of the alert it resolves")
		assert.True(t, evt.IsRecover)
		assert.Equal(t, alert.SourceCertificate, evt.Source)
		assert.Equal(t, "certificate", evt.EntityType)
		assert.Equal(t, "cert-1", evt.EntityID)
		assert.Equal(t, "agent-1", evt.AgentID)
		assert.Contains(t, evt.Message, "app.example.com")
	}
}

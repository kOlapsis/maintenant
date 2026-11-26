// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/certificate"
)

func TestCertificateStore_CountAgentLabelMonitors(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedAgent(t, db, "agent-staging")
	seedAgent(t, db, "agent-prod")
	cs := NewCertificateStore(db)

	create := func(agentID, host string, source certificate.CertSource) string {
		t.Helper()
		id, err := cs.CreateMonitor(ctx, &certificate.CertMonitor{
			Hostname: host, Port: 443, Source: source, Status: certificate.StatusUnknown,
			CheckIntervalSeconds: 43200, WarningThresholds: certificate.DefaultWarningThresholds(), AgentID: agentID,
		})
		require.NoError(t, err)
		return id
	}
	create("agent-staging", "a.example.com", certificate.SourceLabel)
	inactive := create("agent-staging", "b.example.com", certificate.SourceLabel)
	create("agent-staging", "c.example.com", certificate.SourceStandalone)
	create("agent-prod", "a.example.com", certificate.SourceLabel)
	require.NoError(t, cs.DeactivateMonitor(ctx, inactive))

	held, err := cs.CountAgentLabelMonitors(ctx, "agent-staging")
	require.NoError(t, err)
	assert.Equal(t, 2, held, "inactive label monitors still hold a row, standalone ones and other agents do not count")
}

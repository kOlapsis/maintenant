// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/status"
)

func TestStatusProvider_WithdrawnMonitorIsOperational(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, nil)
	derive := func(monitorType, id string) string {
		return a.statusSvc.DeriveComponentStatus(ctx, &status.Component{
			Monitors: []status.MonitorRef{{Type: monitorType, ID: id}},
		})
	}

	epID, err := a.epStore.UpsertEndpoint(ctx, &endpoint.Endpoint{
		ContainerName: "web", LabelKey: "http", EndpointType: endpoint.TypeHTTP, Target: "http://web/health", Active: true,
	})
	require.NoError(t, err)
	require.NoError(t, a.epStore.UpdateCheckResult(ctx, epID, endpoint.StatusUnknown, endpoint.AlertNormal, 0, 0, 0, nil, "container stopped"))
	assert.Equal(t, status.StatusMajorOutage, derive("endpoint", epID), "container stopped")

	require.NoError(t, a.epStore.DeactivateEndpoint(ctx, epID))
	assert.Equal(t, status.StatusOperational, derive("endpoint", epID), "endpoint withdrawn")

	certID, err := a.certStore.CreateMonitor(ctx, &certificate.CertMonitor{
		Hostname: "example.com", Port: 443, Source: certificate.SourceLabel,
		Status: certificate.StatusExpired, CheckIntervalSeconds: 3600, WarningThresholds: []int{30, 7},
	})
	require.NoError(t, err)
	assert.Equal(t, status.StatusMajorOutage, derive("certificate", certID), "certificate expired")

	require.NoError(t, a.certStore.DeactivateMonitor(ctx, certID))
	assert.Equal(t, status.StatusOperational, derive("certificate", certID), "certificate withdrawn")
}

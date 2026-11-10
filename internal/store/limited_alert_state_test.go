// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/uid"
)

func TestEndpointStore_LimitedEndpointForgetsItsAlertState(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	eps := NewEndpointStore(db)

	id := clLabelEndpoint(t, eps, uid.LocalAgent, "web", "maintenant.endpoint.http", "http://web:80")
	require.NoError(t, eps.UpdateCheckResult(ctx, id, endpoint.StatusDown, endpoint.AlertAlerting, 5, 0, 0, nil, "refused"))

	require.NoError(t, eps.SetEndpointLimited(ctx, id, true))
	ep, err := eps.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	require.Equal(t, endpoint.AlertNormal, ep.AlertState)
	require.Zero(t, ep.ConsecutiveFailures)
	require.Zero(t, ep.ConsecutiveSuccesses)
}

func TestCertificateStore_LimitedMonitorForgetsItsAlertedThreshold(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	certs := NewCertificateStore(db)

	id := clCertMonitor(t, certs, "shop.example.com", 443, certificate.SourceLabel, "ext", nil)
	_, err := db.Writer().Exec(ctx, `UPDATE cert_monitors SET last_alerted_threshold=7 WHERE id=?`, id)
	require.NoError(t, err)

	require.NoError(t, certs.SetMonitorLimited(ctx, id, true))
	m, err := certs.GetMonitorByID(ctx, id)
	require.NoError(t, err)
	require.Nil(t, m.LastAlertedThreshold)
}

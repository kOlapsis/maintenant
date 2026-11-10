// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/certificate"
)

func TestCertificateStore_ReadoptedMonitorKeepsItsRight(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	certs := NewCertificateStore(db)

	id := clCertMonitor(t, certs, "shop.example.com", 443, certificate.SourceLabel, "ext-old", nil)
	_, err := db.Writer().Exec(ctx, `UPDATE cert_monitors SET grandfathered=1, created_at=? WHERE id=?`, int64(1000), id)
	require.NoError(t, err)
	require.NoError(t, certs.DeactivateMonitor(ctx, id))

	again := clCertMonitor(t, certs, "shop.example.com", 443, certificate.SourceLabel, "ext-new", nil)
	require.Equal(t, id, again)

	m, err := certs.GetMonitorByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Nil(t, m.InactiveSince)
	require.True(t, m.Grandfathered)
	require.Equal(t, int64(1000), m.CreatedAt.Unix())
	require.Equal(t, "ext-new", m.ExternalID)

	n, err := certs.CountConfigured(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, certs.DeactivateMonitor(ctx, id))
	n, err = certs.CountConfigured(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n, "an inactive monitor is not configured")
}

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

package store

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Nobody connects an external database right on the first try. These are the
// four families FR-018 requires to be distinguishable, and SC-005 requires to
// arrive within thirty seconds of startup.

func TestOpenPostgres_InvalidDSN(t *testing.T) {
	ctx := context.Background()
	for name, raw := range map[string]string{
		"not a url":      "not-a-dsn",
		"wrong scheme":   "mysql://app:pw@db/maintenant",
		"keyword syntax": "host=db user=app dbname=maintenant",
		"empty":          "",
	} {
		t.Run(name, func(t *testing.T) {
			db, err := OpenPostgres(ctx, raw, testLogger())
			require.ErrorIs(t, err, ErrInvalidDSN)
			assert.Nil(t, db)
			assert.NotContains(t, err.Error(), "pw")
		})
	}
}

// TestOpenPostgres_Unreachable needs no test server: port 1 on loopback is
// closed. It also pins SC-005's deadline.
func TestOpenPostgres_Unreachable(t *testing.T) {
	const password = "s3cr3t-Sentinel-open"
	start := time.Now()
	db, err := OpenPostgres(context.Background(),
		"postgres://app:"+password+"@127.0.0.1:1/maintenant?sslmode=disable", testLogger())

	require.ErrorIs(t, err, ErrUnreachable)
	assert.Nil(t, db)
	assert.NotErrorIs(t, err, ErrAuthRefused, "an unreachable host is not refused credentials")
	assert.Less(t, time.Since(start), 30*time.Second, "SC-005: the operator learns why within 30s")
	assert.NotContains(t, err.Error(), password)
}

// TestOpenPostgres_AuthRefused distinguishes refused credentials from an
// unreachable host: the same message for both would send the operator looking
// at the firewall over a typo in a password.
func TestOpenPostgres_AuthRefused(t *testing.T) {
	adminDSN := testAdminDSN(t)
	u, err := ParseDSN(adminDSN)
	require.NoError(t, err)

	name := u.User.Username()
	if name == "" {
		name = "postgres"
	}
	u.User = url.UserPassword(name, "definitely-not-the-password")

	db, err := OpenPostgres(context.Background(), u.String(), testLogger())
	require.ErrorIs(t, err, ErrAuthRefused)
	assert.Nil(t, db)
	assert.NotErrorIs(t, err, ErrUnreachable, "the database answered; it refused us")
	assert.NotContains(t, err.Error(), "definitely-not-the-password")
}

// TestCheckServerVersion covers the version refusal without needing an old
// server to run against.
func TestCheckServerVersion(t *testing.T) {
	assert.NoError(t, checkServerVersion(140000), "PostgreSQL 14 is the minimum")
	assert.NoError(t, checkServerVersion(160004))

	err := checkServerVersion(130010)
	require.ErrorIs(t, err, ErrUnsupportedVersion)
	assert.Contains(t, err.Error(), "14", "the message names the minimum expected")
}

// TestOpenPostgres_AppliesDefaultSSLMode pins FR-022 at the open boundary: a
// remote host with no explicit sslmode is reached over TLS. The test server is
// local, so it must be left alone — that is the other half of the rule.
func TestOpenPostgres_AppliesDefaultSSLMode(t *testing.T) {
	dsn := createTestDatabase(t, testAdminDSN(t))
	db, err := OpenPostgres(context.Background(), dsn, testLogger())
	require.NoError(t, err, "a local test server without TLS must still open")
	t.Cleanup(func() { _ = db.Close() })

	// The stored DSN is what the driver received.
	assert.NotContains(t, db.dsn, "sslmode=require",
		"a loopback host keeps whatever the operator wrote")
}

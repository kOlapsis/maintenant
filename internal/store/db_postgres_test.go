// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"encoding/binary"
	"net"
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

// sslRequestServer answers each SSLRequest as PostgreSQL does with ssl=off ('N') or ssl=on ('S'), then hangs up.
func sslRequestServer(t *testing.T, answer byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				var req [2]int32
				if binary.Read(conn, binary.BigEndian, &req) != nil || req != [2]int32{8, sslRequestCode} {
					return
				}
				_, _ = conn.Write([]byte{answer})
			}()
		}
	}()
	return ln.Addr().String()
}

func TestOpenPostgres_TLSRefused(t *testing.T) {
	const password = "s3cr3t-Sentinel-tls"
	addr := sslRequestServer(t, 'N')

	db, err := OpenPostgres(context.Background(),
		"postgres://app:"+password+"@"+addr+"/maintenant?sslmode=require", testLogger())

	require.ErrorIs(t, err, ErrTLSRefused)
	assert.Nil(t, db)
	assert.NotErrorIs(t, err, ErrUnreachable, "the server answered")
	assert.NotContains(t, err.Error(), password)
}

func TestOpenPostgres_TLSOfferedIsNotRefused(t *testing.T) {
	addr := sslRequestServer(t, 'S')

	_, err := OpenPostgres(context.Background(),
		"postgres://app:pw@"+addr+"/maintenant?sslmode=require", testLogger())

	require.ErrorIs(t, err, ErrUnreachable, "a server offering TLS did not refuse it")
	assert.NotErrorIs(t, err, ErrTLSRefused)
}

func TestOpenPostgres_OptionalTLSIsNeverBlamed(t *testing.T) {
	addr := sslRequestServer(t, 'N')

	_, err := OpenPostgres(context.Background(),
		"postgres://app:pw@"+addr+"/maintenant?sslmode=prefer", testLogger())

	require.ErrorIs(t, err, ErrUnreachable, "pgx falls back to plain text, so TLS is not the failure")
	assert.NotErrorIs(t, err, ErrTLSRefused)
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

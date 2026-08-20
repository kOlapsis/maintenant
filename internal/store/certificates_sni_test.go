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
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/stretchr/testify/require"
)

func TestCertificateStore_SNIMonitorsCoexist(t *testing.T) {
	requireSQLite(t)
	db := openTestDB(t)
	store := NewCertificateStore(db)
	ctx := context.Background()

	plain := &certificate.CertMonitor{
		Hostname: "proxy2.lan", Port: 443,
		Source: certificate.SourceStandalone, Status: certificate.StatusUnknown,
		CheckIntervalSeconds: 43200, WarningThresholds: certificate.DefaultWarningThresholds(),
	}
	withSNI := &certificate.CertMonitor{
		Hostname: "proxy2.lan", Port: 443, ServerName: "service.example.com",
		Source: certificate.SourceStandalone, Status: certificate.StatusUnknown,
		CheckIntervalSeconds: 43200, WarningThresholds: certificate.DefaultWarningThresholds(),
	}

	plainID, err := store.CreateMonitor(ctx, plain)
	require.NoError(t, err)
	sniID, err := store.CreateMonitor(ctx, withSNI)
	require.NoError(t, err)
	require.NotEqual(t, plainID, sniID, "SNI must be part of the derived identity")

	// Each lookup resolves its own monitor.
	got, err := store.GetMonitorByHostPort(ctx, "proxy2.lan", 443, "")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, plainID, got.ID)
	require.Empty(t, got.ServerName)

	got, err = store.GetMonitorByHostPort(ctx, "proxy2.lan", 443, "service.example.com")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, sniID, got.ID)
	require.Equal(t, "service.example.com", got.ServerName, "server_name must round-trip through the scanner")

	got, err = store.GetMonitorByHostPortAgent(ctx, nil, "proxy2.lan", 443, "service.example.com")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, sniID, got.ID)
}

// TestRebuildCertMonitorsForSNI exercises the one-time identity rebuild against
// a database shaped like a pre-SNI UUID conversion: three-column UNIQUE
// constraint plus the server_name column appended by migration 25.
func TestRebuildCertMonitorsForSNI(t *testing.T) {
	requireSQLite(t)
	db := openTestDB(t)
	rw := db.ReadDB()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Downgrade the table to the pre-SNI shape, with one row to carry over.
	_, err := rw.ExecContext(ctx, `
		DROP TABLE cert_monitors;
		CREATE TABLE cert_monitors (
		    id              TEXT PRIMARY KEY NOT NULL,
		    agent_id        TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
		    hostname        TEXT NOT NULL,
		    port            INTEGER NOT NULL DEFAULT 443,
		    source          TEXT NOT NULL CHECK(source IN ('auto','standalone','label')),
		    endpoint_id     TEXT REFERENCES endpoints(id) ON DELETE SET NULL,
		    status          TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('valid','expiring','expired','error','unknown')),
		    check_interval_seconds INTEGER NOT NULL DEFAULT 43200,
		    warning_thresholds_json TEXT NOT NULL DEFAULT '[30,14,7,3,1]',
		    last_alerted_threshold INTEGER,
		    last_check_at   BIGINT,
		    next_check_at   BIGINT,
		    last_error      TEXT,
		    created_at      BIGINT NOT NULL,
		    external_id     TEXT NOT NULL DEFAULT '',
		    UNIQUE(agent_id, hostname, port)
		);
		ALTER TABLE cert_monitors ADD COLUMN server_name TEXT NOT NULL DEFAULT '';
		INSERT INTO cert_monitors (id, hostname, port, source, created_at)
		VALUES ('legacy-id', 'legacy.example.com', 443, 'standalone', 1700000000);
	`)
	require.NoError(t, err)

	require.NoError(t, rebuildCertMonitorsForSNI(ctx, rw, logger))

	ddl := scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='cert_monitors'`)
	require.Contains(t, ddl, certSNIConstraint)

	// Row and indexes survived the rebuild.
	require.Equal(t, "legacy-id", scanString(t, rw, `SELECT id FROM cert_monitors`))
	var indexes int
	require.NoError(t, rw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='cert_monitors' AND name LIKE 'idx_%'`).Scan(&indexes))
	require.Equal(t, 5, indexes)

	// Second run is a no-op (guard matches the new constraint).
	require.NoError(t, rebuildCertMonitorsForSNI(ctx, rw, logger))
	require.Equal(t, ddl, scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='cert_monitors'`))
}

// Compile-time-ish guard: the rebuild's CREATE TABLE must embed the exact
// constraint string the idempotency check looks for.
func TestCertSNIConstraint_MatchesSchema(t *testing.T) {
	requireSQLite(t)
	schema, err := os.ReadFile("uuid_schema.sql")
	require.NoError(t, err)
	require.True(t, strings.Contains(string(schema), certSNIConstraint),
		"uuid_schema.sql must contain %q verbatim — the rebuild guard depends on it", certSNIConstraint)
}

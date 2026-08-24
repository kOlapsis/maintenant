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
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// certSNIConstraint is the SNI-aware identity constraint. It must stay
// byte-identical to the cert_monitors DDL in uuid_schema.sql — SQLite stores
// the CREATE TABLE text verbatim and the idempotency guard matches on it.
const certSNIConstraint = "UNIQUE(agent_id, hostname, port, server_name)"

// rebuildCertMonitorsForSNI upgrades the cert monitor identity from
// (agent_id, hostname, port) to (agent_id, hostname, port, server_name) on
// databases converted to the UUID schema before the SNI feature. SQLite cannot
// alter a table-level UNIQUE constraint, so the table is rebuilt once; fresh
// installs and databases converted by this release get the new constraint
// straight from uuid_schema.sql and skip the rebuild.
func rebuildCertMonitorsForSNI(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	var ddl string
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='cert_monitors'`).Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cert sni rebuild: probe ddl: %w", err)
	}
	if strings.Contains(ddl, certSNIConstraint) {
		return nil
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("cert sni rebuild: acquire conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("cert sni rebuild: fk off: %w", err)
	}

	logger.Info("cert monitor SNI rebuild starting")

	if err := runCertSNIRebuild(ctx, conn); err != nil {
		return err
	}

	if err := foreignKeyCheck(ctx, conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("cert sni rebuild: fk on: %w", err)
	}

	logger.Info("cert monitor SNI rebuild complete")
	return nil
}

// runCertSNIRebuild performs the table rebuild inside one transaction.
func runCertSNIRebuild(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cert sni rebuild: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	stmts := []stmt{
		// Keep this CREATE TABLE byte-identical to uuid_schema.sql.
		{"create table", `CREATE TABLE cert_monitors_new (
    id              TEXT PRIMARY KEY NOT NULL,              -- uid.CertMonitor(agent,host,port[,server_name]) or minted (standalone)
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
    server_name     TEXT NOT NULL DEFAULT '',               -- SNI; '' = validate against hostname
    ` + certSNIConstraint + `
)`},
		{"copy rows", `INSERT INTO cert_monitors_new
			(id, agent_id, hostname, port, source, endpoint_id, status, check_interval_seconds,
			 warning_thresholds_json, last_alerted_threshold, last_check_at, next_check_at, last_error,
			 created_at, external_id, server_name)
			SELECT id, agent_id, hostname, port, source, endpoint_id, status, check_interval_seconds,
			 warning_thresholds_json, last_alerted_threshold, last_check_at, next_check_at, last_error,
			 created_at, external_id, server_name
			FROM cert_monitors`},
		{"drop old table", `DROP TABLE cert_monitors`},
		{"rename", `ALTER TABLE cert_monitors_new RENAME TO cert_monitors`},
		// DROP TABLE removed the old table's indexes — recreate them all.
		{"index endpoint", `CREATE INDEX idx_cert_monitor_endpoint ON cert_monitors(endpoint_id) WHERE endpoint_id IS NOT NULL`},
		{"index status", `CREATE INDEX idx_cert_monitor_status ON cert_monitors(status)`},
		{"index next_check", `CREATE INDEX idx_cert_monitor_next_check ON cert_monitors(next_check_at) WHERE source IN ('standalone','label')`},
		{"index external_id", `CREATE INDEX idx_cert_monitor_external_id ON cert_monitors(external_id) WHERE external_id != ''`},
		{"index agent_id", `CREATE INDEX idx_cert_monitors_agent_id ON cert_monitors(agent_id)`},
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("cert sni rebuild: %s: %w", s.desc, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cert sni rebuild: commit: %w", err)
	}
	committed = true
	return nil
}

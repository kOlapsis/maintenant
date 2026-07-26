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

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// endpointStatusConstraint is the status CHECK allowing the degraded state. It
// must stay byte-identical to the endpoints DDL in uuid_schema.sql — SQLite
// stores the CREATE TABLE text verbatim and the idempotency guard matches on it.
const endpointStatusConstraint = "CHECK(status IN ('up','down','degraded','unknown'))"

// rebuildEndpointsForDegraded widens the endpoint status CHECK to accept
// 'degraded' on databases converted to the UUID schema before that state
// existed. SQLite cannot alter a CHECK constraint, so the table is rebuilt once;
// fresh installs and databases converted by this release get it straight from
// uuid_schema.sql and skip the rebuild.
//
// This runs after convertToUUID rather than as a numbered migration on purpose:
// migrations execute against the legacy integer schema, where the endpoints
// table does not have agent_id yet, so one SQL file cannot serve both shapes.
func rebuildEndpointsForDegraded(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	var ddl string
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='endpoints'`).Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("endpoint degraded rebuild: probe ddl: %w", err)
	}
	if strings.Contains(ddl, endpointStatusConstraint) {
		return nil
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("endpoint degraded rebuild: acquire conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("endpoint degraded rebuild: fk off: %w", err)
	}

	logger.Info("endpoint degraded-status rebuild starting")

	if err := runEndpointDegradedRebuild(ctx, conn); err != nil {
		return err
	}

	if err := foreignKeyCheck(ctx, conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("endpoint degraded rebuild: fk on: %w", err)
	}

	logger.Info("endpoint degraded-status rebuild complete")
	return nil
}

// runEndpointDegradedRebuild performs the table rebuild inside one transaction.
func runEndpointDegradedRebuild(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("endpoint degraded rebuild: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	stmts := []stmt{
		// Keep this CREATE TABLE byte-identical to uuid_schema.sql.
		{"create table", `CREATE TABLE endpoints_new (
    id              TEXT PRIMARY KEY NOT NULL,              -- uid.EndpointLabel(agent,container,label) or minted (standalone)
    agent_id        TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    container_name  TEXT NOT NULL,
    label_key       TEXT NOT NULL,
    external_id     TEXT NOT NULL,
    endpoint_type   TEXT NOT NULL CHECK(endpoint_type IN ('http','tcp')),
    target          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown' ` + endpointStatusConstraint + `,
    alert_state     TEXT NOT NULL DEFAULT 'normal' CHECK(alert_state IN ('normal','alerting')),
    consecutive_failures  INTEGER NOT NULL DEFAULT 0,
    consecutive_successes INTEGER NOT NULL DEFAULT 0,
    last_check_at         BIGINT,
    last_response_time_ms BIGINT,
    last_http_status      INTEGER,
    last_error      TEXT,
    config_json     TEXT NOT NULL DEFAULT '{}',
    active          INTEGER NOT NULL DEFAULT 1,
    first_seen_at   BIGINT NOT NULL,
    last_seen_at    BIGINT NOT NULL,
    source          TEXT NOT NULL DEFAULT 'label' CHECK(source IN ('label','standalone')),
    name            TEXT NOT NULL DEFAULT '',
    UNIQUE(agent_id, container_name, label_key)
)`},
		{"copy rows", `INSERT INTO endpoints_new
			(id, agent_id, container_name, label_key, external_id, endpoint_type, target,
			 status, alert_state, consecutive_failures, consecutive_successes, last_check_at,
			 last_response_time_ms, last_http_status, last_error, config_json, active,
			 first_seen_at, last_seen_at, source, name)
			SELECT id, agent_id, container_name, label_key, external_id, endpoint_type, target,
			 status, alert_state, consecutive_failures, consecutive_successes, last_check_at,
			 last_response_time_ms, last_http_status, last_error, config_json, active,
			 first_seen_at, last_seen_at, source, name
			FROM endpoints`},
		{"drop old table", `DROP TABLE endpoints`},
		{"rename", `ALTER TABLE endpoints_new RENAME TO endpoints`},
		// DROP TABLE removed the old table's indexes — recreate them all.
		{"index external_id", `CREATE INDEX idx_endpoint_external_id ON endpoints(external_id)`},
		{"index status", `CREATE INDEX idx_endpoint_status ON endpoints(status) WHERE active=1`},
		{"index active", `CREATE INDEX idx_endpoint_active ON endpoints(active, last_seen_at DESC)`},
		{"index source", `CREATE INDEX idx_endpoint_source ON endpoints(source) WHERE active=1`},
		{"index agent_id", `CREATE INDEX idx_endpoints_agent_id ON endpoints(agent_id)`},
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("endpoint degraded rebuild: %s: %w", s.desc, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("endpoint degraded rebuild: commit: %w", err)
	}
	committed = true
	return nil
}

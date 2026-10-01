// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// escalationDeliveryStatusConstraint must stay byte-identical to the escalation_deliveries DDL in uuid_schema.sql: the rebuild guard matches on it.
const escalationDeliveryStatusConstraint = "CHECK(status IN ('pending','sent','failed','abandoned'))"

// rebuildEscalationDeliveriesStatusCheck drops 'skipped_maintenance' from the delivery status CHECK on databases converted before migration 38, which PostgreSQL applies as an ALTER that SQLite cannot run.
func rebuildEscalationDeliveriesStatusCheck(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	var ddl string
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='escalation_deliveries'`).Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("escalation delivery status rebuild: probe ddl: %w", err)
	}
	if strings.Contains(ddl, escalationDeliveryStatusConstraint) {
		return nil
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("escalation delivery status rebuild: acquire conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("escalation delivery status rebuild: fk off: %w", err)
	}

	logger.Info("escalation delivery status rebuild starting")

	if err := runEscalationDeliveryStatusRebuild(ctx, conn); err != nil {
		return err
	}

	if err := foreignKeyCheck(ctx, conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("escalation delivery status rebuild: fk on: %w", err)
	}

	logger.Info("escalation delivery status rebuild complete")
	return nil
}

func runEscalationDeliveryStatusRebuild(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("escalation delivery status rebuild: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	stmts := []stmt{
		{"create table", `CREATE TABLE escalation_deliveries_new (
    id            TEXT PRIMARY KEY NOT NULL,
    run_id        TEXT NOT NULL REFERENCES escalation_runs(id) ON DELETE CASCADE,
    level_index   INTEGER NOT NULL,
    channel_id    TEXT REFERENCES notification_channels(id) ON DELETE SET NULL,
    status        TEXT NOT NULL ` + escalationDeliveryStatusConstraint + `,
    error         TEXT,
    attempt_started_at BIGINT NOT NULL DEFAULT 0,
    sent_at       BIGINT
)`},
		{"copy rows", `INSERT INTO escalation_deliveries_new
			(id, run_id, level_index, channel_id, status, error, attempt_started_at, sent_at)
			SELECT id, run_id, level_index, channel_id, status, error, attempt_started_at, sent_at
			FROM escalation_deliveries`},
		{"drop old table", `DROP TABLE escalation_deliveries`},
		{"rename", `ALTER TABLE escalation_deliveries_new RENAME TO escalation_deliveries`},
		{"index run_level", `CREATE UNIQUE INDEX idx_escalation_deliveries_run_level ON escalation_deliveries(run_id, level_index, channel_id)`},
		{"index pending", `CREATE INDEX idx_escalation_deliveries_pending ON escalation_deliveries(status, attempt_started_at) WHERE status = 'pending'`},
		{"index run_id", `CREATE INDEX idx_escalation_deliveries_run_id ON escalation_deliveries(run_id)`},
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("escalation delivery status rebuild: %s: %w", s.desc, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("escalation delivery status rebuild: commit: %w", err)
	}
	committed = true
	return nil
}

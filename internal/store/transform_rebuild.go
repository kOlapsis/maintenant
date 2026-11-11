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

// tableRebuild is a one-time SQLite table rebuild, skipped once the stored CREATE TABLE contains marker.
type tableRebuild struct {
	name   string
	table  string
	marker string
	stmts  []stmt
	before func(ctx context.Context, conn *sql.Conn) error // runs before foreign keys are switched off
	after  func(ctx context.Context, conn *sql.Conn) error // runs after the foreign key check
}

// rebuildTable runs r with foreign keys off, all statements in one transaction, then checks the foreign keys.
func rebuildTable(ctx context.Context, db *sql.DB, logger *slog.Logger, r tableRebuild) error {
	var ddl string
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, r.table).Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: probe ddl: %w", r.name, err)
	}
	if strings.Contains(ddl, r.marker) {
		return nil
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("%s: acquire conn: %w", r.name, err)
	}
	defer func() { _ = conn.Close() }()

	if r.before != nil {
		if err := r.before(ctx, conn); err != nil {
			return fmt.Errorf("%s: %w", r.name, err)
		}
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("%s: fk off: %w", r.name, err)
	}

	logger.Info(r.name + " starting")

	if err := execStmtsInTx(ctx, conn, r.name, r.stmts); err != nil {
		return err
	}
	if err := foreignKeyCheck(ctx, conn); err != nil {
		return err
	}
	if r.after != nil {
		if err := r.after(ctx, conn); err != nil {
			return fmt.Errorf("%s: %w", r.name, err)
		}
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("%s: fk on: %w", r.name, err)
	}

	logger.Info(r.name + " complete")
	return nil
}

func execStmtsInTx(ctx context.Context, conn *sql.Conn, name string, stmts []stmt) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin: %w", name, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("%s: %s: %w", name, s.desc, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", name, err)
	}
	committed = true
	return nil
}

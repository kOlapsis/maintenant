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

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/mattn/go-sqlite3"
)

// enrollmentTokenHashColumn is the hashed-token column of enrollment_tokens. It
// must stay byte-identical to the DDL in uuid_schema.sql — SQLite stores the
// CREATE TABLE text verbatim and the idempotency guard matches on it.
const enrollmentTokenHashColumn = "token_hash    TEXT NOT NULL UNIQUE" // #nosec G101 -- a DDL fragment, not a credential.

// rebuildEnrollmentTokensForHashing replaces the cleartext `token` column with
// its SHA-256 plus a short display prefix, on databases converted to the UUID
// schema before the token was hashed.
//
// The cleartext used to sit in the file: any copy of the database (a backup, the
// data volume, a `.db` grabbed for debugging) handed over every unconsumed,
// unexpired token, each one directly replayable to enroll an agent. Hashing
// costs nothing at the lookups — enrollment already knows the cleartext the
// client just sent — and leaves nothing replayable at rest.
//
// Pending tokens survive: the hash is computed from the stored cleartext during
// the copy, so an operator who handed out a token yesterday does not have to
// reissue it. Once this has run the cleartext is gone for good, which is the
// point; the `down` direction does not exist.
//
// Runs after convertToUUID rather than as a numbered migration for the same
// reason as the other rebuilds: migrations execute against the legacy integer
// schema, where this table does not exist at all.
func rebuildEnrollmentTokensForHashing(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	var ddl string
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='enrollment_tokens'`).Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("enrollment token hash rebuild: probe ddl: %w", err)
	}
	if strings.Contains(ddl, enrollmentTokenHashColumn) {
		return nil
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("enrollment token hash rebuild: acquire conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// The copy hashes set-based rather than row-by-row in Go, so the whole
	// rewrite stays inside the one transaction below.
	if err := registerHashFunc(ctx, conn); err != nil {
		return fmt.Errorf("enrollment token hash rebuild: register func: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("enrollment token hash rebuild: fk off: %w", err)
	}

	// Dropping the old table marks its pages free without overwriting them, so
	// the cleartext could stay legible in the file's slack space. secure_delete
	// is SQLite's documented way to zero freed content instead.
	//
	// Precaution, not a guarantee: it cannot reach a copy of the file taken
	// before the upgrade, nor blocks a copy-on-write filesystem or an SSD's
	// wear-levelling still holds underneath. Revoking an outstanding token is
	// the only thing that fully closes it — their 7-day TTL does the rest.
	if _, err := conn.ExecContext(ctx, "PRAGMA secure_delete=ON"); err != nil {
		return fmt.Errorf("enrollment token hash rebuild: secure_delete on: %w", err)
	}

	logger.Info("enrollment token hashing rebuild starting")

	if err := runEnrollmentTokenHashRebuild(ctx, conn); err != nil {
		return err
	}

	if err := foreignKeyCheck(ctx, conn); err != nil {
		return err
	}
	// Return the freed pages to the file rather than leaving them on the
	// freelist. The database is auto_vacuum=incremental, so this is bounded
	// work rather than a full VACUUM rewrite.
	if _, err := conn.ExecContext(ctx, "PRAGMA incremental_vacuum"); err != nil {
		return fmt.Errorf("enrollment token hash rebuild: incremental_vacuum: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA secure_delete=OFF"); err != nil {
		return fmt.Errorf("enrollment token hash rebuild: secure_delete off: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("enrollment token hash rebuild: fk on: %w", err)
	}

	logger.Info("enrollment token hashing rebuild complete")
	return nil
}

// registerHashFunc exposes the token hash and prefix helpers to SQL so the copy
// below can be a single INSERT ... SELECT.
func registerHashFunc(ctx context.Context, conn *sql.Conn) error {
	return conn.Raw(func(driverConn any) error {
		c, ok := driverConn.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("not a sqlite3 conn: %T", driverConn)
		}
		if err := c.RegisterFunc("mnt_token_hash", agent.HashToken, true); err != nil {
			return err
		}
		return c.RegisterFunc("mnt_token_prefix", agent.TokenPrefix, true)
	})
}

// runEnrollmentTokenHashRebuild performs the table rebuild in one transaction.
func runEnrollmentTokenHashRebuild(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("enrollment token hash rebuild: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	stmts := []stmt{
		// Keep this CREATE TABLE byte-identical to uuid_schema.sql.
		{"create table", `CREATE TABLE enrollment_tokens_new (
    id            TEXT PRIMARY KEY NOT NULL,                -- hex(sha256(token))[:16]
    ` + enrollmentTokenHashColumn + `,                     -- hex(sha256(token)); the cleartext is never stored
    token_prefix  TEXT NOT NULL DEFAULT '',                 -- leading non-secret chars, display only
    created_at    BIGINT NOT NULL DEFAULT 0,
    expires_at    BIGINT NOT NULL,
    consumed_at   BIGINT,
    consumed_by_agent_id TEXT                               -- audit only, no FK (agent may not exist yet)
)`},
		// A duplicate hash cannot happen (the source column is UNIQUE and the
		// hash is injective over it), so the UNIQUE above stays satisfiable.
		{"copy rows", `INSERT INTO enrollment_tokens_new
			(id, token_hash, token_prefix, created_at, expires_at, consumed_at, consumed_by_agent_id)
			SELECT id, mnt_token_hash(token), mnt_token_prefix(token),
			       created_at, expires_at, consumed_at, consumed_by_agent_id
			FROM enrollment_tokens`},
		{"drop old table", `DROP TABLE enrollment_tokens`},
		{"rename", `ALTER TABLE enrollment_tokens_new RENAME TO enrollment_tokens`},
		// DROP TABLE removed the old table's index — recreate it.
		{"index expires_at", `CREATE INDEX idx_enrollment_tokens_expires_at ON enrollment_tokens(expires_at)`},
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("enrollment token hash rebuild: %s: %w", s.desc, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("enrollment token hash rebuild: commit: %w", err)
	}
	committed = true
	return nil
}

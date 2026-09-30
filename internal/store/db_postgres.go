// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver named "pgx"
)

// minPostgresVersionNum is PostgreSQL 14, the oldest release still supported
// by its publisher, as server_version_num reports it.
const minPostgresVersionNum = 140000

const openPingTimeout = 10 * time.Second

// OpenPostgres opens the operator-supplied PostgreSQL database. Failures are
// classified into the distinct startup families (FR-018) and never echo the
// connection string (FR-021). There is no fallback: a configured but unusable
// database refuses to start (FR-004).
func OpenPostgres(ctx context.Context, dsn string, logger *slog.Logger) (*DB, error) {
	if _, err := ParseDSN(dsn); err != nil {
		return nil, err
	}
	dsn = ApplyDefaultSSLMode(dsn)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidDSN, RedactDSN(dsn))
	}
	// The engine governs concurrency; the pool just needs sane bounds. Capped
	// lifetimes let a managed-provider failover resolve itself as connections
	// renew instead of pinning dead backends.
	db.SetMaxOpenConns(16)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, openPingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, classifyOpenError(ctx, err)
	}

	var versionNum int
	if err := db.QueryRowContext(ctx, "SHOW server_version_num").Scan(&versionNum); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("read server version: %w", classifyOpenError(ctx, err))
	}
	if err := checkServerVersion(versionNum); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &DB{
		db:      db,
		writer:  NewWriter(db, DialectPostgres, logger),
		logger:  logger,
		dialect: DialectPostgres,
		dsn:     dsn,
	}, nil
}

// migrationLockKey is the advisory lock instances take for their whole
// migration sequence. golang-migrate takes one of its own, but only around the
// apply: the version read and the dirty recovery that precede it sit outside
// it, so without this an instance starting while another migrates reads that
// instance's in-flight dirty flag, mistakes it for a crash, and rewinds the
// version under it. The key sits above the uint32 range golang-migrate derives
// its own ids in, so the two can never collide.
const migrationLockKey int64 = 0x6D61696E74656E00 // "maintena"

// lockMigrations blocks until no other instance is migrating and returns the
// release. The lock is session-scoped: an instance that dies holding it
// releases it as PostgreSQL drops its connection, so nothing has to be
// unstuck by hand.
func (d *DB) lockMigrations(ctx context.Context) (func(), error) {
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return func() {
		// Deliberately not ctx: a startup cancelled mid-migration must still
		// release, or the pooled session carries the lock until the process
		// exits and the next instance waits on nothing.
		release := context.WithoutCancel(ctx)
		if _, err := conn.ExecContext(release, "SELECT pg_advisory_unlock($1)", migrationLockKey); err != nil {
			d.logger.Warn("release migration lock", "error", err)
		}
		_ = conn.Close()
	}, nil
}

// checkServerVersion refuses engines older than the supported minimum, naming
// the version required (FR-018).
func checkServerVersion(versionNum int) error {
	if versionNum < minPostgresVersionNum {
		return fmt.Errorf("%w: server reports %d", ErrUnsupportedVersion, versionNum)
	}
	return nil
}

// classifyOpenError maps a connection failure onto the startup sentinels so
// the operator can tell refused credentials from an unreachable host. The
// original error is kept in the chain; pgx never puts the password in it.
func classifyOpenError(ctx context.Context, err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		// 28P01 invalid_password, 28000 invalid_authorization_specification.
		if pe.Code == "28P01" || pe.Code == "28000" {
			return fmt.Errorf("%w: %s", ErrAuthRefused, pe.Message)
		}
	}
	if tlsRefused(ctx, err) {
		return fmt.Errorf("%w: %v", ErrTLSRefused, err)
	}
	return fmt.Errorf("%w: %v", ErrUnreachable, err)
}

const (
	sslRequestCode  = 80877103
	tlsProbeTimeout = 3 * time.Second
)

// tlsRefused reports whether err is a connection that had to use TLS failing on a server that declines TLS.
func tlsRefused(ctx context.Context, err error) bool {
	var ce *pgconn.ConnectError
	if !errors.As(err, &ce) || ce.Config == nil || !requiresTLS(ce.Config) {
		return false
	}
	// pgx reports the refusal as a bare string, so the server is asked again.
	return declinesTLS(ctx, ce.Config.Host, ce.Config.Port)
}

func requiresTLS(cfg *pgconn.Config) bool {
	if cfg.TLSConfig == nil {
		return false
	}
	for _, fb := range cfg.Fallbacks {
		if fb.TLSConfig == nil {
			return false
		}
	}
	return true
}

// declinesTLS sends the protocol's SSLRequest, and nothing else, and reports whether the server answers 'N'.
func declinesTLS(ctx context.Context, host string, port uint16) bool {
	ctx, cancel := context.WithTimeout(ctx, tlsProbeTimeout)
	defer cancel()

	network, address := pgconn.NetworkAddress(host, port)
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, address)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if err := binary.Write(conn, binary.BigEndian, []int32{8, sslRequestCode}); err != nil {
		return false
	}
	var answer [1]byte
	if _, err := io.ReadFull(conn, answer[:]); err != nil {
		return false
	}
	return answer[0] == 'N'
}

// RedactedDSN exposes the connection target without its credentials, for the
// startup log line and error messages.
func (d *DB) RedactedDSN() string {
	if d.dsn == "" {
		return ""
	}
	return RedactDSN(d.dsn)
}

// Host returns the database host for the startup log line; empty on SQLite.
func (d *DB) Host() string {
	if d.dsn == "" {
		return ""
	}
	return DSNHost(d.dsn)
}

// Database returns the database name for the startup log line; empty on SQLite.
func (d *DB) Database() string {
	if d.dsn == "" {
		return ""
	}
	return DSNDatabase(d.dsn)
}

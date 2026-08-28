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
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mattn/go-sqlite3"
)

// Startup sentinels. Each maps to one of the distinct operator-facing failure
// families (FR-018): the message the operator reads names the cause, never the
// connection string.
var (
	// ErrInvalidDSN: the connection string cannot be parsed or uses an
	// unsupported scheme.
	ErrInvalidDSN = errors.New("invalid database connection string")
	// ErrUnreachable: the database host does not answer.
	ErrUnreachable = errors.New("database unreachable")
	// ErrAuthRefused: the database answered and refused the credentials.
	ErrAuthRefused = errors.New("database credentials refused")
	// ErrUnsupportedVersion: the server runs a version older than the minimum.
	ErrUnsupportedVersion = errors.New("database version unsupported (PostgreSQL 14 or newer required)")
	// ErrSchemaNewer: the schema was written by a newer release of this binary.
	ErrSchemaNewer = errors.New("database schema is newer than this binary")
)

// IsUniqueViolation reports whether err is a unique or primary-key constraint
// violation, on either engine.
func IsUniqueViolation(err error) bool {
	var se sqlite3.Error
	if errors.As(err, &se) {
		return se.ExtendedCode == sqlite3.ErrConstraintUnique ||
			se.ExtendedCode == sqlite3.ErrConstraintPrimaryKey
	}
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// IsForeignKeyViolation reports whether err is a foreign-key constraint
// violation, on either engine.
func IsForeignKeyViolation(err error) bool {
	var se sqlite3.Error
	if errors.As(err, &se) {
		return se.ExtendedCode == sqlite3.ErrConstraintForeignKey
	}
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23503"
}

// IsBusy reports whether err is a transient contention failure worth retrying:
// SQLITE_BUSY/SQLITE_LOCKED, a PostgreSQL lock timeout (55P03) or a
// serialization failure (40001).
func IsBusy(err error) bool {
	var se sqlite3.Error
	if errors.As(err, &se) {
		return se.Code == sqlite3.ErrBusy || se.Code == sqlite3.ErrLocked
	}
	var pe *pgconn.PgError
	return errors.As(err, &pe) && (pe.Code == "55P03" || pe.Code == "40001")
}

// IsUnavailable reports whether err means the storage cannot serve requests
// right now but may recover on its own: the connection could not be made or
// was dropped mid-flight, a PostgreSQL resource-exhaustion error (class 53:
// disk full, too many connections), a shutdown in progress (57P01..57P03,
// seen during managed-provider failovers), or SQLITE_FULL on the local file.
//
// It is what separates an outage the operator should wait out from a mistake
// the caller made: the API answers 503 on the former and 500 on the latter.
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var se sqlite3.Error
	if errors.As(err, &se) {
		return se.Code == sqlite3.ErrFull
	}
	// A server-side error carries a class; only some classes are outages.
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return strings.HasPrefix(pe.Code, "53") ||
			pe.Code == "57P01" || pe.Code == "57P02" || pe.Code == "57P03"
	}
	// The connection could not be established at all.
	var ce *pgconn.ConnectError
	if errors.As(err, &ce) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	// A connection dropped mid-flight surfaces as an I/O error: the driver
	// read half a message, or the peer reset the socket.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	return errors.Is(err, driver.ErrBadConn) || pgconn.SafeToRetry(err)
}

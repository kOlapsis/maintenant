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
	"fmt"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func sqliteErr(code sqlite3.ErrNo, ext sqlite3.ErrNoExtended) error {
	return fmt.Errorf("wrapped: %w", sqlite3.Error{Code: code, ExtendedCode: ext})
}

func pgErr(code string) error {
	return fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: code})
}

type fakeNetError struct{}

func (fakeNetError) Error() string   { return "connection refused" }
func (fakeNetError) Timeout() bool   { return false }
func (fakeNetError) Temporary() bool { return true }

var _ net.Error = fakeNetError{}

func TestIsUniqueViolation(t *testing.T) {
	assert.True(t, IsUniqueViolation(sqliteErr(sqlite3.ErrConstraint, sqlite3.ErrConstraintUnique)))
	assert.True(t, IsUniqueViolation(sqliteErr(sqlite3.ErrConstraint, sqlite3.ErrConstraintPrimaryKey)))
	assert.True(t, IsUniqueViolation(pgErr("23505")))
	assert.False(t, IsUniqueViolation(sqliteErr(sqlite3.ErrConstraint, sqlite3.ErrConstraintForeignKey)))
	assert.False(t, IsUniqueViolation(pgErr("23503")))
	assert.False(t, IsUniqueViolation(errors.New("UNIQUE constraint failed: not a typed error")))
	assert.False(t, IsUniqueViolation(nil))
}

func TestIsForeignKeyViolation(t *testing.T) {
	assert.True(t, IsForeignKeyViolation(sqliteErr(sqlite3.ErrConstraint, sqlite3.ErrConstraintForeignKey)))
	assert.True(t, IsForeignKeyViolation(pgErr("23503")))
	assert.False(t, IsForeignKeyViolation(sqliteErr(sqlite3.ErrConstraint, sqlite3.ErrConstraintUnique)))
	assert.False(t, IsForeignKeyViolation(pgErr("23505")))
	assert.False(t, IsForeignKeyViolation(nil))
}

func TestIsBusy(t *testing.T) {
	assert.True(t, IsBusy(sqliteErr(sqlite3.ErrBusy, 0)))
	assert.True(t, IsBusy(sqliteErr(sqlite3.ErrLocked, 0)))
	assert.True(t, IsBusy(pgErr("55P03")))
	assert.True(t, IsBusy(pgErr("40001")))
	assert.False(t, IsBusy(pgErr("23505")))
	assert.False(t, IsBusy(nil))
}

func TestIsUnavailable(t *testing.T) {
	// PostgreSQL class 53: disk full, too many connections.
	assert.True(t, IsUnavailable(pgErr("53100")))
	assert.True(t, IsUnavailable(pgErr("53300")))
	// Shutdown in progress, seen during managed-provider failovers.
	assert.True(t, IsUnavailable(pgErr("57P01")))
	assert.True(t, IsUnavailable(pgErr("57P03")))
	// Network and driver-level failures.
	assert.True(t, IsUnavailable(fmt.Errorf("query: %w", fakeNetError{})))
	assert.True(t, IsUnavailable(fmt.Errorf("exec: %w", driver.ErrBadConn)))
	assert.True(t, IsUnavailable(&net.OpError{Op: "dial", Err: errors.New("refused")}))
	// SQLite: only a full disk qualifies.
	assert.True(t, IsUnavailable(sqliteErr(sqlite3.ErrFull, 0)))
	assert.False(t, IsUnavailable(sqliteErr(sqlite3.ErrBusy, 0)))
	// Constraint violations are the caller's problem, not an outage.
	assert.False(t, IsUnavailable(pgErr("23505")))
	assert.False(t, IsUnavailable(nil))
}

// TestSentinelsCarryNoSecret pins that no startup sentinel can ever leak a
// credential: their messages are constants.
func TestSentinelsCarryNoSecret(t *testing.T) {
	for _, err := range []error{ErrInvalidDSN, ErrUnreachable, ErrAuthRefused, ErrUnsupportedVersion, ErrSchemaNewer} {
		assert.NotContains(t, err.Error(), "://")
		assert.NotContains(t, err.Error(), "password")
	}
}

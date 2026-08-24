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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRebindPostgres(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no placeholder", "SELECT 1", "SELECT 1"},
		{"simple", "SELECT * FROM t WHERE a = ? AND b = ?", "SELECT * FROM t WHERE a = $1 AND b = $2"},
		{"question mark in string literal", "SELECT '?' , a FROM t WHERE b = ?", "SELECT '?' , a FROM t WHERE b = $1"},
		{"escaped quote in literal", "SELECT 'it''s ?' FROM t WHERE a = ?", "SELECT 'it''s ?' FROM t WHERE a = $1"},
		{"quoted identifier", `SELECT "col?umn" FROM t WHERE a = ?`, `SELECT "col?umn" FROM t WHERE a = $1`},
		{"line comment", "SELECT a -- what?\nFROM t WHERE b = ?", "SELECT a -- what?\nFROM t WHERE b = $1"},
		{"line comment at end", "SELECT a FROM t WHERE b = ? -- really?", "SELECT a FROM t WHERE b = $1 -- really?"},
		{"block comment", "SELECT a /* ? mystery ? */ FROM t WHERE b = ?", "SELECT a /* ? mystery ? */ FROM t WHERE b = $1"},
		{"double question mark kept", "SELECT a ?? b, c FROM t WHERE d = ?", "SELECT a ?? b, c FROM t WHERE d = $1"},
		{"unterminated literal copied verbatim", "SELECT 'oops ?", "SELECT 'oops ?"},
		{"unterminated block comment copied verbatim", "SELECT a /* ?", "SELECT a /* ?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, rebindPostgres(tc.in))
		})
	}
}

// TestRebindPostgres_ManyParams pins ordinal numbering over the whole range the
// codebase actually binds (~1100 parameters).
func TestRebindPostgres_ManyParams(t *testing.T) {
	const n = 1100
	in := "INSERT INTO t VALUES (" + strings.TrimSuffix(strings.Repeat("?,", n), ",") + ")"
	out := rebindPostgres(in)
	assert.True(t, strings.HasPrefix(out, "INSERT INTO t VALUES ($1,$2,"))
	assert.Contains(t, out, fmt.Sprintf("$%d)", n))
	assert.NotContains(t, out, "?")
}

// TestRebindSQLite_NoAllocation pins the identity fast path: the SQLite
// dialect must return the query untouched without allocating.
func TestRebindSQLite_NoAllocation(t *testing.T) {
	q := "SELECT * FROM containers WHERE agent_id = ? AND status = ?"
	allocs := testing.AllocsPerRun(100, func() {
		if DialectSQLite.Rebind(q) != q {
			t.Fatal("SQLite rebind must be the identity")
		}
	})
	assert.Zero(t, allocs)
}

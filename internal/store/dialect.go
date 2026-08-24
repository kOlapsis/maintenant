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

import "fmt"

// Dialect selects the engine-specific form at the few places where SQLite and
// PostgreSQL genuinely differ. Every engine variation in this package MUST go
// through a Dialect method — never an inline engine check in a query file.
//
// The six variation points, and nothing else:
//   - placeholder syntax (Rebind)
//   - batched deletes (BatchDeleteSQL)
//   - opening PRAGMAs (db.go, SQLite path only)
//   - error classification (errors.go)
//   - write serialization (writer.go: single goroutine on SQLite, pool on PG)
//   - UUID generation in SQL for set-based rollups (UUIDExpr)
//
// Dialect-specific findings from running the shared suite on both engines:
//   - PostgreSQL requires every non-aggregated SELECT column in GROUP BY.
//   - Booleans bound as Go bool map to INTEGER 0/1 columns; bind ints.
//   - BLOB columns read back as BYTEA; []byte scans work on both.
type Dialect int

const (
	DialectSQLite Dialect = iota
	DialectPostgres
)

func (d Dialect) String() string {
	switch d {
	case DialectPostgres:
		return "postgres"
	default:
		return "sqlite"
	}
}

// Rebind converts `?` placeholders to the engine's native form. On SQLite it
// is the identity and performs no allocation.
func (d Dialect) Rebind(query string) string {
	if d != DialectPostgres {
		return query
	}
	return rebindPostgres(query)
}

// UUIDExpr is a SQL expression minting a well-formed UUID string per row, for
// set-based INSERT ... SELECT statements that cannot mint ids in Go.
func (d Dialect) UUIDExpr() string {
	if d == DialectPostgres {
		return `gen_random_uuid()::text`
	}
	return sqliteUUIDExpr
}

// sqliteUUIDExpr builds a v7-shaped random UUID from randomblob bytes.
const sqliteUUIDExpr = `lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || ` +
	`'7' || substr(lower(hex(randomblob(2))), 2) || '-' || ` +
	`substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' || ` +
	`lower(hex(randomblob(6)))`

// BatchDeleteSQL returns the engine's form of "delete at most ? rows of table
// matching where". The where fragment uses `?` placeholders; the batch size is
// always the statement's last parameter. SQLite deletes by rowid so the
// subquery is served by the index on the filtered column alone; PostgreSQL has
// no rowid nor DELETE ... LIMIT, so it goes through the primary key.
func (d Dialect) BatchDeleteSQL(table, pk, where string) string {
	if d == DialectPostgres {
		return fmt.Sprintf(
			`DELETE FROM %s WHERE %s IN (SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT ?)`,
			table, pk, pk, table, where, pk)
	}
	return fmt.Sprintf(
		`DELETE FROM %s WHERE rowid IN (SELECT rowid FROM %s WHERE %s LIMIT ?)`,
		table, table, where)
}

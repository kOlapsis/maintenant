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
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test is what makes the shared-migration rule tenable (R4): it migrates
// a fresh database on each engine, introspects both heads and requires exact
// equality of tables, columns (type, nullability, default), indexes
// (columns, uniqueness), foreign keys (target, ON DELETE) and CHECK counts.
// Any migration written for one engine only fails here, at write time.

type colDesc struct {
	Type    string
	NotNull bool
	Default string
}

type idxDesc struct {
	Table   string
	Columns string
	Unique  bool
}

type fkDesc struct {
	Table    string
	From     string
	RefTable string
	RefCol   string
	OnDelete string
}

type schemaDesc struct {
	tables  map[string]map[string]colDesc // table -> column -> desc
	indexes map[idxDesc]bool
	fks     map[fkDesc]bool
	checks  map[string]int // table -> CHECK constraint count
}

func TestSchemaHeads_Equivalent(t *testing.T) {
	pg := requirePostgres(t)
	lite := openTestSQLite(t)

	pgDesc := introspectPostgres(t, pg.ReadDB())
	liteDesc := introspectSQLite(t, lite.ReadDB())

	// Tables.
	assert.Equal(t, sortedKeys(liteDesc.tables), sortedKeys(pgDesc.tables), "table sets differ")

	// Columns, per table.
	for table, liteCols := range liteDesc.tables {
		pgCols, ok := pgDesc.tables[table]
		if !ok {
			continue // already reported by the table-set assertion
		}
		assert.Equal(t, sortedKeys(liteCols), sortedKeys(pgCols), "column sets differ on %s", table)
		for col, ld := range liteCols {
			pd, ok := pgCols[col]
			if !ok {
				continue
			}
			assert.Equal(t, ld, pd, "column %s.%s differs", table, col)
		}
	}

	// Indexes: same set of (table, columns, unique), names ignored.
	for _, idx := range sortedIdx(liteDesc.indexes) {
		assert.True(t, pgDesc.indexes[idx], "index missing on postgres: %+v", idx)
	}
	for _, idx := range sortedIdx(pgDesc.indexes) {
		assert.True(t, liteDesc.indexes[idx], "index missing on sqlite: %+v", idx)
	}

	// Foreign keys.
	assert.Equal(t, sortedFks(liteDesc.fks), sortedFks(pgDesc.fks), "foreign key sets differ")

	// CHECK constraints, by count per table (SQLite cannot introspect their
	// expressions without parsing DDL).
	assert.Equal(t, liteDesc.checks, pgDesc.checks, "CHECK constraint counts differ")
}

// ---- normalization --------------------------------------------------------

func normType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "real":
		return "double precision"
	case "blob":
		return "bytea"
	}
	return s
}

var pgCastSuffix = regexp.MustCompile(`::[a-z ]+$`)

// checkConstraintRe counts real CHECK constraints in SQLite DDL without
// matching column names like check_result_id or comments.
var checkConstraintRe = regexp.MustCompile(`(?i)\bCHECK\s*\(`)

// quotedNumberRe unwraps PostgreSQL's quoting of numeric defaults ('-1' vs -1).
var quotedNumberRe = regexp.MustCompile(`^'(-?[0-9.]+)'$`)

func normDefault(raw sql.NullString) string {
	if !raw.Valid {
		return ""
	}
	s := pgCastSuffix.ReplaceAllString(strings.TrimSpace(raw.String), "")
	if m := quotedNumberRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

func normOnDelete(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return "NO ACTION"
	}
	return s
}

// ---- SQLite ---------------------------------------------------------------

func introspectSQLite(t *testing.T, db *sql.DB) schemaDesc {
	t.Helper()
	ctx := context.Background()
	d := schemaDesc{tables: map[string]map[string]colDesc{}, indexes: map[idxDesc]bool{}, fks: map[fkDesc]bool{}, checks: map[string]int{}}

	rows, err := db.QueryContext(ctx, `SELECT name, sql FROM sqlite_master WHERE type='table'
		AND name NOT LIKE 'sqlite_%' AND name NOT IN ('schema_migrations', 'schema_meta')`)
	require.NoError(t, err)
	ddl := map[string]string{}
	for rows.Next() {
		var name, sqlText string
		require.NoError(t, rows.Scan(&name, &sqlText))
		d.tables[name] = map[string]colDesc{}
		ddl[name] = sqlText
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	for table, sqlText := range ddl {
		d.checks[table] = len(checkConstraintRe.FindAllString(sqlText, -1))

		cols, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", table))
		require.NoError(t, err)
		for cols.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt sql.NullString
			require.NoError(t, cols.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk))
			d.tables[table][name] = colDesc{
				Type:    normType(ctype),
				NotNull: notnull == 1 || pk > 0,
				Default: normDefault(dflt),
			}
		}
		require.NoError(t, cols.Err())
		require.NoError(t, cols.Close())

		idxs, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%q)", table))
		require.NoError(t, err)
		type idxRow struct {
			name   string
			unique bool
			origin string
		}
		var list []idxRow
		for idxs.Next() {
			var seq int
			var name, origin string
			var unique, partial int
			require.NoError(t, idxs.Scan(&seq, &name, &unique, &origin, &partial))
			list = append(list, idxRow{name, unique == 1, origin})
		}
		require.NoError(t, idxs.Err())
		require.NoError(t, idxs.Close())
		for _, ir := range list {
			if ir.origin == "pk" {
				continue // the PK index is implicit on both engines
			}
			cols := indexColumnsSQLite(t, db, ir.name)
			d.indexes[idxDesc{Table: table, Columns: cols, Unique: ir.unique}] = true
		}

		fks, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%q)", table))
		require.NoError(t, err)
		for fks.Next() {
			var id, seq int
			var refTable, from, onUpdate, onDelete, match string
			var to sql.NullString
			require.NoError(t, fks.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &match))
			d.fks[fkDesc{Table: table, From: from, RefTable: refTable, RefCol: to.String, OnDelete: normOnDelete(onDelete)}] = true
		}
		require.NoError(t, fks.Err())
		require.NoError(t, fks.Close())
	}
	return d
}

func indexColumnsSQLite(t *testing.T, db *sql.DB, index string) string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), fmt.Sprintf("PRAGMA index_info(%q)", index))
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	type c struct {
		seq  int
		name string
	}
	var cs []c
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		require.NoError(t, rows.Scan(&seqno, &cid, &name))
		cs = append(cs, c{seqno, name.String})
	}
	require.NoError(t, rows.Err())
	sort.Slice(cs, func(i, j int) bool { return cs[i].seq < cs[j].seq })
	names := make([]string, len(cs))
	for i, cc := range cs {
		names[i] = cc.name
	}
	return strings.Join(names, ",")
}

// ---- PostgreSQL -----------------------------------------------------------

func introspectPostgres(t *testing.T, db *sql.DB) schemaDesc {
	t.Helper()
	ctx := context.Background()
	d := schemaDesc{tables: map[string]map[string]colDesc{}, indexes: map[idxDesc]bool{}, fks: map[fkDesc]bool{}, checks: map[string]int{}}

	rows, err := db.QueryContext(ctx, `SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE' AND table_name != 'schema_migrations'`)
	require.NoError(t, err)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		d.tables[name] = map[string]colDesc{}
		d.checks[name] = 0
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	cols, err := db.QueryContext(ctx, `SELECT table_name, column_name, data_type, is_nullable, column_default
		FROM information_schema.columns WHERE table_schema = 'public' AND table_name != 'schema_migrations'`)
	require.NoError(t, err)
	for cols.Next() {
		var table, col, ctype, nullable string
		var dflt sql.NullString
		require.NoError(t, cols.Scan(&table, &col, &ctype, &nullable, &dflt))
		if _, ok := d.tables[table]; !ok {
			continue
		}
		d.tables[table][col] = colDesc{
			Type:    normType(ctype),
			NotNull: nullable == "NO",
			Default: normDefault(dflt),
		}
	}
	require.NoError(t, cols.Err())
	require.NoError(t, cols.Close())

	idxs, err := db.QueryContext(ctx, `
		SELECT tc.relname AS table_name, ix.indisunique,
		       string_agg(a.attname, ',' ORDER BY k.ord)
		FROM pg_index ix
		JOIN pg_class tc ON tc.oid = ix.indrelid
		JOIN pg_namespace n ON n.oid = tc.relnamespace
		JOIN LATERAL unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord) ON true
		JOIN pg_attribute a ON a.attrelid = tc.oid AND a.attnum = k.attnum
		WHERE n.nspname = 'public' AND NOT ix.indisprimary AND tc.relname != 'schema_migrations'
		GROUP BY tc.relname, ix.indexrelid, ix.indisunique`)
	require.NoError(t, err)
	for idxs.Next() {
		var table, columns string
		var unique bool
		require.NoError(t, idxs.Scan(&table, &unique, &columns))
		d.indexes[idxDesc{Table: table, Columns: columns, Unique: unique}] = true
	}
	require.NoError(t, idxs.Err())
	require.NoError(t, idxs.Close())

	fks, err := db.QueryContext(ctx, `
		SELECT tc.relname, a.attname, rt.relname, ra.attname,
		       CASE c.confdeltype WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN 'SET NULL'
		            WHEN 'r' THEN 'RESTRICT' WHEN 'd' THEN 'SET DEFAULT' ELSE 'NO ACTION' END
		FROM pg_constraint c
		JOIN pg_class tc ON tc.oid = c.conrelid
		JOIN pg_class rt ON rt.oid = c.confrelid
		JOIN pg_namespace n ON n.oid = tc.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		JOIN pg_attribute ra ON ra.attrelid = c.confrelid AND ra.attnum = c.confkey[1]
		WHERE n.nspname = 'public' AND c.contype = 'f'`)
	require.NoError(t, err)
	for fks.Next() {
		var f fkDesc
		require.NoError(t, fks.Scan(&f.Table, &f.From, &f.RefTable, &f.RefCol, &f.OnDelete))
		d.fks[f] = true
	}
	require.NoError(t, fks.Err())
	require.NoError(t, fks.Close())

	checks, err := db.QueryContext(ctx, `
		SELECT tc.relname, count(*)
		FROM pg_constraint c
		JOIN pg_class tc ON tc.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = tc.relnamespace
		WHERE n.nspname = 'public' AND c.contype = 'c'
		GROUP BY tc.relname`)
	require.NoError(t, err)
	for checks.Next() {
		var table string
		var count int
		require.NoError(t, checks.Scan(&table, &count))
		if _, ok := d.checks[table]; ok {
			d.checks[table] = count
		}
	}
	require.NoError(t, checks.Err())
	require.NoError(t, checks.Close())
	return d
}

// ---- helpers ---------------------------------------------------------------

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedIdx(m map[idxDesc]bool) []idxDesc {
	out := make([]idxDesc, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Table != out[j].Table {
			return out[i].Table < out[j].Table
		}
		return out[i].Columns < out[j].Columns
	})
	return out
}

func sortedFks(m map[fkDesc]bool) []fkDesc {
	out := make([]fkDesc, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Table != out[j].Table {
			return out[i].Table < out[j].Table
		}
		return out[i].From < out[j].From
	})
	return out
}

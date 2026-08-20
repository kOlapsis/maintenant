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
	"testing"

	"github.com/stretchr/testify/assert"
)

// The dialect layer must cost the SQLite path nothing (FR-029, SC-003): an
// install that asked for none of this must not pay for it. These benchmarks
// are the measurement; the allocation assertions are the gate that fails
// without anyone having to read a benchstat report.

const benchQuery = `SELECT id, container_id, cpu_percent, timestamp FROM resource_snapshots
	WHERE container_id = ? AND timestamp >= ? AND timestamp < ? ORDER BY timestamp DESC LIMIT ?`

func BenchmarkRebindSQLite(b *testing.B) {
	d := DialectSQLite
	b.ReportAllocs()
	for b.Loop() {
		_ = d.Rebind(benchQuery)
	}
}

func BenchmarkRebindPostgres(b *testing.B) {
	d := DialectPostgres
	b.ReportAllocs()
	for b.Loop() {
		_ = d.Rebind(benchQuery)
	}
}

// TestRebindSQLite_CostsNothing is the gate behind BenchmarkRebindSQLite: on
// the SQLite path Rebind is the identity and allocates nothing, so no query
// pays for the PostgreSQL support.
func TestRebindSQLite_CostsNothing(t *testing.T) {
	allocs := testing.AllocsPerRun(1000, func() {
		if DialectSQLite.Rebind(benchQuery) != benchQuery {
			t.Fatal("SQLite Rebind must return the query untouched")
		}
	})
	assert.Zero(t, allocs, "the SQLite read/write path must allocate nothing for the dialect")
}

// TestWriterExecSQLite_NoDialectAllocation pins the same property one level
// up: a write through the serialized writer carries no dialect cost beyond
// what it already had.
func TestWriterExecSQLite_NoDialectAllocation(t *testing.T) {
	requireSQLite(t)
	db := openTestDB(t)
	w := db.Writer()

	const q = `INSERT INTO instances (id, hostname, version, started_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET last_seen_at = excluded.last_seen_at`

	// Only the rebind step is measured: the rest of Exec talks to SQLite and
	// allocates for reasons that predate this feature.
	allocs := testing.AllocsPerRun(1000, func() {
		_ = w.dialect.Rebind(q)
	})
	assert.Zero(t, allocs)

	// And the write itself still works, so the measurement is not on a dead path.
	_, err := w.Exec(context.Background(), q, "cost-test", "h", "v", 1, 1)
	assert.NoError(t, err)
}

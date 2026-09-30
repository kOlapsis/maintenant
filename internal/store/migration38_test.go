// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration38_OverrideMemoryColumn(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	hasColumn := func() bool {
		t.Helper()
		desc := introspectSQLite
		if db.dialect == DialectPostgres {
			desc = introspectPostgres
		}
		_, ok := desc(t, db.ReadDB()).tables["status_components"]["override_before_maintenance"]
		return ok
	}
	apply := func(direction string) {
		t.Helper()
		sqlText, err := fs.ReadFile(migrationFS,
			"migrations/"+db.dialect.String()+"/38_override_memory_and_delivery_status."+direction+".sql")
		require.NoError(t, err)
		_, err = db.ReadDB().ExecContext(ctx, string(sqlText))
		require.NoError(t, err, direction)
	}

	assert.True(t, hasColumn())
	apply("down")
	assert.False(t, hasColumn())
	apply("up")
	assert.True(t, hasColumn())
}

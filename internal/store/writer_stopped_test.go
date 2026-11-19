// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUnavailable_StoppedWriter(t *testing.T) {
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })

	w := NewWriter(rawDB, DialectSQLite, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	cancel()
	<-w.stopped

	_, err = w.Exec(context.Background(), "SELECT 1")
	require.Error(t, err)
	assert.True(t, IsUnavailable(err), "a write refused by a stopped writer may succeed once the server is back")
}

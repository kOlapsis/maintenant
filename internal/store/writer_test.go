// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriter_StopReleasesWritesInFlight stops the writer under writes whose own
// context never ends: each must return, and a write after the stop is refused.
func TestWriter_StopReleasesWritesInFlight(t *testing.T) {
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	rawDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = rawDB.Close() })
	_, err = rawDB.Exec("CREATE TABLE w (v INTEGER)")
	require.NoError(t, err)

	w := NewWriter(rawDB, DialectSQLite, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	const writers = 8
	var written atomic.Int64
	returned := make(chan struct{}, writers)
	for range writers {
		go func() {
			defer func() { returned <- struct{}{} }()
			for {
				if _, err := w.Exec(context.Background(), "INSERT INTO w (v) VALUES (1)"); err != nil {
					return
				}
				written.Add(1)
			}
		}()
	}
	require.Eventually(t, func() bool { return written.Load() > 100 }, 5*time.Second, time.Millisecond)
	cancel()

	for range writers {
		select {
		case <-returned:
		case <-time.After(5 * time.Second):
			t.Fatal("a write queued before the stop never returned")
		}
	}
	_, err = w.Exec(context.Background(), "INSERT INTO w (v) VALUES (1)")
	assert.ErrorIs(t, err, errWriterStopped)
}

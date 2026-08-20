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

package app

import (
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStorageState_ServesFreshAnswers pins the property the health diagnostic
// depends on: Connected never asserts a state it cannot back. While a recent
// answer stands it is served as-is; once it goes stale the state is probed
// again rather than assumed.
func TestStorageState_ServesFreshAnswers(t *testing.T) {
	s := &storageState{engine: "postgres"}

	// A fresh answer is served without probing. db is nil here, so a probe
	// would be visible: it would return the stored value unchanged anyway,
	// but the stamp would move.
	s.connected.Store(true)
	stamp := time.Now().UnixNano()
	s.probedAtUnixNano.Store(stamp)
	assert.True(t, s.Connected())
	assert.Equal(t, stamp, s.probedAtUnixNano.Load(), "a fresh answer is not re-probed")

	// A stale answer is re-probed. With no database handle the probe keeps the
	// last known value rather than inventing one.
	s.probedAtUnixNano.Store(time.Now().Add(-time.Hour).UnixNano())
	assert.True(t, s.Connected(), "no handle means the last known answer stands")
}

// TestStorageState_ProbeRecordsOutcome covers both directions of the flag the
// supervisor and the health endpoint share.
func TestStorageState_ProbeRecordsOutcome(t *testing.T) {
	db := openSQLiteForTest(t)
	s := &storageState{engine: db.Engine(), db: db}

	assert.True(t, s.probe(t.Context()), "an open database answers")
	assert.True(t, s.connected.Load())
	assert.NotZero(t, s.probedAtUnixNano.Load(), "the answer is stamped")

	require.NoError(t, db.Close())

	assert.False(t, s.probe(t.Context()), "a closed database does not")
	assert.False(t, s.connected.Load())
	assert.False(t, s.Connected(), "and the health view says so")
}

// TestStorageState_PeersAreReported pins FR-012's read side: the count the
// health diagnostic serves is the one the heartbeat measured.
func TestStorageState_PeersAreReported(t *testing.T) {
	s := &storageState{engine: "postgres"}
	assert.Equal(t, 0, s.Peers(), "alone by default")

	s.peers.Store(2)
	assert.Equal(t, 2, s.Peers())
}

// openSQLiteForTest opens a throwaway SQLite database through the same path
// the application uses.
func openSQLiteForTest(t *testing.T) *store.DB {
	t.Helper()
	cfg, _, logger := storageEnv(t)
	db, err := openStorage(t.Context(), cfg, logger)
	require.NoError(t, err)
	return db
}

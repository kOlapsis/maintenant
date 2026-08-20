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
	"time"

	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testInstance(hostname string, seen time.Time) Instance {
	return Instance{
		ID:         uid.New(),
		Hostname:   hostname,
		Version:    "1.4.0-test",
		StartedAt:  seen.Add(-time.Minute),
		LastSeenAt: seen,
	}
}

func TestInstanceStore_RegisterAndPeers(t *testing.T) {
	db := openTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()
	now := time.Now()

	self := testInstance("host-self", now)
	other := testInstance("host-other", now)
	require.NoError(t, s.Register(ctx, self))
	require.NoError(t, s.Register(ctx, other))

	peers, err := s.Peers(ctx, self.ID, now.Add(-2*time.Minute))
	require.NoError(t, err)
	require.Len(t, peers, 1, "self is never a peer of itself")
	assert.Equal(t, other.ID, peers[0].ID)
	assert.Equal(t, "host-other", peers[0].Hostname)
	assert.Equal(t, "1.4.0-test", peers[0].Version)
}

func TestInstanceStore_PeersHonorSince(t *testing.T) {
	db := openTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()
	now := time.Now()

	self := testInstance("host-self", now)
	stale := testInstance("host-stale", now.Add(-10*time.Minute))
	require.NoError(t, s.Register(ctx, self))
	require.NoError(t, s.Register(ctx, stale))

	peers, err := s.Peers(ctx, self.ID, now.Add(-2*time.Minute))
	require.NoError(t, err)
	assert.Empty(t, peers, "an instance without a recent beat is not a peer")

	// Its beat resumes: it becomes visible again.
	require.NoError(t, s.Beat(ctx, stale.ID, now))
	peers, err = s.Peers(ctx, self.ID, now.Add(-2*time.Minute))
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, stale.ID, peers[0].ID)
}

func TestInstanceStore_PurgeStale(t *testing.T) {
	db := openTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()
	now := time.Now()

	live := testInstance("host-live", now)
	dead := testInstance("host-dead", now.Add(-time.Hour))
	require.NoError(t, s.Register(ctx, live))
	require.NoError(t, s.Register(ctx, dead))

	purged, err := s.PurgeStale(ctx, now.Add(-5*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(1), purged)

	var count int
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM instances").Scan(&count))
	assert.Equal(t, 1, count, "only the live instance remains")
}

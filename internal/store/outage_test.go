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
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tcpRelay stands between the instance and its database so a test can cut the
// link and restore it, the way a managed provider's maintenance or a network
// blip does.
type tcpRelay struct {
	listener net.Listener
	target   string

	mu       sync.Mutex
	cut      bool
	openConn []net.Conn
}

func newTCPRelay(t *testing.T, target string) *tcpRelay {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	r := &tcpRelay{listener: ln, target: target}
	go r.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return r
}

func (r *tcpRelay) addr() string { return r.listener.Addr().String() }

func (r *tcpRelay) serve() {
	for {
		client, err := r.listener.Accept()
		if err != nil {
			return
		}
		r.mu.Lock()
		cut := r.cut
		r.mu.Unlock()
		if cut {
			_ = client.Close()
			continue
		}

		upstream, err := net.Dial("tcp", r.target)
		if err != nil {
			_ = client.Close()
			continue
		}
		r.mu.Lock()
		r.openConn = append(r.openConn, client, upstream)
		r.mu.Unlock()

		go func() { _, _ = io.Copy(upstream, client) }()
		go func() { _, _ = io.Copy(client, upstream) }()
	}
}

// Cut drops every live connection and refuses new ones, as an unreachable
// database does.
func (r *tcpRelay) Cut() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cut = true
	for _, c := range r.openConn {
		_ = c.Close()
	}
	r.openConn = nil
}

// Restore lets connections through again.
func (r *tcpRelay) Restore() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cut = false
}

// TestStorageSurvivesOutage is FR-019 and SC-006: an outage shorter than five
// minutes needs no restart and no intervention, and loses no acknowledged
// data. The instance reports the incident and recovers on its own.
func TestStorageSurvivesOutage(t *testing.T) {
	adminDSN := testAdminDSN(t)
	dsn := createTestDatabase(t, adminDSN)

	u, err := ParseDSN(dsn)
	require.NoError(t, err)
	relay := newTCPRelay(t, u.Host)
	u.Host = relay.addr()

	ctx := context.Background()
	db, err := OpenPostgres(ctx, u.String(), testLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, Migrate(ctx, db, testLogger()))
	db.StartWriter(ctx)

	// A write acknowledged before the outage.
	s := NewInstanceStore(db)
	before := Instance{
		ID: "outage-survivor", Hostname: "before", Version: "test",
		StartedAt: time.Now(), LastSeenAt: time.Now(),
	}
	require.NoError(t, s.Register(ctx, before))

	// --- The database becomes unreachable. ---
	relay.Cut()

	_, err = s.Peers(ctx, "someone-else", time.Now().Add(-time.Hour))
	require.Error(t, err, "a query must fail while the database is unreachable")
	assert.True(t, IsUnavailable(err),
		"the failure must read as an outage, not as a caller mistake: %v", err)
	assert.Error(t, db.PingContext(ctx))

	// --- The database comes back. ---
	relay.Restore()

	// No restart, no re-open: the pool renews its connections by itself.
	var recovered bool
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if err := db.PingContext(ctx); err == nil {
			recovered = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.True(t, recovered, "FR-019: the instance recovers on its own, without a restart")

	peers, err := s.Peers(ctx, "someone-else", time.Now().Add(-time.Hour))
	require.NoError(t, err, "queries work again with the same handle")
	require.Len(t, peers, 1, "SC-006: data acknowledged before the outage is still there")
	assert.Equal(t, "before", peers[0].Hostname)

	// And writes work again too.
	require.NoError(t, s.Beat(ctx, before.ID, time.Now()))
}

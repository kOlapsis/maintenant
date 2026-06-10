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

package sqlite

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumeAtomic_Concurrent(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	// Insert a token valid for 1 hour.
	tok := &agent.EnrollmentToken{
		TokenID:   "testtoken01",
		Token:     "mnt_enr_testtoken",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, store.InsertToken(ctx, tok))

	const goroutines = 10
	var (
		successes       atomic.Int32
		alreadyConsumed atomic.Int32
		other           atomic.Int32
	)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			agentID := fmt.Sprintf("agent-%d", i)
			err := store.ConsumeAtomic(ctx, tok.Token, agentID)
			switch {
			case err == nil:
				successes.Add(1)
			case err == agent.ErrTokenAlreadyConsumed:
				alreadyConsumed.Add(1)
			default:
				other.Add(1)
			}
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(1), successes.Load(), "exactly one goroutine should succeed")
	assert.Equal(t, int32(goroutines-1), alreadyConsumed.Load(), "all others should get ErrTokenAlreadyConsumed")
	assert.Equal(t, int32(0), other.Load(), "no unexpected errors")
}

func TestConsumeAtomic_NotFound(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	err := store.ConsumeAtomic(ctx, "mnt_enr_doesnotexist", "agent-x")
	assert.ErrorIs(t, err, agent.ErrTokenNotFound)
}

func TestConsumeAtomic_Expired(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	tok := &agent.EnrollmentToken{
		TokenID:   "expiredtoken01",
		Token:     "mnt_enr_expired",
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
		ExpiresAt: time.Now().UTC().Add(-1 * time.Hour), // already expired
	}
	require.NoError(t, store.InsertToken(ctx, tok))

	err := store.ConsumeAtomic(ctx, tok.Token, "agent-y")
	assert.ErrorIs(t, err, agent.ErrTokenExpired)
}

func TestAgentStore_Delete_CascadePurge(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()
	rw := db.ReadDB()

	insertAgent := func(id string) {
		t.Helper()
		require.NoError(t, store.Insert(ctx, &agent.Agent{
			AgentID:         id,
			PublicKey:       []byte("pubkeyplaceholder12345678901234"),
			Hostname:        "host-" + id,
			Label:           id,
			OSArch:          "linux/amd64",
			AgentVersion:    "1.0.0",
			DetectedRuntime: "docker",
			Status:          "active",
			CreatedAt:       time.Now().UTC(),
		}))
	}

	insertRows := func(agentID string) (containerID string) {
		t.Helper()
		now := time.Now().Unix()
		// container
		containerID = uid.Container(agentID, "ext-"+agentID)
		_, err := db.Writer().Exec(ctx,
			`INSERT INTO containers (id, external_id, name, image, state, first_seen_at, last_state_change_at, agent_id)
			 VALUES (?, ?, ?, 'img', 'running', ?, ?, ?)`,
			containerID, "ext-"+agentID, "ctr-"+agentID, now, now, agentID)
		require.NoError(t, err)
		// endpoint
		_, err = db.Writer().Exec(ctx,
			`INSERT INTO endpoints (id, container_name, label_key, external_id, endpoint_type, target, first_seen_at, last_seen_at, agent_id)
			 VALUES (?, ?, 'key', ?, 'http', 'http://localhost', ?, ?, ?)`,
			uid.EndpointLabel(agentID, "ctr-"+agentID, "key"), "ctr-"+agentID, "ext-ep-"+agentID, now, now, agentID)
		require.NoError(t, err)
		// heartbeat (id is the ping token)
		_, err = db.Writer().Exec(ctx,
			`INSERT INTO heartbeats (id, name, interval_seconds, grace_seconds, created_at, updated_at, agent_id)
			 VALUES (?, ?, 60, 30, ?, ?, ?)`,
			"hb-"+agentID, "hb-name-"+agentID, now, now, agentID)
		require.NoError(t, err)
		// resource_snapshot
		_, err = db.Writer().Exec(ctx,
			`INSERT INTO resource_snapshots (id, container_id, cpu_percent, mem_used, mem_limit,
			  net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp, agent_id)
			 VALUES (?, ?, 1.0, 100, 200, 0, 0, 0, 0, ?, ?)`,
			uid.New(), containerID, now, agentID)
		require.NoError(t, err)
		// cert_monitor
		_, err = db.Writer().Exec(ctx,
			`INSERT INTO cert_monitors (id, hostname, port, source, created_at, agent_id) VALUES (?, ?, 443, 'standalone', ?, ?)`,
			uid.CertMonitor(agentID, "host-"+agentID, 443), "host-"+agentID, now, agentID)
		require.NoError(t, err)
		return containerID
	}

	countRows := func(table, agentID string) int {
		t.Helper()
		var n int
		require.NoError(t, rw.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM "+table+" WHERE agent_id = ?", agentID).Scan(&n))
		return n
	}

	// Insert two agents with events.
	insertAgent("agent-A")
	insertAgent("agent-B")
	insertRows("agent-A")
	insertRows("agent-B")

	// Verify rows exist before delete.
	for _, table := range []string{"containers", "endpoints", "heartbeats", "resource_snapshots", "cert_monitors"} {
		assert.Equal(t, 1, countRows(table, "agent-A"), "table %s before delete", table)
		assert.Equal(t, 1, countRows(table, "agent-B"), "table %s before delete", table)
	}

	// Delete agent A — cascade should purge all its rows.
	require.NoError(t, store.Delete(ctx, "agent-A"))

	for _, table := range []string{"containers", "endpoints", "heartbeats", "resource_snapshots", "cert_monitors"} {
		assert.Equal(t, 0, countRows(table, "agent-A"), "table %s after delete", table)
		assert.Equal(t, 1, countRows(table, "agent-B"), "table %s must be intact", table)
	}

	// agent-A itself must be gone.
	_, err := store.Get(ctx, "agent-A")
	assert.ErrorIs(t, err, agent.ErrAgentNotFound)
	// agent-B must still exist.
	_, err = store.Get(ctx, "agent-B")
	assert.NoError(t, err)
}

func TestAgentStore_LocalSentinelHiddenFromListings(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	// The schema seeds the local sentinel agent (FK anchor). It must remain
	// reachable via Get but never appear in any user-facing enumeration/count.
	sentinel, err := store.Get(ctx, uid.LocalAgent)
	require.NoError(t, err, "sentinel must stay reachable via Get")
	assert.Equal(t, uid.LocalAgent, sentinel.AgentID)

	// With no enrolled agents, every listing/count must be empty.
	list, err := store.List(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, list, "List must exclude the local sentinel")

	active, revoked, err := store.CountByStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, active, "CountByStatus must exclude the local sentinel")
	assert.Equal(t, 0, revoked)

	docker, swarm, k8s, err := store.CountByRuntime(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, docker, "CountByRuntime must exclude the local sentinel")
	assert.Equal(t, 0, swarm)
	assert.Equal(t, 0, k8s)

	// A real enrolled agent shows up; the sentinel still does not.
	require.NoError(t, store.Insert(ctx, &agent.Agent{
		AgentID:         "real-agent-1",
		PublicKey:       []byte("pubkeyplaceholder12345678901234"),
		Hostname:        "remote-host",
		Label:           "Remote",
		OSArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: "docker",
		Status:          "active",
		CreatedAt:       time.Now().UTC(),
	}))

	list, err = store.List(ctx, "")
	require.NoError(t, err)
	require.Len(t, list, 1, "only the enrolled agent is listed")
	assert.Equal(t, "real-agent-1", list[0].AgentID)

	active, _, err = store.CountByStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, active)

	docker, _, _, err = store.CountByRuntime(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, docker)
}

func TestAgentStore_InsertGet(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	a := &agent.Agent{
		AgentID:         "test-agent-id-1",
		PublicKey:       []byte("pubkeyplaceholder12345678901234"),
		Hostname:        "myhost",
		Label:           "My Host",
		OSArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: "docker",
		Status:          "active",
		CreatedAt:       now,
	}
	require.NoError(t, store.Insert(ctx, a))

	got, err := store.Get(ctx, a.AgentID)
	require.NoError(t, err)
	assert.Equal(t, a.AgentID, got.AgentID)
	assert.Equal(t, a.Label, got.Label)
	assert.Equal(t, a.Hostname, got.Hostname)
	assert.Equal(t, a.Status, got.Status)
}

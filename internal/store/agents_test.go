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
	"errors"
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

// enrollAgentRecord builds a minimal valid agent record for EnrollAtomic.
func enrollAgentRecord(id string) *agent.Agent {
	return &agent.Agent{
		AgentID:         id,
		PublicKey:       []byte("pubkeyplaceholder12345678901234"),
		Hostname:        "host-" + id,
		Label:           id,
		OSArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: "docker",
		Status:          "active",
		CreatedAt:       time.Now().UTC(),
	}
}

// tokenFor builds the stored record for a cleartext token the way the creation
// handler does. The store never sees the cleartext: only its hash, plus the
// prefix kept for display.
func tokenFor(cleartext string, createdAt, expiresAt time.Time) *agent.EnrollmentToken {
	hash := agent.HashToken(cleartext)
	return &agent.EnrollmentToken{
		TokenID:     agent.TokenIDFromHash(hash),
		TokenHash:   hash,
		TokenPrefix: agent.TokenPrefix(cleartext),
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
	}
}

// Many agents racing on the SAME one-time token: exactly one wins (consumes the
// token and is inserted); the rest get ErrTokenAlreadyConsumed. limit < 0 keeps
// the cap out of the picture so this isolates the token-consume race.
func TestEnrollAtomic_ConcurrentSameToken(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	const cleartext = "mnt_enr_testtoken"
	require.NoError(t, store.InsertToken(ctx,
		tokenFor(cleartext, time.Now().UTC(), time.Now().UTC().Add(time.Hour))))

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
			err := store.EnrollAtomic(ctx, -1, cleartext, enrollAgentRecord(fmt.Sprintf("agent-%d", i)))
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, agent.ErrTokenAlreadyConsumed):
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

	active, _, err := store.CountByStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, active, "only the winning enrollment is persisted")
}

// Many agents racing on distinct tokens against a finite cap: exactly `limit`
// succeed and the active count lands exactly on the cap — no over-fill. This is
// the regression test for the count/insert race the atomic transaction closes.
func TestEnrollAtomic_ConcurrentCap(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	const limit = 10
	const goroutines = 40
	for i := range goroutines {
		require.NoError(t, store.InsertToken(ctx, tokenFor(
			fmt.Sprintf("mnt_enr_captoken%012d", i),
			time.Now().UTC(), time.Now().UTC().Add(time.Hour))))
	}

	var (
		successes atomic.Int32
		atCap     atomic.Int32
		other     atomic.Int32
	)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			err := store.EnrollAtomic(ctx, limit,
				fmt.Sprintf("mnt_enr_captoken%012d", i),
				enrollAgentRecord(fmt.Sprintf("cap-agent-%d", i)))
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, agent.ErrHostLimitReached):
				atCap.Add(1)
			default:
				other.Add(1)
			}
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(limit), successes.Load(), "exactly `limit` enrollments succeed")
	assert.Equal(t, int32(goroutines-limit), atCap.Load(), "the rest are rejected at the cap")
	assert.Equal(t, int32(0), other.Load(), "no unexpected errors")

	active, _, err := store.CountByStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, limit, active, "active agents never exceed the cap")
}

func TestEnrollAtomic_TokenNotFound(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	err := store.EnrollAtomic(ctx, -1, "mnt_enr_doesnotexist", enrollAgentRecord("agent-x"))
	assert.ErrorIs(t, err, agent.ErrTokenNotFound)
}

func TestEnrollAtomic_TokenExpired(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	const cleartext = "mnt_enr_expired"
	require.NoError(t, store.InsertToken(ctx, tokenFor(cleartext,
		time.Now().UTC().Add(-2*time.Hour),
		time.Now().UTC().Add(-1*time.Hour)))) // already expired

	err := store.EnrollAtomic(ctx, -1, cleartext, enrollAgentRecord("agent-y"))
	assert.ErrorIs(t, err, agent.ErrTokenExpired)
}

func TestAgentStore_Delete_CascadePurge(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()
	rw := db.Reader()

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
			uid.CertMonitor(agentID, "host-"+agentID, 443, ""), "host-"+agentID, now, agentID)
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

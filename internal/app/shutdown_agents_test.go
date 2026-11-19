// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/uid"
)

func storedSnapshotIDs(t *testing.T, dbPath string) map[string]bool {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	rows, err := db.Query(`SELECT id FROM resource_snapshots`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	stored := map[string]bool{}
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		stored[id] = true
	}
	require.NoError(t, rows.Err())
	return stored
}

// The agent streams stop while the writer still accepts writes: an event handled
// during the shutdown is stored, never acknowledged over a stopped writer.
func TestShutdown_AgentServerStopsBeforeTheWriter(t *testing.T) {
	cfg, _, logger := storageEnv(t)
	a, err := New(cfg, logger)
	require.NoError(t, err)
	var writeErr error
	stopped := false
	a.stopAgents = func(ctx context.Context) {
		stopped = true
		_, writeErr = a.db.Writer().Exec(ctx, `DELETE FROM instances WHERE id = ?`, "none")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	require.Eventually(t, func() bool { return instanceRows(t, cfg.DBPath, a.instanceID) == 1 }, 10*time.Second, 20*time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	require.True(t, stopped, "shutdown stops the agent server")
	require.NoError(t, writeErr, "the writer must outlive the agent streams")
}

// Resource samples log their store errors instead of returning them: only the
// shutdown order keeps one from being acknowledged after the writer stopped.
func TestShutdown_AgentEventsAreAcknowledgedOnlyOnceStored(t *testing.T) {
	proEdition(t)
	cfg, _, logger := storageEnv(t)
	a, stop := multihostServer(t, &cfg, logger)
	enrollment := createEnrollmentToken(t, a)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	id, err := agent.LoadOrCreate(dir)
	require.NoError(t, err)
	client, err := agent.NewClientWithFingerprint(ctx, "grpcs://"+cfg.MultiHost.GRPCListen,
		enrollment["grpc_server_fingerprint_sha256"].(string), false, logger)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
	require.NoError(t, agent.RunEnrollment(ctx, id, dir, enrollment["token"].(string), "docker", "shutdown", "1.0.0", client))

	var acked atomic.Uint64
	stream, err := client.DialPush(ctx, id, logger, agent.StreamHooks{Acked: func(seq uint64) { acked.Store(max(acked.Load(), seq)) }})
	require.NoError(t, err)
	require.NoError(t, stream.Send(&agentpb.AgentEvent{
		AgentId: id.AgentID, Seq: 1, EventId: uuid.NewString(), ObservedAt: timestamppb.Now(),
		Body: &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{
			ContainerId: "ctr-1", State: agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		}},
	}))

	var mu sync.Mutex
	eventIDs := map[uint64]string{}
	sending := make(chan struct{})
	go func() {
		defer close(sending)
		for seq := uint64(2); ctx.Err() == nil; seq++ {
			eventID := uuid.NewString()
			mu.Lock()
			eventIDs[seq] = eventID
			mu.Unlock()
			if stream.Send(&agentpb.AgentEvent{
				AgentId: id.AgentID, Seq: seq, EventId: eventID, ObservedAt: timestamppb.Now(),
				Body: &agentpb.AgentEvent_Resource{Resource: &agentpb.ResourceSample{ContainerId: "ctr-1", CpuPercent: 1}},
			}) != nil {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	require.Eventually(t, func() bool { return acked.Load() > 1 }, 15*time.Second, 20*time.Millisecond, "the server must acknowledge samples while it runs")

	stop()
	select {
	case <-sending:
	case <-time.After(3 * time.Second):
	}
	cancel()
	<-sending

	stored := storedSnapshotIDs(t, cfg.DBPath)
	last := acked.Load()
	mu.Lock()
	defer mu.Unlock()
	for seq := uint64(2); seq <= last; seq++ {
		snapshotID := uid.EventRecord(uid.Agent(id.AgentID), eventIDs[seq], "resource_snapshot")
		require.True(t, stored[snapshotID], "sample %d was acknowledged (last ack %d) but never stored (%d stored)", seq, last, len(stored))
	}
}

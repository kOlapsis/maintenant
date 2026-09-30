// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/heartbeat"
)

// PauseHeartbeat must clear alert_state so a heartbeat paused while alerting
// does not stay "alerting" forever: the operator deliberately stopped monitoring.
func TestPauseHeartbeat_ResetsAlertStateToNormal(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	store := NewHeartbeatStore(db)

	h := &heartbeat.Heartbeat{
		ID:              "hb-pause-1",
		Name:            "job",
		IntervalSeconds: 300,
		GraceSeconds:    60,
	}
	_, err := store.CreateHeartbeat(ctx, h)
	require.NoError(t, err)

	// Simulate the heartbeat going down and alerting before it gets paused.
	require.NoError(t, store.UpdateHeartbeatState(ctx, h.ID, heartbeat.StatusDown, heartbeat.AlertAlerting,
		nil, nil, nil, nil, nil, 3, 0))

	require.NoError(t, store.PauseHeartbeat(ctx, h.ID, time.Now()))

	got, err := store.GetHeartbeatByID(ctx, h.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, heartbeat.StatusPaused, got.Status)
	assert.Equal(t, heartbeat.AlertNormal, got.AlertState, "pausing must reset alert_state to normal")
}

// Deleting a heartbeat removes it for good, with everything recorded about it:
// no row is left behind for a retention pass to find.
func TestDeleteHeartbeat_RemovesTheHeartbeatAndItsHistory(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	store := NewHeartbeatStore(db)

	h := &heartbeat.Heartbeat{ID: "hb-delete-1", Name: "job", IntervalSeconds: 300, GraceSeconds: 60}
	_, err := store.CreateHeartbeat(ctx, h)
	require.NoError(t, err)
	now := time.Now()
	_, err = store.InsertPing(ctx, &heartbeat.HeartbeatPing{HeartbeatID: h.ID, PingType: heartbeat.PingSuccess, SourceIP: "10.0.0.1", HTTPMethod: "GET", Timestamp: now})
	require.NoError(t, err)
	_, err = store.InsertExecution(ctx, &heartbeat.HeartbeatExecution{HeartbeatID: h.ID, CompletedAt: &now, Outcome: heartbeat.OutcomeSuccess})
	require.NoError(t, err)
	require.NoError(t, store.PauseHeartbeat(ctx, h.ID, now))

	require.NoError(t, store.DeleteHeartbeat(ctx, h.ID))

	got, err := store.GetHeartbeatByID(ctx, h.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
	for _, table := range []string{"heartbeats", "heartbeat_pings", "heartbeat_executions", "heartbeat_pauses"} {
		var n int
		require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n))
		assert.Zero(t, n, table)
	}
}

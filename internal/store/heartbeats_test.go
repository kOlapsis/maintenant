// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"

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

	require.NoError(t, store.PauseHeartbeat(ctx, h.ID))

	got, err := store.GetHeartbeatByID(ctx, h.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, heartbeat.StatusPaused, got.Status)
	assert.Equal(t, heartbeat.AlertNormal, got.AlertState, "pausing must reset alert_state to normal")
}

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

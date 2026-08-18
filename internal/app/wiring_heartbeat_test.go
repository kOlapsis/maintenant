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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/heartbeat"
)

// TestHeartbeatAlertEvents_Recovery_ClearsBothAlertTypes pins FIX 3c/3d: a single
// recovery must clear both dedup keys a failing ping can have used (deadline_missed
// and exit_code_failure), since the engine resolves by exact key.
func TestHeartbeatAlertEvents_Recovery_ClearsBothAlertTypes(t *testing.T) {
	h := &heartbeat.Heartbeat{ID: "hb-1", Name: "backup"}

	events := heartbeatAlertEvents(h, "recovery", map[string]any{"heartbeat_id": "hb-1"})

	require.Len(t, events, 2)
	types := map[string]bool{}
	for _, evt := range events {
		types[evt.AlertType] = true
		assert.True(t, evt.IsRecover)
		assert.Equal(t, alert.SeverityInfo, evt.Severity)
		assert.Equal(t, "Heartbeat 'backup' recovered", evt.Message)
		assert.Equal(t, alert.SourceHeartbeat, evt.Source)
		assert.Equal(t, "heartbeat", evt.EntityType)
		assert.Equal(t, "hb-1", evt.EntityID)
	}
	assert.True(t, types[heartbeat.AlertTypeDeadlineMissed])
	assert.True(t, types[heartbeat.AlertTypeExitCodeFailure])
}

// TestHeartbeatAlertEvents_DeadlineMissed pins the default failure path: no
// alert_type in details means a missed deadline.
func TestHeartbeatAlertEvents_DeadlineMissed(t *testing.T) {
	h := &heartbeat.Heartbeat{ID: "hb-2", Name: "nightly-backup"}

	events := heartbeatAlertEvents(h, "alert", map[string]any{"heartbeat_id": "hb-2"})

	require.Len(t, events, 1)
	evt := events[0]
	assert.False(t, evt.IsRecover)
	assert.Equal(t, alert.SeverityCritical, evt.Severity)
	assert.Equal(t, heartbeat.AlertTypeDeadlineMissed, evt.AlertType)
	assert.Equal(t, "Heartbeat 'nightly-backup' missed deadline", evt.Message)
}

// TestHeartbeatAlertEvents_ExitCodeFailure pins the exit-code failure path: a
// distinct alert_type and a message naming the exit code.
func TestHeartbeatAlertEvents_ExitCodeFailure(t *testing.T) {
	h := &heartbeat.Heartbeat{ID: "hb-3", Name: "etl-job"}

	events := heartbeatAlertEvents(h, "alert", map[string]any{
		"heartbeat_id": "hb-3",
		"alert_type":   heartbeat.AlertTypeExitCodeFailure,
		"exit_code":    2,
	})

	require.Len(t, events, 1)
	evt := events[0]
	assert.False(t, evt.IsRecover)
	assert.Equal(t, alert.SeverityCritical, evt.Severity)
	assert.Equal(t, heartbeat.AlertTypeExitCodeFailure, evt.AlertType)
	assert.Equal(t, "Heartbeat 'etl-job' failed with exit code 2", evt.Message)
}

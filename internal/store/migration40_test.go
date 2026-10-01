// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/uid"
)

func TestMigration40_ExistingAlertsBelongToTheLocalAgent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	apply := func(direction string) {
		t.Helper()
		sqlText, err := fs.ReadFile(migrationFS,
			"migrations/"+db.dialect.String()+"/40_alert_agent_id."+direction+".sql")
		require.NoError(t, err)
		_, err = db.ReadDB().ExecContext(ctx, string(sqlText))
		require.NoError(t, err, direction)
	}

	apply("down")
	_, err := db.ReadDB().ExecContext(ctx, db.dialect.Rebind(
		`INSERT INTO alerts (id, source, alert_type, message, entity_type, entity_id, entity_name, fired_at)
		VALUES (?, 'container', 'restart_loop', 'm', 'container', 'c1', 'web', 1)`), uid.New())
	require.NoError(t, err)
	apply("up")

	var agentID string
	require.NoError(t, db.ReadDB().QueryRowContext(ctx, `SELECT agent_id FROM alerts`).Scan(&agentID))
	assert.Equal(t, uid.LocalAgent, agentID)
}

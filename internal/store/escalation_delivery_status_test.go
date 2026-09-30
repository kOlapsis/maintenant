// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const downgradeEscalationDeliveriesStatusCheck = `
DROP TABLE escalation_deliveries;
CREATE TABLE escalation_deliveries (
    id            TEXT PRIMARY KEY NOT NULL,
    run_id        TEXT NOT NULL REFERENCES escalation_runs(id) ON DELETE CASCADE,
    level_index   INTEGER NOT NULL,
    channel_id    TEXT REFERENCES notification_channels(id) ON DELETE SET NULL,
    status        TEXT NOT NULL CHECK(status IN ('pending','sent','failed','abandoned','skipped_maintenance')),
    error         TEXT,
    attempt_started_at BIGINT NOT NULL DEFAULT 0,
    sent_at       BIGINT
);
CREATE UNIQUE INDEX idx_escalation_deliveries_run_level ON escalation_deliveries(run_id, level_index, channel_id);
CREATE INDEX idx_escalation_deliveries_pending ON escalation_deliveries(status, attempt_started_at) WHERE status = 'pending';
CREATE INDEX idx_escalation_deliveries_run_id ON escalation_deliveries(run_id);
INSERT INTO alerts (id, source, alert_type, message, entity_type, entity_id, entity_name, fired_at)
VALUES ('legacy-alert', 'container', 'down', 'down', 'container', 'c1', 'web', 1700000000);
INSERT INTO escalation_runs (id, policy_snapshot_json, alert_id, status)
VALUES ('legacy-run', '{}', 'legacy-alert', 'exhausted');
INSERT INTO escalation_deliveries (id, run_id, level_index, status, attempt_started_at)
VALUES ('legacy-delivery', 'legacy-run', 0, 'sent', 1700000000);
`

func TestRebuildEscalationDeliveriesStatusCheck(t *testing.T) {
	requireSQLite(t)
	db := openTestDB(t)
	rw := db.ReadDB()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err := rw.ExecContext(ctx, downgradeEscalationDeliveriesStatusCheck)
	require.NoError(t, err)

	require.NoError(t, rebuildEscalationDeliveriesStatusCheck(ctx, rw, logger))

	ddl := scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='escalation_deliveries'`)
	require.Contains(t, ddl, escalationDeliveryStatusConstraint)
	require.NotContains(t, ddl, "skipped_maintenance")

	require.Equal(t, "sent", scanString(t, rw, `SELECT status FROM escalation_deliveries WHERE id='legacy-delivery'`))
	_, err = rw.ExecContext(ctx, `UPDATE escalation_deliveries SET status='skipped_maintenance' WHERE id='legacy-delivery'`)
	require.Error(t, err, "the rebuilt CHECK must refuse the removed status")

	var indexes int
	require.NoError(t, rw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='escalation_deliveries' AND name LIKE 'idx_%'`).Scan(&indexes))
	require.Equal(t, 3, indexes)

	require.NoError(t, rebuildEscalationDeliveriesStatusCheck(ctx, rw, logger))
	require.Equal(t, ddl, scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='escalation_deliveries'`))
}

func TestEscalationDeliveryStatusConstraint_MatchesSchema(t *testing.T) {
	requireSQLite(t)
	schema, err := os.ReadFile("uuid_schema.sql")
	require.NoError(t, err)
	require.True(t, strings.Contains(string(schema), escalationDeliveryStatusConstraint),
		"uuid_schema.sql must contain %q verbatim: the rebuild guard depends on it", escalationDeliveryStatusConstraint)
}

func TestEscalationDeliveries_RefuseTheRemovedStatus(t *testing.T) {
	db := openTestDB(t)
	rw := db.ReadDB()
	ctx := context.Background()

	for _, q := range []string{
		`INSERT INTO alerts (id, source, alert_type, message, entity_type, entity_id, entity_name, fired_at)
		VALUES ('a1', 'container', 'down', 'down', 'container', 'c1', 'web', 1700000000)`,
		`INSERT INTO escalation_runs (id, policy_snapshot_json, alert_id, status) VALUES ('r1', '{}', 'a1', 'exhausted')`,
	} {
		_, err := rw.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	_, err := rw.ExecContext(ctx,
		`INSERT INTO escalation_deliveries (id, run_id, level_index, status) VALUES ('d1', 'r1', 0, 'skipped_maintenance')`)
	require.Error(t, err, "no code writes skipped_maintenance, the schema must not accept it either")
}

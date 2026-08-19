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
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// downgradeEndpointsStatusCheck restores the pre-degraded CHECK, standing in for
// a database converted before the state existed.
const downgradeEndpointsStatusCheck = `
DROP TABLE endpoints;
CREATE TABLE endpoints (
    id              TEXT PRIMARY KEY NOT NULL,
    agent_id        TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    container_name  TEXT NOT NULL,
    label_key       TEXT NOT NULL,
    external_id     TEXT NOT NULL,
    endpoint_type   TEXT NOT NULL CHECK(endpoint_type IN ('http','tcp')),
    target          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('up','down','unknown')),
    alert_state     TEXT NOT NULL DEFAULT 'normal' CHECK(alert_state IN ('normal','alerting')),
    consecutive_failures  INTEGER NOT NULL DEFAULT 0,
    consecutive_successes INTEGER NOT NULL DEFAULT 0,
    last_check_at         BIGINT,
    last_response_time_ms BIGINT,
    last_http_status      INTEGER,
    last_error      TEXT,
    config_json     TEXT NOT NULL DEFAULT '{}',
    active          INTEGER NOT NULL DEFAULT 1,
    first_seen_at   BIGINT NOT NULL,
    last_seen_at    BIGINT NOT NULL,
    source          TEXT NOT NULL DEFAULT 'label' CHECK(source IN ('label','standalone')),
    name            TEXT NOT NULL DEFAULT '',
    UNIQUE(agent_id, container_name, label_key)
);
INSERT INTO endpoints (id, container_name, label_key, external_id, endpoint_type, target,
                       status, first_seen_at, last_seen_at)
VALUES ('legacy-ep', 'web', 'http', 'ext-1', 'http', 'https://legacy.example.com',
        'up', 1700000000, 1700000000);
`

func TestRebuildEndpointsForDegraded(t *testing.T) {
	db := openTestDB(t)
	rw := db.ReadDB()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err := rw.ExecContext(ctx, downgradeEndpointsStatusCheck)
	require.NoError(t, err)

	// Before the rebuild the state is rejected outright — this is what an
	// upgraded install would hit on the first degraded check.
	_, err = rw.ExecContext(ctx, `UPDATE endpoints SET status='degraded' WHERE id='legacy-ep'`)
	require.Error(t, err, "the old CHECK must reject 'degraded'")

	require.NoError(t, rebuildEndpointsForDegraded(ctx, rw, logger))

	ddl := scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='endpoints'`)
	require.Contains(t, ddl, endpointStatusConstraint)

	// The row survived, and the state now persists.
	require.Equal(t, "legacy-ep", scanString(t, rw, `SELECT id FROM endpoints`))
	_, err = rw.ExecContext(ctx, `UPDATE endpoints SET status='degraded' WHERE id='legacy-ep'`)
	require.NoError(t, err)
	require.Equal(t, "degraded", scanString(t, rw, `SELECT status FROM endpoints`))

	// Indexes were recreated after the DROP TABLE.
	var indexes int
	require.NoError(t, rw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='endpoints' AND name LIKE 'idx_%'`).Scan(&indexes))
	require.Equal(t, 5, indexes)

	// Second run is a no-op.
	require.NoError(t, rebuildEndpointsForDegraded(ctx, rw, logger))
	require.Equal(t, ddl, scanString(t, rw, `SELECT sql FROM sqlite_master WHERE type='table' AND name='endpoints'`))
	require.Equal(t, "degraded", scanString(t, rw, `SELECT status FROM endpoints`))
}

// The rebuild's CREATE TABLE must embed the exact constraint string the
// idempotency guard looks for, or fresh installs would be rebuilt on every boot.
func TestEndpointStatusConstraint_MatchesSchema(t *testing.T) {
	schema, err := os.ReadFile("uuid_schema.sql")
	require.NoError(t, err)
	require.True(t, strings.Contains(string(schema), endpointStatusConstraint),
		"uuid_schema.sql must contain %q verbatim — the rebuild guard depends on it", endpointStatusConstraint)
}

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/uid"
)

const (
	m46Workload = "shop/Deployment/api"
	m46Local    = "ctr-web-local"
	m46Gone     = "ctr-gone"
	m46Shared   = "ctr-web-shared"
	m46Archived = "ctr-job-archived"
)

type m46Row struct{ agent, container string }

var m46Tables = []struct{ table, container string }{
	{"version_pins", "container_id"},
	{"container_cves", "container_id"},
	{"cve_evaluations", "container_id"},
	{"risk_score_history", "container_id"},
	{"risk_acknowledgments", "container_external_id"},
	{"digest_baselines", "container_id"},
}

func applyMigration46(t *testing.T, db *DB, direction string) {
	t.Helper()
	matches, err := fs.Glob(migrationFS, "migrations/"+db.dialect.String()+"/46_*."+direction+".sql")
	require.NoError(t, err)
	require.Len(t, matches, 1, "migration 46 (%s) must exist for %s", direction, db.dialect.String())
	sqlText, err := fs.ReadFile(migrationFS, matches[0])
	require.NoError(t, err)
	_, err = db.ReadDB().ExecContext(context.Background(), string(sqlText))
	require.NoError(t, err, direction)
}

func m46Schema(t *testing.T, db *DB) schemaDesc {
	t.Helper()
	if db.dialect == DialectPostgres {
		return introspectPostgres(t, db.ReadDB())
	}
	return introspectSQLite(t, db.ReadDB())
}

func insertPre46Rows(t *testing.T, db *DB, externalID string) {
	t.Helper()
	now := time.Now().Unix()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO version_pins (id, container_id, image, pinned_tag, pinned_digest, reason, pinned_at)
			VALUES (?, ?, 'shop/api', '1.0', 'sha256:aaa', 'frozen by ops', ?)`, []any{uid.New(), externalID, now}},
		{`INSERT INTO container_cves (id, container_id, cve_id, severity, first_detected_at)
			VALUES (?, ?, 'CVE-2026-0001', 'high', ?)`, []any{uid.New(), externalID, now}},
		{`INSERT INTO cve_evaluations (container_id, status, evaluated_at) VALUES (?, 'evaluated', ?)`,
			[]any{externalID, now}},
		{`INSERT INTO risk_score_history (id, container_id, score, factors_json, recorded_at)
			VALUES (?, ?, 64, '{}', ?)`, []any{uid.New(), externalID, now}},
		{`INSERT INTO risk_acknowledgments (id, container_external_id, finding_type, finding_key, reason, acknowledged_at)
			VALUES (?, ?, 'cve', 'CVE-2026-0001', 'not exposed', ?)`, []any{uid.New(), externalID, now}},
		{`INSERT INTO digest_baselines (container_id, image, tag, remote_digest, checked_at)
			VALUES (?, 'shop/api:1.0', '1.0', 'sha256:bbb', ?)`, []any{externalID, now}},
	} {
		_, err := db.ReadDB().ExecContext(context.Background(), db.dialect.Rebind(q.sql), q.args...)
		require.NoError(t, err, q.sql)
	}
}

func m46Rows(t *testing.T, db *DB, table, column string) []m46Row {
	t.Helper()
	rows, err := db.ReadDB().QueryContext(context.Background(),
		`SELECT agent_id, `+column+` FROM `+table)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []m46Row
	for rows.Next() {
		var r m46Row
		require.NoError(t, rows.Scan(&r.agent, &r.container))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// A row recorded before the upgrade under an external id several agents share is
// kept for each of them, the local runtime included, and an archived container
// still claims its row; a row whose container is unknown stays on the local runtime.
func TestMigration46_EveryHomonymKeepsItsRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	seedAgent(t, db, "agent-staging")
	seedAgent(t, db, "agent-prod")
	head := m46Schema(t, db)

	seedHostedContainer(t, cs, "agent-staging", m46Workload, "api", "shop/api:1.0")
	seedHostedContainer(t, cs, "agent-prod", m46Workload, "api", "shop/api:1.0")
	seedHostedContainer(t, cs, uid.LocalAgent, m46Local, "web", "nginx:1.27")
	seedHostedContainer(t, cs, uid.LocalAgent, m46Shared, "web", "nginx:1.27")
	seedHostedContainer(t, cs, "agent-prod", m46Shared, "web", "nginx:1.27")
	archived := seedHostedContainer(t, cs, "agent-staging", m46Archived, "job", "busybox:1.37")
	require.NoError(t, cs.ArchiveContainer(ctx, archived, time.Now()))

	applyMigration46(t, db, "down")
	for _, id := range []string{m46Workload, m46Local, m46Gone, m46Shared, m46Archived} {
		insertPre46Rows(t, db, id)
	}
	applyMigration46(t, db, "up")

	want := []m46Row{
		{"agent-prod", m46Workload},
		{"agent-staging", m46Workload},
		{uid.LocalAgent, m46Local},
		{uid.LocalAgent, m46Gone},
		{uid.LocalAgent, m46Shared},
		{"agent-prod", m46Shared},
		{"agent-staging", m46Archived},
	}
	for _, tc := range m46Tables {
		assert.ElementsMatch(t, want, m46Rows(t, db, tc.table, tc.container), tc.table)
	}

	for _, agentID := range []string{"agent-staging", "agent-prod"} {
		var reason string
		require.NoError(t, db.ReadDB().QueryRowContext(ctx, db.dialect.Rebind(
			`SELECT reason FROM version_pins WHERE agent_id = ? AND container_id = ?`), agentID, m46Workload).Scan(&reason))
		assert.Equal(t, "frozen by ops", reason, "the pin of %s keeps the operator's reason", agentID)
	}

	applyMigration46(t, db, "down")
	applyMigration46(t, db, "up")
	assert.Equal(t, head, m46Schema(t, db), "the migration rebuilds the head schema")
	for _, tc := range m46Tables {
		assert.ElementsMatch(t, want, m46Rows(t, db, tc.table, tc.container), "%s after a round trip", tc.table)
	}
}

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

const lockedDeadline = 1893456000

func lockSourceMonitors(t *testing.T, src *DB) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`UPDATE endpoints SET status='down', alert_state='alerting', consecutive_failures=5, consecutive_successes=0`,
		`UPDATE heartbeats SET status='down', alert_state='alerting', next_deadline_at=1893456000`,
		`UPDATE cert_monitors SET last_alerted_threshold=7`,
		`UPDATE resource_alert_configs SET alert_state='alerting'`,
	} {
		_, err := src.Reader().ExecContext(ctx, q)
		require.NoError(t, err, q)
	}
}

type alertLocks struct {
	endpointState                string
	endpointFailures             int
	heartbeatStatus, heartbeatAS string
	heartbeatDeadline            int64
	certThreshold                sql.NullInt64
	resourceState                string
}

func readAlertLocks(t *testing.T, db *sql.DB) alertLocks {
	t.Helper()
	var l alertLocks
	require.NoError(t, db.QueryRow(`SELECT alert_state, consecutive_failures FROM endpoints`).
		Scan(&l.endpointState, &l.endpointFailures))
	require.NoError(t, db.QueryRow(`SELECT status, alert_state, next_deadline_at FROM heartbeats`).
		Scan(&l.heartbeatStatus, &l.heartbeatAS, &l.heartbeatDeadline))
	require.NoError(t, db.QueryRow(`SELECT last_alerted_threshold FROM cert_monitors`).Scan(&l.certThreshold))
	require.NoError(t, db.QueryRow(`SELECT alert_state FROM resource_alert_configs`).Scan(&l.resourceState))
	return l
}

func requireNeutralLocks(t *testing.T, l alertLocks) {
	t.Helper()
	require.Equal(t, "normal", l.endpointState)
	require.Zero(t, l.endpointFailures)
	require.Equal(t, "up", l.heartbeatStatus)
	require.Equal(t, "normal", l.heartbeatAS)
	require.EqualValues(t, lockedDeadline, l.heartbeatDeadline, "the deadline stays, it is what brings a dead heartbeat down again")
	require.False(t, l.certThreshold.Valid)
	require.Equal(t, "normal", l.resourceState)
}

func TestNeutralizeAlertLocks_ResetsTheFourLocks(t *testing.T) {
	src, _ := seededSource(t)
	lockSourceMonitors(t, src)
	ctx := context.Background()

	tx, err := src.ReadDB().BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	require.NoError(t, neutralizeAlertLocks(ctx, tx))

	var l alertLocks
	require.NoError(t, tx.QueryRow(`SELECT alert_state, consecutive_failures FROM endpoints`).
		Scan(&l.endpointState, &l.endpointFailures))
	require.NoError(t, tx.QueryRow(`SELECT status, alert_state, next_deadline_at FROM heartbeats`).
		Scan(&l.heartbeatStatus, &l.heartbeatAS, &l.heartbeatDeadline))
	require.NoError(t, tx.QueryRow(`SELECT last_alerted_threshold FROM cert_monitors`).Scan(&l.certThreshold))
	require.NoError(t, tx.QueryRow(`SELECT alert_state FROM resource_alert_configs`).Scan(&l.resourceState))
	requireNeutralLocks(t, l)
}

func TestCopy_CarriedMonitorsArriveWithNeutralAlertLocks(t *testing.T) {
	src, _ := seededSource(t)
	lockSourceMonitors(t, src)
	dst := openEmptyTarget(t)

	_, err := Copy(context.Background(), src.ReadDB(), dst, nil, func(Plan) bool { return true })
	require.NoError(t, err)

	requireNeutralLocks(t, readAlertLocks(t, dst))

	var failures int
	require.NoError(t, src.ReadDB().QueryRow(`SELECT consecutive_failures FROM endpoints`).Scan(&failures))
	require.Equal(t, 5, failures, "the source is left as it was")
}

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/uid"
)

func seedUptimeEndpoint(t *testing.T, db *DB) string {
	t.Helper()
	id, err := NewEndpointStore(db).UpsertEndpoint(context.Background(), &endpoint.Endpoint{
		ContainerName: "web-" + uid.New(),
		LabelKey:      "maintenant.endpoint.http",
		ExternalID:    "ext-web",
		EndpointType:  endpoint.TypeHTTP,
		Target:        "http://web:8080",
	})
	require.NoError(t, err)
	return id
}

func seedUptimeHeartbeat(t *testing.T, db *DB) string {
	t.Helper()
	id, err := NewHeartbeatStore(db).CreateHeartbeat(context.Background(), &heartbeat.Heartbeat{
		Name: "backup", IntervalSeconds: 300, GraceSeconds: 60,
	})
	require.NoError(t, err)
	return id
}

func addCheck(t *testing.T, db *DB, endpointID string, success bool, ts time.Time) {
	t.Helper()
	ok := 0
	if success {
		ok = 1
	}
	_, err := db.Writer().Exec(context.Background(),
		`INSERT INTO check_results (id, endpoint_id, success, response_time_ms, timestamp) VALUES (?, ?, ?, 100, ?)`,
		uid.New(), endpointID, ok, ts.Unix())
	require.NoError(t, err)
}

func addPing(t *testing.T, db *DB, heartbeatID, pingType string, exitCode *int, ts time.Time) {
	t.Helper()
	_, err := db.Writer().Exec(context.Background(),
		`INSERT INTO heartbeat_pings (id, heartbeat_id, ping_type, exit_code, source_ip, http_method, timestamp)
		VALUES (?, ?, ?, ?, '127.0.0.1', 'GET', ?)`,
		uid.New(), heartbeatID, pingType, exitCode, ts.Unix())
	require.NoError(t, err)
}

func addTransition(t *testing.T, db *DB, containerID, prevState, newState string, ts time.Time) {
	t.Helper()
	_, err := db.Writer().Exec(context.Background(),
		`INSERT INTO state_transitions (id, container_id, previous_state, new_state, timestamp) VALUES (?, ?, ?, ?, ?)`,
		uid.New(), containerID, prevState, newState, ts.Unix())
	require.NoError(t, err)
}

func exitCode(n int) *int { return &n }

// dayOf returns the entry for day in a most-recent-first series.
func dayOf(t *testing.T, series []DailyUptime, day time.Time) DailyUptime {
	t.Helper()
	for _, du := range series {
		if du.Date == day.Format("2006-01-02") {
			return du
		}
	}
	t.Fatalf("day %s missing from the series", day.Format("2006-01-02"))
	return DailyUptime{}
}

func requirePercent(t *testing.T, du DailyUptime, want float64) {
	t.Helper()
	require.NotNil(t, du.UptimePercent, "day %s has no uptime", du.Date)
	assert.InDelta(t, want, *du.UptimePercent, 0.001, "day %s", du.Date)
}

func TestEndpointDailyUptime(t *testing.T) {
	ctx := context.Background()
	today := startOfUTCDay(time.Now())
	yesterday := today.AddDate(0, 0, -1)

	t.Run("no checks returns all null days, most recent first", func(t *testing.T) {
		db := openTestDB(t)
		result, err := NewUptimeDailyStore(db).GetEndpointDailyUptime(ctx, seedUptimeEndpoint(t, db), 3)
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, today.Format("2006-01-02"), result[0].Date)
		assert.Equal(t, yesterday.Format("2006-01-02"), result[1].Date)
		for _, du := range result {
			assert.Nil(t, du.UptimePercent)
			assert.Zero(t, du.IncidentCount)
		}
	})

	t.Run("partial uptime with incident", func(t *testing.T) {
		db := openTestDB(t)
		id := seedUptimeEndpoint(t, db)
		for i := 0; i < 4; i++ {
			addCheck(t, db, id, true, yesterday.Add(time.Duration(i)*time.Hour))
		}
		addCheck(t, db, id, false, yesterday.Add(4*time.Hour))

		result, err := NewUptimeDailyStore(db).GetEndpointDailyUptime(ctx, id, 2)
		require.NoError(t, err)
		requirePercent(t, result[1], 80)
		assert.Equal(t, 1, result[1].IncidentCount)
	})

	t.Run("a day opening on a failure after a success counts an incident", func(t *testing.T) {
		db := openTestDB(t)
		id := seedUptimeEndpoint(t, db)
		addCheck(t, db, id, true, yesterday.Add(-time.Hour))
		addCheck(t, db, id, false, yesterday.Add(time.Minute))

		result, err := NewUptimeDailyStore(db).GetEndpointDailyUptime(ctx, id, 2)
		require.NoError(t, err)
		requirePercent(t, result[1], 0)
		assert.Equal(t, 1, result[1].IncidentCount)
	})

	t.Run("gap days stay null", func(t *testing.T) {
		db := openTestDB(t)
		id := seedUptimeEndpoint(t, db)
		addCheck(t, db, id, false, today.AddDate(0, 0, -3).Add(5*time.Hour))
		addCheck(t, db, id, true, yesterday.Add(time.Hour))

		result, err := NewUptimeDailyStore(db).GetEndpointDailyUptime(ctx, id, 4)
		require.NoError(t, err)
		requirePercent(t, result[1], 100)
		assert.Nil(t, result[2].UptimePercent)
		requirePercent(t, result[3], 0)
	})

	t.Run("days default to 90 and cap at 365", func(t *testing.T) {
		db := openTestDB(t)
		id := seedUptimeEndpoint(t, db)
		store := NewUptimeDailyStore(db)

		result, err := store.GetEndpointDailyUptime(ctx, id, 0)
		require.NoError(t, err)
		assert.Len(t, result, 90)

		result, err = store.GetEndpointDailyUptime(ctx, id, 500)
		require.NoError(t, err)
		assert.Len(t, result, 365)
	})
}

func TestHeartbeatDailyUptime(t *testing.T) {
	ctx := context.Background()
	yesterday := startOfUTCDay(time.Now()).AddDate(0, 0, -1)

	t.Run("no pings returns null days", func(t *testing.T) {
		db := openTestDB(t)
		result, err := NewUptimeDailyStore(db).GetHeartbeatDailyUptime(ctx, seedUptimeHeartbeat(t, db), 3)
		require.NoError(t, err)
		require.Len(t, result, 3)
		for _, du := range result {
			assert.Nil(t, du.UptimePercent)
		}
	})

	t.Run("a run reported with start then exit code 0 is up", func(t *testing.T) {
		db := openTestDB(t)
		id := seedUptimeHeartbeat(t, db)
		for i := 0; i < 3; i++ {
			run := yesterday.Add(time.Duration(i) * time.Hour)
			addPing(t, db, id, "start", nil, run)
			addPing(t, db, id, "exit_code", exitCode(0), run.Add(time.Minute))
		}

		result, err := NewUptimeDailyStore(db).GetHeartbeatDailyUptime(ctx, id, 2)
		require.NoError(t, err)
		requirePercent(t, result[1], 100)
		assert.Zero(t, result[1].IncidentCount)
	})

	t.Run("a failing exit code after a success is an incident", func(t *testing.T) {
		db := openTestDB(t)
		id := seedUptimeHeartbeat(t, db)
		addPing(t, db, id, "success", nil, yesterday.Add(1*time.Hour))
		addPing(t, db, id, "success", nil, yesterday.Add(2*time.Hour))
		addPing(t, db, id, "exit_code", exitCode(0), yesterday.Add(3*time.Hour))
		addPing(t, db, id, "exit_code", exitCode(2), yesterday.Add(4*time.Hour))

		result, err := NewUptimeDailyStore(db).GetHeartbeatDailyUptime(ctx, id, 2)
		require.NoError(t, err)
		requirePercent(t, result[1], 75)
		assert.Equal(t, 1, result[1].IncidentCount)
	})
}

func TestContainerDailyUptime(t *testing.T) {
	ctx := context.Background()
	today := startOfUTCDay(time.Now())

	t.Run("no transitions returns null days", func(t *testing.T) {
		db := openTestDB(t)
		cid := seedHostContainer(t, NewContainerStore(db), "ext-none", "")
		result, err := NewUptimeDailyStore(db).GetContainerDailyUptime(ctx, cid, 3)
		require.NoError(t, err)
		require.Len(t, result, 3)
		for _, du := range result {
			assert.Nil(t, du.UptimePercent)
		}
	})

	t.Run("full past day with a down period is time-weighted", func(t *testing.T) {
		db := openTestDB(t)
		cid := seedHostContainer(t, NewContainerStore(db), "ext-weighted", "")
		addTransition(t, db, cid, "created", "running", today.AddDate(0, 0, -10))
		twoDaysAgo := today.AddDate(0, 0, -2)
		addTransition(t, db, cid, "running", "exited", twoDaysAgo.Add(8*time.Hour))
		addTransition(t, db, cid, "exited", "running", twoDaysAgo.Add(16*time.Hour))

		result, err := NewUptimeDailyStore(db).GetContainerDailyUptime(ctx, cid, 3)
		require.NoError(t, err)
		requirePercent(t, result[2], 66.66)
		assert.Equal(t, 1, result[2].IncidentCount)
		requirePercent(t, result[1], 100)
	})

	t.Run("days after its archival have no uptime", func(t *testing.T) {
		db := openTestDB(t)
		cs := NewContainerStore(db)
		cid := seedHostContainer(t, cs, "ext-archived", "")
		addTransition(t, db, cid, "created", "running", today.AddDate(0, 0, -5))
		addTransition(t, db, cid, "running", "exited", today.AddDate(0, 0, -3).Add(12*time.Hour))
		require.NoError(t, cs.ArchiveContainer(ctx, cid, today.AddDate(0, 0, -3).Add(13*time.Hour)))

		result, err := NewUptimeDailyStore(db).GetContainerDailyUptime(ctx, cid, 5)
		require.NoError(t, err)
		requirePercent(t, result[4], 100)
		require.NotNil(t, result[3].UptimePercent)
		assert.Nil(t, result[2].UptimePercent, "the container no longer existed")
		assert.Nil(t, result[0].UptimePercent)
	})
}

// A completed day is written to the daily aggregate before its raw rows are
// purged, so the 90-day bars outlive the 30-day raw retention.
func TestUptimeDaily_SurvivesRawPurge(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := testLogger()
	today := startOfUTCDay(time.Now())
	day := today.AddDate(0, 0, -3)

	epID := seedUptimeEndpoint(t, db)
	for i := 0; i < 3; i++ {
		addCheck(t, db, epID, true, day.Add(time.Duration(i)*time.Hour))
	}
	addCheck(t, db, epID, false, day.Add(3*time.Hour))

	hbID := seedUptimeHeartbeat(t, db)
	addPing(t, db, hbID, "success", nil, day.Add(time.Hour))
	addPing(t, db, hbID, "exit_code", exitCode(1), day.Add(2*time.Hour))

	cs := NewContainerStore(db)
	cid := seedHostContainer(t, cs, "ext-purge", "")
	addTransition(t, db, cid, "created", "running", day)
	addTransition(t, db, cid, "running", "exited", day.Add(18*time.Hour))
	addTransition(t, db, cid, "exited", "running", today.AddDate(0, 0, -2))

	uptime := NewUptimeDailyStore(db)
	eps, hbs := NewEndpointStore(db), NewHeartbeatStore(db)
	pass := func(raw time.Duration) {
		cfg := RetentionConfig{CheckResults: raw, HeartbeatPings: raw, Transitions: raw}.withDefaults(logger)
		var p retentionPass
		runCleanup(ctx, cs, uptime, logger, cfg, &p)
		runEndpointCleanup(ctx, eps, uptime, logger, cfg, &p)
		runHeartbeatCleanup(ctx, hbs, uptime, logger, cfg, &p)
	}

	pass(30 * 24 * time.Hour)
	pass(time.Hour)

	assert.Zero(t, countTableRows(t, db, "check_results"), "the raw checks are gone")
	assert.Zero(t, countTableRows(t, db, "heartbeat_pings"), "the raw pings are gone")

	ep, err := uptime.GetEndpointDailyUptime(ctx, epID, 7)
	require.NoError(t, err)
	requirePercent(t, dayOf(t, ep, day), 75)
	assert.Equal(t, 1, dayOf(t, ep, day).IncidentCount)

	hb, err := uptime.GetHeartbeatDailyUptime(ctx, hbID, 7)
	require.NoError(t, err)
	requirePercent(t, dayOf(t, hb, day), 50)

	ct, err := uptime.GetContainerDailyUptime(ctx, cid, 7)
	require.NoError(t, err)
	requirePercent(t, dayOf(t, ct, day), 75)
	assert.Equal(t, 1, dayOf(t, ct, day).IncidentCount)
	requirePercent(t, dayOf(t, ct, today), 100)
}

// A check replayed by an agent after the day was aggregated still lands in it.
func TestUptimeRollup_RewritesTheLastAggregatedDay(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	yesterday := startOfUTCDay(now).AddDate(0, 0, -1)
	uptime := NewUptimeDailyStore(db)

	id := seedUptimeEndpoint(t, db)
	addCheck(t, db, id, true, yesterday.Add(time.Hour))
	require.NoError(t, uptime.rollupEndpoints(ctx, now, 30*24*time.Hour))

	addCheck(t, db, id, false, yesterday.Add(2*time.Hour))
	require.NoError(t, uptime.rollupEndpoints(ctx, now, 30*24*time.Hour))

	stored, err := uptime.storedDays(ctx, endpointUptimeTable, id, yesterday, yesterday.Add(uptimeDay))
	require.NoError(t, err)
	require.Contains(t, stored, yesterday.Unix())
	assert.InDelta(t, 50, stored[yesterday.Unix()].percent, 0.001)
}

// Once the purge has started eating a day, the rollup must not rewrite it from
// what is left.
func TestUptimeRollup_LeavesAPartlyPurgedDayAlone(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	day := startOfUTCDay(now).AddDate(0, 0, -5)
	uptime := NewUptimeDailyStore(db)

	id := seedUptimeEndpoint(t, db)
	addCheck(t, db, id, true, day.Add(time.Hour))
	addCheck(t, db, id, false, day.Add(2*time.Hour))
	require.NoError(t, uptime.rollupEndpoints(ctx, now, 30*24*time.Hour))

	_, err := db.Writer().Exec(ctx, `DELETE FROM check_results WHERE endpoint_id = ? AND success = 0`, id)
	require.NoError(t, err)
	require.NoError(t, uptime.rollupEndpoints(ctx, now, 5*24*time.Hour))

	stored, err := uptime.storedDays(ctx, endpointUptimeTable, id, day, day.Add(uptimeDay))
	require.NoError(t, err)
	assert.InDelta(t, 50, stored[day.Unix()].percent, 0.001)
}

func TestUptimeDailyCleanup_KeepsAYear(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	today := startOfUTCDay(time.Now())
	id := seedUptimeEndpoint(t, db)

	for _, day := range []time.Time{today.AddDate(0, 0, -364), today.AddDate(0, 0, -400)} {
		_, err := db.Writer().Exec(ctx,
			`INSERT INTO endpoint_uptime_daily (id, endpoint_id, day, uptime_percent, incident_count) VALUES (?, ?, ?, 99.5, 0)`,
			uid.New(), id, day.Unix())
		require.NoError(t, err)
	}

	var p retentionPass
	runUptimeDailyCleanup(ctx, NewUptimeDailyStore(db), testLogger(), RetentionConfig{}.withDefaults(testLogger()), &p)

	assert.Equal(t, int64(1), p.deleted)
	result, err := NewUptimeDailyStore(db).GetEndpointDailyUptime(ctx, id, 365)
	require.NoError(t, err)
	requirePercent(t, dayOf(t, result, today.AddDate(0, 0, -364)), 99.5)
}

// A container that has not changed state for longer than the transition
// retention still knows the state it is in.
func TestDeleteTransitionsBefore_KeepsTheLatestOfEachContainer(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	old := time.Now().AddDate(0, 0, -200)

	stable := seedHostContainer(t, cs, "ext-stable", "")
	addTransition(t, db, stable, "created", "running", old)

	flappy := seedHostContainer(t, cs, "ext-flappy", "")
	addTransition(t, db, flappy, "created", "running", old)
	addTransition(t, db, flappy, "running", "exited", old.Add(time.Hour))
	addTransition(t, db, flappy, "exited", "running", time.Now().Add(-time.Hour))

	deleted, err := cs.DeleteTransitionsBefore(ctx, time.Now().AddDate(0, 0, -90), 1000)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)
	assert.Equal(t, 2, countTableRows(t, db, "state_transitions"))

	result, err := NewUptimeDailyStore(db).GetContainerDailyUptime(ctx, stable, 1)
	require.NoError(t, err)
	requirePercent(t, result[0], 100)
}

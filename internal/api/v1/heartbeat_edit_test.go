// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Acceptance tests of 048-heartbeat-edit on the real router and the real store
// (SQLite, or PostgreSQL through MAINTENANT_TEST_DATABASE_URL).

type sentEvent struct {
	Type string
	Data json.RawMessage
}

type editBench struct {
	api http.Handler
	svc *heartbeat.Service
	hs  *store.HeartbeatStore
	db  *store.DB

	mu     sync.Mutex
	events []sentEvent
	alerts []string
}

func newEditBench(t *testing.T) *editBench {
	t.Helper()
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	b := &editBench{hs: store.NewHeartbeatStore(db), db: db}
	b.svc = heartbeat.NewService(heartbeat.Deps{
		Store:  b.hs,
		Logger: logger,
		EventCallback: func(eventType string, data interface{}) {
			raw, err := json.Marshal(data)
			assert.NoError(t, err, "the %s payload is broadcast as JSON", eventType)
			b.mu.Lock()
			defer b.mu.Unlock()
			b.events = append(b.events, sentEvent{Type: eventType, Data: raw})
		},
		AlertCallback: func(_ *heartbeat.Heartbeat, alertType string, _ map[string]interface{}) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.alerts = append(b.alerts, alertType)
		},
	})
	b.api = NewRouter(HandlerDeps{Logger: logger, Heartbeats: b.svc, UptimeDaily: store.NewUptimeDailyStore(db)}).Handler()
	return b
}

func (b *editBench) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events, b.alerts = nil, nil
}

func (b *editBench) eventsOf(eventType string) []json.RawMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []json.RawMessage
	for _, e := range b.events {
		if e.Type == eventType {
			out = append(out, e.Data)
		}
	}
	return out
}

func (b *editBench) assertNoAlertRaisedOrResolved(t *testing.T) {
	t.Helper()
	b.mu.Lock()
	alerts := append([]string(nil), b.alerts...)
	b.mu.Unlock()
	assert.Empty(t, alerts, "an edit alone neither raises nor resolves an alert")
	assert.Empty(t, b.eventsOf(event.HeartbeatAlert), "an edit alone sends no heartbeat.alert")
	assert.Empty(t, b.eventsOf(event.HeartbeatRecovery), "an edit alone sends no heartbeat.recovery")
}

func (b *editBench) create(t *testing.T, name string, interval, grace int) heartbeat.Heartbeat {
	t.Helper()
	rec := serve(t, b.api, http.MethodPost, "/api/v1/heartbeats", dashboardBody(name, interval, grace))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var hb heartbeat.Heartbeat
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hb))
	return hb
}

func (b *editBench) put(t *testing.T, id, body string) (int, []byte) {
	t.Helper()
	rec := serve(t, b.api, http.MethodPut, "/api/v1/heartbeats/"+id, body)
	return rec.Code, rec.Body.Bytes()
}

func (b *editBench) edit(t *testing.T, id, body string) heartbeat.Heartbeat {
	t.Helper()
	code, raw := b.put(t, id, body)
	require.Equal(t, http.StatusOK, code, string(raw))
	var hb heartbeat.Heartbeat
	require.NoError(t, json.Unmarshal(raw, &hb))
	return hb
}

func (b *editBench) ping(t *testing.T, token string) {
	t.Helper()
	rec := serve(t, b.api, http.MethodGet, "/ping/"+token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func (b *editBench) setState(t *testing.T, id string, status heartbeat.HeartbeatStatus, alert heartbeat.AlertState,
	lastPing, deadline, runStart *time.Time, failures, successes int) {
	t.Helper()
	require.NoError(t, b.hs.UpdateHeartbeatState(context.Background(), id, status, alert,
		lastPing, deadline, runStart, nil, nil, failures, successes))
}

// pingedAt leaves the heartbeat as a success ping at `at` does.
func (b *editBench) pingedAt(t *testing.T, hb heartbeat.Heartbeat, at time.Time) {
	t.Helper()
	b.setState(t, hb.ID, heartbeat.StatusUp, heartbeat.AlertNormal, ptrTime(at), ptrTime(at.Add(window(hb))), nil, 0, 4)
}

// startedAt leaves the heartbeat as a /start at runStart does, after a success ping at lastPing.
func (b *editBench) startedAt(t *testing.T, hb heartbeat.Heartbeat, lastPing, runStart time.Time) {
	t.Helper()
	b.setState(t, hb.ID, heartbeat.StatusStarted, heartbeat.AlertNormal, ptrTime(lastPing), ptrTime(runStart.Add(window(hb))), ptrTime(runStart), 0, 4)
}

// resumedAt pauses the heartbeat at pausedAt and resumes it at resumedAt, as the service does.
func (b *editBench) resumedAt(t *testing.T, hb heartbeat.Heartbeat, pausedAt, resumedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, b.hs.PauseHeartbeat(ctx, hb.ID, pausedAt))
	require.NoError(t, b.hs.ResumeHeartbeat(ctx, hb.ID, resumedAt, resumedAt.Add(window(hb))))
}

// downAfter leaves the heartbeat as the deadline checker does once the deadline of a ping at lastPing has passed.
func (b *editBench) downAfter(t *testing.T, hb heartbeat.Heartbeat, lastPing time.Time) {
	t.Helper()
	b.setState(t, hb.ID, heartbeat.StatusDown, heartbeat.AlertAlerting, ptrTime(lastPing), ptrTime(lastPing.Add(window(hb))), nil, 2, 0)
}

func dashboardBody(name string, interval, grace int) string {
	return fmt.Sprintf(`{"name":%q,"interval_seconds":%d,"grace_seconds":%d}`, name, interval, grace)
}

func window(hb heartbeat.Heartbeat) time.Duration {
	return time.Duration(hb.IntervalSeconds+hb.GraceSeconds) * time.Second
}

func ptrTime(at time.Time) *time.Time { return &at }

// ago returns a whole second in the past: the store keeps epoch seconds.
func ago(d time.Duration) time.Time { return time.Unix(time.Now().Add(-d).Unix(), 0) }

func unixOrNil(at *time.Time) any {
	if at == nil {
		return nil
	}
	return at.Unix()
}

func deadlineOf(t *testing.T, hb heartbeat.Heartbeat) int64 {
	t.Helper()
	require.NotNil(t, hb.NextDeadlineAt, "%s has a deadline", hb.Name)
	return hb.NextDeadlineAt.Unix()
}

// US1 scenario 3, FR-002, SC-002, edge case "rotation stays a separate action": an edit keeps the id,
// the token and the ping URL, even when the body carries other values for them.
func TestHeartbeatEdit_KeepsTheIDTheTokenAndThePingURL(t *testing.T) {
	b := newEditBench(t)
	for _, c := range []struct {
		name, newName                        string
		interval, grace, newInterval, newGrace int
	}{
		{"nightly backup", "nightly db backup", 86400, 600, 43200, 300},
		{"hourly sync", "sync eu", 3600, 60, 900, 0},
	} {
		hb := b.create(t, c.name, c.interval, c.grace)
		b.ping(t, hb.PingToken)

		body := fmt.Sprintf(`{"id":%q,"ping_token":%q,"name":%q,"interval_seconds":%d,"grace_seconds":%d}`,
			uid.New(), uid.New(), c.newName, c.newInterval, c.newGrace)
		got := b.edit(t, hb.ID, body)
		assert.Equal(t, hb.ID, got.ID, "%s: the id does not change", c.name)
		assert.Equal(t, hb.PingToken, got.PingToken, "%s: the token does not change", c.name)
		assert.Equal(t, c.newName, got.Name)
		assert.Equal(t, c.newInterval, got.IntervalSeconds)
		assert.Equal(t, c.newGrace, got.GraceSeconds)

		detail := getHeartbeat(t, b.api, hb.ID)
		assert.Equal(t, hb.PingToken, detail.PingToken, "%s: the stored token does not change", c.name)
		assert.Equal(t, c.newName, detail.Name)

		b.ping(t, hb.PingToken)
		assert.Equal(t, 2, pingCount(t, b.svc, hb.ID), "%s: the ping URL of before the edit still records on the same heartbeat", c.name)
		assert.Equal(t, heartbeat.StatusUp, getHeartbeat(t, b.api, hb.ID).Status)
	}
}

// US1 scenario 4, FR-003, SC-004: pings, executions, pauses and the daily uptime outlive the edit.
func TestHeartbeatEdit_KeepsPingsExecutionsPausesAndUptimeHistory(t *testing.T) {
	b := newEditBench(t)
	ctx := context.Background()
	hb := b.create(t, "nightly backup", 3600, 300)

	for h := 72; h >= 1; h-- {
		_, err := b.hs.InsertPing(ctx, &heartbeat.HeartbeatPing{HeartbeatID: hb.ID, PingType: heartbeat.PingSuccess,
			SourceIP: "10.0.0.1", HTTPMethod: "GET", Timestamp: ago(time.Duration(h) * time.Hour)})
		require.NoError(t, err)
	}
	for _, path := range []string{"/start", "/0", ""} {
		rec := serve(t, b.api, http.MethodGet, "/ping/"+hb.PingToken+path, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}
	for _, action := range []string{"pause", "resume"} {
		rec := serve(t, b.api, http.MethodPost, "/api/v1/heartbeats/"+hb.ID+"/"+action, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	pings, executions, pauses, days := history(t, b, hb.ID)
	require.Equal(t, 75, pings, "fixture: 72 stored pings and 3 through /ping")
	require.Equal(t, 2, executions, "fixture: one start/finish run and one plain ping")
	require.Equal(t, 1, pauses)
	require.GreaterOrEqual(t, len(days), 2, "fixture: several days of uptime history")

	b.edit(t, hb.ID, dashboardBody("nightly db backup", 86400, 600))

	afterPings, afterExecutions, afterPauses, afterDays := history(t, b, hb.ID)
	assert.Equal(t, pings, afterPings, "no ping disappears")
	assert.Equal(t, executions, afterExecutions, "no execution disappears")
	assert.Equal(t, pauses, afterPauses, "no pause disappears")
	assert.Equal(t, days, afterDays, "no day of uptime history disappears")
}

func history(t *testing.T, b *editBench, id string) (pings, executions, pauses int, daysWithData []string) {
	t.Helper()
	var list struct {
		Total int `json:"total"`
	}
	rec := serve(t, b.api, http.MethodGet, "/api/v1/heartbeats/"+id+"/pings?limit=500", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	pings = list.Total

	rec = serve(t, b.api, http.MethodGet, "/api/v1/heartbeats/"+id+"/executions?limit=500", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	executions = list.Total

	require.NoError(t, b.db.Reader().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM heartbeat_pauses WHERE heartbeat_id = ?`, id).Scan(&pauses))

	rec = serve(t, b.api, http.MethodGet, "/api/v1/heartbeats/"+id+"/uptime/daily?days=7", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var uptime struct {
		Days []store.DailyUptime `json:"days"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &uptime))
	for _, d := range uptime.Days {
		if d.UptimePercent != nil {
			daysWithData = append(daysWithData, d.Date)
		}
	}
	return pings, executions, pauses, daysWithData
}

// US2 scenarios 1, 2 and 3, FR-004, FR-009, SC-003: the deadline becomes start + new interval + new grace,
// the start being the last ping, the start of the current run or the resume, whichever the current deadline counts from.
func TestHeartbeatEdit_RecomputesTheDeadlineFromTheSameStartingPoint(t *testing.T) {
	cases := []struct {
		name                  string
		interval, grace       int
		arrange               func(t *testing.T, b *editBench, hb heartbeat.Heartbeat) time.Time
		newInterval, newGrace int
		status                heartbeat.HeartbeatStatus
	}{
		{
			name: "up, pinged 3h ago, one day shortened to one hour", interval: 86400, grace: 600,
			arrange: func(t *testing.T, b *editBench, hb heartbeat.Heartbeat) time.Time {
				at := ago(3 * time.Hour)
				b.pingedAt(t, hb, at)
				return at
			},
			newInterval: 3600, newGrace: 300, status: heartbeat.StatusUp,
		},
		{
			name: "up, pinged 20 min ago, one hour lengthened to two", interval: 3600, grace: 60,
			arrange: func(t *testing.T, b *editBench, hb heartbeat.Heartbeat) time.Time {
				at := ago(20 * time.Minute)
				b.pingedAt(t, hb, at)
				return at
			},
			newInterval: 7200, newGrace: 120, status: heartbeat.StatusUp,
		},
		{
			name: "started 10 min ago, last ping the day before", interval: 86400, grace: 600,
			arrange: func(t *testing.T, b *editBench, hb heartbeat.Heartbeat) time.Time {
				run := ago(10 * time.Minute)
				b.startedAt(t, hb, ago(26*time.Hour), run)
				return run
			},
			newInterval: 3600, newGrace: 60, status: heartbeat.StatusStarted,
		},
		{
			name: "resumed 2h ago and never pinged", interval: 300, grace: 60,
			arrange: func(t *testing.T, b *editBench, hb heartbeat.Heartbeat) time.Time {
				resumed := ago(2 * time.Hour)
				b.resumedAt(t, hb, ago(5*time.Hour), resumed)
				return resumed
			},
			newInterval: 21600, newGrace: 600, status: heartbeat.StatusUp,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newEditBench(t)
			hb := b.create(t, "job", tc.interval, tc.grace)
			start := tc.arrange(t, b, hb)
			before := getHeartbeat(t, b.api, hb.ID)
			want := start.Unix() + int64(tc.newInterval+tc.newGrace)

			got := b.edit(t, hb.ID, dashboardBody(before.Name, tc.newInterval, tc.newGrace))
			assert.Equal(t, want, deadlineOf(t, got), "PUT answers start + new interval + new grace")

			after := getHeartbeat(t, b.api, hb.ID)
			assert.Equal(t, want, deadlineOf(t, after), "the stored deadline is the recomputed one")
			assert.Equal(t, tc.newInterval, after.IntervalSeconds)
			assert.Equal(t, tc.newGrace, after.GraceSeconds)
			assert.Equal(t, tc.status, after.Status, "the edit does not change the status")
			assert.Equal(t, before.AlertState, after.AlertState)
			assert.Equal(t, unixOrNil(before.LastPingAt), unixOrNil(after.LastPingAt))
			assert.Equal(t, unixOrNil(before.CurrentRunStartedAt), unixOrNil(after.CurrentRunStartedAt))
			assert.Equal(t, before.ConsecutiveSuccesses, after.ConsecutiveSuccesses, "the counters are not reset")
			assert.Equal(t, before.ConsecutiveFailures, after.ConsecutiveFailures, "the counters are not reset")
		})
	}
}

// FR-009, contract api.md: an API update carrying only some fields recomputes the deadline the same way.
func TestHeartbeatEdit_APIPartialUpdateRecomputesTheSameWay(t *testing.T) {
	b := newEditBench(t)
	hb := b.create(t, "hourly sync", 3600, 60)
	at := ago(time.Hour)
	b.pingedAt(t, hb, at)

	steps := []struct {
		body            string
		interval, grace int
		name            string
	}{
		{`{"grace_seconds":120}`, 3600, 120, "hourly sync"},
		{`{"interval_seconds":7200}`, 7200, 120, "hourly sync"},
		{`{"interval_seconds":1800,"grace_seconds":0}`, 1800, 0, "hourly sync"},
		{`{"name":"hourly sync (eu)"}`, 1800, 0, "hourly sync (eu)"},
	}
	for _, s := range steps {
		got := b.edit(t, hb.ID, s.body)
		assert.Equal(t, s.name, got.Name, s.body)
		assert.Equal(t, s.interval, got.IntervalSeconds, s.body)
		assert.Equal(t, s.grace, got.GraceSeconds, s.body)
		assert.Equal(t, at.Unix()+int64(s.interval+s.grace), deadlineOf(t, got), "%s: last ping + interval + grace", s.body)
	}
}

// US2 scenario 8, edge case "no value changed": a rename, or the same values sent again, moves neither the deadline nor the state.
func TestHeartbeatEdit_NameOnlyOrUnchangedValuesKeepTheDeadlineAndState(t *testing.T) {
	cases := []struct {
		name            string
		interval, grace int
		arrange         func(t *testing.T, b *editBench, hb heartbeat.Heartbeat)
	}{
		{"up", 86400, 600, func(t *testing.T, b *editBench, hb heartbeat.Heartbeat) { b.pingedAt(t, hb, ago(3*time.Hour)) }},
		{"started", 3600, 60, func(t *testing.T, b *editBench, hb heartbeat.Heartbeat) {
			b.startedAt(t, hb, ago(2*time.Hour), ago(10*time.Minute))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newEditBench(t)
			hb := b.create(t, "nightly backup", tc.interval, tc.grace)
			tc.arrange(t, b, hb)
			before := getHeartbeat(t, b.api, hb.ID)

			for _, body := range []string{
				`{"name":"nightly db backup"}`,
				dashboardBody("nightly db backup", tc.interval, tc.grace),
				dashboardBody("nightly db backup", tc.interval, tc.grace),
			} {
				got := b.edit(t, hb.ID, body)
				assert.Equal(t, "nightly db backup", got.Name)
				assert.Equal(t, deadlineOf(t, before), deadlineOf(t, got), "%s: the deadline does not move", body)
				assert.Equal(t, before.Status, got.Status, body)
				assert.Equal(t, before.AlertState, got.AlertState, body)
				assert.Equal(t, unixOrNil(before.LastPingAt), unixOrNil(got.LastPingAt), body)
			}
		})
	}
}

// US2 scenario 5, FR-005, assumption "a heartbeat down stays down": the edit keeps it down and alerting,
// with its deadline; the next ping brings it back with the new values.
func TestHeartbeatEdit_DownStaysDownUntilTheNextPing(t *testing.T) {
	b := newEditBench(t)
	hb := b.create(t, "nightly backup", 3600, 60)
	b.downAfter(t, hb, ago(3*time.Hour))
	before := getHeartbeat(t, b.api, hb.ID)
	b.reset()

	got := b.edit(t, hb.ID, dashboardBody("nightly backup", 86400, 600))
	assert.Equal(t, heartbeat.StatusDown, got.Status, "the new values would have avoided the miss, the heartbeat stays down all the same")
	assert.Equal(t, heartbeat.AlertAlerting, got.AlertState)
	assert.Equal(t, deadlineOf(t, before), deadlineOf(t, got), "a heartbeat down keeps its deadline")
	assert.Equal(t, 2, got.ConsecutiveFailures, "the counters are not reset")
	b.assertNoAlertRaisedOrResolved(t)
	assert.Empty(t, b.eventsOf(event.HeartbeatStatusChanged), "the edit does not change the status")

	from := time.Now().Unix()
	b.ping(t, hb.PingToken)
	to := time.Now().Unix()
	after := getHeartbeat(t, b.api, hb.ID)
	assert.Equal(t, heartbeat.StatusUp, after.Status, "the next ping brings it back as usual")
	assert.Equal(t, heartbeat.AlertNormal, after.AlertState)
	assert.GreaterOrEqual(t, deadlineOf(t, after), from+86400+600)
	assert.LessOrEqual(t, deadlineOf(t, after), to+86400+600)
}

// US2 scenarios 6 and 7, FR-005: a heartbeat never pinged stays waiting without deadline; a paused one stays paused,
// and its resume counts the deadline with the new values.
func TestHeartbeatEdit_NewAndPausedHeartbeatsKeepTheirStateWithoutDeadline(t *testing.T) {
	t.Run("never pinged", func(t *testing.T) {
		b := newEditBench(t)
		hb := b.create(t, "weekly report", 604800, 3600)
		got := b.edit(t, hb.ID, dashboardBody("weekly report", 86400, 600))
		assert.Equal(t, heartbeat.StatusNew, got.Status)
		assert.Nil(t, got.NextDeadlineAt, "still waiting for its first ping, without deadline")
		assert.Nil(t, got.LastPingAt)
		b.assertNoAlertRaisedOrResolved(t)
	})

	t.Run("paused", func(t *testing.T) {
		b := newEditBench(t)
		hb := b.create(t, "nightly backup", 3600, 60)
		b.pingedAt(t, hb, ago(time.Hour))
		rec := serve(t, b.api, http.MethodPost, "/api/v1/heartbeats/"+hb.ID+"/pause", "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		got := b.edit(t, hb.ID, dashboardBody("nightly backup", 21600, 900))
		assert.Equal(t, heartbeat.StatusPaused, got.Status)
		assert.Nil(t, got.NextDeadlineAt, "a paused heartbeat has no deadline")

		from := time.Now().Unix()
		rec = serve(t, b.api, http.MethodPost, "/api/v1/heartbeats/"+hb.ID+"/resume", "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		to := time.Now().Unix()
		after := getHeartbeat(t, b.api, hb.ID)
		assert.Equal(t, heartbeat.StatusUp, after.Status)
		assert.GreaterOrEqual(t, deadlineOf(t, after), from+21600+900, "the resume counts with the new values")
		assert.LessOrEqual(t, deadlineOf(t, after), to+21600+900)
	})
}

// Edge case "a ping arrives while the form is open": the recomputed deadline counts from that ping,
// not from the one displayed when the form was opened.
func TestHeartbeatEdit_APingDuringTheEditMovesTheStartingPoint(t *testing.T) {
	b := newEditBench(t)
	hb := b.create(t, "nightly backup", 86400, 600)
	b.pingedAt(t, hb, ago(3*time.Hour))
	opened := getHeartbeat(t, b.api, hb.ID)

	from := time.Now().Unix()
	b.ping(t, hb.PingToken)
	to := time.Now().Unix()

	got := b.edit(t, hb.ID, dashboardBody(opened.Name, 3600, 60))
	assert.GreaterOrEqual(t, deadlineOf(t, got), from+3660, "the deadline counts from the ping received during the edit")
	assert.LessOrEqual(t, deadlineOf(t, got), to+3660, "the deadline counts from the ping received during the edit")
}

// US3 scenarios 1 to 4, FR-007, FR-008, contract api.md: the creation rules apply, a refusal names what to fix
// and changes no field, even the valid ones sent with it.
func TestHeartbeatEdit_RefusedValuesChangeNothing(t *testing.T) {
	refused := []struct {
		name, body string
		mentions   []string
	}{
		{"empty name", `{"name":""}`, []string{"name"}},
		{"name of 256 characters", `{"name":"` + strings.Repeat("n", 256) + `"}`, []string{"name", "255"}},
		{"interval under one minute", `{"interval_seconds":59}`, []string{"interval", "60", "604800"}},
		{"interval over seven days", `{"interval_seconds":604801}`, []string{"interval", "60", "604800"}},
		{"dashboard body with an interval under one minute", `{"name":"renamed","interval_seconds":30,"grace_seconds":10}`, []string{"interval", "60", "604800"}},
		{"negative grace", `{"grace_seconds":-1}`, []string{"grace", "3600"}},
		{"grace above the current interval", `{"grace_seconds":3601}`, []string{"grace", "3600"}},
		{"dashboard body with a grace above the chosen interval", `{"name":"renamed","interval_seconds":300,"grace_seconds":301}`, []string{"grace", "300"}},
		{"interval lowered under the current grace", `{"interval_seconds":300}`, []string{"grace", "300"}},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			b := newEditBench(t)
			hb := b.create(t, "nightly backup", 3600, 600)
			b.pingedAt(t, hb, ago(30*time.Minute))
			before := getHeartbeat(t, b.api, hb.ID)

			code, raw := b.put(t, hb.ID, tc.body)
			require.Equal(t, http.StatusBadRequest, code, string(raw))
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(raw, &body))
			assert.Equal(t, "INVALID_INPUT", body.Error.Code)
			for _, m := range tc.mentions {
				assert.Contains(t, strings.ToLower(body.Error.Message), m, "the message says what to fix")
			}

			after := getHeartbeat(t, b.api, hb.ID)
			assert.Equal(t, before.Name, after.Name, "a refused edit changes no field")
			assert.Equal(t, before.IntervalSeconds, after.IntervalSeconds)
			assert.Equal(t, before.GraceSeconds, after.GraceSeconds)
			assert.Equal(t, deadlineOf(t, before), deadlineOf(t, after))
			assert.Equal(t, before.Status, after.Status)
		})
	}

	accepted := []struct {
		name, body      string
		interval, grace int
		nameLen         int
	}{
		{"name of 255 characters", `{"name":"` + strings.Repeat("n", 255) + `"}`, 3600, 600, 255},
		{"one minute with a grace equal to it", `{"interval_seconds":60,"grace_seconds":60}`, 60, 60, len("nightly backup")},
		{"seven days without grace", `{"interval_seconds":604800,"grace_seconds":0}`, 604800, 0, len("nightly backup")},
	}
	for _, tc := range accepted {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			b := newEditBench(t)
			hb := b.create(t, "nightly backup", 3600, 600)
			got := b.edit(t, hb.ID, tc.body)
			assert.Len(t, got.Name, tc.nameLen)
			assert.Equal(t, tc.interval, got.IntervalSeconds)
			assert.Equal(t, tc.grace, got.GraceSeconds)
		})
	}
}

// Edge case "deleted in another tab": the save answers 404 and does not bring the heartbeat back.
func TestHeartbeatEdit_DeletedHeartbeatAnswersNotFound(t *testing.T) {
	b := newEditBench(t)
	hb := b.create(t, "nightly backup", 3600, 60)
	rec := serve(t, b.api, http.MethodDelete, "/api/v1/heartbeats/"+hb.ID, "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	for _, id := range []string{hb.ID, uid.New()} {
		code, raw := b.put(t, id, dashboardBody("nightly backup", 7200, 120))
		assert.Equal(t, http.StatusNotFound, code, string(raw))
		var body ErrorResponse
		require.NoError(t, json.Unmarshal(raw, &body))
		assert.Equal(t, "NOT_FOUND", body.Error.Code)
	}
	rec = serve(t, b.api, http.MethodGet, "/api/v1/heartbeats/"+hb.ID, "")
	assert.Equal(t, http.StatusNotFound, rec.Code, "the failed edit does not bring the heartbeat back")
}

// Edge case "heartbeat of a remote agent": it is edited the same way, and the agent keeps pinging the same token.
func TestHeartbeatEdit_RemoteAgentHeartbeatIsEditedTheSameWay(t *testing.T) {
	b := newEditBench(t)
	ctx := context.Background()
	agentID := uid.New()
	require.NoError(t, store.NewAgentStore(b.db).Insert(ctx, &agent.Agent{
		AgentID: agentID, Hostname: "edge-1", Label: "edge-1", OSArch: "linux/amd64",
		AgentVersion: "1.0.0", DetectedRuntime: "docker", Status: "active", CreatedAt: time.Now().UTC(),
	}))
	seed := &heartbeat.Heartbeat{ID: uid.New(), PingToken: uid.New(), AgentID: agentID, Name: "edge cron", IntervalSeconds: 3600, GraceSeconds: 60}
	_, err := b.hs.CreateHeartbeat(ctx, seed)
	require.NoError(t, err)
	hb := getHeartbeat(t, b.api, seed.ID)
	require.Equal(t, agentID, hb.AgentID)
	at := ago(30 * time.Minute)
	b.pingedAt(t, hb, at)

	got := b.edit(t, hb.ID, dashboardBody("edge cron (eu)", 7200, 120))
	assert.Equal(t, agentID, got.AgentID, "the heartbeat stays attached to its agent")
	assert.Equal(t, seed.PingToken, got.PingToken)
	assert.Equal(t, "edge cron (eu)", got.Name)
	assert.Equal(t, at.Unix()+7200+120, deadlineOf(t, got), "the deadline is recomputed as for a local heartbeat")

	require.NoError(t, b.svc.HandleAgentEvent(ctx, agentID, &agentpb.HeartbeatEvent{HeartbeatToken: seed.PingToken, SourceIp: "10.1.2.3"}))
	after := getHeartbeat(t, b.api, hb.ID)
	assert.Equal(t, heartbeat.StatusUp, after.Status)
	assert.Equal(t, agentID, after.AgentID)
	assert.Equal(t, 1, pingCount(t, b.svc, hb.ID), "the ping relayed by the agent lands on the same heartbeat")
}

type updatedPayload struct {
	HeartbeatID     string     `json:"heartbeat_id"`
	Name            string     `json:"name"`
	IntervalSeconds int        `json:"interval_seconds"`
	GraceSeconds    int        `json:"grace_seconds"`
	NextDeadlineAt  *time.Time `json:"next_deadline_at"`
}

func onlyUpdatedEvent(t *testing.T, b *editBench) (updatedPayload, map[string]json.RawMessage, string) {
	t.Helper()
	events := b.eventsOf(event.HeartbeatUpdated)
	require.Len(t, events, 1, "one heartbeat.updated per successful edit")
	var p updatedPayload
	require.NoError(t, json.Unmarshal(events[0], &p), string(events[0]))
	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(events[0], &keys), string(events[0]))
	return p, keys, string(events[0])
}

// FR-010, FR-009, contract sse.md: every successful edit, from the dashboard or the API, broadcasts heartbeat.updated
// with the new values and the recomputed deadline, never the ping token.
func TestHeartbeatEdit_BroadcastsHeartbeatUpdatedWithoutThePingToken(t *testing.T) {
	b := newEditBench(t)
	hb := b.create(t, "nightly backup", 86400, 600)
	at := ago(2 * time.Hour)
	b.pingedAt(t, hb, at)
	b.reset()

	got := b.edit(t, hb.ID, dashboardBody("db dump", 3600, 120))
	p, keys, raw := onlyUpdatedEvent(t, b)
	assert.Equal(t, hb.ID, p.HeartbeatID)
	assert.Equal(t, "db dump", p.Name)
	assert.Equal(t, 3600, p.IntervalSeconds)
	assert.Equal(t, 120, p.GraceSeconds)
	require.NotNil(t, p.NextDeadlineAt, raw)
	assert.Equal(t, at.Unix()+3720, p.NextDeadlineAt.Unix(), "the event carries the recomputed deadline")
	assert.Equal(t, deadlineOf(t, got), p.NextDeadlineAt.Unix())
	assert.NotContains(t, raw, hb.PingToken, "the ping token is never broadcast")
	assert.NotContains(t, keys, "ping_token")
	b.assertNoAlertRaisedOrResolved(t)

	b.reset()
	b.edit(t, hb.ID, `{"grace_seconds":300}`)
	p, _, raw = onlyUpdatedEvent(t, b)
	assert.Equal(t, hb.ID, p.HeartbeatID)
	assert.Equal(t, "db dump", p.Name, "a partial API edit broadcasts the full set of values")
	assert.Equal(t, 3600, p.IntervalSeconds)
	assert.Equal(t, 300, p.GraceSeconds)
	require.NotNil(t, p.NextDeadlineAt, raw)
	assert.Equal(t, at.Unix()+3900, p.NextDeadlineAt.Unix())
	assert.NotContains(t, raw, hb.PingToken)

	fresh := b.create(t, "weekly report", 604800, 3600)
	b.reset()
	b.edit(t, fresh.ID, dashboardBody("weekly report", 86400, 600))
	p, keys, raw = onlyUpdatedEvent(t, b)
	assert.Equal(t, fresh.ID, p.HeartbeatID)
	assert.Equal(t, 86400, p.IntervalSeconds)
	assert.NotContains(t, keys, "next_deadline_at", "a heartbeat without deadline broadcasts none: %s", raw)
	assert.NotContains(t, raw, fresh.PingToken)
}

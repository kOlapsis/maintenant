// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
)

// DailyUptime is the uptime of one monitor over one UTC day.
type DailyUptime struct {
	Date          string   `json:"date"`
	UptimePercent *float64 `json:"uptime_percent"`
	IncidentCount int      `json:"incident_count"`
}

// UptimeDailyStore serves per-day uptime: completed days from the daily
// aggregates, the days not aggregated yet computed from the raw rows.
type UptimeDailyStore struct {
	db     *Reader
	writer *Writer
}

// NewUptimeDailyStore creates a new daily uptime store.
func NewUptimeDailyStore(d *DB) *UptimeDailyStore {
	return &UptimeDailyStore{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

const (
	maxUptimeDays    = 365
	secondsPerDay    = 86400
	uptimeDay        = 24 * time.Hour
	incidentLookback = 24 * time.Hour // how far back the check preceding a day is searched for
)

type dayUptime struct {
	percent   float64
	incidents int
}

// dayUptimes maps the UTC midnight of a day, in epoch seconds, to its uptime.
type dayUptimes map[int64]dayUptime

// computeDays returns the uptime of monitor id for each day of [from, to) that
// has raw data; from is a UTC midnight.
type computeDays func(ctx context.Context, id string, from, to time.Time) (dayUptimes, error)

type uptimeTable struct {
	name   string
	column string
}

var (
	endpointUptimeTable  = uptimeTable{name: "endpoint_uptime_daily", column: "endpoint_id"}
	heartbeatUptimeTable = uptimeTable{name: "heartbeat_uptime_daily", column: "heartbeat_id"}
	containerUptimeTable = uptimeTable{name: "container_uptime_daily", column: "container_id"}
)

// GetEndpointDailyUptime returns one entry per day up to the day of now, most recent first.
func (s *UptimeDailyStore) GetEndpointDailyUptime(ctx context.Context, endpointID string, days int, now time.Time) ([]DailyUptime, error) {
	return s.daily(ctx, endpointUptimeTable, endpointID, days, now, s.endpointDays)
}

// GetHeartbeatDailyUptime returns one entry per day up to the day of now, most recent first.
func (s *UptimeDailyStore) GetHeartbeatDailyUptime(ctx context.Context, heartbeatID string, days int, now time.Time) ([]DailyUptime, error) {
	return s.daily(ctx, heartbeatUptimeTable, heartbeatID, days, now, s.heartbeatDays)
}

// GetContainerDailyUptime returns one entry per day up to the day of now, most recent first.
func (s *UptimeDailyStore) GetContainerDailyUptime(ctx context.Context, containerID string, days int, now time.Time) ([]DailyUptime, error) {
	until, err := s.containerUntil(ctx, containerID)
	if err != nil {
		return nil, err
	}
	return s.daily(ctx, containerUptimeTable, containerID, days, now,
		func(ctx context.Context, id string, from, to time.Time) (dayUptimes, error) {
			if until != nil && until.Before(to) {
				to = *until
			}
			return s.containerDays(ctx, id, from, to)
		})
}

func (s *UptimeDailyStore) daily(ctx context.Context, t uptimeTable, id string, days int, now time.Time, compute computeDays) ([]DailyUptime, error) {
	days = clampUptimeDays(days)

	now = now.UTC()
	today := startOfUTCDay(now)
	windowStart := today.AddDate(0, 0, -(days - 1))

	stored, err := s.storedDays(ctx, t, id, windowStart, today)
	if err != nil {
		return nil, err
	}

	computeFrom := windowStart
	for d := range stored {
		if next := time.Unix(d, 0).UTC().Add(uptimeDay); next.After(computeFrom) {
			computeFrom = next
		}
	}
	computed, err := compute(ctx, id, computeFrom, now)
	if err != nil {
		return nil, err
	}

	result := make([]DailyUptime, 0, days)
	for i := 0; i < days; i++ {
		day := today.AddDate(0, 0, -i)
		du := DailyUptime{Date: day.Format("2006-01-02")}
		v, ok := stored[day.Unix()]
		if !ok {
			v, ok = computed[day.Unix()]
		}
		if ok {
			pct := v.percent
			du.UptimePercent = &pct
			du.IncidentCount = v.incidents
		}
		result = append(result, du)
	}
	return result, nil
}

func (s *UptimeDailyStore) storedDays(ctx context.Context, t uptimeTable, id string, from, to time.Time) (dayUptimes, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT day, uptime_percent, incident_count FROM `+t.name+`
		WHERE `+t.column+` = ? AND day >= ? AND day < ?`,
		id, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", t.name, err)
	}
	defer func() { _ = rows.Close() }()

	out := dayUptimes{}
	for rows.Next() {
		var day int64
		var v dayUptime
		if err := rows.Scan(&day, &v.percent, &v.incidents); err != nil {
			return nil, fmt.Errorf("scan %s: %w", t.name, err)
		}
		out[day] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", t.name, err)
	}
	return out, nil
}

// endpointDays counts successful checks per day; an incident is a failed check
// following a successful one.
func (s *UptimeDailyStore) endpointDays(ctx context.Context, id string, from, to time.Time) (dayUptimes, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp, success FROM check_results
		WHERE endpoint_id = ? AND timestamp >= ? AND timestamp < ?
		ORDER BY timestamp`,
		id, from.Add(-incidentLookback).Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("endpoint daily uptime: %w", err)
	}
	defer func() { _ = rows.Close() }()

	agg := newCheckDays(from)
	for rows.Next() {
		var ts int64
		var success int
		if err := rows.Scan(&ts, &success); err != nil {
			return nil, fmt.Errorf("scan endpoint daily uptime: %w", err)
		}
		agg.add(ts, success != 0)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate endpoint daily uptime: %w", err)
	}
	return agg.result(), nil
}

// heartbeatDays replays the pings and pauses as the service does (every ping resets the deadline, a completion sets up or down, a lapsed deadline is down, a pause counts for nothing) and weighs each day by its time up.
func (s *UptimeDailyStore) heartbeatDays(ctx context.Context, id string, from, to time.Time) (dayUptimes, error) {
	out := dayUptimes{}
	if !to.After(from) {
		return out, nil
	}

	var period int64
	err := s.db.QueryRowContext(ctx,
		`SELECT interval_seconds + grace_seconds FROM heartbeats WHERE id = ?`, id).Scan(&period)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return out, nil
	case err != nil:
		return nil, fmt.Errorf("heartbeat daily uptime: %w", err)
	}

	// Replay from the last completion before the window: it fixes the status the window opens on.
	var seed sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(timestamp) FROM heartbeat_pings
		WHERE heartbeat_id = ? AND ping_type IN ('success', 'exit_code') AND timestamp < ?`,
		id, from.Unix()).Scan(&seed); err != nil {
		return nil, fmt.Errorf("heartbeat daily uptime seed: %w", err)
	}

	events, err := s.heartbeatPingEvents(ctx, id, seed.Int64, to.Unix())
	if err != nil {
		return nil, err
	}
	pauses, err := s.heartbeatPauseEvents(ctx, id, seed.Int64, to.Unix())
	if err != nil {
		return nil, err
	}
	events = append(events, pauses...)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].ts != events[j].ts {
			return events[i].ts < events[j].ts
		}
		return events[i].kind < events[j].kind
	})

	tl := &hbTimeline{period: period}
	for _, e := range events {
		switch e.kind {
		case hbResume:
			tl.resume(e.ts)
		case hbPing:
			tl.ping(e.ts, e.pingType, e.exitCode)
		case hbPause:
			tl.pause(e.ts)
		}
	}
	if !tl.started {
		return out, nil
	}
	tl.close(to.Unix())

	dataStart := max(tl.first, from.Unix())
	for day := from.Unix(); day < to.Unix(); day += secondsPerDay {
		start, end := max(day, dataStart), min(day+secondsPerDay, to.Unix())
		if end <= start {
			continue
		}
		if v, ok := tl.over(start, end); ok {
			out[day] = v
		}
	}
	return out, nil
}

func (s *UptimeDailyStore) heartbeatPingEvents(ctx context.Context, id string, from, to int64) ([]hbEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp, ping_type, exit_code FROM heartbeat_pings
		WHERE heartbeat_id = ? AND timestamp >= ? AND timestamp < ?
		ORDER BY timestamp`,
		id, from, to)
	if err != nil {
		return nil, fmt.Errorf("heartbeat daily uptime: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []hbEvent
	for rows.Next() {
		e := hbEvent{kind: hbPing}
		if err := rows.Scan(&e.ts, &e.pingType, &e.exitCode); err != nil {
			return nil, fmt.Errorf("scan heartbeat daily uptime: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate heartbeat daily uptime: %w", err)
	}
	return events, nil
}

// heartbeatPauseEvents returns the start and the end of every pause that overlaps [from, to).
func (s *UptimeDailyStore) heartbeatPauseEvents(ctx context.Context, id string, from, to int64) ([]hbEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT paused_at, resumed_at FROM heartbeat_pauses
		WHERE heartbeat_id = ? AND paused_at < ? AND (resumed_at IS NULL OR (resumed_at > ? AND resumed_at > paused_at))`,
		id, to, from)
	if err != nil {
		return nil, fmt.Errorf("heartbeat daily uptime pauses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []hbEvent
	for rows.Next() {
		var pausedAt int64
		var resumedAt sql.NullInt64
		if err := rows.Scan(&pausedAt, &resumedAt); err != nil {
			return nil, fmt.Errorf("scan heartbeat daily uptime pauses: %w", err)
		}
		events = append(events, hbEvent{ts: pausedAt, kind: hbPause})
		if resumedAt.Valid {
			events = append(events, hbEvent{ts: resumedAt.Int64, kind: hbResume})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate heartbeat daily uptime pauses: %w", err)
	}
	return events, nil
}

// Within one second, a resume comes before the ping that caused it, and a pause after the ping it follows.
const (
	hbResume = iota
	hbPing
	hbPause
)

type hbEvent struct {
	ts       int64
	kind     int
	pingType string
	exitCode sql.NullInt64
}

type hbState int8

const (
	hbDown hbState = iota
	hbUp
	hbPaused
)

type hbTimeline struct {
	period    int64
	started   bool
	first     int64
	up        bool
	paused    bool
	cursor    int64
	deadline  int64
	spans     []hbSpan
	incidents []int64
}

type hbSpan struct {
	from, to int64
	state    hbState
}

func (t *hbTimeline) ping(ts int64, pingType string, exitCode sql.NullInt64) {
	switch {
	case !t.started:
		t.started, t.first, t.up, t.cursor = true, ts, true, ts
	case t.paused:
		t.resume(ts)
	default:
		t.advance(ts)
	}
	switch {
	case pingType == "success" || (pingType == "exit_code" && exitCode.Valid && exitCode.Int64 == 0):
		t.up = true
	case pingType == "exit_code":
		if t.up {
			t.incidents = append(t.incidents, ts)
		}
		t.up = false
	}
	t.deadline = ts + t.period
}

// pause stops the clock: until the resume, time counts neither up nor down.
func (t *hbTimeline) pause(ts int64) {
	if !t.started || t.paused {
		return
	}
	t.advance(ts)
	t.paused = true
}

// resume puts the heartbeat back up with a fresh deadline, as the service does.
func (t *hbTimeline) resume(ts int64) {
	if !t.paused {
		return
	}
	t.span(t.cursor, ts, hbPaused)
	t.cursor, t.paused, t.up, t.deadline = ts, false, true, ts+t.period
}

// advance records the time up to ts, going down where the deadline lapsed first.
func (t *hbTimeline) advance(ts int64) {
	if t.up && ts > t.deadline {
		t.span(t.cursor, t.deadline, hbUp)
		t.incidents = append(t.incidents, t.deadline)
		t.cursor, t.up = t.deadline, false
	}
	state := hbDown
	if t.up {
		state = hbUp
	}
	t.span(t.cursor, ts, state)
	t.cursor = ts
}

func (t *hbTimeline) span(from, to int64, state hbState) {
	if n := len(t.spans); n > 0 && t.spans[n-1].state == state && t.spans[n-1].to == from {
		t.spans[n-1].to = to
		return
	}
	t.spans = append(t.spans, hbSpan{from, to, state})
}

func (t *hbTimeline) close(end int64) {
	switch {
	case end <= t.cursor:
	case t.paused:
		t.span(t.cursor, end, hbPaused)
	default:
		t.advance(end)
	}
}

// over weighs [start, end) by its time up, paused time left out; a span paused throughout has no value.
func (t *hbTimeline) over(start, end int64) (dayUptime, bool) {
	var up, paused int64
	for _, sp := range t.spans {
		d := max(0, min(sp.to, end)-max(sp.from, start))
		switch sp.state {
		case hbUp:
			up += d
		case hbPaused:
			paused += d
		}
	}
	observed := end - start - paused
	if observed <= 0 {
		return dayUptime{}, false
	}
	n := 0
	for _, ts := range t.incidents {
		if ts >= start && ts < end {
			n++
		}
	}
	return dayUptime{
		percent:   math.Round(float64(up)/float64(observed)*10000) / 100,
		incidents: n,
	}, true
}

// containerDays weighs each day by the time spent up, starting from the last
// transition before the window.
func (s *UptimeDailyStore) containerDays(ctx context.Context, id string, from, to time.Time) (dayUptimes, error) {
	out := dayUptimes{}
	if !to.After(from) {
		return out, nil
	}

	transitions := make([]*container.StateTransition, 0)
	seed, err := scanTransitionRow(s.db.QueryRowContext(ctx,
		`SELECT `+transitionColumns+` FROM state_transitions
		 WHERE container_id = ? AND timestamp < ? ORDER BY timestamp DESC LIMIT 1`,
		id, from.Unix(),
	))
	switch {
	case err == nil:
		transitions = append(transitions, seed)
	case errors.Is(err, sql.ErrNoRows):
	default:
		return nil, fmt.Errorf("container daily uptime seed: %w", err)
	}
	seeded := len(transitions) > 0

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+transitionColumns+` FROM state_transitions
		 WHERE container_id = ? AND timestamp >= ? AND timestamp < ? ORDER BY timestamp ASC`,
		id, from.Unix(), to.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("container daily uptime: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		t, err := scanTransitionRow(rows)
		if err != nil {
			return nil, err
		}
		transitions = append(transitions, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate container daily uptime: %w", err)
	}

	if len(transitions) == 0 {
		return out, nil
	}
	dataStart := transitions[0].Timestamp
	if seeded {
		dataStart = from
	}

	for day := from; day.Before(to); day = day.Add(uptimeDay) {
		dayEnd := day.Add(uptimeDay)
		if dayEnd.After(to) {
			dayEnd = to
		}
		start := day
		if start.Before(dataStart) {
			start = dataStart
		}
		if !dayEnd.After(start) {
			continue
		}
		out[day.Unix()] = dayUptime{
			percent:   container.ComputeUptime(transitions, start, dayEnd),
			incidents: countContainerIncidents(transitions, start, dayEnd),
		}
	}
	return out, nil
}

// containerUntil returns when an archived container stopped existing, nil
// while it still runs.
func (s *UptimeDailyStore) containerUntil(ctx context.Context, id string) (*time.Time, error) {
	var archived int
	var archivedAt sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT archived, archived_at FROM containers WHERE id = ?`, id).
		Scan(&archived, &archivedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("container daily uptime: %w", err)
	case archived == 0 || !archivedAt.Valid:
		return nil, nil
	}
	until := time.Unix(archivedAt.Int64, 0).UTC()
	return &until, nil
}

func countContainerIncidents(transitions []*container.StateTransition, from, to time.Time) int {
	n := 0
	for _, t := range transitions {
		if t.Timestamp.Before(from) || !t.Timestamp.Before(to) {
			continue
		}
		prevUp := t.PreviousState == container.StateRunning &&
			(t.PreviousHealth == nil || *t.PreviousHealth != container.HealthUnhealthy)
		newUp := t.NewState == container.StateRunning &&
			(t.NewHealth == nil || *t.NewHealth != container.HealthUnhealthy)
		if prevUp && !newUp {
			n++
		}
	}
	return n
}

// checkDays tallies ordered up/down samples into days. Samples before from only
// tell whether the first sample of the window follows an up one.
type checkDays struct {
	from    int64
	prevUp  bool
	hasPrev bool
	counts  map[int64]*checkCount
}

type checkCount struct {
	up, total, incidents int
}

func newCheckDays(from time.Time) *checkDays {
	return &checkDays{from: from.Unix(), counts: map[int64]*checkCount{}}
}

func (c *checkDays) add(ts int64, up bool) {
	if ts >= c.from {
		day := ts - ts%secondsPerDay
		n, ok := c.counts[day]
		if !ok {
			n = &checkCount{}
			c.counts[day] = n
		}
		n.total++
		switch {
		case up:
			n.up++
		case c.hasPrev && c.prevUp:
			n.incidents++
		}
	}
	c.prevUp, c.hasPrev = up, true
}

func (c *checkDays) result() dayUptimes {
	out := make(dayUptimes, len(c.counts))
	for day, n := range c.counts {
		out[day] = dayUptime{
			percent:   math.Round(float64(n.up)/float64(n.total)*10000) / 100,
			incidents: n.incidents,
		}
	}
	return out
}

type uptimeMonitor struct {
	id    string
	until *time.Time
}

// rollupEndpoints aggregates the completed days of every endpoint.
func (s *UptimeDailyStore) rollupEndpoints(ctx context.Context, now time.Time, rawRetention time.Duration) error {
	monitors, err := s.monitors(ctx, `SELECT id, NULL FROM endpoints`)
	if err != nil {
		return err
	}
	return s.rollup(ctx, endpointUptimeTable, monitors, now, rawRetention, s.endpointDays)
}

// rollupHeartbeats aggregates the completed days of every heartbeat.
func (s *UptimeDailyStore) rollupHeartbeats(ctx context.Context, now time.Time, rawRetention time.Duration) error {
	monitors, err := s.monitors(ctx, `SELECT id, NULL FROM heartbeats`)
	if err != nil {
		return err
	}
	return s.rollup(ctx, heartbeatUptimeTable, monitors, now, rawRetention, s.heartbeatDays)
}

// rollupContainers aggregates the completed days of every container, up to its
// archival for an archived one.
func (s *UptimeDailyStore) rollupContainers(ctx context.Context, now time.Time, rawRetention time.Duration) error {
	monitors, err := s.monitors(ctx, `SELECT id, CASE WHEN archived = 1 THEN archived_at END FROM containers`)
	if err != nil {
		return err
	}
	return s.rollup(ctx, containerUptimeTable, monitors, now, rawRetention, s.containerDays)
}

func (s *UptimeDailyStore) monitors(ctx context.Context, query string) ([]uptimeMonitor, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list uptime monitors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []uptimeMonitor
	for rows.Next() {
		var m uptimeMonitor
		var until sql.NullInt64
		if err := rows.Scan(&m.id, &until); err != nil {
			return nil, fmt.Errorf("scan uptime monitor: %w", err)
		}
		if until.Valid {
			t := time.Unix(until.Int64, 0).UTC()
			m.until = &t
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate uptime monitors: %w", err)
	}
	return out, nil
}

// rollup writes the uptime of each completed day from the last aggregated one
// on. It starts no earlier than the first day whose raw rows the purge has not
// touched yet, so a day is never rewritten from partial data.
func (s *UptimeDailyStore) rollup(ctx context.Context, t uptimeTable, monitors []uptimeMonitor, now time.Time, rawRetention time.Duration, compute computeDays) error {
	today := startOfUTCDay(now)
	from := startOfUTCDay(now.Add(-rawRetention)).Add(uptimeDay)

	var last sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(day) FROM `+t.name).Scan(&last); err != nil {
		return fmt.Errorf("last day of %s: %w", t.name, err)
	}
	if last.Valid {
		if d := time.Unix(last.Int64, 0).UTC(); d.After(from) {
			from = d
		}
	}

	type aggregate struct {
		id  string
		day int64
		v   dayUptime
	}
	var rows []aggregate
	for _, m := range monitors {
		to := today
		if m.until != nil && m.until.Before(to) {
			to = *m.until
		}
		if !to.After(from) {
			continue
		}
		days, err := compute(ctx, m.id, from, to)
		if err != nil {
			return err
		}
		for day, v := range days {
			rows = append(rows, aggregate{id: m.id, day: day, v: v})
		}
	}
	if len(rows) == 0 {
		return nil
	}

	upsert := `INSERT INTO ` + t.name + ` (id, ` + t.column + `, day, uptime_percent, incident_count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(` + t.column + `, day) DO UPDATE SET
			uptime_percent = excluded.uptime_percent, incident_count = excluded.incident_count`
	return s.writer.Tx(ctx, func(ctx context.Context, tx *Tx) error {
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx, upsert, uid.New(), r.id, r.day, r.v.percent, r.v.incidents); err != nil {
				return fmt.Errorf("write %s: %w", t.name, err)
			}
		}
		return nil
	})
}

func (s *UptimeDailyStore) deleteBefore(ctx context.Context, t uptimeTable, before time.Time, o batchOpts) (int64, bool, error) {
	return deleteRowsBefore(ctx, s.writer, o, t.name, "day", before)
}

func startOfUTCDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func clampUptimeDays(days int) int {
	if days <= 0 {
		return 90
	}
	return min(days, maxUptimeDays)
}

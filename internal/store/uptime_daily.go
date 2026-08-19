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
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
)

// DailyUptime represents a single day's uptime aggregation.
type DailyUptime struct {
	Date          string   `json:"date"`
	UptimePercent *float64 `json:"uptime_percent"`
	IncidentCount int      `json:"incident_count"`
}

// UptimeDailyStore provides daily uptime aggregation queries.
type UptimeDailyStore struct {
	db *sql.DB
}

// NewUptimeDailyStore creates a new daily uptime store.
func NewUptimeDailyStore(d *DB) *UptimeDailyStore {
	return &UptimeDailyStore{
		db: d.ReadDB(),
	}
}

// GetEndpointDailyUptime aggregates endpoint check results by UTC day.
// Returns up to `days` days of data, most recent first.
// Days with no checks have UptimePercent = nil.
func (s *UptimeDailyStore) GetEndpointDailyUptime(ctx context.Context, endpointID string, days int) ([]DailyUptime, error) {
	if days <= 0 {
		days = 90
	}
	if days > 365 {
		days = 365
	}

	// Calculate the start of the window (beginning of the day N days ago in UTC).
	now := time.Now().UTC()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	windowStart := startOfToday.AddDate(0, 0, -(days - 1))

	// Query: aggregate check_results by UTC day.
	// the success column is 1 for success, 0 for failure.
	// incident_count = number of transitions from success to failure within the day.
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			date(timestamp, 'unixepoch') AS day,
			ROUND(CAST(SUM(success) AS REAL) / COUNT(*) * 100.0, 2) AS uptime_percent,
			COUNT(CASE WHEN success = 0 AND prev_success = 1 THEN 1 END) AS incident_count
		FROM (
			SELECT
				timestamp,
				success,
				LAG(success) OVER (ORDER BY timestamp) AS prev_success
			FROM check_results
			WHERE endpoint_id = ? AND timestamp >= ?
		)
		GROUP BY day
		ORDER BY day DESC
	`, endpointID, windowStart.Unix())
	if err != nil {
		return nil, fmt.Errorf("endpoint daily uptime: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	// Build a map of day -> DailyUptime from query results.
	dayMap := make(map[string]*DailyUptime)
	for rows.Next() {
		var du DailyUptime
		var uptimePct float64
		if err := rows.Scan(&du.Date, &uptimePct, &du.IncidentCount); err != nil {
			return nil, fmt.Errorf("scan endpoint daily uptime: %w", err)
		}
		du.UptimePercent = &uptimePct
		dayMap[du.Date] = &du
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate endpoint daily uptime: %w", err)
	}

	// Generate a full day range, filling gaps with null uptime.
	result := make([]DailyUptime, 0, days)
	for i := 0; i < days; i++ {
		day := startOfToday.AddDate(0, 0, -i)
		dateStr := day.Format("2006-01-02")
		if du, ok := dayMap[dateStr]; ok {
			result = append(result, *du)
		} else {
			result = append(result, DailyUptime{Date: dateStr, UptimePercent: nil, IncidentCount: 0})
		}
	}

	return result, nil
}

// GetContainerDailyUptime computes a per-day, time-weighted uptime series for a
// container from its state transitions. Unlike endpoints/heartbeats (discrete
// checks), container uptime is the running+healthy fraction of each day. Days
// before the first recorded transition return nil (no data), most recent first.
func (s *UptimeDailyStore) GetContainerDailyUptime(ctx context.Context, containerID string, days int) ([]DailyUptime, error) {
	if days <= 0 {
		days = 90
	}
	if days > 365 {
		days = 365
	}

	now := time.Now().UTC()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	windowStart := startOfToday.AddDate(0, 0, -(days - 1))

	// Seed with the last transition before the window so the earliest days know
	// the state they began in.
	transitions := make([]*container.StateTransition, 0)
	seed, err := scanTransitionRow(s.db.QueryRowContext(ctx,
		`SELECT `+transitionColumns+` FROM state_transitions
		 WHERE container_id = ? AND timestamp < ? ORDER BY timestamp DESC LIMIT 1`,
		containerID, windowStart.Unix(),
	))
	switch {
	case err == nil:
		transitions = append(transitions, seed)
	case errors.Is(err, sql.ErrNoRows):
		// No prior transition; data (if any) starts inside the window.
	default:
		return nil, fmt.Errorf("container daily uptime seed: %w", err)
	}
	seeded := len(transitions) > 0

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+transitionColumns+` FROM state_transitions
		 WHERE container_id = ? AND timestamp >= ? ORDER BY timestamp ASC`,
		containerID, windowStart.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("container daily uptime: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
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

	// Determine when data begins: with a seed it predates the window; otherwise
	// it starts at the first in-window transition. No transitions => no data.
	var dataStart time.Time
	hasData := false
	if seeded {
		dataStart = windowStart
		hasData = true
	} else if len(transitions) > 0 {
		dataStart = transitions[0].Timestamp
		hasData = true
	}

	result := make([]DailyUptime, 0, days)
	for i := 0; i < days; i++ {
		day := startOfToday.AddDate(0, 0, -i)
		dateStr := day.Format("2006-01-02")
		dayEnd := day.AddDate(0, 0, 1)
		if dayEnd.After(now) {
			dayEnd = now
		}

		from := day
		if from.Before(dataStart) {
			from = dataStart
		}

		if !hasData || !dayEnd.After(from) {
			result = append(result, DailyUptime{Date: dateStr, UptimePercent: nil, IncidentCount: 0})
			continue
		}

		pct := container.ComputeUptime(transitions, from, dayEnd)
		result = append(result, DailyUptime{
			Date:          dateStr,
			UptimePercent: &pct,
			IncidentCount: countContainerIncidents(transitions, from, dayEnd),
		})
	}

	return result, nil
}

// countContainerIncidents counts up->down transitions within [from, to).
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

// GetHeartbeatDailyUptime aggregates heartbeat pings by UTC day.
// Returns up to `days` days of data, most recent first.
// Days with no pings have UptimePercent = nil.
func (s *UptimeDailyStore) GetHeartbeatDailyUptime(ctx context.Context, heartbeatID string, days int) ([]DailyUptime, error) {
	if days <= 0 {
		days = 90
	}
	if days > 365 {
		days = 365
	}

	now := time.Now().UTC()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	windowStart := startOfToday.AddDate(0, 0, -(days - 1))

	// For heartbeat pings, success pings are ping_type='success'.
	// We count total pings and success pings per day.
	// incident_count = transitions from success to a non-success ping type.
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			date(timestamp, 'unixepoch') AS day,
			ROUND(
				CAST(SUM(CASE WHEN ping_type = 'success' THEN 1 ELSE 0 END) AS REAL)
				/ COUNT(*) * 100.0, 2
			) AS uptime_percent,
			COUNT(CASE WHEN ping_type != 'success' AND prev_type = 'success' THEN 1 END) AS incident_count
		FROM (
			SELECT
				timestamp,
				ping_type,
				LAG(ping_type) OVER (ORDER BY timestamp) AS prev_type
			FROM heartbeat_pings
			WHERE heartbeat_id = ? AND timestamp >= ?
		)
		GROUP BY day
		ORDER BY day DESC
	`, heartbeatID, windowStart.Unix())
	if err != nil {
		return nil, fmt.Errorf("heartbeat daily uptime: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	dayMap := make(map[string]*DailyUptime)
	for rows.Next() {
		var du DailyUptime
		var uptimePct float64
		if err := rows.Scan(&du.Date, &uptimePct, &du.IncidentCount); err != nil {
			return nil, fmt.Errorf("scan heartbeat daily uptime: %w", err)
		}
		du.UptimePercent = &uptimePct
		dayMap[du.Date] = &du
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate heartbeat daily uptime: %w", err)
	}

	result := make([]DailyUptime, 0, days)
	for i := 0; i < days; i++ {
		day := startOfToday.AddDate(0, 0, -i)
		dateStr := day.Format("2006-01-02")
		if du, ok := dayMap[dateStr]; ok {
			result = append(result, *du)
		} else {
			result = append(result, DailyUptime{Date: dateStr, UptimePercent: nil, IncidentCount: 0})
		}
	}

	return result, nil
}

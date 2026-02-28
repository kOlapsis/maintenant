package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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
func (s *UptimeDailyStore) GetEndpointDailyUptime(ctx context.Context, endpointID int64, days int) ([]DailyUptime, error) {
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
	// success column is 1 for success, 0 for failure.
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
	defer rows.Close()

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

	// Generate full day range, filling gaps with null uptime.
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

// GetHeartbeatDailyUptime aggregates heartbeat pings by UTC day.
// Returns up to `days` days of data, most recent first.
// Days with no pings have UptimePercent = nil.
func (s *UptimeDailyStore) GetHeartbeatDailyUptime(ctx context.Context, heartbeatID int64, days int) ([]DailyUptime, error) {
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
	// incident_count = transitions from success to non-success ping type.
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
	defer rows.Close()

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

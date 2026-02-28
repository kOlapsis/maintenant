package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates an in-memory SQLite database with the required schema for testing.
func setupTestDB(t *testing.T) *DB {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { rawDB.Close() })

	// Create required tables.
	_, err = rawDB.Exec(`
		CREATE TABLE IF NOT EXISTS endpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			container_name TEXT NOT NULL,
			label_key TEXT NOT NULL,
			external_id TEXT NOT NULL DEFAULT '',
			endpoint_type TEXT NOT NULL DEFAULT 'http',
			target TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'unknown',
			alert_state TEXT NOT NULL DEFAULT 'normal',
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			consecutive_successes INTEGER NOT NULL DEFAULT 0,
			last_check_at INTEGER,
			last_response_time_ms INTEGER,
			last_http_status INTEGER,
			last_error TEXT,
			config_json TEXT NOT NULL DEFAULT '{}',
			active INTEGER NOT NULL DEFAULT 1,
			first_seen_at INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS check_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			endpoint_id INTEGER NOT NULL,
			success INTEGER NOT NULL,
			response_time_ms INTEGER NOT NULL DEFAULT 0,
			http_status INTEGER,
			error_message TEXT,
			timestamp INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS heartbeats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'new',
			alert_state TEXT NOT NULL DEFAULT 'normal',
			interval_seconds INTEGER NOT NULL DEFAULT 300,
			grace_seconds INTEGER NOT NULL DEFAULT 60,
			last_ping_at INTEGER,
			next_deadline_at INTEGER,
			current_run_started_at INTEGER,
			last_exit_code INTEGER,
			last_duration_ms INTEGER,
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			consecutive_successes INTEGER NOT NULL DEFAULT 0,
			active INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS heartbeat_pings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			heartbeat_id INTEGER NOT NULL,
			ping_type TEXT NOT NULL,
			exit_code INTEGER,
			source_ip TEXT NOT NULL DEFAULT '',
			http_method TEXT NOT NULL DEFAULT 'GET',
			payload TEXT,
			timestamp INTEGER NOT NULL
		);
	`)
	require.NoError(t, err)

	db := &DB{
		db:     rawDB,
		logger: logger,
	}
	return db
}

func insertCheckResult(t *testing.T, db *sql.DB, endpointID int64, success int, ts time.Time) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO check_results (endpoint_id, success, response_time_ms, timestamp) VALUES (?, ?, 100, ?)`,
		endpointID, success, ts.Unix(),
	)
	require.NoError(t, err)
}

func insertHeartbeatPing(t *testing.T, db *sql.DB, heartbeatID int64, pingType string, ts time.Time) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO heartbeat_pings (heartbeat_id, ping_type, source_ip, http_method, timestamp) VALUES (?, ?, '127.0.0.1', 'GET', ?)`,
		heartbeatID, pingType, ts.Unix(),
	)
	require.NoError(t, err)
}

func TestEndpointDailyUptime(t *testing.T) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		endpointID     int64
		days           int
		setup          func(t *testing.T, db *sql.DB)
		wantLen        int
		checkFirstDay  func(t *testing.T, du DailyUptime)
		checkNullDays  bool // expect null uptime for days with no data
	}{
		{
			name:       "no checks returns all null days",
			endpointID: 1,
			days:       3,
			setup:      func(t *testing.T, db *sql.DB) {},
			wantLen:    3,
			checkFirstDay: func(t *testing.T, du DailyUptime) {
				assert.Equal(t, today.Format("2006-01-02"), du.Date)
				assert.Nil(t, du.UptimePercent, "no checks should yield null uptime")
				assert.Equal(t, 0, du.IncidentCount)
			},
			checkNullDays: true,
		},
		{
			name:       "100% uptime day",
			endpointID: 1,
			days:       1,
			setup: func(t *testing.T, db *sql.DB) {
				for i := 0; i < 10; i++ {
					insertCheckResult(t, db, 1, 1, today.Add(time.Duration(i)*time.Hour))
				}
			},
			wantLen: 1,
			checkFirstDay: func(t *testing.T, du DailyUptime) {
				require.NotNil(t, du.UptimePercent)
				assert.Equal(t, 100.0, *du.UptimePercent)
				assert.Equal(t, 0, du.IncidentCount)
			},
		},
		{
			name:       "0% uptime day",
			endpointID: 2,
			days:       1,
			setup: func(t *testing.T, db *sql.DB) {
				for i := 0; i < 5; i++ {
					insertCheckResult(t, db, 2, 0, today.Add(time.Duration(i)*time.Hour))
				}
			},
			wantLen: 1,
			checkFirstDay: func(t *testing.T, du DailyUptime) {
				require.NotNil(t, du.UptimePercent)
				assert.Equal(t, 0.0, *du.UptimePercent)
			},
		},
		{
			name:       "partial uptime with incident",
			endpointID: 3,
			days:       1,
			setup: func(t *testing.T, db *sql.DB) {
				// 4 success, then 1 failure = 80% uptime, 1 incident
				for i := 0; i < 4; i++ {
					insertCheckResult(t, db, 3, 1, today.Add(time.Duration(i)*time.Hour))
				}
				insertCheckResult(t, db, 3, 0, today.Add(4*time.Hour))
			},
			wantLen: 1,
			checkFirstDay: func(t *testing.T, du DailyUptime) {
				require.NotNil(t, du.UptimePercent)
				assert.Equal(t, 80.0, *du.UptimePercent)
				assert.Equal(t, 1, du.IncidentCount)
			},
		},
		{
			name:       "multi-day with gap",
			endpointID: 4,
			days:       3,
			setup: func(t *testing.T, db *sql.DB) {
				// Today: 2 checks both success
				insertCheckResult(t, db, 4, 1, today.Add(1*time.Hour))
				insertCheckResult(t, db, 4, 1, today.Add(2*time.Hour))
				// Yesterday: no checks (should be null)
				// Day before: 1 check, failure
				twoDaysAgo := today.AddDate(0, 0, -2)
				insertCheckResult(t, db, 4, 0, twoDaysAgo.Add(5*time.Hour))
			},
			wantLen: 3,
			checkFirstDay: func(t *testing.T, du DailyUptime) {
				// Most recent first = today
				assert.Equal(t, today.Format("2006-01-02"), du.Date)
				require.NotNil(t, du.UptimePercent)
				assert.Equal(t, 100.0, *du.UptimePercent)
			},
		},
		{
			name:       "default days clamped from 0 to 90",
			endpointID: 1,
			days:       0,
			setup:      func(t *testing.T, db *sql.DB) {},
			wantLen:    90,
		},
		{
			name:       "max days clamped to 365",
			endpointID: 1,
			days:       500,
			setup:      func(t *testing.T, db *sql.DB) {},
			wantLen:    365,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := setupTestDB(t)
			store := NewUptimeDailyStore(d)
			tt.setup(t, d.ReadDB())

			result, err := store.GetEndpointDailyUptime(context.Background(), tt.endpointID, tt.days)
			require.NoError(t, err)
			assert.Len(t, result, tt.wantLen)

			if tt.checkFirstDay != nil && len(result) > 0 {
				tt.checkFirstDay(t, result[0])
			}

			if tt.checkNullDays {
				for _, du := range result {
					assert.Nil(t, du.UptimePercent, "day %s should have null uptime", du.Date)
				}
			}

			// Verify ordering: most recent first.
			if len(result) > 1 {
				assert.GreaterOrEqual(t, result[0].Date, result[1].Date, "days should be ordered most recent first")
			}
		})
	}
}

func TestHeartbeatDailyUptime(t *testing.T) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		heartbeatID   int64
		days          int
		setup         func(t *testing.T, db *sql.DB)
		wantLen       int
		checkFirstDay func(t *testing.T, du DailyUptime)
		checkNullDays bool
	}{
		{
			name:        "no pings returns null days",
			heartbeatID: 1,
			days:        3,
			setup:       func(t *testing.T, db *sql.DB) {},
			wantLen:     3,
			checkFirstDay: func(t *testing.T, du DailyUptime) {
				assert.Nil(t, du.UptimePercent)
				assert.Equal(t, 0, du.IncidentCount)
			},
			checkNullDays: true,
		},
		{
			name:        "all success pings = 100%",
			heartbeatID: 1,
			days:        1,
			setup: func(t *testing.T, db *sql.DB) {
				for i := 0; i < 6; i++ {
					insertHeartbeatPing(t, db, 1, "success", today.Add(time.Duration(i)*time.Hour))
				}
			},
			wantLen: 1,
			checkFirstDay: func(t *testing.T, du DailyUptime) {
				require.NotNil(t, du.UptimePercent)
				assert.Equal(t, 100.0, *du.UptimePercent)
				assert.Equal(t, 0, du.IncidentCount)
			},
		},
		{
			name:        "mixed pings with exit_code type",
			heartbeatID: 2,
			days:        1,
			setup: func(t *testing.T, db *sql.DB) {
				// 3 success + 1 exit_code (not success) = 75%
				insertHeartbeatPing(t, db, 2, "success", today.Add(1*time.Hour))
				insertHeartbeatPing(t, db, 2, "success", today.Add(2*time.Hour))
				insertHeartbeatPing(t, db, 2, "success", today.Add(3*time.Hour))
				insertHeartbeatPing(t, db, 2, "exit_code", today.Add(4*time.Hour))
			},
			wantLen: 1,
			checkFirstDay: func(t *testing.T, du DailyUptime) {
				require.NotNil(t, du.UptimePercent)
				assert.Equal(t, 75.0, *du.UptimePercent)
				assert.Equal(t, 1, du.IncidentCount) // success->exit_code transition
			},
		},
		{
			name:        "90 day window default",
			heartbeatID: 1,
			days:        0,
			setup:       func(t *testing.T, db *sql.DB) {},
			wantLen:     90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := setupTestDB(t)
			store := NewUptimeDailyStore(d)
			tt.setup(t, d.ReadDB())

			result, err := store.GetHeartbeatDailyUptime(context.Background(), tt.heartbeatID, tt.days)
			require.NoError(t, err)
			assert.Len(t, result, tt.wantLen)

			if tt.checkFirstDay != nil && len(result) > 0 {
				tt.checkFirstDay(t, result[0])
			}

			if tt.checkNullDays {
				for _, du := range result {
					assert.Nil(t, du.UptimePercent, "day %s should have null uptime", du.Date)
				}
			}

			if len(result) > 1 {
				assert.GreaterOrEqual(t, result[0].Date, result[1].Date, "days should be ordered most recent first")
			}
		})
	}
}

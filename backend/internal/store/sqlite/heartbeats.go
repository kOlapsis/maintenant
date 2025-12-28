package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/pulseboard/internal/heartbeat"
)

// HeartbeatStore implements heartbeat.HeartbeatStore using SQLite.
type HeartbeatStore struct {
	db     *sql.DB
	writer *Writer
}

// NewHeartbeatStore creates a new SQLite-backed heartbeat store.
func NewHeartbeatStore(d *DB) *HeartbeatStore {
	return &HeartbeatStore{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

const heartbeatColumns = `id, uuid, name, status, alert_state,
	interval_seconds, grace_seconds,
	last_ping_at, next_deadline_at, current_run_started_at,
	last_exit_code, last_duration_ms,
	consecutive_failures, consecutive_successes,
	active, created_at, updated_at`

func (s *HeartbeatStore) CreateHeartbeat(ctx context.Context, h *heartbeat.Heartbeat) (int64, error) {
	now := time.Now().Unix()
	res, err := s.writer.Exec(ctx,
		`INSERT INTO heartbeats (uuid, name, status, alert_state,
			interval_seconds, grace_seconds,
			consecutive_failures, consecutive_successes,
			active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 0, 1, ?, ?)`,
		h.UUID, h.Name, string(heartbeat.StatusNew), string(heartbeat.AlertNormal),
		h.IntervalSeconds, h.GraceSeconds,
		now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert heartbeat: %w", err)
	}
	h.ID = res.LastInsertID
	h.CreatedAt = time.Unix(now, 0)
	h.UpdatedAt = time.Unix(now, 0)
	return res.LastInsertID, nil
}

func (s *HeartbeatStore) GetHeartbeatByID(ctx context.Context, id int64) (*heartbeat.Heartbeat, error) {
	return s.scanHeartbeat(s.db.QueryRowContext(ctx,
		`SELECT `+heartbeatColumns+` FROM heartbeats WHERE id=?`, id))
}

func (s *HeartbeatStore) GetHeartbeatByUUID(ctx context.Context, uuid string) (*heartbeat.Heartbeat, error) {
	return s.scanHeartbeat(s.db.QueryRowContext(ctx,
		`SELECT `+heartbeatColumns+` FROM heartbeats WHERE uuid=? AND active=1`, uuid))
}

func (s *HeartbeatStore) ListHeartbeats(ctx context.Context, opts heartbeat.ListHeartbeatsOpts) ([]*heartbeat.Heartbeat, error) {
	query := `SELECT ` + heartbeatColumns + ` FROM heartbeats WHERE 1=1`
	var args []interface{}

	if !opts.IncludeInactive {
		query += ` AND active=1`
	}
	if opts.Status != "" {
		query += ` AND status=?`
		args = append(args, opts.Status)
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list heartbeats: %w", err)
	}
	defer rows.Close()

	var results []*heartbeat.Heartbeat
	for rows.Next() {
		h, err := s.scanHeartbeatRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, h)
	}
	return results, rows.Err()
}

func (s *HeartbeatStore) UpdateHeartbeat(ctx context.Context, id int64, input heartbeat.UpdateHeartbeatInput) error {
	now := time.Now().Unix()
	query := `UPDATE heartbeats SET updated_at=?`
	args := []interface{}{now}

	if input.Name != nil {
		query += `, name=?`
		args = append(args, *input.Name)
	}
	if input.IntervalSeconds != nil {
		query += `, interval_seconds=?`
		args = append(args, *input.IntervalSeconds)
	}
	if input.GraceSeconds != nil {
		query += `, grace_seconds=?`
		args = append(args, *input.GraceSeconds)
	}

	query += ` WHERE id=?`
	args = append(args, id)

	_, err := s.writer.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update heartbeat %d: %w", id, err)
	}
	return nil
}

func (s *HeartbeatStore) DeleteHeartbeat(ctx context.Context, id int64) error {
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`UPDATE heartbeats SET active=0, updated_at=? WHERE id=?`, now, id)
	if err != nil {
		return fmt.Errorf("delete heartbeat %d: %w", id, err)
	}
	return nil
}

func (s *HeartbeatStore) UpdateHeartbeatState(ctx context.Context, id int64,
	status heartbeat.HeartbeatStatus, alertState heartbeat.AlertState,
	lastPingAt *time.Time, nextDeadlineAt *time.Time, currentRunStartedAt *time.Time,
	lastExitCode *int, lastDurationMs *int64,
	consecutiveFailures, consecutiveSuccesses int) error {

	now := time.Now().Unix()
	var lastPingUnix, deadlineUnix, runStartUnix interface{}
	if lastPingAt != nil {
		lastPingUnix = lastPingAt.Unix()
	}
	if nextDeadlineAt != nil {
		deadlineUnix = nextDeadlineAt.Unix()
	}
	if currentRunStartedAt != nil {
		runStartUnix = currentRunStartedAt.Unix()
	}

	_, err := s.writer.Exec(ctx,
		`UPDATE heartbeats SET status=?, alert_state=?,
			last_ping_at=?, next_deadline_at=?, current_run_started_at=?,
			last_exit_code=?, last_duration_ms=?,
			consecutive_failures=?, consecutive_successes=?,
			updated_at=?
		WHERE id=?`,
		string(status), string(alertState),
		lastPingUnix, deadlineUnix, runStartUnix,
		lastExitCode, lastDurationMs,
		consecutiveFailures, consecutiveSuccesses,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("update heartbeat state %d: %w", id, err)
	}
	return nil
}

func (s *HeartbeatStore) PauseHeartbeat(ctx context.Context, id int64) error {
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`UPDATE heartbeats SET status='paused', next_deadline_at=NULL, updated_at=? WHERE id=?`,
		now, id)
	if err != nil {
		return fmt.Errorf("pause heartbeat %d: %w", id, err)
	}
	return nil
}

func (s *HeartbeatStore) ResumeHeartbeat(ctx context.Context, id int64, nextDeadlineAt time.Time) error {
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`UPDATE heartbeats SET status='up', next_deadline_at=?, updated_at=? WHERE id=?`,
		nextDeadlineAt.Unix(), now, id)
	if err != nil {
		return fmt.Errorf("resume heartbeat %d: %w", id, err)
	}
	return nil
}

func (s *HeartbeatStore) ListOverdueHeartbeats(ctx context.Context, now time.Time) ([]*heartbeat.Heartbeat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+heartbeatColumns+` FROM heartbeats
		WHERE active=1 AND status IN ('up', 'started') AND next_deadline_at IS NOT NULL AND next_deadline_at<?`,
		now.Unix())
	if err != nil {
		return nil, fmt.Errorf("list overdue heartbeats: %w", err)
	}
	defer rows.Close()

	var results []*heartbeat.Heartbeat
	for rows.Next() {
		h, err := s.scanHeartbeatRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, h)
	}
	return results, rows.Err()
}

func (s *HeartbeatStore) CountActiveHeartbeats(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM heartbeats WHERE active=1`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active heartbeats: %w", err)
	}
	return count, nil
}

// --- Pings ---

func (s *HeartbeatStore) InsertPing(ctx context.Context, p *heartbeat.HeartbeatPing) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO heartbeat_pings (heartbeat_id, ping_type, exit_code, source_ip, http_method, payload, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.HeartbeatID, string(p.PingType), p.ExitCode, p.SourceIP, p.HTTPMethod, p.Payload, p.Timestamp.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert ping: %w", err)
	}
	p.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *HeartbeatStore) ListPings(ctx context.Context, heartbeatID int64, opts heartbeat.ListPingsOpts) ([]*heartbeat.HeartbeatPing, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM heartbeat_pings WHERE heartbeat_id=?`, heartbeatID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count pings: %w", err)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query := `SELECT id, heartbeat_id, ping_type, exit_code, source_ip, http_method, payload, timestamp
		FROM heartbeat_pings WHERE heartbeat_id=? ORDER BY timestamp DESC`
	query += fmt.Sprintf(` LIMIT %d`, limit)
	if opts.Offset > 0 {
		query += fmt.Sprintf(` OFFSET %d`, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, heartbeatID)
	if err != nil {
		return nil, 0, fmt.Errorf("list pings: %w", err)
	}
	defer rows.Close()

	var results []*heartbeat.HeartbeatPing
	for rows.Next() {
		p, err := scanPingRow(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, p)
	}
	return results, total, rows.Err()
}

// --- Executions ---

func (s *HeartbeatStore) InsertExecution(ctx context.Context, e *heartbeat.HeartbeatExecution) (int64, error) {
	var startedAtUnix, completedAtUnix interface{}
	if e.StartedAt != nil {
		startedAtUnix = e.StartedAt.Unix()
	}
	if e.CompletedAt != nil {
		completedAtUnix = e.CompletedAt.Unix()
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO heartbeat_executions (heartbeat_id, started_at, completed_at, duration_ms, exit_code, outcome, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.HeartbeatID, startedAtUnix, completedAtUnix, e.DurationMs, e.ExitCode, string(e.Outcome), e.Payload,
	)
	if err != nil {
		return 0, fmt.Errorf("insert execution: %w", err)
	}
	e.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *HeartbeatStore) UpdateExecution(ctx context.Context, id int64, completedAt *time.Time, durationMs *int64, exitCode *int, outcome heartbeat.ExecutionOutcome, payload *string) error {
	var completedAtUnix interface{}
	if completedAt != nil {
		completedAtUnix = completedAt.Unix()
	}

	_, err := s.writer.Exec(ctx,
		`UPDATE heartbeat_executions SET completed_at=?, duration_ms=?, exit_code=?, outcome=?, payload=? WHERE id=?`,
		completedAtUnix, durationMs, exitCode, string(outcome), payload, id,
	)
	if err != nil {
		return fmt.Errorf("update execution %d: %w", id, err)
	}
	return nil
}

func (s *HeartbeatStore) GetCurrentExecution(ctx context.Context, heartbeatID int64) (*heartbeat.HeartbeatExecution, error) {
	return scanExecutionSingle(s.db.QueryRowContext(ctx,
		`SELECT id, heartbeat_id, started_at, completed_at, duration_ms, exit_code, outcome, payload
		FROM heartbeat_executions WHERE heartbeat_id=? AND outcome='in_progress'
		ORDER BY id DESC LIMIT 1`, heartbeatID))
}

func (s *HeartbeatStore) ListExecutions(ctx context.Context, heartbeatID int64, opts heartbeat.ListExecutionsOpts) ([]*heartbeat.HeartbeatExecution, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM heartbeat_executions WHERE heartbeat_id=?`, heartbeatID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count executions: %w", err)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}

	query := `SELECT id, heartbeat_id, started_at, completed_at, duration_ms, exit_code, outcome, payload
		FROM heartbeat_executions WHERE heartbeat_id=? ORDER BY id DESC`
	query += fmt.Sprintf(` LIMIT %d`, limit)
	if opts.Offset > 0 {
		query += fmt.Sprintf(` OFFSET %d`, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, heartbeatID)
	if err != nil {
		return nil, 0, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()

	var results []*heartbeat.HeartbeatExecution
	for rows.Next() {
		e, err := scanExecutionRow(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, e)
	}
	return results, total, rows.Err()
}

// --- Retention ---

func (s *HeartbeatStore) DeletePingsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	var totalDeleted int64
	for {
		res, err := s.writer.Exec(ctx,
			`DELETE FROM heartbeat_pings WHERE rowid IN (
				SELECT rowid FROM heartbeat_pings WHERE timestamp<? LIMIT ?
			)`, before.Unix(), batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete heartbeat pings: %w", err)
		}
		totalDeleted += res.RowsAffected
		if res.RowsAffected < int64(batchSize) {
			break
		}
	}
	return totalDeleted, nil
}

func (s *HeartbeatStore) DeleteExecutionsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	var totalDeleted int64
	for {
		res, err := s.writer.Exec(ctx,
			`DELETE FROM heartbeat_executions WHERE rowid IN (
				SELECT rowid FROM heartbeat_executions WHERE completed_at IS NOT NULL AND completed_at<? LIMIT ?
			)`, before.Unix(), batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete heartbeat executions: %w", err)
		}
		totalDeleted += res.RowsAffected
		if res.RowsAffected < int64(batchSize) {
			break
		}
	}
	return totalDeleted, nil
}

// --- Scanners ---

func (s *HeartbeatStore) scanHeartbeat(row rowScanner) (*heartbeat.Heartbeat, error) {
	var h heartbeat.Heartbeat
	var lastPingAt, nextDeadlineAt, currentRunStartedAt, lastExitCode, lastDurationMs sql.NullInt64
	var active int
	var createdAt, updatedAt int64

	err := row.Scan(
		&h.ID, &h.UUID, &h.Name, &h.Status, &h.AlertState,
		&h.IntervalSeconds, &h.GraceSeconds,
		&lastPingAt, &nextDeadlineAt, &currentRunStartedAt,
		&lastExitCode, &lastDurationMs,
		&h.ConsecutiveFailures, &h.ConsecutiveSuccesses,
		&active, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan heartbeat: %w", err)
	}

	h.Active = active != 0
	h.CreatedAt = time.Unix(createdAt, 0)
	h.UpdatedAt = time.Unix(updatedAt, 0)

	if lastPingAt.Valid {
		t := time.Unix(lastPingAt.Int64, 0)
		h.LastPingAt = &t
	}
	if nextDeadlineAt.Valid {
		t := time.Unix(nextDeadlineAt.Int64, 0)
		h.NextDeadlineAt = &t
	}
	if currentRunStartedAt.Valid {
		t := time.Unix(currentRunStartedAt.Int64, 0)
		h.CurrentRunStartedAt = &t
	}
	if lastExitCode.Valid {
		v := int(lastExitCode.Int64)
		h.LastExitCode = &v
	}
	if lastDurationMs.Valid {
		v := lastDurationMs.Int64
		h.LastDurationMs = &v
	}

	return &h, nil
}

func (s *HeartbeatStore) scanHeartbeatRow(rows *sql.Rows) (*heartbeat.Heartbeat, error) {
	return s.scanHeartbeat(rows)
}

func scanPingRow(row rowScanner) (*heartbeat.HeartbeatPing, error) {
	var p heartbeat.HeartbeatPing
	var exitCode sql.NullInt64
	var payload sql.NullString
	var ts int64

	err := row.Scan(
		&p.ID, &p.HeartbeatID, &p.PingType, &exitCode,
		&p.SourceIP, &p.HTTPMethod, &payload, &ts,
	)
	if err != nil {
		return nil, fmt.Errorf("scan ping: %w", err)
	}

	p.Timestamp = time.Unix(ts, 0)
	if exitCode.Valid {
		v := int(exitCode.Int64)
		p.ExitCode = &v
	}
	if payload.Valid {
		p.Payload = &payload.String
	}

	return &p, nil
}

func scanExecutionRow(row rowScanner) (*heartbeat.HeartbeatExecution, error) {
	var e heartbeat.HeartbeatExecution
	var startedAt, completedAt, durationMs, exitCode sql.NullInt64
	var payload sql.NullString

	err := row.Scan(
		&e.ID, &e.HeartbeatID, &startedAt, &completedAt,
		&durationMs, &exitCode, &e.Outcome, &payload,
	)
	if err != nil {
		return nil, fmt.Errorf("scan execution: %w", err)
	}

	if startedAt.Valid {
		t := time.Unix(startedAt.Int64, 0)
		e.StartedAt = &t
	}
	if completedAt.Valid {
		t := time.Unix(completedAt.Int64, 0)
		e.CompletedAt = &t
	}
	if durationMs.Valid {
		v := durationMs.Int64
		e.DurationMs = &v
	}
	if exitCode.Valid {
		v := int(exitCode.Int64)
		e.ExitCode = &v
	}
	if payload.Valid {
		e.Payload = &payload.String
	}

	return &e, nil
}

func scanExecutionSingle(row *sql.Row) (*heartbeat.HeartbeatExecution, error) {
	var e heartbeat.HeartbeatExecution
	var startedAt, completedAt, durationMs, exitCode sql.NullInt64
	var payload sql.NullString

	err := row.Scan(
		&e.ID, &e.HeartbeatID, &startedAt, &completedAt,
		&durationMs, &exitCode, &e.Outcome, &payload,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan execution: %w", err)
	}

	if startedAt.Valid {
		t := time.Unix(startedAt.Int64, 0)
		e.StartedAt = &t
	}
	if completedAt.Valid {
		t := time.Unix(completedAt.Int64, 0)
		e.CompletedAt = &t
	}
	if durationMs.Valid {
		v := durationMs.Int64
		e.DurationMs = &v
	}
	if exitCode.Valid {
		v := int(exitCode.Int64)
		e.ExitCode = &v
	}
	if payload.Valid {
		e.Payload = &payload.String
	}

	return &e, nil
}

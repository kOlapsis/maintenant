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

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/mattn/go-sqlite3"
)

// EscalationStore implements escalation.Store using SQLite.
type EscalationStore struct {
	db     *sql.DB
	writer *Writer
}

// NewEscalationStore creates a new SQLite-backed escalation store.
func NewEscalationStore(d *DB) *EscalationStore {
	return &EscalationStore{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *EscalationStore) InsertPolicy(ctx context.Context, p *escalation.Policy) (int64, error) {
	sevJSON, err := json.Marshal(p.Filters.Severities)
	if err != nil {
		return 0, fmt.Errorf("marshal severities: %w", err)
	}
	scopesJSON, err := json.Marshal(p.Filters.Scopes)
	if err != nil {
		return 0, fmt.Errorf("marshal scopes: %w", err)
	}
	tagsJSON, err := json.Marshal(p.Filters.Tags)
	if err != nil {
		return 0, fmt.Errorf("marshal tags: %w", err)
	}
	levelsJSON, err := json.Marshal(p.Levels)
	if err != nil {
		return 0, fmt.Errorf("marshal levels: %w", err)
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO escalation_policies
			(name, active, active_before_downgrade, severities_json, scopes_json, tags_json, levels_json, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, boolToInt(p.Active), boolToInt(p.ActiveBeforeDowngrade),
		string(sevJSON), string(scopesJSON), string(tagsJSON), string(levelsJSON),
		NullableString(p.CreatedBy), NullableString(p.UpdatedBy),
	)
	if err != nil {
		return 0, fmt.Errorf("insert escalation policy: %w", err)
	}
	p.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *EscalationStore) UpdatePolicy(ctx context.Context, p *escalation.Policy) error {
	sevJSON, err := json.Marshal(p.Filters.Severities)
	if err != nil {
		return fmt.Errorf("marshal severities: %w", err)
	}
	scopesJSON, err := json.Marshal(p.Filters.Scopes)
	if err != nil {
		return fmt.Errorf("marshal scopes: %w", err)
	}
	tagsJSON, err := json.Marshal(p.Filters.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	levelsJSON, err := json.Marshal(p.Levels)
	if err != nil {
		return fmt.Errorf("marshal levels: %w", err)
	}

	_, err = s.writer.Exec(ctx,
		`UPDATE escalation_policies SET
			name=?, active=?, active_before_downgrade=?,
			severities_json=?, scopes_json=?, tags_json=?, levels_json=?,
			updated_at=?, updated_by=?
		WHERE id=?`,
		p.Name, boolToInt(p.Active), boolToInt(p.ActiveBeforeDowngrade),
		string(sevJSON), string(scopesJSON), string(tagsJSON), string(levelsJSON),
		time.Now().UTC().Format(time.RFC3339), NullableString(p.UpdatedBy),
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("update escalation policy: %w", err)
	}
	return nil
}

func (s *EscalationStore) SelectPolicy(ctx context.Context, id int64) (*escalation.Policy, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, active, active_before_downgrade,
			severities_json, scopes_json, tags_json, levels_json,
			created_at, created_by, updated_at, updated_by
		FROM escalation_policies WHERE id = ?`, id)
	p, err := scanEscalationPolicy(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (s *EscalationStore) SelectPolicies(ctx context.Context, activeOnly bool) ([]*escalation.Policy, error) {
	q := `SELECT id, name, active, active_before_downgrade,
		severities_json, scopes_json, tags_json, levels_json,
		created_at, created_by, updated_at, updated_by
		FROM escalation_policies`
	if activeOnly {
		q += ` WHERE active = 1`
	}
	q += ` ORDER BY id ASC`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("select escalation policies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var policies []*escalation.Policy
	for rows.Next() {
		p, err := scanEscalationPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *EscalationStore) DeletePolicy(ctx context.Context, id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	// Stop active runs before deleting; runs retain history via ended_at.
	_, err := s.writer.Exec(ctx,
		`UPDATE escalation_runs SET status='stopped_by_policy_deletion', ended_at=?, next_action_at=NULL
		WHERE policy_id=? AND status IN ('active','paused_by_maintenance')`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("stop runs before policy delete: %w", err)
	}
	_, err = s.writer.Exec(ctx, `DELETE FROM escalation_policies WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete escalation policy: %w", err)
	}
	return nil
}

func (s *EscalationStore) CountActivePolicies(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM escalation_policies WHERE active = 1`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active policies: %w", err)
	}
	return n, nil
}

func (s *EscalationStore) SelectRun(ctx context.Context, id int64) (*escalation.Run, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, policy_id, policy_snapshot_json, alert_id, status,
			last_executed_level_index, started_at, ended_at, next_action_at
		FROM escalation_runs WHERE id = ?`, id)
	r, err := scanEscalationRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

func (s *EscalationStore) SelectRunsByAlert(ctx context.Context, alertID int64) ([]*escalation.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, policy_id, policy_snapshot_json, alert_id, status,
			last_executed_level_index, started_at, ended_at, next_action_at
		FROM escalation_runs WHERE alert_id = ? ORDER BY started_at DESC`, alertID)
	if err != nil {
		return nil, fmt.Errorf("select runs by alert: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEscalationRuns(rows)
}

func (s *EscalationStore) SelectRunsByPolicy(ctx context.Context, policyID int64, limit int, cursor int64) ([]*escalation.Run, error) {
	q := `SELECT id, policy_id, policy_snapshot_json, alert_id, status,
		last_executed_level_index, started_at, ended_at, next_action_at
		FROM escalation_runs WHERE policy_id = ?`
	args := []interface{}{policyID}
	if cursor > 0 {
		q += ` AND id < ?`
		args = append(args, cursor)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("select runs by policy: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEscalationRuns(rows)
}

func (s *EscalationStore) SelectRunDeliveries(ctx context.Context, runID int64) ([]*escalation.Delivery, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, level_index, channel_id, status, error,
			attempt_started_at, sent_at
		FROM escalation_deliveries WHERE run_id = ? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("select run deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var deliveries []*escalation.Delivery
	for rows.Next() {
		d, err := scanEscalationDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

func (s *EscalationStore) BulkDeactivateAllPolicies(ctx context.Context) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE escalation_policies SET active_before_downgrade = active, active = 0 WHERE active = 1`)
	if err != nil {
		return fmt.Errorf("bulk deactivate policies: %w", err)
	}
	return nil
}

func (s *EscalationStore) BulkRestorePoliciesFromDowngrade(ctx context.Context) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE escalation_policies SET active = 1, active_before_downgrade = 0 WHERE active_before_downgrade = 1`)
	if err != nil {
		return fmt.Errorf("bulk restore policies: %w", err)
	}
	return nil
}

func (s *EscalationStore) BulkStopActiveRuns(ctx context.Context, stopStatus string, endedAt time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE escalation_runs SET status = ?, ended_at = ?, next_action_at = NULL
		WHERE status IN ('active','paused_by_maintenance')`,
		stopStatus, endedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("bulk stop active runs: %w", err)
	}
	return nil
}

// InsertRun persists a new escalation run.
func (s *EscalationStore) InsertRun(ctx context.Context, r *escalation.Run) (int64, error) {
	var policyID interface{}
	if r.PolicyID != nil {
		policyID = *r.PolicyID
	}
	var nextActionAt interface{}
	if r.NextActionAt != nil {
		nextActionAt = r.NextActionAt.UTC().Format(time.RFC3339)
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO escalation_runs
			(policy_id, policy_snapshot_json, alert_id, status,
			 last_executed_level_index, started_at, next_action_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
		policyID, r.PolicySnapshotJSON, r.AlertID, r.Status,
		r.LastExecutedLevelIndex,
		r.StartedAt.UTC().Format(time.RFC3339),
		nextActionAt,
	)
	if err != nil {
		return 0, fmt.Errorf("insert escalation run: %w", err)
	}
	r.ID = res.LastInsertID
	return res.LastInsertID, nil
}

// UpdateRunProgress advances a run's level cursor and reschedules its next action.
// Used by the runner after executing a level (R4 reserve-then-deliver).
func (s *EscalationStore) UpdateRunProgress(ctx context.Context, runID int64, lastExecutedLevelIndex int, nextActionAt *time.Time, status string) error {
	var nextAt interface{}
	if nextActionAt != nil {
		nextAt = nextActionAt.UTC().Format(time.RFC3339)
	}
	_, err := s.writer.Exec(ctx,
		`UPDATE escalation_runs
		 SET last_executed_level_index = ?, next_action_at = ?, status = ?
		 WHERE id = ?`,
		lastExecutedLevelIndex, nextAt, status, runID,
	)
	if err != nil {
		return fmt.Errorf("update run progress: %w", err)
	}
	return nil
}

// TerminateRun moves a run to a terminal status (stopped_by_*, exhausted) and stamps ended_at.
func (s *EscalationStore) TerminateRun(ctx context.Context, runID int64, status string, endedAt time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE escalation_runs
		 SET status = ?, ended_at = ?, next_action_at = NULL
		 WHERE id = ?`,
		status, endedAt.UTC().Format(time.RFC3339), runID,
	)
	if err != nil {
		return fmt.Errorf("terminate run: %w", err)
	}
	return nil
}

// SelectActiveRunsByAlert returns runs in non-terminal state for a given alert.
// Used by ack/resolve hooks and OnAlertCreated dedup.
func (s *EscalationStore) SelectActiveRunsByAlert(ctx context.Context, alertID int64) ([]*escalation.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, policy_id, policy_snapshot_json, alert_id, status,
			last_executed_level_index, started_at, ended_at, next_action_at
		FROM escalation_runs
		WHERE alert_id = ? AND status IN ('active','paused_by_maintenance')
		ORDER BY id ASC`, alertID)
	if err != nil {
		return nil, fmt.Errorf("select active runs by alert: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEscalationRuns(rows)
}

// SelectDueRuns returns runs in status 'active' or 'paused_by_maintenance' whose
// next_action_at is at or before now. The partial index covers the active case;
// paused runs fall back to a small-table scan (acceptable: <200 paused runs per
// spec hypothesis v1).
func (s *EscalationStore) SelectDueRuns(ctx context.Context, now time.Time) ([]*escalation.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, policy_id, policy_snapshot_json, alert_id, status,
			last_executed_level_index, started_at, ended_at, next_action_at
		FROM escalation_runs
		WHERE status IN ('active','paused_by_maintenance')
		  AND next_action_at IS NOT NULL
		  AND next_action_at <= ?
		ORDER BY next_action_at ASC`,
		now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("select due runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEscalationRuns(rows)
}

// PauseRunForMaintenance moves an active run to paused_by_maintenance and
// schedules a recheck. Returning to active happens via ResumeRunFromMaintenance.
func (s *EscalationStore) PauseRunForMaintenance(ctx context.Context, runID int64, recheckAt time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE escalation_runs
		 SET status = 'paused_by_maintenance', next_action_at = ?
		 WHERE id = ? AND status = 'active'`,
		recheckAt.UTC().Format(time.RFC3339), runID,
	)
	if err != nil {
		return fmt.Errorf("pause run: %w", err)
	}
	return nil
}

// ResumeRunFromMaintenance flips a paused run back to active with an updated due time.
func (s *EscalationStore) ResumeRunFromMaintenance(ctx context.Context, runID int64, nextActionAt time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE escalation_runs
		 SET status = 'active', next_action_at = ?
		 WHERE id = ? AND status = 'paused_by_maintenance'`,
		nextActionAt.UTC().Format(time.RFC3339), runID,
	)
	if err != nil {
		return fmt.Errorf("resume run: %w", err)
	}
	return nil
}

// InsertDelivery reserves a delivery slot. Returns escalation.ErrDeliveryDuplicate
// when (run_id, level_index, channel_id) already exists — the caller treats this
// as "already attempted" (R4 reserve-then-deliver idempotence).
func (s *EscalationStore) InsertDelivery(ctx context.Context, d *escalation.Delivery) (int64, error) {
	var channelID interface{}
	if d.ChannelID != nil {
		channelID = *d.ChannelID
	}
	var sentAt interface{}
	if d.SentAt != nil {
		sentAt = d.SentAt.UTC().Format(time.RFC3339)
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO escalation_deliveries
			(run_id, level_index, channel_id, status, error, attempt_started_at, sent_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.RunID, d.LevelIndex, channelID, d.Status, NullableString(d.Error),
		d.AttemptStartedAt.UTC().Format(time.RFC3339), sentAt,
	)
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return 0, escalation.ErrDeliveryDuplicate
		}
		return 0, fmt.Errorf("insert delivery: %w", err)
	}
	d.ID = res.LastInsertID
	return res.LastInsertID, nil
}

// UpdateDelivery persists status/error/sent_at after a send attempt completes.
func (s *EscalationStore) UpdateDelivery(ctx context.Context, d *escalation.Delivery) error {
	var sentAt interface{}
	if d.SentAt != nil {
		sentAt = d.SentAt.UTC().Format(time.RFC3339)
	}
	_, err := s.writer.Exec(ctx,
		`UPDATE escalation_deliveries
		 SET status = ?, error = ?, sent_at = ?
		 WHERE id = ?`,
		d.Status, NullableString(d.Error), sentAt, d.ID,
	)
	if err != nil {
		return fmt.Errorf("update delivery: %w", err)
	}
	return nil
}

// SelectOrphanPendingDeliveries returns deliveries stuck in 'pending' for longer
// than the runner's orphan timeout. The runner decides whether to retry or abandon.
func (s *EscalationStore) SelectOrphanPendingDeliveries(ctx context.Context, before time.Time) ([]*escalation.Delivery, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, level_index, channel_id, status, error,
			attempt_started_at, sent_at
		FROM escalation_deliveries
		WHERE status = 'pending' AND attempt_started_at < ?
		ORDER BY attempt_started_at ASC`,
		before.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("select orphan pending deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var deliveries []*escalation.Delivery
	for rows.Next() {
		d, err := scanEscalationDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// PurgeRunsAndDeliveriesOlderThan deletes terminated runs (ended_at < before) in batches of 1000.
// Cascade delete handles escalation_deliveries automatically.
func (s *EscalationStore) PurgeRunsAndDeliveriesOlderThan(ctx context.Context, before time.Time) error {
	cutoff := before.UTC().Format(time.RFC3339)
	for {
		res, err := s.writer.Exec(ctx,
			`DELETE FROM escalation_runs WHERE id IN (
				SELECT id FROM escalation_runs
				WHERE ended_at IS NOT NULL AND ended_at < ?
				LIMIT 1000
			)`,
			cutoff,
		)
		if err != nil {
			return fmt.Errorf("purge escalation runs: %w", err)
		}
		if res.RowsAffected == 0 {
			break
		}
	}
	return nil
}

// --- scan helpers ---

func scanEscalationPolicy(scanner rowScanner) (*escalation.Policy, error) {
	var p escalation.Policy
	var active, activeBeforeDowngrade bool
	var sevJSON, scopesJSON, tagsJSON, levelsJSON string
	var createdAt, updatedAt string
	var createdBy, updatedBy sql.NullString

	err := scanner.Scan(
		&p.ID, &p.Name, &active, &activeBeforeDowngrade,
		&sevJSON, &scopesJSON, &tagsJSON, &levelsJSON,
		&createdAt, &createdBy, &updatedAt, &updatedBy,
	)
	if err != nil {
		return nil, err
	}

	p.Active = active
	p.ActiveBeforeDowngrade = activeBeforeDowngrade
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if createdBy.Valid {
		p.CreatedBy = createdBy.String
	}
	if updatedBy.Valid {
		p.UpdatedBy = updatedBy.String
	}

	_ = json.Unmarshal([]byte(sevJSON), &p.Filters.Severities)
	_ = json.Unmarshal([]byte(scopesJSON), &p.Filters.Scopes)
	_ = json.Unmarshal([]byte(tagsJSON), &p.Filters.Tags)
	_ = json.Unmarshal([]byte(levelsJSON), &p.Levels)

	if p.Filters.Severities == nil {
		p.Filters.Severities = []string{}
	}
	if p.Filters.Scopes == nil {
		p.Filters.Scopes = []escalation.Scope{}
	}
	if p.Filters.Tags == nil {
		p.Filters.Tags = []string{}
	}
	if p.Levels == nil {
		p.Levels = []escalation.Level{}
	}

	return &p, nil
}

func scanEscalationRun(scanner rowScanner) (*escalation.Run, error) {
	var r escalation.Run
	var policyID sql.NullInt64
	var endedAt, nextActionAt sql.NullString
	var startedAt string

	err := scanner.Scan(
		&r.ID, &policyID, &r.PolicySnapshotJSON, &r.AlertID, &r.Status,
		&r.LastExecutedLevelIndex, &startedAt, &endedAt, &nextActionAt,
	)
	if err != nil {
		return nil, err
	}

	if policyID.Valid {
		v := policyID.Int64
		r.PolicyID = &v
	}
	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		r.EndedAt = &t
	}
	if nextActionAt.Valid {
		t, _ := time.Parse(time.RFC3339, nextActionAt.String)
		r.NextActionAt = &t
	}
	return &r, nil
}

func scanEscalationRuns(rows *sql.Rows) ([]*escalation.Run, error) {
	var runs []*escalation.Run
	for rows.Next() {
		r, err := scanEscalationRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func scanEscalationDelivery(scanner rowScanner) (*escalation.Delivery, error) {
	var d escalation.Delivery
	var channelID sql.NullInt64
	var errStr sql.NullString
	var sentAt sql.NullString
	var attemptStartedAt string

	err := scanner.Scan(
		&d.ID, &d.RunID, &d.LevelIndex, &channelID, &d.Status, &errStr,
		&attemptStartedAt, &sentAt,
	)
	if err != nil {
		return nil, err
	}

	if channelID.Valid {
		v := channelID.Int64
		d.ChannelID = &v
	}
	if errStr.Valid {
		d.Error = errStr.String
	}
	d.AttemptStartedAt, _ = time.Parse(time.RFC3339, attemptStartedAt)
	if sentAt.Valid {
		t, _ := time.Parse(time.RFC3339, sentAt.String)
		d.SentAt = &t
	}
	return &d, nil
}

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

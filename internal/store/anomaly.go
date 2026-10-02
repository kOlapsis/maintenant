// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kolapsis/maintenant/internal/anomaly"
	"github.com/kolapsis/maintenant/internal/uid"
)

// AnomalyStore implements anomaly.Store.
type AnomalyStore struct {
	db     *Reader
	writer *Writer
}

var _ anomaly.Store = (*AnomalyStore)(nil)

// NewAnomalyStore creates an anomaly store.
func NewAnomalyStore(d *DB) *AnomalyStore {
	return &AnomalyStore{db: d.Reader(), writer: d.Writer()}
}

const anomalyIdentityColumns = `c.name, c.agent_id, c.runtime_type, COALESCE(c.orchestration_group, ''), COALESCE(c.orchestration_unit, ''),
	c.controller_kind, c.namespace, c.swarm_service_name`

// ListContainerHourly returns the hourly rollups from since onward of every container neither archived nor ignored.
func (s *AnomalyStore) ListContainerHourly(ctx context.Context, since int64) ([]anomaly.HourlyResourceRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+anomalyIdentityColumns+`,
			rh.bucket, rh.avg_cpu_percent, rh.avg_mem_used, rh.avg_net_rx_bytes, rh.avg_net_tx_bytes, rh.sample_count
		FROM resource_hourly rh
		JOIN containers c ON rh.container_id = c.id
		WHERE rh.bucket >= ? AND c.archived = 0 AND c.is_ignored = 0`, since)
	if err != nil {
		return nil, fmt.Errorf("list container hourly: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []anomaly.HourlyResourceRow
	for rows.Next() {
		var r anomaly.HourlyResourceRow
		id := &r.ScopeIdentity
		if err := rows.Scan(&id.Name, &r.AgentID, &id.RuntimeType, &id.OrchestrationGroup, &id.OrchestrationUnit,
			&id.ControllerKind, &id.Namespace, &id.SwarmServiceName,
			&r.Bucket, &r.CPUPercent, &r.MemUsed, &r.NetRxBytes, &r.NetTxBytes, &r.SampleCount); err != nil {
			return nil, fmt.Errorf("scan container hourly: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListContainerSnapshotsSince returns the raw snapshots from since onward of every container neither archived nor ignored.
func (s *AnomalyStore) ListContainerSnapshotsSince(ctx context.Context, since int64) ([]anomaly.SnapshotRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+anomalyIdentityColumns+`,
			rs.timestamp, rs.cpu_percent, rs.mem_used, rs.net_rx_bytes, rs.net_tx_bytes
		FROM resource_snapshots rs
		JOIN containers c ON rs.container_id = c.id
		WHERE rs.timestamp >= ? AND c.archived = 0 AND c.is_ignored = 0
		ORDER BY rs.timestamp`, since)
	if err != nil {
		return nil, fmt.Errorf("list container snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []anomaly.SnapshotRow
	for rows.Next() {
		var r anomaly.SnapshotRow
		id := &r.ScopeIdentity
		if err := rows.Scan(&id.Name, &r.AgentID, &id.RuntimeType, &id.OrchestrationGroup, &id.OrchestrationUnit,
			&id.ControllerKind, &id.Namespace, &id.SwarmServiceName,
			&r.Timestamp, &r.CPUPercent, &r.MemUsed, &r.NetRxBytes, &r.NetTxBytes); err != nil {
			return nil, fmt.Errorf("scan container snapshot: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const anomalyBaselineColumns = `scope_type, scope_id, metric, dimension, bucket, median, mad, sample_count, updated_at`

func (s *AnomalyStore) UpsertBaseline(ctx context.Context, b anomaly.Baseline) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO anomaly_baseline (`+anomalyBaselineColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, metric, dimension, bucket) DO UPDATE SET
			median=excluded.median, mad=excluded.mad, sample_count=excluded.sample_count, updated_at=excluded.updated_at`,
		b.ScopeType, b.ScopeID, b.Metric, b.Dimension, b.Bucket, b.Median, b.MAD, b.SampleCount, b.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert baseline: %w", err)
	}
	return nil
}

func (s *AnomalyStore) GetBaseline(ctx context.Context, key anomaly.SeriesKey, bucket int) (*anomaly.Baseline, error) {
	b, err := scanBaseline(s.db.QueryRowContext(ctx,
		`SELECT `+anomalyBaselineColumns+` FROM anomaly_baseline
		WHERE scope_type = ? AND scope_id = ? AND metric = ? AND dimension = ? AND bucket = ?`,
		key.ScopeType, key.ScopeID, key.Metric, key.Dimension, bucket))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get baseline: %w", err)
	}
	return b, nil
}

func (s *AnomalyStore) ListBaselines(ctx context.Context, key anomaly.SeriesKey) ([]anomaly.Baseline, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+anomalyBaselineColumns+` FROM anomaly_baseline
		WHERE scope_type = ? AND scope_id = ? AND metric = ? AND dimension = ?
		ORDER BY bucket`,
		key.ScopeType, key.ScopeID, key.Metric, key.Dimension)
	if err != nil {
		return nil, fmt.Errorf("list baselines: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []anomaly.Baseline
	for rows.Next() {
		b, err := scanBaseline(rows)
		if err != nil {
			return nil, fmt.Errorf("scan baseline: %w", err)
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func scanBaseline(row rowScanner) (*anomaly.Baseline, error) {
	var b anomaly.Baseline
	if err := row.Scan(&b.ScopeType, &b.ScopeID, &b.Metric, &b.Dimension, &b.Bucket,
		&b.Median, &b.MAD, &b.SampleCount, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *AnomalyStore) DeleteBaselinesForScope(ctx context.Context, scopeType, scopeID string) error {
	_, err := s.writer.Exec(ctx,
		`DELETE FROM anomaly_baseline WHERE scope_type = ? AND scope_id = ?`, scopeType, scopeID)
	if err != nil {
		return fmt.Errorf("delete baselines for scope: %w", err)
	}
	return nil
}

const anomalySeriesStateColumns = `scope_type, scope_id, metric, dimension, node_id, state,
	first_seen_at, days_observed, active_buckets, ready_buckets, progress, ready_at,
	last_reset_at, last_reset_reason, sensitivity, current_score, score_updated_at,
	global_median, global_mad, updated_at`

func (s *AnomalyStore) UpsertSeriesState(ctx context.Context, st *anomaly.SeriesState) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO anomaly_series_state (`+anomalySeriesStateColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, metric, dimension) DO UPDATE SET
			node_id=excluded.node_id, state=excluded.state, first_seen_at=excluded.first_seen_at,
			days_observed=excluded.days_observed, active_buckets=excluded.active_buckets,
			ready_buckets=excluded.ready_buckets, progress=excluded.progress, ready_at=excluded.ready_at,
			last_reset_at=excluded.last_reset_at, last_reset_reason=excluded.last_reset_reason,
			sensitivity=excluded.sensitivity, current_score=excluded.current_score,
			score_updated_at=excluded.score_updated_at, global_median=excluded.global_median,
			global_mad=excluded.global_mad, updated_at=excluded.updated_at`,
		st.ScopeType, st.ScopeID, st.Metric, st.Dimension, NullableString(st.NodeID), st.State,
		st.FirstSeenAt, st.DaysObserved, st.ActiveBuckets, st.ReadyBuckets, st.Progress, nullableInt64(st.ReadyAt),
		nullableInt64(st.LastResetAt), NullableString(st.LastResetReason), st.Sensitivity,
		st.CurrentScore, nullableInt64(st.ScoreUpdatedAt), st.GlobalMedian, st.GlobalMAD, st.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert series state: %w", err)
	}
	return nil
}

func (s *AnomalyStore) GetSeriesState(ctx context.Context, key anomaly.SeriesKey) (*anomaly.SeriesState, error) {
	st, err := scanSeriesState(s.db.QueryRowContext(ctx,
		`SELECT `+anomalySeriesStateColumns+` FROM anomaly_series_state
		WHERE scope_type = ? AND scope_id = ? AND metric = ? AND dimension = ?`,
		key.ScopeType, key.ScopeID, key.Metric, key.Dimension))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get series state: %w", err)
	}
	return st, nil
}

// ListSeriesStates returns the states of one scope type, of every scope type when scopeType is empty.
func (s *AnomalyStore) ListSeriesStates(ctx context.Context, scopeType string) ([]*anomaly.SeriesState, error) {
	if scopeType == "" {
		return s.querySeriesStates(ctx, `SELECT `+anomalySeriesStateColumns+` FROM anomaly_series_state`)
	}
	return s.querySeriesStates(ctx,
		`SELECT `+anomalySeriesStateColumns+` FROM anomaly_series_state WHERE scope_type = ?`, scopeType)
}

func (s *AnomalyStore) ListReadySeriesStates(ctx context.Context) ([]*anomaly.SeriesState, error) {
	return s.querySeriesStates(ctx,
		`SELECT `+anomalySeriesStateColumns+` FROM anomaly_series_state WHERE state = ?`, anomaly.StateReady)
}

func (s *AnomalyStore) querySeriesStates(ctx context.Context, query string, args ...any) ([]*anomaly.SeriesState, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list series states: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*anomaly.SeriesState
	for rows.Next() {
		st, err := scanSeriesState(rows)
		if err != nil {
			return nil, fmt.Errorf("scan series state: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func scanSeriesState(row rowScanner) (*anomaly.SeriesState, error) {
	var st anomaly.SeriesState
	var nodeID, lastResetReason sql.NullString
	var readyAt, lastResetAt, scoreUpdatedAt sql.NullInt64
	if err := row.Scan(&st.ScopeType, &st.ScopeID, &st.Metric, &st.Dimension, &nodeID, &st.State,
		&st.FirstSeenAt, &st.DaysObserved, &st.ActiveBuckets, &st.ReadyBuckets, &st.Progress, &readyAt,
		&lastResetAt, &lastResetReason, &st.Sensitivity, &st.CurrentScore, &scoreUpdatedAt,
		&st.GlobalMedian, &st.GlobalMAD, &st.UpdatedAt); err != nil {
		return nil, err
	}
	st.NodeID = nodeID.String
	st.LastResetReason = lastResetReason.String
	st.ReadyAt = int64Ptr(readyAt)
	st.LastResetAt = int64Ptr(lastResetAt)
	st.ScoreUpdatedAt = int64Ptr(scoreUpdatedAt)
	return &st, nil
}

const anomalyEventColumns = `id, scope_type, scope_id, metric, dimension, node_id, detector, tier,
	started_at, ended_at, peak_value, baseline_median, peak_deviation, alert_id, suppressed_by, created_at`

// InsertAnomalyEvent stores e, minting its id when it has none.
func (s *AnomalyStore) InsertAnomalyEvent(ctx context.Context, e *anomaly.AnomalyEvent) error {
	if e.ID == "" {
		e.ID = uid.New()
	}
	_, err := s.writer.Exec(ctx,
		`INSERT INTO anomaly_event (`+anomalyEventColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.ScopeType, e.ScopeID, e.Metric, e.Dimension, NullableString(e.NodeID), e.Detector, e.Tier,
		e.StartedAt, nullableInt64(e.EndedAt), e.PeakValue, e.BaselineMedian, e.PeakDeviation,
		NullableString(e.AlertID), NullableString(e.SuppressedBy), e.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert anomaly event: %w", err)
	}
	return nil
}

func (s *AnomalyStore) GetOpenAnomalyEvent(ctx context.Context, key anomaly.SeriesKey, detector string) (*anomaly.AnomalyEvent, error) {
	query := `SELECT ` + anomalyEventColumns + ` FROM anomaly_event
		WHERE scope_type = ? AND scope_id = ? AND metric = ? AND dimension = ? AND ended_at IS NULL`
	args := []any{key.ScopeType, key.ScopeID, key.Metric, key.Dimension}
	if detector != "" {
		query += ` AND detector = ?`
		args = append(args, detector)
	}
	query += ` ORDER BY started_at DESC LIMIT 1`
	e, err := scanAnomalyEvent(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get open anomaly event: %w", err)
	}
	return e, nil
}

func (s *AnomalyStore) UpdateAnomalyEvent(ctx context.Context, e *anomaly.AnomalyEvent) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE anomaly_event SET tier=?, detector=?, peak_value=?, baseline_median=?, peak_deviation=?,
			alert_id=?, suppressed_by=?, ended_at=?
		WHERE id=?`,
		e.Tier, e.Detector, e.PeakValue, e.BaselineMedian, e.PeakDeviation,
		NullableString(e.AlertID), NullableString(e.SuppressedBy), nullableInt64(e.EndedAt), e.ID)
	if err != nil {
		return fmt.Errorf("update anomaly event: %w", err)
	}
	return nil
}

func (s *AnomalyStore) CloseAnomalyEvent(ctx context.Context, id string, endedAt int64) error {
	_, err := s.writer.Exec(ctx, `UPDATE anomaly_event SET ended_at=? WHERE id=? AND ended_at IS NULL`, endedAt, id)
	if err != nil {
		return fmt.Errorf("close anomaly event: %w", err)
	}
	return nil
}

// ListAnomalyEvents returns the matching events, open ones first, then the most recent first.
func (s *AnomalyStore) ListAnomalyEvents(ctx context.Context, f anomaly.AnomalyEventFilter) ([]*anomaly.AnomalyEvent, error) {
	var where []string
	var args []any
	add := func(clause string, val any) {
		where = append(where, clause)
		args = append(args, val)
	}
	if f.ScopeType != "" {
		add("scope_type = ?", f.ScopeType)
	}
	if f.ScopeID != "" {
		add("scope_id = ?", f.ScopeID)
	}
	if f.NodeID != "" {
		add("node_id = ?", f.NodeID)
	}
	if f.Metric != "" {
		add("metric = ?", f.Metric)
	}
	if f.Tier != "" {
		add("tier = ?", f.Tier)
	}
	if f.Active != nil {
		if *f.Active {
			where = append(where, "ended_at IS NULL")
		} else {
			where = append(where, "ended_at IS NOT NULL")
		}
	}

	query := `SELECT ` + anomalyEventColumns + ` FROM anomaly_event`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY (ended_at IS NULL) DESC, started_at DESC"
	if f.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list anomaly events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*anomaly.AnomalyEvent
	for rows.Next() {
		e, err := scanAnomalyEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan anomaly event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanAnomalyEvent(row rowScanner) (*anomaly.AnomalyEvent, error) {
	var e anomaly.AnomalyEvent
	var nodeID, alertID, suppressedBy sql.NullString
	var endedAt sql.NullInt64
	if err := row.Scan(&e.ID, &e.ScopeType, &e.ScopeID, &e.Metric, &e.Dimension, &nodeID, &e.Detector, &e.Tier,
		&e.StartedAt, &endedAt, &e.PeakValue, &e.BaselineMedian, &e.PeakDeviation, &alertID, &suppressedBy, &e.CreatedAt); err != nil {
		return nil, err
	}
	e.NodeID = nodeID.String
	e.AlertID = alertID.String
	e.SuppressedBy = suppressedBy.String
	e.EndedAt = int64Ptr(endedAt)
	return &e, nil
}

func (s *AnomalyStore) GetSettings(ctx context.Context) (*anomaly.Settings, error) {
	var out anomaly.Settings
	err := s.db.QueryRowContext(ctx,
		`SELECT bucket_pull, updated_at FROM anomaly_settings WHERE id = 1`).Scan(&out.BucketPull, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get anomaly settings: %w", err)
	}
	return &out, nil
}

func (s *AnomalyStore) UpdateSettings(ctx context.Context, st anomaly.Settings) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO anomaly_settings (id, bucket_pull, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET bucket_pull=excluded.bucket_pull, updated_at=excluded.updated_at`,
		st.BucketPull, st.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update anomaly settings: %w", err)
	}
	return nil
}

func nullableInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func int64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

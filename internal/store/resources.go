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

	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/uid"
)

// ResourceStore implements resource.ResourceStore using SQLite.
type ResourceStore struct {
	db     *Reader
	writer *Writer
}

// NewResourceStore creates a new SQLite-backed resource store.
func NewResourceStore(d *DB) *ResourceStore {
	return &ResourceStore{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

func (s *ResourceStore) InsertSnapshot(ctx context.Context, snap *resource.ResourceSnapshot) (string, error) {
	snap.ID = uid.New()
	_, err := s.writer.Exec(ctx,
		`INSERT INTO resource_snapshots (id, container_id, agent_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.ContainerID, uid.Agent(snap.AgentID), snap.CPUPercent, snap.MemUsed, snap.MemLimit,
		snap.NetRxBytes, snap.NetTxBytes, snap.BlockReadBytes, snap.BlockWriteBytes,
		snap.Timestamp.Unix(),
	)
	if err != nil {
		return "", fmt.Errorf("insert resource snapshot: %w", err)
	}
	return snap.ID, nil
}

func (s *ResourceStore) GetLatestSnapshot(ctx context.Context, containerID string) (*resource.ResourceSnapshot, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, container_id, agent_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp
		FROM resource_snapshots WHERE container_id = ? ORDER BY timestamp DESC LIMIT 1`, containerID)
	return scanSnapshot(row)
}

func (s *ResourceStore) ListSnapshots(ctx context.Context, containerID string, from, to time.Time) ([]*resource.ResourceSnapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, container_id, agent_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp
		FROM resource_snapshots WHERE container_id = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp`,
		containerID, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return collectSnapshots(rows)
}

func (s *ResourceStore) ListSnapshotsAggregated(ctx context.Context, containerID string, from, to time.Time, granularity resource.Granularity) ([]*resource.ResourceSnapshot, error) {
	if granularity == resource.GranularityRaw {
		return s.ListSnapshots(ctx, containerID, from, to)
	}

	bucketSec := granularityToSeconds(granularity)
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT '' AS id, container_id, MAX(agent_id) AS agent_id,
			AVG(cpu_percent), CAST(AVG(mem_used) AS BIGINT), CAST(AVG(mem_limit) AS BIGINT),
			CAST(AVG(net_rx_bytes) AS BIGINT), CAST(AVG(net_tx_bytes) AS BIGINT),
			CAST(AVG(block_read_bytes) AS BIGINT), CAST(AVG(block_write_bytes) AS BIGINT),
			(timestamp / %d) * %d AS bucket
		FROM resource_snapshots
		WHERE container_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY container_id, bucket
		ORDER BY bucket`, bucketSec, bucketSec),
		containerID, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("list snapshots aggregated: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return collectSnapshots(rows)
}

// ListHourlyInRange returns the hourly rollup for a container as snapshots, so
// the long ranges read pre-aggregated buckets instead of grouping a week of raw
// rows. Block I/O is 0 for buckets aggregated before it was rolled up.
func (s *ResourceStore) ListHourlyInRange(ctx context.Context, containerID string, from, to time.Time) ([]*resource.ResourceSnapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT '' AS id, container_id, '' AS agent_id,
			avg_cpu_percent, avg_mem_used, avg_mem_limit,
			avg_net_rx_bytes, avg_net_tx_bytes,
			avg_block_read_bytes, avg_block_write_bytes, bucket
		FROM resource_hourly
		WHERE container_id = ? AND bucket >= ? AND bucket <= ?
		ORDER BY bucket`,
		containerID, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("list hourly in range: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return collectSnapshots(rows)
}

// ListDailyInRange returns the daily rollup for a container as snapshots. The
// 90-day window reads it rather than the hourly rollup, which is kept exactly
// 90 days: served from there, its oldest buckets would fall away mid-read and
// two consecutive calls would not cover the same period.
//
// The daily schema carries no block I/O columns, so those two counters are 0 on
// this window. Adding them would need a migration.
func (s *ResourceStore) ListDailyInRange(ctx context.Context, containerID string, from, to time.Time) ([]*resource.ResourceSnapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT '' AS id, container_id, '' AS agent_id,
			avg_cpu_percent, avg_mem_used, avg_mem_limit,
			avg_net_rx_bytes, avg_net_tx_bytes,
			0 AS avg_block_read_bytes, 0 AS avg_block_write_bytes, bucket
		FROM resource_daily
		WHERE container_id = ? AND bucket >= ? AND bucket <= ?
		ORDER BY bucket`,
		containerID, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("list daily in range: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return collectSnapshots(rows)
}

func (s *ResourceStore) GetAlertConfig(ctx context.Context, containerID string) (*resource.ResourceAlertConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, container_id, cpu_threshold, mem_threshold, enabled, alert_state,
			cpu_consecutive_breaches, mem_consecutive_breaches, last_alerted_at, created_at, updated_at
		FROM resource_alert_configs WHERE container_id = ?`, containerID)
	return scanAlertConfig(row)
}

func (s *ResourceStore) UpsertAlertConfig(ctx context.Context, cfg *resource.ResourceAlertConfig) error {
	now := time.Now().Unix()
	var lastAlerted *int64
	if cfg.LastAlertedAt != nil {
		v := cfg.LastAlertedAt.Unix()
		lastAlerted = &v
	}
	_, err := s.writer.Exec(ctx,
		`INSERT INTO resource_alert_configs (id, container_id, cpu_threshold, mem_threshold, enabled, alert_state,
			cpu_consecutive_breaches, mem_consecutive_breaches, last_alerted_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(container_id) DO UPDATE SET
			cpu_threshold=excluded.cpu_threshold, mem_threshold=excluded.mem_threshold,
			enabled=excluded.enabled, alert_state=excluded.alert_state,
			cpu_consecutive_breaches=excluded.cpu_consecutive_breaches,
			mem_consecutive_breaches=excluded.mem_consecutive_breaches,
			last_alerted_at=excluded.last_alerted_at, updated_at=excluded.updated_at`,
		uid.New(), cfg.ContainerID, cfg.CPUThreshold, cfg.MemThreshold,
		boolToInt(cfg.Enabled), string(cfg.AlertState),
		cfg.CPUConsecutiveBreaches, cfg.MemConsecutiveBreaches,
		lastAlerted, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert alert config: %w", err)
	}
	return nil
}

func (s *ResourceStore) DeleteSnapshotsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	deleted, _, err := s.deleteSnapshotsBefore(ctx, before, batchOpts{batchSize: batchSize})
	return deleted, err
}

func (s *ResourceStore) deleteSnapshotsBefore(ctx context.Context, before time.Time, o batchOpts) (int64, bool, error) {
	return deleteRowsBefore(ctx, s.writer, o, "resource_snapshots", "timestamp", before)
}

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type resourceRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanSnapshot(row resourceRowScanner) (*resource.ResourceSnapshot, error) {
	var s resource.ResourceSnapshot
	var ts int64
	err := row.Scan(&s.ID, &s.ContainerID, &s.AgentID, &s.CPUPercent, &s.MemUsed, &s.MemLimit,
		&s.NetRxBytes, &s.NetTxBytes, &s.BlockReadBytes, &s.BlockWriteBytes, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Timestamp = time.Unix(ts, 0)
	return &s, nil
}

func collectSnapshots(rows *sql.Rows) ([]*resource.ResourceSnapshot, error) {
	var result []*resource.ResourceSnapshot
	for rows.Next() {
		s, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func scanAlertConfig(row resourceRowScanner) (*resource.ResourceAlertConfig, error) {
	var cfg resource.ResourceAlertConfig
	var enabled int
	var alertState string
	var lastAlerted sql.NullInt64
	var createdAt, updatedAt int64
	err := row.Scan(&cfg.ID, &cfg.ContainerID, &cfg.CPUThreshold, &cfg.MemThreshold,
		&enabled, &alertState, &cfg.CPUConsecutiveBreaches, &cfg.MemConsecutiveBreaches,
		&lastAlerted, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cfg.Enabled = enabled != 0
	cfg.AlertState = resource.AlertState(alertState)
	if lastAlerted.Valid {
		t := time.Unix(lastAlerted.Int64, 0)
		cfg.LastAlertedAt = &t
	}
	cfg.CreatedAt = time.Unix(createdAt, 0)
	cfg.UpdatedAt = time.Unix(updatedAt, 0)
	return &cfg, nil
}

func (s *ResourceStore) InsertHourlyRollup(ctx context.Context, r *resource.RollupRow) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO resource_hourly (id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(container_id, bucket) DO UPDATE SET
			avg_cpu_percent=excluded.avg_cpu_percent, avg_mem_used=excluded.avg_mem_used,
			avg_mem_limit=excluded.avg_mem_limit, avg_net_rx_bytes=excluded.avg_net_rx_bytes,
			avg_net_tx_bytes=excluded.avg_net_tx_bytes, sample_count=excluded.sample_count`,
		uid.New(), r.ContainerID, r.Bucket.Unix(), r.AvgCPUPercent, r.AvgMemUsed, r.AvgMemLimit, r.AvgNetRx, r.AvgNetTx, r.SampleCount,
	)
	if err != nil {
		return fmt.Errorf("insert hourly rollup: %w", err)
	}
	return nil
}

func (s *ResourceStore) InsertDailyRollup(ctx context.Context, r *resource.RollupRow) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO resource_daily (id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(container_id, bucket) DO UPDATE SET
			avg_cpu_percent=excluded.avg_cpu_percent, avg_mem_used=excluded.avg_mem_used,
			avg_mem_limit=excluded.avg_mem_limit, avg_net_rx_bytes=excluded.avg_net_rx_bytes,
			avg_net_tx_bytes=excluded.avg_net_tx_bytes, sample_count=excluded.sample_count`,
		uid.New(), r.ContainerID, r.Bucket.Unix(), r.AvgCPUPercent, r.AvgMemUsed, r.AvgMemLimit, r.AvgNetRx, r.AvgNetTx, r.SampleCount,
	)
	if err != nil {
		return fmt.Errorf("insert daily rollup: %w", err)
	}
	return nil
}

// GetTopConsumersByPeriod ranks containers by average resource usage over a
// period. agentID filters by host: nil = all hosts, a pointer to "" = the local
// server (containers owned by the LocalAgent sentinel), a pointer to an id = that agent.
func (s *ResourceStore) GetTopConsumersByPeriod(ctx context.Context, metric string, period string, limit int, agentID *string) ([]resource.TopConsumerRow, error) {
	now := time.Now()

	var table, timeCol, valExpr, pctExpr string
	var from int64
	switch period {
	case "1h", "6h":
		hours := 1
		if period == "6h" {
			hours = 6
		}
		table, timeCol, from = "resource_snapshots", "timestamp", now.Add(-time.Duration(hours)*time.Hour).Unix()
		switch metric {
		case "cpu":
			valExpr, pctExpr = "AVG(cpu_percent)", "AVG(cpu_percent)"
		case "memory":
			valExpr = "CAST(AVG(mem_used) AS REAL)"
			pctExpr = "CASE WHEN AVG(mem_limit) > 0 THEN AVG(mem_used) * 100.0 / AVG(mem_limit) ELSE 0 END"
		default:
			return nil, fmt.Errorf("invalid metric: %s", metric)
		}
	case "24h", "7d", "30d", "90d":
		table, timeCol = "resource_daily", "bucket"
		switch period {
		case "24h":
			table = "resource_hourly"
			from = now.Add(-24 * time.Hour).Unix()
		case "7d":
			from = now.AddDate(0, 0, -7).Unix()
		case "90d":
			from = now.AddDate(0, 0, -90).Unix()
		default:
			from = now.AddDate(0, 0, -30).Unix()
		}
		switch metric {
		case "cpu":
			valExpr, pctExpr = "AVG(avg_cpu_percent)", "AVG(avg_cpu_percent)"
		case "memory":
			valExpr = "CAST(AVG(avg_mem_used) AS REAL)"
			pctExpr = "CASE WHEN AVG(avg_mem_limit) > 0 THEN AVG(avg_mem_used) * 100.0 / AVG(avg_mem_limit) ELSE 0 END"
		default:
			return nil, fmt.Errorf("invalid metric: %s", metric)
		}
	default:
		return nil, fmt.Errorf("invalid period: %s", period)
	}

	// Filter by host through the container's owning agent so it works across
	// the snapshot and rollup tables alike (rollups carry no agent_id column).
	args := []any{from}
	hostClause := ""
	if agentID != nil {
		hostClause = " AND container_id IN (SELECT id FROM containers WHERE agent_id = ?)"
		args = append(args, uid.Agent(*agentID))
	}
	args = append(args, limit)

	// #nosec G201 -- every fragment comes from the exhaustive period/metric
	// switches above (invalid values already returned an error); args are bound.
	query := fmt.Sprintf(
		`SELECT container_id, %s AS avg_val, %s AS avg_pct
		FROM %s WHERE %s >= ?%s
		GROUP BY container_id ORDER BY avg_val DESC LIMIT ?`,
		valExpr, pctExpr, table, timeCol, hostClause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get top consumers by period: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var result []resource.TopConsumerRow
	for rows.Next() {
		var row resource.TopConsumerRow
		if err := rows.Scan(&row.ContainerID, &row.AvgValue, &row.AvgPercent); err != nil {
			return nil, fmt.Errorf("scan top consumer row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *ResourceStore) AggregateHourlyRollup(ctx context.Context, bucketStart, bucketEnd time.Time) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO resource_hourly (id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit,
			avg_net_rx_bytes, avg_net_tx_bytes, avg_block_read_bytes, avg_block_write_bytes, sample_count)
		SELECT `+s.db.Dialect().UUIDExpr()+`, container_id, ? AS bucket,
			AVG(cpu_percent), CAST(AVG(mem_used) AS BIGINT), CAST(AVG(mem_limit) AS BIGINT),
			CAST(AVG(net_rx_bytes) AS BIGINT), CAST(AVG(net_tx_bytes) AS BIGINT),
			CAST(AVG(block_read_bytes) AS BIGINT), CAST(AVG(block_write_bytes) AS BIGINT),
			COUNT(*)
		FROM resource_snapshots
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY container_id
		ON CONFLICT(container_id, bucket) DO UPDATE SET
			avg_cpu_percent=excluded.avg_cpu_percent, avg_mem_used=excluded.avg_mem_used,
			avg_mem_limit=excluded.avg_mem_limit, avg_net_rx_bytes=excluded.avg_net_rx_bytes,
			avg_net_tx_bytes=excluded.avg_net_tx_bytes,
			avg_block_read_bytes=excluded.avg_block_read_bytes,
			avg_block_write_bytes=excluded.avg_block_write_bytes,
			sample_count=excluded.sample_count`,
		bucketStart.Unix(), bucketStart.Unix(), bucketEnd.Unix(),
	)
	if err != nil {
		return fmt.Errorf("aggregate hourly rollup: %w", err)
	}
	return nil
}

func (s *ResourceStore) AggregateDailyRollup(ctx context.Context, bucketStart, bucketEnd time.Time) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO resource_daily (id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		SELECT `+s.db.Dialect().UUIDExpr()+`, container_id, ? AS bucket,
			AVG(avg_cpu_percent), CAST(AVG(avg_mem_used) AS BIGINT), CAST(AVG(avg_mem_limit) AS BIGINT),
			CAST(AVG(avg_net_rx_bytes) AS BIGINT), CAST(AVG(avg_net_tx_bytes) AS BIGINT),
			SUM(sample_count)
		FROM resource_hourly
		WHERE bucket >= ? AND bucket < ?
		GROUP BY container_id
		ON CONFLICT(container_id, bucket) DO UPDATE SET
			avg_cpu_percent=excluded.avg_cpu_percent, avg_mem_used=excluded.avg_mem_used,
			avg_mem_limit=excluded.avg_mem_limit, avg_net_rx_bytes=excluded.avg_net_rx_bytes,
			avg_net_tx_bytes=excluded.avg_net_tx_bytes, sample_count=excluded.sample_count`,
		bucketStart.Unix(), bucketStart.Unix(), bucketEnd.Unix(),
	)
	if err != nil {
		return fmt.Errorf("aggregate daily rollup: %w", err)
	}
	return nil
}

func (s *ResourceStore) DeleteHourlyBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	deleted, _, err := s.deleteHourlyBefore(ctx, before, batchOpts{batchSize: batchSize})
	return deleted, err
}

func (s *ResourceStore) deleteHourlyBefore(ctx context.Context, before time.Time, o batchOpts) (int64, bool, error) {
	return deleteRowsBefore(ctx, s.writer, o, "resource_hourly", "bucket", before)
}

func (s *ResourceStore) DeleteDailyBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	deleted, _, err := s.deleteDailyBefore(ctx, before, batchOpts{batchSize: batchSize})
	return deleted, err
}

func (s *ResourceStore) deleteDailyBefore(ctx context.Context, before time.Time, o batchOpts) (int64, bool, error) {
	return deleteRowsBefore(ctx, s.writer, o, "resource_daily", "bucket", before)
}

func granularityToSeconds(g resource.Granularity) int64 {
	switch g {
	case resource.Granularity1m:
		return 60
	case resource.Granularity5m:
		return 300
	case resource.Granularity1h:
		return 3600
	default:
		return 60
	}
}

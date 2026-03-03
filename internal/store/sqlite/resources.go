// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/resource"
)

// ResourceStore implements resource.ResourceStore using SQLite.
type ResourceStore struct {
	db     *sql.DB
	writer *Writer
}

// NewResourceStore creates a new SQLite-backed resource store.
func NewResourceStore(d *DB) *ResourceStore {
	return &ResourceStore{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *ResourceStore) InsertSnapshot(ctx context.Context, snap *resource.ResourceSnapshot) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO resource_snapshots (container_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ContainerID, snap.CPUPercent, snap.MemUsed, snap.MemLimit,
		snap.NetRxBytes, snap.NetTxBytes, snap.BlockReadBytes, snap.BlockWriteBytes,
		snap.Timestamp.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert resource snapshot: %w", err)
	}
	snap.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *ResourceStore) GetLatestSnapshot(ctx context.Context, containerID int64) (*resource.ResourceSnapshot, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, container_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp
		FROM resource_snapshots WHERE container_id = ? ORDER BY timestamp DESC LIMIT 1`, containerID)
	return scanSnapshot(row)
}

func (s *ResourceStore) ListSnapshots(ctx context.Context, containerID int64, from, to time.Time) ([]*resource.ResourceSnapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, container_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp
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

func (s *ResourceStore) ListSnapshotsAggregated(ctx context.Context, containerID int64, from, to time.Time, granularity resource.Granularity) ([]*resource.ResourceSnapshot, error) {
	if granularity == resource.GranularityRaw {
		return s.ListSnapshots(ctx, containerID, from, to)
	}

	bucketSec := granularityToSeconds(granularity)
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT 0 AS id, container_id,
			AVG(cpu_percent), CAST(AVG(mem_used) AS INTEGER), CAST(AVG(mem_limit) AS INTEGER),
			CAST(AVG(net_rx_bytes) AS INTEGER), CAST(AVG(net_tx_bytes) AS INTEGER),
			CAST(AVG(block_read_bytes) AS INTEGER), CAST(AVG(block_write_bytes) AS INTEGER),
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

func (s *ResourceStore) GetAlertConfig(ctx context.Context, containerID int64) (*resource.ResourceAlertConfig, error) {
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
		`INSERT INTO resource_alert_configs (container_id, cpu_threshold, mem_threshold, enabled, alert_state,
			cpu_consecutive_breaches, mem_consecutive_breaches, last_alerted_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(container_id) DO UPDATE SET
			cpu_threshold=excluded.cpu_threshold, mem_threshold=excluded.mem_threshold,
			enabled=excluded.enabled, alert_state=excluded.alert_state,
			cpu_consecutive_breaches=excluded.cpu_consecutive_breaches,
			mem_consecutive_breaches=excluded.mem_consecutive_breaches,
			last_alerted_at=excluded.last_alerted_at, updated_at=excluded.updated_at`,
		cfg.ContainerID, cfg.CPUThreshold, cfg.MemThreshold,
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
	res, err := s.writer.Exec(ctx,
		`DELETE FROM resource_snapshots WHERE id IN (SELECT id FROM resource_snapshots WHERE timestamp < ? LIMIT ?)`,
		before.Unix(), batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete snapshots before: %w", err)
	}
	return res.RowsAffected, nil
}

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type resourceRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanSnapshot(row resourceRowScanner) (*resource.ResourceSnapshot, error) {
	var s resource.ResourceSnapshot
	var ts int64
	err := row.Scan(&s.ID, &s.ContainerID, &s.CPUPercent, &s.MemUsed, &s.MemLimit,
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
		`INSERT OR REPLACE INTO resource_hourly (container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ContainerID, r.Bucket.Unix(), r.AvgCPUPercent, r.AvgMemUsed, r.AvgMemLimit, r.AvgNetRx, r.AvgNetTx, r.SampleCount,
	)
	if err != nil {
		return fmt.Errorf("insert hourly rollup: %w", err)
	}
	return nil
}

func (s *ResourceStore) InsertDailyRollup(ctx context.Context, r *resource.RollupRow) error {
	_, err := s.writer.Exec(ctx,
		`INSERT OR REPLACE INTO resource_daily (container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ContainerID, r.Bucket.Unix(), r.AvgCPUPercent, r.AvgMemUsed, r.AvgMemLimit, r.AvgNetRx, r.AvgNetTx, r.SampleCount,
	)
	if err != nil {
		return fmt.Errorf("insert daily rollup: %w", err)
	}
	return nil
}

func (s *ResourceStore) GetTopConsumersByPeriod(ctx context.Context, metric string, period string, limit int) ([]resource.TopConsumerRow, error) {
	now := time.Now()
	var query string

	switch period {
	case "1h":
		from := now.Add(-1 * time.Hour).Unix()
		switch metric {
		case "cpu":
			query = fmt.Sprintf(
				`SELECT container_id, AVG(cpu_percent) AS avg_val, AVG(cpu_percent) AS avg_pct
				FROM resource_snapshots WHERE timestamp >= %d
				GROUP BY container_id ORDER BY avg_val DESC LIMIT %d`, from, limit)
		case "memory":
			query = fmt.Sprintf(
				`SELECT container_id, CAST(AVG(mem_used) AS REAL) AS avg_val,
					CASE WHEN AVG(mem_limit) > 0 THEN AVG(mem_used) * 100.0 / AVG(mem_limit) ELSE 0 END AS avg_pct
				FROM resource_snapshots WHERE timestamp >= %d
				GROUP BY container_id ORDER BY avg_pct DESC LIMIT %d`, from, limit)
		}
	case "24h":
		from := now.Add(-24 * time.Hour).Unix()
		switch metric {
		case "cpu":
			query = fmt.Sprintf(
				`SELECT container_id, AVG(avg_cpu_percent) AS avg_val, AVG(avg_cpu_percent) AS avg_pct
				FROM resource_hourly WHERE bucket >= %d
				GROUP BY container_id ORDER BY avg_val DESC LIMIT %d`, from, limit)
		case "memory":
			query = fmt.Sprintf(
				`SELECT container_id, CAST(AVG(avg_mem_used) AS REAL) AS avg_val,
					CASE WHEN AVG(avg_mem_limit) > 0 THEN AVG(avg_mem_used) * 100.0 / AVG(avg_mem_limit) ELSE 0 END AS avg_pct
				FROM resource_hourly WHERE bucket >= %d
				GROUP BY container_id ORDER BY avg_pct DESC LIMIT %d`, from, limit)
		}
	case "7d", "30d":
		days := 7
		if period == "30d" {
			days = 30
		}
		from := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
		switch metric {
		case "cpu":
			query = fmt.Sprintf(
				`SELECT container_id, AVG(avg_cpu_percent) AS avg_val, AVG(avg_cpu_percent) AS avg_pct
				FROM resource_daily WHERE bucket >= %d
				GROUP BY container_id ORDER BY avg_val DESC LIMIT %d`, from, limit)
		case "memory":
			query = fmt.Sprintf(
				`SELECT container_id, CAST(AVG(avg_mem_used) AS REAL) AS avg_val,
					CASE WHEN AVG(avg_mem_limit) > 0 THEN AVG(avg_mem_used) * 100.0 / AVG(avg_mem_limit) ELSE 0 END AS avg_pct
				FROM resource_daily WHERE bucket >= %d
				GROUP BY container_id ORDER BY avg_pct DESC LIMIT %d`, from, limit)
		}
	default:
		return nil, fmt.Errorf("invalid period: %s", period)
	}

	rows, err := s.db.QueryContext(ctx, query)
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
		`INSERT OR REPLACE INTO resource_hourly (container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		SELECT container_id, ? AS bucket,
			AVG(cpu_percent), CAST(AVG(mem_used) AS INTEGER), CAST(AVG(mem_limit) AS INTEGER),
			CAST(AVG(net_rx_bytes) AS INTEGER), CAST(AVG(net_tx_bytes) AS INTEGER),
			COUNT(*)
		FROM resource_snapshots
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY container_id`,
		bucketStart.Unix(), bucketStart.Unix(), bucketEnd.Unix(),
	)
	if err != nil {
		return fmt.Errorf("aggregate hourly rollup: %w", err)
	}
	return nil
}

func (s *ResourceStore) AggregateDailyRollup(ctx context.Context, bucketStart, bucketEnd time.Time) error {
	_, err := s.writer.Exec(ctx,
		`INSERT OR REPLACE INTO resource_daily (container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		SELECT container_id, ? AS bucket,
			AVG(avg_cpu_percent), CAST(AVG(avg_mem_used) AS INTEGER), CAST(AVG(avg_mem_limit) AS INTEGER),
			CAST(AVG(avg_net_rx_bytes) AS INTEGER), CAST(AVG(avg_net_tx_bytes) AS INTEGER),
			SUM(sample_count)
		FROM resource_hourly
		WHERE bucket >= ? AND bucket < ?
		GROUP BY container_id`,
		bucketStart.Unix(), bucketStart.Unix(), bucketEnd.Unix(),
	)
	if err != nil {
		return fmt.Errorf("aggregate daily rollup: %w", err)
	}
	return nil
}

func (s *ResourceStore) DeleteHourlyBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`DELETE FROM resource_hourly WHERE id IN (SELECT id FROM resource_hourly WHERE bucket < ? LIMIT ?)`,
		before.Unix(), batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete hourly before: %w", err)
	}
	return res.RowsAffected, nil
}

func (s *ResourceStore) DeleteDailyBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`DELETE FROM resource_daily WHERE id IN (SELECT id FROM resource_daily WHERE bucket < ? LIMIT ?)`,
		before.Unix(), batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete daily before: %w", err)
	}
	return res.RowsAffected, nil
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

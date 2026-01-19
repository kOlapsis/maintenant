package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/pulseboard/internal/resource"
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
	defer rows.Close()
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
	defer rows.Close()
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
	if err == sql.ErrNoRows {
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
	if err == sql.ErrNoRows {
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

package resource

import (
	"context"
	"time"
)

// Granularity defines the time-bucket size for aggregated queries.
type Granularity string

const (
	GranularityRaw  Granularity = "raw"
	Granularity1m   Granularity = "1m"
	Granularity5m   Granularity = "5m"
	Granularity1h   Granularity = "1h"
)

// ResourceStore defines the persistence interface for resource monitoring data.
type ResourceStore interface {
	InsertSnapshot(ctx context.Context, s *ResourceSnapshot) (int64, error)
	GetLatestSnapshot(ctx context.Context, containerID int64) (*ResourceSnapshot, error)
	ListSnapshots(ctx context.Context, containerID int64, from, to time.Time) ([]*ResourceSnapshot, error)
	ListSnapshotsAggregated(ctx context.Context, containerID int64, from, to time.Time, granularity Granularity) ([]*ResourceSnapshot, error)

	GetAlertConfig(ctx context.Context, containerID int64) (*ResourceAlertConfig, error)
	UpsertAlertConfig(ctx context.Context, cfg *ResourceAlertConfig) error

	DeleteSnapshotsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
}

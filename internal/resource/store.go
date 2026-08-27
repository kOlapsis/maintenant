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

package resource

import (
	"context"
	"time"
)

// Granularity defines the time-bucket size for aggregated queries.
type Granularity string

const (
	GranularityRaw Granularity = "raw"
	Granularity1m  Granularity = "1m"
	Granularity5m  Granularity = "5m"
	Granularity1h  Granularity = "1h"
	// Granularity1d labels the points of the 90-day window. It is not an
	// on-the-fly bucket size: the daily rollup already holds those buckets.
	Granularity1d Granularity = "1d"
)

// DefaultSnapshotRetention is how long raw samples are kept. Only the ranges up
// to 24h read them; longer ranges are served from the hourly rollup, so keeping
// raw data beyond that would grow the database for nothing. It also bounds how
// far back the rollup backfills: past it there is no raw data left to aggregate.
const DefaultSnapshotRetention = 48 * time.Hour

// ResourceStore defines the persistence interface for resource monitoring data.
type ResourceStore interface {
	InsertSnapshot(ctx context.Context, s *ResourceSnapshot) (string, error)
	GetLatestSnapshot(ctx context.Context, containerID string) (*ResourceSnapshot, error)
	ListSnapshots(ctx context.Context, containerID string, from, to time.Time) ([]*ResourceSnapshot, error)
	ListSnapshotsAggregated(ctx context.Context, containerID string, from, to time.Time, granularity Granularity) ([]*ResourceSnapshot, error)
	ListHourlyInRange(ctx context.Context, containerID string, from, to time.Time) ([]*ResourceSnapshot, error)
	ListDailyInRange(ctx context.Context, containerID string, from, to time.Time) ([]*ResourceSnapshot, error)

	GetAlertConfig(ctx context.Context, containerID string) (*ResourceAlertConfig, error)
	UpsertAlertConfig(ctx context.Context, cfg *ResourceAlertConfig) error

	DeleteSnapshotsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)

	InsertHourlyRollup(ctx context.Context, r *RollupRow) error
	InsertDailyRollup(ctx context.Context, r *RollupRow) error
	AggregateHourlyRollup(ctx context.Context, bucketStart, bucketEnd time.Time) error
	AggregateDailyRollup(ctx context.Context, bucketStart, bucketEnd time.Time) error
	GetTopConsumersByPeriod(ctx context.Context, metric string, period string, limit int, agentID *string) ([]TopConsumerRow, error)
	DeleteHourlyBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteDailyBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
}

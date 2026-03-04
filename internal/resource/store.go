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

	InsertHourlyRollup(ctx context.Context, r *RollupRow) error
	InsertDailyRollup(ctx context.Context, r *RollupRow) error
	AggregateHourlyRollup(ctx context.Context, bucketStart, bucketEnd time.Time) error
	AggregateDailyRollup(ctx context.Context, bucketStart, bucketEnd time.Time) error
	GetTopConsumersByPeriod(ctx context.Context, metric string, period string, limit int) ([]TopConsumerRow, error)
	DeleteHourlyBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteDailyBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
}

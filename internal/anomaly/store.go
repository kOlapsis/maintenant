// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package anomaly

import "context"

// Store persists baselines, series states, anomaly events and settings, and reads the resource history they are learned from.
type Store interface {
	ListContainerHourly(ctx context.Context, since int64) ([]HourlyResourceRow, error)
	ListContainerSnapshotsSince(ctx context.Context, since int64) ([]SnapshotRow, error)

	UpsertBaseline(ctx context.Context, b Baseline) error
	GetBaseline(ctx context.Context, key SeriesKey, bucket int) (*Baseline, error)
	ListBaselines(ctx context.Context, key SeriesKey) ([]Baseline, error)
	DeleteBaselinesForScope(ctx context.Context, scopeType, scopeID string) error

	UpsertSeriesState(ctx context.Context, s *SeriesState) error
	GetSeriesState(ctx context.Context, key SeriesKey) (*SeriesState, error)
	ListSeriesStates(ctx context.Context, scopeType string) ([]*SeriesState, error)
	ListReadySeriesStates(ctx context.Context) ([]*SeriesState, error)

	// GetSettings returns nil until the operator has saved settings once.
	GetSettings(ctx context.Context) (*Settings, error)
	UpdateSettings(ctx context.Context, s Settings) error

	InsertAnomalyEvent(ctx context.Context, e *AnomalyEvent) error
	// GetOpenAnomalyEvent matches any detector when detector is empty.
	GetOpenAnomalyEvent(ctx context.Context, key SeriesKey, detector string) (*AnomalyEvent, error)
	UpdateAnomalyEvent(ctx context.Context, e *AnomalyEvent) error
	CloseAnomalyEvent(ctx context.Context, id string, endedAt int64) error
	ListAnomalyEvents(ctx context.Context, f AnomalyEventFilter) ([]*AnomalyEvent, error)
}

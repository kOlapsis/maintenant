// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"time"
)

const rollupInterval = 5 * time.Minute

func (s *Service) startRollupLoop(ctx context.Context) {
	s.logger.Info("starting resource rollup loop", "interval", rollupInterval)

	s.runRollups(ctx)

	ticker := time.NewTicker(rollupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runRollups(ctx)
		}
	}
}

func (s *Service) runRollups(ctx context.Context) {
	now := time.Now().UTC()

	hourlyStart := now.Add(-s.rawWindow).Truncate(time.Hour)
	currentHour := now.Truncate(time.Hour)
	hourlyBuckets := 0
	for b := hourlyStart; b.Before(currentHour); b = b.Add(time.Hour) {
		hourlyBuckets++
	}

	dailyStart := now.Add(-s.rawWindow).UTC()
	dailyStart = time.Date(dailyStart.Year(), dailyStart.Month(), dailyStart.Day(), 0, 0, 0, 0, time.UTC)
	currentDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dailyBuckets := 0
	for b := dailyStart; b.Before(currentDay); b = b.Add(24 * time.Hour) {
		dailyBuckets++
	}

	s.rollupHourly(ctx, now)
	s.rollupDaily(ctx, now)

	s.logger.Info("resource: rollup completed", "hourly_buckets", hourlyBuckets, "daily_buckets", dailyBuckets)
}

func (s *Service) rollupHourly(ctx context.Context, now time.Time) {
	currentHour := now.Truncate(time.Hour)
	backfillStart := now.Add(-s.rawWindow).Truncate(time.Hour)

	for bucket := backfillStart; bucket.Before(currentHour); bucket = bucket.Add(time.Hour) {
		bucketEnd := bucket.Add(time.Hour)
		if err := s.store.AggregateHourlyRollup(ctx, bucket, bucketEnd); err != nil {
			s.logger.Error("resource rollup: hourly bucket", "bucket", bucket, "error", err)
		}
	}
}

func (s *Service) rollupDaily(ctx context.Context, now time.Time) {
	currentDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	backfillStart := now.Add(-s.rawWindow).UTC()
	backfillStart = time.Date(backfillStart.Year(), backfillStart.Month(), backfillStart.Day(), 0, 0, 0, 0, time.UTC)

	for bucket := backfillStart; bucket.Before(currentDay); bucket = bucket.Add(24 * time.Hour) {
		bucketEnd := bucket.Add(24 * time.Hour)
		if err := s.store.AggregateDailyRollup(ctx, bucket, bucketEnd); err != nil {
			s.logger.Error("resource rollup: daily bucket", "bucket", bucket, "error", err)
		}
	}
}

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

const (
	rollupInterval = 5 * time.Minute
	backfillLimit  = 7 * 24 * time.Hour
)

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

	hourlyStart := now.Add(-backfillLimit).Truncate(time.Hour)
	currentHour := now.Truncate(time.Hour)
	hourlyBuckets := 0
	for b := hourlyStart; b.Before(currentHour); b = b.Add(time.Hour) {
		hourlyBuckets++
	}

	dailyStart := now.Add(-backfillLimit).UTC()
	dailyStart = time.Date(dailyStart.Year(), dailyStart.Month(), dailyStart.Day(), 0, 0, 0, 0, time.UTC)
	currentDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dailyBuckets := 0
	for b := dailyStart; b.Before(currentDay); b = b.Add(24 * time.Hour) {
		dailyBuckets++
	}

	s.rollupHourly(ctx)
	s.rollupDaily(ctx)

	s.logger.Info("resource: rollup completed", "hourly_buckets", hourlyBuckets, "daily_buckets", dailyBuckets)
}

func (s *Service) rollupHourly(ctx context.Context) {
	now := time.Now().UTC()
	currentHour := now.Truncate(time.Hour)
	backfillStart := now.Add(-backfillLimit).Truncate(time.Hour)

	for bucket := backfillStart; bucket.Before(currentHour); bucket = bucket.Add(time.Hour) {
		bucketEnd := bucket.Add(time.Hour)
		if err := s.store.AggregateHourlyRollup(ctx, bucket, bucketEnd); err != nil {
			s.logger.Error("resource rollup: hourly bucket", "bucket", bucket, "error", err)
		}
	}
}

func (s *Service) rollupDaily(ctx context.Context) {
	now := time.Now().UTC()
	currentDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	backfillStart := now.Add(-backfillLimit).UTC()
	backfillStart = time.Date(backfillStart.Year(), backfillStart.Month(), backfillStart.Day(), 0, 0, 0, 0, time.UTC)

	for bucket := backfillStart; bucket.Before(currentDay); bucket = bucket.Add(24 * time.Hour) {
		bucketEnd := bucket.Add(24 * time.Hour)
		if err := s.store.AggregateDailyRollup(ctx, bucket, bucketEnd); err != nil {
			s.logger.Error("resource rollup: daily bucket", "bucket", bucket, "error", err)
		}
	}
}

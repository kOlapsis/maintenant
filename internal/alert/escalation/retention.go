// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package escalation

import (
	"context"
	"log/slog"
	"time"
)

// RetentionDays is the number of days after which ended runs are purged.
const RetentionDays = 90

// RunRetentionLoop runs a nightly purge of escalation runs and deliveries older than RetentionDays.
// It wakes up each time it is 03:00 local time (computed at each iteration). Batches of 1000 rows
// are deleted in separate transactions to avoid blocking writes. Use ctx cancellation to stop.
func (s *Service) RunRetentionLoop(ctx context.Context) {
	for {
		next := nextRetentionTick(s.clock())
		select {
		case <-ctx.Done():
			return
		case <-time.After(next):
		}
		before := s.clock().Add(-RetentionDays * 24 * time.Hour)
		if err := s.store.PurgeRunsAndDeliveriesOlderThan(ctx, before); err != nil {
			slog.ErrorContext(ctx, "escalation: retention purge failed", "err", err)
		} else {
			slog.InfoContext(ctx, "escalation: retention purge complete", "before", before.Format(time.RFC3339))
		}
	}
}

// clock returns the current time. Overridable in tests via clockFn field.
func (s *Service) clock() time.Time {
	if s.clockFn != nil {
		return s.clockFn()
	}
	return time.Now()
}

// nextRetentionTick returns the duration until 03:00 local time tomorrow (or today if it hasn't happened yet).
func nextRetentionTick(now time.Time) time.Duration {
	target := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}

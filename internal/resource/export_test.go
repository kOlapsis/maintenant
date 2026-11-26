// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"log/slog"
	"time"
)

// Collect runs one collection pass.
func (c *Collector) Collect(ctx context.Context) { c.collect(ctx) }

// RollupHourlyAt runs the hourly rollup as the service would at now.
func RollupHourlyAt(ctx context.Context, store ResourceStore, rawWindow time.Duration, now time.Time) {
	(&Service{store: store, rawWindow: rawWindow, logger: slog.Default()}).rollupHourly(ctx, now)
}

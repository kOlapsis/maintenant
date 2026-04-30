// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import (
	"context"
	"log/slog"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
)

// counter is the consumer-side interface satisfied structurally by each
// store the telemetry subsystem reads from. Defining it here (per
// constitution principle IV) keeps store packages free of telemetry
// awareness.
type counter interface {
	CountConfigured(ctx context.Context) (int, error)
}

// editionSource exposes the in-process edition flag. Implementations may
// simply close over `extension.CurrentEdition` — the indirection lets
// tests inject deterministic values.
type editionSource interface {
	Current() extension.Edition
}

// EditionFunc adapts a func into an editionSource so callers can pass
// `telemetry.EditionFunc(extension.CurrentEdition)` at wiring time
// without declaring a struct.
type EditionFunc func() extension.Edition

// Current returns the edition resolved at call time. No caching.
func (f EditionFunc) Current() extension.Edition { return f() }

// providerCollectTimeout bounds how long the closure spends pulling
// counts from SQLite per cycle. Snapshots are infrequent (1h default),
// so a generous timeout is safe — but unbounded would risk blocking the
// SDK ticker if a counter ever hung.
const providerCollectTimeout = 5 * time.Second

// newMetricsProvider returns a closure suitable for shm.MetricsProvider.
// The closure builds the seven-key flat map fresh per cycle, never
// returning an error (the SDK's interface has no error channel anyway).
// On per-counter failure, the value is reported as 0 and a single WARN
// is logged. This preserves partial visibility (FR-009) and contains
// counter panics (FR-012).
func newMetricsProvider(deps Deps, logger *slog.Logger) func() map[string]any {
	if logger == nil {
		logger = slog.Default()
	}
	return func() map[string]any {
		ctx, cancel := context.WithTimeout(context.Background(), providerCollectTimeout)
		defer cancel()

		return map[string]any{
			"edition":                 safeEdition(deps.Edition, logger),
			"containers_total":        safeCount(ctx, deps.Containers, "containers", logger),
			"endpoints_total":         safeCount(ctx, deps.Endpoints, "endpoints", logger),
			"heartbeats_total":        safeCount(ctx, deps.Heartbeats, "heartbeats", logger),
			"certificates_total":      safeCount(ctx, deps.Certificates, "certificates", logger),
			"webhooks_total":          safeCount(ctx, deps.Webhooks, "webhooks", logger),
			"status_components_total": safeCount(ctx, deps.StatusComponents, "status_components", logger),
		}
	}
}

// safeCount calls c.CountConfigured under recover. On panic or error,
// returns 0 and logs once at WARN with a stable counter name attribute.
func safeCount(ctx context.Context, c counter, name string, logger *slog.Logger) (n int) {
	if c == nil {
		logger.Warn("telemetry counter missing", "counter", name)
		return 0
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("telemetry counter panic", "counter", name, "panic", r)
			n = 0
		}
	}()
	count, err := c.CountConfigured(ctx)
	if err != nil {
		logger.Warn("telemetry counter failed", "counter", name, "error", err)
		return 0
	}
	if count < 0 {
		return 0
	}
	return count
}

// safeEdition resolves the edition flag under recover. On panic or nil
// source, returns "community" and logs at WARN.
func safeEdition(src editionSource, logger *slog.Logger) (out string) {
	out = editionCommunity
	if src == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("telemetry edition source panic", "panic", r)
			out = editionCommunity
		}
	}()
	return mapEdition(src.Current())
}

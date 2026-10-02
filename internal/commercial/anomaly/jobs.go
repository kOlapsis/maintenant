// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"
	"log/slog"
	"sync"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// jobs runs the baseline and detection loops while the running edition opens anomaly detection.
type jobs struct {
	baseline *BaselineEngine
	runner   *DetectionRunner
	cfg      Config
	logger   *slog.Logger

	mu     sync.Mutex
	parent context.Context
	cancel context.CancelFunc
}

func newJobs(d extpoint.AnomalyDeps, cfg Config, logger *slog.Logger) *jobs {
	state := NewSeriesStateManager(d.Store, cfg, logger)
	baseline := NewBaselineEngine(d.Store, state, cfg, logger)
	baseline.SetStateChangeBroadcaster(func(st *model.SeriesState) {
		d.Broadcaster.BroadcastEvent(EventStateChanged(st))
	})
	runner := NewDetectionRunner(d.Store, cfg, logger)
	runner.SetBroadcaster(d.Broadcaster.BroadcastEvent)
	runner.SetAlertBridge(NewAnomalyAlertBridge(d.Alerts, cfg.Severity))
	return &jobs{baseline: baseline, runner: runner, cfg: cfg, logger: logger}
}

// Start runs the loops under ctx if the running edition opens anomaly detection.
func (j *jobs) Start(ctx context.Context) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.parent = ctx
	j.apply(extension.Allows(extension.CapAnomalyDetection))
}

// OnEditionChange starts or stops the loops when the edition crosses the anomaly detection boundary.
func (j *jobs) OnEditionChange(_ context.Context, _, next extension.Edition) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.apply(next.AtLeast(extension.MinEdition(extension.CapAnomalyDetection)))
}

// apply must be called with mu held; before Start there is no parent context to run under yet.
func (j *jobs) apply(open bool) {
	switch {
	case open && j.cancel == nil && j.parent != nil:
		ctx, cancel := context.WithCancel(j.parent)
		j.cancel = cancel
		go j.baselineLoop(ctx)
		go j.detectionLoop(ctx)
		j.logger.Info("anomaly detection started")
	case !open && j.cancel != nil:
		j.cancel()
		j.cancel = nil
		j.logger.Info("anomaly detection stopped")
	}
}

func (j *jobs) baselineLoop(ctx context.Context) {
	j.recompute(ctx)
	ticker := time.NewTicker(j.cfg.BaselineInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.recompute(ctx)
		}
	}
}

func (j *jobs) recompute(ctx context.Context) {
	if err := j.baseline.Recompute(ctx); err != nil && ctx.Err() == nil {
		j.logger.WarnContext(ctx, "anomaly: baseline recompute failed", "error", err)
	}
}

func (j *jobs) detectionLoop(ctx context.Context) {
	defer j.runner.Flush()
	ticker := time.NewTicker(j.cfg.DetectInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := j.runner.Run(ctx); err != nil && ctx.Err() == nil {
				j.logger.WarnContext(ctx, "anomaly: detection run failed", "error", err)
			}
		}
	}
}

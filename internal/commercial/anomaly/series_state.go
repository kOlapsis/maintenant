// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

// Progression is the learning evidence a recompute pass gathered for one series.
type Progression struct {
	FirstSeenAt   int64
	DaysObserved  float64 // capped at the baseline window
	ActiveBuckets int
	ReadyBuckets  int     // buckets holding at least min_samples rows
	CoverageFill  float64 // 0..1, how full the active buckets are toward min_samples
	GlobalMedian  float64
	GlobalMAD     float64
}

// SeriesStateManager owns the lifecycle of a series: learning, ready, relearning, disabled.
type SeriesStateManager struct {
	store  model.Store
	cfg    Config
	logger *slog.Logger
	now    func() int64
}

// NewSeriesStateManager builds a state manager.
func NewSeriesStateManager(store model.Store, cfg Config, logger *slog.Logger) *SeriesStateManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SeriesStateManager{
		store:  store,
		cfg:    cfg,
		logger: logger,
		now:    func() int64 { return time.Now().Unix() },
	}
}

func (m *SeriesStateManager) requiredDaysFor(state string) float64 {
	if state == model.StateRelearning {
		return float64(m.cfg.RelearnDays)
	}
	return float64(m.cfg.RequiredDays)
}

// ComputeProgress returns the lower of time progress and bucket coverage, so 1.0 means both readiness criteria hold.
func (m *SeriesStateManager) ComputeProgress(state string, p Progression) float64 {
	required := m.requiredDaysFor(state)
	timeProg := 1.0
	if required > 0 {
		timeProg = clamp01(p.DaysObserved / required)
	}
	coverageProg := 0.0
	if p.ActiveBuckets > 0 {
		coverageProg = clamp01(p.CoverageFill)
	}
	return math.Min(timeProg, coverageProg)
}

// EstimatedReadyAt estimates when a learning or relearning series turns ready.
func EstimatedReadyAt(cfg Config, state string, firstSeenAt int64) int64 {
	requiredDays := float64(cfg.RequiredDays)
	if state == model.StateRelearning {
		requiredDays = float64(cfg.RelearnDays)
	}
	coverageDays := float64(cfg.MinSamples * 7)
	days := math.Max(requiredDays, coverageDays)
	return firstSeenAt + int64(days*86400)
}

// ApplyProgression writes the state a recompute pass leads to and reports whether it moved enough to broadcast.
func (m *SeriesStateManager) ApplyProgression(ctx context.Context, key model.SeriesKey, nodeID, sensitivity string, p Progression) (*model.SeriesState, bool, error) {
	now := m.now()
	existing, err := m.store.GetSeriesState(ctx, key)
	if err != nil {
		return nil, false, fmt.Errorf("apply progression: load state: %w", err)
	}

	st := &model.SeriesState{SeriesKey: key}
	prevState := model.StateLearning
	prevProgress := 0.0
	if existing != nil {
		if existing.State == model.StateDisabled {
			return existing, false, nil
		}
		st = existing
		prevState = existing.State
		prevProgress = existing.Progress
	} else {
		st.State = model.StateLearning
		st.FirstSeenAt = p.FirstSeenAt
	}

	st.NodeID = nodeID
	st.Sensitivity = sensitivity
	st.DaysObserved = p.DaysObserved
	st.ActiveBuckets = p.ActiveBuckets
	st.ReadyBuckets = p.ReadyBuckets
	st.GlobalMedian = p.GlobalMedian
	st.GlobalMAD = p.GlobalMAD
	st.UpdatedAt = now
	if st.FirstSeenAt == 0 {
		st.FirstSeenAt = p.FirstSeenAt
	}

	progress := m.ComputeProgress(st.State, p)
	switch st.State {
	case model.StateLearning, model.StateRelearning:
		if progress >= 1.0 && p.ActiveBuckets > 0 {
			st.State = model.StateReady
			st.Progress = 1.0
			st.ReadyAt = &now
		} else {
			st.Progress = progress
		}
	case model.StateReady:
		st.Progress = 1.0
	}

	if err := m.store.UpsertSeriesState(ctx, st); err != nil {
		return nil, false, fmt.Errorf("apply progression: persist state: %w", err)
	}

	changed := st.State != prevState || math.Abs(st.Progress-prevProgress) >= progressStep
	return st, changed, nil
}

// Disable marks a series disabled and reports whether it changed.
func (m *SeriesStateManager) Disable(ctx context.Context, key model.SeriesKey) (bool, error) {
	existing, err := m.store.GetSeriesState(ctx, key)
	if err != nil {
		return false, fmt.Errorf("disable series: load state: %w", err)
	}
	now := m.now()
	st := &model.SeriesState{SeriesKey: key, FirstSeenAt: now, Sensitivity: m.cfg.DefaultSensitivity}
	if existing != nil {
		if existing.State == model.StateDisabled {
			return false, nil
		}
		st = existing
	}
	st.State = model.StateDisabled
	st.Progress = 0
	st.UpdatedAt = now
	if err := m.store.UpsertSeriesState(ctx, st); err != nil {
		return false, fmt.Errorf("disable series: persist: %w", err)
	}
	return true, nil
}

// ResetScope purges a scope's baselines and restarts the learning of each of its series, relearning after a new image.
func (m *SeriesStateManager) ResetScope(ctx context.Context, scopeType, scopeID, reason string) error {
	if err := m.store.DeleteBaselinesForScope(ctx, scopeType, scopeID); err != nil {
		return fmt.Errorf("reset scope: purge baselines: %w", err)
	}
	states, err := m.store.ListSeriesStates(ctx, scopeType)
	if err != nil {
		return fmt.Errorf("reset scope: list states: %w", err)
	}
	now := m.now()
	target := model.StateLearning
	if reason == model.ResetReasonImageDigestChange {
		target = model.StateRelearning
	}
	for _, st := range states {
		if st.ScopeID != scopeID || st.State == model.StateDisabled {
			continue
		}
		st.State = target
		st.Progress = 0
		st.ReadyAt = nil
		st.ReadyBuckets = 0
		st.LastResetAt = &now
		st.LastResetReason = reason
		st.UpdatedAt = now
		if err := m.store.UpsertSeriesState(ctx, st); err != nil {
			return fmt.Errorf("reset scope: persist %s: %w", st.Metric, err)
		}
	}
	m.logger.InfoContext(ctx, "anomaly: scope baselines reset", "scope_type", scopeType, "scope_id", scopeID, "reason", reason, "target_state", target)
	return nil
}

// progressStep is the smallest progress move worth an anomaly.state_changed event.
const progressStep = 0.05

func clamp01(v float64) float64 {
	return min(max(v, 0), 1)
}

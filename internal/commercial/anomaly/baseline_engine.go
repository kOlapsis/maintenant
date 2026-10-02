// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

// BaselineEngine recomputes per-series baselines from the container hourly rollups and advances each series' learning.
type BaselineEngine struct {
	store          model.Store
	stateMgr       *SeriesStateManager
	cfg            Config
	logger         *slog.Logger
	now            func() int64
	onStateChanged func(*model.SeriesState)
}

// NewBaselineEngine builds a baseline engine.
func NewBaselineEngine(store model.Store, stateMgr *SeriesStateManager, cfg Config, logger *slog.Logger) *BaselineEngine {
	if logger == nil {
		logger = slog.Default()
	}
	return &BaselineEngine{
		store:    store,
		stateMgr: stateMgr,
		cfg:      cfg,
		logger:   logger,
		now:      func() int64 { return time.Now().Unix() },
	}
}

// SetStateChangeBroadcaster registers the callback told when a series state or progress moves enough to show.
func (e *BaselineEngine) SetStateChangeBroadcaster(fn func(*model.SeriesState)) {
	e.onStateChanged = fn
}

// Recompute reads the rolling window of hourly rollups, recomputes every baseline bucket and advances each series.
func (e *BaselineEngine) Recompute(ctx context.Context) error {
	now := e.now()
	windowStart := now - int64(e.cfg.BaselineWindowDays)*86400

	rows, err := e.store.ListContainerHourly(ctx, windowStart)
	if err != nil {
		return fmt.Errorf("recompute baselines: load hourly: %w", err)
	}

	scopes := map[string]*scopeAccum{}
	for _, r := range rows {
		sid := ScopeID(r.ScopeIdentity)
		sa := scopes[sid]
		if sa == nil {
			sa = &scopeAccum{nodeID: NodeID(r.AgentID), hours: map[int64]*hourAgg{}}
			scopes[sid] = sa
		}
		sa.add(r)
	}

	metrics := supportedContainerMetrics(e.cfg.DefaultMetrics)
	for sid, sa := range scopes {
		if err := e.recomputeScope(ctx, sid, sa, metrics, now); err != nil {
			return err
		}
	}
	e.logger.DebugContext(ctx, "anomaly: baselines recomputed", "scopes", len(scopes), "hourly_rows", len(rows))
	return nil
}

func (e *BaselineEngine) recomputeScope(ctx context.Context, scopeID string, sa *scopeAccum, metrics []string, now int64) error {
	loc := e.cfg.Location
	if loc == nil {
		loc = time.UTC
	}

	valsByMetric := make(map[string]map[int][]float64, len(metrics))
	for _, m := range metrics {
		valsByMetric[m] = map[int][]float64{}
	}
	towCount := map[int]int{}

	for hourBucket, agg := range sa.hours {
		tow := TimeOfWeekBucket(time.Unix(hourBucket, 0), loc)
		towCount[tow]++
		for _, m := range metrics {
			valsByMetric[m][tow] = append(valsByMetric[m][tow], agg.value(m))
		}
	}

	activeBuckets := len(towCount)
	readyBuckets := 0
	filled := 0
	for _, cnt := range towCount {
		if cnt >= e.cfg.MinSamples {
			readyBuckets++
			filled += e.cfg.MinSamples
		} else {
			filled += cnt
		}
	}

	type level struct{ median, mad float64 }
	globals := make(map[string]level, len(metrics))
	for _, m := range metrics {
		var all []float64
		for _, vals := range valsByMetric[m] {
			all = append(all, vals...)
		}
		globals[m] = level{Median(all), MAD(all)}

		for tow, vals := range valsByMetric[m] {
			b := model.Baseline{
				SeriesKey:   model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: scopeID, Metric: m},
				Bucket:      tow,
				Median:      Median(vals),
				MAD:         MAD(vals),
				SampleCount: len(vals),
				UpdatedAt:   now,
			}
			if err := e.store.UpsertBaseline(ctx, b); err != nil {
				return fmt.Errorf("recompute scope %s: %w", scopeID, err)
			}
		}
	}

	daysObserved := float64(now-sa.earliest) / 86400
	if windowDays := float64(e.cfg.BaselineWindowDays); daysObserved > windowDays {
		daysObserved = windowDays
	}
	coverageFill := 0.0
	if activeBuckets > 0 {
		coverageFill = float64(filled) / float64(activeBuckets*e.cfg.MinSamples)
	}
	prog := Progression{
		FirstSeenAt:   sa.earliest,
		DaysObserved:  daysObserved,
		ActiveBuckets: activeBuckets,
		ReadyBuckets:  readyBuckets,
		CoverageFill:  coverageFill,
	}

	for _, m := range metrics {
		key := model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: scopeID, Metric: m}
		prog.GlobalMedian, prog.GlobalMAD = globals[m].median, globals[m].mad
		st, changed, err := e.stateMgr.ApplyProgression(ctx, key, sa.nodeID, e.cfg.DefaultSensitivity, prog)
		if err != nil {
			return fmt.Errorf("recompute scope %s: advance %s: %w", scopeID, m, err)
		}
		if changed && e.onStateChanged != nil {
			e.onStateChanged(st)
		}
	}
	return nil
}

// supportedContainerMetrics keeps the configured container metrics the hourly rollup can feed.
func supportedContainerMetrics(metrics []string) []string {
	var out []string
	for _, m := range metrics {
		if IsMetricValid(model.ScopeTypeContainer, m) && isBaselineSupported(m) {
			out = append(out, m)
		}
	}
	return out
}

func isBaselineSupported(metric string) bool {
	switch metric {
	case model.MetricCPU, model.MetricMemory, model.MetricNetworkIO:
		return true
	default:
		return false
	}
}

type scopeAccum struct {
	nodeID   string
	hours    map[int64]*hourAgg
	earliest int64
}

func (s *scopeAccum) add(r model.HourlyResourceRow) {
	h := s.hours[r.Bucket]
	if h == nil {
		h = &hourAgg{}
		s.hours[r.Bucket] = h
	}
	h.cpuSum += r.CPUPercent
	h.memSum += r.MemUsed
	h.netRxSum += r.NetRxBytes
	h.netTxSum += r.NetTxBytes
	h.replicaN++
	if s.earliest == 0 || r.Bucket < s.earliest {
		s.earliest = r.Bucket
	}
}

type hourAgg struct {
	cpuSum, memSum     float64
	netRxSum, netTxSum float64
	replicaN           int
}

// value averages utilization metrics across replicas and sums throughput.
func (h *hourAgg) value(metric string) float64 {
	switch metric {
	case model.MetricCPU:
		if h.replicaN == 0 {
			return 0
		}
		return h.cpuSum / float64(h.replicaN)
	case model.MetricMemory:
		if h.replicaN == 0 {
			return 0
		}
		return h.memSum / float64(h.replicaN)
	case model.MetricNetworkIO:
		return h.netRxSum + h.netTxSum
	default:
		return 0
	}
}

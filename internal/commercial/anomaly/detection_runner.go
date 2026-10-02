// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

const (
	recentWindowSec = 1800
	sampleAlignSec  = 60
)

// AlertBridge raises and recovers the alert of an active anomaly.
type AlertBridge interface {
	OnAnomalyOpened(ctx context.Context, e *model.AnomalyEvent) (alertID string, err error)
	OnAnomalyClosed(ctx context.Context, e *model.AnomalyEvent) error
}

// DetectionRunner runs one detection pass over the ready series, opening, upgrading and closing anomaly events.
type DetectionRunner struct {
	store     model.Store
	cfg       Config
	logger    *slog.Logger
	now       func() int64
	bridge    AlertBridge
	broadcast func(eventType string, data any)

	mu       sync.Mutex
	breaches map[string]int // consecutive spike breaches per series
}

// NewDetectionRunner builds a detection runner; without a bridge or a broadcaster it raises no alert or event.
func NewDetectionRunner(store model.Store, cfg Config, logger *slog.Logger) *DetectionRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &DetectionRunner{
		store:    store,
		cfg:      cfg,
		logger:   logger,
		now:      func() int64 { return time.Now().Unix() },
		breaches: map[string]int{},
	}
}

// SetAlertBridge wires the bridge told about active anomalies.
func (r *DetectionRunner) SetAlertBridge(b AlertBridge) { r.bridge = b }

// SetBroadcaster wires the sender of anomaly.opened and anomaly.closed events.
func (r *DetectionRunner) SetBroadcaster(fn func(eventType string, data any)) { r.broadcast = fn }

// Flush forgets the spike persistence counters, so detection restarts clean after a pause.
func (r *DetectionRunner) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.breaches = map[string]int{}
}

// Run executes one detection pass over every ready series.
func (r *DetectionRunner) Run(ctx context.Context) error {
	now := r.now()
	loc := r.cfg.Location
	if loc == nil {
		loc = time.UTC
	}
	tow := TimeOfWeekBucket(time.Unix(now, 0), loc)

	settings, err := LoadSettings(ctx, r.store)
	if err != nil {
		return fmt.Errorf("detection run: load settings: %w", err)
	}

	metrics := supportedContainerMetrics(r.cfg.DefaultMetrics)
	rows, err := r.store.ListContainerSnapshotsSince(ctx, now-recentWindowSec)
	if err != nil {
		return fmt.Errorf("detection run: load snapshots: %w", err)
	}
	samples := BuildContainerSamples(rows, metrics, sampleAlignSec)

	readyStates, err := r.store.ListReadySeriesStates(ctx)
	if err != nil {
		return fmt.Errorf("detection run: load ready series: %w", err)
	}
	ready := make(map[model.SeriesKey]*model.SeriesState, len(readyStates))
	for _, st := range readyStates {
		ready[st.SeriesKey] = st
	}

	for _, sample := range samples {
		st, ok := ready[sample.Key]
		if !ok {
			continue
		}
		baseline, err := r.store.GetBaseline(ctx, sample.Key, tow)
		if err != nil {
			return fmt.Errorf("detection run: get baseline: %w", err)
		}
		if baseline == nil {
			continue
		}
		effective := Shrink(*baseline, st.GlobalMedian, st.GlobalMAD, settings.BucketPull)
		if err := r.processSample(ctx, sample, st, effective, now); err != nil {
			return err
		}
	}
	return nil
}

type classification struct {
	anomalous bool
	active    bool
	detector  string
	deviation float64 // signed, in effective MAD units
}

func (r *DetectionRunner) processSample(ctx context.Context, sample SeriesSample, st *model.SeriesState, baseline model.Baseline, now int64) error {
	metric := sample.Key.Metric
	th := ThresholdsFor(st.Sensitivity)
	madEff := EffectiveMAD(baseline.MAD, baseline.Median, RelativeMADFloor, AbsoluteMADFloor(metric))

	spike := EvaluateSpike(sample.Value, baseline, metric, th)
	drift := EvaluateDrift(sample.Recent, baseline, metric, th)
	cp := EvaluateChangePoint(sample.Recent, baseline, metric, th)

	ks := keyStr(sample.Key)
	r.mu.Lock()
	if spike.Breach {
		r.breaches[ks]++
	} else {
		delete(r.breaches, ks)
	}
	spikeActive := r.breaches[ks] >= r.cfg.SpikePersistence
	r.mu.Unlock()

	cls := classify(spike, spikeActive, drift, cp, madEff)

	open, err := r.store.GetOpenAnomalyEvent(ctx, sample.Key, "")
	if err != nil {
		return fmt.Errorf("process sample: get open event: %w", err)
	}

	if !cls.anomalous {
		if open == nil {
			return nil
		}
		return r.resolve(ctx, open, now)
	}

	tier := model.TierPassive
	if cls.active {
		tier = model.TierActive
	}

	if open == nil {
		return r.openEvent(ctx, sample, st, baseline, cls, tier, now)
	}
	return r.updateEvent(ctx, open, sample, cls, tier)
}

func (r *DetectionRunner) resolve(ctx context.Context, open *model.AnomalyEvent, now int64) error {
	if err := r.store.CloseAnomalyEvent(ctx, open.ID, now); err != nil {
		return fmt.Errorf("resolve anomaly: %w", err)
	}
	open.EndedAt = &now
	if open.Tier == model.TierActive && r.bridge != nil {
		if err := r.bridge.OnAnomalyClosed(ctx, open); err != nil {
			r.logger.WarnContext(ctx, "anomaly: recovery alert failed", "error", err)
		}
	}
	r.emit(EventClosed(open))
	return nil
}

func (r *DetectionRunner) openEvent(ctx context.Context, sample SeriesSample, st *model.SeriesState, baseline model.Baseline, cls classification, tier string, now int64) error {
	ev := &model.AnomalyEvent{
		SeriesKey:      sample.Key,
		NodeID:         st.NodeID,
		Detector:       cls.detector,
		Tier:           tier,
		StartedAt:      now,
		PeakValue:      sample.Value,
		BaselineMedian: baseline.Median,
		PeakDeviation:  cls.deviation,
		CreatedAt:      now,
	}
	if err := r.store.InsertAnomalyEvent(ctx, ev); err != nil {
		return fmt.Errorf("open anomaly: %w", err)
	}
	if tier == model.TierActive {
		r.raise(ctx, ev)
	}
	r.emit(EventOpened(ev))
	return nil
}

func (r *DetectionRunner) updateEvent(ctx context.Context, open *model.AnomalyEvent, sample SeriesSample, cls classification, tier string) error {
	changed := false
	if math.Abs(cls.deviation) > math.Abs(open.PeakDeviation) {
		open.PeakDeviation = cls.deviation
		open.PeakValue = sample.Value
		changed = true
	}
	upgraded := tier == model.TierActive && open.Tier == model.TierPassive
	if upgraded {
		open.Tier = model.TierActive
		open.Detector = cls.detector
		changed = true
	}
	if changed {
		if err := r.store.UpdateAnomalyEvent(ctx, open); err != nil {
			return fmt.Errorf("update anomaly: %w", err)
		}
	}
	if upgraded {
		r.raise(ctx, open)
		r.emit(EventOpened(open))
	}
	return nil
}

func (r *DetectionRunner) raise(ctx context.Context, ev *model.AnomalyEvent) {
	if r.bridge == nil {
		return
	}
	alertID, err := r.bridge.OnAnomalyOpened(ctx, ev)
	if err != nil {
		r.logger.WarnContext(ctx, "anomaly: alert raise failed", "error", err)
		return
	}
	if alertID != "" && alertID != ev.AlertID {
		ev.AlertID = alertID
		if err := r.store.UpdateAnomalyEvent(ctx, ev); err != nil {
			r.logger.WarnContext(ctx, "anomaly: persist alert id failed", "error", err)
		}
	}
}

func (r *DetectionRunner) emit(eventType string, data any) {
	if r.broadcast != nil {
		r.broadcast(eventType, data)
	}
}

// classify ranks the signals: a persistent spike first, then drift, then a level shift; a band hit or a young breach stays passive.
func classify(spike SpikeSignal, spikeActive bool, drift DriftSignal, cp ChangePointSignal, madEff float64) classification {
	switch {
	case spikeActive:
		return classification{anomalous: true, active: true, detector: model.DetectorSpike, deviation: spike.Deviation}
	case drift.Drift:
		return classification{anomalous: true, active: true, detector: model.DetectorDrift, deviation: drift.TotalChange / madEff}
	case cp.Changed:
		return classification{anomalous: true, active: true, detector: model.DetectorChangePoint, deviation: cp.Shift}
	case spike.Band || spike.Breach:
		return classification{anomalous: true, active: false, detector: model.DetectorSpike, deviation: spike.Deviation}
	default:
		return classification{}
	}
}

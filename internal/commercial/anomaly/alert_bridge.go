// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	model "github.com/kolapsis/maintenant/internal/anomaly"
)

// AlertTypeBehavioralAnomaly is the alert type of every series anomaly, whatever the detector, so a series holds one alert at most.
const AlertTypeBehavioralAnomaly = "behavioral_anomaly"

// EntityTypeAnomalySeries is the alert entity type of a series anomaly.
const EntityTypeAnomalySeries = "anomaly_series"

// AnomalyAlertBridge raises an alert when an active anomaly opens and its recovery when it closes.
type AnomalyAlertBridge struct {
	send     func(alert.Event)
	severity string
	now      func() int64
}

// NewAnomalyAlertBridge builds a bridge that sends alert events through send at the given severity.
func NewAnomalyAlertBridge(send func(alert.Event), severity string) *AnomalyAlertBridge {
	if severity == "" {
		severity = alert.SeverityWarning
	}
	return &AnomalyAlertBridge{
		send:     send,
		severity: severity,
		now:      func() int64 { return time.Now().Unix() },
	}
}

func anomalyEntityID(k model.SeriesKey) string {
	return fmt.Sprintf("%s:%s:%s:%s", k.ScopeType, k.ScopeID, k.Metric, k.Dimension)
}

// OnAnomalyOpened raises the series alert; the engine resolves the alert id on its own, so none is returned.
func (b *AnomalyAlertBridge) OnAnomalyOpened(_ context.Context, e *model.AnomalyEvent) (string, error) {
	b.send(alert.Event{
		Source:     alert.SourceResourceAnomaly,
		AlertType:  AlertTypeBehavioralAnomaly,
		Severity:   b.severity,
		Message:    b.message(e),
		EntityType: EntityTypeAnomalySeries,
		EntityID:   anomalyEntityID(e.SeriesKey),
		EntityName: e.ScopeID + " · " + e.Metric,
		AgentID:    e.NodeID,
		Details: map[string]any{
			"detector":        e.Detector,
			"metric":          e.Metric,
			"scope_type":      e.ScopeType,
			"scope_id":        e.ScopeID,
			"node_id":         e.NodeID,
			"peak_value":      e.PeakValue,
			"baseline_median": e.BaselineMedian,
			"peak_deviation":  e.PeakDeviation,
		},
		Timestamp: time.Unix(e.StartedAt, 0),
	})
	return "", nil
}

// OnAnomalyClosed sends the recovery of a previously active anomaly.
func (b *AnomalyAlertBridge) OnAnomalyClosed(_ context.Context, e *model.AnomalyEvent) error {
	ended := b.now()
	if e.EndedAt != nil {
		ended = *e.EndedAt
	}
	b.send(alert.Event{
		Source:     alert.SourceResourceAnomaly,
		AlertType:  AlertTypeBehavioralAnomaly,
		Severity:   b.severity,
		IsRecover:  true,
		Message:    fmt.Sprintf("%s · %s returned to its expected range", e.ScopeID, e.Metric),
		EntityType: EntityTypeAnomalySeries,
		EntityID:   anomalyEntityID(e.SeriesKey),
		EntityName: e.ScopeID + " · " + e.Metric,
		AgentID:    e.NodeID,
		Timestamp:  time.Unix(ended, 0),
	})
	return nil
}

func (b *AnomalyAlertBridge) message(e *model.AnomalyEvent) string {
	switch e.Detector {
	case model.DetectorDrift:
		return fmt.Sprintf("%s · %s is drifting from its learned baseline (median %.2f)", e.ScopeID, e.Metric, e.BaselineMedian)
	case model.DetectorChangePoint:
		return fmt.Sprintf("%s · %s shifted to a new level away from its baseline (median %.2f)", e.ScopeID, e.Metric, e.BaselineMedian)
	default:
		return fmt.Sprintf("%s · %s spiked well outside its learned baseline (median %.2f)", e.ScopeID, e.Metric, e.BaselineMedian)
	}
}

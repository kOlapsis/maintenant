// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	model "github.com/kolapsis/maintenant/internal/anomaly"
	"github.com/kolapsis/maintenant/internal/event"
)

// EventOpened builds the anomaly.opened event.
func EventOpened(e *model.AnomalyEvent) (string, any) {
	return event.AnomalyOpened, map[string]any{
		"id":              e.ID,
		"scope_type":      e.ScopeType,
		"scope_id":        e.ScopeID,
		"metric":          e.Metric,
		"dimension":       e.Dimension,
		"node_id":         e.NodeID,
		"detector":        e.Detector,
		"tier":            e.Tier,
		"started_at":      e.StartedAt,
		"peak_deviation":  e.PeakDeviation,
		"baseline_median": e.BaselineMedian,
		"peak_value":      e.PeakValue,
	}
}

// EventClosed builds the anomaly.closed event.
func EventClosed(e *model.AnomalyEvent) (string, any) {
	var endedAt int64
	if e.EndedAt != nil {
		endedAt = *e.EndedAt
	}
	return event.AnomalyClosed, map[string]any{
		"id":         e.ID,
		"scope_type": e.ScopeType,
		"scope_id":   e.ScopeID,
		"metric":     e.Metric,
		"ended_at":   endedAt,
	}
}

// EventStateChanged builds the anomaly.state_changed event.
func EventStateChanged(st *model.SeriesState) (string, any) {
	data := map[string]any{
		"scope_type": st.ScopeType,
		"scope_id":   st.ScopeID,
		"metric":     st.Metric,
		"dimension":  st.Dimension,
		"state":      st.State,
		"progress":   st.Progress,
	}
	if st.ReadyAt != nil {
		data["ready_at"] = *st.ReadyAt
	}
	if st.LastResetReason != "" {
		data["last_reset_reason"] = st.LastResetReason
	}
	return event.AnomalyStateChanged, data
}

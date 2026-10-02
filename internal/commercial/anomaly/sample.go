// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"slices"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

// SeriesSample is the latest value and the recent window, oldest first, of one series.
type SeriesSample struct {
	Key       model.SeriesKey
	NodeID    string
	Value     float64
	Recent    []float64
	Timestamp int64
}

// BuildContainerSamples groups raw snapshots by scope and aligns replicas on alignSec ticks the way baselines aggregate them.
func BuildContainerSamples(rows []model.SnapshotRow, metrics []string, alignSec int64) []SeriesSample {
	if alignSec <= 0 {
		alignSec = 60
	}
	type scopeData struct {
		nodeID  string
		buckets map[int64]*hourAgg
	}
	scopes := map[string]*scopeData{}
	for _, r := range rows {
		sid := ScopeID(r.ScopeIdentity)
		sd := scopes[sid]
		if sd == nil {
			sd = &scopeData{nodeID: NodeID(r.AgentID), buckets: map[int64]*hourAgg{}}
			scopes[sid] = sd
		}
		ts := (r.Timestamp / alignSec) * alignSec
		agg := sd.buckets[ts]
		if agg == nil {
			agg = &hourAgg{}
			sd.buckets[ts] = agg
		}
		agg.cpuSum += r.CPUPercent
		agg.memSum += r.MemUsed
		agg.netRxSum += r.NetRxBytes
		agg.netTxSum += r.NetTxBytes
		agg.replicaN++
	}

	var out []SeriesSample
	for sid, sd := range scopes {
		times := make([]int64, 0, len(sd.buckets))
		for ts := range sd.buckets {
			times = append(times, ts)
		}
		slices.Sort(times)
		latest := times[len(times)-1]

		for _, m := range metrics {
			recent := make([]float64, len(times))
			for i, ts := range times {
				recent[i] = sd.buckets[ts].value(m)
			}
			out = append(out, SeriesSample{
				Key:       model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: sid, Metric: m},
				NodeID:    sd.nodeID,
				Value:     sd.buckets[latest].value(m),
				Recent:    recent,
				Timestamp: latest,
			})
		}
	}
	return out
}

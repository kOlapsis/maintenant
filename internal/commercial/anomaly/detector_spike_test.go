// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"testing"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
)

func cpuBaseline(median, mad float64) model.Baseline {
	return model.Baseline{SeriesKey: model.SeriesKey{ScopeType: model.ScopeTypeContainer, Metric: model.MetricCPU}, Median: median, MAD: mad, SampleCount: 4}
}

func TestEvaluateSpike(t *testing.T) {
	b := cpuBaseline(50, 5) // madEff = max(5, 2.5, 1) = 5
	th := ThresholdsFor(model.SensitivityMedium)

	// Normal jitter near the median: neither band nor breach.
	n := EvaluateSpike(52, b, model.MetricCPU, th)
	assert.False(t, n.Breach)
	assert.False(t, n.Band)

	// Moderate deviation: passive band (kLow < |z| <= k).
	mod := EvaluateSpike(70, b, model.MetricCPU, th)
	assert.True(t, mod.Band)
	assert.False(t, mod.Breach)

	// Large deviation: active breach.
	big := EvaluateSpike(90, b, model.MetricCPU, th)
	assert.True(t, big.Breach)
	assert.Greater(t, big.Deviation, th.K)
}

func TestEvaluateSpikeConstantSeriesNoDivByZero(t *testing.T) {
	// Constant baseline (mad 0): the floor keeps it division-safe and a tiny
	// jitter does not blow up to a breach.
	b := cpuBaseline(10, 0)
	th := ThresholdsFor(model.SensitivityMedium)
	s := EvaluateSpike(10.5, b, model.MetricCPU, th)
	assert.False(t, s.Breach)
}

func TestEvaluateSpikeSensitivityMapping(t *testing.T) {
	b := cpuBaseline(50, 5) // madEff = 5
	// value 74 -> |z| ≈ 3.24: an active breach at high, only a passive band at medium.
	assert.True(t, EvaluateSpike(74, b, model.MetricCPU, ThresholdsFor(model.SensitivityHigh)).Breach)
	mid74 := EvaluateSpike(74, b, model.MetricCPU, ThresholdsFor(model.SensitivityMedium))
	assert.False(t, mid74.Breach)
	assert.True(t, mid74.Band)
	// value 70 -> |z| ≈ 2.70: a band at medium, nothing at low.
	assert.True(t, EvaluateSpike(70, b, model.MetricCPU, ThresholdsFor(model.SensitivityMedium)).Band)
	low70 := EvaluateSpike(70, b, model.MetricCPU, ThresholdsFor(model.SensitivityLow))
	assert.False(t, low70.Band)
	assert.False(t, low70.Breach)
}

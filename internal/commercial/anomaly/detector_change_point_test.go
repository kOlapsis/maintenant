// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"testing"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateChangePointStep(t *testing.T) {
	b := cpuBaseline(20, 2) // madEff = 2; threshold |shift| > 3.5 => recent median > 27
	th := ThresholdsFor(model.SensitivityMedium)
	step := []float64{30, 31, 29, 30, 32, 30} // settled at ~30
	sig := EvaluateChangePoint(step, b, model.MetricCPU, th)
	assert.True(t, sig.Changed)
	assert.Greater(t, sig.Shift, th.K)
}

func TestEvaluateChangePointIsolatedSpikeNotAChange(t *testing.T) {
	b := cpuBaseline(20, 2)
	th := ThresholdsFor(model.SensitivityMedium)
	// One transient spike among normal values: the recent median is unmoved.
	spike := []float64{20, 21, 90, 20, 19, 21}
	assert.False(t, EvaluateChangePoint(spike, b, model.MetricCPU, th).Changed)
}

func TestEvaluateChangePointTooFewPoints(t *testing.T) {
	b := cpuBaseline(20, 2)
	th := ThresholdsFor(model.SensitivityMedium)
	assert.False(t, EvaluateChangePoint([]float64{40, 40}, b, model.MetricCPU, th).Changed)
}

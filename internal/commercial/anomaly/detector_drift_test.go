// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"testing"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateDriftLeak(t *testing.T) {
	b := cpuBaseline(20, 2) // madEff = max(2, 1, 1) = 2; threshold = 3.5*2 = 7
	th := ThresholdsFor(model.SensitivityMedium)
	// A steady leak: end-to-end change 14 > 7, perfect fit.
	leak := []float64{20, 22, 24, 26, 28, 30, 32, 34}
	sig := EvaluateDrift(leak, b, model.MetricCPU, th)
	assert.True(t, sig.Drift)
	assert.GreaterOrEqual(t, sig.R2, driftR2Gate)
}

func TestEvaluateDriftFlat(t *testing.T) {
	b := cpuBaseline(20, 2)
	th := ThresholdsFor(model.SensitivityMedium)
	flat := []float64{20, 20, 20, 20, 20, 20}
	assert.False(t, EvaluateDrift(flat, b, model.MetricCPU, th).Drift)
}

func TestEvaluateDriftNoisyLowR2(t *testing.T) {
	b := cpuBaseline(20, 2)
	th := ThresholdsFor(model.SensitivityMedium)
	// Large swings but no consistent direction: low R² rejects it.
	noisy := []float64{20, 35, 12, 33, 14, 31, 15}
	sig := EvaluateDrift(noisy, b, model.MetricCPU, th)
	assert.False(t, sig.Drift)
}

func TestEvaluateDriftTooFewPoints(t *testing.T) {
	b := cpuBaseline(20, 2)
	th := ThresholdsFor(model.SensitivityMedium)
	assert.False(t, EvaluateDrift([]float64{20, 40}, b, model.MetricCPU, th).Drift)
}

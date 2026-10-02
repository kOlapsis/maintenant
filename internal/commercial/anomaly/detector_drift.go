// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"math"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

const (
	driftMinPoints = 5
	driftR2Gate    = 0.6
)

// DriftSignal reports a sustained trend over the recent window.
type DriftSignal struct {
	TotalChange float64 // end-to-end change implied by the fit
	R2          float64
	Drift       bool
}

// EvaluateDrift flags drift when a linear fit of the recent window moves by more than K effective MADs with a good fit.
func EvaluateDrift(recent []float64, b model.Baseline, metric string, th Thresholds) DriftSignal {
	if len(recent) < driftMinPoints {
		return DriftSignal{}
	}
	xs := make([]float64, len(recent))
	for i := range recent {
		xs[i] = float64(i)
	}
	res := Linreg(xs, recent)
	madEff := EffectiveMAD(b.MAD, b.Median, RelativeMADFloor, AbsoluteMADFloor(metric))
	totalChange := res.Slope * float64(len(recent)-1)
	return DriftSignal{
		TotalChange: totalChange,
		R2:          res.R2,
		Drift:       math.Abs(totalChange) >= th.K*madEff && res.R2 >= driftR2Gate,
	}
}

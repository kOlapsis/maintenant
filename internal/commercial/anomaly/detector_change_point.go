// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"math"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

const changePointMinPoints = 5

// ChangePointSignal reports a level the recent window has settled at, away from the baseline.
type ChangePointSignal struct {
	Shift   float64 // (recent median - baseline median) / effective MAD
	Changed bool
}

// EvaluateChangePoint compares the median of the recent window to the baseline median, so a lone spike does not trip it.
func EvaluateChangePoint(recent []float64, b model.Baseline, metric string, th Thresholds) ChangePointSignal {
	if len(recent) < changePointMinPoints {
		return ChangePointSignal{}
	}
	madEff := EffectiveMAD(b.MAD, b.Median, RelativeMADFloor, AbsoluteMADFloor(metric))
	shift := (Median(recent) - b.Median) / madEff
	return ChangePointSignal{
		Shift:   shift,
		Changed: math.Abs(shift) > th.K,
	}
}

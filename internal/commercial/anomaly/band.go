// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import model "github.com/kolapsis/maintenant/internal/anomaly"

// BaselineBand returns the expected range (median ± k·MAD) of a baseline bucket, the lower edge clamped at zero.
func BaselineBand(b model.Baseline, sensitivity, metric string) (lower, median, upper float64) {
	madEff := EffectiveMAD(b.MAD, b.Median, RelativeMADFloor, AbsoluteMADFloor(metric))
	k := ThresholdsFor(sensitivity).K
	lower = b.Median - k*madEff
	if lower < 0 {
		lower = 0
	}
	return lower, b.Median, b.Median + k*madEff
}

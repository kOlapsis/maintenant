// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"math"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

// SpikeSignal is the evaluation of one sample against its seasonal baseline, before the persistence gate.
type SpikeSignal struct {
	Deviation float64 // signed modified z-score
	Breach    bool    // |z| > K
	Band      bool    // KLow < |z| <= K
}

// EvaluateSpike scores value against its hour-of-week baseline.
func EvaluateSpike(value float64, b model.Baseline, metric string, th Thresholds) SpikeSignal {
	madEff := EffectiveMAD(b.MAD, b.Median, RelativeMADFloor, AbsoluteMADFloor(metric))
	z := ModifiedZ(value, b.Median, madEff)
	az := math.Abs(z)
	return SpikeSignal{
		Deviation: z,
		Breach:    az > th.K,
		Band:      az > th.KLow && az <= th.K,
	}
}

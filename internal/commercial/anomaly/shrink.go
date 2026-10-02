// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

// Bounds and default of the bucket pull setting; 0 keeps the raw per-bucket baseline.
const (
	MinBucketPull     = 0
	MaxBucketPull     = 20
	DefaultBucketPull = 3
)

// DefaultSettings returns the settings in force until the operator saves their own.
func DefaultSettings() model.Settings {
	return model.Settings{BucketPull: DefaultBucketPull}
}

// LoadSettings returns the stored settings, or the defaults when none were saved.
func LoadSettings(ctx context.Context, store model.Store) (model.Settings, error) {
	s, err := store.GetSettings(ctx)
	if err != nil {
		return model.Settings{}, err
	}
	if s == nil {
		return DefaultSettings(), nil
	}
	out := *s
	out.BucketPull = ClampBucketPull(out.BucketPull)
	return out, nil
}

// Shrink pulls a bucket baseline toward the series level, weighing its sample count against pull virtual samples.
func Shrink(b model.Baseline, globalMedian, globalMAD float64, pull int) model.Baseline {
	if pull <= 0 || b.SampleCount <= 0 {
		return b
	}
	if globalMedian == 0 && globalMAD == 0 {
		return b
	}
	n := float64(b.SampleCount)
	w := n / (n + float64(pull))

	b.Median = w*b.Median + (1-w)*globalMedian
	b.MAD = w*b.MAD + (1-w)*globalMAD
	return b
}

// ClampBucketPull keeps a pull inside the supported range.
func ClampBucketPull(v int) int {
	return min(max(v, MinBucketPull), MaxBucketPull)
}

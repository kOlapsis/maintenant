// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"testing"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
)

func bucketBaseline(median, mad float64, samples int) model.Baseline {
	return model.Baseline{Median: median, MAD: mad, SampleCount: samples}
}

func TestShrinkPullIsProportionalToSampleCount(t *testing.T) {
	const global, pull = 400.0, 3

	// One sample keeps a quarter of its own level, four keep four sevenths.
	one := Shrink(bucketBaseline(100, 0, 1), global, 0, pull)
	assert.InDelta(t, 0.25*100+0.75*400, one.Median, 1e-9)

	four := Shrink(bucketBaseline(100, 0, 4), global, 0, pull)
	assert.InDelta(t, (4.0/7.0)*100+(3.0/7.0)*400, four.Median, 1e-9)

	// The better observed bucket stays closer to what it measured.
	assert.Less(t, four.Median, one.Median)
}

func TestShrinkDisabledAtZero(t *testing.T) {
	b := bucketBaseline(100, 12, 1)
	assert.Equal(t, b, Shrink(b, 400, 50, 0))
	assert.Equal(t, b, Shrink(b, 400, 50, -1))
}

func TestShrinkPullsSpreadToo(t *testing.T) {
	// A bucket whose samples happened to agree keeps a near-zero MAD and would
	// flag on any movement; the series spread pulls it back to something usable.
	got := Shrink(bucketBaseline(100, 0, 1), 100, 40, 3)
	assert.InDelta(t, 30.0, got.MAD, 1e-9)
}

func TestShrinkLeavesUnrecordedSeriesAlone(t *testing.T) {
	// A series with no recorded level is left as measured rather than pulled toward zero.
	b := bucketBaseline(100, 12, 1)
	assert.Equal(t, b, Shrink(b, 0, 0, 5))
}

func TestShrinkIgnoresEmptyBucket(t *testing.T) {
	b := bucketBaseline(0, 0, 0)
	assert.Equal(t, b, Shrink(b, 400, 50, 3))
}

func TestShrinkConvergesToSeriesLevel(t *testing.T) {
	// A large pull leaves the bucket almost no say, which is the "no seasonality"
	// end of the operator setting.
	got := Shrink(bucketBaseline(100, 0, 4), 400, 0, MaxBucketPull)
	assert.InDelta(t, 350, got.Median, 1.0)
}

func TestClampBucketPull(t *testing.T) {
	assert.Equal(t, MinBucketPull, ClampBucketPull(-5))
	assert.Equal(t, MaxBucketPull, ClampBucketPull(999))
	assert.Equal(t, 7, ClampBucketPull(7))
}

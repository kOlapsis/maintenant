// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package devseed

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial/anomaly"
)

// weekStart is a Monday 00:00 UTC, so bucket 0 lines up with the first hour.
var weekStart = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

// observedAt builds rows a week apart, all in the same hour-of-week bucket.
func observedAt(t *testing.T, bucketHour int, cpus []float64) []HourlyRow {
	t.Helper()
	rows := make([]HourlyRow, len(cpus))
	for i, cpu := range cpus {
		ts := weekStart.Add(time.Duration(bucketHour)*time.Hour + time.Duration(i)*7*24*time.Hour)
		rows[i] = HourlyRow{
			Bucket: ts.Unix(), CPUPercent: cpu, MemUsed: cpu * 1e6,
			MemLimit: 1e9, SampleCount: 360,
		}
	}
	return rows
}

func TestParseSpan(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want time.Duration
	}{
		{"28d", 28 * 24 * time.Hour},
		{"672h", 672 * time.Hour},
		{"1d", 24 * time.Hour},
	} {
		got, err := ParseSpan(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, got, tc.in)
	}

	for _, bad := range []string{"", "0d", "-3d", "abc", "0h", "-1h", "d"} {
		_, err := ParseSpan(bad)
		assert.Error(t, err, "expected %q to be rejected", bad)
	}
}

func TestCenterUsesObservedBucketMedian(t *testing.T) {
	const bucketHour = 10
	rows := observedAt(t, bucketHour, []float64{40, 42, 44, 46})
	cp := BuildProfile(rows, time.UTC)

	// A bucket the container was observed in keeps its own median as the centre.
	var vals []float64
	for i := range 12 {
		ts := weekStart.Add(time.Duration(bucketHour)*time.Hour + time.Duration(i+10)*7*24*time.Hour)
		vals = append(vals, cp.Generate("ctr-a", ts.Unix(), time.UTC).CPUPercent)
	}

	observedMedian := anomaly.Median([]float64{40, 42, 44, 46}) // 43
	assert.InDelta(t, observedMedian, anomaly.Median(vals), observedMedian*0.15)
}

// A bucket observed once, far from the container's level, must not become the normal for that hour.
func TestThinBucketDoesNotImposeItsLevel(t *testing.T) {
	const outlierHour = 20
	var rows []HourlyRow
	// A container sitting around 400, sampled four times per bucket everywhere...
	for h := range 24 {
		if h == outlierHour {
			continue
		}
		rows = append(rows, observedAt(t, h, []float64{395, 400, 405, 400})...)
	}
	// ...except one hour seen a single time, and low.
	rows = append(rows, observedAt(t, outlierHour, []float64{110})...)

	cp := BuildProfile(rows, time.UTC)

	var vals []float64
	for i := range 40 {
		ts := weekStart.Add(time.Duration(outlierHour)*time.Hour + time.Duration(i+40)*7*24*time.Hour)
		vals = append(vals, cp.Generate("ctr-thin", ts.Unix(), time.UTC).CPUPercent)
	}
	got := anomaly.Median(vals)

	// One sample keeps a quarter of its say, so the hour lands much closer to the container's level.
	assert.Greater(t, got, 250.0, "a single sample must not drag the hour down to its own level")
	assert.Less(t, got, 400.0, "the observation still counts for something")
}

func TestSpreadTracksObservedDispersion(t *testing.T) {
	const bucketHour = 14
	// MAD of {30,40,50,60,70} about its median 50 is 10.
	rows := observedAt(t, bucketHour, []float64{30, 40, 50, 60, 70})
	cp := BuildProfile(rows, time.UTC)
	observedMAD := anomaly.MAD([]float64{30, 40, 50, 60, 70})
	require.InDelta(t, 10.0, observedMAD, 1e-9)

	var vals []float64
	for i := range 200 {
		ts := weekStart.Add(time.Duration(bucketHour)*time.Hour + time.Duration(i+5)*7*24*time.Hour)
		vals = append(vals, cp.Generate("ctr-b", ts.Unix(), time.UTC).CPUPercent)
	}

	// σ = spread/ZConst makes the generated MAD land on the requested spread.
	assert.InDelta(t, observedMAD, anomaly.MAD(vals), observedMAD*0.25)
}

func TestConstantSeriesGeneratesZeroMAD(t *testing.T) {
	const bucketHour = 3
	rows := observedAt(t, bucketHour, []float64{0, 0, 0, 0})
	cp := BuildProfile(rows, time.UTC)

	var vals []float64
	for i := range 50 {
		ts := weekStart.Add(time.Duration(bucketHour)*time.Hour + time.Duration(i+8)*7*24*time.Hour)
		vals = append(vals, cp.Generate("ctr-idle", ts.Unix(), time.UTC).CPUPercent)
	}

	// A flat container stays flat: no synthetic floor is applied.
	assert.Zero(t, anomaly.MAD(vals))
	assert.Zero(t, anomaly.Median(vals))
}

func TestFallbackShapeIsDiurnal(t *testing.T) {
	// One observed bucket only: every other bucket falls back on the shape.
	rows := observedAt(t, 0, []float64{50, 50, 50})
	cp := BuildProfile(rows, time.UTC)

	// Wednesday of an unobserved week, so both hours use the fallback.
	day := weekStart.Add(20*7*24*time.Hour + 2*24*time.Hour)
	peak := cp.Generate("ctr-c", day.Add(15*time.Hour).Unix(), time.UTC).CPUPercent
	trough := cp.Generate("ctr-c", day.Add(3*time.Hour).Unix(), time.UTC).CPUPercent

	assert.Greater(t, peak, trough, "15:00 should sit above 03:00")
}

func TestWeeklyShapeMeanIsOne(t *testing.T) {
	var sum float64
	for h := range anomaly.BucketsPerWeek {
		sum += shape(weekStart.Add(time.Duration(h) * time.Hour))
	}
	assert.InDelta(t, 1.0, sum/float64(anomaly.BucketsPerWeek), 1e-9)
}

func TestGeneratorIsDeterministic(t *testing.T) {
	rows := observedAt(t, 7, []float64{10, 20, 30, 40})
	first := BuildProfile(rows, time.UTC)
	second := BuildProfile(rows, time.UTC)

	for i := range 24 {
		ts := weekStart.Add(30*7*24*time.Hour + time.Duration(i)*time.Hour)
		assert.Equal(t, first.Generate("ctr-d", ts.Unix(), time.UTC), second.Generate("ctr-d", ts.Unix(), time.UTC))
	}
}

func TestGeneratedValuesAreClamped(t *testing.T) {
	// A median near zero with a wide spread would otherwise go negative.
	rows := observedAt(t, 9, []float64{0, 0, 30, 0, 0})
	cp := BuildProfile(rows, time.UTC)
	cp.MemLimit = 1000

	for i := range 300 {
		ts := weekStart.Add(time.Duration(i) * time.Hour)
		r := cp.Generate("ctr-e", ts.Unix(), time.UTC)
		assert.GreaterOrEqual(t, r.CPUPercent, 0.0)
		assert.GreaterOrEqual(t, r.MemUsed, 0.0)
		assert.LessOrEqual(t, r.MemUsed, cp.MemLimit)
		assert.False(t, math.IsNaN(r.CPUPercent))
	}
}

func TestProfileCarriesLimitAndSampleCount(t *testing.T) {
	rows := observedAt(t, 5, []float64{1, 2, 3})
	cp := BuildProfile(rows, time.UTC)
	assert.Equal(t, 1e9, cp.MemLimit)
	assert.Equal(t, 360, cp.SampleCount)

	// No usable count in the source falls back to one hour at the 10s cadence.
	for i := range rows {
		rows[i].SampleCount = 0
	}
	assert.Equal(t, defaultSampleCount, BuildProfile(rows, time.UTC).SampleCount)
}

func TestMissingBucketsSkipsExisting(t *testing.T) {
	from := weekStart.Unix()
	to := from + 10*3600
	present := map[int64]struct{}{
		from:          {},
		from + 3*3600: {},
		from + 9*3600: {},
	}

	got := MissingBuckets(from, to, present)
	require.Len(t, got, 7)
	assert.Equal(t, []int64{
		from + 1*3600, from + 2*3600, from + 4*3600, from + 5*3600,
		from + 6*3600, from + 7*3600, from + 8*3600,
	}, got)
}

func TestMissingBucketsEmptyWhenFullyCovered(t *testing.T) {
	from := weekStart.Unix()
	to := from + 5*3600
	present := map[int64]struct{}{}
	for b := from; b < to; b += 3600 {
		present[b] = struct{}{}
	}
	assert.Empty(t, MissingBuckets(from, to, present))
}

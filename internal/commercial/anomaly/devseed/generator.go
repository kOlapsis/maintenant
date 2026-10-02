// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package devseed

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/commercial/anomaly"
)

// Shape of the fallback profile, for the hours of the week a container was never observed in.
const (
	diurnalAmplitude = 0.25 // ±25% around the container's own median
	weekendFactor    = 0.80 // -20% on Saturday and Sunday
	diurnalPeakHour  = 15.0 // busiest hour of the day; trough sits 12h away
	noiseClampSigma  = 2.5  // no manufactured outliers for detectors to unlearn
	minBucketObs     = 3    // below this, a per-bucket MAD is too unstable to trust
)

const bucketPull = float64(anomaly.DefaultBucketPull)

// weeklyShapeMean keeps the weekly mean of shape at 1, or synthetic weeks would read as a level shift at the seam.
var weeklyShapeMean = 1 - (2.0/7.0)*(1-weekendFactor)

// shape is a diurnal sine peaking at diurnalPeakHour, damped on weekends.
func shape(t time.Time) float64 {
	h := float64(t.Hour()) + float64(t.Minute())/60
	s := 1 + diurnalAmplitude*math.Sin(2*math.Pi*(h-diurnalPeakHour+6)/24)
	if d := t.Weekday(); d == time.Saturday || d == time.Sunday {
		s *= weekendFactor
	}
	return s / weeklyShapeMean
}

// HourlyRow is one hourly rollup as the seeder reads and writes it.
type HourlyRow struct {
	Bucket      int64
	CPUPercent  float64
	MemUsed     float64
	MemLimit    float64
	NetRxBytes  float64
	NetTxBytes  float64
	SampleCount int
}

const (
	metricCPU   = "cpu"
	metricMem   = "mem"
	metricNetRx = "net_rx"
	metricNetTx = "net_tx"
)

var generatedMetrics = []string{metricCPU, metricMem, metricNetRx, metricNetTx}

type profile struct {
	bucketMedian map[int]float64
	bucketSpread map[int]float64
	bucketCount  map[int]int
	globalMedian float64
	globalSpread float64
}

// center pulls a bucket toward the container's own level by how little it was observed, as anomaly.Shrink does.
func (p profile) center(bucket int, t time.Time) float64 {
	fallback := p.globalMedian * shape(t)
	v, ok := p.bucketMedian[bucket]
	if !ok {
		return fallback
	}
	n := float64(p.bucketCount[bucket])
	w := n / (n + bucketPull)
	return w*v + (1-w)*fallback
}

// spread trusts a per-bucket MAD only with enough observations, else rescales the global MAD to the bucket's level.
func (p profile) spread(bucket int, center float64) float64 {
	if v, ok := p.bucketSpread[bucket]; ok {
		return v
	}
	if p.globalMedian > 0 {
		return p.globalSpread * center / p.globalMedian
	}
	return p.globalSpread
}

// ContainerProfile holds what generation draws from one container's real rollups.
type ContainerProfile struct {
	metrics     map[string]profile
	MemLimit    float64
	SampleCount int
}

// BuildProfile derives the generation inputs from a container's rollups; loc must be the zone the baseline engine buckets in.
func BuildProfile(rows []HourlyRow, loc *time.Location) ContainerProfile {
	if loc == nil {
		loc = time.UTC
	}
	byMetric := map[string]map[int][]float64{}
	global := map[string][]float64{}
	for _, m := range generatedMetrics {
		byMetric[m] = map[int][]float64{}
	}

	var limits, counts []float64
	for _, r := range rows {
		tow := anomaly.TimeOfWeekBucket(time.Unix(r.Bucket, 0), loc)
		for _, m := range generatedMetrics {
			v := r.value(m)
			byMetric[m][tow] = append(byMetric[m][tow], v)
			global[m] = append(global[m], v)
		}
		if r.MemLimit > 0 {
			limits = append(limits, r.MemLimit)
		}
		if r.SampleCount > 0 {
			counts = append(counts, float64(r.SampleCount))
		}
	}

	cp := ContainerProfile{
		metrics:     make(map[string]profile, len(generatedMetrics)),
		MemLimit:    anomaly.Median(limits),
		SampleCount: int(anomaly.Median(counts)),
	}
	if cp.SampleCount <= 0 {
		cp.SampleCount = defaultSampleCount
	}

	for _, m := range generatedMetrics {
		p := profile{
			bucketMedian: map[int]float64{},
			bucketSpread: map[int]float64{},
			bucketCount:  map[int]int{},
			globalMedian: anomaly.Median(global[m]),
			globalSpread: anomaly.MAD(global[m]),
		}
		for tow, vals := range byMetric[m] {
			p.bucketMedian[tow] = anomaly.Median(vals)
			p.bucketCount[tow] = len(vals)
			if len(vals) >= minBucketObs {
				p.bucketSpread[tow] = anomaly.MAD(vals)
			}
		}
		cp.metrics[m] = p
	}
	return cp
}

// defaultSampleCount is one hour at the collector's 10s cadence.
const defaultSampleCount = 360

// Generate builds the row of one missing hour, the same inputs always yielding the same row.
func (cp ContainerProfile) Generate(containerID string, bucket int64, loc *time.Location) HourlyRow {
	if loc == nil {
		loc = time.UTC
	}
	t := time.Unix(bucket, 0).In(loc)
	tow := anomaly.TimeOfWeekBucket(t, loc)

	row := HourlyRow{Bucket: bucket, MemLimit: cp.MemLimit, SampleCount: cp.SampleCount}
	for _, m := range generatedMetrics {
		p := cp.metrics[m]
		center := p.center(tow, t)
		v := draw(streamFor(containerID, m, bucket), center, p.spread(tow, center))
		if m == metricMem && cp.MemLimit > 0 && v > cp.MemLimit {
			v = cp.MemLimit
		}
		row.set(m, v)
	}
	return row
}

// draw samples a value whose MAD matches spread: Gaussian noise of deviation σ has a MAD of ZConst·σ.
func draw(rng *rand.Rand, center, spread float64) float64 {
	if spread <= 0 {
		return math.Max(0, center)
	}
	n := rng.NormFloat64()
	if n > noiseClampSigma {
		n = noiseClampSigma
	} else if n < -noiseClampSigma {
		n = -noiseClampSigma
	}
	return math.Max(0, center+(spread/anomaly.ZConst)*n)
}

// streamFor keys the random stream on the hour so generation does not depend on order.
func streamFor(containerID, metric string, bucket int64) *rand.Rand {
	h := fnv.New64a()
	_, _ = h.Write([]byte(containerID))
	_, _ = h.Write([]byte{'|'})
	_, _ = h.Write([]byte(metric))
	seed := h.Sum64()
	// #nosec G404,G115 -- reproducible seeding, not unpredictability; the epoch is reinterpreted as seed bits.
	return rand.New(rand.NewPCG(seed, uint64(bucket)))
}

func (r HourlyRow) value(metric string) float64 {
	switch metric {
	case metricCPU:
		return r.CPUPercent
	case metricMem:
		return r.MemUsed
	case metricNetRx:
		return r.NetRxBytes
	case metricNetTx:
		return r.NetTxBytes
	}
	return 0
}

func (r *HourlyRow) set(metric string, v float64) {
	switch metric {
	case metricCPU:
		r.CPUPercent = v
	case metricMem:
		r.MemUsed = v
	case metricNetRx:
		r.NetRxBytes = v
	case metricNetTx:
		r.NetTxBytes = v
	}
}

// MissingBuckets returns the hour starts in [from, to) present does not cover, oldest first.
func MissingBuckets(from, to int64, present map[int64]struct{}) []int64 {
	var out []int64
	for b := from; b < to; b += 3600 {
		if _, ok := present[b]; !ok {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ParseSpan reads a positive seeding window, a day count such as "28d" or a Go duration.
func ParseSpan(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty span")
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid day span %q: %w", s, err)
		}
		if n <= 0 {
			return 0, fmt.Errorf("span must be positive, got %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid span %q: %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("span must be positive, got %q", s)
	}
	return d, nil
}

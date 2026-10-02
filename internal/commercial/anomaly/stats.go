// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"math"
	"sort"
)

// ZConst is the Iglewicz-Hoaglin constant that scales a MAD deviation to standard-deviation units.
const ZConst = 0.6745

// Median returns the median of xs without mutating it, 0 for an empty slice.
func Median(xs []float64) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}
	cp := make([]float64, n)
	copy(cp, xs)
	sort.Float64s(cp)
	mid := n / 2
	if n%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

// MAD returns the median absolute deviation of xs about its own median.
func MAD(xs []float64) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}
	med := Median(xs)
	devs := make([]float64, n)
	for i, x := range xs {
		devs[i] = math.Abs(x - med)
	}
	return Median(devs)
}

// EffectiveMAD returns max(mad, relFloor·|median|, absFloor), never zero.
func EffectiveMAD(mad, median, relFloor, absFloor float64) float64 {
	eff := mad
	if rel := relFloor * math.Abs(median); rel > eff {
		eff = rel
	}
	if absFloor > eff {
		eff = absFloor
	}
	if eff <= 0 {
		return math.SmallestNonzeroFloat64
	}
	return eff
}

// ModifiedZ returns the modified z-score of x against a median and a positive effective MAD.
func ModifiedZ(x, median, madEff float64) float64 {
	if madEff <= 0 {
		return 0
	}
	return ZConst * (x - median) / madEff
}

// LinregResult is an ordinary least-squares fit and its coefficient of determination.
type LinregResult struct {
	Slope     float64
	Intercept float64
	R2        float64
}

// Linreg fits an ordinary least-squares line through the paired points.
func Linreg(xs, ys []float64) LinregResult {
	n := len(xs)
	if n < 2 || n != len(ys) {
		return LinregResult{}
	}
	var sumX, sumY float64
	for i := range n {
		sumX += xs[i]
		sumY += ys[i]
	}
	fn := float64(n)
	meanX := sumX / fn
	meanY := sumY / fn

	var sxx, sxy, syy float64
	for i := range n {
		dx := xs[i] - meanX
		dy := ys[i] - meanY
		sxx += dx * dx
		sxy += dx * dy
		syy += dy * dy
	}
	if sxx == 0 {
		return LinregResult{Intercept: meanY}
	}
	slope := sxy / sxx
	res := LinregResult{Slope: slope, Intercept: meanY - slope*meanX}
	if syy > 0 {
		res.R2 = (sxy * sxy) / (sxx * syy)
	}
	return res
}

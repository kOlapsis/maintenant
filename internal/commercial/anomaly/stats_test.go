// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMedian(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want float64
	}{
		{"empty", nil, 0},
		{"single", []float64{42}, 42},
		{"odd", []float64{3, 1, 2}, 2},
		{"even", []float64{4, 1, 3, 2}, 2.5},
		{"unsorted negatives", []float64{-5, 0, 5, 10}, 2.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, Median(tt.in), 1e-9)
		})
	}
}

func TestMedianDoesNotMutateInput(t *testing.T) {
	in := []float64{3, 1, 2}
	_ = Median(in)
	assert.Equal(t, []float64{3, 1, 2}, in, "Median must not reorder its input")
}

func TestMAD(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want float64
	}{
		{"empty", nil, 0},
		{"constant series", []float64{5, 5, 5, 5}, 0},
		{"simple", []float64{1, 2, 3, 4, 5}, 1}, // median 3, devs {2,1,0,1,2} -> 1
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, MAD(tt.in), 1e-9)
		})
	}
}

func TestEffectiveMAD(t *testing.T) {
	// Raw MAD dominates.
	assert.InDelta(t, 3.0, EffectiveMAD(3, 100, 0.0, 0.0), 1e-9)
	// Relative floor dominates: 0.05 * 100 = 5 > 3.
	assert.InDelta(t, 5.0, EffectiveMAD(3, 100, 0.05, 0.0), 1e-9)
	// Absolute floor dominates.
	assert.InDelta(t, 10.0, EffectiveMAD(3, 100, 0.05, 10), 1e-9)
	// Constant series (mad 0) with all-zero floors never yields zero.
	assert.Greater(t, EffectiveMAD(0, 0, 0, 0), 0.0)
}

func TestModifiedZ(t *testing.T) {
	// x exactly at median -> 0.
	assert.InDelta(t, 0.0, ModifiedZ(5, 5, 1), 1e-9)
	// Known value: 0.6745 * (7-5)/1.
	assert.InDelta(t, ZConst*2, ModifiedZ(7, 5, 1), 1e-9)
	// Division-safe when madEff is non-positive.
	assert.InDelta(t, 0.0, ModifiedZ(7, 5, 0), 1e-9)
	assert.False(t, math.IsInf(ModifiedZ(7, 5, 0), 0))
}

func TestLinreg(t *testing.T) {
	// Perfect positive line y = 2x + 1.
	xs := []float64{0, 1, 2, 3, 4}
	ys := []float64{1, 3, 5, 7, 9}
	res := Linreg(xs, ys)
	assert.InDelta(t, 2.0, res.Slope, 1e-9)
	assert.InDelta(t, 1.0, res.Intercept, 1e-9)
	assert.InDelta(t, 1.0, res.R2, 1e-9)

	// Flat line -> zero slope, R² 0.
	flat := Linreg([]float64{0, 1, 2, 3}, []float64{5, 5, 5, 5})
	assert.InDelta(t, 0.0, flat.Slope, 1e-9)
	assert.InDelta(t, 0.0, flat.R2, 1e-9)
}

func TestLinregDegenerate(t *testing.T) {
	// Fewer than two points.
	assert.Equal(t, LinregResult{}, Linreg([]float64{1}, []float64{2}))
	// Mismatched lengths.
	assert.Equal(t, LinregResult{}, Linreg([]float64{1, 2}, []float64{2}))
	// Identical x values -> no division by zero, zero slope.
	deg := Linreg([]float64{3, 3, 3}, []float64{1, 2, 3})
	assert.InDelta(t, 0.0, deg.Slope, 1e-9)
	assert.False(t, math.IsNaN(deg.Slope))
}

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeOfWeekBucketBoundaries(t *testing.T) {
	utc := time.UTC
	// 2026-06-22 is a Monday.
	monMidnight := time.Date(2026, 6, 22, 0, 0, 0, 0, utc)
	assert.Equal(t, 0, TimeOfWeekBucket(monMidnight, utc), "Monday 00:00 -> 0")

	monMidnightEnd := time.Date(2026, 6, 22, 0, 59, 59, 0, utc)
	assert.Equal(t, 0, TimeOfWeekBucket(monMidnightEnd, utc), "Monday 00:59 still -> 0")

	// 2026-06-28 is a Sunday.
	sun23 := time.Date(2026, 6, 28, 23, 0, 0, 0, utc)
	assert.Equal(t, 167, TimeOfWeekBucket(sun23, utc), "Sunday 23:00 -> 167")

	// Tuesday 13:00 -> day index 1 * 24 + 13 = 37.
	tue13 := time.Date(2026, 6, 23, 13, 0, 0, 0, utc)
	assert.Equal(t, 37, TimeOfWeekBucket(tue13, utc))
}

func TestTimeOfWeekBucketNonUTCZone(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	// 2026-06-22 04:00 UTC is Monday 00:00 in New York (UTC-4 in summer).
	utcInstant := time.Date(2026, 6, 22, 4, 0, 0, 0, time.UTC)
	assert.Equal(t, 0, TimeOfWeekBucket(utcInstant, loc), "should bucket in local zone, not UTC")

	// Same instant in UTC is Monday 04:00 -> bucket 4.
	assert.Equal(t, 4, TimeOfWeekBucket(utcInstant, time.UTC))
}

func TestTimeOfWeekBucketDSTTransition(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	// US spring-forward 2026: clocks jump 02:00 -> 03:00 on Sunday 2026-03-08.
	// An instant at local 03:30 must bucket as Sunday hour 3 (6*24+3 = 147).
	afterSpring := time.Date(2026, 3, 8, 3, 30, 0, 0, loc)
	assert.Equal(t, 6*24+3, TimeOfWeekBucket(afterSpring, loc))
}

func TestTimeOfWeekBucketNilLocation(t *testing.T) {
	monMidnight := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, 0, TimeOfWeekBucket(monMidnight, nil), "nil location defaults to UTC")
}

func TestTimeOfWeekBucketAllInRange(t *testing.T) {
	start := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	for h := range 24 * 8 {
		b := TimeOfWeekBucket(start.Add(time.Duration(h)*time.Hour), time.UTC)
		assert.GreaterOrEqual(t, b, 0)
		assert.Less(t, b, BucketsPerWeek)
	}
}

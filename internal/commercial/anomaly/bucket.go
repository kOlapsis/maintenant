// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import "time"

// BucketsPerWeek is the number of hour-of-week buckets.
const BucketsPerWeek = 168

// TimeOfWeekBucket maps an instant to its hour of the week in loc, Monday 00:00 being bucket 0.
func TimeOfWeekBucket(t time.Time, loc *time.Location) int {
	if loc == nil {
		loc = time.UTC
	}
	lt := t.In(loc)
	weekday := (int(lt.Weekday()) + 6) % 7
	return weekday*24 + lt.Hour()
}

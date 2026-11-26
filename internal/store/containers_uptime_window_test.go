// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
)

func TestUptimeCalculator_AgreesWithDailyBar(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	today := startOfUTCDay(now)
	yesterday := today.AddDate(0, 0, -1)
	const day = 24 * time.Hour
	type step struct {
		prev, next string
		age        time.Duration
	}

	cases := []struct {
		name        string
		transitions []step
		want24h     float64
		want7d      float64
		barDay      time.Time
		minBar      float64
		maxBar      float64
	}{
		{
			name: "up for 20 days, restarted 45 minutes ago",
			transitions: []step{
				{"created", "running", 20 * day},
				{"running", "exited", 45 * time.Minute},
				{"exited", "running", 44 * time.Minute},
			},
			want24h: 99.93, want7d: 99.99,
			barDay: yesterday, minBar: 99.9, maxBar: 100,
		},
		{
			name:        "discovered an hour ago",
			transitions: []step{{"created", "running", time.Hour}},
			want24h:     100, want7d: 100,
			barDay: today, minBar: 100, maxBar: 100,
		},
		{
			name: "stopped three days ago",
			transitions: []step{
				{"created", "running", 10 * day},
				{"running", "exited", 3 * day},
			},
			want24h: 0, want7d: 57.14,
			barDay: yesterday, minBar: 0, maxBar: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			cs := NewContainerStore(db)
			cid := seedHostContainer(t, cs, "ext-uptime", "")
			for _, st := range tc.transitions {
				addTransition(t, db, cid, st.prev, st.next, now.Add(-st.age))
			}

			got, err := container.NewUptimeCalculator(cs).Calculate(ctx, cid, 90*day)
			require.NoError(t, err)
			require.NotNil(t, got.Hours24)
			require.NotNil(t, got.Days7)
			assert.InDelta(t, tc.want24h, *got.Hours24, 0.011, "24h")
			assert.InDelta(t, tc.want7d, *got.Days7, 0.011, "7d")

			bar, err := NewUptimeDailyStore(db).GetContainerDailyUptime(ctx, cid, 90, now)
			require.NoError(t, err)
			du := dayOf(t, bar, tc.barDay)
			require.NotNil(t, du.UptimePercent)
			assert.GreaterOrEqual(t, *du.UptimePercent, tc.minBar)
			assert.LessOrEqual(t, *du.UptimePercent, tc.maxBar)
		})
	}
}

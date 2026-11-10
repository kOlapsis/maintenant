// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package extension

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Acceptance tests of 051-community-label-limits: the slot allocation every
// capped resource shares (FR-003, FR-004, FR-008, FR-009, FR-012, FR-013, FR-020).

var allocBase = time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)

func allocCands(prefix string, n int, grandfathered bool, from time.Time) []LimitCandidate {
	out := make([]LimitCandidate, n)
	for i := range out {
		name := fmt.Sprintf("%s-%02d", prefix, i+1)
		out[i] = LimitCandidate{
			ID:            "id-" + name,
			Grandfathered: grandfathered,
			Since:         from.Add(time.Duration(i) * time.Minute),
			Key:           name + "/maintenant.endpoint.http",
		}
	}
	return out
}

func allocLimited(m map[string]bool) []string {
	out := []string{}
	for id, limited := range m {
		if limited {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func allocIDs(cands []LimitCandidate) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.ID)
	}
	sort.Strings(out)
	return out
}

func allocShuffled(cands []LimitCandidate, seed int64) []LimitCandidate {
	out := append([]LimitCandidate(nil), cands...)
	rand.New(rand.NewSource(seed)).Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// FR-001, FR-008, US2 scenarios 1, 3 and 5: the first discovered get the slots, the rest wait.
func TestAllocate_FirstDiscoveredTakeTheSlots(t *testing.T) {
	for name, tc := range map[string]struct {
		cands, limit, monitored int
	}{
		"eleventh endpoint":       {11, 10, 10},
		"thirty Traefik routers":  {30, 10, 10},
		"sixth label certificate": {6, 5, 5},
		"cap of three":            {8, 3, 3},
	} {
		t.Run(name, func(t *testing.T) {
			cands := allocCands("svc", tc.cands, false, allocBase)
			got := allocLimited(Allocate(allocShuffled(cands, 7), tc.limit))
			assert.Equal(t, allocIDs(cands[tc.monitored:]), got)
		})
	}
}

// FR-008: at the same discovery time, the order follows the container name then the label key.
func TestAllocate_SimultaneousDiscoveryFollowsTheKey(t *testing.T) {
	keys := []string{"web/maintenant.endpoint.http", "api/maintenant.endpoint.1.http", "api/maintenant.endpoint.0.http", "db/maintenant.endpoint.tcp"}
	cands := make([]LimitCandidate, len(keys))
	for i, k := range keys {
		cands[i] = LimitCandidate{ID: "id-" + k, Since: allocBase, Key: k}
	}

	assert.Equal(t, []string{"id-web/maintenant.endpoint.http"}, allocLimited(Allocate(cands, 3)))
	assert.Equal(t, []string{"id-db/maintenant.endpoint.tcp", "id-web/maintenant.endpoint.http"},
		allocLimited(Allocate(cands, 2)))
}

// FR-008, edge case "redémarrer l'instance ne change pas lesquels sont surveillés":
// the order the candidates are read in never changes the outcome.
func TestAllocate_IsStableWhateverTheInputOrder(t *testing.T) {
	cands := append(allocCands("old", 4, true, allocBase.Add(time.Hour)), allocCands("new", 26, false, allocBase)...)
	want := allocLimited(Allocate(cands, 10))
	assert.Len(t, want, 20)
	for seed := int64(1); seed <= 25; seed++ {
		assert.Equal(t, want, allocLimited(Allocate(allocShuffled(cands, seed), 10)), "seed %d", seed)
	}
}

// FR-003, US1 scenarios 1 and 5, SC-001: what existed before the upgrade stays monitored past the cap.
func TestAllocate_GrandfatheredStayMonitoredPastTheCap(t *testing.T) {
	for name, tc := range map[string]struct{ n, limit int }{
		"14 endpoints":    {14, 10},
		"7 certificates":  {7, 5},
		"40 endpoints":    {40, 10},
		"exactly the cap": {10, 10},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, allocLimited(Allocate(allocCands("gf", tc.n, true, allocBase), tc.limit)))
		})
	}
}

// FR-004, US1 scenarios 4 and 6: grandfathered elements count, nothing new gets in while the total reaches the cap.
func TestAllocate_GrandfatheredCountInTheTotal(t *testing.T) {
	for name, tc := range map[string]struct {
		grandfathered, fresh, limit, freshMonitored int
	}{
		"14 kept, one new":            {14, 1, 10, 0},
		"11 left after 3 removed":     {11, 2, 10, 0},
		"exactly at the cap":          {10, 3, 10, 0},
		"one slot left under the cap": {9, 3, 10, 1},
		"certificates, 6 kept":        {6, 2, 5, 0},
	} {
		t.Run(name, func(t *testing.T) {
			old := allocCands("gf", tc.grandfathered, true, allocBase)
			fresh := allocCands("new", tc.fresh, false, allocBase.Add(24*time.Hour))
			got := allocLimited(Allocate(append(old, fresh...), tc.limit))
			assert.Equal(t, allocIDs(fresh[tc.freshMonitored:]), got)
		})
	}
}

// FR-012: past the cap, the grandfathered keep their slot first, then the first discovered fill the rest.
func TestAllocate_GrandfatheredGoBeforeOlderNewcomers(t *testing.T) {
	old := allocCands("gf", 3, true, allocBase.Add(48*time.Hour))
	fresh := allocCands("new", 12, false, allocBase)

	got := allocLimited(Allocate(append(fresh, old...), 10))

	assert.Equal(t, allocIDs(fresh[7:]), got)
}

// FR-009, US2 scenario 10: a freed slot goes to the oldest waiting candidate.
func TestAllocate_FreedSlotGoesToTheOldestWaiting(t *testing.T) {
	monitored := allocCands("svc", 10, false, allocBase)
	waiting := allocCands("wait", 3, false, allocBase.Add(time.Hour))

	before := allocLimited(Allocate(append(monitored, waiting...), 10))
	assert.Equal(t, allocIDs(waiting), before)

	after := allocLimited(Allocate(append(monitored[1:], waiting...), 10))
	assert.Equal(t, allocIDs(waiting[1:]), after)
}

// FR-013, SC-002: under the cap nobody is left out.
func TestAllocate_UnderTheCapNobodyIsLeftOut(t *testing.T) {
	assert.Empty(t, allocLimited(Allocate(nil, 10)))
	assert.Empty(t, allocLimited(Allocate(allocCands("svc", 6, false, allocBase), 10)))
	assert.Empty(t, allocLimited(Allocate(allocCands("svc", 10, false, allocBase), 10)))
	assert.Empty(t, allocLimited(Allocate(allocCands("cert", 5, false, allocBase), 5)))
}

// FR-011, FR-020: an unlimited edition leaves nobody out, whatever the origin and the count.
func TestAllocate_UnlimitedLeavesNobodyOut(t *testing.T) {
	cands := append(allocCands("gf", 5, true, allocBase), allocCands("new", 60, false, allocBase)...)
	assert.Empty(t, allocLimited(Allocate(cands, Unlimited)))
}

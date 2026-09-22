// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package eol

import (
	"testing"
	"time"
)

func day(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation(time.DateOnly, s, time.UTC)
	if err != nil {
		t.Fatalf("parse day %q: %v", s, err)
	}
	return parsed
}

func TestEvaluate(t *testing.T) {
	table, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}

	cases := []struct {
		name     string
		identity Identity
		today    string
		state    SupportState
		days     *int
	}{
		{name: "debian 11 before the warning", identity: Identity{ID: "debian", VersionID: "11"}, today: "2026-07-31", state: StateSecurityOnly, days: ptr(31)},
		{name: "debian 11 at the warning", identity: Identity{ID: "debian", VersionID: "11"}, today: "2026-08-01", state: StateEndingSoon, days: ptr(30)},
		{name: "debian 11 on the last day", identity: Identity{ID: "debian", VersionID: "11"}, today: "2026-08-31", state: StateEndingSoon, days: ptr(0)},
		{name: "debian 11 the day after", identity: Identity{ID: "debian", VersionID: "11"}, today: "2026-09-01", state: StateEnded, days: ptr(-1)},
		{name: "debian 12", identity: Identity{ID: "debian", VersionID: "12"}, today: "2026-09-18", state: StateSecurityOnly},
		{name: "alpine without active support date", identity: Identity{ID: "alpine", VersionID: "3.24.0"}, today: "2026-09-18", state: StateSupported},
		{name: "empty identity", identity: Identity{}, today: "2026-09-18", state: StateUnknown},
		{name: "mount missing", identity: Identity{Source: "host_file", UnavailableReason: "mount_missing"}, today: "2026-09-18", state: StateUnknown},
		{name: "untracked distribution", identity: Identity{ID: "fedora", VersionID: "42"}, today: "2026-09-18", state: StateUntracked},
		{name: "unknown kubernetes node image", identity: Identity{PrettyName: "Talos (v1.7.0)", Source: "kubernetes_node"}, today: "2026-09-18", state: StateUntracked},
		{name: "tracked distribution, unknown cycle", identity: Identity{ID: "debian", VersionID: "99"}, today: "2026-09-18", state: StateUntracked},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.identity, table, day(t, tc.today))
			if got.State != tc.state {
				t.Fatalf("state = %q, want %q", got.State, tc.state)
			}
			if got.TableSource != table.Source {
				t.Errorf("table source = %q, want %q", got.TableSource, table.Source)
			}
			if tc.days != nil {
				if got.DaysRemaining == nil {
					t.Fatalf("days remaining = nil, want %d", *tc.days)
				}
				if *got.DaysRemaining != *tc.days {
					t.Errorf("days remaining = %d, want %d", *got.DaysRemaining, *tc.days)
				}
			}
			switch tc.state {
			case StateUnknown, StateUntracked:
				if got.DaysRemaining != nil || got.Product != "" || got.Cycle != "" || got.SecurityUntil != nil {
					t.Errorf("%s carries cycle detail: %+v", tc.state, got)
				}
			default:
				if got.SecurityUntil == nil || got.DaysRemaining == nil {
					t.Errorf("%s misses cycle detail: %+v", tc.state, got)
				}
			}
		})
	}
}

func TestEvaluateFillsCycleDates(t *testing.T) {
	table, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}

	got := Evaluate(Identity{ID: "debian", VersionID: "11", Source: "host_file"}, table, day(t, "2026-08-01"))
	if got.Product != "debian" || got.Cycle != "11" {
		t.Fatalf("product/cycle = %q/%q, want debian/11", got.Product, got.Cycle)
	}
	if got.SecurityUntil.String() != "2026-08-31" {
		t.Errorf("security until = %s, want 2026-08-31", got.SecurityUntil)
	}
	if got.ActiveUntil == nil || got.ActiveUntil.String() != "2024-08-14" {
		t.Errorf("active until = %v, want 2024-08-14", got.ActiveUntil)
	}
	if got.ExtendedUntil == nil || got.ExtendedUntil.String() != "2031-06-30" {
		t.Errorf("extended until = %v, want 2031-06-30", got.ExtendedUntil)
	}
}

func ptr(v int) *int { return &v }

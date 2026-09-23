// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package extension

import (
	"context"
	"testing"
)

func TestCurrentEditionReturnsCommunity(t *testing.T) {
	if got := CurrentEdition(); got != Community {
		t.Fatalf("expected Community, got %s", got)
	}
}

func TestErrNotAvailable(t *testing.T) {
	if ErrNotAvailable == nil {
		t.Fatal("ErrNotAvailable should not be nil")
	}
}

// TestEditionOrder pins Community < Personal < Pro. Everything else in the
// authorization model is expressed in terms of this order.
func TestEditionOrder(t *testing.T) {
	if Community.rank() >= Personal.rank() || Personal.rank() >= Pro.rank() {
		t.Fatalf("editions are out of order: community=%d personal=%d pro=%d",
			Community.rank(), Personal.rank(), Pro.rank())
	}
}

func TestAtLeast(t *testing.T) {
	cases := []struct {
		edition, required Edition
		want              bool
	}{
		{Community, Community, true},
		{Community, Personal, false},
		{Community, Pro, false},
		{Personal, Community, true},
		{Personal, Personal, true},
		{Personal, Pro, false},
		{Pro, Community, true},
		{Pro, Personal, true},
		{Pro, Pro, true},

		// An unrecognised edition grants nothing, and nothing satisfies it.
		{"enterprise", Community, false},
		{"enterprise", Pro, false},
		{Pro, "enterprise", false},
		{"", Community, false},
	}

	for _, c := range cases {
		if got := c.edition.AtLeast(c.required); got != c.want {
			t.Errorf("Edition(%q).AtLeast(%q) = %v, want %v", c.edition, c.required, got, c.want)
		}
	}
}

func TestParseEdition(t *testing.T) {
	for _, want := range []Edition{Community, Personal, Pro} {
		got, ok := ParseEdition(string(want))
		if !ok || got != want {
			t.Errorf("ParseEdition(%q) = (%q, %v), want (%q, true)", want, got, ok, want)
		}
	}

	// An unknown value falls back to the most restrictive edition and says so,
	// leaving the caller to log the discrepancy (FR-010).
	for _, s := range []string{"enterprise", "PRO", "", "personal "} {
		got, ok := ParseEdition(s)
		if ok {
			t.Errorf("ParseEdition(%q) reported the value as known", s)
		}
		if got != Community {
			t.Errorf("ParseEdition(%q) = %q, want %q", s, got, Community)
		}
	}
}

func TestNoopMaintenanceSuppressor(t *testing.T) {
	suppressed, err := NoopMaintenanceSuppressor{}.IsSuppressed(context.Background(), "update", "container", "c-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suppressed {
		t.Fatal("expected not suppressed")
	}
}

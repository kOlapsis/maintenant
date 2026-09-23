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
	"testing"
	"time"
)

func withEdition(t *testing.T, e Edition) {
	t.Helper()
	prev := CurrentEdition
	CurrentEdition = func() Edition { return e }
	t.Cleanup(func() { CurrentEdition = prev })
}

func withPolicy(t *testing.T, p Policy) {
	t.Helper()
	prev := policy
	policy = p
	t.Cleanup(func() { policy = prev })
}

type fakePolicy struct {
	min     map[Capability]Edition
	limits  map[Edition]map[Resource]int
	history map[Edition]time.Duration
}

func (f fakePolicy) Capabilities() []Capability {
	out := make([]Capability, 0, len(f.min))
	for c := range f.min {
		out = append(out, c)
	}
	return out
}

func (f fakePolicy) MinEdition(c Capability) Edition {
	if e, ok := f.min[c]; ok {
		return e
	}
	return Pro
}

func (f fakePolicy) Limit(e Edition, r Resource) int { return f.limits[e][r] }

func (f fakePolicy) HistoryCap(e Edition) time.Duration { return f.history[e] }

func TestDefaultPolicy_OpensOnlyCommunityCapabilities(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 4 {
		t.Fatalf("default catalog holds %d capabilities, expected 4", len(catalog))
	}
	for _, c := range []Capability{CapAlertRouting, CapSwarmDashboard, CapK8sCluster, CapResourceHistory} {
		if got := catalog[c]; got != Community {
			t.Errorf("default catalog[%q] = %q, want %q", c, got, Community)
		}
		if !Allows(c) {
			t.Errorf("Allows(%q) under the default policy = false, want true", c)
		}
	}
	for _, c := range []Capability{CapMultihost, CapTelegram, CapSlack, CapAlertEscalation, "no_such_capability"} {
		if Allows(c) {
			t.Errorf("Allows(%q) under the default policy = true, want false", c)
		}
	}
}

func TestAllows_DelegatesToThePolicy(t *testing.T) {
	withPolicy(t, fakePolicy{min: map[Capability]Edition{CapSlack: Personal, CapMultihost: Community}})

	cases := []struct {
		edition Edition
		cap     Capability
		want    bool
	}{
		{Community, CapMultihost, true},
		{Community, CapSlack, false},
		{Personal, CapSlack, true},
		{Personal, "no_such_capability", false},
		{Pro, "no_such_capability", true},
	}
	for _, c := range cases {
		withEdition(t, c.edition)
		if got := Allows(c.cap); got != c.want {
			t.Errorf("Allows(%q) under %q = %v, want %v", c.cap, c.edition, got, c.want)
		}
	}

	catalog := Catalog()
	if len(catalog) != 2 || catalog[CapSlack] != Personal || catalog[CapMultihost] != Community {
		t.Errorf("Catalog() = %v, want the policy table", catalog)
	}
}

func TestCatalog_ReturnsACopy(t *testing.T) {
	c := Catalog()
	c[CapAlertRouting] = Pro
	c["invented"] = Community

	if got := MinEdition(CapAlertRouting); got != Community {
		t.Errorf("mutating the Catalog copy changed the policy: alert_routing = %q, want %q", got, Community)
	}
	if len(Catalog()) != 4 {
		t.Errorf("mutating the Catalog copy changed the catalog size: %d", len(Catalog()))
	}
}

func TestChannelCapability(t *testing.T) {
	for channel, want := range map[string]Capability{
		"slack": CapSlack, "teams": CapTeams, "email": CapSMTP, "telegram": CapTelegram,
	} {
		got, ok := ChannelCapability(channel)
		if !ok || got != want {
			t.Errorf("ChannelCapability(%q) = (%q, %v), want (%q, true)", channel, got, ok, want)
		}
	}
	if _, ok := ChannelCapability("webhook"); ok {
		t.Error("webhook must not be gated")
	}
}

func TestNewEditionSource_NoneRegistered(t *testing.T) {
	prev := newSource
	newSource = nil
	t.Cleanup(func() { newSource = prev })

	src, err := NewEditionSource(SourceConfig{LicenseKey: "key"})
	if err != nil || src != nil {
		t.Errorf("NewEditionSource without a registered factory = (%v, %v), want (nil, nil)", src, err)
	}
}

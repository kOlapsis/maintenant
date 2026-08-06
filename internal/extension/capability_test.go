package extension

import "testing"

// withEdition swaps the global edition for the duration of a test. Tests using
// it cannot run in parallel with each other.
func withEdition(t *testing.T, e Edition) {
	t.Helper()
	prev := CurrentEdition
	CurrentEdition = func() Edition { return e }
	t.Cleanup(func() { CurrentEdition = prev })
}

// TestAllows_EveryEditionEveryCapability walks the whole matrix — 3 editions ×
// 20 capabilities — and asserts Allows agrees with the declared order. This is
// SC-003 on the extension side.
func TestAllows_EveryEditionEveryCapability(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 20 {
		t.Fatalf("catalog holds %d capabilities, expected 20", len(catalog))
	}

	for _, edition := range []Edition{Community, Personal, Pro} {
		for cap, min := range catalog {
			t.Run(string(edition)+"/"+string(cap), func(t *testing.T) {
				withEdition(t, edition)
				want := edition.AtLeast(min)
				if got := Allows(cap); got != want {
					t.Errorf("Allows(%q) under %q = %v, want %v (min edition %q)",
						cap, edition, got, want, min)
				}
			})
		}
	}
}

// TestCatalog_ReturnsACopy: the registry must not be mutable through Catalog.
func TestCatalog_ReturnsACopy(t *testing.T) {
	c := Catalog()
	c[CapAlertEscalation] = Community
	c["invented"] = Community

	if got := MinEdition(CapAlertEscalation); got != Pro {
		t.Errorf("mutating the Catalog copy changed the registry: alert_escalation = %q, want %q", got, Pro)
	}
	if len(Catalog()) != 20 {
		t.Errorf("mutating the Catalog copy changed the registry size: %d", len(Catalog()))
	}
}

// TestMinEdition_TierMembership pins the three tiers, so a capability cannot
// silently change price.
func TestMinEdition_TierMembership(t *testing.T) {
	tiers := map[Edition][]Capability{
		Community: {CapAlertRouting, CapSwarmDashboard, CapK8sCluster},
		Personal: {
			CapMultihost, CapCVEEnrichment, CapRiskScoring, CapChangelog,
			CapIncidents, CapSMTP, CapResourceHistory, CapAlertAdvancedFilters,
			CapSecurityPosture, CapOCSPStapling,
		},
		Pro: {
			CapSlack, CapTeams, CapAlertEscalation, CapAlertEntityRouting,
			CapMaintenanceWindows, CapSubscribers, CapPersonalization,
		},
	}

	total := 0
	for want, caps := range tiers {
		total += len(caps)
		for _, c := range caps {
			if got := MinEdition(c); got != want {
				t.Errorf("MinEdition(%q) = %q, want %q", c, got, want)
			}
		}
	}
	if total != 20 {
		t.Errorf("the three tiers list %d capabilities, expected 20", total)
	}
}

// TestMinEdition_UnknownCapability: an undeclared capability grants nothing.
func TestMinEdition_UnknownCapability(t *testing.T) {
	if got := MinEdition("no_such_capability"); got != Pro {
		t.Errorf("MinEdition of an unknown capability = %q, want %q", got, Pro)
	}

	withEdition(t, Personal)
	if Allows("no_such_capability") {
		t.Error("an unknown capability must not be allowed on Personal")
	}
	withEdition(t, Pro)
	if !Allows("no_such_capability") {
		t.Error("an unknown capability resolves to Pro, so Pro must allow it")
	}
}

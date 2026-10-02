// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package tiers

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

func TestMain(m *testing.M) {
	extension.Register(Policy{}, nil)
	os.Exit(m.Run())
}

func withEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = prev })
}

var editions = []extension.Edition{extension.Community, extension.Personal, extension.Pro}

func TestAllows_EveryEditionEveryCapability(t *testing.T) {
	catalog := extension.Catalog()
	require.Len(t, catalog, 21)

	for _, edition := range editions {
		for c, min := range catalog {
			t.Run(string(edition)+"/"+string(c), func(t *testing.T) {
				withEdition(t, edition)
				assert.Equal(t, edition.AtLeast(min), extension.Allows(c), "min edition %q", min)
			})
		}
	}
}

func TestMinEdition_TierMembership(t *testing.T) {
	tiers := map[extension.Edition][]extension.Capability{
		extension.Community: {
			extension.CapAlertRouting, extension.CapSwarmDashboard,
			extension.CapK8sCluster, extension.CapResourceHistory,
		},
		extension.Personal: {
			extension.CapMultihost, extension.CapCVEEnrichment, extension.CapRiskScoring,
			extension.CapChangelog, extension.CapIncidents, extension.CapSMTP,
			extension.CapAlertAdvancedFilters, extension.CapSecurityPosture,
			extension.CapOCSPStapling, extension.CapTelegram,
		},
		extension.Pro: {
			extension.CapSlack, extension.CapTeams, extension.CapAlertEscalation,
			extension.CapMaintenanceWindows,
			extension.CapSubscribers, extension.CapPersonalization,
			extension.CapAnomalyDetection,
		},
	}

	total := 0
	for want, caps := range tiers {
		total += len(caps)
		for _, c := range caps {
			assert.Equal(t, want, extension.MinEdition(c), "capability %q", c)
			for _, edition := range editions {
				withEdition(t, edition)
				assert.Equal(t, edition.AtLeast(want), extension.Allows(c), "%q under %q", c, edition)
			}
		}
	}
	assert.Equal(t, 21, total)
}

func TestMinEdition_UnknownCapability(t *testing.T) {
	assert.Equal(t, extension.Pro, extension.MinEdition("no_such_capability"))

	withEdition(t, extension.Community)
	assert.False(t, extension.Allows("no_such_capability"))
	withEdition(t, extension.Personal)
	assert.False(t, extension.Allows("no_such_capability"))
	withEdition(t, extension.Pro)
	assert.True(t, extension.Allows("no_such_capability"))
}

func TestCatalog_ReturnsACopy(t *testing.T) {
	c := extension.Catalog()
	c[extension.CapAlertEscalation] = extension.Community
	c["invented"] = extension.Community

	assert.Equal(t, extension.Pro, extension.MinEdition(extension.CapAlertEscalation))
	assert.Len(t, extension.Catalog(), 21)
}

func TestLimit_Matrix(t *testing.T) {
	u := extension.Unlimited
	want := map[extension.Resource]map[extension.Edition]int{
		extension.ResourceEndpoints:        {extension.Community: 10, extension.Personal: u, extension.Pro: u},
		extension.ResourceHeartbeats:       {extension.Community: 5, extension.Personal: u, extension.Pro: u},
		extension.ResourceCertificates:     {extension.Community: 5, extension.Personal: u, extension.Pro: u},
		extension.ResourceStatusComponents: {extension.Community: 3, extension.Personal: u, extension.Pro: u},
		extension.ResourceAgentHosts:       {extension.Community: 0, extension.Personal: 20, extension.Pro: u},
		"no_such_resource":                 {extension.Community: 0, extension.Personal: 0, extension.Pro: 0},
	}

	for resource, perEdition := range want {
		for edition, limit := range perEdition {
			t.Run(string(edition)+"/"+string(resource), func(t *testing.T) {
				withEdition(t, edition)
				assert.Equal(t, limit, extension.Limit(resource))
			})
		}
	}
}

func TestTiers_MatchesTheLimits(t *testing.T) {
	u := extension.Unlimited
	assert.Equal(t, map[extension.Edition]map[extension.Resource]int{
		extension.Community: {"endpoints": 10, "heartbeats": 5, "certificates": 5, "status_components": 3, "agent_hosts": 0},
		extension.Personal:  {"endpoints": u, "heartbeats": u, "certificates": u, "status_components": u, "agent_hosts": 20},
		extension.Pro:       {"endpoints": u, "heartbeats": u, "certificates": u, "status_components": u, "agent_hosts": u},
	}, extension.Tiers())
}

func TestHistoryCap_IsTheAnnouncedTiering(t *testing.T) {
	assert.Equal(t, 7*24*time.Hour, Policy{}.HistoryCap(extension.Community))
	assert.Equal(t, 30*24*time.Hour, Policy{}.HistoryCap(extension.Personal))
	assert.Equal(t, 90*24*time.Hour, Policy{}.HistoryCap(extension.Pro))
}

func TestMinEditionForHistoryWindow_IsDerivedFromTheCaps(t *testing.T) {
	want := map[string]extension.Edition{
		"1h": extension.Community, "6h": extension.Community, "24h": extension.Community, "7d": extension.Community,
		"30d": extension.Personal,
		"90d": extension.Pro,
	}
	for name, expected := range want {
		w, ok := extension.ResolveHistoryWindow(name)
		require.True(t, ok, "window %q is not in the catalogue", name)
		assert.Equal(t, expected, extension.MinEditionForHistoryWindow(w), "window %q", name)
	}
}

func TestMaxHistoryWindow_PerEdition(t *testing.T) {
	for edition, want := range map[extension.Edition]string{
		extension.Community: "7d",
		extension.Personal:  "30d",
		extension.Pro:       "90d",
	} {
		withEdition(t, edition)
		assert.Equal(t, want, extension.MaxHistoryWindow().Name, "edition %q", edition)
	}
}

func TestAllowsHistoryWindow_MatchesTheCaps(t *testing.T) {
	opens := map[extension.Edition]map[string]bool{
		extension.Community: {"1h": true, "6h": true, "24h": true, "7d": true, "30d": false, "90d": false},
		extension.Personal:  {"1h": true, "6h": true, "24h": true, "7d": true, "30d": true, "90d": false},
		extension.Pro:       {"1h": true, "6h": true, "24h": true, "7d": true, "30d": true, "90d": true},
	}
	required := map[string]extension.Edition{
		"1h": extension.Community, "6h": extension.Community, "24h": extension.Community, "7d": extension.Community,
		"30d": extension.Personal, "90d": extension.Pro,
	}
	for edition, windows := range opens {
		withEdition(t, edition)
		for name, want := range windows {
			w, ok := extension.ResolveHistoryWindow(name)
			require.True(t, ok)
			allowed, req := extension.AllowsHistoryWindow(w)
			assert.Equal(t, want, allowed, "%s / %s", edition, name)
			assert.Equal(t, required[name], req, "%s / %s required edition", edition, name)
		}
	}
}

func TestMaxHistoryWindow_UnknownEditionFallsBackToTheFloor(t *testing.T) {
	withEdition(t, extension.Edition("enterprise-2030"))

	assert.Equal(t, "7d", extension.MaxHistoryWindow().Name)

	paid, ok := extension.ResolveHistoryWindow("30d")
	require.True(t, ok)
	allowed, required := extension.AllowsHistoryWindow(paid)
	assert.False(t, allowed)
	assert.Equal(t, extension.Personal, required)
}

func TestLiftingEdition_EveryEditionEveryResource(t *testing.T) {
	capped := []extension.Resource{
		extension.ResourceEndpoints, extension.ResourceHeartbeats,
		extension.ResourceCertificates, extension.ResourceStatusComponents,
	}
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro, "enterprise-2030"} {
		withEdition(t, edition)
		for _, r := range capped {
			assert.Equal(t, extension.Personal, extension.LiftingEdition(r), "%s under %q", r, edition)
		}
	}

	for edition, want := range map[extension.Edition]extension.Edition{
		extension.Community: extension.Personal,
		extension.Personal:  extension.Pro,
		extension.Pro:       extension.Pro,
		"enterprise-2030":   extension.Personal,
	} {
		withEdition(t, edition)
		assert.Equal(t, want, extension.LiftingEdition(extension.ResourceAgentHosts), "agent_hosts under %q", edition)
	}
}

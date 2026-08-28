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

import "maps"

// Capability names a unit of functionality that an edition may or may not
// open. The identifiers are the ones already exposed by GET /api/v1/edition,
// and the MCP tools use the same vocabulary, so a capability resolves the same
// way whichever surface asks.
type Capability string

const (
	// Community
	CapAlertRouting   Capability = "alert_routing"
	CapSwarmDashboard Capability = "swarm_dashboard"
	CapK8sCluster     Capability = "k8s_cluster"
	// CapResourceHistory is the right to see a history at all, which every
	// edition now has. What separates the editions is how far back they may
	// look, and that is a duration. See history_window.go.
	CapResourceHistory Capability = "resource_history"

	// Personal
	CapMultihost            Capability = "multihost"
	CapCVEEnrichment        Capability = "cve_enrichment"
	CapRiskScoring          Capability = "risk_scoring"
	CapChangelog            Capability = "changelog"
	CapIncidents            Capability = "incidents"
	CapSMTP                 Capability = "smtp"
	CapAlertAdvancedFilters Capability = "alert_advanced_filters"
	CapSecurityPosture      Capability = "security_posture"
	CapOCSPStapling         Capability = "ocsp_stapling"

	// Pro
	CapSlack              Capability = "slack"
	CapTeams              Capability = "teams"
	CapAlertEscalation    Capability = "alert_escalation"
	CapAlertEntityRouting Capability = "alert_entity_routing"
	CapMaintenanceWindows Capability = "maintenance_windows"
	CapSubscribers        Capability = "subscribers"
	CapPersonalization    Capability = "personalization"
)

// minEdition is the single source of truth for authorization. The REST
// middleware, the MCP tools and the /api/v1/edition response all read it, which
// is what keeps the three surfaces from drifting apart.
var minEdition = map[Capability]Edition{
	CapAlertRouting:    Community,
	CapSwarmDashboard:  Community,
	CapK8sCluster:      Community,
	CapResourceHistory: Community,

	CapMultihost:            Personal,
	CapCVEEnrichment:        Personal,
	CapRiskScoring:          Personal,
	CapChangelog:            Personal,
	CapIncidents:            Personal,
	CapSMTP:                 Personal,
	CapAlertAdvancedFilters: Personal,
	CapSecurityPosture:      Personal,
	CapOCSPStapling:         Personal,

	CapSlack:              Pro,
	CapTeams:              Pro,
	CapAlertEscalation:    Pro,
	CapAlertEntityRouting: Pro,
	CapMaintenanceWindows: Pro,
	CapSubscribers:        Pro,
	CapPersonalization:    Pro,
}

// MinEdition returns the lowest edition that opens c. An unknown capability
// resolves to Pro: a capability nobody declared is not one we hand out.
func MinEdition(c Capability) Edition {
	if e, ok := minEdition[c]; ok {
		return e
	}
	return Pro
}

// Allows reports whether the running edition opens c.
func Allows(c Capability) bool {
	return CurrentEdition().AtLeast(MinEdition(c))
}

// Catalog returns a copy of the capability table, for callers that project it
// (the edition endpoint) rather than query it.
func Catalog() map[Capability]Edition {
	out := make(map[Capability]Edition, len(minEdition))
	maps.Copy(out, minEdition)
	return out
}

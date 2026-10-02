// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package extension

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
	CapTelegram             Capability = "telegram"

	// Pro
	CapSlack              Capability = "slack"
	CapTeams              Capability = "teams"
	CapAlertEscalation    Capability = "alert_escalation"
	CapMaintenanceWindows Capability = "maintenance_windows"
	CapSubscribers        Capability = "subscribers"
	CapPersonalization    Capability = "personalization"
	CapAnomalyDetection   Capability = "anomaly_detection"
)

// channelCapabilities maps a notification channel type to the capability that
// opens it. A type absent from the map is open in every edition.
var channelCapabilities = map[string]Capability{
	"slack":    CapSlack,
	"teams":    CapTeams,
	"email":    CapSMTP,
	"telegram": CapTelegram,
}

// ChannelCapability returns the capability gating a channel type, and whether
// that type is gated at all.
func ChannelCapability(channelType string) (Capability, bool) {
	c, ok := channelCapabilities[channelType]
	return c, ok
}

// MinEdition returns the lowest edition that opens c.
func MinEdition(c Capability) Edition {
	return policy.MinEdition(c)
}

// Allows reports whether the running edition opens c.
func Allows(c Capability) bool {
	return CurrentEdition().AtLeast(MinEdition(c))
}

// Catalog returns a copy of the capability table, for callers that project it
// (the edition endpoint) rather than query it.
func Catalog() map[Capability]Edition {
	caps := policy.Capabilities()
	out := make(map[Capability]Edition, len(caps))
	for _, c := range caps {
		out[c] = policy.MinEdition(c)
	}
	return out
}

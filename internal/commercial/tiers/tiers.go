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
package tiers

import (
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
)

var minEdition = map[extension.Capability]extension.Edition{
	extension.CapMultihost:            extension.Personal,
	extension.CapCVEEnrichment:        extension.Personal,
	extension.CapRiskScoring:          extension.Personal,
	extension.CapChangelog:            extension.Personal,
	extension.CapIncidents:            extension.Personal,
	extension.CapSMTP:                 extension.Personal,
	extension.CapAlertAdvancedFilters: extension.Personal,
	extension.CapSecurityPosture:      extension.Personal,
	extension.CapOCSPStapling:         extension.Personal,
	extension.CapTelegram:             extension.Personal,

	extension.CapSlack:              extension.Pro,
	extension.CapTeams:              extension.Pro,
	extension.CapAlertEscalation:    extension.Pro,
	extension.CapAlertEntityRouting: extension.Pro,
	extension.CapMaintenanceWindows: extension.Pro,
	extension.CapSubscribers:        extension.Pro,
	extension.CapPersonalization:    extension.Pro,
}

var limits = map[extension.Edition]map[extension.Resource]int{
	extension.Personal: {
		extension.ResourceEndpoints:        extension.Unlimited,
		extension.ResourceHeartbeats:       extension.Unlimited,
		extension.ResourceCertificates:     extension.Unlimited,
		extension.ResourceStatusComponents: extension.Unlimited,
		extension.ResourceAgentHosts:       20,
	},
	extension.Pro: {
		extension.ResourceEndpoints:        extension.Unlimited,
		extension.ResourceHeartbeats:       extension.Unlimited,
		extension.ResourceCertificates:     extension.Unlimited,
		extension.ResourceStatusComponents: extension.Unlimited,
		extension.ResourceAgentHosts:       extension.Unlimited,
	},
}

var historyCaps = map[extension.Edition]time.Duration{
	extension.Personal: 30 * 24 * time.Hour,
	extension.Pro:      90 * 24 * time.Hour,
}

// Policy is the commercial tier table: Community as the core declares it, plus what Personal and Pro add.
type Policy struct{}

// Capabilities lists every declared capability.
func (Policy) Capabilities() []extension.Capability {
	out := extension.CommunityPolicy{}.Capabilities()
	for c := range minEdition {
		out = append(out, c)
	}
	return out
}

// MinEdition returns the lowest edition that opens c, Pro for an undeclared capability.
func (Policy) MinEdition(c extension.Capability) extension.Edition {
	if e, ok := minEdition[c]; ok {
		return e
	}
	return extension.CommunityPolicy{}.MinEdition(c)
}

// Limit returns the cap on r under e, 0 for an undeclared resource.
func (Policy) Limit(e extension.Edition, r extension.Resource) int {
	if l, ok := limits[e]; ok {
		return l[r]
	}
	return extension.CommunityPolicy{}.Limit(e, r)
}

// HistoryCap returns how far back e may look, the Community cap for an unknown edition.
func (Policy) HistoryCap(e extension.Edition) time.Duration {
	if d, ok := historyCaps[e]; ok {
		return d
	}
	return extension.CommunityPolicy{}.HistoryCap(e)
}

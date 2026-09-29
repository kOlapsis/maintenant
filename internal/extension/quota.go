// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package extension

// Resource names a capped resource type.
type Resource string

const (
	ResourceEndpoints        Resource = "endpoints"
	ResourceHeartbeats       Resource = "heartbeats"
	ResourceCertificates     Resource = "certificates"
	ResourceStatusComponents Resource = "status_components"
	ResourceAgentHosts       Resource = "agent_hosts"
)

// Unlimited is the limit value meaning "no cap". It is reported to the UI as-is.
const Unlimited = -1

var resources = []Resource{
	ResourceEndpoints,
	ResourceHeartbeats,
	ResourceCertificates,
	ResourceStatusComponents,
	ResourceAgentHosts,
}

// Limit returns the cap for r under the running edition.
func Limit(r Resource) int {
	return policy.Limit(CurrentEdition(), r)
}

// Tiers returns the cap of every resource for every edition.
func Tiers() map[Edition]map[Resource]int {
	out := make(map[Edition]map[Resource]int, len(editionOrder))
	for _, e := range editionOrder {
		limits := make(map[Resource]int, len(resources))
		for _, r := range resources {
			limits[r] = policy.Limit(e, r)
		}
		out[e] = limits
	}
	return out
}

// LiftingEdition returns the lowest edition whose cap on r exceeds the running edition's cap.
func LiftingEdition(r Resource) Edition {
	current := Limit(r)
	for _, e := range editionOrder {
		if l := policy.Limit(e, r); l == Unlimited || (current != Unlimited && l > current) {
			return e
		}
	}
	return Pro
}

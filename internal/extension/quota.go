// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package extension

import (
	"cmp"
	"slices"
	"time"
)

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

// WithinLimit reports whether a new r fits beside count existing ones under the running edition's cap.
func WithinLimit(r Resource, count int) bool {
	limit := Limit(r)
	return limit < 0 || count < limit
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

// LimitCandidate is one element competing for a slot under a cap.
type LimitCandidate struct {
	ID            string
	Grandfathered bool
	Since         time.Time
	Key           string
}

// Allocate returns the IDs of the candidates left without a slot under limit.
func Allocate(cands []LimitCandidate, limit int) map[string]bool {
	limited := map[string]bool{}
	if limit == Unlimited {
		return limited
	}
	sorted := slices.Clone(cands)
	slices.SortFunc(sorted, func(a, b LimitCandidate) int {
		if a.Grandfathered != b.Grandfathered {
			if a.Grandfathered {
				return -1
			}
			return 1
		}
		return cmp.Or(a.Since.Compare(b.Since), cmp.Compare(a.Key, b.Key), cmp.Compare(a.ID, b.ID))
	})
	grandfathered := 0
	for _, c := range sorted {
		if c.Grandfathered {
			grandfathered++
		}
	}
	for _, c := range sorted[min(max(limit, grandfathered), len(sorted)):] {
		limited[c.ID] = true
	}
	return limited
}

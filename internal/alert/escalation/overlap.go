// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package escalation

import (
	"slices"
	"strings"
)

// DetectOverlap checks which policies in existing plausibly overlap with candidate.
// "Overlap" (R7): filter sets intersect AND at least one shared channel across any level.
// Empty filter field = matches all (universe); non-empty = explicit set.
// The candidate is compared against all policies in existing; policies with the same ID
// as candidate are skipped (for update flows).
func DetectOverlap(candidate *Policy, existing []*Policy) []OverlapWarning {
	var warnings []OverlapWarning
	for _, p := range existing {
		if candidate.ID != "" && p.ID == candidate.ID {
			continue
		}
		if !filtersIntersect(candidate.Filters, p.Filters) {
			continue
		}
		shared := sharedChannels(candidate, p)
		if len(shared) == 0 {
			continue
		}
		desc := filterIntersectionDescription(candidate.Filters, p.Filters)
		warnings = append(warnings, OverlapWarning{
			PolicyID:           p.ID,
			PolicyName:         p.Name,
			SharedChannels:     shared,
			FilterIntersection: desc,
		})
	}
	return warnings
}

// filtersIntersect returns true if the two filter sets can match the same alert.
// Empty list = universe (matches all). Non-empty list = explicit set.
func filtersIntersect(a, b Filters) bool {
	return setsIntersect(a.Severities, b.Severities) &&
		scopeSetsIntersect(a.Scopes, b.Scopes) &&
		setsIntersect(a.Tags, b.Tags)
}

// setsIntersect returns true if two string slices share at least one element,
// treating empty slice as "all" (universe).
func setsIntersect(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for _, av := range a {
		if slices.Contains(b, av) {
			return true
		}
	}
	return false
}

// scopeSetsIntersect returns true if scope slices share at least one element,
// treating empty slice as "all".
func scopeSetsIntersect(a, b []Scope) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for _, av := range a {
		for _, bv := range b {
			if av.Kind == bv.Kind && av.RefID == bv.RefID {
				return true
			}
		}
	}
	return false
}

// sharedChannels returns the channel IDs present in any level of both policies.
func sharedChannels(a, b *Policy) []string {
	aChans := make(map[string]struct{})
	for _, lvl := range a.Levels {
		for _, id := range lvl.ChannelIDs {
			aChans[id] = struct{}{}
		}
	}
	seen := make(map[string]struct{})
	var shared []string
	for _, lvl := range b.Levels {
		for _, id := range lvl.ChannelIDs {
			if _, ok := aChans[id]; ok {
				if _, already := seen[id]; !already {
					shared = append(shared, id)
					seen[id] = struct{}{}
				}
			}
		}
	}
	return shared
}

// filterIntersectionDescription returns a human-readable description of which filters intersect.
func filterIntersectionDescription(a, b Filters) string {
	var parts []string
	if len(a.Severities) > 0 && len(b.Severities) > 0 {
		parts = append(parts, "severities")
	} else if len(a.Severities) == 0 || len(b.Severities) == 0 {
		parts = append(parts, "all severities")
	}
	if len(a.Scopes) > 0 || len(b.Scopes) > 0 {
		parts = append(parts, "scopes")
	}
	if len(a.Tags) > 0 || len(b.Tags) > 0 {
		parts = append(parts, "tags")
	}
	if len(parts) == 0 {
		return "all"
	}
	return strings.Join(parts, " + ")
}

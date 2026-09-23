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
	"reflect"
	"testing"
)

func TestLimit_DefaultPolicyIsCommunityInEveryEdition(t *testing.T) {
	want := map[Resource]int{
		ResourceEndpoints:        10,
		ResourceHeartbeats:       5,
		ResourceCertificates:     5,
		ResourceStatusComponents: 3,
		ResourceAgentHosts:       0,
		"no_such_resource":       0,
	}
	for _, edition := range []Edition{Community, Personal, Pro} {
		withEdition(t, edition)
		for r, limit := range want {
			if got := Limit(r); got != limit {
				t.Errorf("Limit(%q) under %q with the default policy = %d, want %d", r, edition, got, limit)
			}
		}
	}
}

func TestUnlimited_IsMinusOne(t *testing.T) {
	if Unlimited != -1 {
		t.Errorf("Unlimited = %d, want -1", Unlimited)
	}
}

func TestLimit_DelegatesToThePolicy(t *testing.T) {
	withPolicy(t, fakePolicy{limits: map[Edition]map[Resource]int{
		Personal: {ResourceEndpoints: 42},
	}})
	withEdition(t, Personal)

	if got := Limit(ResourceEndpoints); got != 42 {
		t.Errorf("Limit(endpoints) = %d, want the policy value 42", got)
	}
}

func TestTiers_ProjectsEveryResourceForEveryEdition(t *testing.T) {
	withPolicy(t, fakePolicy{limits: map[Edition]map[Resource]int{
		Community: {ResourceEndpoints: 1, ResourceAgentHosts: 0},
		Personal:  {ResourceEndpoints: 2, ResourceAgentHosts: 20},
		Pro:       {ResourceEndpoints: Unlimited, ResourceAgentHosts: Unlimited},
	}})

	want := map[Edition]map[Resource]int{
		Community: {ResourceEndpoints: 1, ResourceHeartbeats: 0, ResourceCertificates: 0, ResourceStatusComponents: 0, ResourceAgentHosts: 0},
		Personal:  {ResourceEndpoints: 2, ResourceHeartbeats: 0, ResourceCertificates: 0, ResourceStatusComponents: 0, ResourceAgentHosts: 20},
		Pro:       {ResourceEndpoints: Unlimited, ResourceHeartbeats: 0, ResourceCertificates: 0, ResourceStatusComponents: 0, ResourceAgentHosts: Unlimited},
	}
	if got := Tiers(); !reflect.DeepEqual(got, want) {
		t.Errorf("Tiers() = %v, want %v", got, want)
	}
}

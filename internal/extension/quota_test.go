package extension

import "testing"

// TestLimit_Matrix pins the caps for the five resources across the three
// editions. This table is the only declaration; if it changes, the value that
// refuses a creation and the value shown in the UI change together.
func TestLimit_Matrix(t *testing.T) {
	want := map[Resource]map[Edition]int{
		ResourceEndpoints:        {Community: 10, Personal: Unlimited, Pro: Unlimited},
		ResourceHeartbeats:       {Community: 5, Personal: Unlimited, Pro: Unlimited},
		ResourceCertificates:     {Community: 5, Personal: Unlimited, Pro: Unlimited},
		ResourceStatusComponents: {Community: 3, Personal: Unlimited, Pro: Unlimited},
		ResourceAgentHosts:       {Community: 0, Personal: 20, Pro: Unlimited},
	}

	if len(want) != 5 {
		t.Fatalf("the matrix covers %d resources, expected 5", len(want))
	}

	for resource, perEdition := range want {
		for edition, limit := range perEdition {
			t.Run(string(edition)+"/"+string(resource), func(t *testing.T) {
				withEdition(t, edition)
				if got := Limit(resource); got != limit {
					t.Errorf("Limit(%q) under %q = %d, want %d", resource, edition, got, limit)
				}
			})
		}
	}
}

// TestUnlimited_IsMinusOne: the wire contract says -1 means unlimited, and the
// front-end keys off that exact value.
func TestUnlimited_IsMinusOne(t *testing.T) {
	if Unlimited != -1 {
		t.Errorf("Unlimited = %d, want -1", Unlimited)
	}
}

// TestLimit_UnknownResource: an undeclared resource grants no capacity.
func TestLimit_UnknownResource(t *testing.T) {
	for _, edition := range []Edition{Community, Personal, Pro} {
		withEdition(t, edition)
		if got := Limit("no_such_resource"); got != 0 {
			t.Errorf("Limit of an unknown resource under %q = %d, want 0", edition, got)
		}
	}
}

// TestLimit_AgentHostsIsTheOnlyCappedPaidResource guards the segmentation
// choice: Personal lifts every cap except the number of machines.
func TestLimit_AgentHostsIsTheOnlyCappedPaidResource(t *testing.T) {
	withEdition(t, Personal)
	for _, r := range []Resource{
		ResourceEndpoints, ResourceHeartbeats, ResourceCertificates, ResourceStatusComponents,
	} {
		if got := Limit(r); got != Unlimited {
			t.Errorf("Limit(%q) on Personal = %d, want Unlimited", r, got)
		}
	}
	if got := Limit(ResourceAgentHosts); got != 20 {
		t.Errorf("Limit(agent_hosts) on Personal = %d, want 20", got)
	}
}

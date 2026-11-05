// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import "testing"

// Acceptance tests of 051-community-label-limits, FR-021: the documentation of the repository.

var (
	clLabelsGuide   = []string{"docs", "guides", "docker-labels.md"}
	clEndpointsPage = []string{"docs", "features", "endpoints.md"}
	clCertsPage     = []string{"docs", "features", "certificates.md"}
	clTroublePage   = []string{"docs", "troubleshooting.md"}
	clAPIPage       = []string{"docs", "api", "reference.md"}
)

// FR-021: the passages that promise unlimited label endpoints and certificates are corrected.
func TestCommunityLimitDocs_NoLongerPromiseUnlimitedLabels(t *testing.T) {
	mentionsNot(t, clLabelsGuide, "The number of endpoints declared by labels is not capped in any edition")
	mentionsNot(t, configPage, "discovered from container labels are unlimited")
	mentionsNot(t, clTroublePage, "Endpoints and certificate monitors that come from container labels are not counted")
	mentionsNot(t, clEndpointsPage, "Endpoints created from labels are not counted")
	mentionsNot(t, clCertsPage, "Automatic and label monitors are not counted")
}

// FR-021, FR-014, FR-017: the caps count label elements, what existed before the
// upgrade stays, the unmonitored state is described, and the reference pages
// carry the new fields and the new refusal.
func TestCommunityLimitDocs_DescribeTheCapGrandfatheringAndTheState(t *testing.T) {
	for _, page := range [][]string{clLabelsGuide, clEndpointsPage, clCertsPage} {
		matches(t, page, `(?i)not monitored`, "describes the state of an element left out by the cap")
	}
	matches(t, clLabelsGuide, `(?i)(grandfather|upgrad)`, "describes what stays monitored after the upgrade")
	mentions(t, clTroublePage, "label-discovered ones included")
	mentions(t, clAPIPage, "unmonitored")
	mentions(t, mcpPage, "unmonitored")
}

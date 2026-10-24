// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import "testing"

// Documentation floors of 049-email-subject-prefix. The wording itself is reviewed by hand.

var (
	alertsDocPage = []string{"docs", "features", "alerts.md"}
	apiRefPage    = []string{"docs", "api", "reference.md"}
)

// FR-012: the email channel docs describe the label, the default subject, and the subject of the test email.
func TestEmailDocsDescribeTheSubjectLabel(t *testing.T) {
	mentions(t, alertsDocPage, "subject_label")
	mentions(t, alertsDocPage, "[maintenant] ALERT: <message>")
	mentions(t, alertsDocPage, "[maintenant] TEST: Test notification")
	mentions(t, apiRefPage, "subject_label")
}

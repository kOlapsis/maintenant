// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate

import "testing"

func SetMaxAgentLabelMonitors(t *testing.T, n int) {
	prev := maxAgentLabelMonitors
	maxAgentLabelMonitors = n
	t.Cleanup(func() { maxAgentLabelMonitors = prev })
}

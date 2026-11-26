// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import "testing"

func SetMaxAgentContainers(t *testing.T, n int) {
	prev := maxAgentContainers
	maxAgentContainers = n
	t.Cleanup(func() { maxAgentContainers = prev })
}

func UnknownLabelsWarned(externalID string) bool {
	_, ok := unknownLabelsWarned.Load(externalID)
	return ok
}

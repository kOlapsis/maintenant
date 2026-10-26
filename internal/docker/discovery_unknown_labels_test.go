// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13: local discovery reports a typo in a maintenant.* label once.

func warnLinesMentioning(logs, needle string) int {
	n := 0
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "level=WARN") && strings.Contains(line, needle) {
			n++
		}
	}
	return n
}

// FR-010, US4 scenario 2: discovered, then rediscovered at every sync, the container is warned about once.
func TestDiscoverAll_WarnsOnceAboutAnUnknownLabel(t *testing.T) {
	api := &fakeAPI{list: []container.Summary{
		{
			ID:     "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
			Names:  []string{"/web"},
			State:  "running",
			Labels: map[string]string{"maintenant.alert.restart-threshold": "3", "maintenant.group": "web"},
		},
		{
			ID:     "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222",
			Names:  []string{"/db"},
			State:  "running",
			Labels: map[string]string{"maintenant.group": "data"},
		},
	}}
	var logs strings.Builder
	c := &Client{cli: api, logger: slog.New(slog.NewTextHandler(&logs, nil))}

	for i := 0; i < 3; i++ {
		containers, err := c.DiscoverAll(context.Background())
		require.NoError(t, err)
		require.Len(t, containers, 2)
	}

	assert.Equal(t, 1, warnLinesMentioning(logs.String(), "maintenant.alert.restart-threshold"), "got:\n%s", logs.String())
	assert.Contains(t, logs.String(), "aaaa1111aaaa", "the warning names the container")
	assert.Equal(t, 0, warnLinesMentioning(logs.String(), "maintenant.group"), "a known label is never reported")
}

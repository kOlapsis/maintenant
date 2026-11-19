// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTriggerTools_RefuseAnUnknownSeverity(t *testing.T) {
	for _, bad := range []string{"Critical", "critical,urgent", "warn"} {
		in := triggerInput{Name: "x", FilterSeverities: bad, ChannelIDs: []string{"c1"}}
		err := validateTriggerCommon(&in)
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "filter_severities")
	}
	for _, good := range []string{"", "critical", "critical, warning", "info,warning,critical", "critical,"} {
		in := triggerInput{Name: "x", FilterSeverities: good, ChannelIDs: []string{"c1"}}
		assert.NoError(t, validateTriggerCommon(&in), good)
	}
}

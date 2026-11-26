// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

func TestIsSource(t *testing.T) {
	for _, s := range []string{"container", "endpoint", "heartbeat", "certificate", "resource", "update", "security", "agent", "host", "swarm", "kubernetes"} {
		assert.True(t, alert.IsSource(s), s)
	}
	for _, s := range []string{"", "monitor", "test", "Container"} {
		assert.False(t, alert.IsSource(s), s)
	}
}

func TestValidateTriggerFilters(t *testing.T) {
	for _, bad := range []string{"monitor", "container,monitor", "Swarm", "test"} {
		err := alert.ValidateTriggerFilters("", bad, "", nil)
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "filter_sources")
	}
	for _, bad := range []string{"container", "container:", ":42", "monitor:42", "42", "container:1,endpoint"} {
		err := alert.ValidateTriggerFilters("", "", bad, nil)
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "filter_scopes")
	}
	err := alert.ValidateTriggerFilters("urgent", "", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filter_severities")

	require.NoError(t, alert.ValidateTriggerFilters("", "", "", nil))
	require.NoError(t, alert.ValidateTriggerFilters("critical", "swarm, kubernetes,update,", "", nil))
	require.NoError(t, alert.ValidateTriggerFilters("", "",
		"container:0195f3c4-11aa-7f00-8c55-2b9d0e3a4c18, workload:production/Deployment/api,pod:production/api-0,node:worker-1,swarm_service:abc,swarm_node:def,agent:x,heartbeat:y,certificate:z,endpoint:w", nil))

	stored := &alert.AlertTrigger{FilterSources: "monitor", FilterScopes: "monitor:42"}
	require.NoError(t, alert.ValidateTriggerFilters("", "monitor,swarm", "monitor:42,infrastructure:", stored))
	require.Error(t, alert.ValidateTriggerFilters("", "monitor,probe", "", stored))
	require.Error(t, alert.ValidateTriggerFilters("", "", "probe:1", stored))
}

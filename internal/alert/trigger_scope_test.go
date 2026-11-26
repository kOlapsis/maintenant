// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeOfAnEntityWithoutIDIsAcceptedAndMatched(t *testing.T) {
	for _, entityType := range []string{"infrastructure", "swarm_cluster"} {
		scope := entityType + ":"
		require.NoError(t, ValidateTriggerFilters("", "", scope, nil), scope)
		assert.True(t, matchesTrigger(&AlertTrigger{FilterScopes: scope}, &Alert{EntityType: entityType}), scope)
		assert.Error(t, ValidateTriggerFilters("", "", scope+"x", nil), scope+"x")
		assert.Error(t, ValidateTriggerFilters("", "", entityType, nil), entityType)
	}
}

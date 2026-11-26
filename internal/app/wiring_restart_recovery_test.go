// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/container"
)

func TestRestartRecoveryEvent_SaysWhenTheContainerStoppedCleanly(t *testing.T) {
	evt := restartRecoveryEvent(map[string]any{"container_id": "c1", "container_name": "job", "state": container.StateCompleted})
	assert.True(t, evt.IsRecover)
	assert.Equal(t, "Container job stopped cleanly, it no longer restarts", evt.Message)

	evt = restartRecoveryEvent(map[string]any{"container_id": "c1", "container_name": "web", "state": container.StateRunning})
	assert.Equal(t, "Container web restart rate returned to normal", evt.Message)
}

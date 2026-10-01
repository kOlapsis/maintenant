// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
)

func TestRestartAlert_EncodesInSnakeCase(t *testing.T) {
	raw, err := json.Marshal(&RestartAlert{
		ContainerID: "c1", ContainerName: "web", RestartCount: 5, Threshold: 3,
		Severity: container.SeverityWarning, Timestamp: time.Unix(0, 0).UTC(), AgentID: "a1",
	})
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{
		"container_id", "container_name", "restart_count", "threshold", "severity", "timestamp", "agent_id",
	}, keys)
}

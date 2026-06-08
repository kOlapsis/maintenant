// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package docker

import (
	"testing"

	"github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processEvent must surface the container image from the Docker event actor
// attributes so agents can forward it to the server.
func TestProcessEvent_ExtractsImage(t *testing.T) {
	msg := events.Message{
		Type:   events.ContainerEventType,
		Action: "start",
		Actor: events.Actor{
			ID:         "ctr123",
			Attributes: map[string]string{"name": "demo", "image": "adminer:latest"},
		},
		Time: 1,
	}

	evt := processEvent(msg)
	require.NotNil(t, evt)
	assert.Equal(t, "container", evt.ResourceType)
	assert.Equal(t, "ctr123", evt.ExternalID)
	assert.Equal(t, "demo", evt.Name)
	assert.Equal(t, "adminer:latest", evt.Image)
}

func TestProcessEvent_DieCarriesImageAndExitCode(t *testing.T) {
	msg := events.Message{
		Type:   events.ContainerEventType,
		Action: "die",
		Actor: events.Actor{
			ID:         "ctr456",
			Attributes: map[string]string{"name": "crash", "image": "app:v1", "exitCode": "137"},
		},
		Time: 1,
	}

	evt := processEvent(msg)
	require.NotNil(t, evt)
	assert.Equal(t, "app:v1", evt.Image)
	assert.Equal(t, "137", evt.ExitCode)
}

func TestProcessEvent_IgnoresUnknownAction(t *testing.T) {
	msg := events.Message{
		Type:   events.ContainerEventType,
		Action: "exec_create",
		Actor:  events.Actor{ID: "x", Attributes: map[string]string{}},
		Time:   1,
	}
	assert.Nil(t, processEvent(msg))
}

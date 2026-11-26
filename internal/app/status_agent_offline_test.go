// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/status"
)

type fakeAgentSessions struct {
	extpoint.AgentSessions
	live, runtimeLost map[string]bool
}

func (s fakeAgentSessions) IsConnected(agentID string) bool        { return s.live[agentID] }
func (s fakeAgentSessions) RuntimeUnreachable(agentID string) bool { return s.runtimeLost[agentID] }

func TestStatusProvider_RemoteContainerFollowsTheAgentOfflineAlert(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, nil)
	a.rt = answeringRuntime{a.rt}
	a.agentSessions = fakeAgentSessions{
		live:        map[string]bool{"live": true, "live-runtime-lost": true},
		runtimeLost: map[string]bool{"live-runtime-lost": true},
	}

	lastSeen := map[string]time.Duration{
		"live": 0, "live-runtime-lost": 0,
		"reconnecting": 3 * time.Minute, // seen before a server restart that lasted three minutes
		"just-dropped": 10 * time.Second,
		"gone":         time.Hour,
	}
	for name, ago := range lastSeen {
		seen := time.Now().Add(-ago)
		require.NoError(t, a.agentStore.Insert(ctx, &agent.Agent{
			AgentID: name, Hostname: name, Status: "active", DetectedRuntime: "docker", CreatedAt: seen, LastSeenAt: &seen,
		}))
	}
	for _, name := range []string{"just-dropped", "gone"} {
		_, err := a.alertStore.InsertAlert(ctx, &alert.Alert{
			Source: alert.SourceAgent, AlertType: "disconnected", Severity: alert.SeverityWarning, Status: alert.StatusActive,
			Message: "Agent disconnected", EntityType: "agent", EntityID: name, EntityName: name, AgentID: name, Details: "{}", FiredAt: time.Now(),
		})
		require.NoError(t, err)
	}

	for name, want := range map[string]string{
		"live":              status.StatusOperational,
		"live-runtime-lost": status.StatusMajorOutage,
		"reconnecting":      status.StatusOperational,
		"just-dropped":      status.StatusOperational,
		"gone":              status.StatusMajorOutage,
	} {
		got := containerMonitorStatus(t, a, &container.Container{AgentID: name, State: container.StateRunning})
		assert.Equal(t, want, got, name)
	}

	a.agentSessions = nil
	assert.Equal(t, status.StatusMajorOutage, containerMonitorStatus(t, a, &container.Container{AgentID: "live", State: container.StateRunning}),
		"no agent can connect without the agent server")
}

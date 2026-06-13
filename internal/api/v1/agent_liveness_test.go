// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/uid"
)

// A container reported by an agent with no live stream is offline; the local
// runtime and connected agents are not. stubK8sSessions is defined in
// kubernetes_test.go (same package).
func TestContainerHandler_agentOffline(t *testing.T) {
	h := &ContainerHandler{sessions: stubK8sSessions{connected: map[string]bool{"agent-ON": true}}}

	assert.True(t, h.agentOffline("agent-OFF"), "remote agent with no stream is offline")
	assert.False(t, h.agentOffline("agent-ON"), "connected agent is not offline")
	assert.False(t, h.agentOffline(uid.LocalAgent), "local runtime is never agent-offline")
	assert.False(t, h.agentOffline(""), "empty agent id is never offline")

	assert.False(t, (&ContainerHandler{}).agentOffline("agent-OFF"),
		"without sessions wiring nothing is marked offline")
}

// An endpoint probed by an offline remote agent is flagged stale; connected,
// local and server-probed (no agent) endpoints stay live.
func TestEndpointHandler_markStale(t *testing.T) {
	h := &EndpointHandler{sessions: stubK8sSessions{connected: map[string]bool{"agent-ON": true}}}

	off := h.markStale(&endpoint.Endpoint{AgentID: "agent-OFF"})
	assert.NotNil(t, off.Stale, "offline agent's endpoint is stale")
	assert.NotNil(t, off.AgentOffline)

	on := h.markStale(&endpoint.Endpoint{AgentID: "agent-ON"})
	assert.Nil(t, on.Stale, "connected agent's endpoint is live")

	local := h.markStale(&endpoint.Endpoint{AgentID: uid.LocalAgent})
	assert.Nil(t, local.Stale, "local endpoint is not agent-governed")

	serverProbed := h.markStale(&endpoint.Endpoint{AgentID: ""})
	assert.Nil(t, serverProbed.Stale, "server-probed endpoint (no agent) is never stale")
}

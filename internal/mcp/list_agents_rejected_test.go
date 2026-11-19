// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/extension"
)

type listedAgents []*agent.Agent

func (l listedAgents) List(context.Context, string) ([]*agent.Agent, error) { return l, nil }

type rejectingSessions map[string]uint64

func (rejectingSessions) IsConnected(string) bool                { return true }
func (r rejectingSessions) RejectedEvents(agentID string) uint64 { return r[agentID] }

func TestListAgents_ReportsRejectedEvents(t *testing.T) {
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = prev })

	svc := &Services{
		Agents:   listedAgents{{AgentID: "web-01", Label: "web-01", Hostname: "web-01", DetectedRuntime: "docker"}},
		Sessions: rejectingSessions{"web-01": 3},
	}
	res, _, err := listAgentsHandler(svc)(context.Background(), nil, listAgentsInput{})
	require.NoError(t, err)
	require.Len(t, res.Content, 1)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Content[0].(*gomcp.TextContent).Text), &rows))
	require.Len(t, rows, 1)
	assert.EqualValues(t, 3, rows[0]["rejected_events"])
}

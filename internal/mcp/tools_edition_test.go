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

package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callGetEdition(t *testing.T, svc *Services) map[string]any {
	t.Helper()
	result, _, err := getEditionHandler(svc)(context.Background(), nil, getEditionInput{})
	require.NoError(t, err)
	require.False(t, result.IsError)

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &m))
	return m
}

// The tool reports the running edition rather than making the caller ask.
func TestGetEditionHandler_ReportsTheRunningEdition(t *testing.T) {
	for _, e := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		t.Run(string(e), func(t *testing.T) {
			withEdition(t, e)
			m := callGetEdition(t, &Services{})
			assert.Equal(t, string(e), m["edition"])
		})
	}
}

// features and feature_editions are projected from the same registry the gate
// reads, so the tool can never advertise a capability the gate refuses.
func TestGetEditionHandler_FeaturesMatchTheRegistry(t *testing.T) {
	withEdition(t, extension.Personal)
	m := callGetEdition(t, &Services{})

	features, ok := m["features"].(map[string]any)
	require.True(t, ok)
	editions, ok := m["feature_editions"].(map[string]any)
	require.True(t, ok)

	catalog := extension.Catalog()
	require.Len(t, features, len(catalog))
	for c, min := range catalog {
		assert.Equal(t, extension.Allows(c), features[string(c)], "feature %q", c)
		assert.Equal(t, string(min), editions[string(c)], "feature edition %q", c)
	}
}

func TestGetEditionHandler_ResourceHistoryWindow(t *testing.T) {
	withEdition(t, extension.Pro)
	m := callGetEdition(t, &Services{})

	history, ok := m["resource_history"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, extension.MaxHistoryWindow().Name, history["max_window"])
	assert.NotEmpty(t, history["windows"])
}

// Quotas are reported for the resources this server can count, and skipped for
// the ones it has no store for — never guessed at zero.
func TestGetEditionHandler_QuotasOnlyForCountableResources(t *testing.T) {
	withEdition(t, extension.Community)
	m := callGetEdition(t, &Services{})

	quotas, ok := m["quotas"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, quotas)
}

func TestGetEditionHandler_QuotaCarriesUsageAndLimit(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &Services{Agents: &mcpEditionAgentLister{count: 3}}

	quotas := callGetEdition(t, svc)["quotas"].(map[string]any)
	agents, ok := quotas["agent_hosts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(3), agents["used"])
	assert.Equal(t, float64(extension.Limit(extension.ResourceAgentHosts)), agents["limit"])
}

type mcpEditionAgentLister struct{ count int }

func (m *mcpEditionAgentLister) List(_ context.Context, _ string) ([]*agent.Agent, error) {
	out := make([]*agent.Agent, m.count)
	for i := range out {
		out[i] = &agent.Agent{}
	}
	return out, nil
}

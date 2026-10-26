// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 049-email-subject-prefix on the MCP channel tools: contract api.md says create_channel and
// update_channel carry the label in config and apply the same validation as the API.

const prodParisConfig = `{"subject_label":"prod-paris"}`

func labelledEmailChannel(t *testing.T) (*Services, *datedChannelStore, string) {
	t.Helper()
	svc, cs := buildSecretChannelServices(t)
	id, _, _ := createOK(t, svc, createChannelInput{Name: "ops mail", Type: "email", URL: "ops@example.com", Config: prodParisConfig})
	require.Equal(t, prodParisConfig, cs.channels[id].Config)
	return svc, cs, id
}

// US3 scenario 1, FR-007 on MCP: create_channel refuses a label over 64 characters and stores nothing.
func TestMCPChannels_EmailSubjectLabel_CreateRefusesALongLabel(t *testing.T) {
	svc, cs, _ := labelledEmailChannel(t)

	result, _, err := createChannelHandler(svc)(context.Background(), nil, createChannelInput{
		Name: "too long", Type: "email", URL: "ops@example.com",
		Config: `{"subject_label":"` + strings.Repeat("a", 65) + `"}`,
	})

	require.NoError(t, err)
	require.True(t, result.IsError, textFromContent(t, result.Content))
	assert.Contains(t, textFromContent(t, result.Content), "64")
	assert.Len(t, cs.channels, 1, "nothing is stored")
}

// US3 scenarios 2 and 3, FR-007 on MCP: update_channel refuses a line break and the channel keeps its values.
func TestMCPChannels_EmailSubjectLabel_UpdateRefusesALineBreak(t *testing.T) {
	svc, cs, id := labelledEmailChannel(t)

	result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{
		ID: id, Name: str("renamed"), Config: str(`{"subject_label":"prod\nBcc: evil@example.com"}`),
	})

	require.NoError(t, err)
	require.True(t, result.IsError, textFromContent(t, result.Content))
	assert.Regexp(t, `(?i)control`, textFromContent(t, result.Content))
	assert.Equal(t, prodParisConfig, cs.channels[id].Config)
	assert.Equal(t, "ops mail", cs.channels[id].Name)
}

// FR-001, FR-009 on MCP: update_channel replaces the label, and an update without config keeps it.
func TestMCPChannels_EmailSubjectLabel_UpdateReplacesOrKeeps(t *testing.T) {
	svc, cs, id := labelledEmailChannel(t)

	result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Name: str("ops mail 2")})
	require.NoError(t, err)
	require.False(t, result.IsError, textFromContent(t, result.Content))
	assert.Equal(t, prodParisConfig, cs.channels[id].Config)

	result, _, err = updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, Config: str(`{"subject_label":"staging"}`)})
	require.NoError(t, err)
	require.False(t, result.IsError, textFromContent(t, result.Content))
	assert.Equal(t, `{"subject_label":"staging"}`, cs.channels[id].Config)
}

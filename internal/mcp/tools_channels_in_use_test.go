// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
)

type fixedPolicies struct {
	escalation.Service
	policies []*escalation.Policy
}

func (f fixedPolicies) ListPolicies(context.Context, bool) ([]*escalation.Policy, error) {
	return f.policies, nil
}

func TestDeleteChannelHandler_RefusedWhileAnEscalationPolicyUsesIt(t *testing.T) {
	pinEdition(t, extension.Pro)
	svc, cs, events := buildChannelServices()
	id, err := cs.InsertChannel(context.Background(), &alert.NotificationChannel{Name: "ops", Type: "webhook"})
	require.NoError(t, err)
	svc.EscalationSvc = fixedPolicies{policies: []*escalation.Policy{
		{Name: "day shift", Levels: []escalation.Level{{ChannelIDs: []string{"another"}}}},
		{Name: "night shift", Levels: []escalation.Level{{ChannelIDs: []string{"another"}}, {ChannelIDs: []string{id}}}},
	}}

	result, _, err := deleteChannelHandler(svc)(context.Background(), nil, deleteChannelInput{ID: id})
	require.NoError(t, err)
	require.True(t, result.IsError)
	text := textFromContent(t, result.Content)
	assert.Contains(t, text, "night shift")
	assert.NotContains(t, text, "day shift")
	assert.Len(t, cs.channels, 1)
	assert.Empty(t, *events)
}

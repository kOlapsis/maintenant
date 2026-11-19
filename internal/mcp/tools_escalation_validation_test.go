// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	commesc "github.com/kolapsis/maintenant/internal/commercial/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func realEscalationServices(t *testing.T) (*Services, string) {
	t.Helper()
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	channels := store.NewChannelStore(db)
	chID, err := channels.InsertChannel(context.Background(), &alert.NotificationChannel{
		Name: "ops-webhook", Type: "webhook", URL: "https://example.com/hook", Enabled: true,
	})
	require.NoError(t, err)
	svc := commesc.NewService(store.NewEscalationStore(db), channels,
		func() extension.Edition { return extension.Pro }, mcpNoopSuppressor{}, logger)
	return &Services{EscalationSvc: svc, Logger: logger, Version: "test"}, chID
}

func escalationInput(severities []string, channelIDs ...string) createEscalationPolicyInput {
	return createEscalationPolicyInput{
		Name:    "validated",
		Filters: escalationFiltersInput{Severities: severities},
		Levels:  []escalationLevelInput{{DelaySeconds: 300, ChannelIDs: channelIDs}},
	}
}

func TestEscalationTools_RealStore_RejectsInvalid(t *testing.T) {
	svc, chID := realEscalationServices(t)

	cases := []struct {
		name  string
		input createEscalationPolicyInput
	}{
		{"unknown channel", escalationInput([]string{"critical"}, "00000000-0000-0000-0000-00000000dead")},
		{"channel name instead of id", escalationInput([]string{"critical"}, "ops-webhook")},
		{"empty channel id", escalationInput([]string{"critical"}, chID, "")},
		{"repeated channel", escalationInput([]string{"critical"}, chID, chID)},
		{"capitalised severity", escalationInput([]string{"Critical"}, chID)},
	}
	create := createEscalationPolicyHandler(svc)
	update := updateEscalationPolicyHandler(svc)

	ok, _, err := create(context.Background(), nil, escalationInput([]string{"critical", "warning", "info"}, chID))
	require.NoError(t, err)
	require.False(t, ok.IsError, textFromContent(t, ok.Content))
	policies, err := svc.EscalationSvc.ListPolicies(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, policies, 1)

	for _, tc := range cases {
		t.Run("create/"+tc.name, func(t *testing.T) {
			res, _, err := create(context.Background(), nil, tc.input)
			require.NoError(t, err)
			require.True(t, res.IsError)
			assert.Contains(t, textFromContent(t, res.Content), "validation_failed")
		})
		t.Run("update/"+tc.name, func(t *testing.T) {
			res, _, err := update(context.Background(), nil, updateEscalationPolicyInput{
				ID: policies[0].ID, Name: tc.input.Name, Filters: tc.input.Filters, Levels: tc.input.Levels,
			})
			require.NoError(t, err)
			require.True(t, res.IsError)
			assert.Contains(t, textFromContent(t, res.Content), "validation_failed")
		})
	}
}

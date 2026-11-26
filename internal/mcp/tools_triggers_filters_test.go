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
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestTriggerWrites_RefuseAFilterThatMatchesNoAlert(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	channels := store.NewChannelStore(db)
	triggers := store.NewTriggerStore(db)
	svc := &Services{Channels: channels, Triggers: triggers, Logger: logger, Version: "test"}
	ctx := context.Background()

	ch := &alert.NotificationChannel{Name: "ops", Type: "webhook", URL: "https://example.com/hook", Enabled: true}
	_, err := channels.InsertChannel(ctx, ch)
	require.NoError(t, err)

	legacyID, err := triggers.InsertTrigger(ctx, &alert.AlertTrigger{Name: "legacy", FilterSources: "container,monitor", Enabled: true, ChannelIDs: []string{ch.ID}})
	require.NoError(t, err)

	inputs := map[string]triggerInput{
		"filter_sources": {Name: "x", FilterSources: "container,monitor", ChannelIDs: []string{ch.ID}},
		"filter_scopes":  {Name: "x", FilterScopes: "monitor:42", ChannelIDs: []string{ch.ID}},
	}
	for field, in := range inputs {
		result, _, err := createTriggerHandler(svc)(ctx, nil, in)
		require.NoError(t, err)
		require.True(t, result.IsError, field)
		assert.Contains(t, textFromContent(t, result.Content), field)

	}

	off := false
	result, _, err := updateTriggerHandler(svc)(ctx, nil, updateTriggerInputWithID{ID: legacyID, triggerInput: triggerInput{
		Name: "legacy", FilterSources: "container,monitor", Enabled: &off, ChannelIDs: []string{ch.ID},
	}})
	require.NoError(t, err)
	require.False(t, result.IsError, textFromContent(t, result.Content))
	got, err := triggers.GetTrigger(ctx, legacyID)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, "container,monitor", got.FilterSources)

	result, _, err = updateTriggerHandler(svc)(ctx, nil, updateTriggerInputWithID{ID: legacyID, triggerInput: triggerInput{
		Name: "legacy", FilterSources: "container,monitor,probe", ChannelIDs: []string{ch.ID},
	}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), `"probe"`)

	result, _, err = listTriggersHandler(svc)(ctx, nil, listTriggersInput{})
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "container,monitor")

	all, err := triggers.ListTriggers(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1, "a refused creation leaves no trigger")
	assert.Equal(t, "legacy", all[0].Name, "a refused update leaves the trigger alone")

	result, _, err = updateTriggerHandler(svc)(ctx, nil, updateTriggerInputWithID{ID: legacyID, triggerInput: triggerInput{
		Name: "legacy", FilterSources: "container,swarm,kubernetes", FilterScopes: "container:0195f3c4-11aa-7f00-8c55-2b9d0e3a4c18", ChannelIDs: []string{ch.ID},
	}})
	require.NoError(t, err)
	assert.False(t, result.IsError, textFromContent(t, result.Content))

	result, _, err = createTriggerHandler(svc)(ctx, nil, triggerInput{Name: "posture", FilterScopes: "infrastructure:,swarm_cluster:", ChannelIDs: []string{ch.ID}})
	require.NoError(t, err)
	assert.False(t, result.IsError, textFromContent(t, result.Content))
}

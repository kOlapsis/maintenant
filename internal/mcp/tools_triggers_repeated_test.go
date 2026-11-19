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

func TestTriggerWrites_RefuseARepeatedChannelWithoutWritingAnything(t *testing.T) {
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
	other := &alert.NotificationChannel{Name: "dev", Type: "webhook", URL: "https://example.com/dev", Enabled: true}
	_, err = channels.InsertChannel(ctx, other)
	require.NoError(t, err)

	result, _, err := createTriggerHandler(svc)(ctx, nil, triggerInput{Name: "dup", ChannelIDs: []string{ch.ID, ch.ID}})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "repeated")
	all, err := triggers.ListTriggers(ctx)
	require.NoError(t, err)
	assert.Empty(t, all, "a refused creation leaves no trigger")

	id, err := triggers.InsertTrigger(ctx, &alert.AlertTrigger{Name: "kept", Enabled: true, ChannelIDs: []string{ch.ID}})
	require.NoError(t, err)

	result, _, err = updateTriggerHandler(svc)(ctx, nil, updateTriggerInputWithID{
		ID: id, triggerInput: triggerInput{Name: "renamed", ChannelIDs: []string{other.ID, other.ID}},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)

	got, err := triggers.GetTrigger(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "kept", got.Name)
	assert.Equal(t, []string{ch.ID}, got.ChannelIDs)
}

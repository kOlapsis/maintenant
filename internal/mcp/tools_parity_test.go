// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

type broadcastLog struct{ types []string }

func (b *broadcastLog) record(eventType string, _ any) { b.types = append(b.types, eventType) }

func TestTriggerTools_WritesBroadcastToTheInterface(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	channels := store.NewChannelStore(db)
	heard := &broadcastLog{}
	svc := &Services{Channels: channels, Triggers: store.NewTriggerStore(db), Logger: logger, Version: "test", Broadcast: heard.record}
	ctx := context.Background()
	ch := &alert.NotificationChannel{Name: "ops", Type: "webhook", URL: "https://example.com/hook", Enabled: true}
	_, err := channels.InsertChannel(ctx, ch)
	require.NoError(t, err)

	created, _, err := createTriggerHandler(svc)(ctx, nil, triggerInput{Name: "crit", ChannelIDs: []string{ch.ID}})
	require.NoError(t, err)
	require.False(t, created.IsError, textFromContent(t, created.Content))
	var trig alert.AlertTrigger
	require.NoError(t, json.Unmarshal([]byte(textFromContent(t, created.Content)), &trig))

	updated, _, err := updateTriggerHandler(svc)(ctx, nil, updateTriggerInputWithID{ID: trig.ID, triggerInput: triggerInput{Name: "renamed", ChannelIDs: []string{ch.ID}}})
	require.NoError(t, err)
	require.False(t, updated.IsError, textFromContent(t, updated.Content))

	deleted, _, err := deleteTriggerHandler(svc)(ctx, nil, deleteTriggerInput{ID: trig.ID})
	require.NoError(t, err)
	require.False(t, deleted.IsError, textFromContent(t, deleted.Content))

	assert.Equal(t, []string{event.TriggerCreated, event.TriggerUpdated, event.TriggerDeleted}, heard.types)
}

func TestTriggerTools_ARefusedWriteBroadcastsNothing(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	heard := &broadcastLog{}
	svc := &Services{Channels: store.NewChannelStore(db), Triggers: store.NewTriggerStore(db), Logger: logger, Broadcast: heard.record}

	result, _, err := createTriggerHandler(svc)(context.Background(), nil, triggerInput{Name: "x", ChannelIDs: []string{"missing"}})

	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Empty(t, heard.types)
}

func TestCreateEscalationPolicy_ReportsOverlappingPolicies(t *testing.T) {
	withEdition(t, extension.Pro)
	svc := buildProEscalationServices()
	ctx := context.Background()
	in := createEscalationPolicyInput{Active: true, Levels: []escalationLevelInput{{DelaySeconds: 300, ChannelIDs: []string{"1"}}}}

	in.Name = "first"
	first, _, err := createEscalationPolicyHandler(svc)(ctx, nil, in)
	require.NoError(t, err)
	require.False(t, first.IsError, textFromContent(t, first.Content))
	in.Name = "second"
	second, _, err := createEscalationPolicyHandler(svc)(ctx, nil, in)
	require.NoError(t, err)
	require.False(t, second.IsError, textFromContent(t, second.Content))

	var created struct {
		Warnings []struct {
			PolicyName string `json:"policy_name"`
		} `json:"overlapping_warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(textFromContent(t, second.Content)), &created))
	require.Len(t, created.Warnings, 1)
	assert.Equal(t, "first", created.Warnings[0].PolicyName)

	var alone struct {
		Warnings []any `json:"overlapping_warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(textFromContent(t, first.Content)), &alone))
	assert.NotNil(t, alone.Warnings, "no overlap is an empty list, not a missing field")
	assert.Empty(t, alone.Warnings)
}

func TestGetUpdates_ChangelogFieldsFollowTheCapability(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	now := time.Now()
	_, err := store.NewContainerStore(db).InsertContainer(ctx, &container.Container{
		ExternalID: "ctr", AgentID: uid.LocalAgent, Name: "web", Image: "nginx:1.0", State: container.StateRunning,
		AlertSeverity: container.SeverityWarning, RestartThreshold: 3, RuntimeType: "docker", FirstSeenAt: now, LastStateChangeAt: now,
	})
	require.NoError(t, err)
	updateStore := store.NewUpdateStore(db)
	scanID, err := updateStore.InsertScanRecord(ctx, &update.ScanRecord{StartedAt: time.Now(), Status: update.ScanStatusCompleted})
	require.NoError(t, err)
	_, err = updateStore.InsertImageUpdate(ctx, &update.ImageUpdate{
		ScanID: scanID, ContainerID: "ctr", AgentID: uid.LocalAgent, ContainerName: "web", Image: "nginx:1.0", CurrentTag: "1.0", LatestTag: "1.1",
		UpdateType: update.UpdateTypeMinor, Status: update.StatusAvailable, DetectedAt: time.Now(),
		ChangelogURL: "https://example.com/changelog", ChangelogSummary: "breaking things", SourceURL: "https://example.com/src",
		PreviousDigest: "sha256:old", HasBreakingChanges: true,
	})
	require.NoError(t, err)
	svc := &Services{
		Updates: update.NewService(update.Deps{
			Store:      updateStore,
			Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
			Containers: noContainers{},
			Logger:     logger,
		}),
		Logger: logger,
	}
	fields := []string{"changelog_url", "changelog_summary", "source_url", "previous_digest", "has_breaking_changes"}
	read := func() map[string]any {
		result, _, err := getUpdatesHandler(svc)(ctx, nil, getUpdatesInput{})
		require.NoError(t, err)
		var payload struct {
			Updates []map[string]any `json:"updates"`
		}
		require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &payload))
		require.Len(t, payload.Updates, 1)
		return payload.Updates[0]
	}

	withEdition(t, extension.Community)
	require.False(t, extension.Allows(extension.CapChangelog))
	row := read()
	assert.Equal(t, "nginx:1.0", row["image"])
	for _, f := range fields {
		assert.NotContains(t, row, f)
	}

	withEdition(t, extension.Pro)
	require.True(t, extension.Allows(extension.CapChangelog))
	row = read()
	for _, f := range fields {
		assert.Contains(t, row, f)
	}
}

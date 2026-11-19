// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
)

type guardedRun struct {
	runs      *EscalationStore
	alerts    *AlertStoreImpl
	run       *escalation.Run
	channelID string
}

func newGuardedRun(t *testing.T) guardedRun {
	t.Helper()
	db := openTestDB(t)
	ctx := context.Background()
	alerts := NewAlertStore(db)
	a := &alert.Alert{
		Source: alert.SourceContainer, AlertType: "container_stopped", Severity: alert.SeverityCritical,
		Status: alert.StatusActive, Message: "x stopped", EntityType: "container", EntityID: "e1", EntityName: "x",
		FiredAt: time.Now().UTC(),
	}
	_, err := alerts.InsertAlert(ctx, a)
	require.NoError(t, err)

	channelID, err := NewChannelStore(db).InsertChannel(ctx, &alert.NotificationChannel{
		Name: "ops", Type: "webhook", URL: "https://example.test/ops", Enabled: true,
	})
	require.NoError(t, err)

	runs := NewEscalationStore(db)
	next := time.Now().Add(time.Minute)
	run := &escalation.Run{
		PolicySnapshotJSON: "{}", AlertID: a.ID, Status: escalation.RunStatusActive,
		LastExecutedLevelIndex: -1, StartedAt: time.Now(), NextActionAt: &next,
	}
	_, err = runs.InsertRun(ctx, run)
	require.NoError(t, err)
	return guardedRun{runs: runs, alerts: alerts, run: run, channelID: channelID}
}

func (g guardedRun) levelDelivery() *escalation.Delivery {
	return &escalation.Delivery{
		RunID: g.run.ID, LevelIndex: 0, ChannelID: &g.channelID,
		Status: escalation.DeliveryStatusPending, AttemptStartedAt: time.Now(),
	}
}

func TestReserveDeliveryOnlyWhileTheRunMayNotify(t *testing.T) {
	ctx := context.Background()

	t.Run("live run", func(t *testing.T) {
		g := newGuardedRun(t)
		d := g.levelDelivery()
		reserved, err := g.runs.ReserveDelivery(ctx, d)
		require.NoError(t, err)
		assert.True(t, reserved)
		assert.NotEmpty(t, d.ID)

		_, err = g.runs.ReserveDelivery(ctx, g.levelDelivery())
		assert.True(t, errors.Is(err, escalation.ErrDeliveryDuplicate), "got %v", err)
		ds, err := g.runs.SelectRunDeliveries(ctx, g.run.ID)
		require.NoError(t, err)
		assert.Len(t, ds, 1)
	})

	for name, stop := range map[string]func(g guardedRun) error{
		"stopped run": func(g guardedRun) error {
			return g.runs.TerminateRun(ctx, g.run.ID, escalation.RunStatusStoppedByAck, time.Now())
		},
		"acknowledged alert": func(g guardedRun) error {
			_, err := g.alerts.AcknowledgeAlert(ctx, g.run.AlertID, "alice", time.Now())
			return err
		},
		"resolved alert": func(g guardedRun) error {
			now := time.Now()
			return g.alerts.UpdateAlertStatus(ctx, g.run.AlertID, alert.StatusResolved, &now, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := newGuardedRun(t)
			require.NoError(t, stop(g))
			reserved, err := g.runs.ReserveDelivery(ctx, g.levelDelivery())
			require.NoError(t, err)
			assert.False(t, reserved)
			ds, err := g.runs.SelectRunDeliveries(ctx, g.run.ID)
			require.NoError(t, err)
			assert.Empty(t, ds)
		})
	}
}

func TestUpdateRunProgressLeavesAStoppedRunStopped(t *testing.T) {
	g := newGuardedRun(t)
	ctx := context.Background()

	require.NoError(t, g.runs.TerminateRun(ctx, g.run.ID, escalation.RunStatusStoppedByAck, time.Now()))
	next := time.Now().Add(time.Hour)
	require.NoError(t, g.runs.UpdateRunProgress(ctx, g.run.ID, 0, &next, escalation.RunStatusActive))

	got, err := g.runs.SelectRun(ctx, g.run.ID)
	require.NoError(t, err)
	assert.Equal(t, escalation.RunStatusStoppedByAck, got.Status)
	assert.Equal(t, -1, got.LastExecutedLevelIndex)
	assert.Nil(t, got.NextActionAt)
}

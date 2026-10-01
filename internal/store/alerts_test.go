// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/uid"
)

func seedAlert(t *testing.T, s *AlertStoreImpl, entityID, status string, firedAt time.Time, resolvedAt *time.Time) string {
	t.Helper()
	id, err := s.InsertAlert(context.Background(), &alert.Alert{
		Source: alert.SourceContainer, AlertType: "health_unhealthy", Severity: alert.SeverityWarning,
		Status: status, Message: "unhealthy", EntityType: "container", EntityID: entityID, EntityName: entityID,
		FiredAt: firedAt, ResolvedAt: resolvedAt, CreatedAt: firedAt,
	})
	require.NoError(t, err)
	return id
}

func TestAlertStore_RetentionNeverDeletesAnActiveAlert(t *testing.T) {
	s := NewAlertStore(openTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-100 * 24 * time.Hour)
	recent := now.Add(-24 * time.Hour)

	activeOld := seedAlert(t, s, "c1", alert.StatusActive, old, nil)
	resolvedOld := seedAlert(t, s, "c2", alert.StatusResolved, old, &old)
	resolvedLately := seedAlert(t, s, "c3", alert.StatusResolved, old, &recent)
	recoveryOld := seedAlert(t, s, "c4", alert.StatusResolved, old, nil)
	silencedOld := seedAlert(t, s, "c5", alert.StatusSilenced, old, nil)
	resolvedRecent := seedAlert(t, s, "c6", alert.StatusResolved, recent, &recent)

	deleted, err := s.DeleteInactiveAlertsOlderThan(ctx, now.Add(-90*24*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 3, deleted)

	for id, kept := range map[string]bool{
		activeOld:      true,
		resolvedOld:    false,
		resolvedLately: true,
		recoveryOld:    false,
		silencedOld:    false,
		resolvedRecent: true,
	} {
		a, err := s.GetAlert(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, kept, a != nil, "alert %s", id)
	}
}

func TestAlertStore_PagesThroughAlertsFiredInTheSameSecond(t *testing.T) {
	s := NewAlertStore(openTestDB(t))
	ctx := context.Background()
	firedAt := time.Now().UTC().Truncate(time.Second)

	want := map[string]bool{}
	for i := range 5 {
		want[seedAlert(t, s, "c"+strconv.Itoa(i), alert.StatusActive, firedAt, nil)] = true
	}

	got := map[string]bool{}
	opts := alert.ListAlertsOpts{Limit: 2}
	for page := 0; ; page++ {
		require.Less(t, page, 5, "pagination must end")
		alerts, err := s.ListAlerts(ctx, opts)
		require.NoError(t, err)
		hasMore := len(alerts) > opts.Limit
		if hasMore {
			alerts = alerts[:opts.Limit]
		}
		for _, a := range alerts {
			assert.False(t, got[a.ID], "alert %s listed twice", a.ID)
			got[a.ID] = true
		}
		if !hasMore {
			break
		}
		last := alerts[len(alerts)-1]
		opts.Before, opts.BeforeID = &last.FiredAt, last.ID
	}
	assert.Equal(t, want, got, "every alert must be listed once across the pages")
}

func TestAlertStore_GetAlertOfUnknownIDIsNil(t *testing.T) {
	s := NewAlertStore(openTestDB(t))
	a, err := s.GetAlert(context.Background(), "00000000-0000-0000-0000-00000000dead")
	require.NoError(t, err)
	assert.Nil(t, a)
}

func TestAlertStore_AcknowledgeAlertReportsWhetherItAcknowledged(t *testing.T) {
	s := NewAlertStore(openTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()
	active := seedAlert(t, s, "c1", alert.StatusActive, now, nil)
	resolved := seedAlert(t, s, "c2", alert.StatusResolved, now, &now)

	done, err := s.AcknowledgeAlert(ctx, active, "alice", now)
	require.NoError(t, err)
	assert.True(t, done)

	done, err = s.AcknowledgeAlert(ctx, active, "bob", now)
	require.NoError(t, err)
	assert.False(t, done, "an alert is acknowledged once")

	done, err = s.AcknowledgeAlert(ctx, resolved, "bob", now)
	require.NoError(t, err)
	assert.False(t, done, "a resolved alert cannot be acknowledged")

	a, err := s.GetAlert(ctx, active)
	require.NoError(t, err)
	assert.Equal(t, "alice", a.AcknowledgedBy)
}

func TestAlertStore_ListUnacknowledgedActiveAlerts(t *testing.T) {
	s := NewAlertStore(openTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()
	open := seedAlert(t, s, "c1", alert.StatusActive, now, nil)
	acked := seedAlert(t, s, "c2", alert.StatusActive, now, nil)
	seedAlert(t, s, "c3", alert.StatusResolved, now, &now)
	_, err := s.AcknowledgeAlert(ctx, acked, "alice", now)
	require.NoError(t, err)
	require.NoError(t, s.SetEscalatedAt(ctx, open, now))

	alerts, err := s.ListUnacknowledgedActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	assert.Equal(t, open, alerts[0].ID, "an escalated alert is still unacknowledged")
	require.NotNil(t, alerts[0].EscalatedAt)
}

func TestAlertStore_AgentIDRoundTrips(t *testing.T) {
	s := NewAlertStore(openTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()
	const remote = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"

	id, err := s.InsertAlert(ctx, &alert.Alert{
		Source: alert.SourceContainer, AlertType: "health_unhealthy", Severity: alert.SeverityWarning,
		Status: alert.StatusActive, Message: "unhealthy", EntityType: "container", EntityID: "c1", EntityName: "web",
		FiredAt: now, AgentID: remote,
	})
	require.NoError(t, err)
	local := seedAlert(t, s, "c2", alert.StatusActive, now, nil)

	got, err := s.GetAlert(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, remote, got.AgentID)

	got, err = s.GetAlert(ctx, local)
	require.NoError(t, err)
	assert.Equal(t, uid.LocalAgent, got.AgentID, "an alert without an agent belongs to the local runtime")

	active, err := s.ListActiveAlerts(ctx)
	require.NoError(t, err)
	byID := map[string]string{}
	for _, a := range active {
		byID[a.ID] = a.AgentID
	}
	assert.Equal(t, map[string]string{id: remote, local: uid.LocalAgent}, byID)
}

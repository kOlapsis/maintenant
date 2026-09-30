// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type mcpRecordingEscalator struct {
	acked   bool
	ackedID string
}

func (m *mcpRecordingEscalator) EvaluateCycle(_ context.Context) error                  { return nil }
func (m *mcpRecordingEscalator) OnAlertCreated(_ context.Context, _ *alert.Alert) error { return nil }
func (m *mcpRecordingEscalator) OnAlertAcknowledged(_ context.Context, alertID string, _ alert.Acknowledgment) error {
	m.acked = true
	m.ackedID = alertID
	return nil
}
func (m *mcpRecordingEscalator) OnAlertResolved(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mcpRecordingEscalator) OnEditionDowngraded(_ context.Context) error { return nil }

type mcpRecordingBroadcaster struct {
	mu     sync.Mutex
	events []string
}

func (b *mcpRecordingBroadcaster) Broadcast(eventType string, _ any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, eventType)
}

type alertFixture struct {
	svc         *Services
	alerts      alert.AlertStore
	broadcaster *mcpRecordingBroadcaster
	escalator   *mcpRecordingEscalator
}

func newAlertFixture(t *testing.T) alertFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	alerts := store.NewAlertStore(db)
	b := &mcpRecordingBroadcaster{}
	eng := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alerts,
		ChannelStore: store.NewChannelStore(db),
		TriggerStore: store.NewTriggerStore(db),
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
		Broadcaster:  b,
	})
	esc := &mcpRecordingEscalator{}
	eng.SetEscalator(esc)
	return alertFixture{
		svc:         &Services{Alerts: alerts, Acknowledger: eng, Logger: logger, Version: "test"},
		alerts:      alerts,
		broadcaster: b,
		escalator:   esc,
	}
}

func (f alertFixture) insertActive(t *testing.T, entityID string) string {
	t.Helper()
	id, err := f.alerts.InsertAlert(context.Background(), &alert.Alert{
		Source: alert.SourceContainer, AlertType: "health_unhealthy", Severity: alert.SeverityWarning,
		Status: alert.StatusActive, Message: "unhealthy", EntityType: "container", EntityID: entityID,
		EntityName: entityID, FiredAt: time.Now(),
	})
	require.NoError(t, err)
	return id
}

func TestAcknowledgeAlertHandler_Success(t *testing.T) {
	f := newAlertFixture(t)
	id := f.insertActive(t, "c1")

	result, _, err := acknowledgeAlertHandler(f.svc)(context.Background(), nil, acknowledgeAlertInput{AlertID: id, AcknowledgedBy: "benjamin"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "acknowledged")

	stored, err := f.alerts.GetAlert(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, stored.AcknowledgedAt)
	assert.Equal(t, "benjamin", stored.AcknowledgedBy)
	assert.Equal(t, []string{event.AlertAcknowledged}, f.broadcaster.events, "an MCP acknowledgment must reach the interfaces like a REST one")
	assert.True(t, f.escalator.acked, "an MCP acknowledgment must stop the escalation")
	assert.Equal(t, id, f.escalator.ackedID)
}

func TestAcknowledgeAlertHandler_DefaultActor(t *testing.T) {
	f := newAlertFixture(t)
	id := f.insertActive(t, "c1")

	result, _, err := acknowledgeAlertHandler(f.svc)(context.Background(), nil, acknowledgeAlertInput{AlertID: id})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	stored, err := f.alerts.GetAlert(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "mcp", stored.AcknowledgedBy)
}

func TestAcknowledgeAlertHandler_EmptyID(t *testing.T) {
	f := newAlertFixture(t)
	result, _, err := acknowledgeAlertHandler(f.svc)(context.Background(), nil, acknowledgeAlertInput{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "invalid input")
}

func TestAcknowledgeAlertHandler_NotFound(t *testing.T) {
	f := newAlertFixture(t)
	result, _, err := acknowledgeAlertHandler(f.svc)(context.Background(), nil, acknowledgeAlertInput{AlertID: "00000000-0000-0000-0000-00000000dead"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "not found")
}

func TestAcknowledgeAlertHandler_Conflict(t *testing.T) {
	f := newAlertFixture(t)
	id := f.insertActive(t, "c1")
	_, _, err := acknowledgeAlertHandler(f.svc)(context.Background(), nil, acknowledgeAlertInput{AlertID: id})
	require.NoError(t, err)

	result, _, err := acknowledgeAlertHandler(f.svc)(context.Background(), nil, acknowledgeAlertInput{AlertID: id})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "conflict")
	assert.Len(t, f.broadcaster.events, 1, "a refused acknowledgment announces nothing")
}

func TestListAlertsHandler_ActiveLeavesOutAcknowledged(t *testing.T) {
	f := newAlertFixture(t)
	open := f.insertActive(t, "c1")
	acked := f.insertActive(t, "c2")
	_, _, err := acknowledgeAlertHandler(f.svc)(context.Background(), nil, acknowledgeAlertInput{AlertID: acked})
	require.NoError(t, err)

	result, _, err := listAlertsHandler(f.svc)(context.Background(), nil, listAlertsInput{})
	require.NoError(t, err)
	var listed []alert.Alert
	require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &listed))
	require.Len(t, listed, 1, "list_alerts must match GET /alerts/active, which leaves acknowledged alerts out")
	assert.Equal(t, open, listed[0].ID)
}

func TestListAlertsHandler_RecentStopsAtAHundred(t *testing.T) {
	f := newAlertFixture(t)
	for i := range 101 {
		f.insertActive(t, "c"+strconv.Itoa(i))
	}
	activeOnly := false

	result, _, err := listAlertsHandler(f.svc)(context.Background(), nil, listAlertsInput{ActiveOnly: &activeOnly})
	require.NoError(t, err)
	var listed []alert.Alert
	require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &listed))
	assert.Len(t, listed, 100)
}

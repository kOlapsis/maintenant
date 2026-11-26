// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

type announcedBeforeStop struct {
	recordingEscalator
	broadcaster *recordingBroadcaster
	announced   int
}

func (e *announcedBeforeStop) OnAlertAcknowledged(context.Context, string, alert.Acknowledgment) error {
	e.announced = e.broadcaster.count(event.AlertAcknowledged)
	return nil
}

func TestEngineAcknowledge_StopsTheEscalationBeforeAnnouncing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newAckFixture(t, ctx)
	escalator := &announcedBeforeStop{broadcaster: f.broadcaster, announced: -1}
	f.engine.SetEscalator(escalator)

	id, err := f.alerts.InsertAlert(ctx, &alert.Alert{
		Source: alert.SourceContainer, AlertType: "health_unhealthy", Severity: alert.SeverityWarning,
		Status: alert.StatusActive, Message: "unhealthy", EntityType: "container", EntityID: "c1",
		EntityName: "web", Details: "{}", FiredAt: time.Now(), CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	_, err = f.engine.Acknowledge(ctx, id, "alice")
	require.NoError(t, err)

	assert.Equal(t, 0, escalator.announced, "nothing is announced while the escalation still runs")
	assert.Equal(t, 1, f.broadcaster.count(event.AlertAcknowledged))
}

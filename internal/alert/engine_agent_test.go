// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

func TestEngine_AgentIDFollowsTheAlert(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	alertStore := store.NewAlertStore(db)
	eng := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: store.NewChannelStore(db),
		TriggerStore: store.NewTriggerStore(db),
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
	})
	eng.Start(ctx)

	const remote = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	event := func(entityID, agentID, severity string, recover bool) alert.Event {
		return alert.Event{
			Source:     alert.SourceContainer,
			AlertType:  "health_unhealthy",
			Severity:   severity,
			IsRecover:  recover,
			Message:    "unhealthy",
			EntityType: "container",
			EntityID:   entityID,
			EntityName: entityID,
			AgentID:    agentID,
			Timestamp:  time.Now(),
		}
	}
	activeOf := func(entityID string) *alert.Alert {
		active, err := alertStore.ListActiveAlerts(ctx)
		if err != nil {
			return nil
		}
		for _, a := range active {
			if a.EntityID == entityID {
				return a
			}
		}
		return nil
	}

	eng.EventChannel() <- event("remote-web", remote, alert.SeverityWarning, false)
	eng.EventChannel() <- event("local-web", "", alert.SeverityWarning, false)
	require.Eventually(t, func() bool { return activeOf("remote-web") != nil && activeOf("local-web") != nil },
		5*time.Second, 10*time.Millisecond)
	assert.Equal(t, remote, activeOf("remote-web").AgentID)
	assert.Equal(t, uid.LocalAgent, activeOf("local-web").AgentID, "an event without an agent belongs to the local runtime")

	eng.EventChannel() <- event("remote-web", "", alert.SeverityCritical, false)
	require.Eventually(t, func() bool {
		a := activeOf("remote-web")
		return a != nil && a.Severity == alert.SeverityCritical
	}, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, remote, activeOf("remote-web").AgentID, "a severity raise keeps the agent")

	original := activeOf("remote-web")
	eng.EventChannel() <- event("remote-web", "", alert.SeverityInfo, true)
	var resolved *alert.Alert
	require.Eventually(t, func() bool {
		a, err := alertStore.GetAlert(ctx, original.ID)
		if err != nil || a == nil || a.ResolvedByID == nil {
			return false
		}
		resolved = a
		return true
	}, 5*time.Second, 10*time.Millisecond)

	recovery, err := alertStore.GetAlert(ctx, *resolved.ResolvedByID)
	require.NoError(t, err)
	require.NotNil(t, recovery)
	assert.Equal(t, remote, recovery.AgentID, "the recovery record inherits the agent of the alert it resolves")
}

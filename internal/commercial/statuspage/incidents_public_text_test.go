// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestAutoIncident_PublishesNothingFromTheAlert(t *testing.T) {
	pinEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	incidents := store.NewIncidentStore(db)
	public := &eventRecorder{}
	svc := status.NewService(status.Deps{
		Components: components, Logger: logger, Incidents: incidents,
		PublicBroadcaster: public.broadcast,
	})
	svc.SetIncidentHandler(NewIncidentHandler(components, incidents, svc, logger))
	svc.SetMonitorStatusProvider(func(context.Context, string, string) string { return status.StatusMajorOutage })
	svc.SetSubscriberService(status.NewSubscriberService(&confirmedSubscribers{}, &fakeMailer{}, "https://status.example.com", logger))
	n := &recordingNotifier{calls: make(chan notifyCall, 8)}
	svc.SetSubscriberNotifier(n)

	ctx := context.Background()
	compID, err := components.CreateComponent(ctx, &status.Component{
		DisplayName:     "API Gateway",
		CompositionMode: status.CompositionExplicit,
		Monitors:        []status.MonitorRef{{Type: "endpoint", ID: "ep-5"}},
		Visible:         true,
		AutoIncident:    true,
	})
	require.NoError(t, err)

	evt := alert.Event{
		Source:     alert.SourceEndpoint,
		AlertType:  "http_check",
		Severity:   "critical",
		Message:    "Endpoint http://10.0.3.7:8080/health?token=abc failed 3 consecutive checks",
		EntityType: "endpoint",
		EntityID:   "ep-5",
		EntityName: "db-internal",
		Details:    map[string]any{"target": "http://10.0.3.7:8080/health?token=abc"},
	}
	svc.HandleAlertEvent(ctx, evt)
	opened := n.next(t)
	svc.HandleAlertEvent(ctx, evt)

	inc, err := incidents.GetActiveIncidentByComponent(ctx, compID)
	require.NoError(t, err)
	require.NotNil(t, inc)
	updates, err := incidents.ListUpdates(ctx, inc.ID)
	require.NoError(t, err)
	require.Len(t, updates, 2)

	published := map[string]string{
		"title":         inc.Title,
		"mail subject":  opened.subject,
		"mail body":     opened.message,
		"created event": fmt.Sprint(public.payload(event.StatusIncidentCreated)),
		"updated event": fmt.Sprint(public.payload(event.StatusIncidentUpdated)),
	}
	for i, u := range updates {
		published[fmt.Sprintf("update %d", i)] = u.Message
	}
	for where, text := range published {
		assert.NotEmpty(t, text, where)
		for _, secret := range []string{"10.0.3.7", "token=abc", "db-internal"} {
			assert.NotContains(t, text, secret, where)
		}
	}
	assert.Equal(t, "API Gateway - Major Outage", inc.Title)
	for _, u := range updates {
		assert.Equal(t, "Auto-detected: Major Outage", u.Message)
	}
	assert.Equal(t, "Auto-detected: Major Outage", public.payload(event.StatusIncidentUpdated)["message"])
}

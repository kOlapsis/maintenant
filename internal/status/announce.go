// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"
	"strings"

	"github.com/kolapsis/maintenant/internal/event"
)

// AnnounceComponentChange tells SSE clients that a component was created, updated or deleted, by id only since it may be hidden, then republishes the global status.
func (s *Service) AnnounceComponentChange(ctx context.Context, eventType, componentID string) {
	s.Broadcast(eventType, map[string]any{"component_id": componentID})
	s.broadcastGlobalStatus(ctx)
}

// AnnounceIncident pushes a newly opened incident to the public page and emails confirmed subscribers.
func (s *Service) AnnounceIncident(ctx context.Context, inc *Incident, message string) {
	names := make([]string, 0, len(inc.Components))
	for _, c := range inc.Components {
		names = append(names, c.Name)
	}
	s.Broadcast(event.StatusIncidentCreated, map[string]any{
		"id":         inc.ID,
		"title":      inc.Title,
		"severity":   inc.Severity,
		"status":     inc.Status,
		"components": names,
	})

	var body strings.Builder
	fmt.Fprintf(&body, "%s\n\nSeverity: %s\nStatus: %s\n", inc.Title, inc.Severity, inc.Status)
	if len(names) > 0 {
		fmt.Fprintf(&body, "Affected components: %s\n", strings.Join(names, ", "))
	}
	if message != "" {
		fmt.Fprintf(&body, "\n%s\n", message)
	}
	s.NotifySubscribers(ctx, "["+inc.Severity+"] "+inc.Title, body.String())
}

// AnnounceIncidentUpdate pushes an update to the public page and emails confirmed subscribers; inc is the incident as it stood before upd.
func (s *Service) AnnounceIncidentUpdate(ctx context.Context, inc *Incident, upd *IncidentUpdate) {
	if upd.Status == IncidentResolved && inc.Status != IncidentResolved {
		s.Broadcast(event.StatusIncidentResolved, map[string]any{
			"id":    inc.ID,
			"title": inc.Title,
		})
		s.NotifySubscribers(ctx, "Resolved: "+inc.Title,
			fmt.Sprintf("%s has been resolved.\n\n%s\n", inc.Title, upd.Message))
		return
	}

	s.Broadcast(event.StatusIncidentUpdated, map[string]any{
		"id":      inc.ID,
		"status":  upd.Status,
		"message": upd.Message,
	})
	s.NotifySubscribers(ctx, "Update: "+inc.Title,
		fmt.Sprintf("%s\n\nStatus: %s\n\n%s\n", inc.Title, upd.Status, upd.Message))
}

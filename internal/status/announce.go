// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/kolapsis/maintenant/internal/event"
)

// AnnounceComponentChange tells the dashboard that a component was created, updated or deleted, by id only; the public page hears of it, then of the global status, only when the component is or was visible there.
func (s *Service) AnnounceComponentChange(ctx context.Context, eventType, componentID string, public bool) {
	payload := map[string]any{"component_id": componentID}
	if !public {
		s.broadcastAdmin(eventType, payload)
		return
	}
	s.Broadcast(eventType, payload)
	s.broadcastGlobalStatus(ctx)
}

// PublicComponentNames returns the names of the linked components the public page shows.
func PublicComponentNames(refs []IncidentCompRef) []string {
	var names []string
	for _, c := range refs {
		if c.Visible {
			names = append(names, c.Name)
		}
	}
	return names
}

// BroadcastNamingComponents sends an event whose components field names every linked component to the dashboard, and only the visible ones to the public page.
func (s *Service) BroadcastNamingComponents(eventType string, data map[string]any, refs []IncidentCompRef) {
	if s.publicBroadcaster != nil {
		public := maps.Clone(data)
		public["components"] = PublicComponentNames(refs)
		s.publicBroadcaster(eventType, public)
	}
	all := make([]string, 0, len(refs))
	for _, c := range refs {
		all = append(all, c.Name)
	}
	admin := maps.Clone(data)
	admin["components"] = all
	s.broadcastAdmin(eventType, admin)
}

// AnnounceIncident pushes a newly opened incident to the public page and emails confirmed subscribers.
func (s *Service) AnnounceIncident(ctx context.Context, inc *Incident, message string) {
	s.BroadcastNamingComponents(event.StatusIncidentCreated, map[string]any{
		"id":       inc.ID,
		"title":    inc.Title,
		"severity": inc.Severity,
		"status":   inc.Status,
	}, inc.Components)

	names := PublicComponentNames(inc.Components)
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

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"context"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/status"
)

// IncidentHandler opens and resolves status page incidents from alert events.
type IncidentHandler struct {
	components status.ComponentStore
	incidents  status.IncidentStore
	service    *status.Service
	logger     *slog.Logger
}

// NewIncidentHandler returns the automatic incident handler.
func NewIncidentHandler(components status.ComponentStore, incidents status.IncidentStore, service *status.Service, logger *slog.Logger) *IncidentHandler {
	return &IncidentHandler{components: components, incidents: incidents, service: service, logger: logger}
}

// HandleAlertEvent creates, updates or resolves the incident of every auto-incident component the event touches.
func (h *IncidentHandler) HandleAlertEvent(ctx context.Context, evt alert.Event) {
	if h.incidents == nil {
		h.logger.Debug("status: no incident store, skipping alert")
		return
	}

	comps, err := h.components.ListComponentsByMonitor(ctx, evt.EntityType, evt.EntityID)
	if err != nil {
		h.logger.Error("failed to list components by monitor for alert", "error", err,
			"monitor_type", evt.EntityType, "monitor_id", evt.EntityID)
		return
	}

	for _, comp := range comps {
		if !comp.AutoIncident {
			continue
		}
		h.handleAlertForComponent(ctx, evt, &comp)
	}
}

func (h *IncidentHandler) handleAlertForComponent(ctx context.Context, evt alert.Event, comp *status.Component) {
	aggregateStatus := h.service.DeriveComponentStatus(ctx, comp)

	existing, err := h.incidents.GetActiveIncidentByComponent(ctx, comp.ID)
	if err != nil {
		h.logger.Error("failed to check active incident", "error", err, "component_id", comp.ID)
		return
	}

	// Skip if override is set.
	if comp.StatusOverride != nil {
		return
	}

	isNonOperational := aggregateStatus != status.StatusOperational

	if evt.IsRecover && !isNonOperational {
		if existing != nil {
			upd := &status.IncidentUpdate{
				IncidentID: existing.ID,
				Status:     status.IncidentResolved,
				Message:    "Auto-resolved: all monitors operational",
				IsAuto:     true,
			}
			if _, err := h.incidents.CreateUpdate(ctx, upd); err != nil {
				h.logger.Error("failed to auto-resolve incident", "error", err)
				return
			}
			h.logger.Info("status: auto-incident resolved", "incident_id", existing.ID)
			h.service.AnnounceIncidentUpdate(ctx, existing, upd)
		}
		return
	}

	if !isNonOperational {
		return
	}

	if existing != nil {
		upd := &status.IncidentUpdate{
			IncidentID: existing.ID,
			Status:     existing.Status,
			Message:    evt.Message,
			IsAuto:     true,
		}
		if _, err := h.incidents.CreateUpdate(ctx, upd); err != nil {
			h.logger.Error("failed to add auto update", "error", err)
		}
		h.service.Broadcast(event.StatusIncidentUpdated, map[string]any{
			"id":      existing.ID,
			"status":  existing.Status,
			"message": evt.Message,
		})
		return
	}

	severity := status.SeverityMinor
	switch evt.Severity {
	case "critical":
		severity = status.SeverityCritical
	case "warning":
		severity = status.SeverityMajor
	}

	inc := &status.Incident{
		Title:    comp.DisplayName + " - " + evt.Message,
		Severity: severity,
		Status:   status.IncidentInvestigating,
	}
	incID, err := h.incidents.CreateIncident(ctx, inc, []string{comp.ID}, evt.Message)
	if err != nil {
		h.logger.Error("failed to create auto incident", "error", err)
		return
	}

	h.logger.Info("status: auto-incident created", "incident_id", incID, "title", inc.Title)
	inc.ID = incID
	inc.Components = []status.IncidentCompRef{{ID: comp.ID, Name: comp.DisplayName}}
	h.service.AnnounceIncident(ctx, inc, evt.Message)
}

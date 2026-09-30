// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/status"
)

// MaintenanceScheduler polls for maintenance windows that need activation or deactivation.
type MaintenanceScheduler struct {
	maintenance status.MaintenanceStore
	components  status.ComponentStore
	incidents   status.IncidentStore
	service     *status.Service
	logger      *slog.Logger

	mu sync.Mutex
}

// NewMaintenanceScheduler creates a new scheduler.
func NewMaintenanceScheduler(
	maintenance status.MaintenanceStore,
	components status.ComponentStore,
	incidents status.IncidentStore,
	service *status.Service,
	logger *slog.Logger,
) *MaintenanceScheduler {
	return &MaintenanceScheduler{
		maintenance: maintenance,
		components:  components,
		incidents:   incidents,
		service:     service,
		logger:      logger,
	}
}

// Start begins the scheduler loop with 60-second polling interval.
func (s *MaintenanceScheduler) Start(ctx context.Context) {
	s.logger.Info("status: maintenance scheduler started")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	s.applyTransitions(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.applyTransitions(ctx)
		}
	}
}

// DeleteWindow removes a window, closing it first as its scheduled end would when it is running.
func (s *MaintenanceScheduler) DeleteWindow(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mw, err := s.maintenance.GetMaintenance(ctx, id)
	if err != nil {
		return fmt.Errorf("load maintenance window: %w", err)
	}
	if err := s.maintenance.DeleteMaintenance(ctx, id); err != nil {
		return err
	}
	if mw != nil && mw.Active {
		s.logger.Info("closing deleted maintenance window", "id", mw.ID, "title", mw.Title)
		s.closeWindow(ctx, mw)
	}
	return nil
}

func (s *MaintenanceScheduler) applyTransitions(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	// Activate pending windows
	pending, err := s.maintenance.GetPendingActivation(ctx, now)
	if err != nil {
		s.logger.Error("failed to get pending activations", "error", err)
	}
	for _, mw := range pending {
		s.activateWindow(ctx, &mw)
	}

	expired, err := s.maintenance.GetPendingDeactivation(ctx, now)
	if err != nil {
		s.logger.Error("failed to get pending deactivations", "error", err)
	}
	for _, mw := range expired {
		s.deactivateWindow(ctx, &mw)
	}

	s.logger.Debug("status: maintenance transitions", "activations", len(pending), "deactivations", len(expired))
}

func (s *MaintenanceScheduler) activateWindow(ctx context.Context, mw *status.MaintenanceWindow) {
	s.logger.Info("activating maintenance window", "id", mw.ID, "title", mw.Title)

	// Create maintenance incident
	compIDs := make([]string, 0, len(mw.Components))
	for _, c := range mw.Components {
		compIDs = append(compIDs, c.ID)
	}

	inc := &status.Incident{
		Title:               "Scheduled Maintenance: " + mw.Title,
		Severity:            status.SeverityMinor,
		Status:              status.IncidentInvestigating,
		IsMaintenance:       true,
		MaintenanceWindowID: &mw.ID,
	}

	incID, err := s.incidents.CreateIncident(ctx, inc, compIDs, mw.Description)
	if err != nil {
		s.logger.Error("failed to create maintenance incident", "error", err)
		return
	}

	// Set maintenance window as active with incident ID
	if err := s.maintenance.SetActive(ctx, mw.ID, true, &incID); err != nil {
		s.logger.Error("failed to activate maintenance window", "error", err)
		return
	}

	override := status.StatusUnderMaint
	for _, c := range mw.Components {
		comp, err := s.components.GetComponent(ctx, c.ID)
		if err != nil || comp == nil {
			continue
		}
		held, err := s.maintenance.CoveredByAnotherActiveWindow(ctx, c.ID, mw.ID)
		if err != nil {
			s.logger.Error("failed to check the other maintenance windows", "error", err, "component_id", c.ID)
			continue
		}
		if !held {
			comp.OverrideBeforeMaintenance = comp.StatusOverride
		}
		comp.StatusOverride = &override
		if err := s.components.UpdateComponent(ctx, comp); err != nil {
			s.logger.Error("failed to set component to maintenance", "error", err, "component_id", c.ID)
		}
	}

	// Broadcast
	s.service.BroadcastNamingComponents(event.StatusMaintenanceStart, map[string]any{
		"id":    mw.ID,
		"title": mw.Title,
	}, mw.Components)

	// Notify subscribers
	s.service.NotifySubscribers(ctx,
		"Maintenance Started: "+mw.Title,
		fmt.Sprintf("Scheduled maintenance has started: %s\nAffected components: %s\n%s",
			mw.Title, strings.Join(status.PublicComponentNames(mw.Components), ", "), mw.Description))
}

func (s *MaintenanceScheduler) deactivateWindow(ctx context.Context, mw *status.MaintenanceWindow) {
	s.logger.Info("deactivating maintenance window", "id", mw.ID, "title", mw.Title)

	if err := s.maintenance.SetActive(ctx, mw.ID, false, nil); err != nil {
		s.logger.Error("failed to deactivate maintenance window", "error", err)
		return
	}
	s.closeWindow(ctx, mw)
}

// closeWindow resolves the incident of a window that stopped running, gives its components back their own status, and tells visitors.
func (s *MaintenanceScheduler) closeWindow(ctx context.Context, mw *status.MaintenanceWindow) {
	if mw.IncidentID != nil {
		update := &status.IncidentUpdate{
			IncidentID: *mw.IncidentID,
			Status:     status.IncidentResolved,
			Message:    "Scheduled maintenance completed",
			IsAuto:     true,
		}
		if _, err := s.incidents.CreateUpdate(ctx, update); err != nil {
			s.logger.Error("failed to resolve maintenance incident", "error", err)
		}
	}

	for _, c := range mw.Components {
		s.releaseComponent(ctx, mw.ID, c.ID)
	}

	// Broadcast
	s.service.BroadcastNamingComponents(event.StatusMaintenanceEnd, map[string]any{
		"id":    mw.ID,
		"title": mw.Title,
	}, mw.Components)

	// Notify subscribers
	s.service.NotifySubscribers(ctx,
		"Maintenance Completed: "+mw.Title,
		"Scheduled maintenance has been completed: "+mw.Title)
}

// releaseComponent puts back the override a component had before maintenance, once no running window holds it any more.
func (s *MaintenanceScheduler) releaseComponent(ctx context.Context, windowID, componentID string) {
	held, err := s.maintenance.CoveredByAnotherActiveWindow(ctx, componentID, windowID)
	if err != nil {
		s.logger.Error("failed to check the other maintenance windows", "error", err, "component_id", componentID)
		return
	}
	if held {
		return
	}
	comp, err := s.components.GetComponent(ctx, componentID)
	if err != nil || comp == nil {
		return
	}
	forced := comp.StatusOverride != nil && *comp.StatusOverride == status.StatusUnderMaint
	if !forced && comp.OverrideBeforeMaintenance == nil {
		return
	}
	if forced {
		comp.StatusOverride = comp.OverrideBeforeMaintenance
	}
	comp.OverrideBeforeMaintenance = nil
	if err := s.components.UpdateComponent(ctx, comp); err != nil {
		s.logger.Error("failed to restore component status after maintenance", "error", err, "component_id", componentID)
	}
}

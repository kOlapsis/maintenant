// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package status

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
)

// MaintenanceScheduler polls for maintenance windows that need activation or deactivation.
type MaintenanceScheduler struct {
	maintenance MaintenanceStore
	components  ComponentStore
	incidents   IncidentStore
	service     *Service
	logger      *slog.Logger
}

// NewMaintenanceScheduler creates a new scheduler.
func NewMaintenanceScheduler(
	maintenance MaintenanceStore,
	components ComponentStore,
	incidents IncidentStore,
	service *Service,
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

func (s *MaintenanceScheduler) applyTransitions(ctx context.Context) {
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

func (s *MaintenanceScheduler) activateWindow(ctx context.Context, mw *MaintenanceWindow) {
	s.logger.Info("activating maintenance window", "id", mw.ID, "title", mw.Title)

	// Create maintenance incident
	compIDs := make([]int64, 0, len(mw.Components))
	compNames := make([]string, 0, len(mw.Components))
	for _, c := range mw.Components {
		compIDs = append(compIDs, c.ID)
		compNames = append(compNames, c.Name)
	}

	inc := &Incident{
		Title:               "Scheduled Maintenance: " + mw.Title,
		Severity:            SeverityMinor,
		Status:              IncidentInvestigating,
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

	// Set affected components to under_maintenance
	override := StatusUnderMaint
	for _, c := range mw.Components {
		comp, err := s.components.GetComponent(ctx, c.ID)
		if err != nil || comp == nil {
			continue
		}
		comp.StatusOverride = &override
		if err := s.components.UpdateComponent(ctx, comp); err != nil {
			s.logger.Error("failed to set component to maintenance", "error", err, "component_id", c.ID)
		}
	}

	// Broadcast
	s.service.broadcast(event.StatusMaintenanceStart, map[string]interface{}{
		"id":         mw.ID,
		"title":      mw.Title,
		"components": compNames,
	})

	// Notify subscribers
	s.service.notifySubscribers(ctx,
		"Maintenance Started: "+mw.Title,
		fmt.Sprintf("Scheduled maintenance has started: %s\nAffected components: %s\n%s",
			mw.Title, strings.Join(compNames, ", "), mw.Description))
}

func (s *MaintenanceScheduler) deactivateWindow(ctx context.Context, mw *MaintenanceWindow) {
	s.logger.Info("deactivating maintenance window", "id", mw.ID, "title", mw.Title)

	// Resolve the maintenance incident
	if mw.IncidentID != nil {
		update := &IncidentUpdate{
			IncidentID: *mw.IncidentID,
			Status:     IncidentResolved,
			Message:    "Scheduled maintenance completed",
			IsAuto:     true,
		}
		if _, err := s.incidents.CreateUpdate(ctx, update); err != nil {
			s.logger.Error("failed to resolve maintenance incident", "error", err)
		}
	}

	// Deactivate the window
	if err := s.maintenance.SetActive(ctx, mw.ID, false, nil); err != nil {
		s.logger.Error("failed to deactivate maintenance window", "error", err)
		return
	}

	// Clear component overrides
	compNames := make([]string, 0, len(mw.Components))
	for _, c := range mw.Components {
		compNames = append(compNames, c.Name)
		comp, err := s.components.GetComponent(ctx, c.ID)
		if err != nil || comp == nil {
			continue
		}
		if comp.StatusOverride != nil && *comp.StatusOverride == StatusUnderMaint {
			comp.StatusOverride = nil
			if err := s.components.UpdateComponent(ctx, comp); err != nil {
				s.logger.Error("failed to clear component maintenance override", "error", err, "component_id", c.ID)
			}
		}
	}

	// Broadcast
	s.service.broadcast(event.StatusMaintenanceEnd, map[string]interface{}{
		"id":         mw.ID,
		"title":      mw.Title,
		"components": compNames,
	})

	// Notify subscribers
	s.service.notifySubscribers(ctx,
		"Maintenance Completed: "+mw.Title,
		"Scheduled maintenance has been completed: "+mw.Title)
}

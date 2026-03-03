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
	"log/slog"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

// MonitorStatusProvider resolves the current health status of a specific monitor.
type MonitorStatusProvider func(ctx context.Context, monitorType string, monitorID int64) string

// Service encapsulates public status page business logic.
type Service struct {
	components  ComponentStore
	incidents   IncidentStore
	maintenance MaintenanceStore

	monitorStatus MonitorStatusProvider
	broadcaster   func(eventType string, data interface{})
	subscribers   *SubscriberService
	smtpConfig    *SmtpConfig

	logger *slog.Logger
}

// NewService creates a new status page service.
func NewService(
	components ComponentStore,
	logger *slog.Logger,
) *Service {
	return &Service{
		components: components,
		logger:     logger,
	}
}

// SetMonitorStatusProvider sets the function used to derive component status from monitors.
func (s *Service) SetMonitorStatusProvider(fn MonitorStatusProvider) {
	s.monitorStatus = fn
}

// SetBroadcaster sets the function used to broadcast SSE events.
func (s *Service) SetBroadcaster(fn func(eventType string, data interface{})) {
	s.broadcaster = fn
}

// SetIncidentStore sets the incident store used by the feed handler.
func (s *Service) SetIncidentStore(store IncidentStore) {
	s.incidents = store
}

// SetSubscriberService sets the subscriber service used for notifications.
func (s *Service) SetSubscriberService(sub *SubscriberService) {
	s.subscribers = sub
}

// SetMaintenanceStore sets the maintenance store used by GetPageData.
func (s *Service) SetMaintenanceStore(store MaintenanceStore) {
	s.maintenance = store
}

// GetSmtpConfig returns the current SMTP configuration.
func (s *Service) GetSmtpConfig() *SmtpConfig {
	return s.smtpConfig
}

// SetSmtpConfig updates the SMTP configuration.
func (s *Service) SetSmtpConfig(cfg *SmtpConfig) {
	s.smtpConfig = cfg
}

// notifySubscribers sends a notification to all confirmed subscribers if configured.
func (s *Service) notifySubscribers(ctx context.Context, subject, message string) {
	if s.subscribers != nil {
		go s.subscribers.NotifyAll(ctx, subject, message)
	}
}

// broadcast sends an event if a broadcaster is configured.
func (s *Service) broadcast(eventType string, data interface{}) {
	if s.broadcaster != nil {
		s.broadcaster(eventType, data)
	}
}

// --- Status Derivation ---

// DeriveComponentStatus computes the effective status for a single component.
func (s *Service) DeriveComponentStatus(ctx context.Context, c *Component) string {
	if c.StatusOverride != nil {
		return *c.StatusOverride
	}
	if s.monitorStatus != nil {
		derived := s.monitorStatus(ctx, c.MonitorType, c.MonitorID)
		if derived != "" {
			return derived
		}
	}
	return StatusOperational
}

// Severity returns a numeric severity for status comparison (higher = worse).
func Severity(s string) int {
	return statusSeverity(s)
}

func statusSeverity(s string) int {
	switch s {
	case StatusMajorOutage:
		return 4
	case StatusUnderMaint:
		return 3
	case StatusPartialOutage:
		return 2
	case StatusDegraded:
		return 1
	default:
		return 0
	}
}

// ComputeGlobalStatus derives the global status from all visible components.
func (s *Service) ComputeGlobalStatus(ctx context.Context) (string, string) {
	components, err := s.components.ListVisibleComponents(ctx)
	if err != nil {
		s.logger.Error("failed to list visible components for global status", "error", err)
		return StatusOperational, GlobalAllOperational
	}

	worst := StatusOperational
	for _, c := range components {
		effective := s.DeriveComponentStatus(ctx, &c)
		if statusSeverity(effective) > statusSeverity(worst) {
			worst = effective
		}
	}

	switch worst {
	case StatusMajorOutage:
		return worst, GlobalMajorOutage
	case StatusPartialOutage:
		return worst, GlobalPartialOutage
	case StatusDegraded:
		return worst, GlobalDegraded
	case StatusUnderMaint:
		return worst, GlobalMaintenance
	default:
		return StatusOperational, GlobalAllOperational
	}
}

// PageData holds all data needed to render the public status page.
type PageData struct {
	GlobalStatus    string
	GlobalMessage   string
	Groups          []GroupData
	Ungrouped       []ComponentData
	ActiveIncidents []Incident
	RecentIncidents []Incident
	Maintenance     []MaintenanceWindow
}

// GroupData holds a component group with its components for rendering.
type GroupData struct {
	Name       string
	Components []ComponentData
}

// ComponentData holds a component with its effective status for rendering.
type ComponentData struct {
	ID              int64
	DisplayName     string
	EffectiveStatus string
	StatusLabel     string
}

func statusLabel(s string) string {
	switch s {
	case StatusOperational:
		return "Operational"
	case StatusDegraded:
		return "Degraded Performance"
	case StatusPartialOutage:
		return "Partial Outage"
	case StatusMajorOutage:
		return "Major Outage"
	case StatusUnderMaint:
		return "Under Maintenance"
	default:
		return "Unknown"
	}
}

// GetPageData assembles all data for the public status page.
func (s *Service) GetPageData(ctx context.Context) (*PageData, error) {
	globalStatus, globalMsg := s.ComputeGlobalStatus(ctx)

	components, err := s.components.ListVisibleComponents(ctx)
	if err != nil {
		return nil, err
	}

	groupMap := make(map[string]*GroupData)
	var groupOrder []string
	var ungrouped []ComponentData

	for i := range components {
		c := &components[i]
		effective := s.DeriveComponentStatus(ctx, c)
		cd := ComponentData{
			ID:              c.ID,
			DisplayName:     c.DisplayName,
			EffectiveStatus: effective,
			StatusLabel:     statusLabel(effective),
		}

		if c.GroupName != "" {
			if _, ok := groupMap[c.GroupName]; !ok {
				groupMap[c.GroupName] = &GroupData{Name: c.GroupName}
				groupOrder = append(groupOrder, c.GroupName)
			}
			groupMap[c.GroupName].Components = append(groupMap[c.GroupName].Components, cd)
		} else {
			ungrouped = append(ungrouped, cd)
		}
	}

	var groups []GroupData
	for _, name := range groupOrder {
		groups = append(groups, *groupMap[name])
	}

	pd := &PageData{
		GlobalStatus:  globalStatus,
		GlobalMessage: globalMsg,
		Groups:        groups,
		Ungrouped:     ungrouped,
	}

	if s.incidents != nil {
		active, err := s.incidents.ListActiveIncidents(ctx)
		if err != nil {
			s.logger.Error("failed to list active incidents", "error", err)
		} else {
			pd.ActiveIncidents = active
		}

		recent, err := s.incidents.ListRecentIncidents(ctx, 7)
		if err != nil {
			s.logger.Error("failed to list recent incidents", "error", err)
		} else {
			pd.RecentIncidents = recent
		}
	}

	if s.maintenance != nil {
		maint, err := s.maintenance.ListMaintenance(ctx, "upcoming", 5)
		if err != nil {
			s.logger.Error("failed to list upcoming maintenance", "error", err)
		} else {
			pd.Maintenance = maint
		}
	}

	return pd, nil
}

// NotifyMonitorChanged checks whether a status component is linked to the given
// monitor and, if so, broadcasts the updated status to public SSE clients.
// It also notifies any global components (monitor_id=0) of the same type.
func (s *Service) NotifyMonitorChanged(ctx context.Context, monitorType string, monitorID int64) {
	comp, err := s.components.GetComponentByMonitor(ctx, monitorType, monitorID)
	if err == nil && comp != nil {
		s.BroadcastComponentChange(ctx, comp)
	}
	// Also notify global components (monitor_id=0) that aggregate all monitors of this type.
	globals, err := s.components.ListGlobalComponents(ctx, monitorType)
	if err == nil {
		for i := range globals {
			s.BroadcastComponentChange(ctx, &globals[i])
		}
	}
}

// HandleAlertEvent processes an alert event and creates/updates incidents for auto-incident components.
func (s *Service) HandleAlertEvent(ctx context.Context, evt alert.Event) {
	if s.incidents == nil {
		s.logger.Debug("status: no incident store, skipping alert")
		return
	}

	monitorType := evt.EntityType
	monitorID := evt.EntityID

	comp, err := s.components.GetComponentByMonitor(ctx, monitorType, monitorID)
	if err != nil || comp == nil || !comp.AutoIncident {
		s.logger.Debug("status: no auto-incident component", "monitor_type", monitorType, "monitor_id", monitorID)
		return
	}

	existing, err := s.incidents.GetActiveIncidentByComponent(ctx, comp.ID)
	if err != nil {
		s.logger.Error("failed to check active incident", "error", err, "component_id", comp.ID)
		return
	}

	if evt.IsRecover {
		if existing != nil {
			upd := &IncidentUpdate{
				IncidentID: existing.ID,
				Status:     IncidentResolved,
				Message:    "Auto-resolved: " + evt.Message,
				IsAuto:     true,
			}
			if _, err := s.incidents.CreateUpdate(ctx, upd); err != nil {
				s.logger.Error("failed to auto-resolve incident", "error", err)
				return
			}
			s.logger.Info("status: auto-incident resolved", "incident_id", existing.ID, "title", existing.Title)
			s.broadcast(event.StatusIncidentResolved, map[string]interface{}{
				"id":    existing.ID,
				"title": existing.Title,
			})
			s.notifySubscribers(ctx, "Resolved: "+existing.Title,
				"<p>Incident <strong>"+existing.Title+"</strong> has been resolved.</p><p>"+evt.Message+"</p>")
		}
		return
	}

	if existing != nil {
		upd := &IncidentUpdate{
			IncidentID: existing.ID,
			Status:     existing.Status,
			Message:    evt.Message,
			IsAuto:     true,
		}
		if _, err := s.incidents.CreateUpdate(ctx, upd); err != nil {
			s.logger.Error("failed to add auto update", "error", err)
		}
		s.broadcast(event.StatusIncidentUpdated, map[string]interface{}{
			"id":      existing.ID,
			"status":  existing.Status,
			"message": evt.Message,
		})
		return
	}

	severity := SeverityMinor
	switch evt.Severity {
	case "critical":
		severity = SeverityCritical
	case "warning":
		severity = SeverityMajor
	}

	inc := &Incident{
		Title:    comp.DisplayName + " - " + evt.Message,
		Severity: severity,
		Status:   IncidentInvestigating,
	}
	incID, err := s.incidents.CreateIncident(ctx, inc, []int64{comp.ID}, evt.Message)
	if err != nil {
		s.logger.Error("failed to create auto incident", "error", err)
		return
	}

	s.logger.Info("status: auto-incident created", "incident_id", incID, "title", inc.Title, "severity", inc.Severity)

	s.broadcast(event.StatusIncidentCreated, map[string]interface{}{
		"id":         incID,
		"title":      inc.Title,
		"severity":   inc.Severity,
		"status":     inc.Status,
		"components": []string{comp.DisplayName},
	})

	s.notifySubscribers(ctx, "["+inc.Severity+"] "+inc.Title,
		"<p><strong>"+inc.Title+"</strong></p><p>Severity: "+inc.Severity+"</p><p>"+evt.Message+"</p>")
}

// BroadcastComponentChange notifies public SSE clients of a component status change.
func (s *Service) BroadcastComponentChange(ctx context.Context, comp *Component) {
	effective := s.DeriveComponentStatus(ctx, comp)
	s.broadcast(event.StatusComponentChanged, map[string]interface{}{
		"component_id": comp.ID,
		"name":         comp.DisplayName,
		"status":       effective,
		"group":        comp.GroupName,
	})

	globalStatus, globalMsg := s.ComputeGlobalStatus(ctx)
	s.broadcast(event.StatusGlobalChanged, map[string]interface{}{
		"status":  globalStatus,
		"message": globalMsg,
	})
}

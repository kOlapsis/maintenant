// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
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

// MonitorPopulationProvider returns all monitor refs of a given type (used by match-all components).
type MonitorPopulationProvider func(ctx context.Context, monitorType string) []MonitorRef

// MonitorNameProvider resolves the display name of a specific monitor.
type MonitorNameProvider func(ctx context.Context, monitorType string, monitorID int64) string

// Deps holds all dependencies for the status Service.
type Deps struct {
	Components       ComponentStore        // required
	Logger           *slog.Logger          // required
	Incidents        IncidentStore         // optional — nil-safe
	Maintenance      MaintenanceStore      // optional — nil-safe
	MonitorStatus    MonitorStatusProvider // optional — nil-safe
	MonitorPopulation MonitorPopulationProvider // optional — nil-safe
	MonitorName      MonitorNameProvider   // optional — nil-safe
	Broadcaster      func(eventType string, data any) // optional — nil-safe
	Subscribers      *SubscriberService    // optional — nil-safe
}

// Service encapsulates public status page business logic.
type Service struct {
	components  ComponentStore
	incidents   IncidentStore
	maintenance MaintenanceStore

	monitorStatus     MonitorStatusProvider
	monitorPopulation MonitorPopulationProvider
	monitorName       MonitorNameProvider
	broadcaster       func(eventType string, data any)
	subscribers       *SubscriberService
	smtpConfig        *SmtpConfig

	logger *slog.Logger
}

// NewService creates a new status page service.
func NewService(d Deps) *Service {
	if d.Components == nil {
		panic("status.NewService: Components is required")
	}
	if d.Logger == nil {
		panic("status.NewService: Logger is required")
	}
	return &Service{
		components:        d.Components,
		logger:            d.Logger,
		incidents:         d.Incidents,
		maintenance:       d.Maintenance,
		monitorStatus:     d.MonitorStatus,
		monitorPopulation: d.MonitorPopulation,
		monitorName:       d.MonitorName,
		broadcaster:       d.Broadcaster,
		subscribers:       d.Subscribers,
	}
}

// SetMonitorStatusProvider sets the function used to derive component status from monitors.
func (s *Service) SetMonitorStatusProvider(fn MonitorStatusProvider) {
	s.monitorStatus = fn
}

// SetMonitorPopulationProvider sets the function used to enumerate all monitors of a given type.
func (s *Service) SetMonitorPopulationProvider(fn MonitorPopulationProvider) {
	s.monitorPopulation = fn
}

// SetMonitorNameProvider sets the function used to resolve monitor display names.
func (s *Service) SetMonitorNameProvider(fn MonitorNameProvider) {
	s.monitorName = fn
}

// SetBroadcaster sets the function used to broadcast SSE events.
func (s *Service) SetBroadcaster(fn func(eventType string, data any)) {
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
func (s *Service) broadcast(eventType string, data any) {
	if s.broadcaster != nil {
		s.broadcaster(eventType, data)
	}
}

// --- Status Derivation ---

// ComputeAggregateStatus applies the fractional aggregation rule over a list of monitor states.
// Empty list → operational (vacuous case).
// All major_outage → major_outage.
// Any major_outage + any non-major → partial_outage.
// No major_outage, any partial_outage → partial_outage (pass-through).
// No major_outage, no partial_outage, any degraded → degraded.
// All operational (or empty string treated as operational) → operational.
func ComputeAggregateStatus(states []string) string {
	if len(states) == 0 {
		return StatusOperational
	}
	major := 0
	hasPartial := false
	hasNonOperational := false
	total := len(states)
	for _, st := range states {
		switch st {
		case StatusMajorOutage:
			major++
			hasNonOperational = true
		case StatusPartialOutage:
			hasPartial = true
			hasNonOperational = true
		case StatusDegraded, StatusUnderMaint:
			hasNonOperational = true
		}
	}
	if major == total {
		return StatusMajorOutage
	}
	if major > 0 {
		return StatusPartialOutage
	}
	if hasPartial {
		return StatusPartialOutage
	}
	if hasNonOperational {
		return StatusDegraded
	}
	return StatusOperational
}

// DeriveComponentStatus computes the effective status for a single component.
func (s *Service) DeriveComponentStatus(ctx context.Context, c *Component) string {
	if c.StatusOverride != nil {
		return *c.StatusOverride
	}

	var states []string

	switch c.CompositionMode {
	case CompositionMatchAll:
		if s.monitorPopulation != nil {
			refs := s.monitorPopulation(ctx, c.MatchAllType)
			for _, ref := range refs {
				if s.monitorStatus != nil {
					st := s.monitorStatus(ctx, ref.Type, ref.ID)
					states = append(states, st)
				}
			}
		}
	default: // explicit (and empty/legacy)
		if len(c.Monitors) == 0 {
			c.NeedsAttention = true
			return StatusOperational
		}
		for _, ref := range c.Monitors {
			if s.monitorStatus != nil {
				st := s.monitorStatus(ctx, ref.Type, ref.ID)
				states = append(states, st)
			}
		}
	}

	return ComputeAggregateStatus(states)
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
	Components      []ComponentData
	ActiveIncidents []Incident
	RecentIncidents []Incident
	Maintenance     []MaintenanceWindow
}

// ComponentData holds a component with its effective status for rendering.
type ComponentData struct {
	ID              int64
	DisplayName     string
	EffectiveStatus string
	StatusLabel     string
	Monitors        []MonitorRef
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

	var compData []ComponentData

	for i := range components {
		c := &components[i]
		// Skip components that need attention (no monitors configured).
		if c.NeedsAttention {
			continue
		}

		effective := s.DeriveComponentStatus(ctx, c)

		// Build per-monitor status breakdown.
		var monitorRefs []MonitorRef
		if c.CompositionMode == CompositionExplicit {
			for _, ref := range c.Monitors {
				mr := MonitorRef{Type: ref.Type, ID: ref.ID, Name: ref.Name}
				if s.monitorStatus != nil {
					mr.Status = s.monitorStatus(ctx, ref.Type, ref.ID)
				}
				monitorRefs = append(monitorRefs, mr)
			}
		} else if c.CompositionMode == CompositionMatchAll && s.monitorPopulation != nil {
			refs := s.monitorPopulation(ctx, c.MatchAllType)
			for _, ref := range refs {
				mr := MonitorRef{Type: ref.Type, ID: ref.ID, Name: ref.Name}
				if s.monitorStatus != nil {
					mr.Status = s.monitorStatus(ctx, ref.Type, ref.ID)
				}
				monitorRefs = append(monitorRefs, mr)
			}
		}

		compData = append(compData, ComponentData{
			ID:              c.ID,
			DisplayName:     c.DisplayName,
			EffectiveStatus: effective,
			StatusLabel:     statusLabel(effective),
			Monitors:        monitorRefs,
		})
	}

	pd := &PageData{
		GlobalStatus:  globalStatus,
		GlobalMessage: globalMsg,
		Components:    compData,
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

// NotifyMonitorChanged checks whether any status components are linked to the
// given monitor and, if so, broadcasts updated statuses to public SSE clients.
func (s *Service) NotifyMonitorChanged(ctx context.Context, monitorType string, monitorID int64) {
	comps, err := s.components.ListComponentsByMonitor(ctx, monitorType, monitorID)
	if err != nil {
		s.logger.Error("failed to list components by monitor", "error", err,
			"monitor_type", monitorType, "monitor_id", monitorID)
		return
	}
	for i := range comps {
		s.BroadcastComponentChange(ctx, &comps[i])
	}
}

// HandleAlertEvent processes an alert event and creates/updates incidents for auto-incident components.
func (s *Service) HandleAlertEvent(ctx context.Context, evt alert.Event) {
	if s.incidents == nil {
		s.logger.Debug("status: no incident store, skipping alert")
		return
	}

	comps, err := s.components.ListComponentsByMonitor(ctx, evt.EntityType, evt.EntityID)
	if err != nil {
		s.logger.Error("failed to list components by monitor for alert", "error", err,
			"monitor_type", evt.EntityType, "monitor_id", evt.EntityID)
		return
	}

	for _, comp := range comps {
		if !comp.AutoIncident {
			continue
		}
		s.handleAlertForComponent(ctx, evt, &comp)
	}
}

func (s *Service) handleAlertForComponent(ctx context.Context, evt alert.Event, comp *Component) {
	aggregateStatus := s.DeriveComponentStatus(ctx, comp)

	existing, err := s.incidents.GetActiveIncidentByComponent(ctx, comp.ID)
	if err != nil {
		s.logger.Error("failed to check active incident", "error", err, "component_id", comp.ID)
		return
	}

	// Skip if override is set.
	if comp.StatusOverride != nil {
		return
	}

	isNonOperational := aggregateStatus != StatusOperational

	if evt.IsRecover && !isNonOperational {
		if existing != nil {
			upd := &IncidentUpdate{
				IncidentID: existing.ID,
				Status:     IncidentResolved,
				Message:    "Auto-resolved: all monitors operational",
				IsAuto:     true,
			}
			if _, err := s.incidents.CreateUpdate(ctx, upd); err != nil {
				s.logger.Error("failed to auto-resolve incident", "error", err)
				return
			}
			s.logger.Info("status: auto-incident resolved", "incident_id", existing.ID)
			s.broadcast(event.StatusIncidentResolved, map[string]any{
				"id":    existing.ID,
				"title": existing.Title,
			})
			s.notifySubscribers(ctx, "Resolved: "+existing.Title,
				"<p>Incident <strong>"+existing.Title+"</strong> has been resolved.</p>")
		}
		return
	}

	if !isNonOperational {
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
		s.broadcast(event.StatusIncidentUpdated, map[string]any{
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

	s.logger.Info("status: auto-incident created", "incident_id", incID, "title", inc.Title)
	s.broadcast(event.StatusIncidentCreated, map[string]any{
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

	var monitorsWithStatus []map[string]any
	if comp.CompositionMode == CompositionExplicit && len(comp.Monitors) > 0 {
		for _, ref := range comp.Monitors {
			m := map[string]any{
				"type": ref.Type,
				"id":   ref.ID,
				"name": ref.Name,
			}
			if s.monitorStatus != nil {
				m["status"] = s.monitorStatus(ctx, ref.Type, ref.ID)
			}
			monitorsWithStatus = append(monitorsWithStatus, m)
		}
	} else if comp.CompositionMode == CompositionMatchAll && s.monitorPopulation != nil {
		refs := s.monitorPopulation(ctx, comp.MatchAllType)
		for _, ref := range refs {
			m := map[string]any{
				"type": ref.Type,
				"id":   ref.ID,
				"name": ref.Name,
			}
			if s.monitorStatus != nil {
				m["status"] = s.monitorStatus(ctx, ref.Type, ref.ID)
			}
			monitorsWithStatus = append(monitorsWithStatus, m)
		}
	}

	s.broadcast(event.StatusComponentChanged, map[string]any{
		"component_id": comp.ID,
		"name":         comp.DisplayName,
		"status":       effective,
		"monitors":     monitorsWithStatus,
	})

	globalStatus, globalMsg := s.ComputeGlobalStatus(ctx)
	s.broadcast(event.StatusGlobalChanged, map[string]any{
		"status":  globalStatus,
		"message": globalMsg,
	})
}

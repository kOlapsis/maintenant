// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

// MonitorStatusProvider resolves the current health status of a specific monitor.
type MonitorStatusProvider func(ctx context.Context, monitorType string, monitorID string) string

// MonitorPopulationProvider returns all monitor refs of a given type (used by match-all components).
type MonitorPopulationProvider func(ctx context.Context, monitorType string) []MonitorRef

// MonitorNameProvider resolves the display name of a specific monitor.
type MonitorNameProvider func(ctx context.Context, monitorType string, monitorID string) string

// Deps holds all dependencies for the status Service.
type Deps struct {
	Components        ComponentStore                   // required
	Logger            *slog.Logger                     // required
	Incidents         IncidentStore                    // optional — nil-safe
	Maintenance       MaintenanceStore                 // optional — nil-safe
	MonitorStatus     MonitorStatusProvider            // optional — nil-safe
	MonitorPopulation MonitorPopulationProvider        // optional — nil-safe
	MonitorName       MonitorNameProvider              // optional — nil-safe
	PublicBroadcaster func(eventType string, data any) // optional, nil-safe
	AdminBroadcaster  func(eventType string, data any) // optional, nil-safe
	Subscribers       *SubscriberService               // optional — nil-safe
}

// Service encapsulates public status page business logic.
type Service struct {
	components  ComponentStore
	incidents   IncidentStore
	maintenance MaintenanceStore

	monitorStatus     MonitorStatusProvider
	monitorPopulation MonitorPopulationProvider
	monitorName       MonitorNameProvider
	publicBroadcaster func(eventType string, data any)
	adminBroadcaster  func(eventType string, data any)
	subscribers       *SubscriberService
	notifier          SubscriberNotifier
	incidentHandler   AlertIncidentHandler

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
		publicBroadcaster: d.PublicBroadcaster,
		adminBroadcaster:  d.AdminBroadcaster,
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

// SetSubscriberNotifier sets what notifies subscribers of status changes.
func (s *Service) SetSubscriberNotifier(n SubscriberNotifier) {
	s.notifier = n
}

// SetIncidentHandler sets what turns alert events into status page incidents.
func (s *Service) SetIncidentHandler(h AlertIncidentHandler) {
	s.incidentHandler = h
}

// SubscriptionsEnabled reports whether visitors can subscribe to email updates and subscribers get them.
func (s *Service) SubscriptionsEnabled() bool {
	return s.subscribers.Enabled()
}

// NotifySubscribers emails a plain-text message to every confirmed subscriber, in the background, when subscriptions are enabled.
func (s *Service) NotifySubscribers(ctx context.Context, subject, message string) {
	if s.notifier == nil || !s.SubscriptionsEnabled() {
		return
	}
	go s.notifier.NotifyAll(context.WithoutCancel(ctx), subject, message)
}

// Broadcast sends an event to the public page and to the dashboard.
func (s *Service) Broadcast(eventType string, data any) {
	if s.publicBroadcaster != nil {
		s.publicBroadcaster(eventType, data)
	}
	s.broadcastAdmin(eventType, data)
}

func (s *Service) broadcastAdmin(eventType string, data any) {
	if s.adminBroadcaster != nil {
		s.adminBroadcaster(eventType, data)
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
	Maintenance     []MaintenanceWindow
}

// ComponentData holds a component with its effective status for rendering.
type ComponentData struct {
	ID              string
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

		compData = append(compData, ComponentData{
			ID:              c.ID,
			DisplayName:     c.DisplayName,
			EffectiveStatus: effective,
			StatusLabel:     statusLabel(effective),
			Monitors:        s.componentMonitors(ctx, c),
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

// NotifyMonitorChanged broadcasts the updated status of every component linked to the given monitor.
func (s *Service) NotifyMonitorChanged(ctx context.Context, monitorType string, monitorID string) {
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

// HandleAlertEvent hands an alert event to the incident handler, if one is set.
func (s *Service) HandleAlertEvent(ctx context.Context, evt alert.Event) {
	if s.incidentHandler != nil {
		s.incidentHandler.HandleAlertEvent(ctx, evt)
	}
}

// BroadcastComponentChange sends a component's status and monitors to the dashboard, and to the public page only when the component is visible there.
func (s *Service) BroadcastComponentChange(ctx context.Context, comp *Component) {
	payload := map[string]any{
		"component_id": comp.ID,
		"name":         comp.DisplayName,
		"status":       s.DeriveComponentStatus(ctx, comp),
		"monitors":     s.componentMonitors(ctx, comp),
	}
	if !comp.Visible {
		s.broadcastAdmin(event.StatusComponentChanged, payload)
		return
	}
	s.Broadcast(event.StatusComponentChanged, payload)
	s.broadcastGlobalStatus(ctx)
}

// componentMonitors lists the monitors a component aggregates, each with its current status.
func (s *Service) componentMonitors(ctx context.Context, c *Component) []MonitorRef {
	var refs []MonitorRef
	switch {
	case c.CompositionMode == CompositionExplicit:
		refs = c.Monitors
	case c.CompositionMode == CompositionMatchAll && s.monitorPopulation != nil:
		refs = s.monitorPopulation(ctx, c.MatchAllType)
	}
	var out []MonitorRef
	for _, ref := range refs {
		mr := MonitorRef{Type: ref.Type, ID: ref.ID, Name: ref.Name}
		if s.monitorStatus != nil {
			mr.Status = s.monitorStatus(ctx, ref.Type, ref.ID)
		}
		out = append(out, mr)
	}
	return out
}

func (s *Service) broadcastGlobalStatus(ctx context.Context) {
	globalStatus, globalMsg := s.ComputeGlobalStatus(ctx)
	s.Broadcast(event.StatusGlobalChanged, map[string]any{
		"status":  globalStatus,
		"message": globalMsg,
	})
}

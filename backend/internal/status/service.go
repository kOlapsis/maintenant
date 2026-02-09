package status

import (
	"context"
	"log/slog"
)

// MonitorStatusProvider resolves the current health status of a specific monitor.
type MonitorStatusProvider func(ctx context.Context, monitorType string, monitorID int64) string

// Service encapsulates public status page business logic.
type Service struct {
	components ComponentStore

	monitorStatus MonitorStatusProvider
	broadcaster   func(eventType string, data interface{})

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

// broadcast sends an event if a broadcaster is configured.
func (s *Service) broadcast(eventType string, data interface{}) {
	if s.broadcaster != nil {
		s.broadcaster(eventType, data)
	}
}

// --- Status Derivation ---

// DeriveComponentStatus computes the effective status for a single component.
func (s *Service) DeriveComponentStatus(ctx context.Context, c *StatusComponent) string {
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

// statusSeverity returns a numeric severity for status comparison (higher = worse).
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
	GlobalStatus  string
	GlobalMessage string
	Groups        []GroupData
	Ungrouped     []ComponentData
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

	return &PageData{
		GlobalStatus:  globalStatus,
		GlobalMessage: globalMsg,
		Groups:        groups,
		Ungrouped:     ungrouped,
	}, nil
}

// BroadcastComponentChange notifies public SSE clients of a component status change.
func (s *Service) BroadcastComponentChange(ctx context.Context, comp *StatusComponent) {
	effective := s.DeriveComponentStatus(ctx, comp)
	s.broadcast("status.component_changed", map[string]interface{}{
		"component_id": comp.ID,
		"name":         comp.DisplayName,
		"status":       effective,
		"group":        comp.GroupName,
	})

	globalStatus, globalMsg := s.ComputeGlobalStatus(ctx)
	s.broadcast("status.global_changed", map[string]interface{}{
		"status":  globalStatus,
		"message": globalMsg,
	})
}

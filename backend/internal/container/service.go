package container

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// RuntimeDiscoverer abstracts container/workload discovery operations.
type RuntimeDiscoverer interface {
	DiscoverAll(ctx context.Context) ([]*Container, error)
}

// ContainerEvent represents a normalized runtime event for container state changes.
// This mirrors runtime.RuntimeEvent but lives in the container package to avoid import cycles.
type ContainerEvent struct {
	Action       string
	ExternalID   string
	Name         string
	ExitCode     string
	HealthStatus string
	ErrorDetail  string
	Timestamp    time.Time
	Labels       map[string]string
}

// LogFetcher abstracts log retrieval.
type LogFetcher interface {
	FetchLogSnippet(ctx context.Context, externalID string) (string, error)
}

// RestartChecker checks restart thresholds.
type RestartChecker interface {
	Check(ctx context.Context, c *Container) (interface{}, error)
}

// EventCallback is called when a container event occurs (for SSE broadcasting).
type EventCallback func(eventType string, data interface{})

// Service orchestrates container discovery, event processing, and persistence.
type Service struct {
	store          ContainerStore
	logger         *slog.Logger
	onEvent        EventCallback
	logFetcher     LogFetcher
	restartChecker RestartChecker
	discoverer     RuntimeDiscoverer
}

// NewService creates a new container service.
func NewService(store ContainerStore, logger *slog.Logger) *Service {
	return &Service{
		store:  store,
		logger: logger,
	}
}

// SetEventCallback sets the callback for broadcasting container events.
func (s *Service) SetEventCallback(cb EventCallback) {
	s.onEvent = cb
}

// SetLogFetcher sets the log fetcher for capturing log snippets.
func (s *Service) SetLogFetcher(lf LogFetcher) {
	s.logFetcher = lf
}

// SetRestartChecker sets the restart threshold checker.
func (s *Service) SetRestartChecker(rc RestartChecker) {
	s.restartChecker = rc
}

// SetDiscoverer stores the runtime discoverer so the service can auto-discover
// containers that start after the initial reconciliation.
func (s *Service) SetDiscoverer(d RuntimeDiscoverer) {
	s.discoverer = d
}

// ProcessEvent handles a single container/workload event.
func (s *Service) ProcessEvent(ctx context.Context, evt ContainerEvent) {
	switch evt.Action {
	case "start":
		s.handleStateChange(ctx, evt, StateRunning)
	case "stop":
		s.handleStateChange(ctx, evt, StateExited)
	case "die":
		// Exit code 0 means the container terminated normally (e.g. migration, init container).
		// Use StateCompleted so it is not treated as an error in monitoring.
		if evt.ExitCode != "" && parseExitCode(evt.ExitCode) == 0 {
			s.handleStateChange(ctx, evt, StateCompleted)
		} else {
			s.handleStateChange(ctx, evt, StateExited)
		}
	case "kill":
		s.handleStateChange(ctx, evt, StateExited)
	case "pause":
		s.handleStateChange(ctx, evt, StatePaused)
	case "unpause":
		s.handleStateChange(ctx, evt, StateRunning)
	case "destroy":
		s.handleDestroy(ctx, evt)
	case "health_status":
		s.handleHealthChange(ctx, evt)
	}
}

func (s *Service) handleStateChange(ctx context.Context, evt ContainerEvent, newState ContainerState) {
	c, err := s.store.GetContainerByExternalID(ctx, evt.ExternalID)
	if err != nil {
		s.logger.Error("get container for state change", "external_id", evt.ExternalID[:12], "error", err)
		return
	}
	if c == nil {
		if newState != StateRunning || s.discoverer == nil {
			s.logger.Debug("unknown container event, skipping", "external_id", evt.ExternalID[:12], "action", evt.Action)
			return
		}
		// New container started after initial reconciliation — discover it.
		s.logger.Info("new container detected, running reconciliation", "external_id", evt.ExternalID[:12])
		if err := s.Reconcile(ctx, s.discoverer); err != nil {
			s.logger.Error("on-demand reconciliation failed", "error", err)
		}
		return
	}

	previousState := c.State

	// Skip no-op transitions (e.g. start event when already marked running)
	if previousState == newState {
		return
	}

	c.State = newState
	c.LastStateChangeAt = evt.Timestamp

	if err := s.store.UpdateContainer(ctx, c); err != nil {
		s.logger.Error("update container state", "id", c.ID, "error", err)
		return
	}

	// Record transition
	transition := &StateTransition{
		ContainerID:   c.ID,
		PreviousState: previousState,
		NewState:      newState,
		Timestamp:     evt.Timestamp,
	}

	if evt.ExitCode != "" {
		ec := parseExitCode(evt.ExitCode)
		transition.ExitCode = &ec
	}

	// Capture log snippet on die events with non-zero exit code (T028)
	if evt.Action == "die" && s.logFetcher != nil {
		snippet, err := s.logFetcher.FetchLogSnippet(ctx, evt.ExternalID)
		if err != nil {
			s.logger.Warn("fetch log snippet", "external_id", evt.ExternalID[:12], "error", err)
		} else {
			transition.LogSnippet = snippet
		}
	}

	if _, err := s.store.InsertTransition(ctx, transition); err != nil {
		s.logger.Error("insert transition", "container_id", c.ID, "error", err)
	}

	// Check restart threshold (T030)
	if newState == StateRunning && previousState == StateRestarting && s.restartChecker != nil {
		alert, err := s.restartChecker.Check(ctx, c)
		if err != nil {
			s.logger.Error("restart check", "container_id", c.ID, "error", err)
		} else if alert != nil {
			s.emitEvent("container.restart_alert", alert)
		}
	}

	s.emitEvent("container.state_changed", map[string]interface{}{
		"id":             c.ID,
		"state":          newState,
		"previous_state": previousState,
		"health_status":  c.HealthStatus,
		"exit_code":      transition.ExitCode,
		"timestamp":      evt.Timestamp,
	})
}

func (s *Service) handleDestroy(ctx context.Context, evt ContainerEvent) {
	c, err := s.store.GetContainerByExternalID(ctx, evt.ExternalID)
	if err != nil {
		s.logger.Error("get container for destroy", "external_id", evt.ExternalID[:12], "error", err)
		return
	}
	if c == nil {
		return
	}

	now := evt.Timestamp
	if err := s.store.ArchiveContainer(ctx, evt.ExternalID, now); err != nil {
		s.logger.Error("archive container", "external_id", evt.ExternalID[:12], "error", err)
		return
	}

	s.logger.Info("archived container", "id", c.ID, "name", c.Name)
	s.emitEvent("container.archived", map[string]interface{}{
		"id":          c.ID,
		"archived_at": now,
	})
}

func (s *Service) handleHealthChange(ctx context.Context, evt ContainerEvent) {
	c, err := s.store.GetContainerByExternalID(ctx, evt.ExternalID)
	if err != nil {
		s.logger.Error("get container for health change", "external_id", evt.ExternalID[:12], "error", err)
		return
	}
	if c == nil {
		return
	}

	previousHealth := c.HealthStatus
	newHealth := HealthStatus(evt.HealthStatus)
	c.HealthStatus = &newHealth
	c.LastStateChangeAt = evt.Timestamp

	if err := s.store.UpdateContainer(ctx, c); err != nil {
		s.logger.Error("update container health", "id", c.ID, "error", err)
		return
	}

	transition := &StateTransition{
		ContainerID:   c.ID,
		PreviousState: c.State,
		NewState:      c.State,
		PreviousHealth: previousHealth,
		NewHealth:      &newHealth,
		Timestamp:     evt.Timestamp,
	}
	if _, err := s.store.InsertTransition(ctx, transition); err != nil {
		s.logger.Error("insert health transition", "container_id", c.ID, "error", err)
	}

	s.emitEvent("container.health_changed", map[string]interface{}{
		"id":              c.ID,
		"health_status":   newHealth,
		"previous_health": previousHealth,
		"timestamp":       evt.Timestamp,
	})
}

func (s *Service) emitEvent(eventType string, data interface{}) {
	if s.onEvent != nil {
		s.onEvent(eventType, data)
	}
}

// GetContainer retrieves a container by its PulseBoard ID.
func (s *Service) GetContainer(ctx context.Context, id int64) (*Container, error) {
	return s.store.GetContainerByID(ctx, id)
}

// ListContainers returns containers matching the given options.
func (s *Service) ListContainers(ctx context.Context, opts ListContainersOpts) ([]*Container, error) {
	return s.store.ListContainers(ctx, opts)
}

// Reconcile compares stored container states with actual runtime state
// and generates synthetic transitions for changes that occurred while PulseBoard was offline.
func (s *Service) Reconcile(ctx context.Context, discoverer RuntimeDiscoverer) error {
	current, err := discoverer.DiscoverAll(ctx)
	if err != nil {
		return fmt.Errorf("reconcile discover: %w", err)
	}

	currentByExternalID := make(map[string]*Container, len(current))
	for _, c := range current {
		currentByExternalID[c.ExternalID] = c
	}

	// Get stored containers
	stored, err := s.store.ListContainers(ctx, ListContainersOpts{IncludeArchived: false, IncludeIgnored: true})
	if err != nil {
		return fmt.Errorf("reconcile list stored: %w", err)
	}

	now := time.Now()

	for _, sc := range stored {
		dc, exists := currentByExternalID[sc.ExternalID]
		if !exists {
			// Container was removed while PulseBoard was offline — archive it
			if err := s.store.ArchiveContainer(ctx, sc.ExternalID, now); err != nil {
				s.logger.Error("reconcile archive", "external_id", sc.ExternalID, "error", err)
			}
			s.emitEvent("container.archived", map[string]interface{}{
				"id": sc.ID, "archived_at": now,
			})
			continue
		}

		// Check for state changes
		if sc.State != dc.State {
			transition := &StateTransition{
				ContainerID:   sc.ID,
				PreviousState: sc.State,
				NewState:      dc.State,
				Timestamp:     now,
			}
			if _, err := s.store.InsertTransition(ctx, transition); err != nil {
				s.logger.Error("reconcile transition", "container_id", sc.ID, "error", err)
			}

			sc.State = dc.State
			sc.LastStateChangeAt = now
			if err := s.store.UpdateContainer(ctx, sc); err != nil {
				s.logger.Error("reconcile update", "container_id", sc.ID, "error", err)
			}

			s.emitEvent("container.state_changed", map[string]interface{}{
				"id": sc.ID, "state": dc.State, "previous_state": sc.State, "timestamp": now,
			})
		}
	}

	// Discover new containers that appeared while offline
	for _, dc := range current {
		found := false
		for _, sc := range stored {
			if sc.ExternalID == dc.ExternalID {
				found = true
				break
			}
		}
		if !found {
			id, err := s.store.InsertContainer(ctx, dc)
			if err != nil {
				s.logger.Error("reconcile insert new", "external_id", dc.ExternalID, "error", err)
				continue
			}
			dc.ID = id

			// Record initial state transition so uptime tracking has a starting point
			if _, err := s.store.InsertTransition(ctx, &StateTransition{
				ContainerID:   id,
				PreviousState: StateCreated,
				NewState:      dc.State,
				Timestamp:     now,
			}); err != nil {
				s.logger.Error("reconcile initial transition", "container_id", id, "error", err)
			}

			s.emitEvent("container.discovered", dc)
		}
	}

	return nil
}

// ListContainersGrouped returns containers organized into groups.
func (s *Service) ListContainersGrouped(ctx context.Context, opts ListContainersOpts) ([]*ContainerGroup, int, int, error) {
	containers, err := s.store.ListContainers(ctx, opts)
	if err != nil {
		return nil, 0, 0, err
	}

	groupMap := make(map[string]*ContainerGroup)
	var groupOrder []string
	var archivedCount int

	for _, c := range containers {
		if c.Archived {
			archivedCount++
		}
		name := c.GroupName()
		if _, ok := groupMap[name]; !ok {
			groupMap[name] = &ContainerGroup{
				Name:   name,
				Source: c.GroupSource(),
			}
			groupOrder = append(groupOrder, name)
		}
		groupMap[name].Containers = append(groupMap[name].Containers, c)
	}

	groups := make([]*ContainerGroup, 0, len(groupOrder))
	for _, name := range groupOrder {
		groups = append(groups, groupMap[name])
	}

	return groups, len(containers), archivedCount, nil
}

// ListTransitions returns state transitions for a container.
func (s *Service) ListTransitions(ctx context.Context, containerID int64, opts ListTransitionsOpts) ([]*StateTransition, int, error) {
	return s.store.ListTransitionsByContainer(ctx, containerID, opts)
}

func parseExitCode(s string) int {
	var ec int
	fmt.Sscanf(s, "%d", &ec)
	return ec
}

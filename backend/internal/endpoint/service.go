package endpoint

import (
	"context"
	"log/slog"
	"time"
)

// EventCallback is called when an endpoint event occurs (for SSE broadcasting).
type EventCallback func(eventType string, data interface{})

// AlertCallback is called after ProcessCheckResult to evaluate alert thresholds.
// It receives the updated endpoint and the check result. It should return an event type
// ("endpoint.alert" or "endpoint.recovery") and event data, or empty string if no alert.
type AlertCallback func(ep *Endpoint, result CheckResult) (eventType string, eventData interface{})

// Service orchestrates endpoint discovery, persistence, and the check engine.
type Service struct {
	store         EndpointStore
	engine        *CheckEngine
	logger        *slog.Logger
	onEvent       EventCallback
	alertCallback AlertCallback
	ctx           context.Context
}

// NewService creates a new endpoint service.
func NewService(store EndpointStore, engine *CheckEngine, logger *slog.Logger) *Service {
	return &Service{
		store:  store,
		engine: engine,
		logger: logger,
	}
}

// SetEventCallback sets the callback for broadcasting endpoint events.
func (s *Service) SetEventCallback(cb EventCallback) {
	s.onEvent = cb
}

// SetAlertCallback sets the callback for evaluating alert thresholds on check results.
func (s *Service) SetAlertCallback(cb AlertCallback) {
	s.alertCallback = cb
}

// Start begins the check engine and stores the context for adding endpoints later.
func (s *Service) Start(ctx context.Context) {
	s.ctx = ctx
}

// Stop shuts down the check engine.
func (s *Service) Stop() {
	s.engine.Stop()
}

// SyncEndpoints synchronizes endpoint definitions from container labels with the store and check engine.
func (s *Service) SyncEndpoints(ctx context.Context, containerName, externalID string, labels map[string]string, orchestrationGroup, orchestrationUnit string) {
	parsed, parseErrors := ParseEndpointLabels(labels, s.logger)

	// Emit config errors
	for _, pe := range parseErrors {
		s.emitEvent("endpoint.config_error", map[string]interface{}{
			"endpoint_id":    nil,
			"container_name": containerName,
			"label_key":      pe.LabelKey,
			"error":          pe.Message,
			"timestamp":      time.Now(),
		})
	}

	// Get currently stored endpoints for this container
	existing, err := s.store.ListEndpointsByExternalID(ctx, externalID)
	if err != nil {
		s.logger.Error("list endpoints by external ID", "external_id", externalID, "error", err)
		return
	}

	// Build maps for comparison
	existingByKey := make(map[string]*Endpoint, len(existing))
	for _, ep := range existing {
		existingByKey[ep.LabelKey] = ep
	}

	parsedKeys := make(map[string]bool, len(parsed))

	for _, p := range parsed {
		parsedKeys[p.LabelKey] = true

		ep := &Endpoint{
			ContainerName:      containerName,
			LabelKey:           p.LabelKey,
			ExternalID:         externalID,
			EndpointType:       p.EndpointType,
			Target:             p.Target,
			Config:             p.Config,
			OrchestrationGroup: orchestrationGroup,
			OrchestrationUnit:  orchestrationUnit,
		}

		id, err := s.store.UpsertEndpoint(ctx, ep)
		if err != nil {
			s.logger.Error("upsert endpoint", "container", containerName, "label", p.LabelKey, "error", err)
			continue
		}
		ep.ID = id

		// Reload full endpoint from store to get current status/counters
		full, err := s.store.GetEndpointByID(ctx, id)
		if err != nil || full == nil {
			s.logger.Error("reload endpoint after upsert", "id", id, "error", err)
			continue
		}

		// Check if this is new (not in existing map) or reconfigured
		prev, wasExisting := existingByKey[p.LabelKey]
		if !wasExisting {
			s.emitEvent("endpoint.discovered", map[string]interface{}{
				"endpoint_id":    id,
				"container_name": containerName,
				"endpoint_type":  string(p.EndpointType),
				"target":         p.Target,
			})
		} else if prev.Target != p.Target || prev.EndpointType != p.EndpointType {
			// Target or type changed — reconfigure
			ClearLinkLocalWarning(id)
		}

		// Start or reconfigure check
		if s.ctx != nil {
			s.engine.ReconfigureEndpoint(s.ctx, full)
		}
	}

	// Deactivate endpoints that are no longer in labels
	for key, ep := range existingByKey {
		if !parsedKeys[key] {
			if err := s.store.DeactivateEndpoint(ctx, ep.ID); err != nil {
				s.logger.Error("deactivate endpoint", "id", ep.ID, "error", err)
				continue
			}
			s.engine.RemoveEndpoint(ep.ID)
			s.emitEvent("endpoint.removed", map[string]interface{}{
				"endpoint_id":    ep.ID,
				"container_name": containerName,
				"reason":         "label_removed",
			})
		}
	}
}

// ProcessCheckResult handles a check result: updates the endpoint state and persists the result.
func (s *Service) ProcessCheckResult(ctx context.Context, endpointID int64, result CheckResult) {
	ep, err := s.store.GetEndpointByID(ctx, endpointID)
	if err != nil || ep == nil {
		s.logger.Error("get endpoint for check result", "endpoint_id", endpointID, "error", err)
		return
	}

	previousStatus := ep.Status

	var newStatus EndpointStatus
	if result.Success {
		newStatus = StatusUp
		ep.ConsecutiveSuccesses++
		ep.ConsecutiveFailures = 0
	} else {
		newStatus = StatusDown
		ep.ConsecutiveFailures++
		ep.ConsecutiveSuccesses = 0
	}

	if err := s.store.UpdateCheckResult(ctx, endpointID, newStatus, ep.AlertState,
		ep.ConsecutiveFailures, ep.ConsecutiveSuccesses,
		result.ResponseTimeMs, result.HTTPStatus, result.ErrorMessage); err != nil {
		s.logger.Error("update check result on endpoint", "endpoint_id", endpointID, "error", err)
	}

	if _, err := s.store.InsertCheckResult(ctx, &result); err != nil {
		s.logger.Error("insert check result", "endpoint_id", endpointID, "error", err)
	}

	// Emit status change if applicable
	if newStatus != previousStatus {
		s.emitEvent("endpoint.status_changed", map[string]interface{}{
			"endpoint_id":      endpointID,
			"container_name":   ep.ContainerName,
			"target":           ep.Target,
			"previous_status":  string(previousStatus),
			"new_status":       string(newStatus),
			"response_time_ms": result.ResponseTimeMs,
			"http_status":      result.HTTPStatus,
			"error":            result.ErrorMessage,
			"timestamp":        result.Timestamp,
		})
	}

	// Evaluate alert thresholds
	if s.alertCallback != nil {
		// Reload endpoint with updated counters
		updated, err := s.store.GetEndpointByID(ctx, endpointID)
		if err == nil && updated != nil {
			if eventType, eventData := s.alertCallback(updated, result); eventType != "" {
				// Update alert state in store
				newAlertState := updated.AlertState
				if eventType == "endpoint.alert" {
					newAlertState = AlertAlerting
				} else if eventType == "endpoint.recovery" {
					newAlertState = AlertNormal
				}
				if err := s.store.UpdateCheckResult(ctx, endpointID, newStatus, newAlertState,
					updated.ConsecutiveFailures, updated.ConsecutiveSuccesses,
					result.ResponseTimeMs, result.HTTPStatus, result.ErrorMessage); err != nil {
					s.logger.Error("update alert state", "endpoint_id", endpointID, "error", err)
				}
				s.emitEvent(eventType, eventData)
			}
		}
	}
}

// HandleContainerStop pauses checks and sets endpoints to unknown for a stopped container.
func (s *Service) HandleContainerStop(ctx context.Context, externalID string) {
	endpoints, err := s.store.ListEndpointsByExternalID(ctx, externalID)
	if err != nil {
		s.logger.Error("list endpoints for container stop", "external_id", externalID, "error", err)
		return
	}

	for _, ep := range endpoints {
		s.engine.RemoveEndpoint(ep.ID)
		if err := s.store.UpdateCheckResult(ctx, ep.ID, StatusUnknown, ep.AlertState,
			ep.ConsecutiveFailures, ep.ConsecutiveSuccesses,
			0, nil, "container stopped"); err != nil {
			s.logger.Error("set endpoint unknown on container stop", "endpoint_id", ep.ID, "error", err)
		}
		s.emitEvent("endpoint.status_changed", map[string]interface{}{
			"endpoint_id":     ep.ID,
			"container_name":  ep.ContainerName,
			"target":          ep.Target,
			"previous_status": string(ep.Status),
			"new_status":      string(StatusUnknown),
			"error":           "container stopped",
			"timestamp":       time.Now(),
		})
	}
}

// HandleContainerStart re-syncs endpoint labels and resumes checks when a container starts.
func (s *Service) HandleContainerStart(ctx context.Context, containerName, externalID string, labels map[string]string, orchestrationGroup, orchestrationUnit string) {
	s.SyncEndpoints(ctx, containerName, externalID, labels, orchestrationGroup, orchestrationUnit)
}

// HandleContainerDestroy deactivates all endpoints for a destroyed container.
func (s *Service) HandleContainerDestroy(ctx context.Context, externalID string) {
	endpoints, err := s.store.ListEndpointsByExternalID(ctx, externalID)
	if err != nil {
		s.logger.Error("list endpoints for container destroy", "external_id", externalID, "error", err)
		return
	}

	for _, ep := range endpoints {
		s.engine.RemoveEndpoint(ep.ID)
		if err := s.store.DeactivateEndpoint(ctx, ep.ID); err != nil {
			s.logger.Error("deactivate endpoint on destroy", "endpoint_id", ep.ID, "error", err)
		}
		s.emitEvent("endpoint.removed", map[string]interface{}{
			"endpoint_id":    ep.ID,
			"container_name": ep.ContainerName,
			"reason":         "container_destroyed",
		})
	}
}

// ListEndpoints returns endpoints matching the given options.
func (s *Service) ListEndpoints(ctx context.Context, opts ListEndpointsOpts) ([]*Endpoint, error) {
	return s.store.ListEndpoints(ctx, opts)
}

// GetEndpoint retrieves an endpoint by ID.
func (s *Service) GetEndpoint(ctx context.Context, id int64) (*Endpoint, error) {
	return s.store.GetEndpointByID(ctx, id)
}

// ListCheckResults returns check results for an endpoint.
func (s *Service) ListCheckResults(ctx context.Context, endpointID int64, opts ListChecksOpts) ([]*CheckResult, int, error) {
	return s.store.ListCheckResults(ctx, endpointID, opts)
}

// CalculateUptime computes uptime percentages for an endpoint across multiple time windows.
func (s *Service) CalculateUptime(ctx context.Context, endpointID int64) map[string]float64 {
	now := time.Now()
	windows := map[string]time.Duration{
		"1h":  1 * time.Hour,
		"24h": 24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
	}

	uptimes := make(map[string]float64, len(windows))
	for label, dur := range windows {
		from := now.Add(-dur)
		total, successes, err := s.store.GetCheckResultsInWindow(ctx, endpointID, from, now)
		if err != nil || total == 0 {
			uptimes[label] = 0
			continue
		}
		uptimes[label] = float64(successes) / float64(total) * 100
	}
	return uptimes
}

func (s *Service) emitEvent(eventType string, data interface{}) {
	if s.onEvent != nil {
		s.onEvent(eventType, data)
	}
}

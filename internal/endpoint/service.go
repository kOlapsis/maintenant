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

package endpoint

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/uid"
)

var (
	ErrNotStandalone    = errors.New("endpoint is not standalone")
	ErrEndpointLive     = errors.New("endpoint is still discovered from container labels")
	ErrEndpointNotFound = errors.New("endpoint not found")
	ErrLimitReached     = errors.New("endpoint limit reached")
	// ErrAgentProbed rejects an on-demand check for an endpoint the server never
	// dials itself: it lives on an agent's network, and that agent probes it.
	ErrAgentProbed = errors.New("endpoint is probed by a remote agent")
	// ErrCheckInProgress rejects a second on-demand check while one is running,
	// so a stuck button cannot turn the server into a load generator.
	ErrCheckInProgress = errors.New("a check is already running for this endpoint")
)

// LicenseChecker determines license-gated capabilities for endpoints.
type LicenseChecker interface {
	CanCreateEndpoint(currentCount int) bool
}

// DefaultLicenseChecker caps how many endpoints may exist. A negative maximum
// means unlimited — the value comes straight from extension.Limit, where -1 is
// the declared "no cap" value.
type DefaultLicenseChecker struct {
	MaxEndpoints int
}

func (c *DefaultLicenseChecker) CanCreateEndpoint(currentCount int) bool {
	if c.MaxEndpoints < 0 {
		return true
	}
	return currentCount < c.MaxEndpoints
}

// EventCallback is called when an endpoint event occurs (for SSE broadcasting).
type EventCallback func(eventType string, data interface{})

// AlertCallback is called after ProcessCheckResult to evaluate alert thresholds.
// It receives the updated endpoint and the check result. It should return an event type
// ("endpoint.alert" or "endpoint.recovery") and event data, or empty string if no alert.
type AlertCallback func(ep *Endpoint, result CheckResult) (eventType string, eventData interface{})

// EndpointRemovedCallback is called when an endpoint is deactivated (label removed or container destroyed).
type EndpointRemovedCallback func(ctx context.Context, endpointID string)

// Deps holds all dependencies for the endpoint Service.
type Deps struct {
	Store                   EndpointStore           // required
	Engine                  *CheckEngine            // required
	Logger                  *slog.Logger            // required
	LicenseChecker          LicenseChecker          // optional — defaults to community limits
	EventCallback           EventCallback           // optional — nil-safe
	AlertCallback           AlertCallback           // optional — nil-safe
	EndpointRemovedCallback EndpointRemovedCallback // optional — nil-safe
}

// Service orchestrates endpoint discovery, persistence, and the check engine.
type Service struct {
	store             EndpointStore
	engine            *CheckEngine
	logger            *slog.Logger
	licenseChecker    LicenseChecker
	onEvent           EventCallback
	alertCallback     AlertCallback
	onEndpointRemoved EndpointRemovedCallback
	ctx               context.Context

	// manualChecks holds the endpoint ids with an on-demand check in flight.
	manualChecks sync.Map
}

// NewService creates a new endpoint service with all dependencies.
func NewService(d Deps) *Service {
	if d.Store == nil {
		panic("endpoint.NewService: Store is required")
	}
	if d.Engine == nil {
		panic("endpoint.NewService: Engine is required")
	}
	if d.Logger == nil {
		panic("endpoint.NewService: Logger is required")
	}
	lc := d.LicenseChecker
	if lc == nil {
		lc = &DefaultLicenseChecker{MaxEndpoints: 10}
	}
	return &Service{
		store:             d.Store,
		engine:            d.Engine,
		logger:            d.Logger,
		licenseChecker:    lc,
		onEvent:           d.EventCallback,
		alertCallback:     d.AlertCallback,
		onEndpointRemoved: d.EndpointRemovedCallback,
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

// SetEndpointRemovedCallback sets the callback for when an endpoint is deactivated.
func (s *Service) SetEndpointRemovedCallback(cb EndpointRemovedCallback) {
	s.onEndpointRemoved = cb
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
	s.logger.Debug("endpoint: sync started", "external_id", externalID, "count", len(parsed))

	// Emit config errors
	for _, pe := range parseErrors {
		s.emitEvent(event.EndpointConfigError, map[string]interface{}{
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
			s.emitEvent(event.EndpointDiscovered, map[string]interface{}{
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
			if s.onEndpointRemoved != nil {
				s.onEndpointRemoved(ctx, ep.ID)
			}
			s.emitEvent(event.EndpointRemoved, map[string]interface{}{
				"endpoint_id":    ep.ID,
				"container_name": containerName,
				"reason":         "label_removed",
			})
		}
	}
}

// SyncAgentEndpoints provisions label-discovered endpoints for a REMOTE agent's
// container. Mirrors SyncEndpoints but (a) attributes every endpoint to agentID
// and (b) never enrolls them in the local check engine — the agent probes them
// on its own host and pushes results (FR-018a). Reconciles removed labels.
func (s *Service) SyncAgentEndpoints(ctx context.Context, agentID, containerName, externalID string, labels map[string]string) {
	parsed, parseErrors := ParseEndpointLabels(labels, s.logger)
	for _, pe := range parseErrors {
		s.emitEvent(event.EndpointConfigError, map[string]interface{}{
			"endpoint_id":    nil,
			"container_name": containerName,
			"label_key":      pe.LabelKey,
			"error":          pe.Message,
			"agent_id":       agentID,
			"timestamp":      time.Now(),
		})
	}

	// external_id is unique per container, so this scopes to this agent's container.
	existing, err := s.store.ListEndpointsByExternalID(ctx, externalID)
	if err != nil {
		s.logger.Error("list agent endpoints by external ID", "external_id", externalID, "error", err)
		return
	}
	existingByKey := make(map[string]*Endpoint, len(existing))
	for _, ep := range existing {
		existingByKey[ep.LabelKey] = ep
	}
	parsedKeys := make(map[string]bool, len(parsed))

	for _, p := range parsed {
		parsedKeys[p.LabelKey] = true
		ep := &Endpoint{
			ContainerName: containerName,
			LabelKey:      p.LabelKey,
			ExternalID:    externalID,
			EndpointType:  p.EndpointType,
			Target:        p.Target,
			Config:        p.Config,
			Source:        SourceLabel,
			AgentID:       agentID,
		}
		id, err := s.store.UpsertEndpoint(ctx, ep)
		if err != nil {
			s.logger.Error("upsert agent endpoint", "container", containerName, "label", p.LabelKey, "agent_id", agentID, "error", err)
			continue
		}
		if _, wasExisting := existingByKey[p.LabelKey]; !wasExisting {
			s.emitEvent(event.EndpointDiscovered, map[string]interface{}{
				"endpoint_id":    id,
				"container_name": containerName,
				"endpoint_type":  string(p.EndpointType),
				"target":         p.Target,
				"agent_id":       agentID,
			})
		}
		// Intentionally NOT enrolled in s.engine: the agent owns probing.
	}

	// Deactivate endpoints whose label was removed.
	for key, ep := range existingByKey {
		if parsedKeys[key] {
			continue
		}
		if err := s.store.DeactivateEndpoint(ctx, ep.ID); err != nil {
			s.logger.Error("deactivate agent endpoint", "id", ep.ID, "error", err)
			continue
		}
		if s.onEndpointRemoved != nil {
			s.onEndpointRemoved(ctx, ep.ID)
		}
		s.emitEvent(event.EndpointRemoved, map[string]interface{}{
			"endpoint_id":    ep.ID,
			"container_name": containerName,
			"reason":         "label_removed",
			"agent_id":       agentID,
		})
	}
}

// statusForResult maps a probe outcome to the endpoint's reported status. A
// degraded host is reachable and serving, so it stays a success everywhere that
// matters — uptime, consecutive counters, outage alerting — and differs only in
// the status it shows.
func statusForResult(result CheckResult) EndpointStatus {
	switch {
	case result.Success && result.Degraded:
		return StatusDegraded
	case result.Success:
		return StatusUp
	default:
		return StatusDown
	}
}

// CheckNow probes an endpoint immediately and processes the result exactly like a
// scheduled run: same persistence, same status-change event, same alert evaluation.
// It backs the UI's refresh button, so a target fixed by hand stops alerting right
// away instead of at the next scheduled check.
//
// Only endpoints the server probes itself can be checked this way; an agent's
// endpoints are unreachable from here, and the agent re-probes them on its own
// short cycle anyway.
func (s *Service) CheckNow(ctx context.Context, endpointID string) (*Endpoint, error) {
	ep, err := s.store.GetEndpointByID(ctx, endpointID)
	if err != nil {
		return nil, fmt.Errorf("get endpoint: %w", err)
	}
	if ep == nil {
		return nil, ErrEndpointNotFound
	}
	if ep.AgentID != "" && ep.AgentID != uid.LocalAgent {
		return nil, ErrAgentProbed
	}

	if _, running := s.manualChecks.LoadOrStore(endpointID, struct{}{}); running {
		return nil, ErrCheckInProgress
	}
	defer s.manualChecks.Delete(endpointID)

	var result CheckResult
	switch ep.EndpointType {
	case TypeHTTP:
		result = CheckHTTP(ctx, ep, s.logger)
	case TypeTCP:
		result = CheckTCP(ctx, ep, s.logger)
	default:
		return nil, fmt.Errorf("unsupported endpoint type %q", ep.EndpointType)
	}

	s.logger.Info("endpoint: on-demand check", "endpoint_id", endpointID,
		"target", ep.Target, "success", result.Success)
	s.ProcessCheckResult(ctx, endpointID, result)

	return s.store.GetEndpointByID(ctx, endpointID)
}

// ProcessCheckResult handles a check result: updates the endpoint state and persists the result.
func (s *Service) ProcessCheckResult(ctx context.Context, endpointID string, result CheckResult) {
	ep, err := s.store.GetEndpointByID(ctx, endpointID)
	if err != nil || ep == nil {
		s.logger.Error("get endpoint for check result", "endpoint_id", endpointID, "error", err)
		return
	}

	previousStatus := ep.Status

	newStatus := statusForResult(result)
	if result.Success {
		ep.ConsecutiveSuccesses++
		ep.ConsecutiveFailures = 0
	} else {
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

	s.logger.Debug("endpoint: check result processed", "endpoint_id", endpointID, "target", ep.Target, "success", result.Success, "response_time_ms", result.ResponseTimeMs, "status", string(newStatus))

	if newStatus != previousStatus {
		s.logger.Debug("endpoint: status changed", "endpoint_id", endpointID, "previous_status", string(previousStatus), "new_status", string(newStatus))
		s.emitEvent(event.EndpointStatusChanged, map[string]interface{}{
			"endpoint_id":      endpointID,
			"container_name":   ep.ContainerName,
			"target":           ep.Target,
			"previous_status":  string(previousStatus),
			"new_status":       string(newStatus),
			"response_time_ms": result.ResponseTimeMs,
			"http_status":      result.HTTPStatus,
			"error":            result.ErrorMessage,
			"timestamp":        result.Timestamp,
			"agent_id":         result.AgentID,
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
				switch eventType {
				case "endpoint.alert":
					newAlertState = AlertAlerting
				case "endpoint.recovery":
					newAlertState = AlertNormal
				}
				if err := s.store.UpdateCheckResult(ctx, endpointID, newStatus, newAlertState,
					updated.ConsecutiveFailures, updated.ConsecutiveSuccesses,
					result.ResponseTimeMs, result.HTTPStatus, result.ErrorMessage); err != nil {
					s.logger.Error("update alert state", "endpoint_id", endpointID, "error", err)
				}
				s.logger.Debug("endpoint: alert triggered", "endpoint_id", endpointID, "event_type", eventType)
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
		s.logger.Debug("endpoint: pausing check for stopped container", "endpoint_id", ep.ID)
		s.engine.RemoveEndpoint(ep.ID)
		if err := s.store.UpdateCheckResult(ctx, ep.ID, StatusUnknown, ep.AlertState,
			ep.ConsecutiveFailures, ep.ConsecutiveSuccesses,
			0, nil, "container stopped"); err != nil {
			s.logger.Error("set endpoint unknown on container stop", "endpoint_id", ep.ID, "error", err)
		}
		s.emitEvent(event.EndpointStatusChanged, map[string]interface{}{
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
		s.logger.Debug("endpoint: deactivating endpoint", "endpoint_id", ep.ID)
		s.engine.RemoveEndpoint(ep.ID)
		if err := s.store.DeactivateEndpoint(ctx, ep.ID); err != nil {
			s.logger.Error("deactivate endpoint on destroy", "endpoint_id", ep.ID, "error", err)
		}
		if s.onEndpointRemoved != nil {
			s.onEndpointRemoved(ctx, ep.ID)
		}
		s.emitEvent(event.EndpointRemoved, map[string]interface{}{
			"endpoint_id":    ep.ID,
			"container_name": ep.ContainerName,
			"reason":         "container_destroyed",
		})
	}
}

// SweepOrphanedLabelEndpoints deactivates label-discovered endpoints whose
// container is absent from a full discovery pass, and returns how many it
// deactivated.
//
// The destroy event is what normally retires them, but an event that arrives
// while the process is down is an event nobody hears, and the instance is
// meant to be stopped for a storage migration, which is exactly when one-off
// containers come and go. Without this sweep such an endpoint stays active
// forever, reporting a container that no longer exists.
func (s *Service) SweepOrphanedLabelEndpoints(ctx context.Context, seen map[string]struct{}) int {
	endpoints, err := s.store.ListEndpoints(ctx, ListEndpointsOpts{Source: string(SourceLabel)})
	if err != nil {
		s.logger.Error("list label endpoints for orphan sweep", "error", err)
		return 0
	}

	swept := 0
	for _, ep := range endpoints {
		if _, ok := seen[ep.ExternalID]; ok {
			continue
		}
		s.engine.RemoveEndpoint(ep.ID)
		if err := s.store.DeactivateEndpoint(ctx, ep.ID); err != nil {
			s.logger.Error("deactivate orphaned endpoint", "endpoint_id", ep.ID, "error", err)
			continue
		}
		if s.onEndpointRemoved != nil {
			s.onEndpointRemoved(ctx, ep.ID)
		}
		s.emitEvent(event.EndpointRemoved, map[string]interface{}{
			"endpoint_id":    ep.ID,
			"container_name": ep.ContainerName,
			"reason":         "container_gone",
		})
		swept++
	}
	return swept
}

// ListEndpoints returns endpoints matching the given options.
func (s *Service) ListEndpoints(ctx context.Context, opts ListEndpointsOpts) ([]*Endpoint, error) {
	return s.store.ListEndpoints(ctx, opts)
}

// CountActiveEndpoints returns the count of all active endpoints (both standalone and label-discovered).
func (s *Service) CountActiveEndpoints(ctx context.Context) (int, error) {
	return s.store.CountActiveEndpoints(ctx)
}

// GetEndpoint retrieves an endpoint by ID.
func (s *Service) GetEndpoint(ctx context.Context, id string) (*Endpoint, error) {
	return s.store.GetEndpointByID(ctx, id)
}

// ListCheckResults returns check results for an endpoint.
func (s *Service) ListCheckResults(ctx context.Context, endpointID string, opts ListChecksOpts) ([]*CheckResult, int, error) {
	return s.store.ListCheckResults(ctx, endpointID, opts)
}

// CalculateUptime computes uptime percentages for an endpoint across multiple time windows.
func (s *Service) CalculateUptime(ctx context.Context, endpointID string) map[string]float64 {
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

// CreateStandalone creates a manually-defined endpoint and starts monitoring it.
func (s *Service) CreateStandalone(ctx context.Context, name, target string, epType EndpointType, config EndpointConfig) (*Endpoint, error) {
	count, err := s.store.CountActiveEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("count endpoints: %w", err)
	}
	if !s.licenseChecker.CanCreateEndpoint(count) {
		return nil, ErrLimitReached
	}

	ep := &Endpoint{
		Name:         name,
		Target:       target,
		EndpointType: epType,
		Config:       config,
		Source:       SourceStandalone,
	}

	id, err := s.store.InsertStandaloneEndpoint(ctx, ep)
	if err != nil {
		return nil, fmt.Errorf("create standalone endpoint: %w", err)
	}

	full, err := s.store.GetEndpointByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reload standalone endpoint: %w", err)
	}

	if s.ctx != nil {
		s.engine.AddEndpoint(s.ctx, full)
	}

	s.emitEvent(event.EndpointDiscovered, map[string]interface{}{
		"endpoint_id":   id,
		"endpoint_type": string(epType),
		"target":        target,
		"source":        "standalone",
		"name":          name,
	})

	return full, nil
}

// UpdateStandalone updates a standalone endpoint's configuration and restarts monitoring.
func (s *Service) UpdateStandalone(ctx context.Context, id string, name, target string, epType EndpointType, config EndpointConfig) (*Endpoint, error) {
	existing, err := s.store.GetEndpointByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get endpoint: %w", err)
	}
	if existing == nil {
		return nil, ErrEndpointNotFound
	}
	if existing.Source != SourceStandalone {
		return nil, ErrNotStandalone
	}

	configJSON := (&Endpoint{Config: config}).ConfigJSON()
	if err := s.store.UpdateStandaloneEndpoint(ctx, id, name, target, epType, configJSON); err != nil {
		return nil, err
	}

	full, err := s.store.GetEndpointByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reload updated endpoint: %w", err)
	}

	if s.ctx != nil {
		s.engine.ReconfigureEndpoint(s.ctx, full)
	}

	return full, nil
}

// Delete removes an endpoint and stops monitoring it.
//
// A standalone endpoint is the operator's to delete at any time. A
// label-discovered one is the container's, and is refused while that container
// is still around: the next discovery pass would recreate it, so the deletion
// would be theatre. Once the container is gone the endpoint is deactivated and
// nothing will bring it back, and then it is deletable. Otherwise an endpoint
// orphaned by a container that vanished while the instance was down could only
// be removed with SQL.
func (s *Service) Delete(ctx context.Context, id string) error {
	existing, err := s.store.GetEndpointByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get endpoint: %w", err)
	}
	if existing == nil {
		return ErrEndpointNotFound
	}
	if existing.Source != SourceStandalone && existing.Active {
		return ErrEndpointLive
	}

	s.engine.RemoveEndpoint(id)

	if s.onEndpointRemoved != nil {
		s.onEndpointRemoved(ctx, id)
	}

	if err := s.store.DeleteEndpoint(ctx, id); err != nil {
		return err
	}

	s.emitEvent(event.EndpointRemoved, map[string]interface{}{
		"endpoint_id": id,
		"reason":      "user_deleted",
	})

	return nil
}

func (s *Service) emitEvent(eventType string, data interface{}) {
	if s.onEvent != nil {
		s.onEvent(eventType, data)
	}
}

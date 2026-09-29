// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	esc "github.com/kolapsis/maintenant/internal/alert/escalation"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
)

var _ esc.Service = (*Service)(nil)

// Service is the CE-side escalation CRUD service.
// It is the single write point for escalation data in CE; all reads/writes go through here.
// The Pro concrete Escalator handles runtime orchestration (timing, dispatch, state transitions).
//
// Maintenance window contract: the active↔paused_by_maintenance status transitions are the
// exclusive responsibility of the concrete Pro Escalator. This CE service only persists
// transitions and guarantees SQL CHECK constraints. The Pro Escalator calls
// store methods directly to update run status; this service provides IsAlertSuppressed
// as a helper to query the maintenance suppressor.
//
// Edition downgrade contract: when Pro→CE transition is detected, OnEditionDowngraded deactivates
// all active policies (preserving active_before_downgrade for restore) and stops active runs.
type Service struct {
	store        esc.Store
	channelStore alert.ChannelStore
	edition      func() extension.Edition
	suppressor   alert.MaintenanceSuppressor
	logger       *slog.Logger
	clockFn      func() time.Time
}

// NewService constructs a new escalation Service.
func NewService(
	store esc.Store,
	channelStore alert.ChannelStore,
	edition func() extension.Edition,
	suppressor alert.MaintenanceSuppressor,
	logger *slog.Logger,
) *Service {
	if suppressor == nil {
		suppressor = noopRunnerSuppressor{}
	}
	return &Service{
		store:        store,
		channelStore: channelStore,
		edition:      edition,
		suppressor:   suppressor,
		logger:       logger,
	}
}

// SetClockFn overrides the clock used by RunRetentionLoop. For testing only.
func (s *Service) SetClockFn(fn func() time.Time) {
	s.clockFn = fn
}

// IsAlertSuppressed delegates to the maintenance suppressor for a given alert.
// The concrete Pro Escalator uses this to determine if a run should be paused.
func (s *Service) IsAlertSuppressed(ctx context.Context, alertID string) (bool, error) {
	return s.suppressor.IsSuppressed(ctx, "", "", alertID)
}

// OnEditionDowngraded deactivates all active policies and stops all active runs.
// Called when the edition transitions from Pro to CE.
func (s *Service) OnEditionDowngraded(ctx context.Context) error {
	if err := s.store.BulkDeactivateAllPolicies(ctx); err != nil {
		return fmt.Errorf("downgrade: deactivate policies: %w", err)
	}
	if err := s.store.BulkStopActiveRuns(ctx, "stopped_by_edition_downgrade", time.Now()); err != nil {
		return fmt.Errorf("downgrade: stop active runs: %w", err)
	}
	s.logger.Info("escalation: policies deactivated due to edition downgrade")
	return nil
}

// OnEditionUpgraded restores policies that were active before the last downgrade.
func (s *Service) OnEditionUpgraded(ctx context.Context) error {
	if err := s.store.BulkRestorePoliciesFromDowngrade(ctx); err != nil {
		return fmt.Errorf("upgrade: restore policies: %w", err)
	}
	s.logger.Info("escalation: policies restored after edition upgrade")
	return nil
}

// GetPlanLimits returns escalation usage stats. Pro is unlimited; CE has no
// access to escalation (routes are gated by requireCapability upstream), so
// the only signal exposed here is the current active count.
func (s *Service) GetPlanLimits(ctx context.Context) (esc.Limits, error) {
	current, err := s.store.CountActivePolicies(ctx)
	if err != nil {
		return esc.Limits{}, fmt.Errorf("get plan limits: %w", err)
	}
	return esc.Limits{
		MaxActive:     -1,
		MaxLevels:     -1,
		CurrentActive: current,
	}, nil
}

// CreatePolicy validates and persists a new escalation policy.
func (s *Service) CreatePolicy(ctx context.Context, req esc.PolicyRequest) (*esc.Policy, error) {
	// Validate name
	if req.Name == "" {
		return nil, fmt.Errorf("field=name: %w", esc.ErrValidationFailed)
	}
	if len(req.Name) > 120 {
		return nil, fmt.Errorf("field=name: name must be 120 characters or fewer: %w", esc.ErrValidationFailed)
	}

	// Validate levels count
	if len(req.Levels) < 1 {
		return nil, fmt.Errorf("field=levels: at least one level is required: %w", esc.ErrValidationFailed)
	}

	// Validate each level
	for i, lvl := range req.Levels {
		if lvl.DelaySeconds < 60 || lvl.DelaySeconds > 86400 {
			return nil, fmt.Errorf("field=levels[%d].delay_seconds: must be between 60 and 86400: %w", i, esc.ErrValidationFailed)
		}
		if len(lvl.ChannelIDs) == 0 {
			return nil, fmt.Errorf("field=levels[%d].channel_ids: at least one channel is required: %w", i, esc.ErrValidationFailed)
		}
		// Consecutive levels must be at least 60s apart
		if i > 0 && req.Levels[i].DelaySeconds-req.Levels[i-1].DelaySeconds < 60 {
			return nil, fmt.Errorf("field=levels[%d].delay_seconds: must be at least 60 seconds after the previous level: %w", i, esc.ErrValidationFailed)
		}
	}

	// Build levels with assigned order
	levels := make([]esc.Level, len(req.Levels))
	for i, lvl := range req.Levels {
		levels[i] = esc.Level{
			Order:        i,
			DelaySeconds: lvl.DelaySeconds,
			ChannelIDs:   lvl.ChannelIDs,
		}
	}

	now := time.Now().UTC()
	p := &esc.Policy{
		Name:      req.Name,
		Active:    req.Active,
		Filters:   req.Filters,
		Levels:    levels,
		CreatedAt: now,
		UpdatedAt: now,
	}

	id, err := s.store.InsertPolicy(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("create policy: insert: %w", err)
	}
	p.ID = id

	s.logger.Info("escalation: policy created", "id", id, "name", p.Name, "active", p.Active)
	return p, nil
}

// GetPolicy retrieves a policy by ID. Returns (nil, ErrPolicyNotFound) if not found.
func (s *Service) GetPolicy(ctx context.Context, id string) (*esc.Policy, error) {
	p, err := s.store.SelectPolicy(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get policy %s: %w", id, err)
	}
	if p == nil {
		return nil, esc.ErrPolicyNotFound
	}
	return p, nil
}

// ListPolicies returns all policies, optionally filtered by active status.
func (s *Service) ListPolicies(ctx context.Context, activeOnly bool) ([]*esc.Policy, error) {
	policies, err := s.store.SelectPolicies(ctx, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	if policies == nil {
		policies = []*esc.Policy{}
	}
	return policies, nil
}

// DeletePolicy removes a policy and stops active runs (stopped_by_policy_deletion).
func (s *Service) DeletePolicy(ctx context.Context, id string) error {
	p, err := s.store.SelectPolicy(ctx, id)
	if err != nil {
		return fmt.Errorf("delete policy: lookup: %w", err)
	}
	if p == nil {
		return esc.ErrPolicyNotFound
	}
	if err := s.store.DeletePolicy(ctx, id); err != nil {
		return fmt.Errorf("delete policy %s: %w", id, err)
	}
	s.logger.Info("escalation: policy deleted", "id", id)
	return nil
}

// UpdatePolicy validates and updates an existing escalation policy (last-write-wins).
func (s *Service) UpdatePolicy(ctx context.Context, id string, req esc.PolicyRequest) (*esc.Policy, error) {
	existing, err := s.store.SelectPolicy(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update policy: lookup: %w", err)
	}
	if existing == nil {
		return nil, esc.ErrPolicyNotFound
	}

	if req.Name == "" {
		return nil, fmt.Errorf("field=name: %w", esc.ErrValidationFailed)
	}
	if len(req.Name) > 120 {
		return nil, fmt.Errorf("field=name: name must be 120 characters or fewer: %w", esc.ErrValidationFailed)
	}
	if len(req.Levels) < 1 {
		return nil, fmt.Errorf("field=levels: at least one level is required: %w", esc.ErrValidationFailed)
	}
	for i, lvl := range req.Levels {
		if lvl.DelaySeconds < 60 || lvl.DelaySeconds > 86400 {
			return nil, fmt.Errorf("field=levels[%d].delay_seconds: must be between 60 and 86400: %w", i, esc.ErrValidationFailed)
		}
		if len(lvl.ChannelIDs) == 0 {
			return nil, fmt.Errorf("field=levels[%d].channel_ids: at least one channel is required: %w", i, esc.ErrValidationFailed)
		}
		if i > 0 && req.Levels[i].DelaySeconds-req.Levels[i-1].DelaySeconds < 60 {
			return nil, fmt.Errorf("field=levels[%d].delay_seconds: must be at least 60 seconds after the previous level: %w", i, esc.ErrValidationFailed)
		}
	}

	levels := make([]esc.Level, len(req.Levels))
	for i, lvl := range req.Levels {
		levels[i] = esc.Level{Order: i, DelaySeconds: lvl.DelaySeconds, ChannelIDs: lvl.ChannelIDs}
	}

	existing.Name = req.Name
	existing.Active = req.Active
	existing.Filters = req.Filters
	existing.Levels = levels
	existing.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdatePolicy(ctx, existing); err != nil {
		return nil, fmt.Errorf("update policy %s: %w", id, err)
	}

	s.logger.Info("escalation: policy updated", "id", id, "name", existing.Name, "active", existing.Active)
	return existing, nil
}

// SetPolicyActive activates or deactivates a policy.
func (s *Service) SetPolicyActive(ctx context.Context, id string, active bool) (*esc.Policy, error) {
	existing, err := s.store.SelectPolicy(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("set policy active: lookup: %w", err)
	}
	if existing == nil {
		return nil, esc.ErrPolicyNotFound
	}

	existing.Active = active
	existing.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdatePolicy(ctx, existing); err != nil {
		return nil, fmt.Errorf("set policy active %s: %w", id, err)
	}

	s.logger.Info("escalation: policy active status changed", "id", id, "active", active)
	return existing, nil
}

// ListRunsForAlert returns runs attached to an alert.
func (s *Service) ListRunsForAlert(ctx context.Context, alertID string) ([]*esc.Run, error) {
	runs, err := s.store.SelectRunsByAlert(ctx, alertID)
	if err != nil {
		return nil, fmt.Errorf("list runs for alert %s: %w", alertID, err)
	}
	if runs == nil {
		runs = []*esc.Run{}
	}
	return runs, nil
}

// GetRun returns a run with its deliveries. Returns (nil, ErrRunNotFound) if not found.
func (s *Service) GetRun(ctx context.Context, id string) (*esc.Run, error) {
	r, err := s.store.SelectRun(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get run %s: %w", id, err)
	}
	if r == nil {
		return nil, esc.ErrRunNotFound
	}
	deliveries, err := s.store.SelectRunDeliveries(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get run deliveries %s: %w", id, err)
	}
	r.Deliveries = deliveries
	return r, nil
}

// ListPolicyRuns returns paginated runs for a policy.
func (s *Service) ListPolicyRuns(ctx context.Context, policyID string, limit int, cursor string) ([]*esc.Run, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	runs, err := s.store.SelectRunsByPolicy(ctx, policyID, limit, cursor)
	if err != nil {
		return nil, fmt.Errorf("list policy runs %s: %w", policyID, err)
	}
	if runs == nil {
		runs = []*esc.Run{}
	}
	return runs, nil
}

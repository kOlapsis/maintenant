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

package escalation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
)

// Sentinel errors returned by Service methods.
var (
	ErrValidationFailed = errors.New("validation_failed")
	ErrPlanLimitReached = errors.New("plan_limit_reached")
	ErrPolicyNotFound   = errors.New("policy_not_found")
	ErrRunNotFound      = errors.New("run_not_found")
)

// PolicyRequest is the input for creating or updating a policy.
type PolicyRequest struct {
	Name    string     `json:"name"`
	Active  bool       `json:"active"`
	Filters Filters    `json:"filters"`
	Levels  []LevelReq `json:"levels"`
}

// LevelReq is one escalation step in a request (order is assigned by service).
type LevelReq struct {
	DelaySeconds int     `json:"delay_seconds"`
	ChannelIDs   []int64 `json:"channel_ids"`
}

// Store defines the persistence interface for escalation data.
type Store interface {
	InsertPolicy(ctx context.Context, p *Policy) (int64, error)
	UpdatePolicy(ctx context.Context, p *Policy) error
	SelectPolicy(ctx context.Context, id int64) (*Policy, error)
	SelectPolicies(ctx context.Context, activeOnly bool) ([]*Policy, error)
	DeletePolicy(ctx context.Context, id int64) error
	CountActivePolicies(ctx context.Context) (int, error)
	SelectRun(ctx context.Context, id int64) (*Run, error)
	SelectRunsByAlert(ctx context.Context, alertID int64) ([]*Run, error)
	SelectRunsByPolicy(ctx context.Context, policyID int64, limit int, cursor int64) ([]*Run, error)
	SelectRunDeliveries(ctx context.Context, runID int64) ([]*Delivery, error)
	BulkDeactivateAllPolicies(ctx context.Context) error
	BulkRestorePoliciesFromDowngrade(ctx context.Context) error
	BulkStopActiveRuns(ctx context.Context, stopStatus string, endedAt time.Time) error
	PurgeRunsAndDeliveriesOlderThan(ctx context.Context, before time.Time) error
}

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
	store        Store
	channelStore alert.ChannelStore
	edition      func() extension.Edition
	planTier     func() extension.PlanTier
	suppressor   alert.MaintenanceSuppressor
	logger       *slog.Logger
	clockFn      func() time.Time
}

// NewService constructs a new escalation Service.
func NewService(
	store Store,
	channelStore alert.ChannelStore,
	edition func() extension.Edition,
	planTier func() extension.PlanTier,
	suppressor alert.MaintenanceSuppressor,
	logger *slog.Logger,
) *Service {
	return &Service{
		store:        store,
		channelStore: channelStore,
		edition:      edition,
		planTier:     planTier,
		suppressor:   suppressor,
		logger:       logger,
	}
}

// IsAlertSuppressed delegates to the maintenance suppressor for a given alert.
// The concrete Pro Escalator uses this to determine if a run should be paused.
func (s *Service) IsAlertSuppressed(ctx context.Context, alertID int64) (bool, error) {
	return s.suppressor.IsSuppressed(ctx, "", "", fmt.Sprint(alertID))
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

// planLimits returns the (maxActive, maxLevels) constraints for the current plan tier.
func planLimits(tier extension.PlanTier) (maxActive, maxLevels int) {
	switch tier {
	case extension.PlanTierSolo:
		return 3, 3
	case extension.PlanTierTeam:
		return 25, 5
	case extension.PlanTierBusiness:
		return 200, 5
	default: // PlanTierNone / CE
		return 0, 0
	}
}

// GetPlanLimits returns the plan limits and current active count for the current tier.
func (s *Service) GetPlanLimits(ctx context.Context) (Limits, error) {
	maxActive, maxLevels := planLimits(s.planTier())
	current, err := s.store.CountActivePolicies(ctx)
	if err != nil {
		return Limits{}, fmt.Errorf("get plan limits: %w", err)
	}
	return Limits{
		MaxActive:     maxActive,
		MaxLevels:     maxLevels,
		CurrentActive: current,
	}, nil
}

// CreatePolicy validates and persists a new escalation policy.
func (s *Service) CreatePolicy(ctx context.Context, req PolicyRequest) (*Policy, error) {
	maxActive, maxLevels := planLimits(s.planTier())

	// Validate name
	if req.Name == "" {
		return nil, fmt.Errorf("field=name: %w", ErrValidationFailed)
	}
	if len(req.Name) > 120 {
		return nil, fmt.Errorf("field=name: name must be 120 characters or fewer: %w", ErrValidationFailed)
	}

	// Validate levels count
	if len(req.Levels) < 1 {
		return nil, fmt.Errorf("field=levels: at least one level is required: %w", ErrValidationFailed)
	}
	if len(req.Levels) > maxLevels {
		return nil, fmt.Errorf("field=levels: plan allows at most %d levels: %w", maxLevels, ErrValidationFailed)
	}

	// Validate each level
	for i, lvl := range req.Levels {
		if lvl.DelaySeconds < 60 || lvl.DelaySeconds > 86400 {
			return nil, fmt.Errorf("field=levels[%d].delay_seconds: must be between 60 and 86400: %w", i, ErrValidationFailed)
		}
		if len(lvl.ChannelIDs) == 0 {
			return nil, fmt.Errorf("field=levels[%d].channel_ids: at least one channel is required: %w", i, ErrValidationFailed)
		}
		// Consecutive levels must be at least 60s apart
		if i > 0 && req.Levels[i].DelaySeconds-req.Levels[i-1].DelaySeconds < 60 {
			return nil, fmt.Errorf("field=levels[%d].delay_seconds: must be at least 60 seconds after the previous level: %w", i, ErrValidationFailed)
		}
	}

	// Check plan limit for active policies
	if req.Active {
		current, err := s.store.CountActivePolicies(ctx)
		if err != nil {
			return nil, fmt.Errorf("create policy: count active: %w", err)
		}
		if current >= maxActive {
			return nil, fmt.Errorf("plan allows at most %d active policies: %w", maxActive, ErrPlanLimitReached)
		}
	}

	// Build levels with assigned order
	levels := make([]Level, len(req.Levels))
	for i, lvl := range req.Levels {
		levels[i] = Level{
			Order:        i,
			DelaySeconds: lvl.DelaySeconds,
			ChannelIDs:   lvl.ChannelIDs,
		}
	}

	now := time.Now().UTC()
	p := &Policy{
		Name:    req.Name,
		Active:  req.Active,
		Filters: req.Filters,
		Levels:  levels,
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
func (s *Service) GetPolicy(ctx context.Context, id int64) (*Policy, error) {
	p, err := s.store.SelectPolicy(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get policy %d: %w", id, err)
	}
	if p == nil {
		return nil, ErrPolicyNotFound
	}
	return p, nil
}

// ListPolicies returns all policies, optionally filtered by active status.
func (s *Service) ListPolicies(ctx context.Context, activeOnly bool) ([]*Policy, error) {
	policies, err := s.store.SelectPolicies(ctx, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	if policies == nil {
		policies = []*Policy{}
	}
	return policies, nil
}

// DeletePolicy removes a policy and stops active runs (stopped_by_policy_deletion).
func (s *Service) DeletePolicy(ctx context.Context, id int64) error {
	p, err := s.store.SelectPolicy(ctx, id)
	if err != nil {
		return fmt.Errorf("delete policy: lookup: %w", err)
	}
	if p == nil {
		return ErrPolicyNotFound
	}
	if err := s.store.DeletePolicy(ctx, id); err != nil {
		return fmt.Errorf("delete policy %d: %w", id, err)
	}
	s.logger.Info("escalation: policy deleted", "id", id)
	return nil
}

// UpdatePolicy validates and updates an existing escalation policy (last-write-wins).
func (s *Service) UpdatePolicy(ctx context.Context, id int64, req PolicyRequest) (*Policy, error) {
	maxActive, maxLevels := planLimits(s.planTier())

	existing, err := s.store.SelectPolicy(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update policy: lookup: %w", err)
	}
	if existing == nil {
		return nil, ErrPolicyNotFound
	}

	if req.Name == "" {
		return nil, fmt.Errorf("field=name: %w", ErrValidationFailed)
	}
	if len(req.Name) > 120 {
		return nil, fmt.Errorf("field=name: name must be 120 characters or fewer: %w", ErrValidationFailed)
	}
	if len(req.Levels) < 1 {
		return nil, fmt.Errorf("field=levels: at least one level is required: %w", ErrValidationFailed)
	}
	if len(req.Levels) > maxLevels {
		return nil, fmt.Errorf("field=levels: plan allows at most %d levels: %w", maxLevels, ErrValidationFailed)
	}
	for i, lvl := range req.Levels {
		if lvl.DelaySeconds < 60 || lvl.DelaySeconds > 86400 {
			return nil, fmt.Errorf("field=levels[%d].delay_seconds: must be between 60 and 86400: %w", i, ErrValidationFailed)
		}
		if len(lvl.ChannelIDs) == 0 {
			return nil, fmt.Errorf("field=levels[%d].channel_ids: at least one channel is required: %w", i, ErrValidationFailed)
		}
		if i > 0 && req.Levels[i].DelaySeconds-req.Levels[i-1].DelaySeconds < 60 {
			return nil, fmt.Errorf("field=levels[%d].delay_seconds: must be at least 60 seconds after the previous level: %w", i, ErrValidationFailed)
		}
	}

	if req.Active && !existing.Active {
		current, err := s.store.CountActivePolicies(ctx)
		if err != nil {
			return nil, fmt.Errorf("update policy: count active: %w", err)
		}
		if current >= maxActive {
			return nil, fmt.Errorf("plan allows at most %d active policies: %w", maxActive, ErrPlanLimitReached)
		}
	}

	levels := make([]Level, len(req.Levels))
	for i, lvl := range req.Levels {
		levels[i] = Level{Order: i, DelaySeconds: lvl.DelaySeconds, ChannelIDs: lvl.ChannelIDs}
	}

	existing.Name = req.Name
	existing.Active = req.Active
	existing.Filters = req.Filters
	existing.Levels = levels
	existing.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdatePolicy(ctx, existing); err != nil {
		return nil, fmt.Errorf("update policy %d: %w", id, err)
	}

	s.logger.Info("escalation: policy updated", "id", id, "name", existing.Name, "active", existing.Active)
	return existing, nil
}

// SetPolicyActive activates or deactivates a policy.
func (s *Service) SetPolicyActive(ctx context.Context, id int64, active bool) (*Policy, error) {
	maxActive, _ := planLimits(s.planTier())

	existing, err := s.store.SelectPolicy(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("set policy active: lookup: %w", err)
	}
	if existing == nil {
		return nil, ErrPolicyNotFound
	}

	if active && !existing.Active {
		current, err := s.store.CountActivePolicies(ctx)
		if err != nil {
			return nil, fmt.Errorf("set policy active: count active: %w", err)
		}
		if current >= maxActive {
			return nil, fmt.Errorf("plan allows at most %d active policies: %w", maxActive, ErrPlanLimitReached)
		}
	}

	existing.Active = active
	existing.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdatePolicy(ctx, existing); err != nil {
		return nil, fmt.Errorf("set policy active %d: %w", id, err)
	}

	s.logger.Info("escalation: policy active status changed", "id", id, "active", active)
	return existing, nil
}

// ListRunsForAlert returns runs attached to an alert.
func (s *Service) ListRunsForAlert(ctx context.Context, alertID int64) ([]*Run, error) {
	runs, err := s.store.SelectRunsByAlert(ctx, alertID)
	if err != nil {
		return nil, fmt.Errorf("list runs for alert %d: %w", alertID, err)
	}
	if runs == nil {
		runs = []*Run{}
	}
	return runs, nil
}

// GetRun returns a run with its deliveries. Returns (nil, ErrRunNotFound) if not found.
func (s *Service) GetRun(ctx context.Context, id int64) (*Run, error) {
	r, err := s.store.SelectRun(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get run %d: %w", id, err)
	}
	if r == nil {
		return nil, ErrRunNotFound
	}
	deliveries, err := s.store.SelectRunDeliveries(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get run deliveries %d: %w", id, err)
	}
	r.Deliveries = deliveries
	return r, nil
}

// ListPolicyRuns returns paginated runs for a policy.
func (s *Service) ListPolicyRuns(ctx context.Context, policyID int64, limit int, cursor int64) ([]*Run, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	runs, err := s.store.SelectRunsByPolicy(ctx, policyID, limit, cursor)
	if err != nil {
		return nil, fmt.Errorf("list policy runs %d: %w", policyID, err)
	}
	if runs == nil {
		runs = []*Run{}
	}
	return runs, nil
}
